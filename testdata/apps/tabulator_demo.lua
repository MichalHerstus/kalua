-- tabulator_demo.lua
-- Demonstrates the Tabulator-enhanced table control (kforms_enhancements.md §1).
-- Run: ./KALUA run testdata/apps/tabulator_demo.lua --allow-fs .

local ROWS = {
  {name = "Alice",   role = "Admin",   active = true,  score = 98},
  {name = "Bob",     role = "Editor",  active = false, score = 74},
  {name = "Carol",   role = "Editor",  active = true,  score = 86},
  {name = "Dave",    role = "Viewer",  active = true,  score = 55},
  {name = "Eve",     role = "Admin",   active = false, score = 91},
}

local DB_FILE = ".kalua_tabdemo.db"

-- Seed a sqlite DB with products for the DB-linked table.
local function seed_db()
  local db = k.connect_sqlite(DB_FILE)
  k.sql(db, "DROP TABLE IF EXISTS products")
  k.sql(db, "CREATE TABLE products (id INTEGER, name TEXT, price REAL, stock INTEGER)")
  for i = 1, 50 do
    k.sql(db, "INSERT INTO products (id, name, price, stock) VALUES (?, ?, ?, ?)",
      i, "Product " .. i, 1.5 + (i * 0.25), (i * 13) % 40)
  end
  return db
end

-- Big synthetic dataset for remote pagination (10 per page x 4 pages).
local function big_rows()
  local rows = {}
  for i = 1, 40 do
    rows[i] = {ord = i, name = "User" .. i, score = (i * 7) % 100}
  end
  return rows
end

function main()
  local db = seed_db()

  k.form.new("main", {title = "Tabulator Table Demo"})

  k.ctrl.table("main", "users", {
    label = "Users",
    tabulator = true,
    -- data is inferred into columns by type (number/boolean/string) client-side
    data = ROWS,
  })

  k.ctrl.button("main", "reload", {
    label = "Reload",
    onclick = function()
      k.table.set_data("main", "users", ROWS)
      k.status_show("Reloaded " .. #ROWS .. " rows")
    end,
  })

  k.form.on("main", "users", "tabulator_selection_change", function(sel)
    k.print("selection:", #sel.rows, "rows selected")
  end)

  -- Remote-paginated table: the browser asks for a page, the handler slices
  -- the dataset server-side and returns {data, last_page}.
  k.ctrl.table("main", "big", {
    label = "Remote paginated",
    tabulator = true,
    tabulatorOptions = {
      paginationMode = "remote",
      paginationSize = 10,
    },
  })

  local ALL = big_rows()
  local PAGE_SIZE = 10
  k.form.on("main", "big", "tabulator_ajax_request", function(req)
    local page = tonum(req.page) or 1
    local size = tonum(req.size) or PAGE_SIZE
    local start = (page - 1) * size + 1
    local stop = math.min(start + size - 1, #ALL)
    local slice = {}
    local n = 1
    for i = start, stop do
      slice[n] = ALL[i]
      n = n + 1
    end
    local last_page = math.ceil(#ALL / size)
    return {data = slice, last_page = last_page}
  end)

  -- DB-linked table: the Go pager runs the SELECT server-side; header
  -- sort/filter are whitelisted and translated to ORDER BY / WHERE.
  k.ctrl.table("main", "products", {
    label = "DB-linked (sqlite)",
    tabulator = true,
    db = db,
    query = "SELECT id, name, price, stock FROM products",
    page_size = 10,
  })

  k.ctrl.button("main", "refresh_products", {
    label = "Refresh DB table",
    onclick = function()
      k.table.refresh("main", "products")
      k.status_show("Refreshed DB-linked table")
    end,
  })

  -- Swap to a filtered source at runtime.
  k.ctrl.button("main", "filter_low", {
    label = "Low stock only",
    onclick = function()
      k.table.set_db_source("main", "products", {
        query = "SELECT id, name, price, stock FROM products WHERE stock < 10",
      })
      k.status_show("Filtered to low stock")
    end,
  })

  k.form.show("main")
end