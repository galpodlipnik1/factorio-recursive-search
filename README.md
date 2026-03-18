# Recursive Blueprint Finder

A [Factorio](https://www.factorio.com/) mod (v2.0+) that adds a recursive search UI for your blueprint libraries, letting you instantly find any blueprint or book by name across all nested books.

## Features

- Full-text search across all blueprints and blueprint books, including deeply nested ones
- Browse into any book to see its direct children
- One-click open or paste of any result
- Lazy rebuilds on open or manual refresh
- Title warmup while the search window is open, with visible results prioritized first
- Status bar showing entry count, match count, index state, and last rebuild time
- Keyboard-friendly: `Enter` selects the first result, `Esc` closes the window

## Installation

### From the Factorio Mod Portal

Search for **Recursive Blueprint Finder** in the in-game mod browser or on the [Factorio Mod Portal](https://mods.factorio.com/).

### Manual

1. Download or clone this repository.
2. Copy the folder into your Factorio `mods/` directory.
3. Launch Factorio and enable the mod.

## Usage

| Action                   | Default binding                                    |
| ------------------------ | -------------------------------------------------- |
| Toggle the search window | `rbf-toggle` (configurable in Settings → Controls) |
| Shortcut bar button      | Click **Recursive Blueprint Finder**               |

- Type part of a blueprint or book name to filter results.
- Click a result row to select it, then use **Open** or **Paste**.
- Click **Refresh** to rebuild the index after importing new blueprints.

## Requirements

- Factorio **2.0** or newer
- Base mod **≥ 2.0.0**

## Version

**0.1.0** — initial release

## License

This project is licensed under the [MIT License](LICENSE).
