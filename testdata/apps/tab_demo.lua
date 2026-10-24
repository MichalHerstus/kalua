-- Tab Control demo (kforms_enhancements.md §10)
-- Organizes content into multiple tabs, showing only the active tab's panel
-- at a time. Click headers to switch; k.tab.set_tab switches programmatically.
-- Run with: ./KALUA run testdata/apps/tab_demo.lua

function main()
    k.form.new("main", { title = "Tab Demo (k.ctrl.tab)", layout = "vertical" })

    k.ctrl.tab("main", "tabs", {
        tabs = {
            { title = "Dashboard", icon = "📊" },
            { title = "Reports", icon = "📈" },
            { title = "Settings", icon = "⚙️" },
        },
        tab_position = "top",
        swipable = true,
    })

    k.ctrl.textbox("main", "status", { label = "Status", value = "Tab 1 active (get_tab=" .. tostr(k.tab.get_tab("main", "tabs")) .. ", count=" .. tostr(k.tab.get_tab_count("main", "tabs")) .. ")" })

    k.ctrl.button("main", "btn_goto_3", { label = "Go to Settings", onclick = function()
        k.tab.set_tab("main", "tabs", 3)
        k.ctrl.set_value("main", "status", "Programmatically switched to tab 3 (now on " .. tostr(k.tab.get_tab("main", "tabs")) .. ")")
    end })

    k.ctrl.button("main", "btn_add", { label = "Add Tab", onclick = function()
        local n = k.tab.get_tab_count("main", "tabs") + 1
        k.tab.add_tab("main", "tabs", { title = "Tab " .. tostr(n), icon = "➕" })
        k.ctrl.set_value("main", "status", "Added tab; count=" .. tostr(k.tab.get_tab_count("main", "tabs")))
    end })

    k.form.on("main", "tabs", "onchange", function(new_idx, old_idx)
        k.ctrl.set_value("main", "status", "Switched from tab " .. old_idx .. " to " .. new_idx)
    end)

    k.form.show("main")
end
