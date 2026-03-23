-- Path resolver. Traverses the player blueprint inventory by slot-index path
-- to retrieve a specific blueprint or book record.

local M = {}

function M.resolve_record_by_path(player, path)
  local records = player.blueprints
  local record = nil

  for depth = 1, #path do
    local slot = path[depth]
    if not records then
      return nil
    end

    record = records[slot]
    if not (record and record.valid) then
      return nil
    end

    if depth < #path then
      if record.type ~= "blueprint-book" then
        return nil
      end

      records = record.contents
    end
  end

  return record
end

return M
