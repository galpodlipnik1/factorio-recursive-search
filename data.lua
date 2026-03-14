---@diagnostic disable: undefined-global
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
