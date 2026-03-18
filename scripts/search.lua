local util = require("scripts.util")

local M = {}

local function rank_entry(entry, query)
  if entry.search_name == query then
    return 1
  end

  if entry.search_name:find(query, 1, true) == 1 then
    return 2
  end

  if entry.search_name:find(query, 1, true) then
    return 3
  end

  if entry.search_description:find(query, 1, true) then
    return 4
  end

  if entry.search_breadcrumb:find(query, 1, true) then
    return 5
  end

  return 6
end

function M.query(entries, query, max_results)
  local normalized = util.normalize(query)
  if normalized == "" then
    return {
      matches = {},
      total_matches = 0
    }
  end

  local ranked = {}

  for index = 1, #entries do
    local entry = entries[index]
    if entry.search_text:find(normalized, 1, true) then
      ranked[#ranked + 1] = {
        entry = entry,
        rank = rank_entry(entry, normalized)
      }
    end
  end

  table.sort(ranked, function(left, right)
    if left.rank ~= right.rank then
      return left.rank < right.rank
    end

    if left.entry.name ~= right.entry.name then
      return left.entry.name < right.entry.name
    end

    return left.entry.breadcrumb < right.entry.breadcrumb
  end)

  local matches = {}
  local total_matches = #ranked
  local limit = math.min(total_matches, max_results)

  for index = 1, limit do
    matches[index] = ranked[index].entry
  end

  return {
    matches = matches,
    total_matches = total_matches
  }
end

function M.children(entries, parent_path_key, max_results)
  local matches = {}

  for index = 1, #entries do
    local entry = entries[index]
    if entry.parent_path_key == parent_path_key then
      matches[#matches + 1] = entry
    end
  end

  table.sort(matches, function(left, right)
    if left.record_type ~= right.record_type then
      return left.record_type == "blueprint-book"
    end

    if left.name ~= right.name then
      return left.name < right.name
    end

    return left.path_key < right.path_key
  end)

  local total_matches = #matches
  local limit = math.min(total_matches, max_results)
  local limited = {}

  for index = 1, limit do
    limited[index] = matches[index]
  end

  return {
    matches = limited,
    total_matches = total_matches
  }
end

return M
