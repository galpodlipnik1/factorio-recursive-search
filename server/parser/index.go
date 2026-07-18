package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const IndexSchemaVersion = 2

// IndexPayload is the compact, consumer-owned representation embedded in the
// Recursive Blueprint Finder mod. It intentionally excludes full blueprint
// entities, tiles, wires, and schedules.
type IndexPayload struct {
	SchemaVersion int          `json:"schema_version"`
	Entries       []IndexEntry `json:"entries"`
}

// IndexEntry contains only the data needed to search, browse, and resolve a
// live record in a player's blueprint library.
type IndexEntry struct {
	Path              []uint32        `json:"path"`
	PathKey           string          `json:"path_key"`
	ParentPathKey     string          `json:"parent_path_key,omitempty"`
	RecordType        string          `json:"record_type"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Breadcrumb        string          `json:"breadcrumb"`
	SearchName        string          `json:"search_name"`
	SearchDescription string          `json:"search_description"`
	SearchBreadcrumb  string          `json:"search_breadcrumb"`
	SearchText        string          `json:"search_text"`
	ChildPathKeys     []string        `json:"child_path_keys"`
	IconSprite        string          `json:"icon_sprite,omitempty"`
	EntityCount       int             `json:"entity_count"`
	EntityNames       []string        `json:"entity_names"`
	Tags              map[string]any  `json:"tags"`
	Planner           *PlannerDetails `json:"planner,omitempty"`
}

// PlannerDetails stores the searchable subset of deconstruction and upgrade
// planner settings. Fields not applicable to a planner kind remain empty.
type PlannerDetails struct {
	EntityFilters     []PlannerFilter `json:"entity_filters,omitempty"`
	TileFilters       []PlannerFilter `json:"tile_filters,omitempty"`
	EntityFilterMode  string          `json:"entity_filter_mode,omitempty"`
	TileFilterMode    string          `json:"tile_filter_mode,omitempty"`
	TileSelectionMode string          `json:"tile_selection_mode,omitempty"`
	TreesAndRocksOnly bool            `json:"trees_and_rocks_only,omitempty"`
	Mappers           []PlannerMapper `json:"mappers,omitempty"`
}

type PlannerFilter struct {
	Index      uint32 `json:"index"`
	Type       string `json:"type,omitempty"`
	Name       string `json:"name,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Comparator string `json:"comparator,omitempty"`
}

type PlannerMapper struct {
	Index uint32                 `json:"index"`
	From  *PlannerMapperEndpoint `json:"from,omitempty"`
	To    *PlannerMapperEndpoint `json:"to,omitempty"`
}

type PlannerMapperEndpoint struct {
	Type         string          `json:"type,omitempty"`
	Name         string          `json:"name,omitempty"`
	Quality      string          `json:"quality,omitempty"`
	Comparator   string          `json:"comparator,omitempty"`
	ModuleFilter *PlannerFilter  `json:"module_filter,omitempty"`
	ModuleLimit  uint32          `json:"module_limit,omitempty"`
	ModuleSlots  []PlannerFilter `json:"module_slots,omitempty"`
}

// RenderLuaModule renders schema version 2 as deterministic, parseable Lua.
func RenderLuaModule(payload IndexPayload) string {
	var builder strings.Builder
	payload.SchemaVersion = IndexSchemaVersion

	builder.WriteString("return {\n")
	builder.WriteString("  schema_version = ")
	builder.WriteString(strconv.Itoa(payload.SchemaVersion))
	builder.WriteString(",\n")
	builder.WriteString("  entries = {\n")

	for index := range payload.Entries {
		writeEntry(&builder, payload.Entries[index])
	}

	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String()
}

func writeEntry(builder *strings.Builder, entry IndexEntry) {
	builder.WriteString("    {\n")
	writeLuaField(builder, 6, "path", renderUint32Array(entry.Path))
	writeLuaField(builder, 6, "path_key", renderLuaString(entry.PathKey))
	writeLuaField(builder, 6, "parent_path_key", renderOptionalString(entry.ParentPathKey))
	writeLuaField(builder, 6, "record_type", renderLuaString(entry.RecordType))
	writeLuaField(builder, 6, "name", renderLuaString(entry.Name))
	writeLuaField(builder, 6, "description", renderLuaString(entry.Description))
	writeLuaField(builder, 6, "breadcrumb", renderLuaString(entry.Breadcrumb))
	writeLuaField(builder, 6, "search_name", renderLuaString(entry.SearchName))
	writeLuaField(builder, 6, "search_description", renderLuaString(entry.SearchDescription))
	writeLuaField(builder, 6, "search_breadcrumb", renderLuaString(entry.SearchBreadcrumb))
	writeLuaField(builder, 6, "search_text", renderLuaString(entry.SearchText))
	writeLuaField(builder, 6, "label_resolved", "true")
	writeLuaField(builder, 6, "description_resolved", "true")
	// The API output is the complete searchable snapshot consumed by the mod.
	// Runtime metadata work is reserved for the missing/invalid-index fallback.
	writeLuaField(builder, 6, "runtime_semantics_resolved", "true")
	writeLuaField(builder, 6, "child_path_keys", renderStringArray(entry.ChildPathKeys))
	writeLuaField(builder, 6, "icon_sprite", renderOptionalBoolString(entry.IconSprite))
	writeLuaField(builder, 6, "entity_count", strconv.Itoa(entry.EntityCount))
	writeLuaField(builder, 6, "entity_names", renderStringArray(entry.EntityNames))
	writeLuaField(builder, 6, "tags", renderLuaValue(entry.Tags))
	writeLuaField(builder, 6, "planner", renderPlanner(entry.Planner))
	builder.WriteString("    },\n")
}

func writeLuaField(builder *strings.Builder, indent int, name string, value string) {
	builder.WriteString(strings.Repeat(" ", indent))
	builder.WriteString(name)
	builder.WriteString(" = ")
	builder.WriteString(value)
	builder.WriteString(",\n")
}

func renderPlanner(planner *PlannerDetails) string {
	if planner == nil {
		return "nil"
	}
	return renderLuaValue(plannerDetailsToAny(planner))
}

func plannerDetailsToAny(planner *PlannerDetails) any {
	if planner == nil {
		return nil
	}
	values := map[string]any{
		"entity_filters":       plannerFiltersToAny(planner.EntityFilters),
		"tile_filters":         plannerFiltersToAny(planner.TileFilters),
		"trees_and_rocks_only": planner.TreesAndRocksOnly,
		"mappers":              plannerMappersToAny(planner.Mappers),
	}
	putOptionalString(values, "entity_filter_mode", planner.EntityFilterMode)
	putOptionalString(values, "tile_filter_mode", planner.TileFilterMode)
	putOptionalString(values, "tile_selection_mode", planner.TileSelectionMode)
	return values
}

func plannerFiltersToAny(filters []PlannerFilter) []any {
	values := make([]any, 0, len(filters))
	for _, filter := range filters {
		value := map[string]any{"index": filter.Index}
		putOptionalString(value, "type", filter.Type)
		putOptionalString(value, "name", filter.Name)
		putOptionalString(value, "quality", filter.Quality)
		putOptionalString(value, "comparator", filter.Comparator)
		values = append(values, value)
	}
	return values
}

func plannerMappersToAny(mappers []PlannerMapper) []any {
	values := make([]any, 0, len(mappers))
	for _, mapper := range mappers {
		values = append(values, map[string]any{
			"index": mapper.Index,
			"from":  plannerEndpointToAny(mapper.From),
			"to":    plannerEndpointToAny(mapper.To),
		})
	}
	return values
}

func plannerEndpointToAny(endpoint *PlannerMapperEndpoint) any {
	if endpoint == nil {
		return nil
	}
	value := map[string]any{}
	putOptionalString(value, "type", endpoint.Type)
	putOptionalString(value, "name", endpoint.Name)
	putOptionalString(value, "quality", endpoint.Quality)
	putOptionalString(value, "comparator", endpoint.Comparator)
	if endpoint.ModuleFilter != nil {
		value["module_filter"] = plannerFilterToAny(endpoint.ModuleFilter)
	}
	if endpoint.ModuleLimit != 0 {
		value["module_limit"] = endpoint.ModuleLimit
	}
	if len(endpoint.ModuleSlots) > 0 {
		value["module_slots"] = plannerFiltersToAny(endpoint.ModuleSlots)
	}
	return value
}

func plannerFilterToAny(filter *PlannerFilter) any {
	if filter == nil {
		return nil
	}
	value := map[string]any{}
	if filter.Index != 0 {
		value["index"] = filter.Index
	}
	putOptionalString(value, "type", filter.Type)
	putOptionalString(value, "name", filter.Name)
	putOptionalString(value, "quality", filter.Quality)
	putOptionalString(value, "comparator", filter.Comparator)
	return value
}

func putOptionalString(values map[string]any, key string, value string) {
	if value != "" {
		values[key] = value
	}
}

func renderUint32Array(values []uint32) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatUint(uint64(value), 10)
	}
	return renderLuaArray(parts)
}

func renderStringArray(values []string) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = renderLuaString(value)
	}
	return renderLuaArray(parts)
}

func renderLuaArray(values []string) string {
	if len(values) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(values, ", ") + " }"
}

func renderLuaValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "nil"
	case string:
		return renderLuaString(typed)
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case []any:
		values := make([]string, len(typed))
		for index, item := range typed {
			values[index] = renderLuaValue(item)
		}
		return renderLuaArray(values)
	case map[string]any:
		return renderLuaMap(typed)
	default:
		return renderLuaString(fmt.Sprint(typed))
	}
}

func renderLuaMap(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, "["+renderLuaString(key)+"] = "+renderLuaValue(values[key]))
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}

func renderOptionalString(value string) string {
	if value == "" {
		return "nil"
	}
	return renderLuaString(value)
}

func renderOptionalBoolString(value string) string {
	if value == "" {
		return "false"
	}
	return renderLuaString(value)
}

func renderLuaString(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte('"')

	for _, character := range value {
		switch character {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '\a':
			builder.WriteString(`\a`)
		case '\b':
			builder.WriteString(`\b`)
		case '\f':
			builder.WriteString(`\f`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		case '\t':
			builder.WriteString(`\t`)
		case '\v':
			builder.WriteString(`\v`)
		default:
			if character < 0x20 || character == 0x7f {
				builder.WriteByte('\\')
				builder.WriteString(fmt.Sprintf("%03d", character))
				continue
			}
			builder.WriteRune(character)
		}
	}

	builder.WriteByte('"')
	return builder.String()
}
