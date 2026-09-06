-- Demo: Kalipso-style k.msgbox variants (kforms: color strip + custom buttons).
-- Run with: ./KALUA run testdata/apps/msgbox_demo.lua
--
-- k.msgbox has two forms:
--   legacy  k.msgbox(text[, kind])            kind: info/warn/error/ok-cancel/yes-no
--   rich    k.msgbox{title=, message=, type=, buttons={{label, value}, ...}}
-- type sets the left color strip (info/warning/danger); buttons carry return
-- values whose Lua type is preserved (number/boolean/string/table). When
-- buttons are omitted a single "OK" button returning "ok" is added.

function main()
    k.form.new("demo", {title = "MessageBox demo"})

    -- 1. Info + default OK button. Type "info" = blue strip.
    k.form.on("demo", "info_ok", "click", function()
        local v = k.msgbox{title = "Information", message = "Record saved successfully."}
        k.msgbox{title = "Result", message = "Default OK returned: " .. tostring(v) .. " (" .. type(v) .. ")", type = "info"}
    end)

    -- 2. Warning + numeric button values. Type "warning" = amber strip.
    k.form.on("demo", "warning_num", "click", function()
        local v = k.msgbox{title = "Apply changes", message = "Apply settings to all rows?", type = "warning",
            buttons = {{"Apply all", 1}, {"This row", 2}, {"Cancel", 0}}}
        k.msgbox{title = "Result", message = "You chose: " .. tostring(v) .. " (" .. type(v) .. ")", type = "info"}
    end)

    -- 3. Danger + boolean button values. Type "danger" = red strip.
    k.form.on("demo", "danger_bool", "click", function()
        local v = k.msgbox{title = "Delete record", message = "Delete ACME order #42? This cannot be undone.", type = "danger",
            buttons = {{"Yes, delete", true}, {"No, keep", false}}}
        k.msgbox{title = "Result", message = "Delete confirmed: " .. tostring(v), type = "info"}
    end)

    -- 4. Warning + string button values, mixed button formats
    --    ({label=,value=}, positional pairs, bare strings all work).
    k.form.on("demo", "warn_str", "click", function()
        local v = k.msgbox{title = "Export report", message = "Which format do you want?", type = "warning",
            buttons = {{label = "PDF", value = "pdf"}, {"Excel", "xlsx"}, {"HTML"}}}
        k.msgbox{title = "Result", message = "Exporting as: " .. tostring(v), type = "info"}
    end)

    -- 5. Info + result-set (table) button value.
    k.form.on("demo", "info_table", "click", function()
        local v = k.msgbox{title = "Options", message = "Pick a sorting option?", type = "info",
            buttons = {{"By name", {field = "name", asc = true}}, {"By date", {field = "date", asc = false}}}}
        k.msgbox{title = "Result", message = "Sorting by " .. tostring(v.field) .. ", ascending=" .. tostring(v.asc), type = "info"}
    end)

    -- 6. Legacy forms still work: k.msgbox(text[, kind]).
    k.form.on("demo", "legacy_yn", "click", function()
        local v = k.msgbox("Are you sure you want to quit?", "yes-no")
        k.msgbox{title = "Legacy", message = "You answered: " .. tostring(v), type = "info"}
    end)
    k.form.on("demo", "legacy_oc", "click", function()
        local v = k.msgbox("Save changes to disk?", "ok-cancel")
        k.msgbox{title = "Legacy", message = "You answered: " .. tostring(v), type = "info"}
    end)

    k.ctrl.button("demo", "info_ok", {label = "Info + OK"})
    k.ctrl.button("demo", "warning_num", {label = "Warning + numeric buttons"})
    k.ctrl.button("demo", "danger_bool", {label = "Danger + boolean buttons"})
    k.ctrl.button("demo", "warn_str", {label = "Warning + string buttons"})
    k.ctrl.button("demo", "info_table", {label = "Info + table value"})
    k.ctrl.button("demo", "legacy_yn", {label = "Legacy yes-no"})
    k.ctrl.button("demo", "legacy_oc", {label = "Legacy ok-cancel"})

    k.form.show("demo")
end