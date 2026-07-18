-- Player state management. Owns the index and UI state tables stored in
-- `storage`, and handles migration from older save formats.

local M = {}
local CURRENT_SCHEMA_VERSION = 2
local CURRENT_PREVIEW_ICON_RESOLVER_VERSION = 1
local SUPPORTED_RECORD_TYPES = {
  ["blueprint"] = true,
  ["blueprint-book"] = true,
  ["deconstruction-planner"] = true,
  ["upgrade-planner"] = true
}

local function path_key(path)
  return table.concat(path or {}, ".")
end

local function copy_array(values)
  local out = {}
  for index = 1, #(values or {}) do
    out[index] = values[index]
  end
  return out
end

local function validate_schema_two_entries(entries)
  local known_paths = {}

  for index = 1, #entries do
    local entry = entries[index]
    if type(entry) ~= "table"
      or type(entry.path) ~= "table"
      or #entry.path == 0
      or SUPPORTED_RECORD_TYPES[entry.record_type] ~= true
      or entry.label_resolved ~= true
      or entry.description_resolved ~= true
      or entry.runtime_semantics_resolved ~= true then
      return false
    end

    for path_index = 1, #entry.path do
      local slot = entry.path[path_index]
      if type(slot) ~= "number" or slot < 1 or slot ~= math.floor(slot) then
        return false
      end
    end

    local derived_path_key = path_key(entry.path)
    if entry.path_key ~= derived_path_key or known_paths[derived_path_key] then
      return false
    end
    known_paths[derived_path_key] = true

    local expected_parent = nil
    if #entry.path > 1 then
      local parent_path = copy_array(entry.path)
      parent_path[#parent_path] = nil
      expected_parent = path_key(parent_path)
    end
    if entry.parent_path_key ~= expected_parent then
      return false
    end
  end

  for index = 1, #entries do
    local parent_path_key = entries[index].parent_path_key
    if parent_path_key and not known_paths[parent_path_key] then
      return false
    end
  end

  return true
end

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
      priority_label_set = nil,
      preview_icon_queue = {},
      preview_icon_set = {},
      preview_icon_resolver_version = CURRENT_PREVIEW_ICON_RESOLVER_VERSION,
      source = "runtime",
      prefer_runtime = false,
      schema_version = CURRENT_SCHEMA_VERSION
    },
    ui = {
      open = false,
      query = "",
      mode = "search",
      browse_path_key = nil,
      layout = "compact"
    }
  }
end

local function migrate_from_fast_index(player_state)
  if player_state.index ~= nil then
    return
  end

  local fast_index = player_state.fast_index or {}
  local ui_state = player_state.ui_state or {}

  player_state.index = {
    entries = fast_index.entries or {},
    entry_map = fast_index.entry_map or {},
    entry_count = fast_index.entry_count or #(fast_index.entries or {}),
    last_rebuild_tick = fast_index.last_built_tick,
    dirty = fast_index.state == "stale" or fast_index.state == "empty" or fast_index.state == nil,
    rebuilding = fast_index.state == "building",
    resolving_labels = false,
    labels_remaining = 0,
    rebuild_revision = fast_index.rebuild_revision or 0,
    job_revision = fast_index.job_revision,
    pending_entries = fast_index.pending_entries,
    pending_entry_map = fast_index.pending_entry_map,
    pending_tasks = fast_index.pending_tasks,
    job_cursor = fast_index.job_cursor or 1,
    label_queue = nil,
    label_cursor = 1,
    priority_label_queue = nil,
    priority_label_set = nil,
    preview_icon_queue = {},
    preview_icon_set = {},
    preview_icon_resolver_version = CURRENT_PREVIEW_ICON_RESOLVER_VERSION,
    source = "runtime",
    prefer_runtime = false,
    schema_version = 1
  }

  player_state.ui = {
    open = ui_state.open == true,
    query = ui_state.query or "",
    mode = ui_state.mode or "search",
    browse_path_key = ui_state.browse_path_key,
    layout = ui_state.layout or "compact"
  }

  player_state.fast_index = nil
  player_state.ui_state = nil
  player_state.deep_index = nil
  player_state.dirty_flags = nil
end

local function normalize_entry(entry)
  if not entry then
    return nil
  end

  entry.path = copy_array(entry.path or {})
  entry.path_key = entry.path_key or path_key(entry.path)
  entry.parent_path_key = entry.parent_path_key
  entry.record_type = entry.record_type or entry.type or "blueprint"
  entry.name = entry.name or entry.fallback_name or ""
  entry.description = entry.description or ""
  entry.breadcrumb = entry.breadcrumb or entry.name or ""
  entry.search_name = entry.search_name or ""
  entry.search_description = entry.search_description or ""
  entry.search_breadcrumb = entry.search_breadcrumb or ""
  entry.search_text = entry.search_text or ""
  entry.label_resolved = entry.label_resolved ~= false
  entry.description_resolved = entry.description_resolved ~= false
  entry.runtime_semantics_resolved = entry.runtime_semantics_resolved ~= false
  entry.child_path_keys = entry.child_path_keys or {}
  if entry.icon_sprite == true then
    entry.icon_sprite = nil
  end
  if entry.preview_icon_resolved ~= true then
    entry.preview_icon_resolved = type(entry.icon_sprite) == "string" and entry.icon_sprite ~= ""
  end
  entry.entity_count = entry.entity_count or 0
  entry.entity_names = entry.entity_names or {}
  entry.tags = entry.tags or {}
  entry.planner = type(entry.planner) == "table" and entry.planner or nil

  return entry
end

local function rebuild_entry_map(index_state)
  index_state.entry_map = {}

  for index = 1, #index_state.entries do
    local entry = index_state.entries[index]
    if entry and entry.path_key then
      index_state.entry_map[entry.path_key] = entry
    end
  end

  for index = 1, #index_state.entries do
    local entry = index_state.entries[index]
    entry.child_path_keys = {}
  end

  for index = 1, #index_state.entries do
    local entry = index_state.entries[index]
    if entry.parent_path_key then
      local parent = index_state.entry_map[entry.parent_path_key]
      if parent then
        parent.child_path_keys[#parent.child_path_keys + 1] = entry.path_key
      end
    end
  end
end

local function ensure_index_shape(index_state)
  local preview_icon_cache_stale = index_state.preview_icon_resolver_version
    ~= CURRENT_PREVIEW_ICON_RESOLVER_VERSION

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
  index_state.preview_icon_queue = index_state.preview_icon_queue or {}
  index_state.preview_icon_set = index_state.preview_icon_set or {}
  index_state.source = index_state.source or "runtime"
  index_state.prefer_runtime = index_state.prefer_runtime == true
  index_state.schema_version = index_state.schema_version or 1

  for index = 1, #index_state.entries do
    index_state.entries[index] = normalize_entry(index_state.entries[index])
  end

  if preview_icon_cache_stale then
    -- A previous resolver rejected item signals because Factorio omits their
    -- SignalID type. Retry only failed lookups; concrete prebuilt sprites stay
    -- authoritative and do not require live work.
    for index = 1, #index_state.entries do
      local entry = index_state.entries[index]
      if entry.icon_sprite == false then
        entry.preview_icon_resolved = false
      end
    end
    index_state.preview_icon_queue = {}
    index_state.preview_icon_set = {}
  end
  index_state.preview_icon_resolver_version = CURRENT_PREVIEW_ICON_RESOLVER_VERSION

  rebuild_entry_map(index_state)
  index_state.entry_count = #index_state.entries
end

local function ensure_ui_shape(ui_state)
  ui_state.open = ui_state.open == true
  ui_state.query = ui_state.query or ""
  ui_state.mode = ui_state.mode or "search"
  ui_state.browse_path_key = ui_state.browse_path_key
  ui_state.layout = ui_state.layout or "compact"
end

local function normalize_player_state(player_state)
  migrate_from_fast_index(player_state)
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

function M.has_usable_index(player_index)
  local player_state = M.ensure_player_state(player_index)
  return player_state.index.entry_count > 0 and player_state.index.dirty == false
end

function M.apply_prebuilt_index(player_index, payload)
  if type(payload) ~= "table" then
    return false, 0
  end

  local schema_version = tonumber(payload.schema_version)
  local entries = payload.entries
  if type(entries) ~= "table" then
    entries = payload
    schema_version = schema_version or 1
  end

  schema_version = schema_version or 1
  if schema_version < 1 or schema_version > CURRENT_SCHEMA_VERSION then
    return false, 0
  end

  if type(entries) ~= "table" then
    return false, 0
  end

  if schema_version == CURRENT_SCHEMA_VERSION and not validate_schema_two_entries(entries) then
    return false, 0
  end

  local player_state = M.ensure_player_state(player_index)
  local index_state = player_state.index
  local normalized_entries = {}

  for index = 1, #entries do
    local entry = normalize_entry(entries[index])
    if entry and entry.path_key then
      -- A successfully generated prebuilt payload is authoritative. Live
      -- traversal and metadata resolution are fallback behavior only.
      entry.runtime_semantics_resolved = true
      -- Missing binary icons are resolved lazily only when this entry is
      -- visible. Concrete API-projected sprites never need a live lookup.
      entry.preview_icon_resolved = type(entry.icon_sprite) == "string" and entry.icon_sprite ~= ""
      normalized_entries[#normalized_entries + 1] = entry
    end
  end

  if #normalized_entries == 0 and schema_version ~= CURRENT_SCHEMA_VERSION then
    return false, 0
  end

  index_state.entries = normalized_entries
  index_state.entry_map = {}
  index_state.entry_count = #normalized_entries
  index_state.last_rebuild_tick = nil
  index_state.dirty = false
  index_state.rebuilding = false
  index_state.resolving_labels = false
  index_state.labels_remaining = 0
  index_state.job_revision = nil
  index_state.pending_entries = nil
  index_state.pending_entry_map = nil
  index_state.pending_tasks = nil
  index_state.job_cursor = 1
  index_state.label_queue = nil
  index_state.label_cursor = 1
  index_state.priority_label_queue = nil
  index_state.priority_label_set = nil
  index_state.preview_icon_queue = {}
  index_state.preview_icon_set = {}
  index_state.preview_icon_resolver_version = CURRENT_PREVIEW_ICON_RESOLVER_VERSION
  index_state.source = "prebuilt"
  index_state.prefer_runtime = false
  index_state.schema_version = schema_version

  rebuild_entry_map(index_state)
  return true, index_state.entry_count
end

function M.mark_index_dirty(player_index)
  local player_state = M.ensure_player_state(player_index)
  player_state.index.rebuild_revision = player_state.index.rebuild_revision + 1
  player_state.index.dirty = true
  player_state.index.rebuilding = false
  player_state.index.resolving_labels = false
  player_state.index.labels_remaining = 0
  player_state.index.pending_entries = nil
  player_state.index.pending_entry_map = nil
  player_state.index.pending_tasks = nil
  player_state.index.job_cursor = 1
  player_state.index.job_revision = nil
  player_state.index.label_queue = nil
  player_state.index.label_cursor = 1
  player_state.index.priority_label_queue = nil
  player_state.index.priority_label_set = nil
  player_state.index.preview_icon_queue = {}
  player_state.index.preview_icon_set = {}
  player_state.index.preview_icon_resolver_version = CURRENT_PREVIEW_ICON_RESOLVER_VERSION
  player_state.index.source = "runtime"
  player_state.index.schema_version = CURRENT_SCHEMA_VERSION
end

function M.request_runtime_index(player_index)
  M.mark_index_dirty(player_index)
  local player_state = M.ensure_player_state(player_index)
  player_state.index.prefer_runtime = true
end

function M.reset_ui(player_index)
  local player_state = M.ensure_player_state(player_index)
  player_state.ui.open = false
  player_state.ui.mode = "search"
  player_state.ui.browse_path_key = nil
end

return M
