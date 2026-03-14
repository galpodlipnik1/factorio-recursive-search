---@diagnostic disable: undefined-global
local logger = require("scripts.logger")

local M = {}

function M.place_record(player, record)
  if not (player and player.valid and record and record.valid) then
    logger.player(player, "placement.invalid-arguments", {
      has_player = player and player.valid or false,
      has_record = record and record.valid or false
    })
    return false
  end

  if record.type ~= "blueprint" then
    logger.player(player, "placement.unsupported-record-type", {
      record_type = record.type
    })
    return false
  end

  if record.is_preview then
    logger.player(player, "placement.preview-record-skipped")
    return false
  end

  local export_string = record.export_record()
  if not export_string or export_string == "" then
    logger.player(player, "placement.export-missing")
    return false
  end

  local inventory = game.create_inventory(1)
  local stack = inventory[1]
  if not stack then
    inventory.destroy()
    logger.player(player, "placement.stack-missing")
    return false
  end

  local ok = pcall(function()
    stack.import_stack(export_string)
  end)

  if not ok then
    inventory.destroy()
    logger.player(player, "placement.import-failed")
    return false
  end

  player.clear_cursor()
  player.add_to_clipboard(stack)
  player.activate_paste()

  inventory.destroy()
  logger.player(player, "placement.completed")
  return true
end

return M
