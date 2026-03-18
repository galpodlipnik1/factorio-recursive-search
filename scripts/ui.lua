local indexer = require("scripts.indexer")
local state = require("scripts.state")
local search = require("scripts.search")
local util = require("scripts.util")

local M = {}
local LEGACY_TOP_BUTTON = "rbf_top_button"

M.names = {
  frame = "rbf_frame",
  toolbar = "rbf_toolbar",
  query = "rbf_query",
  refresh = "rbf_refresh",
  close = "rbf_close",
  back = "rbf_back",
  status = "rbf_status",
  results = "rbf_results",
  help = "rbf_help"
}
M.shortcut_name = "rbf-shortcut"

local MAX_RESULTS = 30
local MIN_QUERY_LENGTH = 2

local function get_frame(player)
  return player.gui.screen[M.names.frame]
end

local function get_results_model(player)
  local player_state = state.get_player_state(player.index)
  local entries = player_state.index.entries

  if player_state.ui.mode == "browse" and player_state.ui.browse_path_key then
    return search.children(entries, player_state.ui.browse_path_key, MAX_RESULTS)
  end

  if #util.normalize(player_state.ui.query) < MIN_QUERY_LENGTH then
    return {
      matches = {},
      total_matches = 0
    }
  end

  return search.query(entries, player_state.ui.query, MAX_RESULTS)
end

local function status_state_text(index_state)
  if index_state.rebuilding then
    return {"rbf.status-rebuilding"}
  end

  if index_state.resolving_labels then
    return {"rbf.status-warming"}
  end

  if index_state.dirty then
    return {"rbf.status-stale"}
  end

  return {"rbf.status-ready"}
end

local function entry_breadcrumb(player_state, entry)
  local segments = {}
  local current = entry

  while current do
    segments[#segments + 1] = current.name
    if not current.parent_path_key then
      break
    end

    current = player_state.index.entry_map[current.parent_path_key]
  end

  local breadcrumb = {}
  for index = #segments, 1, -1 do
    breadcrumb[#breadcrumb + 1] = segments[index]
  end

  return table.concat(breadcrumb, " / ")
end

local function entry_display_name(entry)
  local name = util.trim(entry.name)
  if name ~= "" then
    return name
  end

  return util.fallback_name_text(entry.record_type)
end

local function entry_type_text(entry)
  if entry.record_type == "blueprint-book" then
    return {"rbf.tag-book"}
  end

  return {"rbf.tag-blueprint"}
end

local function entry_button_caption(player_state, entry)
  local caption = {
    "",
    entry_display_name(entry),
    " [",
    entry_type_text(entry),
    "]",
    "\n",
    {"rbf.path-line", entry_breadcrumb(player_state, entry)}
  }

  if entry.description ~= "" then
    caption[#caption + 1] = "\n"
    caption[#caption + 1] = {"rbf.description-line", entry.description}
  end

  return caption
end

local function add_result_row(scroll_pane, player_state, entry)
  local action = entry.record_type == "blueprint-book" and "open-book" or "place-blueprint"
  local button = scroll_pane.add({
    type = "button",
    caption = entry_button_caption(player_state, entry),
    tags = {
      action = action,
      path_key = entry.path_key
    }
  })
  button.style.horizontally_stretchable = true
  button.style.horizontal_align = "left"
  button.style.font = "default"
  button.style.top_padding = 8
  button.style.bottom_padding = 8
  button.style.left_padding = 10
  button.style.right_padding = 10
end

function M.sync_shortcut(player)
  local player_state = state.get_player_state(player.index)
  local legacy_button = player.gui.top[LEGACY_TOP_BUTTON]
  if legacy_button then
    legacy_button.destroy()
  end
  player.set_shortcut_toggled(M.shortcut_name, player_state.ui.open)
end

function M.build(player)
  local frame = get_frame(player)
  if frame then
    return frame
  end

  frame = player.gui.screen.add({
    type = "frame",
    name = M.names.frame,
    direction = "vertical"
  })
  frame.auto_center = true
  frame.style.minimal_width = 860
  frame.style.minimal_height = 560
  frame.style.maximal_height = 700

  local titlebar = frame.add({
    type = "flow",
    direction = "horizontal"
  })
  titlebar.drag_target = frame

  titlebar.add({
    type = "label",
    caption = {"rbf.title"},
    style = "frame_title",
    ignored_by_interaction = true
  })

  local spacer = titlebar.add({
    type = "empty-widget",
    ignored_by_interaction = true
  })
  spacer.style.horizontally_stretchable = true
  spacer.style.height = 24
  spacer.drag_target = frame

  titlebar.add({
    type = "sprite-button",
    name = M.names.refresh,
    sprite = "utility/refresh",
    style = "frame_action_button",
    tooltip = {"rbf.refresh"}
  })

  titlebar.add({
    type = "sprite-button",
    name = M.names.close,
    sprite = "utility/close",
    style = "frame_action_button",
    tooltip = {"rbf.close"}
  })

  local toolbar = frame.add({
    type = "flow",
    name = M.names.toolbar,
    direction = "horizontal"
  })
  toolbar.style.horizontally_stretchable = true
  toolbar.style.horizontal_spacing = 8

  toolbar.add({
    type = "button",
    name = M.names.back,
    caption = {"rbf.back"}
  })

  local query = toolbar.add({
    type = "textfield",
    name = M.names.query,
    text = ""
  })
  query.style.horizontally_stretchable = true

  frame.add({
    type = "label",
    name = M.names.status,
    caption = ""
  })

  local scroll_pane = frame.add({
    type = "scroll-pane",
    name = M.names.results,
    direction = "vertical"
  })
  scroll_pane.style.horizontally_stretchable = true
  scroll_pane.style.vertically_stretchable = true
  scroll_pane.style.minimal_height = 360
  scroll_pane.style.maximal_height = 520

  frame.add({
    type = "label",
    name = M.names.help,
    caption = {"rbf.help"}
  })

  return frame
end

function M.open(player)
  local player_state = state.get_player_state(player.index)
  local frame = M.build(player)

  player_state.ui.open = true
  M.sync_shortcut(player)
  player.opened = frame

  M.refresh(player)
end

function M.close(player)
  local frame = get_frame(player)
  if frame then
    frame.destroy()
  end

  state.reset_ui(player.index)
  M.sync_shortcut(player)
end

function M.refresh(player)
  local frame = M.build(player)
  local player_state = state.get_player_state(player.index)
  local model = get_results_model(player)
  local toolbar = frame[M.names.toolbar]
  local query = toolbar and toolbar[M.names.query]
  local back = toolbar and toolbar[M.names.back]

  if not query or not back then
    error("Recursive Blueprint Finder UI is missing toolbar controls.")
  end

  if query.text ~= (player_state.ui.query or "") then
    query.text = player_state.ui.query or ""
  end

  back.visible = player_state.ui.mode == "browse"

  frame[M.names.status].caption = {
    "rbf.status-line",
    tostring(player_state.index.entry_count),
    tostring(model.total_matches),
    status_state_text(player_state.index),
    util.last_rebuild_text(player_state.index.last_rebuild_tick)
  }

  indexer.prioritize_entries(player, model.matches)

  local scroll_pane = frame[M.names.results]
  scroll_pane.clear()

  if player_state.index.rebuilding then
    scroll_pane.add({
      type = "label",
      caption = {"rbf.indexing"}
    })
    return
  end

  local normalized_query = util.normalize(player_state.ui.query)
  if player_state.ui.mode == "search" and normalized_query == "" then
    scroll_pane.add({
      type = "label",
      caption = {"rbf.empty-query"}
    })
    return
  end

  if player_state.ui.mode == "search" and #normalized_query < MIN_QUERY_LENGTH then
    scroll_pane.add({
      type = "label",
      caption = {"rbf.short-query", tostring(MIN_QUERY_LENGTH)}
    })
    return
  end

  if #model.matches == 0 then
    scroll_pane.add({
      type = "label",
      caption = player_state.ui.mode == "browse" and {"rbf.browse-empty"} or {"rbf.no-results"}
    })
    return
  end

  for index = 1, #model.matches do
    add_result_row(scroll_pane, player_state, model.matches[index])
  end
end

return M
