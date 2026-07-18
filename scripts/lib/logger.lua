-- Structured log helper. Writes tagged key=value events to the Factorio log,
-- with an optional player context variant.

local M = {}
local MAX_TABLE_DEPTH = 2
local MAX_TABLE_ITEMS = 8

local function safe_tostring(value)
  local ok, text = pcall(tostring, value)
  if ok then
    return text
  end

  return "<unprintable>"
end

local function stringify(value, depth, seen)
  local value_type = type(value)
  depth = depth or 0
  seen = seen or {}

  if value_type == "nil" then
    return "nil"
  end

  if value_type == "boolean" or value_type == "number" then
    return tostring(value)
  end

  if value_type == "string" then
    return value
  end

  if value_type == "table" then
    if seen[value] then
      return "<cycle>"
    end

    if depth >= MAX_TABLE_DEPTH then
      return "<table>"
    end

    seen[value] = true

    local parts = {}
    local count = 0
    for key, nested in pairs(value) do
      count = count + 1
      if count > MAX_TABLE_ITEMS then
        parts[#parts + 1] = "..."
        break
      end

      parts[#parts + 1] = safe_tostring(key) .. "=" .. stringify(nested, depth + 1, seen)
    end

    table.sort(parts)
    seen[value] = nil

    return "{" .. table.concat(parts, ",") .. "}"
  end

  return "<" .. value_type .. ":" .. safe_tostring(value) .. ">"
end

local function serialize_fields(fields)
  if not fields then
    return ""
  end

  local parts = {}

  for key, value in pairs(fields) do
    parts[#parts + 1] = tostring(key) .. "=" .. stringify(value)
  end

  table.sort(parts)

  if #parts == 0 then
    return ""
  end

  return " " .. table.concat(parts, " ")
end

function M.info(event_name, fields)
  log("[rbf] " .. event_name .. serialize_fields(fields))
end

function M.player(player, event_name, fields)
  local payload = fields or {}
  payload.player_index = player and player.index or nil
  payload.player_name = player and player.name or nil
  M.info(event_name, payload)
end

return M
