-- AI Chat Form demo (kforms_enhancements.md §12)
-- Every function the script defines becomes a tool the agent can call.
-- The script first prompts for an API key, then opens the AI chat.

--[[
The concern was valid — without a model specified, the LLM client would send an empty model field to the endpoint. The fix ensures a model is always set:
Chain of model resolution:
1. k.form.ai_agent{model = "..."} — explicit per-call override
2. KALUA_AI_MODEL env var or [AI] model in KALUA.INI — via e.aiConfig.Model fallback
3. "local-model" — hardcoded default (mirrors ai.EnvConfig())
Files changed:
- internal/bindings/aichat.go — defaults model to "local-model" and baseURL to "http://localhost:1234/v1" after fallbacks, stores on form table
- internal/session/aichat.go — defensive defaults for model and baseURL when reading from the form table
- internal/bindings/api_doc.go — documents api_key, base_url, model options
Complete config resolution for k.form.ai_agent{...}:
Option	Source priority
api_key	1. api_key opt  2. KALUA_AI_API_KEY / [AI] api-key  3. (error if empty)
base_url	1. base_url opt  2. KALUA_AI_BASE_URL / [AI] base-url  3. http://localhost:1234/v1
model	1. model opt  2. KALUA_AI_MODEL / [AI] model  3. local-model
The demo script (testdata/apps/ai_agent_demo.lua) demonstrates entering the API key interactively via a setup form, 
then passing it to k.form.ai_agent{api_key = ...} — model defaults to "local-model" for local LLM endpoints like LM Studio, Ollama, etc.
]]

function get_customer(id)
    -- Look up a customer by id
    local rows = k.db_select("select id, name, city from customers where id = ?", {id})
    if #rows > 0 then return rows[1] end
    return {error = "not found"}
end

function search_orders(email, since)
    -- Find a customer's orders since a date (YYYY-MM-DD)
    return k.db_select(
        "select o.id, o.total, o.created from orders o join customers c on c.id=o.customer_id " ..
        "where c.email = ? and o.created >= ? order by o.created desc limit 20",
        {email, since})
end

function money(n)
    -- Format a number as a currency amount
    return string.format("%.2f", n)
end

function main()
    local db = k.connect_sqlite("app.db")

    -- Seed sample data (idempotent for demo purposes)
    k.sql(db, "create table if not exists customers (id integer primary key, name text, city text)")
    k.sql(db, "create table if not exists orders (id integer primary key, customer_id integer, total real, created text)")
    k.sql(db, "insert or ignore into customers (id, name, city) values (1, 'Ada Lovelace', 'London')")
    k.sql(db, "insert or ignore into customers (id, name, city) values (2, 'Alan Turing', 'Manchester')")
    k.sql(db, "insert or ignore into orders (id, customer_id, total, created) values (101, 1, 49.99, '2024-01-15')")
    k.sql(db, "insert or ignore into orders (id, customer_id, total, created) values (102, 1, 12.50, '2024-02-20')")
    k.sql(db, "insert or ignore into orders (id, customer_id, total, created) values (103, 2, 7.25, '2024-03-10')")

    -- Step 1: Prompt for API key
    k.form.new("apikey_form", {title = "AI Chat Setup", align = "center"})
    k.ctrl.label("apikey_form", "prompt", {
        text = "Enter your API key to chat with the AI assistant:",
        align = "center",
    })
    k.ctrl.textbox("apikey_form", "api_key_input", {
        label = "API Key",
        value = "",
        align = "center",
    })
    k.ctrl.button("apikey_form", "start_btn", {
        label = "Start Chat",
        align = "center",
        onclick = function()
            k.form.close("apikey_form")
        end,
    })
    k.form.show("apikey_form", {modal = true})

    -- Read the API key after the setup form closes.
    local apiKey = k.ctrl.get_value("apikey_form", "api_key_input") or ""

    -- Step 2: Open the AI chat with the provided API key.
    local transcript = k.form.ai_agent{
        title     = "Sales assistant",
        welcome   = "Ask me about customers and orders.",
        system    = "You are a sales assistant. Always cite the customer id.",
        max_turns = 6,
        api_key   = apiKey,
    }

    -- Runs after the user closes the chat.
    for _, m in ipairs(transcript) do
        print(m.role .. ": " .. m.content)
    end
end
