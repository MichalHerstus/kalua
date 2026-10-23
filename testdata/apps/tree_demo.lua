-- Tree Control demo (kforms_enhancements.md §9)
-- DB-linked expandable tree: displays a field from a table as an adjacency
-- list (id_field/parent_field). Full load — the host fetches all rows in one
-- query and the client expands/collapses locally.
-- Run with: ./KALUA run testdata/apps/tree_demo.lua

function main()
    -- In-memory SQLite with an org-chart / menu-tree adjacency list.
    local db = k.connect_sqlite(":memory:")
    k.sql(db, "CREATE TABLE categories (id INTEGER PRIMARY KEY, parent_id INTEGER, name TEXT, note TEXT)")
    k.sql(db, "INSERT INTO categories VALUES (1, NULL, 'Electronics', 'Gadgets & devices')")
    k.sql(db, "INSERT INTO categories VALUES (2, 1, 'Laptops', 'Portable computers')")
    k.sql(db, "INSERT INTO categories VALUES (3, 1, 'Phones', 'Mobile phones')")
    k.sql(db, "INSERT INTO categories VALUES (4, 2, 'ThinkPad', 'Business line')")
    k.sql(db, "INSERT INTO categories VALUES (5, 2, 'MacBook', 'Consumer line')")
    k.sql(db, "INSERT INTO categories VALUES (6, 3, 'Pixel', 'Google phones')")
    k.sql(db, "INSERT INTO categories VALUES (7, NULL, 'Books', 'Paper & e-books')")
    k.sql(db, "INSERT INTO categories VALUES (8, 7, 'Fiction', 'Novels & stories')")

    k.form.new("main", { title = "Categories (k.ctrl.tree)", layout = "vertical" })

    k.ctrl.tree("main", "tree", {
        db         = db,
        table      = "categories",
        label_field = "name",
        order_by   = "name ASC",
        expand_all = true,
    })

    k.ctrl.textbox("main", "detail", { label = "Selected node", value = "(click a node)" })
    k.ctrl.label("main", "hint", { text = "Click ▸ to expand/collapse; click a label to select.", multiline = true })

    k.form.on("main", "tree", "on_click", function(node)
        k.print("clicked " .. node.name .. " (id=" .. tostr(node.id) .. ")")
    end)

    k.form.on("main", "tree", "on_select", function(node)
        k.ctrl.set_value("main", "detail", node.name .. " — " .. (node.note or ""))
    end)

    k.form.on("main", "tree", "on_expand", function(node_id, expanded)
        k.print("node " .. tostr(node_id) .. (expanded and " expanded" or " collapsed"))
    end)

    k.form.on("main", "tree", "on_loaded", function(node_count)
        k.print("tree loaded: " .. tostr(node_count) .. " nodes")
    end)

    k.form.on("main", "tree", "on_error", function(message)
        k.print("tree error: " .. message)
    end)

    k.form.show("main")
end
