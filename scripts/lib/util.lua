-- Shared utilities. String normalization, search-text construction, path-key
-- encoding, sprite-path conversion, and misc helpers used across all modules.

local M = {}

function M.trim(value)
  if not value then
    return ""
  end

  return tostring(value):gsub("^%s+", ""):gsub("%s+$", "")
end

function M.normalize(value)
  local trimmed = M.trim(value)
  if trimmed == "" then
    return ""
  end

  local lowered = helpers and helpers.multilingual_to_lower and helpers.multilingual_to_lower(trimmed) or string.lower(trimmed)
  return lowered:gsub("[%s\r\n\t]+", " ")
end

local function sorted_keys(values)
  local keys = {}
  for key in pairs(values) do
    keys[#keys + 1] = key
  end
  table.sort(keys, function(left, right)
    return tostring(left) < tostring(right)
  end)
  return keys
end

local function append_search_values(parts, value, seen)
  if value == nil then
    return
  end

  if type(value) ~= "table" then
    parts[#parts + 1] = tostring(value)
    return
  end

  if seen[value] then
    return
  end
  seen[value] = true

  local keys = sorted_keys(value)
  for index = 1, #keys do
    local key = keys[index]
    if type(key) ~= "number" then
      parts[#parts + 1] = tostring(key)
    end
    append_search_values(parts, value[key], seen)
  end

  seen[value] = nil
end

function M.build_search_text(name, description, breadcrumb, ...)
  local parts = {
    name or "",
    description or "",
    breadcrumb or ""
  }

  local seen = {}
  for index = 1, select("#", ...) do
    append_search_values(parts, select(index, ...), seen)
  end

  return M.normalize(table.concat(parts, " "))
end

function M.copy_array(values)
  local out = {}
  for index = 1, #values do
    out[index] = values[index]
  end
  return out
end

function M.path_key(path)
  return table.concat(path, ".")
end

function M.fallback_name_text(record_type)
  if record_type == "blueprint-book" then
    return "[Unnamed Book]"
  end

  if record_type == "deconstruction-planner" then
    return "[Unnamed Deconstruction Planner]"
  end

  if record_type == "upgrade-planner" then
    return "[Unnamed Upgrade Planner]"
  end

  return "[Unnamed Blueprint]"
end

function M.signal_to_sprite_path(signal)
  if not signal or not signal.name then return nil end
  -- Factorio omits SignalID.type when reading item signals. Treat the absent
  -- value as the documented item default so ordinary blueprint icons resolve.
  local signal_type = signal.type or "item"
  local prefix = signal_type == "virtual" and "virtual-signal" or signal_type
  local path = prefix .. "/" .. signal.name
  if not helpers.is_valid_sprite_path(path) then return nil end
  return path
end

function M.find_entry(entries, path_key)
  if not entries then
    return nil
  end

  if entries[path_key] then
    return entries[path_key]
  end

  for index = 1, #entries do
    if entries[index].path_key == path_key then
      return entries[index]
    end
  end

  return nil
end

function M.last_rebuild_text(last_tick)
  if not last_tick then
    return { "rbf.status-never" }
  end

  return { "", "tick ", tostring(last_tick) }
end

return M
