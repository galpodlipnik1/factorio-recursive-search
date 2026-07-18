//go:build integration

package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const integrationFixtureSHA256 = "740ff917b8fd648d1d3881dc6c3a2081a45bea898036210f1aa9c793e44b2b47"

func TestBuildFixtureProjectsCompleteOracleCensus(t *testing.T) {
	fixturePath := os.Getenv("FACTORIO_BLUEPRINT_STORAGE_FIXTURE")
	if fixturePath == "" {
		fixturePath = filepath.Join("..", "..", "..", "factorio-blueprint-decoder", "testdata", "blueprint-storage-2.dat")
	}
	data, err := os.ReadFile(fixturePath)
	if os.IsNotExist(err) && os.Getenv("FACTORIO_BLUEPRINT_STORAGE_FIXTURE") == "" {
		t.Skip("sibling decoder fixture is not available")
	}
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	digest := sha256.Sum256(data)
	if actual := hex.EncodeToString(digest[:]); actual != integrationFixtureSHA256 {
		t.Fatalf("fixture SHA-256 = %s, want %s", actual, integrationFixtureSHA256)
	}

	payload, err := Build(data)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if payload.SchemaVersion != IndexSchemaVersion {
		t.Fatalf("schema version = %d, want %d", payload.SchemaVersion, IndexSchemaVersion)
	}
	if len(payload.Entries) != 3_538 {
		t.Fatalf("entry count = %d, want 3538", len(payload.Entries))
	}

	counts := map[string]int{}
	entryByPath := make(map[string]*IndexEntry, len(payload.Entries))
	entityCount := 0
	deconstructionEntityFilters := 0
	deconstructionTileFilters := 0
	upgradeMappers := 0
	for index := range payload.Entries {
		entry := &payload.Entries[index]
		counts[entry.RecordType]++
		entityCount += entry.EntityCount
		if entry.PathKey == "" || entry.PathKey != pathKey(entry.Path) {
			t.Fatalf("entry %d has inconsistent path: %#v", index, entry)
		}
		if _, duplicate := entryByPath[entry.PathKey]; duplicate {
			t.Fatalf("duplicate projected path %s", entry.PathKey)
		}
		entryByPath[entry.PathKey] = entry
		if entry.Planner != nil {
			deconstructionEntityFilters += len(entry.Planner.EntityFilters)
			deconstructionTileFilters += len(entry.Planner.TileFilters)
			upgradeMappers += len(entry.Planner.Mappers)
			assertPlannerTermsSearchable(t, entry)
		}
		if index > 0 {
			previous := payload.Entries[index-1]
			if previous.Breadcrumb > entry.Breadcrumb ||
				(previous.Breadcrumb == entry.Breadcrumb && compareNumericPath(previous.Path, entry.Path) >= 0) {
				t.Fatalf("entries are not in deterministic breadcrumb/path order at %s", entry.PathKey)
			}
		}
	}

	wantCounts := map[string]int{
		"blueprint":              3_123,
		"blueprint-book":         300,
		"deconstruction-planner": 28,
		"upgrade-planner":        87,
	}
	for recordType, want := range wantCounts {
		if counts[recordType] != want {
			t.Fatalf("%s count = %d, want %d", recordType, counts[recordType], want)
		}
	}
	if entityCount != 1_363_402 {
		t.Fatalf("aggregate entity count = %d, want 1363402", entityCount)
	}
	if deconstructionEntityFilters != 125 || deconstructionTileFilters != 6 || upgradeMappers != 578 {
		t.Fatalf(
			"planner details = entity filters %d, tile filters %d, mappers %d; want 125, 6, 578",
			deconstructionEntityFilters,
			deconstructionTileFilters,
			upgradeMappers,
		)
	}

	for _, entry := range payload.Entries {
		if entry.ParentPathKey == "" {
			continue
		}
		parent := entryByPath[entry.ParentPathKey]
		if parent == nil {
			t.Fatalf("entry %s has missing parent %s", entry.PathKey, entry.ParentPathKey)
		}
		if !containsString(parent.ChildPathKeys, entry.PathKey) {
			t.Fatalf("parent %s does not link child %s", parent.PathKey, entry.PathKey)
		}
	}

	firstRender := RenderLuaModule(payload)
	secondRender := RenderLuaModule(payload)
	if firstRender != secondRender {
		t.Fatal("fixture Lua rendering is not deterministic")
	}
	if !strings.HasPrefix(firstRender, "return {\n  schema_version = 2,\n") {
		t.Fatal("fixture Lua rendering does not use schema version 2")
	}
}

func assertPlannerTermsSearchable(t *testing.T, entry *IndexEntry) {
	t.Helper()
	for _, filter := range append(append([]PlannerFilter(nil), entry.Planner.EntityFilters...), entry.Planner.TileFilters...) {
		for _, term := range []string{filter.Name, filter.Quality, filter.Comparator} {
			if term != "" && !strings.Contains(entry.SearchText, normalize(term)) {
				t.Fatalf("entry %s search text %q omits planner term %q", entry.PathKey, entry.SearchText, term)
			}
		}
	}
	for _, mapper := range entry.Planner.Mappers {
		for _, endpoint := range []*PlannerMapperEndpoint{mapper.From, mapper.To} {
			if endpoint == nil {
				continue
			}
			for _, term := range []string{endpoint.Name, endpoint.Quality, endpoint.Comparator} {
				if term != "" && !strings.Contains(entry.SearchText, normalize(term)) {
					t.Fatalf("entry %s search text %q omits mapper term %q", entry.PathKey, entry.SearchText, term)
				}
			}
		}
	}
}

func pathKey(path []uint32) string {
	parts := make([]string, len(path))
	for index, value := range path {
		parts[index] = fmt.Sprint(value)
	}
	return strings.Join(parts, ".")
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
