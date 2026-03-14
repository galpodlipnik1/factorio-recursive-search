local M = {}

local function stringify(value)
  local value_type = type(value)

  if value_type == "nil" then
    return "nil"
  end

  if value_type == "boolean" or value_type == "number" then
    return tostring(value)
  end

  if value_type == "string" then
    return value
  end

  return "<" .. value_type .. ">"
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
