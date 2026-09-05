-- looper_demo.lua
-- Demonstrates the DB-linked k.ctrl.looper (kforms_enhancements.md §L).
-- Run: ./KALUA run testdata/apps/looper_demo.lua --allow-fs .

local DB_FILE = ".kalua_loopdemo.db"

-- Seed a sqlite DB with a customer list for the linked looper.
local function seed_db()
  local db = k.connect_sqlite(DB_FILE)
  k.sql(db, "DROP TABLE IF EXISTS customers")
  k.sql(db, "CREATE TABLE customers (id INTEGER, name TEXT, city TEXT, balance REAL)")
  local cities = { "Vienna", "Prague", "Berlin", "Budapest" }
  for i = 1, 250 do
    k.sql(db, "INSERT INTO customers (id, name, city, balance) VALUES (?, ?, ?, ?)",
      i, "Customer " .. i, cities[(i % #cities) + 1], math.floor(i * 1.37 * 100) / 100)
  end
  return db
end

function main()
  local db = seed_db()

  k.form.new("main", {title = "Looper Demo"})

  k.ctrl.looper("main", "customers", {
    db = db,
    query = "SELECT id, name, city, balance FROM customers",
    page_size = 25,
    links = {
      { column = 1, field = "id",      control = "lb_id"     },
      { column = 2, field = "name",    control = "lb_name"   },
      { column = 3, field = "city",    control = "lb_city"   },
      { column = 4, field = "balance", control = "lb_balance", property = "value" },
    },
  })

  local function reload()
    k.looper.refresh("main", "customers")
  end

  -- Row cursor: clicking a row highlights it here and fires onselect/onclick.
  k.form.on("main", "customers", "onselect", function(line_idx)
    print("looper selected row " .. tostring(line_idx))
  end)
  k.form.on("main", "customers", "onclick", function(ctrl_name, line_idx)
    if ctrl_name ~= "" then
      print("looper clicked " .. ctrl_name .. " in row " .. tostring(line_idx))
    end
  end)

  k.ctrl.button("main", "reload", { label = "Reload", onclick = function() reload() end })
  k.form.show("main")
end