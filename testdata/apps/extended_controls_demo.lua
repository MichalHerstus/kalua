-- Extended form controls demo (kforms_enhancements.md §4).
-- Textbox (multiline + datetime), multiline label, image control.
-- Run with: ./KALUA run testdata/apps/extended_controls_demo.lua

function main()
    k.form.new("controls", {title = "Extended Form Controls"})

    -- §4.1 Textbox: multiline (textarea)
    k.ctrl.textbox("controls", "notes", {
        label = "Notes (textarea)",
        multiline = true,
        rows = 5,
        cols = 40,
        value = "First line\nSecond line\n"
    })

    -- §4.1 Textbox: date, time and datetime pickers (flatpickr)
    k.ctrl.textbox("controls", "dt_date", {
        label = "Date",
        value = "2024-01-15",
        datetime = {mode = "date"}
    })
    k.ctrl.textbox("controls", "dt_time", {
        label = "Time",
        value = "09:30",
        datetime = {mode = "time", step = 15}
    })
    k.ctrl.textbox("controls", "dt_dt", {
        label = "Date + Time",
        value = "2024-01-15 09:30",
        datetime = {mode = "datetime", min = "2024-01-01 00:00", max = "2024-12-31 23:59"}
    })
    k.ctrl.textbox("controls", "dt_custom", {
        label = "European date order",
        value = "15/01/2024",
        datetime = {mode = "date", format = "DD/MM/YYYY"}
    })

    -- §4.2 Label: multiline (pre-wrap)
    k.ctrl.label("controls", "multi_label", {
        text = "Multiline label:\nLine two <kept raw>\nLine three",
        multiline = true
    })
    k.ctrl.label("controls", "plain_label", {text = "Plain single line"})

    -- §4.3 Image control
    k.ctrl.image("controls", "logo", {
        src = "https://picsum.photos/seed/kalua/640/200",
        alt = "Sample image",
        width = 320,
        height = 100,
        fit = "cover",
        clickable = true,
        onclick = function(val)
            k.status_show("Clicked image (src length " .. #val .. ")")
        end
    })

    -- Swap the image source and log picker values.
    k.ctrl.button("controls", "btn_swap", {
        label = "Swap image",
        onclick = function()
            k.ctrl.set_value("controls", "logo", "https://picsum.photos/seed/kalua2/640/200")
        end
    })
    k.ctrl.button("controls", "btn_report", {
        label = "Report values",
        onclick = function()
            k.print("notes=", k.ctrl.get_value("controls", "notes"))
            k.print("date=", k.ctrl.get_value("controls", "dt_date"))
            k.print("time=", k.ctrl.get_value("controls", "dt_time"))
            k.print("datetime=", k.ctrl.get_value("controls", "dt_dt"))
            k.print("custom=", k.ctrl.get_value("controls", "dt_custom"))
        end
    })

    -- Demonstrate dynamic src via set_property.
    k.ctrl.button("controls", "btn_alt", {
        label = "Set image alt",
        onclick = function()
            k.ctrl.set_property("controls", "logo", "alt", "Swapped sample")
        end
    })

    k.form.show("controls")
end