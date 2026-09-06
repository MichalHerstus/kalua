-- Demo: k.popup — multilevel menu-style popup (kforms).
-- Run with: ./KALUA run testdata/apps/popup_demo.lua
--
-- k.popup shows a centered menu; branches ({label=, items={...}}) open a
-- fly-out submenu on hover/focus, leaves ({label, value}) return the picked
-- value with its type intact. Esc / click outside dismisses → returns nil.

function main()
    k.form.new("popup_demo", {title = "Popup demo"})

    k.form.on("popup_demo", "open_menu", "click", function()
        local pick = k.popup{
            title = "Maintenance",
            items = {
                { label = "File", items = {
                    { label = "Open",   value = "open" },
                    { "Save As", "save_as" },   -- positional pair
                    { label = "Recent", items = {
                        { label = "report.lua", value = "recent_report" },
                        { label = "data.lua",   value = "recent_data" },
                    }},
                }},
                { label = "Refresh", value = 1 },
                { label = "Show date", value = { format = "short", tz = "local" } },
                { "Quit" },
            },
        }
        if pick == nil then
            k.msgbox{title = "Popup", message = "Dismissed (no pick).", type = "info"}
            return
        end
        local shown = pick
        if type(pick) == "table" then
            shown = pick.format .. " / " .. pick.tz
        end
        k.msgbox{title = "You picked", message = tostring(shown) .. " (type " .. type(pick) .. ")", type = "info"}
    end)

    k.ctrl.button("popup_demo", "open_menu", {label = "Open menu (k.popup)"})
    k.form.show("popup_demo")
end