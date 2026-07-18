package parser

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	bpdecode "github.com/galpodlipnik1/factorio-blueprint-decoder"
)

const (
	maxIndexDepth        = 256
	fallbackNameMaxBytes = 48
)

// Build strictly decodes a blueprint storage file and projects it into the
// compact schema consumed by Recursive Blueprint Finder.
func Build(data []byte) (IndexPayload, error) {
	result, err := bpdecode.Decode(data, bpdecode.DecodeOptions{})
	if err != nil {
		return IndexPayload{}, fmt.Errorf("decode blueprint library: %w", err)
	}
	if result.Library == nil {
		return IndexPayload{}, fmt.Errorf("decode blueprint library: decoder returned no library")
	}

	payload, err := BuildLibrary(result.Library)
	if err != nil {
		return IndexPayload{}, fmt.Errorf("project blueprint library: %w", err)
	}
	return payload, nil
}

// BuildLibrary projects a semantic library. It is exported so projection can
// be tested directly without constructing binary fixtures.
func BuildLibrary(library *bpdecode.Library) (IndexPayload, error) {
	if library == nil {
		return IndexPayload{}, fmt.Errorf("library is nil")
	}

	projector := libraryProjector{
		seenPaths: make(map[string]struct{}),
	}
	if err := projector.projectTable(library.Root, nil, nil, "", 0); err != nil {
		return IndexPayload{}, err
	}

	sort.Slice(projector.entries, func(left, right int) bool {
		if projector.entries[left].Breadcrumb != projector.entries[right].Breadcrumb {
			return projector.entries[left].Breadcrumb < projector.entries[right].Breadcrumb
		}
		return compareNumericPath(projector.entries[left].Path, projector.entries[right].Path) < 0
	})

	entryByPath := make(map[string]*IndexEntry, len(projector.entries))
	for index := range projector.entries {
		projector.entries[index].ChildPathKeys = []string{}
		entryByPath[projector.entries[index].PathKey] = &projector.entries[index]
	}
	for index := range projector.entries {
		entry := &projector.entries[index]
		if entry.ParentPathKey == "" {
			continue
		}
		parent := entryByPath[entry.ParentPathKey]
		if parent == nil {
			return IndexPayload{}, fmt.Errorf("entry %q references missing parent %q", entry.PathKey, entry.ParentPathKey)
		}
		parent.ChildPathKeys = append(parent.ChildPathKeys, entry.PathKey)
	}

	return IndexPayload{
		SchemaVersion: IndexSchemaVersion,
		Entries:       projector.entries,
	}, nil
}

type libraryProjector struct {
	entries   []IndexEntry
	seenPaths map[string]struct{}
}

func (p *libraryProjector) projectTable(
	table bpdecode.SlotTable,
	parentPath []uint32,
	parentBreadcrumb []string,
	parentPathKey string,
	depth uint32,
) error {
	if depth >= maxIndexDepth {
		return fmt.Errorf("record nesting exceeds %d levels", maxIndexDepth)
	}

	slots := append([]bpdecode.Slot(nil), table.Slots...)
	sort.Slice(slots, func(left, right int) bool {
		return slots[left].Index < slots[right].Index
	})

	var previousIndex uint32
	for slotPosition := range slots {
		slot := slots[slotPosition]
		if slot.Index == 0 {
			return fmt.Errorf("slot at depth %d has zero index", depth)
		}
		if table.Size != 0 && slot.Index > table.Size {
			return fmt.Errorf("slot %d exceeds table size %d", slot.Index, table.Size)
		}
		if slotPosition > 0 && slot.Index == previousIndex {
			return fmt.Errorf("duplicate slot index %d at depth %d", slot.Index, depth)
		}
		previousIndex = slot.Index

		if err := slot.Record.Validate(); err != nil {
			return fmt.Errorf("slot %d: %w", slot.Index, err)
		}
		if slot.Record.Metadata.Preview {
			continue
		}

		path := appendPath(parentPath, slot.Index)
		pathKey := bpdecode.Path(path).String()
		if _, exists := p.seenPaths[pathKey]; exists {
			return fmt.Errorf("duplicate record path %q", pathKey)
		}
		p.seenPaths[pathKey] = struct{}{}

		entry, err := projectRecord(path, pathKey, parentPathKey, parentBreadcrumb, slot.Record)
		if err != nil {
			return fmt.Errorf("path %s: %w", pathKey, err)
		}
		p.entries = append(p.entries, entry)

		if slot.Record.Book != nil {
			breadcrumb := appendString(parentBreadcrumb, entry.Name)
			if err := p.projectTable(slot.Record.Book.Contents, path, breadcrumb, pathKey, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func projectRecord(
	path []uint32,
	pathKey string,
	parentPathKey string,
	parentBreadcrumb []string,
	record bpdecode.Record,
) (IndexEntry, error) {
	description := strings.TrimSpace(record.Metadata.Description)
	name := strings.TrimSpace(record.Metadata.Label)
	if name == "" {
		name = descriptionNameFallback(string(record.Kind), description)
	}

	breadcrumbParts := appendString(parentBreadcrumb, name)
	breadcrumb := strings.Join(breadcrumbParts, " / ")
	tags := map[string]any{}
	entityCount := 0
	entityNames := []string{}
	if record.Blueprint != nil {
		var err error
		tags, err = blueprintTagsToAny(record.Blueprint)
		if err != nil {
			return IndexEntry{}, fmt.Errorf("convert blueprint tags: %w", err)
		}
		entityCount = int(record.Blueprint.EntityCount)
		if entityCount == 0 {
			entityCount = len(record.Blueprint.Entities)
		}
		entityNames = blueprintEntityNames(record.Blueprint.Entities)
	}

	planner := projectPlanner(record)
	entry := IndexEntry{
		Path:              append([]uint32(nil), path...),
		PathKey:           pathKey,
		ParentPathKey:     parentPathKey,
		RecordType:        string(record.Kind),
		Name:              name,
		Description:       description,
		Breadcrumb:        breadcrumb,
		SearchName:        normalize(name),
		SearchDescription: normalize(description),
		SearchBreadcrumb:  normalize(breadcrumb),
		ChildPathKeys:     []string{},
		IconSprite:        firstIconSprite(record.Metadata.Icons),
		EntityCount:       entityCount,
		EntityNames:       entityNames,
		Tags:              tags,
		Planner:           planner,
	}
	entry.SearchText = buildSearchText(entry, planner)
	return entry, nil
}

func blueprintTagsToAny(blueprint *bpdecode.Blueprint) (map[string]any, error) {
	tags, err := valueMapToAny(blueprint.Tags)
	if err != nil {
		return nil, err
	}

	entityTags := make(map[string]any)
	for index, entity := range blueprint.Entities {
		if len(entity.Tags) == 0 {
			continue
		}
		converted, convertErr := valueMapToAny(entity.Tags)
		if convertErr != nil {
			return nil, fmt.Errorf("entity %d tags: %w", entity.Number, convertErr)
		}
		entityNumber := entity.Number
		if entityNumber == 0 {
			entityNumber = uint64(index + 1)
		}
		entityTags[fmt.Sprint(entityNumber)] = converted
	}
	if len(entityTags) > 0 {
		tags["entities"] = entityTags
	}
	return tags, nil
}

func blueprintEntityNames(entities []bpdecode.Entity) []string {
	seen := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		name := strings.TrimSpace(entity.Name)
		if name != "" {
			seen[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func projectPlanner(record bpdecode.Record) *PlannerDetails {
	if record.DeconstructionPlanner != nil {
		planner := record.DeconstructionPlanner
		return &PlannerDetails{
			EntityFilters:     projectPlannerFilters(planner.EntityFilters),
			TileFilters:       projectPlannerFilters(planner.TileFilters),
			EntityFilterMode:  string(planner.EntityFilterMode),
			TileFilterMode:    string(planner.TileFilterMode),
			TileSelectionMode: string(planner.TileSelectionMode),
			TreesAndRocksOnly: planner.TreesAndRocksOnly,
			Mappers:           []PlannerMapper{},
		}
	}

	if record.UpgradePlanner == nil {
		return nil
	}

	mappers := make([]PlannerMapper, 0, len(record.UpgradePlanner.Mappers))
	for _, mapper := range record.UpgradePlanner.Mappers {
		mappers = append(mappers, PlannerMapper{
			Index: mapper.Index,
			From:  projectUpgradeTarget(mapper.Source),
			To:    projectUpgradeTarget(mapper.Destination),
		})
	}
	return &PlannerDetails{
		EntityFilters: []PlannerFilter{},
		TileFilters:   []PlannerFilter{},
		Mappers:       mappers,
	}
}

func projectPlannerFilters(filters []bpdecode.PlannerFilter) []PlannerFilter {
	projected := make([]PlannerFilter, 0, len(filters))
	for _, filter := range filters {
		projected = append(projected, PlannerFilter{
			Index:      filter.Index,
			Type:       filter.Prototype.Type,
			Name:       filter.Prototype.Name,
			Quality:    string(filter.Prototype.Quality.Normalized()),
			Comparator: string(filter.Comparator),
		})
	}
	sort.Slice(projected, func(left, right int) bool {
		return projected[left].Index < projected[right].Index
	})
	return projected
}

func projectUpgradeTarget(target bpdecode.UpgradeTarget) *PlannerMapperEndpoint {
	if target.Prototype.Type == "" && target.Prototype.Name == "" && target.ModuleFilter == nil && len(target.ModuleSlots) == 0 && target.ModuleLimit == nil {
		return nil
	}

	moduleSlots := make([]PlannerFilter, 0, len(target.ModuleSlots))
	for index, prototype := range target.ModuleSlots {
		moduleSlots = append(moduleSlots, PlannerFilter{
			Index:   uint32(index + 1),
			Type:    prototype.Type,
			Name:    prototype.Name,
			Quality: string(prototype.Quality.Normalized()),
		})
	}

	var moduleLimit uint32
	if target.ModuleLimit != nil {
		moduleLimit = *target.ModuleLimit
	}
	var moduleFilter *PlannerFilter
	if target.ModuleFilter != nil {
		moduleFilter = &PlannerFilter{
			Type:    target.ModuleFilter.Type,
			Name:    target.ModuleFilter.Name,
			Quality: string(target.ModuleFilter.Quality.Normalized()),
		}
	}
	return &PlannerMapperEndpoint{
		Type:         target.Prototype.Type,
		Name:         target.Prototype.Name,
		Quality:      string(target.Prototype.Quality.Normalized()),
		Comparator:   string(target.QualityComparator),
		ModuleFilter: moduleFilter,
		ModuleLimit:  moduleLimit,
		ModuleSlots:  moduleSlots,
	}
}

func valueMapToAny(values bpdecode.ValueMap) (map[string]any, error) {
	converted := make(map[string]any, len(values))
	for key, value := range values {
		item, err := value.Any()
		if err != nil {
			return nil, fmt.Errorf("tag %q: %w", key, err)
		}
		converted[key] = item
	}
	return converted, nil
}

func firstIconSprite(icons []bpdecode.Icon) string {
	if len(icons) == 0 {
		return ""
	}

	sorted := append([]bpdecode.Icon(nil), icons...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Index < sorted[right].Index
	})
	signal := sorted[0].Signal
	if signal.Type == "" || signal.Name == "" {
		return ""
	}
	prefix := string(signal.Type)
	if signal.Type == bpdecode.SignalTypeVirtual {
		prefix = "virtual-signal"
	}
	return prefix + "/" + signal.Name
}

func buildSearchText(entry IndexEntry, planner *PlannerDetails) string {
	parts := []string{entry.Name, entry.Description, entry.Breadcrumb}
	appendSearchValue(&parts, entry.Tags)
	appendSearchValue(&parts, stringSliceToAny(entry.EntityNames))
	appendSearchValue(&parts, plannerDetailsToAny(planner))
	return normalize(strings.Join(parts, " "))
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func appendSearchValue(parts *[]string, value any) {
	switch typed := value.(type) {
	case nil:
		return
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			*parts = append(*parts, key)
			appendSearchValue(parts, typed[key])
		}
	case []any:
		for _, item := range typed {
			appendSearchValue(parts, item)
		}
	default:
		*parts = append(*parts, fmt.Sprint(typed))
	}
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func descriptionNameFallback(recordType string, description string) string {
	compact := strings.Join(strings.Fields(strings.TrimSpace(description)), " ")
	if compact == "" {
		switch recordType {
		case string(bpdecode.RecordKindBlueprintBook):
			return "[Unnamed Book]"
		case string(bpdecode.RecordKindDeconstructionPlanner):
			return "[Unnamed Deconstruction Planner]"
		case string(bpdecode.RecordKindUpgradePlanner):
			return "[Unnamed Upgrade Planner]"
		default:
			return "[Unnamed Blueprint]"
		}
	}
	if len(compact) <= fallbackNameMaxBytes {
		return compact
	}

	end := fallbackNameMaxBytes - 3
	for end > 0 && !utf8.RuneStart(compact[end]) {
		end--
	}
	return compact[:end] + "..."
}

func appendPath(path []uint32, index uint32) []uint32 {
	result := make([]uint32, len(path)+1)
	copy(result, path)
	result[len(path)] = index
	return result
}

func compareNumericPath(left []uint32, right []uint32) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func appendString(values []string, value string) []string {
	result := make([]string, len(values)+1)
	copy(result, values)
	result[len(values)] = value
	return result
}
