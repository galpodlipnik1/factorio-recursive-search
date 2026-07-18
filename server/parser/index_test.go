package parser

import (
	"strings"
	"testing"

	bpdecode "github.com/galpodlipnik1/factorio-blueprint-decoder"
)

func TestBuildLibrary_ProjectsAllRecordKindsAndPlannerSearchTerms(t *testing.T) {
	blueprintRecord := bpdecode.Record{
		Kind: bpdecode.RecordKindBlueprint,
		Metadata: bpdecode.RecordMetadata{
			Label: "Belt bus",
			Icons: []bpdecode.Icon{
				{Index: 1, Signal: bpdecode.Signal{Type: bpdecode.SignalTypeItem, Name: "transport-belt"}},
			},
		},
		Blueprint: &bpdecode.Blueprint{
			Entities: []bpdecode.Entity{
				{
					Number: 1,
					Name:   "transport-belt",
					Tags: bpdecode.ValueMap{
						"role": bpdecode.StringValue("loader"),
					},
				},
			},
			Tags: bpdecode.ValueMap{
				"category": bpdecode.ObjectValue(bpdecode.ValueMap{
					"name": bpdecode.StringValue("logistics"),
				}),
			},
		},
	}
	bookRecord := bpdecode.Record{
		Kind:     bpdecode.RecordKindBlueprintBook,
		Metadata: bpdecode.RecordMetadata{Label: "Factory"},
		Book: &bpdecode.BlueprintBook{
			Contents: bpdecode.SlotTable{
				Size:  2,
				Slots: []bpdecode.Slot{{Index: 2, Record: blueprintRecord}},
			},
		},
	}
	deconstructionRecord := bpdecode.Record{
		Kind:     bpdecode.RecordKindDeconstructionPlanner,
		Metadata: bpdecode.RecordMetadata{Label: "Remove belts"},
		DeconstructionPlanner: &bpdecode.DeconstructionPlanner{
			EntityFilterMode: "whitelist",
			EntityFilters: []bpdecode.PlannerFilter{
				{Index: 1, Prototype: bpdecode.PrototypeRef{Type: "item", Name: "transport-belt"}},
			},
		},
	}
	upgradeRecord := bpdecode.Record{
		Kind:     bpdecode.RecordKindUpgradePlanner,
		Metadata: bpdecode.RecordMetadata{Label: "Assembler upgrades"},
		UpgradePlanner: &bpdecode.UpgradePlanner{
			Mappers: []bpdecode.UpgradeMapper{
				{
					Index:       1,
					Source:      bpdecode.UpgradeTarget{Prototype: bpdecode.PrototypeRef{Type: "entity", Name: "assembling-machine-1"}},
					Destination: bpdecode.UpgradeTarget{Prototype: bpdecode.PrototypeRef{Type: "entity", Name: "assembling-machine-2", Quality: "legendary"}},
				},
			},
		},
	}
	library := &bpdecode.Library{
		Version: bpdecode.Version{Major: 2, Minor: 0, Patch: 77},
		Root: bpdecode.SlotTable{
			Size: 5,
			Slots: []bpdecode.Slot{
				{Index: 1, Record: bookRecord},
				{Index: 3, Record: deconstructionRecord},
				{Index: 4, Record: upgradeRecord},
				{
					Index: 5,
					Record: bpdecode.Record{
						Kind:      bpdecode.RecordKindBlueprint,
						Metadata:  bpdecode.RecordMetadata{Label: "Preview", Preview: true},
						Blueprint: &bpdecode.Blueprint{},
					},
				},
			},
		},
	}

	payload, err := BuildLibrary(library)

	if err != nil {
		t.Fatalf("BuildLibrary returned error: %v", err)
	}
	if payload.SchemaVersion != IndexSchemaVersion {
		t.Fatalf("expected schema %d, got %d", IndexSchemaVersion, payload.SchemaVersion)
	}
	if len(payload.Entries) != 4 {
		t.Fatalf("expected four live records, got %d", len(payload.Entries))
	}

	blueprint := findTestEntry(payload.Entries, "1.2")
	if blueprint == nil {
		t.Fatal("expected nested blueprint at 1.2")
	}
	if blueprint.ParentPathKey != "1" || blueprint.EntityCount != 1 {
		t.Fatalf("unexpected nested blueprint projection: %#v", blueprint)
	}
	if blueprint.IconSprite != "item/transport-belt" {
		t.Fatalf("unexpected icon sprite %q", blueprint.IconSprite)
	}
	if !strings.Contains(blueprint.SearchText, "logistics") {
		t.Fatalf("expected nested tag in search text, got %q", blueprint.SearchText)
	}
	if !strings.Contains(blueprint.SearchText, "loader") {
		t.Fatalf("expected entity tag in search text, got %q", blueprint.SearchText)
	}
	if !strings.Contains(blueprint.SearchText, "transport-belt") || len(blueprint.EntityNames) != 1 {
		t.Fatalf("expected compact entity prototypes in search text: %#v", blueprint)
	}

	deconstruction := findTestEntry(payload.Entries, "3")
	if deconstruction == nil || !strings.Contains(deconstruction.SearchText, "transport-belt") {
		t.Fatalf("expected deconstruction filter in search text: %#v", deconstruction)
	}
	upgrade := findTestEntry(payload.Entries, "4")
	if upgrade == nil || !strings.Contains(upgrade.SearchText, "assembling-machine-2") || !strings.Contains(upgrade.SearchText, "legendary") {
		t.Fatalf("expected upgrade destination in search text: %#v", upgrade)
	}
}

func TestBuildLibrary_RejectsDuplicateSlotIndexes(t *testing.T) {
	library := &bpdecode.Library{
		Root: bpdecode.SlotTable{
			Size: 1,
			Slots: []bpdecode.Slot{
				{Index: 1, Record: bpdecode.Record{Kind: bpdecode.RecordKindBlueprint, Blueprint: &bpdecode.Blueprint{}}},
				{Index: 1, Record: bpdecode.Record{Kind: bpdecode.RecordKindBlueprint, Blueprint: &bpdecode.Blueprint{}}},
			},
		},
	}

	_, err := BuildLibrary(library)

	if err == nil || !strings.Contains(err.Error(), "duplicate slot index") {
		t.Fatalf("expected duplicate slot error, got %v", err)
	}
}

func TestBuildLibrary_UsesNumericPathOrderingForEqualBreadcrumbs(t *testing.T) {
	record := bpdecode.Record{
		Kind:      bpdecode.RecordKindBlueprint,
		Metadata:  bpdecode.RecordMetadata{Label: "Same"},
		Blueprint: &bpdecode.Blueprint{},
	}
	library := &bpdecode.Library{Root: bpdecode.SlotTable{
		Size: 10,
		Slots: []bpdecode.Slot{
			{Index: 10, Record: record},
			{Index: 2, Record: record},
		},
	}}

	payload, err := BuildLibrary(library)
	if err != nil {
		t.Fatalf("BuildLibrary returned error: %v", err)
	}
	if len(payload.Entries) != 2 || payload.Entries[0].PathKey != "2" || payload.Entries[1].PathKey != "10" {
		t.Fatalf("expected numeric path order 2,10; got %#v", payload.Entries)
	}
}

func TestRenderLuaModule_EmitsSchemaTwoAndPlannerDetails(t *testing.T) {
	payload := IndexPayload{
		Entries: []IndexEntry{
			{
				Path:              []uint32{4, 2},
				PathKey:           "4.2",
				ParentPathKey:     "4",
				RecordType:        "upgrade-planner",
				Name:              "Assembler upgrades",
				Description:       "quality\taware",
				Breadcrumb:        "Factory / Assembler upgrades",
				SearchName:        "assembler upgrades",
				SearchDescription: "quality aware",
				SearchBreadcrumb:  "factory / assembler upgrades",
				SearchText:        "assembler upgrades assembling-machine-2 legendary",
				ChildPathKeys:     []string{},
				Tags: map[string]any{
					"nested": map[string]any{"enabled": true},
				},
				Planner: &PlannerDetails{
					Mappers: []PlannerMapper{
						{
							Index: 1,
							From: &PlannerMapperEndpoint{
								Type:    "entity",
								Name:    "assembling-machine-1",
								Quality: "normal",
							},
							To: &PlannerMapperEndpoint{
								Type:    "entity",
								Name:    "assembling-machine-2",
								Quality: "legendary",
							},
						},
					},
				},
			},
		},
	}

	lua := RenderLuaModule(payload)

	if !strings.Contains(lua, "schema_version = 2") {
		t.Fatalf("expected schema version 2, got:\n%s", lua)
	}
	if !strings.Contains(lua, "description_resolved = true") {
		t.Fatalf("expected prebuilt metadata to be marked resolved, got:\n%s", lua)
	}
	if !strings.Contains(lua, "runtime_semantics_resolved = true") {
		t.Fatalf("expected API-built metadata to be immediately usable, got:\n%s", lua)
	}
	if strings.Contains(lua, "runtime_semantics_resolved = false") {
		t.Fatalf("expected prebuilt index to avoid runtime warmup, got:\n%s", lua)
	}
	if !strings.Contains(lua, `record_type = "upgrade-planner"`) {
		t.Fatalf("expected upgrade planner record, got:\n%s", lua)
	}
	if !strings.Contains(lua, `["mappers"]`) {
		t.Fatalf("expected planner mappers, got:\n%s", lua)
	}
	if !strings.Contains(lua, `assembling-machine-2`) {
		t.Fatalf("expected destination prototype, got:\n%s", lua)
	}
	if !strings.Contains(lua, `entity_names = {}`) {
		t.Fatalf("expected compact entity-name field, got:\n%s", lua)
	}
	if !strings.Contains(lua, `["nested"] = { ["enabled"] = true }`) {
		t.Fatalf("expected nested tag, got:\n%s", lua)
	}
}

func TestRenderLuaModule_IsDeterministicAndEscapesControlCharacters(t *testing.T) {
	payload := IndexPayload{
		Entries: []IndexEntry{
			{
				Path:             []uint32{1},
				PathKey:          "1",
				RecordType:       "deconstruction-planner",
				Name:             "Trees\nRocks\x00",
				Breadcrumb:       "Trees / Rocks",
				SearchName:       "trees rocks",
				SearchBreadcrumb: "trees / rocks",
				SearchText:       "trees rocks",
				ChildPathKeys:    []string{},
				Tags: map[string]any{
					"zeta":  "last",
					"alpha": "first",
				},
			},
		},
	}

	first := RenderLuaModule(payload)
	second := RenderLuaModule(payload)

	if first != second {
		t.Fatal("expected deterministic Lua output")
	}
	if !strings.Contains(first, `name = "Trees\nRocks\000"`) {
		t.Fatalf("expected escaped control characters, got:\n%s", first)
	}
	if strings.Index(first, `["alpha"]`) >= strings.Index(first, `["zeta"]`) {
		t.Fatalf("expected sorted map keys, got:\n%s", first)
	}
}

func FuzzRenderLuaModule_IsDeterministicAndDoesNotPanic(f *testing.F) {
	f.Add("Blueprint", "description", "tag", "value")
	f.Add("quotes \" and slash \\", "line\nfeed\x00", "nested", "planner")

	f.Fuzz(func(t *testing.T, name string, description string, tagKey string, tagValue string) {
		const maxFuzzStringBytes = 1 << 16
		if len(name) > maxFuzzStringBytes || len(description) > maxFuzzStringBytes ||
			len(tagKey) > maxFuzzStringBytes || len(tagValue) > maxFuzzStringBytes {
			t.Skip()
		}

		payload := IndexPayload{Entries: []IndexEntry{{
			Path:          []uint32{1, 2},
			PathKey:       "1.2",
			RecordType:    "blueprint",
			Name:          name,
			Description:   description,
			Breadcrumb:    name,
			SearchText:    name,
			ChildPathKeys: []string{},
			EntityNames:   []string{name},
			Tags: map[string]any{
				tagKey: map[string]any{"value": tagValue},
			},
		}}}

		first := RenderLuaModule(payload)
		second := RenderLuaModule(payload)
		if first != second {
			t.Fatal("renderer output changed between identical calls")
		}
		if !strings.HasPrefix(first, "return {\n") || !strings.HasSuffix(first, "}\n") {
			t.Fatalf("renderer returned an incomplete Lua module: %q", first)
		}
	})
}

func findTestEntry(entries []IndexEntry, pathKey string) *IndexEntry {
	for index := range entries {
		if entries[index].PathKey == pathKey {
			return &entries[index]
		}
	}
	return nil
}
