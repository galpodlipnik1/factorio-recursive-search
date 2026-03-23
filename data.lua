---@diagnostic disable: undefined-global

data.raw["gui-style"]["default"]["rbf_multiline_button"] = {
  type = "button_style",
  parent = "button",
  single_line = false,
  horizontally_stretchable = "on",
  horizontal_align = "left",
  font = "default",
  top_padding = 6,
  bottom_padding = 6,
  left_padding = 10,
  right_padding = 10,
}

data:extend({
  {
    type = "custom-input",
    name = "rbf-toggle",
    key_sequence = "CONTROL + SHIFT + F",
    consuming = "none"
  },
  {
    type = "shortcut",
    name = "rbf-shortcut",
    order = "b[blueprints]-z[recursive-blueprint-finder]",
    action = "lua",
    toggleable = true,
    associated_control_input = "rbf-toggle",
    style = "blue",
    icon = "__base__/graphics/icons/blueprint-book.png",
    icon_size = 64,
    small_icon = "__base__/graphics/icons/blueprint-book.png",
    small_icon_size = 64
  }
})
