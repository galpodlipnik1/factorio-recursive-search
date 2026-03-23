-- Event handler implementations. Orchestrates the indexer, UI, and placement
-- modules in response to player actions and game lifecycle events.

---@diagnostic disable: undefined-global
local indexer = require("scripts.index.indexer")
local logger = require("scripts.lib.logger")
local placement = require("scripts.lib.placement")
local resolver = require("scripts.lib.resolver")
local search = require("scripts.index.search")
local state = require("scripts.index.state")
local ui = require("scripts.ui.ui")
local util = require("scripts.lib.util")

local M = {}

local function get_player(event)
  if not event.player_index then
    return nil
  end

  return game.get_player(event.player_index)
end

local function load_prebuilt_module()
  local ok, cached = pcall(require, "__recursive-blueprint-finder__/generated/index")
  if not ok or type(cached) ~= "table" then
    return nil
  end

  return cached
end

local function try_load_prebuilt_index(player_index, force)
  local player_state = state.ensure_player_state(player_index)
  if not force and player_state.index.entry_count > 0 and player_state.index.dirty == false then
    return false
  end

  local cached = load_prebuilt_module()
  if not cached then
    return false
  end

  local applied, count = state.apply_prebuilt_index(player_index, cached)
  if applied then
    logger.info("index.prebuilt-loaded", {
      entries = count,
      player_index = player_index
    })
  end

  return applied
end

local function ensure_rebuild_started(player, force_rebuild)
  local player_state = state.ensure_player_state(player.index)

  if force_rebuild then
    logger.player(player, "index.rebuild-requested", {
      reason = "force"
    })
    state.mark_index_dirty(player.index)
  end

  if player_state.index.rebuilding or not player_state.index.dirty then
    return false
  end

  indexer.start_rebuild(player)
  return true
end

local function should_run_initial_rebuild(player_state)
  return player_state.index.last_rebuild_tick == nil
    and player_state.index.entry_count == 0
    and player_state.index.dirty
    and not player_state.index.rebuilding
end

local function handle_entry_action(player, entry)
  if entry.record_type == "blueprint-book" then
    logger.player(player, "ui.open-book", {
      path_key = entry.path_key
    })
    local player_state = state.get_player_state(player.index)
    player_state.ui.mode = "browse"
    player_state.ui.browse_path_key = entry.path_key
    ui.refresh(player)
    return
  end

  local record = resolver.resolve_record_by_path(player, entry.path)
  if not record then
    logger.player(player, "action.resolve-failed", {
      path_key = entry.path_key
    })
    player.print({"rbf.select-failed"})
    state.mark_index_dirty(player.index)
    ui.refresh(player)
    return
  end

  local ok = placement.place_record(player, record)
  if not ok then
    logger.player(player, "action.place-failed", {
      path_key = entry.path_key
    })
    player.print({"rbf.select-failed"})
    state.mark_index_dirty(player.index)
    ui.refresh(player)
    return
  end

  logger.player(player, "action.place-succeeded", {
    path_key = entry.path_key
  })
  ui.close(player)
end

function M.on_init()
  state.ensure_globals()
  logger.info("lifecycle.on-init", {
    players = #game.players
  })

  for _, player in pairs(game.players) do
    state.ensure_player_state(player.index)
    if not try_load_prebuilt_index(player.index, true) then
      state.mark_index_dirty(player.index)
    end
    ui.sync_shortcut(player)
  end
end

function M.on_configuration_changed()
  state.ensure_globals()
  logger.info("lifecycle.on-configuration-changed", {
    players = #game.players
  })

  for _, player in pairs(game.players) do
    state.ensure_player_state(player.index)
    if not try_load_prebuilt_index(player.index, true) then
      state.mark_index_dirty(player.index)
    end
    ui.sync_shortcut(player)
  end
end

function M.on_player_created(event)
  local player = get_player(event)
  if not player then
    return
  end

  logger.player(player, "player.created")
  state.ensure_player_state(player.index)
  if not try_load_prebuilt_index(player.index, true) then
    state.mark_index_dirty(player.index)
  end
  ui.sync_shortcut(player)
end

function M.on_player_joined_game(event)
  local player = get_player(event)
  if not player then
    return
  end

  logger.player(player, "player.joined")
  state.ensure_player_state(player.index)
  try_load_prebuilt_index(player.index, false)
  ui.sync_shortcut(player)
end

function M.on_blueprint_related_change(event)
  local player = get_player(event)
  if not player then
    return
  end

  logger.player(player, "blueprints.changed")
  state.mark_index_dirty(player.index)

  local player_state = state.get_player_state(player.index)
  if player_state.ui.open then
    ui.refresh(player)
  end
end

function M.on_toggle_hotkey(event)
  local player = get_player(event)
  if not player then
    return
  end

  local player_state = state.get_player_state(player.index)
  if player_state.ui.open then
    logger.player(player, "ui.toggle-close", {
      source = "hotkey"
    })
    ui.close(player)
    return
  end

  if player_state.index.dirty and player_state.index.entry_count == 0 then
    try_load_prebuilt_index(player.index, false)
    player_state = state.get_player_state(player.index)
  end

  if should_run_initial_rebuild(player_state) then
    ensure_rebuild_started(player, false)
  elseif player_state.index.dirty then
    ensure_rebuild_started(player, false)
  end

  logger.player(player, "ui.toggle-open", {
    source = "hotkey",
    rebuilding = player_state.index.rebuilding,
    entry_count = player_state.index.entry_count
  })
  ui.open(player)
end

function M.on_lua_shortcut(event)
  if event.prototype_name ~= ui.shortcut_name then
    return
  end

  local player = get_player(event)
  if player then
    logger.player(player, "ui.shortcut-clicked", {
      prototype_name = event.prototype_name
    })
  end
  M.on_toggle_hotkey(event)
end

function M.on_tick()
  if not storage.players then
    return
  end

  for player_index, player_state in pairs(storage.players) do
    local player = game.get_player(player_index)
    if player and player.valid then
      if player_state.index.rebuilding then
        local was_rebuilding = true
        indexer.process_rebuild_batch(player)

        if player_state.ui.open and was_rebuilding and not player_state.index.rebuilding then
          ui.refresh(player)
        end
      elseif player_state.ui.open then
        local query = util.normalize(player_state.ui.query)
        local allow_background = player_state.ui.mode == "browse" or query ~= ""
        if indexer.process_label_batch(player, nil, allow_background) then
          ui.refresh(player)
        end
      end
    end
  end
end

function M.on_gui_click(event)
  local player = get_player(event)
  if not player or not event.element or not event.element.valid then
    return
  end

  local element = event.element

  if element.name == ui.names.close then
    logger.player(player, "ui.close-clicked")
    ui.close(player)
    return
  end

  if element.name == ui.names.refresh then
    logger.player(player, "ui.refresh-clicked")
    local started = ensure_rebuild_started(player, true)
    if started or state.get_player_state(player.index).index.rebuilding then
      ui.refresh(player)
    end
    return
  end

  if element.name == ui.names.layout_toggle then
    local player_state = state.get_player_state(player.index)
    player_state.ui.layout = player_state.ui.layout == "detailed" and "compact" or "detailed"
    logger.player(player, "ui.layout-toggled", { layout = player_state.ui.layout })
    ui.refresh(player)
    return
  end

  if element.name == ui.names.back then
    logger.player(player, "ui.back-clicked")
    local player_state = state.get_player_state(player.index)
    if not player_state.ui.browse_path_key then
      player_state.ui.mode = "search"
      ui.refresh(player)
      return
    end

    local current = util.find_entry(player_state.index.entries, player_state.ui.browse_path_key)
    if current and current.parent_path_key then
      player_state.ui.mode = "browse"
      player_state.ui.browse_path_key = current.parent_path_key
    else
      player_state.ui.mode = "search"
      player_state.ui.browse_path_key = nil
    end

    ui.refresh(player)
    return
  end

  local action = element.tags and element.tags.action
  local path_key = element.tags and element.tags.path_key

  if not action or not path_key then
    return
  end

  logger.player(player, "ui.entry-clicked", {
    action = action,
    path_key = path_key
  })
  local entry = util.find_entry(state.get_player_state(player.index).index.entries, path_key)
  if not entry then
    logger.player(player, "ui.entry-missing", {
      path_key = path_key
    })
    player.print({"rbf.select-failed"})
    state.mark_index_dirty(player.index)
    ui.refresh(player)
    return
  end

  handle_entry_action(player, entry)
end

function M.on_gui_text_changed(event)
  local player = get_player(event)
  if not player or not event.element or not event.element.valid then
    return
  end

  if event.element.name ~= ui.names.query then
    return
  end

  local player_state = state.get_player_state(player.index)
  player_state.ui.query = event.element.text
  player_state.ui.mode = "search"
  player_state.ui.browse_path_key = nil

  ui.refresh(player)
end

function M.on_gui_confirmed(event)
  local player = get_player(event)
  if not player or not event.element or not event.element.valid then
    return
  end

  if event.element.name ~= ui.names.query then
    return
  end

  local player_state = state.get_player_state(player.index)
  if #util.normalize(player_state.ui.query) < 2 then
    return
  end

  local model = search.query(player_state.index.entries, player_state.ui.query, 1)
  logger.player(player, "ui.query-confirmed", {
    query = player_state.ui.query,
    matches = model.total_matches
  })

  if model.total_matches == 0 then
    return
  end

  handle_entry_action(player, model.matches[1])
end

function M.on_gui_closed(event)
  local player = get_player(event)
  if not player then
    return
  end

  if event.element and event.element.valid and event.element.name == ui.names.frame then
    logger.player(player, "ui.closed")
    ui.close(player)
  end
end

return M
