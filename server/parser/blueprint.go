package parser

import (
	"fmt"
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

func fallbackName(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "(unnamed)"
	}
	return label
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
