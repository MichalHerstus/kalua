package bindings

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	lua "github.com/yuin/gopher-lua"
)

func TestGridTableFromQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "simple select",
			query:    "SELECT id, name FROM items",
			expected: "items",
		},
		{
			name:     "select with where",
			query:    "SELECT * FROM items WHERE id > 5",
			expected: "items",
		},
		{
			name:     "with cte",
			query:    "WITH cte AS (SELECT * FROM items) SELECT * FROM cte",
			expected: "items",
		},
		{
			name:     "select with join",
			query:    "SELECT i.id, i.name, o.qty FROM items i JOIN orders o ON i.id = o.item_id",
			expected: "items",
		},
		{
			name:     "uppercase",
			query:    "SELECT ID FROM USERS",
			expected: "USERS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			table, err := GridTableFromQuery(tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if table != tc.expected {
				t.Errorf("expected table %q, got %q", tc.expected, table)
			}
		})
	}
}

func TestGridTableFromQueryInvalid(t *testing.T) {
	tests := []string{
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET x=1",
		"DELETE FROM t",
		"EXPLAIN SELECT * FROM t",
	}

	for _, q := range tests {
		_, err := GridTableFromQuery(q)
		if err == nil {
			t.Errorf("expected error for query %q, got nil", q)
		}
	}
}

func TestGridPKFromControl(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctrl := L.CreateTable(0, 0)
	ctrl.RawSetString("pk_field", lua.LString("custom_id"))
	if pk := GridPKFromControl(ctrl); pk != "custom_id" {
		t.Errorf("expected custom_id, got %q", pk)
	}

	ctrl = L.CreateTable(0, 0)
	dbcols := L.CreateTable(0, 0)
	dbcols.Append(lua.LString("id"))
	dbcols.Append(lua.LString("name"))
	dbcols.Append(lua.LString("email"))
	ctrl.RawSetString("db_columns", dbcols)
	if pk := GridPKFromControl(ctrl); pk != "id" {
		t.Errorf("expected id from db_columns, got %q", pk)
	}

	ctrl = L.CreateTable(0, 0)
	dbcols = L.CreateTable(0, 0)
	dbcols.Append(lua.LString("user_id"))
	dbcols.Append(lua.LString("name"))
	ctrl.RawSetString("db_columns", dbcols)
	if pk := GridPKFromControl(ctrl); pk != "user_id" {
		t.Errorf("expected user_id from db_columns, got %q", pk)
	}

	ctrl = L.CreateTable(0, 0)
	dbcols = L.CreateTable(0, 0)
	dbcols.Append(lua.LString("name"))
	dbcols.Append(lua.LString("email"))
	ctrl.RawSetString("db_columns", dbcols)
	if pk := GridPKFromControl(ctrl); pk != "name" {
		t.Errorf("expected first column name, got %q", pk)
	}

	ctrl = L.CreateTable(0, 0)
	if pk := GridPKFromControl(ctrl); pk != "id" {
		t.Errorf("expected fallback id, got %q", pk)
	}
}

func TestGridWriteCRUD(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	dsn := "sqlite://" + dbPath

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test_grid (id INTEGER PRIMARY KEY, name TEXT, value INTEGER)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := RegisterNamedDB("test_grid_crud", dsn); err != nil {
		t.Fatalf("register named db: %v", err)
	}

	t.Run("GridInsert", func(t *testing.T) {
		vals := map[string]interface{}{"name": "foo", "value": 42}
		if err := GridInsert("test_grid_crud", "test_grid", "id", vals); err != nil {
			t.Fatalf("insert: %v", err)
		}
		var name string
		var val int
		err := db.QueryRow("SELECT name, value FROM test_grid WHERE name=?", "foo").Scan(&name, &val)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if name != "foo" || val != 42 {
			t.Errorf("unexpected row: %s %d", name, val)
		}
	})

	t.Run("GridUpdate", func(t *testing.T) {
		_, err := db.Exec("INSERT INTO test_grid (id, name, value) VALUES (100, 'old', 1)")
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		vals := map[string]interface{}{"name": "new", "value": 999}
		if err := GridUpdate("test_grid_crud", "test_grid", "id", 100, vals); err != nil {
			t.Fatalf("update: %v", err)
		}
		var name string
		var val int
		err = db.QueryRow("SELECT name, value FROM test_grid WHERE id=?", 100).Scan(&name, &val)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		if name != "new" || val != 999 {
			t.Errorf("unexpected row after update: %s %d", name, val)
		}
	})

	t.Run("GridDeleteMany", func(t *testing.T) {
		_, err := db.Exec("INSERT INTO test_grid (id, name, value) VALUES (200, 'del1', 1), (201, 'del2', 2)")
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := GridDeleteMany("test_grid_crud", "test_grid", "id", []interface{}{200, 201}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM test_grid WHERE id IN (200, 201)").Scan(&count)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 rows, got %d", count)
		}
	})

	CloseNamedDBs()
}

func TestBuildGridFormAuto(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctrl := L.CreateTable(0, 0)
	cols := L.CreateTable(0, 0)
	cols.Append(lua.LString("id"))
	cols.Append(lua.LString("name"))
	cols.Append(lua.LString("qty"))
	ctrl.RawSetString("db_columns", cols)

	modalName, html, err := BuildGridForm(L, "test", "grid1", ctrl)
	if err != nil {
		t.Fatalf("BuildGridForm error: %v", err)
	}
	t.Logf("HTML: %s", html)
	if html == "" {
		t.Error("expected non-empty HTML")
	}
	if !contains(html, "data-k-form=\"__kgrid_test_grid1\"") {
		t.Error("expected form data-k-form")
	}
	if !contains(html, "data-k-ctrl=\"name\"") {
		t.Error("expected control for name column")
	}
	if !contains(html, "data-k-ctrl=\"qty\"") {
		t.Error("expected control for qty column")
	}
	_ = modalName
}

func TestBuildGridFormInline(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctrl := L.CreateTable(0, 0)
	formTbl := L.CreateTable(0, 0)
	formTbl.RawSetString("title", lua.LString("Edit Record"))
	formTbl.RawSetString("gap", lua.LNumber(12))
	controls := L.CreateTable(0, 0)
	idCtrl := L.CreateTable(0, 0)
	idCtrl.RawSetString("type", lua.LString("textbox"))
	idCtrl.RawSetString("name", lua.LString("id"))
	idCtrl.RawSetString("label", lua.LString("ID"))
	idCtrl.RawSetString("readonly", lua.LTrue)
	nameCtrl := L.CreateTable(0, 0)
	nameCtrl.RawSetString("type", lua.LString("textbox"))
	nameCtrl.RawSetString("name", lua.LString("name"))
	nameCtrl.RawSetString("label", lua.LString("Name"))
	controls.Append(idCtrl)
	controls.Append(nameCtrl)
	formTbl.RawSetString("controls", controls)
	ctrl.RawSetString("form", formTbl)
	t.Logf("inline form controls before: %v", ctrl.RawGetString("form").(*lua.LTable).RawGetString("controls"))

	modalName, html, err := BuildGridForm(L, "test", "grid1", ctrl)
	if err != nil {
		t.Fatalf("BuildGridForm error: %v", err)
	}
	// Debug: inspect the form global
	formGlobal := L.GetGlobal(modalName)
	t.Logf("formGlobal type: %T, value: %v", formGlobal, formGlobal)
	if ftbl, ok := formGlobal.(*lua.LTable); ok {
		controls := ftbl.RawGetString("controls")
		order := ftbl.RawGetString("order")
		t.Logf("controls: %v, order: %v", controls, order)
		if ct, ok := controls.(*lua.LTable); ok {
			ct.ForEach(func(k, v lua.LValue) {
				t.Logf("  control %v: %v", k, v)
			})
		}
		if ot, ok := order.(*lua.LTable); ok {
			ot.ForEach(func(k, v lua.LValue) {
				t.Logf("  order %v: %v", k, v)
			})
		}
	}
	t.Logf("HTML: %s", html)
	if !contains(html, "Edit Record") {
		t.Error("expected inline form title")
	}
	if !contains(html, "data-k-ctrl=\"id\"") {
		t.Error("expected id control")
	}
	if !contains(html, "data-k-ctrl=\"name\"") {
		t.Error("expected name control")
	}
	_ = modalName
}

func TestGridFormGap(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	ctrl := L.CreateTable(0, 0)
	x, y := GridFormGap(ctrl)
	if x != 10 || y != 10 {
		t.Errorf("expected default gap 10,10 got %f,%f", x, y)
	}

	ctrl = L.CreateTable(0, 0)
	ctrl.RawSetString("form_gap", lua.LNumber(24))
	x, y = GridFormGap(ctrl)
	if x != 24 || y != 24 {
		t.Errorf("expected gap 24,24 got %f,%f", x, y)
	}

	ctrl = L.CreateTable(0, 0)
	gapTbl := L.CreateTable(0, 0)
	gapTbl.RawSetString("x", lua.LNumber(8))
	gapTbl.RawSetString("y", lua.LNumber(12))
	ctrl.RawSetString("form_gap", gapTbl)
	x, y = GridFormGap(ctrl)
	if x != 8 || y != 12 {
		t.Errorf("expected gap 8,12 got %f,%f", x, y)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}