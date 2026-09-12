-- CRUD browser for the PostgreSQL database described in pgs.txt.
-- Layout: table sidebar (left, width 3), records table (top right, width 9),
-- edit form (bottom right, width 9).

local CFG_FILE   = "pgs.txt"
local MAX_FIELDS = 18

local db_handle    = nil
local tables       = {}   -- ordered list of public table names
local currentTable = nil
local currentCols  = {}   -- column names of the active table
local currentPk    = nil  -- PK column of the active table (first fallback)
local currentPkVal = nil  -- PK value of the selected row
local fields       = {}   -- fld1..fldN textbox control names

-- ARGS is a positional list of "K=V" strings; fold it into a map.
local args = {}
for i = 1, #ARGS do
    local k, v = ARGS[i]:match("^([^=]+)=(.*)$")
    if k then
        args[k] = v
    else
        args[ARGS[i]] = "1"
    end
end

local function urlencode(s)
    return (tostring(s):gsub("[^A-Za-z0-9_.~-]", function(c)
        return string.format("%%%02X", string.byte(c))
    end))
end

local function loadConfig(path)
    local text = k.file_load(path)
    if text == nil then
        local why = tostring(ERRORMSG)
        if why == "" then why = "file returned nil" end
        error("cannot read " .. path .. " (" .. why .. ")")
    end
    local ip, port, dbname, user, pass = nil, "5432", "postgres", "postgres", ""
    for line in text:gmatch("[^\r\n]+") do
        local trimmed = line:match("^%s*(.-)%s*$")
        if trimmed ~= "" then
            local key, val = trimmed:match("^([^:]+):%s*(.-)$")
            if key then
                key = key:lower()
                if key == "ip" or key == "host" or key == "server" then
                    ip = val
                elseif key == "port" then
                    port = val
                elseif key == "database" or key == "db" then
                    dbname = val
                elseif key == "login" or key == "user" or key == "username" then
                    user = val
                elseif key == "password" or key == "pass" then
                    pass = val
                end
            elseif trimmed:match("^postgres(ql)?://") then
                return trimmed
            end
        end
    end
    if ip == nil then
        error("no host found in " .. path)
    end
    return string.format("postgres://%s:%s@%s:%s/%s",
        urlencode(user), urlencode(pass), ip, port, dbname)
end

local function connect()
    db_handle = k.connect_db(loadConfig(CFG_FILE))
end

local function listTables()
    local res = k.db_select(db_handle, "pg_tables", { "tablename" },
        { schemaname = "public" }, "tablename")
    local out = {}
    if res and res.rows then
        for i = 1, #res.rows do
            out[#out + 1] = res.rows[i].tablename
        end
    end
    return out
end

local function reloadTable()
    local res = k.db_select(db_handle, currentTable, { "*" }, {}, currentPk or "")
    k.ctrl.set_property("main", "records", "data", (res and res.rows) or {})
    currentPkVal = nil
end

local function loadTable(tbl)
    currentTable       = tbl
    local res          = k.db_select(db_handle, tbl, { "*" }, {}, "")
    local cols, rows   = {}, {}
    if res then
        if res.columns then
            for i = 1, #res.columns do
                cols[i] = res.columns[i]
            end
        end
        if res.rows then
            rows = res.rows
        end
    end
    currentCols = cols

    currentPk = nil
    for i = 1, #cols do
        if cols[i] == "id" then
            currentPk = "id"
            break
        end
    end
    if currentPk == nil and #cols > 0 then
        currentPk = cols[1]
    end
    currentPkVal = nil

    local colDefs = {}
    for i = 1, #cols do
        colDefs[i] = { field = cols[i], title = cols[i] }
    end

    for i = 1, MAX_FIELDS do
        local col = cols[i]
        if col then
            k.ctrl.set_property("main", fields[i], "label", col)
            k.ctrl.set_property("main", fields[i], "visible", true)
        else
            k.ctrl.set_property("main", fields[i], "visible", false)
        end
    end

    k.ctrl.set_property("main", "records", "columns", colDefs)
    k.ctrl.set_property("main", "records", "data", rows)
end

local function clearFields()
    for i = 1, MAX_FIELDS do
        if currentCols[i] then
            k.ctrl.set_value("main", fields[i], "")
        end
    end
    currentPkVal = nil
end

local function fillFields(sel)
    local row = sel and sel.data and sel.data[1]
    if not row then
        return
    end
    currentPkVal = row[currentPk]
    for i = 1, MAX_FIELDS do
        local col = currentCols[i]
        if col then
            local v = row[col]
            if v == nil then
                v = ""
            elseif type(v) == "boolean" then
                v = tostring(v)
            else
                v = tostring(v)
            end
            k.ctrl.set_value("main", fields[i], v)
        end
    end
end

local function collectValues()
    local kv = {}
    for i = 1, MAX_FIELDS do
        local col = currentCols[i]
        if col then
            local v = k.ctrl.get_value("main", fields[i])
            if v == nil then
                kv[col] = ""
            else
                kv[col] = tostring(v)
            end
        end
    end
    return kv
end

local function saveRow()
    if #currentCols == 0 then
        return
    end
    local kv  = collectValues()
    local set = {}
    for _, col in ipairs(currentCols) do
        if col ~= currentPk then
            local v = kv[col]
            if v ~= "" then
                set[col] = v
            end
        end
    end

    local n = 0
    for _ in pairs(set) do
        n = n + 1
    end
    if n == 0 then
        k.msgbox{ title = "Save", message = "No editable values entered.", type = "warning" }
        return
    end

    local ok, err = pcall(function()
        if currentPkVal ~= nil and currentPkVal ~= "" then
            -- PK column is read-only for existing rows.
            k.db_update(db_handle, currentTable, set, { [currentPk] = currentPkVal })
        else
            local ins = {}
            for col, v in pairs(set) do
                ins[col] = v
            end
            local pkv = kv[currentPk]
            if pkv ~= "" then
                ins[currentPk] = pkv
            end
            k.db_insert(db_handle, currentTable, ins)
        end
    end)
    if not ok then
        k.msgbox{ title = "Save failed", message = tostring(err), type = "danger" }
        return
    end
    k.msgbox{ title = "Save", message = "Row saved.", type = "info" }
    reloadTable()
end

local function deleteRow()
    if currentPkVal == nil or currentPkVal == "" then
        k.msgbox{ title = "Delete", message = "Select a row to delete first.", type = "warning" }
        return
    end
    local answer = k.msgbox{
        title   = "Delete row",
        type    = "danger",
        message = string.format("Delete %s row %s = %s?",
            currentTable, currentPk, tostring(currentPkVal)),
        buttons = {
            { label = "Delete", value = "yes" },
            { label = "Cancel", value = "no" },
        },
    }
    if answer ~= "yes" then
        return
    end
    local ok, err = pcall(function()
        k.db_delete(db_handle, currentTable, { [currentPk] = currentPkVal })
    end)
    if not ok then
        k.msgbox{ title = "Delete failed", message = tostring(err), type = "danger" }
        return
    end
    clearFields()
    reloadTable()
end

local function buildForm()
    local items = {}
    for i = 1, #tables do
        items[tostring(i)] = tables[i]
    end

    k.form.new("main", {
        title  = "KALUA · PostgreSQL CRUD",
        layout = "grid",
        gap    = 6,
        cells  = {
            { id = "sidebar", width = 3, bg = "#f2f4f7", border = { width = 1, color = "#d0d7de" } },
            { id = "records", width = 9 },
            { id = "edit",    width = 9 },
        },
    })

    k.ctrl.list("main", "tables", { label = "Tables", cell = "sidebar", items = items })
    k.ctrl.table("main", "records", {
        label            = "Rows",
        cell             = "records",
        tabulatorOptions = { layout = "fitColumns", selectable = 1 },
    })

    for i = 1, MAX_FIELDS do
        fields[i] = "fld" .. i
        k.ctrl.textbox("main", fields[i], { cell = "edit", visible = false })
    end

    k.ctrl.button("main", "btn_new",    { label = "New", cell = "edit" })
    k.ctrl.button("main", "btn_save",   { label = "Save", cell = "edit" })
    k.ctrl.button("main", "btn_delete", { label = "Delete", cell = "edit", class = "kalua-button kalua-button-danger" })

    k.form.on("main", "tables", "selection_change", function(idx)
        local tbl = tables[tonumber(idx)]
        if tbl then
            loadTable(tbl)
        end
    end)
    k.form.on("main", "records", "tabulator_selection_change", fillFields)
    k.form.on("main", "btn_new",    "click", clearFields)
    k.form.on("main", "btn_save",   "click", saveRow)
    k.form.on("main", "btn_delete", "click", deleteRow)
    k.form.on("main", "close_form", function()
        if db_handle then
            k.disconnect_db(db_handle)
        end
    end)
end

local function smoke()
    connect()
    tables = listTables()
    k.print("smoke: found " .. #tables .. " public tables")
    for i = 1, #tables do
        local res = k.db_select(db_handle, tables[i], { "*" }, {}, "")
        k.print(string.format("smoke: [%s] columns=%d rows=%d",
            tables[i], res and #res.columns or 0, res and #res.rows or 0))
    end
    if #tables > 0 then
        local res = k.db_select(db_handle, tables[1], { "*" }, {}, "id")
        k.print("smoke: ordered-by-id rows=" .. tostring(res and #res.rows or 0))
    end
    k.disconnect_db(db_handle)
    db_handle = nil
end

function main()
    if args.smoke == "1" then
        smoke()
        return
    end
    connect()
    tables = listTables()
    if #tables == 0 then
        error("no public tables found in database")
    end
    buildForm()
    loadTable(tables[1])
    k.form.show("main")
end