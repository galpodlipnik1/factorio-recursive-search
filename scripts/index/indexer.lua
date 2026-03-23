-- Blueprint index pipeline. Handles incremental rebuild (100 records/tick) and
-- a two-phase warmup that resolves labels and custom icons via decoded temp stacks.

---@diagnostic disable: undefined-global
local logger = require("scripts.lib.logger")
local resolver = require("scripts.lib.resolver")
local state = require("scripts.index.state")
local util = require("scripts.lib.util")

local M = {}
local BUILD_BATCH_SIZE = 100
local LABEL_BATCH_SIZE = 1
local PRIORITY_LIMIT = 30
local FALLBACK_NAME_MAX = 48

local function is_readable_record(record)
  return record
    and record.valid
    and not record.is_preview
    and (record.type == "blueprint" or record.type == "blueprint-book")
end

local function safe_destroy_inventory(inventory)
  if inventory and inventory.valid then
    inventory.destroy()
  end
end

local function build_search_text(name, description, breadcrumb, tags)
  return util.build_search_text(name, description, breadcrumb, tags) ---@diagnostic disable-line: redundant-parameter
end

local function read_direct_label(record)
  local ok, label = pcall(function()
    return record.label
  end)

  if not ok then
    return ""
  end

  return util.trim(label)
end

local function read_icon_sprite_from_stack(stack)
  -- LuaItemStack uses get_blueprint_icons(), not .icons
  local ok, icons = pcall(function() return stack.get_blueprint_icons() end)
  if not ok or not icons or not icons[1] or not icons[1].signal then return nil end
  return util.signal_to_sprite_path(icons[1].signal)
end

local function read_tags(record)
  if record.type ~= "blueprint" then return {} end
  local ok, tags = pcall(function() return record.get_blueprint_tags() end)
  return (ok and type(tags) == "table") and tags or {}
end

local function read_entity_count(record)
  if record.type ~= "blueprint" then return 0 end
  local ok, entities = pcall(function() return record.get_blueprint_entities() end)
  return (ok and type(entities) == "table") and #entities or 0
end

local function description_name_fallback(record_type, description)
  local compact = util.trim(description):gsub("[%r%n]+", " ")
  if compact == "" then
    return util.fallback_name_text(record_type)
  end

  if #compact > FALLBACK_NAME_MAX then
    compact = compact:sub(1, FALLBACK_NAME_MAX - 3) .. "..."
  end

  return compact
end

local function create_temp_stack_from_record(record)
  if not is_readable_record(record) then
    logger.info("index.metadata-record-skipped", {
      is_preview = record and record.valid and record.is_preview or nil,
      record_type = record and record.valid and record.type or nil
    })
    return nil, nil
  end

  local export_string = record.export_record()
  if not export_string or export_string == "" then
    logger.info("index.metadata-export-missing", {
      record_type = record and record.type or nil
    })
    return nil, nil
  end

  local inventory = game.create_inventory(1)
  local stack = inventory[1]
  if not (stack and stack.valid) then
    safe_destroy_inventory(inventory)
    logger.info("index.metadata-stack-missing", {
      record_type = record and record.type or nil
    })
    return nil, nil
  end

  local ok = pcall(function()
    stack.import_stack(export_string)
  end)

  if not ok or not stack.valid_for_read then
    safe_destroy_inventory(inventory)
    logger.info("index.metadata-import-failed", {
      record_type = record and record.type or nil
    })
    return nil, nil
  end

  return inventory, stack
end

local function resolve_label_metadata(record, fallback_name, fallback_description)
  local description = util.trim(record and record.blueprint_description)
  if description == "" then
    description = fallback_description or ""
  end

  -- Always create temp stack: it's the only way to read custom blueprint icons.
  -- Direct label check is done after, so we can fall back without a stack if export fails.
  local inventory, stack = create_temp_stack_from_record(record)
  if not inventory or not stack then
    local direct_name = read_direct_label(record)
    return (direct_name ~= "" and direct_name or fallback_name), description, nil
  end

  local ok, label = pcall(function()
    return stack.label
  end)
  local name = ok and util.trim(label) or ""
  if name == "" then
    name = read_direct_label(record)
  end
  if name == "" then
    name = fallback_name
  end

  local icon_sprite = read_icon_sprite_from_stack(stack)
  safe_destroy_inventory(inventory)
  return name, description, icon_sprite
end

local function compute_breadcrumb(index_state, entry)
  local segments = {}
  local current = entry

  while current do
    segments[#segments + 1] = current.name
    if not current.parent_path_key then
      break
    end

    current = index_state.entry_map[current.parent_path_key]
  end

  local breadcrumb = {}
  for index = #segments, 1, -1 do
    breadcrumb[#breadcrumb + 1] = segments[index]
  end

  return table.concat(breadcrumb, " / ")
end

local function refresh_entry_search(entry, index_state)
  entry.breadcrumb = compute_breadcrumb(index_state, entry)
  entry.search_name = util.normalize(entry.name)
  entry.search_description = util.normalize(entry.description)
  entry.search_breadcrumb = util.normalize(entry.breadcrumb)
  entry.search_text = build_search_text(entry.name, entry.description, entry.breadcrumb, entry.tags)
end

local function clear_rebuild_job(index_state)
  index_state.pending_entries = nil
  index_state.pending_entry_map = nil
  index_state.pending_tasks = nil
  index_state.job_cursor = 1
  index_state.job_revision = nil
  index_state.rebuilding = false
end

local function clear_label_job(index_state)
  index_state.label_queue = nil
  index_state.label_cursor = 1
  index_state.priority_label_queue = nil
  index_state.priority_label_set = nil
  index_state.resolving_labels = false
  index_state.labels_remaining = 0
end

local function queue_root_tasks(player)
  local tasks = {}
  local blueprints = player.blueprints

  if not blueprints then
    return tasks
  end

  for slot, record in pairs(blueprints) do
    if is_readable_record(record) then
      tasks[#tasks + 1] = {
        path = { slot },
        breadcrumbs = {},
        parent_path_key = nil
      }
    elseif record and record.valid and (record.type == "blueprint" or record.type == "blueprint-book") then
      logger.player(player, "index.root-record-skipped", {
        is_preview = record.is_preview,
        record_type = record.type,
        slot = slot
      })
    end
  end

  return tasks
end

local function append_entry(index_state, task, record)
  local description = util.trim(record.blueprint_description)
  local direct_name = read_direct_label(record)
  local name = direct_name ~= "" and direct_name or description_name_fallback(record.type, description)
  local breadcrumbs = util.copy_array(task.breadcrumbs)
  breadcrumbs[#breadcrumbs + 1] = name

  local tags = read_tags(record)
  local entity_count = read_entity_count(record)

  local path_key = util.path_key(task.path)
  local entry = {
    path = task.path,
    path_key = path_key,
    parent_path_key = task.parent_path_key,
    record_type = record.type,
    name = name,
    description = description,
    breadcrumb = table.concat(breadcrumbs, " / "),
    search_name = util.normalize(name),
    search_description = util.normalize(description),
    search_breadcrumb = util.normalize(table.concat(breadcrumbs, " / ")),
    search_text = build_search_text(name, description, table.concat(breadcrumbs, " / "), tags),
    label_resolved = direct_name ~= "",
    child_path_keys = {},
    icon_sprite = nil,
    entity_count = entity_count,
    tags = tags
  }

  index_state.pending_entries[#index_state.pending_entries + 1] = entry
  index_state.pending_entry_map[path_key] = entry

  if task.parent_path_key then
    local parent = index_state.pending_entry_map[task.parent_path_key]
    if parent then
      parent.child_path_keys[#parent.child_path_keys + 1] = path_key
    end
  end

  if record.type ~= "blueprint-book" or not record.contents then
    return
  end

  for slot, child in pairs(record.contents) do
    if is_readable_record(child) then
      local child_path = util.copy_array(task.path)
      child_path[#child_path + 1] = slot

      index_state.pending_tasks[#index_state.pending_tasks + 1] = {
        path = child_path,
        breadcrumbs = breadcrumbs,
        parent_path_key = path_key
      }
    elseif child and child.valid and (child.type == "blueprint" or child.type == "blueprint-book") then
      logger.info("index.child-record-skipped", {
        is_preview = child.is_preview,
        parent_path_key = path_key,
        record_type = child.type,
        slot = slot
      })
    end
  end
end

local function start_label_resolution(index_state)
  index_state.label_queue = {}
  index_state.label_cursor = 1
  index_state.priority_label_queue = {}
  index_state.priority_label_set = {}
  index_state.resolving_labels = #index_state.entries > 0
  index_state.labels_remaining = 0

  for index = 1, #index_state.entries do
    local entry = index_state.entries[index]
    local needs_label = not entry.label_resolved
    local needs_icon = entry.icon_sprite == nil
    if needs_label or needs_icon then
      index_state.label_queue[#index_state.label_queue + 1] = entry.path_key
      if needs_label then
        index_state.labels_remaining = index_state.labels_remaining + 1
      end
    end
  end

  if #index_state.label_queue == 0 then
    clear_label_job(index_state)
  end
end

local function sort_entries(entries)
  table.sort(entries, function(left, right)
    if left.breadcrumb == right.breadcrumb then
      return left.path_key < right.path_key
    end

    return left.breadcrumb < right.breadcrumb
  end)
end

local function enqueue_priority_path(index_state, path_key)
  local entry = index_state.entry_map[path_key]
  if not entry or (entry.label_resolved and entry.icon_sprite ~= nil) then
    return
  end

  if index_state.priority_label_set[path_key] then
    return
  end

  index_state.priority_label_set[path_key] = true
  index_state.priority_label_queue[#index_state.priority_label_queue + 1] = path_key
end

local function next_label_path(index_state, allow_background)
  while index_state.priority_label_queue and #index_state.priority_label_queue > 0 do
    local path_key = table.remove(index_state.priority_label_queue, 1)
    index_state.priority_label_set[path_key] = nil

    local entry = index_state.entry_map[path_key]
    if entry and (not entry.label_resolved or entry.icon_sprite == nil) then
      return path_key, true
    end
  end

  if not allow_background then
    return nil, false
  end

  while index_state.label_queue and index_state.label_cursor <= #index_state.label_queue do
    local path_key = index_state.label_queue[index_state.label_cursor]
    index_state.label_cursor = index_state.label_cursor + 1

    local entry = index_state.entry_map[path_key]
    if entry and (not entry.label_resolved or entry.icon_sprite == nil) then
      return path_key, false
    end
  end

  return nil, false
end

local function resolve_entry_label(player, index_state, path_key)
  local entry = index_state.entry_map[path_key]
  if not entry then return false end

  -- Skip if both label and icon are already resolved (false = resolved with no custom icon)
  if entry.label_resolved and entry.icon_sprite ~= nil then
    return false
  end

  local record = resolver.resolve_record_by_path(player, entry.path)
  if not is_readable_record(record) then
    if not entry.label_resolved then
      entry.label_resolved = true
      index_state.labels_remaining = math.max(index_state.labels_remaining - 1, 0)
    end
    if entry.icon_sprite == nil then
      entry.icon_sprite = false
    end
    return false
  end

  local name, description, icon_sprite = resolve_label_metadata(record, entry.name, entry.description)
  local changed = false

  if not entry.label_resolved then
    changed = name ~= entry.name or description ~= entry.description
    entry.name = name
    entry.description = description
    entry.label_resolved = true
    refresh_entry_search(entry, index_state)
    index_state.labels_remaining = math.max(index_state.labels_remaining - 1, 0)
  end

  if entry.icon_sprite == nil then
    local new_icon = icon_sprite or false  -- false = resolved, no custom icon
    if new_icon ~= false then changed = true end
    entry.icon_sprite = new_icon
  end

  return changed
end

function M.start_rebuild(player)
  local player_state = state.ensure_player_state(player.index)
  local index_state = player_state.index

  clear_label_job(index_state)
  index_state.pending_entries = {}
  index_state.pending_entry_map = {}
  index_state.pending_tasks = queue_root_tasks(player)
  index_state.job_cursor = 1
  index_state.job_revision = index_state.rebuild_revision
  index_state.rebuilding = true
  index_state.dirty = false

  logger.player(player, "index.rebuild-started", {
    queued_tasks = #index_state.pending_tasks,
    revision = index_state.job_revision
  })
end

function M.process_rebuild_batch(player, max_records)
  local player_state = state.ensure_player_state(player.index)
  local index_state = player_state.index
  local limit = max_records or BUILD_BATCH_SIZE

  if not index_state.rebuilding then
    return false
  end

  local start_cursor = index_state.job_cursor
  local processed = 0

  while index_state.job_cursor <= #index_state.pending_tasks and processed < limit do
    local task = index_state.pending_tasks[index_state.job_cursor]
    index_state.job_cursor = index_state.job_cursor + 1
    processed = processed + 1

    local record = resolver.resolve_record_by_path(player, task.path)
    if is_readable_record(record) then
      append_entry(index_state, task, record)
    elseif record and record.valid and (record.type == "blueprint" or record.type == "blueprint-book") then
      logger.player(player, "index.task-record-skipped", {
        is_preview = record.is_preview,
        path_key = util.path_key(task.path),
        record_type = record.type
      })
    end
  end

  if index_state.job_cursor <= #index_state.pending_tasks then
    if start_cursor == 1 or math.fmod(index_state.job_cursor - 1, limit * 5) == 0 then
      logger.player(player, "index.rebuild-progress", {
        processed = index_state.job_cursor - 1,
        queued_tasks = #index_state.pending_tasks,
        revision = index_state.job_revision
      })
    end

    return true
  end

  if index_state.job_revision == index_state.rebuild_revision then
    sort_entries(index_state.pending_entries)
    index_state.entries = index_state.pending_entries
    index_state.entry_map = index_state.pending_entry_map
    index_state.entry_count = #index_state.entries
    index_state.last_rebuild_tick = game.tick
    index_state.dirty = false
    start_label_resolution(index_state)

    logger.player(player, "index.rebuild-finished", {
      entries = index_state.entry_count,
      revision = index_state.job_revision,
      tick = game.tick
    })

    if index_state.resolving_labels then
      logger.player(player, "index.label-warmup-started", {
        remaining = index_state.labels_remaining
      })
    end
  else
    logger.player(player, "index.rebuild-discarded", {
      job_revision = index_state.job_revision,
      current_revision = index_state.rebuild_revision
    })
  end

  clear_rebuild_job(index_state)
  return false
end

function M.process_label_batch(player, max_records, allow_background)
  local player_state = state.ensure_player_state(player.index)
  local index_state = player_state.index
  local limit = max_records or LABEL_BATCH_SIZE

  if not index_state.resolving_labels then
    return false
  end

  local refresh_needed = false
  local processed = 0

  while processed < limit do
    local path_key, prioritized = next_label_path(index_state, allow_background)
    if not path_key then
      if (not index_state.priority_label_queue or #index_state.priority_label_queue == 0)
        and index_state.label_queue
        and index_state.label_cursor > #index_state.label_queue then
        clear_label_job(index_state)
        logger.player(player, "index.label-warmup-finished")
      end
      break
    end

    if resolve_entry_label(player, index_state, path_key) and prioritized then
      refresh_needed = true
    end

    processed = processed + 1
  end

  if index_state.resolving_labels
    and index_state.labels_remaining == 0
    and (not index_state.label_queue or index_state.label_cursor > #index_state.label_queue)
    and (not index_state.priority_label_queue or #index_state.priority_label_queue == 0) then
    clear_label_job(index_state)
    logger.player(player, "index.label-warmup-finished")
  elseif index_state.resolving_labels then
    local resolved = #index_state.entries - index_state.labels_remaining
    if resolved > 0 and math.fmod(resolved, 100) == 0 then
      logger.player(player, "index.label-warmup-progress", {
        remaining = index_state.labels_remaining,
        resolved = resolved
      })
    end
  end

  return refresh_needed
end

function M.prioritize_entries(player, entries)
  local player_state = state.ensure_player_state(player.index)
  local index_state = player_state.index

  if not index_state.resolving_labels then
    return
  end

  local limit = math.min(#entries, PRIORITY_LIMIT)
  for index = 1, limit do
    local current = entries[index]
    local path_key = current.path_key

    while path_key do
      enqueue_priority_path(index_state, path_key)
      local entry = index_state.entry_map[path_key]
      path_key = entry and entry.parent_path_key or nil
    end
  end
end

return M
