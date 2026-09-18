-- Server-rendered looper row templates (Phase 4).
-- Run with: ./KALUA run testdata/apps/looper_rows_demo.lua
-- Rows are rendered by the host from opts.row control templates, so each row is
-- a real (read-only) label/textbox/checkbox control whose value comes from the
-- linked query column.

function main()
    local db = k.connect_sqlite(":memory:")
    k.sql(db, "CREATE TABLE products (id INTEGER, name TEXT, price REAL, in_stock INTEGER)")
    for i = 1, 40 do
        local in_stock = 1
        if i % 4 == 0 then in_stock = 0 end
        k.sql(db, "INSERT INTO products VALUES (?, ?, ?, ?)", i,
              "Product <" .. i .. ">", math.floor(i * 1.5 * 10) / 10, in_stock)
    end

    k.form.new("catalog", {title = "Product Catalog"})
    k.ctrl.looper("catalog", "products", {
        db = db,
        query = "SELECT id, name, price, in_stock FROM products",
        page_size = 8,
        columns = 4,
        row = {
            {type = "label",    name = "lb_id",   property = "text",  field = "id"},
            {type = "textbox",  name = "tx_name", property = "value", field = "name"},
            {type = "textbox",  name = "tx_price",property = "value", field = "price"},
            {type = "checkbox", name = "ck_stock",property = "value", field = "in_stock"},
        },
    })

    k.form.on("catalog", "products", "onselect", function(line_idx, ctrl_name)
        k.print("row selected:", line_idx, "cell:", ctrl_name)
    end)

    -- Re-run the query from the first page after edits elsewhere.
    k.ctrl.button("catalog", "btn_jump", {
        label = "Reload rows",
        onclick = function()
            k.looper.refresh("catalog", "products")
        end
    })

    k.form.show("catalog")
end