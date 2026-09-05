-- tabulator_demo.lua
-- Demonstrates the Tabulator-enhanced table control (kforms_enhancements.md §1).
-- Run: ./KALUA run testdata/apps/tabulator_demo.lua

local ROWS = {
  {name = "Alice",   role = "Admin",   active = true,  score = 98},
  {name = "Bob",     role = "Editor",  active = false, score = 74},
  {name = "Carol",   role = "Editor",  active = true,  score = 86},
  {name = "Dave",    role = "Viewer",  active = true,  score = 55},
  {name = "Eve",     role = "Admin",   active = false, score = 91},
}

function main()
  k.form.new("main", {title = "Tabulator Table Demo"})

  k.ctrl.table("main", "users", {
    label = "Users",
    tabulator = true,
    -- data is inferred into columns by type (number/boolean/string) client-side
    data = ROWS,
  })

  k.ctrl.button("main", "reload", {
    label = "Reload",
    onclick = function()
      k.table.set_data("main", "users", ROWS)
      k.status_show("Reloaded " .. #ROWS .. " rows")
    end,
  })

  k.form.on("main", "users", "tabulator_selection_change", function(sel)
    k.print("selection:", #sel.rows, "rows selected")
  end)

  k.form.show("main")
end