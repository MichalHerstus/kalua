-- Per-Widget Styling demo (kforms_enhancements.md §10)
-- Demonstrates the bg / color / font / font_size / style opts shared by all
-- k.ctrl.* constructors, the h1-h6 / p text presets, and live restyling via
-- k.ctrl.set_property.
-- Run with: ./KALUA run testdata/apps/widget_styles_demo.lua

function main()
  k.form.new("styles", {
    title = "Per-Widget Styling",
    layout = "vertical",
    align = "center",
    gap = 10
  })

  -- Heading via the h1 preset (font-size 2em, weight 700)
  k.ctrl.label("styles", "heading", {
    text  = "Widget Styling",
    font  = "h1",
    color = "#1976d2"
  })

  -- Subtitle via the p preset (font-size 1em, weight 400)
  k.ctrl.label("styles", "subtitle", {
    text  = "Every control accepts bg, color, font, font_size and style.",
    style = "p",
    color = "#757575"
  })

  -- h2/h3 presets + background
  k.ctrl.label("styles", "section", {
    text = "Section heading (h2)",
    font = "h2",
    bg   = "#f5f5f5",
    color = "#212121"
  })
  k.ctrl.label("styles", "note", {
    text = "Muted note using the style = \"h3\" alias preset (1.17em)",
    style = "h3",
    color = "#9e9e9e"
  })

  -- A raw font family instead of a preset
  k.ctrl.label("styles", "serif", {
    text      = "Georgia serif, 16px, white on dark blue",
    font      = "Georgia",
    font_size = 16,
    color     = "#fff",
    bg        = "#1a237e"
  })

  -- Styled input: the styling lands on the .kalua-control wrapper
  k.ctrl.textbox("styles", "txt", {
    label     = "Styled textbox",
    value     = "type something…",
    bg        = "#e3f2fd",
    color     = "#0d47a1",
    font_size = 14
  })

  -- Styled combo
  k.ctrl.combo("styles", "combo", {
    label = "Styled combo",
    items = {["1"] = "Red", ["2"] = "Green", ["3"] = "Blue"},
    value = "2",
    bg    = "#fff8e1",
    font_size = 13
  })

  -- Styled checkbox + radio
  k.ctrl.checkbox("styles", "agree", {
    label = "I agree (styled)",
    value = true,
    color = "#2e7d32",
    bg    = "#e8f5e9"
  })
  k.ctrl.radio("styles", "plan", {
    label = "Plan",
    items = {["1"] = "Basic", ["2"] = "Pro"},
    value = "2",
    color = "#6a1b9a",
    bg    = "#f3e5f5"
  })

  -- Styled button
  k.ctrl.button("styles", "btn_save", {
    label     = "Save",
    bg        = "#1976d2",
    color     = "#fff",
    font_size = 16
  })

  -- Live restyle: cycle the palette on the section label and button
  local palette = {
    {bg = "#1976d2", color = "#fff"},
    {bg = "#2e7d32", color = "#fff"},
    {bg = "#d32f2f", color = "#fff"},
    {bg = "#6a1b9a", color = "#fff"},
    {bg = "#f57f17", color = "#212121"}
  }
  local idx = 1

  k.ctrl.button("styles", "btn_cycle", {
    label = "Cycle styling (set_property)",
    onclick = function()
      idx = idx % #palette + 1
      local p = palette[idx]
      k.ctrl.set_property("styles", "section", "bg", p.bg)
      k.ctrl.set_property("styles", "section", "color", p.color)
      k.ctrl.set_property("styles", "btn_save", "bg", p.bg)
      k.ctrl.set_property("styles", "btn_save", "color", p.color)

      -- Read the keys back with get_property and reflect them in a label
      local bg  = k.ctrl.get_property("styles", "section", "bg")
      local col = k.ctrl.get_property("styles", "section", "color")
      k.ctrl.set_property("styles", "status", "label",
        "section bg = " .. tostr(bg) .. ", color = " .. tostr(col) .. " (step " .. idx .. ")")
    end
  })

  -- Read-back line (updated by the cycle button)
  k.ctrl.label("styles", "status", {
    text      = "Press the cycle button to restyle live.",
    font_size = 13,
    color     = "#757575"
  })

  k.form.show("styles", { modal = true })
end
