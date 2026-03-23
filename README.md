# Recursive Blueprint Finder

A [Factorio](https://www.factorio.com/) mod (v2.0+) that adds a recursive search UI for your blueprint libraries, letting you instantly find any blueprint or book by name, description, tag, or entity count across all nested books.

## Features

- **Full-text search** across all blueprints and books, including deeply nested ones — searches name, description, breadcrumb path, and tags
- **Blueprint icons** displayed per result, resolved from the blueprint's custom icon data
- **Entity count** shown inline for each blueprint (e.g. `Iron Smelter (128)`)
- **Browse mode** — click any book result to drill into its direct children
- **Auto-focus** — the search field is focused the moment you open the window, no click needed
- **Tag search** — blueprints with blueprint tags are indexed and searchable by tag key or value
- **Lazy rebuilds** — index only rebuilds when needed (on open or manual refresh)
- **Two-phase warmup** — labels and icons are resolved progressively while the window is open, with visible results prioritized first
- **Status bar** showing entry count, match count, index state, and last rebuild tick
- Keyboard-friendly: `Enter` pastes the first result, `Esc` closes the window

## Usage

| Action                   | Binding                                                  |
| ------------------------ | -------------------------------------------------------- |
| Toggle the search window | `Ctrl + Shift + F` (configurable in Settings → Controls) |
| Shortcut bar button      | Click **Recursive Blueprint Finder**                     |

- Type at least 2 characters to start searching.
- Results show the blueprint icon, name, entity count, type tag, and full breadcrumb path.
- Click a **blueprint** result to paste it into your cursor.
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
```

## How Indexing Works

On open (or manual refresh), the mod rebuilds its index in two phases to keep lag minimal:

1. **Rebuild** — traverses the entire blueprint library at 100 records per tick, extracting name, description, breadcrumb path, entity count, and tags.
2. **Warmup** — resolves the actual blueprint label and custom icon for every entry by decoding the blueprint string into a temporary item stack (1 entry per tick). Visible search results are always prioritized first in the warmup queue.

Until warmup reaches an entry, its fallback name (from description or a placeholder) and the default blueprint/book icon are shown. As warmup progresses, rows update in place.

## Version

**0.1.2**

## License

This project is licensed under the [MIT License](LICENSE).
