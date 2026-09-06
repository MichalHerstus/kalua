-- Enhanced Form Layout demo (kforms_enhancements.md §6)
-- Grid layout first, then Vertical alignment on button press
-- Run with: ./KALUA run testdata/apps/layout_demo.lua

function main()
    -- Grid form with cells (shown first)
    k.form.new("grid_demo", {
        title = "Dashboard (layout=grid)",
        layout = "grid",
        gap = 16,
        cells = {
            {id = "header", width = 12, bg = "#f5f5f5", align = "center",
             border = {width = 1, color = "#ddd"}},
            {id = "sidebar", width = 3, bg = "#fff", align = "left"},
            {id = "main", width = 9, bg = "#fff", align = "center"},
            {id = "footer", width = 12, bg = "#fafafa",
             border = {width = 2, color = "#ccc"}}
        }
    })

    -- Header cell (full width)
    k.ctrl.textbox("grid_demo", "search", {label = "Search", cell = "header", value = "type to search..."})
    k.ctrl.button("grid_demo", "btn_refresh", {label = "Refresh", cell = "header", onclick = function()
        k.ctrl.set_value("grid_demo", "data", "Refreshed at " .. os.date("%H:%M:%S"))
    end})

    -- Sidebar cell (narrow)
    k.ctrl.list("grid_demo", "menu", {cell = "sidebar", items = {["1"] = "Dashboard", ["2"] = "Reports", ["3"] = "Settings"}, size = 6})

    -- Main cell (wide)
    k.ctrl.textbox("grid_demo", "data", {label = "Content", cell = "main", value = "Main content area"})
    k.ctrl.button("grid_demo", "btn_move", {label = "Move to Sidebar", cell = "main", onclick = function()
        -- Demonstrate set_property("cell") moving control between cells
        k.ctrl.set_property("grid_demo", "data", "cell", "sidebar")
        k.ctrl.set_property("grid_demo", "data", "align", "right")
    end})

    -- Footer cell
    k.ctrl.label("grid_demo", "footer_note", {text = "Footer: grid demo - resize window to see mobile collapse (<600px)", cell = "footer", multiline = true})

    -- Button to switch to vertical demo
    k.ctrl.button("grid_demo", "btn_show_vertical", {label = "Show Vertical Demo", cell = "footer", onclick = function()
        k.form.close("grid_demo")
        k.form.show("vertical_demo")
    end})

    -- Vertical form with center alignment (shown second)
    k.form.new("vertical_demo", {
        title = "Vertical Form (align=center)",
        layout = "vertical",
        align = "center",
        gap = 12
    })
    k.ctrl.textbox("vertical_demo", "v1", {label = "Field 1", value = "centered"})
    k.ctrl.textbox("vertical_demo", "v2", {label = "Field 2", value = "also centered"})
    k.ctrl.button("vertical_demo", "v_show_grid", {label = "Back to Grid", onclick = function()
        k.form.close("vertical_demo")
        k.form.show("grid_demo")
    end})

    -- Show grid first
    k.form.show("grid_demo")
end