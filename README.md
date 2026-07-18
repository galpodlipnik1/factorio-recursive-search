# Recursive Blueprint Finder

A [Factorio](https://www.factorio.com/) mod (v2.0+) that adds a recursive search UI for blueprint libraries. It indexes blueprints, blueprint books, deconstruction planners, and upgrade planners across nested books.

## Features

- **Full-text search** across all four record types, including deeply nested ones — searches names, descriptions, breadcrumb paths, tags, planner filters, qualities, and upgrade mappings
- **Blueprint icons** displayed per result, resolved from the blueprint's custom icon data
- **Entity count** shown inline for each blueprint (e.g. `Iron Smelter (128)`)
- **Browse mode** — click any book result to drill into its direct children
- **Auto-focus** — the search field is focused the moment you open the window, no click needed
- **Tag search** — blueprints with blueprint tags are indexed and searchable by tag key or value
- **Lazy rebuilds** — index only rebuilds when needed (on open or manual refresh)
- **Instant prebuilt startup** — API-generated indexes load without live traversal or metadata warmup
- **Fallback runtime indexing** — missing or stale indexes rebuild progressively, with visible results prioritized first
- **Status bar** showing entry count, match count, index state, and last rebuild tick
- Keyboard-friendly: `Enter` pastes the first result, `Esc` closes the window

## Usage

| Action                   | Binding                                                  |
| ------------------------ | -------------------------------------------------------- |
| Toggle the search window | `Ctrl + Shift + F` (configurable in Settings → Controls) |
| Shortcut bar button      | Click **Recursive Blueprint Finder**                     |

- Type at least 2 characters to start searching.
- Results show the blueprint icon, name, entity count, type tag, and full breadcrumb path.
- Click a **blueprint**, **deconstruction planner**, or **upgrade planner** result to import it into your cursor.
- Click a **book** result to browse its contents.
- Use the **Back** button to navigate up the book hierarchy.
- Click **Refresh** (↻) in the title bar to force a full index rebuild.

## Installation

### From the Factorio Mod Portal

Search for **Recursive Blueprint Finder** in the in-game mod browser or on the [Factorio Mod Portal](https://mods.factorio.com/).

### Manual

1. Download or clone this repository.
2. Copy the folder into your Factorio `mods/` directory.
3. Launch Factorio and enable the mod.

## Requirements

- Factorio **2.0** or newer
- Base mod **≥ 2.0.0**

## Project Structure

```
control.lua              ← mod entry point, registers all event handlers
data.lua                 ← hotkey and shortcut definitions
generated/
└── index.lua            ← API-generated prebuilt search index
scripts/
├── events.lua           ← event wiring and orchestration
├── index/
│   ├── indexer.lua      ← incremental rebuild + label/icon warmup pipeline
│   ├── search.lua       ← ranked full-text query and browse-mode filter
│   └── state.lua        ← per-player state stored in storage
├── ui/
│   └── ui.lua           ← GUI construction and refresh
└── lib/
    ├── logger.lua        ← structured key=value log helper
    ├── placement.lua     ← exports a record and pastes it into the player cursor
    ├── resolver.lua      ← resolves a slot-path to a blueprint record
    └── util.lua          ← normalization, search text, sprite paths, misc helpers
server/                  ← local HTTP API that decodes the blueprint library
tooling/
├── create-mod-local.ps1 ← packages directly into %APPDATA%\Factorio\mods
├── prebuild-index.ps1   ← generates generated/index.lua through the local API
└── install-index.ps1    ← injects a prebuilt index into the installed mod zip
```

## How Indexing Works

The normal local workflow generates `generated/index.lua` through the API before
the mod is packaged. A valid prebuilt index is loaded as the authoritative
search snapshot: opening the finder does not traverse the blueprint shelf,
decode exchange strings, or warm metadata in the background. If the binary
decoder could not provide an icon, only visible results resolve their first
`LuaRecord.preview_icons` signal in batches of ten per tick and cache it in mod
state. This targeted lookup does not change the index from `prebuilt` status.

Run the local workflow in this order:

```powershell
# Terminal 1
Set-Location .\server
go run .

# Terminal 2, from the repository root
.\tooling\prebuild-index.ps1 -ApiUrl http://localhost:8080/index
.\tooling\create-mod-local.ps1
```

`create-mod-local.ps1` fails if the prebuilt file is missing and packages the
exact generated module into the installed mod zip. `install-index.ps1` remains
available for injecting an index into a zip that has already been installed.

The runtime indexer is the fallback for a missing, rejected, or stale prebuilt
index. It rebuilds incrementally at 100 records per tick, then resolves labels
and custom icons at one entry per tick while the window is open. A blueprint
library change marks the packaged snapshot stale and switches to this live
fallback so numeric paths continue to identify the correct records. Refresh is
the manual escape hatch: it declares the packaged snapshot stale and rebuilds
from the live shelf.

## Lua index schema v2

Prebuilt indexes use a consumer-owned Lua payload:

```lua
return {
  schema_version = 2,
  entries = {
    -- compact searchable entries
  }
}
```

Schema v2 retains numeric paths, parent/child links, breadcrumbs, normalized
search fields, icons, tags, and entity counts. It adds `record_type` support for
all four record kinds and a compact `planner` table containing deconstruction
filters/modes or upgrade mapper endpoints. Full entities, tiles, wires, and
schedules are deliberately excluded from `index.lua`.

The loader continues to normalize legacy schema-v1 payloads, but newly
generated indexes are schema v2. Runtime and prebuilt indexing share the same
behavioral contract: numeric path ordering, parent/child relationships,
type-specific search terms, and record actions must agree. Books drill down;
blueprints and both planner types import to the cursor. The server builds only
from a strict semantic decode, so partial or recovered records never become
search results. The generated module contains the complete compact search
snapshot and is immediately usable; full entities, tiles, wires, and schedules
remain excluded because the UI neither searches nor renders them directly.

## Verification

The server keeps the local decoder replacement for cross-repository work. From
`server/`, run:

```powershell
go test -mod=readonly ./...
go test -mod=readonly -tags=integration ./...
go test -mod=readonly -race ./...
go vet -mod=readonly ./...
go build -mod=readonly ./...
```

The tagged projection regression uses the sibling decoder fixture by default;
set `FACTORIO_BLUEPRINT_STORAGE_FIXTURE` to an explicit isolated fixture path
when the repositories are not siblings.

## Version

**0.1.3**

## License

This project is licensed under the [MIT License](LICENSE).
