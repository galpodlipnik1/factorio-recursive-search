-- Mod entry point. Registers all Factorio runtime event handlers and
-- delegates them to scripts/events.lua.

local events = require("scripts.events")

script.on_init(events.on_init)
script.on_configuration_changed(events.on_configuration_changed)

script.on_event(defines.events.on_player_created, events.on_player_created)
script.on_event(defines.events.on_player_joined_game, events.on_player_joined_game)
script.on_event(defines.events.on_player_configured_blueprint, events.on_blueprint_related_change)
script.on_event(defines.events.on_player_setup_blueprint, events.on_blueprint_related_change)

script.on_event(defines.events.on_gui_click, events.on_gui_click)
script.on_event(defines.events.on_gui_text_changed, events.on_gui_text_changed)
script.on_event(defines.events.on_gui_confirmed, events.on_gui_confirmed)
script.on_event(defines.events.on_gui_closed, events.on_gui_closed)
script.on_event(defines.events.on_lua_shortcut, events.on_lua_shortcut)
script.on_event(defines.events.on_tick, events.on_tick)

script.on_event("rbf-toggle", events.on_toggle_hotkey)
