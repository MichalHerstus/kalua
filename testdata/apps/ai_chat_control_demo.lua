-- AI Chat Control demo (kforms_enhancements.md §12.2)
--
-- k.ctrl.ai_chat embeds the same assistant as k.form.ai_agent (the modal) as an
-- ordinary control inside any form — here a grid cell next to the data it can
-- act on. It never blocks the script; interaction is event-driven, state lives
-- on the control, and the model can drive the form through tools that use the
-- suspend-aware runner (k.sql / k.db_select inside a tool work fine).
--
-- Requires an LLM endpoint: set KALUA_AI_BASE_URL / KALUA_AI_API_KEY /
-- KALUA_AI_MODEL (or [AI] in KALUA.INI), or tick "show_apikey" below (enabled)
-- and paste a key into the panel.
-- Run with: ./KALUA run testdata/apps/ai_chat_control_demo.lua

-- ---------------------------------------------------------------------------
-- Tools the model can call. The control's `tools` option allowlists them. They
-- run on the session actor with the suspend-aware runner, so they may call any
-- k.* API — including suspending ones like k.sql / k.db_select.
-- ---------------------------------------------------------------------------

-- DB handle is a global so the tool functions can reach it (tools are globals).
DEMO_DB = nil

function find_customer(id)
    -- Look up a customer by id from the SQLite database.
    local rows = k.sql(DEMO_DB, "SELECT id, name, city FROM customers WHERE id = ?", id)
    if #rows > 0 then return rows[1] end
    return { error = "no customer " .. tostring(id) }
end

function fill_customer(id)
    -- Load a customer into the form's detail fields (drives the form).
    local c = find_customer(id)
    if not c then return { error = "no customer " .. tostring(id) } end
    k.ctrl.set_value("main", "cust_name", c.name)
    k.ctrl.set_value("main", "cust_city", c.city)
    return c
end

function order_total(customer_id)
    -- Sum a customer's order values (suspend-aware k.sql inside a tool).
    local rows = k.sql(DEMO_DB, "SELECT SUM(total) AS total FROM orders WHERE customer_id = ?", customer_id)
    local t = 0
    if #rows > 0 and rows[1].total ~= nil then t = rows[1].total end
    return { customer_id = customer_id, total = t }
end

-- Custom tool: k.ai.tool(name, schema, fn) layers a tool on top of the
-- allowlist. The schema drives the model's parameter types/requirements.
k.ai.tool("customer_summary", {
    desc   = "Return a one-line summary of a customer: name, city and order total.",
    params = {
        { name = "customer_id", type = "number", required = true },
    },
}, function(customer_id)
    local c = find_customer(customer_id)
    if not c then return { error = "no customer " .. tostring(customer_id) } end
    local t = order_total(customer_id)
    return { summary = c.name .. " (" .. c.city .. ") — order total $" .. string.format("%.2f", t.total or 0) }
end)

-- ---------------------------------------------------------------------------
-- Form
-- ---------------------------------------------------------------------------

function main()
    -- Reset the demo DB file (best-effort) and open a fresh in-workdir SQLite
    -- so tools query the same data the table shows.
    if k.file_exists("ai_chat_control_demo.db") then k.file_delete("ai_chat_control_demo.db") end
    DEMO_DB = k.connect_sqlite("ai_chat_control_demo.db")
    k.sql(DEMO_DB, "CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT, city TEXT)")
    k.sql(DEMO_DB, "CREATE TABLE orders   (id INTEGER PRIMARY KEY, customer_id INTEGER, total REAL)")
    k.sql(DEMO_DB, "INSERT INTO customers VALUES (7,  'Ada Lovelace',  'London')")
    k.sql(DEMO_DB, "INSERT INTO customers VALUES (12, 'Grace Hopper',  'New York')")
    k.sql(DEMO_DB, "INSERT INTO customers VALUES (19, 'Alan Turing',   'Manchester')")
    k.sql(DEMO_DB, "INSERT INTO orders VALUES (101, 7, 49.99)")
    k.sql(DEMO_DB, "INSERT INTO orders VALUES (102, 7, 12.50)")
    k.sql(DEMO_DB, "INSERT INTO orders VALUES (103, 12, 7.25)")

    k.form.new("main", {
        title  = "AI Chat Control Demo (k.ctrl.ai_chat)",
        layout = "grid",
        gap    = 12,
        cells  = {
            { id = "data", width = 7 },
            { id = "chat", width = 5 },
        },
    })

    -- Left cell: the customer table the assistant can act on.
    k.ctrl.table("main", "customers", {
        cell    = "data",
        data    = {
            { id = 7,  name = "Ada Lovelace",  city = "London" },
            { id = 12, name = "Grace Hopper",  city = "New York" },
            { id = 19, name = "Alan Turing",   city = "Manchester" },
        },
        columns = {
            { field = "id",   title = "ID" },
            { field = "name", title = "Name" },
            { field = "city", title = "City" },
        },
    })

    -- Detail fields the assistant fills via the fill_customer tool.
    k.ctrl.textbox("main", "cust_name", { cell = "data", label = "Name" })
    k.ctrl.textbox("main", "cust_city", { cell = "data", label = "City" })

    -- Right cell: the assistant itself. `tools` allowlists which script globals
    -- are exposed; k.ai.tool("customer_summary", ...) adds a custom one.
    k.ctrl.ai_chat("main", "assistant", {
        cell        = "chat",
        title       = "Customer assistant",
        welcome     = "Ask me to *load* or *summarize* a customer, e.g. \"load customer 7\" or \"summary for customer 12\".",
        system      = "You are a customer support assistant. Use fill_customer to load a customer's details into the form, order_total to compute an order value, and customer_summary for a company one-liner. Always answer in prose after calling a tool.",
        tools       = { "find_customer", "fill_customer", "order_total", "customer_summary" },
        height      = "100%",
        placeholder = "load customer 7…",
        show_apikey = true, -- in-panel API-key input for easy testing
        show_header = true,
        markdown    = true, -- assistant bubbles rendered as Markdown
    })

    -- Status line fed by the AI chat events.
    k.ctrl.textbox("main", "status", { cell = "data", label = "Events", value = "(ready)", enabled = false })
    k.ctrl.textbox("main", "last_value", { cell = "data", label = "k.ctrl.get_value → last assistant text", value = "", enabled = false })
    k.ctrl.label("main", "hint", {
        cell      = "data",
        text      = "Try: \"load customer 7\", \"what did order 7 total?\", \"summary for customer 12\".\nThe assistant drives this form through its tools — watch Name/City and the log below.",
        multiline = true,
    })

    -- Buttons exercising the k.ai_chat.* operations.
    k.ctrl.button("main", "btn_add", { cell = "data", label = "add_message('user', …)", onclick = function()
        k.ai_chat.add_message("main", "assistant", "user", "Please list the available customers.")
        k.ctrl.set_value("main", "status", "appended a user message via k.ai_chat.add_message")
    end })

    k.ctrl.button("main", "btn_system", { cell = "data", label = "set_system (terse persona)", onclick = function()
        k.ai_chat.set_system("main", "assistant", "You are a terse assistant. One-sentence answers only.")
        k.ctrl.set_value("main", "status", "system prompt replaced via k.ai_chat.set_system")
    end })

    k.ctrl.button("main", "btn_transcript", { cell = "data", label = "get_transcript", onclick = function()
        local msgs = k.ai_chat.get_transcript("main", "assistant")
        k.ctrl.set_value("main", "status",
            "transcript has " .. tostring(#msgs) .. " message(s); busy=" .. tostring(k.ai_chat.is_busy("main", "assistant")))
    end })

    k.ctrl.button("main", "btn_stop", { cell = "data", label = "stop (cancel in-flight turn)", onclick = function()
        k.ai_chat.stop("main", "assistant")
        k.ctrl.set_value("main", "status", "stop requested via k.ai_chat.stop")
    end })

    k.ctrl.button("main", "btn_clear", { cell = "data", label = "clear transcript", onclick = function()
        k.ai_chat.clear("main", "assistant")
        k.ctrl.set_value("main", "status", "transcript cleared via k.ai_chat.clear")
    end })

    k.ctrl.button("main", "btn_style", { cell = "data", label = "set_property title/placeholder", onclick = function()
        k.ctrl.set_property("main", "assistant", "title", "Customer assistant ★")
        k.ctrl.set_property("main", "assistant", "placeholder", "try: summary for customer 19…")
        k.ctrl.set_property("main", "assistant", "bg", "#f7f9ff")
        k.ctrl.set_value("main", "status", "restyled via k.ctrl.set_property (title/placeholder/bg)")
    end })

    -- -----------------------------------------------------------------------
    -- Events
    -- -----------------------------------------------------------------------

    -- on_send runs synchronously and may return false to veto the turn.
    k.form.on("main", "assistant", "on_send", function(text)
        k.ctrl.set_value("main", "status", "on_send: " .. text)
        return true -- return false here to cancel the turn before anything is sent
    end)

    k.form.on("main", "assistant", "on_tool", function(name, args, result)
        k.print("ai tool " .. name .. " -> " .. k.json_string(result))
        k.ctrl.set_value("main", "status", "on_tool: " .. name .. " → " .. k.json_string(result))
    end)

    k.form.on("main", "assistant", "on_reply", function(text, messages)
        k.print("assistant: " .. text)
        -- k.ctrl.get_value returns the last assistant text (the control value).
        k.ctrl.set_value("main", "last_value", k.ctrl.get_value("main", "assistant"))
        k.ctrl.set_value("main", "status", "on_reply (" .. tostring(#messages) .. " messages)")
    end)

    k.form.on("main", "assistant", "on_error", function(message)
        k.print("ai error: " .. message)
        k.ctrl.set_value("main", "status", "on_error: " .. message .. " (check API key / endpoint)")
    end)

    k.form.on("main", "assistant", "on_done", function()
        k.print("turn done")
        k.ctrl.set_value("main", "status", "on_done — busy=" .. tostring(k.ai_chat.is_busy("main", "assistant")))
    end)

    k.form.show("main", { modal = false })
end
