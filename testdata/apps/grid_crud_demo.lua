function main()
    local db = k.connect_sqlite("sqlite://grid_demo.db")
    k.sql(db, "CREATE TABLE IF NOT EXISTS products (id INTEGER PRIMARY KEY, name TEXT, price REAL, category TEXT, active INTEGER)")

    -- Seed some data
    local count = k.db_select(db, "SELECT COUNT(*) as c FROM products")
    if count and count[1] and count[1].c == 0 then
        local items = {
            {name = "Laptop", price = 999.99, category = "Electronics", active = 1},
            {name = "Mouse", price = 29.99, category = "Electronics", active = 1},
            {name = "Keyboard", price = 79.99, category = "Electronics", active = 1},
            {name = "Monitor", price = 299.50, category = "Electronics", active = 1},
            {name = "Desk", price = 449.00, category = "Furniture", active = 1},
            {name = "Chair", price = 199.99, category = "Furniture", active = 1},
        }
        for _, item in ipairs(items) do
            k.sql(db, "INSERT INTO products (name, price, category, active) VALUES (?, ?, ?, ?)",
                item.name, item.price, item.category, item.active)
        end
    end

    local form = k.form.new("grid_demo", {
        title = "Product Grid CRUD Demo",
        layout = "vertical",
        align = "center",
        gap = 16,
        controls = {
            {type = "label", name = "title", label = "Product Inventory (CRUD Grid)", align = "center"},
            {type = "grid", name = "products", db = "db",
                query = "SELECT id, name, price, category, active FROM products",
                pk_field = "id",
                selection_mode = "multi",
                row_click_action = "edit",
                row_actions = {view = true, edit = true, delete = true},
                global_actions = {new_record = true, batch_delete = true},
                column_visibility = true,
                tabulator = true,
                page_size = 10,
            },
        },
    })

    k.form.show("grid_demo")
    k.form.on("grid_demo", "products", "grid_form_save", function(mode, pk, values)
        -- The grid handles save/delete via built-in CRUD, but we can add custom logic here
    end)
end