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

function M.build_search_text(name, description, breadcrumb)
  return M.normalize(table.concat({
    name or "",
    description or "",
    breadcrumb or ""
  }, " "))
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

  return "[Unnamed Blueprint]"
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
