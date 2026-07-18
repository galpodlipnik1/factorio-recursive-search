-- Blueprint index pipeline. Handles incremental rebuild (100 records/tick) and
-- a two-phase warmup that resolves descriptions, labels, and custom icons via
-- decoded temp stacks and record exchange strings.

---@diagnostic disable: undefined-global
local logger = require("scripts.lib.logger")
local resolver = require("scripts.lib.resolver")
local state = require("scripts.index.state")
local util = require("scripts.lib.util")

local M = {}
local BUILD_BATCH_SIZE = 100
local LABEL_BATCH_SIZE = 1
local PRIORITY_LIMIT = 30
local PREVIEW_ICON_BATCH_SIZE = 10
local FALLBACK_NAME_MAX = 48

local SUPPORTED_RECORD_TYPES = {
  ["blueprint"] = true,
  ["blueprint-book"] = true,
  ["deconstruction-planner"] = true,
  ["upgrade-planner"] = true
}

local function is_supported_record_type(record_type)
  return SUPPORTED_RECORD_TYPES[record_type] == true
end

local function is_readable_record(record)
  return record
    and record.valid
    and not record.is_preview
    and is_supported_record_type(record.type)
end

local function safe_destroy_inventory(inventory)
  if inventory and inventory.valid then
    inventory.destroy()
  end
end

local function build_search_text(name, description, breadcrumb, tags, entity_names, planner)
  return util.build_search_text(name, description, breadcrumb, tags, entity_names, planner)
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
  local ok, icons = pcall(function() return stack.preview_icons end)
  local method_ok, get_blueprint_icons = pcall(function() return stack.get_blueprint_icons end)
  if (not ok or type(icons) ~= "table") and method_ok and get_blueprint_icons then
    ok, icons = pcall(function() return stack.get_blueprint_icons() end)
  end
  if not ok or not icons or not icons[1] or not icons[1].signal then return nil end
  return util.signal_to_sprite_path(icons[1].signal)
end

local function read_icon_sprite_from_record(record)
  local ok, icons = pcall(function() return record.preview_icons end)
  if not ok or type(icons) ~= "table" then
    return nil
  end

  local selected = nil
  local selected_index = nil
  for key, icon in pairs(icons) do
    if type(icon) == "table" and icon.signal then
      local icon_index = tonumber(icon.index) or tonumber(key) or 1
      if not selected_index or icon_index < selected_index then
        selected = icon
        selected_index = icon_index
      end
    end
  end
  return selected and util.signal_to_sprite_path(selected.signal) or nil
end

local function read_description(record)
  local ok, description = pcall(function()
    if record.type == "deconstruction-planner" or record.type == "upgrade-planner" then
      return record.planner_description
    end
    return record.blueprint_description
  end)
  return ok and util.trim(description) or ""
end

local function read_description_from_stack(stack, record_type)
  local ok, description = pcall(function()
    if record_type == "deconstruction-planner" or record_type == "upgrade-planner" then
      return stack.planner_description
    end
    return stack.blueprint_description
  end)
  return ok and util.trim(description) or ""
end

local function read_planner_description_from_export(export_string, record_type)
  if record_type ~= "deconstruction-planner" and record_type ~= "upgrade-planner" then
    return ""
  end
  if type(export_string) ~= "string" or #export_string < 2 then
    return ""
  end

  local ok, description = pcall(function()
    local json = helpers.decode_string(export_string:sub(2))
    if not json then return "" end
    local decoded = helpers.json_to_table(json)
    if type(decoded) ~= "table" then return "" end

    local key = record_type:gsub("%-", "_")
    local planner = decoded[key]
    if type(planner) ~= "table" then return "" end
    if planner.description then return planner.description end
    if type(planner.settings) == "table" then
      return planner.settings.description or ""
    end
    return ""
  end)
  return ok and util.trim(description) or ""
end

local function read_tags(record)
  if record.type ~= "blueprint" then return {} end
  local ok, tags = pcall(function()
    local entities = record.get_blueprint_entities()
    if type(entities) ~= "table" then return {} end

    local entity_tags = {}
    for index = 1, #entities do
      local entity = entities[index]
      local entity_index = entity.entity_number or index
      local current = record.get_blueprint_entity_tags(entity_index)
      if type(current) == "table" and next(current) ~= nil then
        entity_tags[tostring(entity_index)] = current
      end
    end

    if next(entity_tags) == nil then return {} end
    return { entities = entity_tags }
  end)
  return (ok and type(tags) == "table") and tags or {}
end

local function read_entity_count(record)
  if record.type ~= "blueprint" then return 0 end
  local ok, entities = pcall(function() return record.get_blueprint_entities() end)
  return (ok and type(entities) == "table") and #entities or 0
end

local function read_entity_names(record)
  if record.type ~= "blueprint" then return {} end

  local ok, names = pcall(function()
    local entities = record.get_blueprint_entities()
    if type(entities) ~= "table" then return {} end

    local seen = {}
    for index = 1, #entities do
      local name = entities[index] and entities[index].name
      if type(name) == "string" and name ~= "" then
        seen[name] = true
      end
    end

    local result = {}
    for name in pairs(seen) do
      result[#result + 1] = name
    end
    table.sort(result)
    return result
  end)

  return (ok and type(names) == "table") and names or {}
end

local function copy_filter(filter, index, prototype_type)
  local result = { index = index, type = prototype_type }
  if type(filter) == "string" then
    result.name = filter
    result.quality = "normal"
    return result
  end

  if type(filter) ~= "table" then
    return result
  end

  result.name = filter.name
  result.quality = filter.quality or "normal"
  result.comparator = filter.comparator
  return result
end

local function enum_name(values, value)
  if type(values) == "table" then
    for name, candidate in pairs(values) do
      if candidate == value then
        return name
      end
    end
  end
  return value ~= nil and tostring(value) or nil
end

local function read_deconstruction_planner(record)
  if record.type ~= "deconstruction-planner" then return nil end

  local ok, details = pcall(function()
    local entity_filters = {}
    for index, filter in pairs(record.entity_filters or {}) do
      entity_filters[#entity_filters + 1] = copy_filter(filter, index - 1, "entity")
    end
    table.sort(entity_filters, function(left, right) return left.index < right.index end)

    local tile_filters = {}
    for index, name in pairs(record.tile_filters or {}) do
      tile_filters[#tile_filters + 1] = {
        index = index - 1,
        type = "tile",
        name = name,
        quality = "normal"
      }
    end
    table.sort(tile_filters, function(left, right) return left.index < right.index end)

    return {
      entity_filters = entity_filters,
      tile_filters = tile_filters,
      entity_filter_mode = enum_name(defines.deconstruction_item.entity_filter_mode, record.entity_filter_mode),
      tile_filter_mode = enum_name(defines.deconstruction_item.tile_filter_mode, record.tile_filter_mode),
      tile_selection_mode = enum_name(defines.deconstruction_item.tile_selection_mode, record.tile_selection_mode),
      trees_and_rocks_only = record.trees_and_rocks_only == true,
      mappers = {}
    }
  end)

  return ok and details or nil
end

local function copy_mapper_endpoint(endpoint)
  if type(endpoint) ~= "table" then return nil end

  local result = {
    type = endpoint.type,
    name = endpoint.name,
    quality = endpoint.quality or "normal",
    comparator = endpoint.comparator
  }

  if endpoint.module_limit and endpoint.module_limit > 0 then
    result.module_limit = endpoint.module_limit
  end

  if type(endpoint.module_filter) == "table" then
    result.module_filter = copy_filter(endpoint.module_filter, nil, "entity")
  elseif type(endpoint.module_filter) == "string" then
    result.module_filter = copy_filter(endpoint.module_filter, nil, "entity")
  end

  if type(endpoint.module_slots) == "table" then
    local module_slots = {}
    for index, module in pairs(endpoint.module_slots) do
      module_slots[#module_slots + 1] = copy_filter(module, index, "item")
    end
    if #module_slots > 0 then
      table.sort(module_slots, function(left, right) return left.index < right.index end)
      result.module_slots = module_slots
    end
  end

  return result
end

local function mapper_endpoint_is_occupied(endpoint)
  if type(endpoint) ~= "table" then return false end
  if endpoint.name or endpoint.module_filter then return true end
  if endpoint.module_limit and endpoint.module_limit > 0 then return true end
  return type(endpoint.module_slots) == "table" and next(endpoint.module_slots) ~= nil
end

local function read_upgrade_planner(record)
  if record.type ~= "upgrade-planner" then return nil end

  local ok, details = pcall(function()
    local mappers = {}
    for index = 1, record.mapper_count do
      local from = record.get_mapper(index, "from")
      local to = record.get_mapper(index, "to")
      if mapper_endpoint_is_occupied(from) or mapper_endpoint_is_occupied(to) then
        mappers[#mappers + 1] = {
          index = index - 1,
          from = copy_mapper_endpoint(from),
          to = copy_mapper_endpoint(to)
        }
      end
    end
    return {
      entity_filters = {},
      tile_filters = {},
      trees_and_rocks_only = false,
      mappers = mappers
    }
  end)

  return ok and details or nil
end

local function read_planner(record)
  return read_deconstruction_planner(record) or read_upgrade_planner(record)
end

local function description_name_fallback(record_type, description)
  local compact = util.trim(description):gsub("%s+", " ")
  if compact == "" then
    return util.fallback_name_text(record_type)
  end

  if #compact > FALLBACK_NAME_MAX then
    local first_excluded = FALLBACK_NAME_MAX - 2
    local byte = compact:byte(first_excluded)
    while first_excluded > 1 and byte and byte >= 0x80 and byte <= 0xbf do
      first_excluded = first_excluded - 1
      byte = compact:byte(first_excluded)
    end
    compact = compact:sub(1, first_excluded - 1) .. "..."
  end

  return compact
end

local function create_temp_stack_from_record(record)
  if not is_readable_record(record) then
    logger.info("index.metadata-record-skipped", {
      is_preview = record and record.valid and record.is_preview or nil,
      record_type = record and record.valid and record.type or nil
    })
    return nil, nil, nil
  end

  local export_ok, export_string = pcall(function() return record.export_record() end)
  if not export_ok or not export_string or export_string == "" then
    logger.info("index.metadata-export-missing", {
      record_type = record and record.type or nil
    })
    return nil, nil, nil
  end

  local inventory = game.create_inventory(1)
  local stack = inventory[1]
  if not (stack and stack.valid) then
    safe_destroy_inventory(inventory)
    logger.info("index.metadata-stack-missing", {
      record_type = record and record.type or nil
    })
    return nil, nil, export_string
  end

  local ok = pcall(function()
    stack.import_stack(export_string)
  end)

  if not ok or not stack.valid_for_read then
    safe_destroy_inventory(inventory)
    logger.info("index.metadata-import-failed", {
      record_type = record and record.type or nil
    })
    return nil, nil, export_string
  end

  return inventory, stack, export_string
end

local function resolve_label_metadata(record, fallback_name, fallback_description)
  local description = read_description(record)
  if description == "" then
    description = fallback_description or ""
  end

  -- Always create temp stack: it's the only way to read custom blueprint icons.
  -- Direct label check is done after, so we can fall back without a stack if export fails.
  local inventory, stack, export_string = create_temp_stack_from_record(record)
  local export_description = read_planner_description_from_export(export_string, record.type)
  if export_description ~= "" then
    description = export_description
  end
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

  local stack_description = read_description_from_stack(stack, record.type)
  if stack_description ~= "" then
    description = stack_description
  end

  local icon_sprite = read_icon_sprite_from_record(record) or read_icon_sprite_from_stack(stack)
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
  entry.search_text = build_search_text(entry.name, entry.description, entry.breadcrumb, entry.tags, entry.entity_names, entry.planner)
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
    elseif record and record.valid and is_supported_record_type(record.type) then
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
  local description = read_description(record)
  local direct_name = read_direct_label(record)
  local name = direct_name ~= "" and direct_name or description_name_fallback(record.type, description)
  local breadcrumbs = util.copy_array(task.breadcrumbs)
  breadcrumbs[#breadcrumbs + 1] = name

  local tags = read_tags(record)
  local entity_count = read_entity_count(record)
  local entity_names = read_entity_names(record)
  local planner = read_planner(record)
  local icon_sprite = read_icon_sprite_from_record(record)

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
    search_text = build_search_text(name, description, table.concat(breadcrumbs, " / "), tags, entity_names, planner),
    label_resolved = direct_name ~= "",
    description_resolved = false,
    child_path_keys = {},
    icon_sprite = icon_sprite,
    entity_count = entity_count,
    entity_names = entity_names,
    tags = tags,
    planner = planner,
    runtime_semantics_resolved = true
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
    elseif child and child.valid and is_supported_record_type(child.type) then
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
    local needs_description = not entry.description_resolved
    local needs_icon = entry.icon_sprite == nil
    local needs_runtime_semantics = not entry.runtime_semantics_resolved
    if needs_label or needs_description or needs_icon or needs_runtime_semantics then
      index_state.label_queue[#index_state.label_queue + 1] = entry.path_key
      index_state.labels_remaining = index_state.labels_remaining + 1
    end
  end

  if #index_state.label_queue == 0 then
    clear_label_job(index_state)
  end
end

local function sort_entries(entries)
  local function path_less(left, right)
    local limit = math.min(#left, #right)
    for index = 1, limit do
      if left[index] ~= right[index] then
        return left[index] < right[index]
      end
    end
    return #left < #right
  end

  table.sort(entries, function(left, right)
    if left.breadcrumb == right.breadcrumb then
      return path_less(left.path, right.path)
    end

    return left.breadcrumb < right.breadcrumb
  end)
end

local function rebuild_child_paths(entries, entry_map)
  for index = 1, #entries do
    entries[index].child_path_keys = {}
  end

  -- Rebuild after the final entry sort so runtime and prebuilt indexes use
  -- identical child ordering regardless of Lua table iteration order.
  for index = 1, #entries do
    local entry = entries[index]
    if entry.parent_path_key then
      local parent = entry_map[entry.parent_path_key]
      if parent then
        parent.child_path_keys[#parent.child_path_keys + 1] = entry.path_key
      end
    end
  end
end

local function enqueue_priority_path(index_state, path_key)
  local entry = index_state.entry_map[path_key]
  if not entry or (
    entry.label_resolved
    and entry.description_resolved
    and entry.icon_sprite ~= nil
    and entry.runtime_semantics_resolved
  ) then
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
    if entry and (
      not entry.label_resolved
      or not entry.description_resolved
      or entry.icon_sprite == nil
      or not entry.runtime_semantics_resolved
    ) then
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
    if entry and (
      not entry.label_resolved
      or not entry.description_resolved
      or entry.icon_sprite == nil
      or not entry.runtime_semantics_resolved
    ) then
      return path_key, false
    end
  end

  return nil, false
end

local function resolve_entry_label(player, index_state, path_key)
  local entry = index_state.entry_map[path_key]
  if not entry then return false end

  -- false icon means the lookup completed and no custom icon was present.
  if entry.label_resolved
    and entry.description_resolved
    and entry.icon_sprite ~= nil
    and entry.runtime_semantics_resolved then
    return false
  end

  local record = resolver.resolve_record_by_path(player, entry.path)
  if not is_readable_record(record) then
    entry.label_resolved = true
    entry.description_resolved = true
    entry.runtime_semantics_resolved = true
    if entry.icon_sprite == nil then
      entry.icon_sprite = false
    end
    index_state.labels_remaining = math.max(index_state.labels_remaining - 1, 0)
    return false
  end

  local name, description, icon_sprite = resolve_label_metadata(record, entry.name, entry.description)
  local changed = name ~= entry.name or description ~= entry.description

  if not entry.runtime_semantics_resolved then
    entry.tags = read_tags(record)
    entry.entity_count = read_entity_count(record)
    entry.entity_names = read_entity_names(record)
    entry.planner = read_planner(record)
    entry.runtime_semantics_resolved = true
    changed = true
  end

  entry.label_resolved = true
  entry.description_resolved = true

  if changed then
    entry.name = name
    entry.description = description
    refresh_entry_search(entry, index_state)
  end

  if entry.icon_sprite == nil then
    local new_icon = icon_sprite or false  -- false = resolved, no custom icon
    if new_icon ~= false then changed = true end
    entry.icon_sprite = new_icon
  end

  index_state.labels_remaining = math.max(index_state.labels_remaining - 1, 0)

  return changed
end

local function enqueue_preview_icon(index_state, entry)
  if index_state.source ~= "prebuilt"
    or not entry
    or entry.preview_icon_resolved
    or index_state.preview_icon_set[entry.path_key] then
    return
  end

  index_state.preview_icon_set[entry.path_key] = true
  index_state.preview_icon_queue[#index_state.preview_icon_queue + 1] = entry.path_key
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
    elseif record and record.valid and is_supported_record_type(record.type) then
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
    rebuild_child_paths(index_state.pending_entries, index_state.pending_entry_map)
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

    if resolve_entry_label(player, index_state, path_key) then
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

-- Resolves only visible prebuilt entries through LuaRecord.preview_icons. This
-- deliberately avoids the export/import and metadata work used by fallback
-- runtime indexing, and it never changes the prebuilt-ready status.
function M.process_preview_icon_batch(player, max_records)
  local index_state = state.ensure_player_state(player.index).index
  if index_state.source ~= "prebuilt" or #index_state.preview_icon_queue == 0 then
    return false
  end

  local limit = max_records or PREVIEW_ICON_BATCH_SIZE
  local processed = 0
  local refresh_needed = false

  while processed < limit and #index_state.preview_icon_queue > 0 do
    local path_key = table.remove(index_state.preview_icon_queue, 1)
    index_state.preview_icon_set[path_key] = nil

    local entry = index_state.entry_map[path_key]
    if entry and not entry.preview_icon_resolved then
      local record = resolver.resolve_record_by_path(player, entry.path)
      local icon_sprite = false
      if is_readable_record(record) and record.type == entry.record_type then
        icon_sprite = read_icon_sprite_from_record(record) or false
      end

      entry.preview_icon_resolved = true
      if icon_sprite ~= false and icon_sprite ~= entry.icon_sprite then
        refresh_needed = true
      end
      entry.icon_sprite = icon_sprite
    end

    processed = processed + 1
  end

  return refresh_needed
end

function M.prioritize_entries(player, entries)
  local player_state = state.ensure_player_state(player.index)
  local index_state = player_state.index

  if index_state.source == "prebuilt" then
    -- A new query replaces pending work so rapid typing never leaves an
    -- unbounded queue of results that are no longer visible.
    index_state.preview_icon_queue = {}
    index_state.preview_icon_set = {}
  end

  local limit = math.min(#entries, PRIORITY_LIMIT)
  for index = 1, limit do
    local current = entries[index]
    enqueue_preview_icon(index_state, current)

    if index_state.resolving_labels then
      local path_key = current.path_key
      while path_key do
        enqueue_priority_path(index_state, path_key)
        local entry = index_state.entry_map[path_key]
        path_key = entry and entry.parent_path_key or nil
      end
    end
  end
end

return M
