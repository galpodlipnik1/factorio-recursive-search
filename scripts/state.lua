local M = {}

local function new_player_state()
  return {
    index = {
      entries = {},
      entry_map = {},
      entry_count = 0,
      last_rebuild_tick = nil,
      dirty = true,
      rebuilding = false,
      resolving_labels = false,
      labels_remaining = 0,
      rebuild_revision = 0,
      job_revision = nil,
      pending_entries = nil,
      pending_entry_map = nil,
      pending_tasks = nil,
      job_cursor = 1,
      label_queue = nil,
      label_cursor = 1,
      priority_label_queue = nil,
      priority_label_set = nil
    },
    ui = {
      open = false,
      query = "",
      mode = "search",
      browse_path_key = nil
    }
  }
end

local function ensure_index_shape(index_state)
  index_state.entries = index_state.entries or {}
  index_state.entry_map = index_state.entry_map or {}
  index_state.entry_count = index_state.entry_count or #index_state.entries
  index_state.last_rebuild_tick = index_state.last_rebuild_tick
  index_state.dirty = index_state.dirty ~= false
  index_state.rebuilding = index_state.rebuilding == true
  index_state.resolving_labels = index_state.resolving_labels == true
  index_state.labels_remaining = index_state.labels_remaining or 0
  index_state.rebuild_revision = index_state.rebuild_revision or 0
  index_state.job_revision = index_state.job_revision
  index_state.pending_entries = index_state.pending_entries
  index_state.pending_entry_map = index_state.pending_entry_map
  index_state.pending_tasks = index_state.pending_tasks
  index_state.job_cursor = index_state.job_cursor or 1
  index_state.label_queue = index_state.label_queue
  index_state.label_cursor = index_state.label_cursor or 1
  index_state.priority_label_queue = index_state.priority_label_queue
  index_state.priority_label_set = index_state.priority_label_set

  if next(index_state.entry_map) == nil and #index_state.entries > 0 then
    for index = 1, #index_state.entries do
      local entry = index_state.entries[index]
      if entry and entry.path_key then
        entry.child_path_keys = entry.child_path_keys or {}
        entry.label_resolved = entry.label_resolved == true
        index_state.entry_map[entry.path_key] = entry
      end
    end
  end
end

local function ensure_ui_shape(ui_state)
  ui_state.open = ui_state.open == true
  ui_state.query = ui_state.query or ""
  ui_state.mode = ui_state.mode or "search"
  ui_state.browse_path_key = ui_state.browse_path_key
end

local function normalize_player_state(player_state)
  player_state.index = player_state.index or {}
  player_state.ui = player_state.ui or {}

  ensure_index_shape(player_state.index)
  ensure_ui_shape(player_state.ui)

  return player_state
end

function M.ensure_globals()
  storage.players = storage.players or {}
end

function M.ensure_player_state(player_index)
  M.ensure_globals()

  if not storage.players[player_index] then
    storage.players[player_index] = new_player_state()
  end

  storage.players[player_index] = normalize_player_state(storage.players[player_index])
  return storage.players[player_index]
end

function M.get_player_state(player_index)
  return M.ensure_player_state(player_index)
end

function M.mark_index_dirty(player_index)
  local player_state = M.ensure_player_state(player_index)
  player_state.index.rebuild_revision = player_state.index.rebuild_revision + 1
  player_state.index.dirty = true
  player_state.index.resolving_labels = false
  player_state.index.labels_remaining = 0
  player_state.index.label_queue = nil
  player_state.index.label_cursor = 1
  player_state.index.priority_label_queue = nil
  player_state.index.priority_label_set = nil
end

function M.reset_ui(player_index)
  local player_state = M.ensure_player_state(player_index)
  player_state.ui.open = false
  player_state.ui.mode = "search"
  player_state.ui.browse_path_key = nil
end

return M
