package parser

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Entry struct {
	Path              []int
	PathKey           string
	ParentPathKey     string
	RecordType        string
	Name              string
	Description       string
	Breadcrumb        string
	SearchName        string
	SearchDescription string
	SearchBreadcrumb  string
	SearchText        string
	LabelResolved     bool
	ChildPathKeys     []string
	IconSprite        string
	EntityCount       int
	Tags              map[string]any
}

type blueprintEnvelope struct {
	Blueprint     *blueprint     `json:"blueprint"`
	BlueprintBook *blueprintBook `json:"blueprint_book"`
}

type blueprint struct {
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Icons       []icon            `json:"icons"`
	Entities    []json.RawMessage `json:"entities"`
	Tags        map[string]any    `json:"tags"`
}

type blueprintBook struct {
	Label       string             `json:"label"`
	Description string             `json:"description"`
	Icons       []icon             `json:"icons"`
	Blueprints  []indexedBlueprint `json:"blueprints"`
}

type indexedBlueprint struct {
	Index         int            `json:"index"`
	Blueprint     *blueprint     `json:"blueprint"`
	BlueprintBook *blueprintBook `json:"blueprint_book"`
}

type icon struct {
	Signal signal `json:"signal"`
}

type signal struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func BuildModule(exportStrings []string) (string, int, error) {
	entries, err := BuildEntries(exportStrings)
	if err != nil {
		return "", 0, err
	}

	return RenderLuaModule(entries), len(entries), nil
}

func BuildEntries(exportStrings []string) ([]Entry, error) {
	entries := make([]Entry, 0)

	for i, exportString := range exportStrings {
		envelope, err := decodeBlueprintString(exportString)
		if err != nil {
			continue
		}

		path := []int{i + 1}
		appendEnvelopeEntries(&entries, envelope, path, "", nil)
	}

	if len(entries) == 0 {
		return nil, errors.New("no valid blueprint or blueprint_book payloads found")
	}

	return entries, nil
}

func decodeBlueprintString(raw string) (blueprintEnvelope, error) {
	if raw == "" {
		return blueprintEnvelope{}, errors.New("empty export string")
	}
	if raw[0] != '0' {
		return blueprintEnvelope{}, errors.New("unsupported export version")
	}

	decoded, err := decodeBase64(raw[1:])
	if err != nil {
		return blueprintEnvelope{}, fmt.Errorf("base64 decode failed: %w", err)
	}

	reader, err := zlib.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return blueprintEnvelope{}, fmt.Errorf("zlib inflate failed: %w", err)
	}
	defer reader.Close()

	payload, err := io.ReadAll(reader)
	if err != nil {
		return blueprintEnvelope{}, fmt.Errorf("zlib read failed: %w", err)
	}

	var envelope blueprintEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return blueprintEnvelope{}, fmt.Errorf("json parse failed: %w", err)
	}

	if envelope.Blueprint == nil && envelope.BlueprintBook == nil {
		return blueprintEnvelope{}, errors.New("json does not contain blueprint or blueprint_book")
	}

	return envelope, nil
}

func decodeBase64(raw string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return decoded, nil
	}

	return base64.RawStdEncoding.DecodeString(raw)
}

func appendEnvelopeEntries(entries *[]Entry, envelope blueprintEnvelope, path []int, parentPathKey string, breadcrumbs []string) string {
	if envelope.Blueprint != nil {
		entry := buildBlueprintEntry(envelope.Blueprint, path, parentPathKey, breadcrumbs)
		*entries = append(*entries, entry)
		return entry.PathKey
	}

	book := envelope.BlueprintBook
	bookName := fallbackName(book.Label)
	bookBreadcrumbs := append(copyStrings(breadcrumbs), bookName)
	pathKey := joinPath(path)

	entry := Entry{
		Path:              copyInts(path),
		PathKey:           pathKey,
		ParentPathKey:     parentPathKey,
		RecordType:        "blueprint-book",
		Name:              bookName,
		Description:       strings.TrimSpace(book.Description),
		Breadcrumb:        strings.Join(bookBreadcrumbs, " / "),
		SearchName:        normalize(bookName),
		SearchDescription: normalize(book.Description),
		SearchBreadcrumb:  normalize(strings.Join(bookBreadcrumbs, " / ")),
		LabelResolved:     true,
		ChildPathKeys:     []string{},
		IconSprite:        firstIconSprite(book.Icons),
		EntityCount:       0,
		Tags:              map[string]any{},
	}
	entry.SearchText = buildSearchText(entry.Name, entry.Description, entry.Breadcrumb, entry.Tags)

	bookIndex := len(*entries)
	*entries = append(*entries, entry)

	children := make([]indexedBlueprint, len(book.Blueprints))
	copy(children, book.Blueprints)
	sort.SliceStable(children, func(i, j int) bool {
		left := children[i].Index
		right := children[j].Index
		if left == 0 {
			left = i + 1
		}
		if right == 0 {
			right = j + 1
		}
		return left < right
	})

	for i, child := range children {
		slot := child.Index
		if slot == 0 {
			slot = i + 1
		}

		childEnvelope := blueprintEnvelope{
			Blueprint:     child.Blueprint,
			BlueprintBook: child.BlueprintBook,
		}
		if childEnvelope.Blueprint == nil && childEnvelope.BlueprintBook == nil {
			continue
		}

		childPath := append(copyInts(path), slot)
		childPathKey := appendEnvelopeEntries(entries, childEnvelope, childPath, pathKey, bookBreadcrumbs)
		if childPathKey != "" {
			entry.ChildPathKeys = append(entry.ChildPathKeys, childPathKey)
		}
	}

	(*entries)[bookIndex] = entry
	return entry.PathKey
}

func buildBlueprintEntry(bp *blueprint, path []int, parentPathKey string, breadcrumbs []string) Entry {
	name := fallbackName(bp.Label)
	fullBreadcrumbs := append(copyStrings(breadcrumbs), name)
	description := strings.TrimSpace(bp.Description)
	tags := normalizeTags(bp.Tags)

	entry := Entry{
		Path:              copyInts(path),
		PathKey:           joinPath(path),
		ParentPathKey:     parentPathKey,
		RecordType:        "blueprint",
		Name:              name,
		Description:       description,
		Breadcrumb:        strings.Join(fullBreadcrumbs, " / "),
		SearchName:        normalize(name),
		SearchDescription: normalize(description),
		SearchBreadcrumb:  normalize(strings.Join(fullBreadcrumbs, " / ")),
		LabelResolved:     true,
		ChildPathKeys:     []string{},
		IconSprite:        firstIconSprite(bp.Icons),
		EntityCount:       len(bp.Entities),
		Tags:              tags,
	}
	entry.SearchText = buildSearchText(entry.Name, entry.Description, entry.Breadcrumb, entry.Tags)
	return entry
}

func normalizeTags(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	out := make(map[string]any, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case string, bool, float64, nil:
			out[key] = typed
		default:
			out[key] = fmt.Sprint(typed)
		}
	}
	return out
}

func firstIconSprite(icons []icon) string {
	limit := len(icons)
	if limit > 4 {
		limit = 4
	}

	for i := 0; i < limit; i++ {
		icon := icons[i]
		if icon.Signal.Type == "" || icon.Signal.Name == "" {
			continue
		}

		prefix := icon.Signal.Type
		if prefix == "virtual" {
			prefix = "virtual-signal"
		}

		return prefix + "/" + icon.Signal.Name
	}

	return ""
}

func fallbackName(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "(unnamed)"
	}
	return label
}

func buildSearchText(name string, description string, breadcrumb string, tags map[string]any) string {
	parts := []string{name, description, breadcrumb}

	if len(tags) > 0 {
		keys := make([]string, 0, len(tags))
		for key := range tags {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			parts = append(parts, key)
			parts = append(parts, fmt.Sprint(tags[key]))
		}
	}

	return normalize(strings.Join(parts, " "))
}

func normalize(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func joinPath(path []int) string {
	parts := make([]string, len(path))
	for i, part := range path {
		parts[i] = strconv.Itoa(part)
	}
	return strings.Join(parts, ".")
}

func copyInts(values []int) []int {
	out := make([]int, len(values))
	copy(out, values)
	return out
}

func copyStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func RenderLuaModule(entries []Entry) string {
	var builder strings.Builder
	builder.WriteString("return {\n")
	builder.WriteString("  entries = {\n")

	for _, entry := range entries {
		builder.WriteString("    {\n")
		builder.WriteString("      path = " + renderIntArray(entry.Path) + ",\n")
		builder.WriteString("      path_key = " + renderLuaString(entry.PathKey) + ",\n")
		builder.WriteString("      parent_path_key = " + renderOptionalString(entry.ParentPathKey) + ",\n")
		builder.WriteString("      record_type = " + renderLuaString(entry.RecordType) + ",\n")
		builder.WriteString("      name = " + renderLuaString(entry.Name) + ",\n")
		builder.WriteString("      description = " + renderLuaString(entry.Description) + ",\n")
		builder.WriteString("      breadcrumb = " + renderLuaString(entry.Breadcrumb) + ",\n")
		builder.WriteString("      search_name = " + renderLuaString(entry.SearchName) + ",\n")
		builder.WriteString("      search_description = " + renderLuaString(entry.SearchDescription) + ",\n")
		builder.WriteString("      search_breadcrumb = " + renderLuaString(entry.SearchBreadcrumb) + ",\n")
		builder.WriteString("      search_text = " + renderLuaString(entry.SearchText) + ",\n")
		builder.WriteString("      label_resolved = true,\n")
		builder.WriteString("      child_path_keys = " + renderStringArray(entry.ChildPathKeys) + ",\n")
		builder.WriteString("      icon_sprite = " + renderOptionalBoolString(entry.IconSprite) + ",\n")
		builder.WriteString("      entity_count = " + strconv.Itoa(entry.EntityCount) + ",\n")
		builder.WriteString("      tags = " + renderStringKeyedTable(entry.Tags) + ",\n")
		builder.WriteString("    },\n")
	}

	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String()
}

func renderIntArray(values []int) string {
	if len(values) == 0 {
		return "{}"
	}

	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func renderStringArray(values []string) string {
	if len(values) == 0 {
		return "{}"
	}

	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = renderLuaString(value)
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func renderStringKeyedTable(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, "["+renderLuaString(key)+"] = "+renderLuaScalar(values[key]))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
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

func renderLuaScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return "nil"
	case string:
		return renderLuaString(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return renderLuaString(fmt.Sprint(typed))
	}
}

func renderLuaString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r", "\\r")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return `"` + value + `"`
}
