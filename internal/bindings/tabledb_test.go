package bindings

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/yuin/gopher-lua"

	_ "modernc.org/sqlite"
)

// newSQLiteHandle creates a fresh sqlite DBHandle registers it under the "x"
// handle id (the same map FetchTablePage resolves), and seeds rows.
func newSQLiteHandle(t *testing.T, rows int) *DBHandle {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE items (id INTEGER, name TEXT, qty INTEGER, active BOOLEAN)"); err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 1; i <= rows; i++ {
		if _, err := db.Exec("INSERT INTO items (id, name, qty, active) VALUES (?, ?, ?, ?)",
			i, "item"+itoa(i), i*10%97, i%2 == 0); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	h := &DBHandle{db: db, driver: "sqlite"}
	dbHandlesMu.Lock()
	dbHandles["x"] = h
	dbHandlesMu.Unlock()
	t.Cleanup(func() {
		dbHandlesMu.Lock()
		delete(dbHandles, "x")
		dbHandlesMu.Unlock()
		db.Close()
	})
	return h
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestFetchTablePagePaging(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 30)

	link := &TableLink{
		HandleID: "x",
		Query:    "SELECT id, name, qty, active FROM items",
		PageSize: 10,
	}

	// Page 2 of 30 rows @ 10/page → rows 11..20, last_page 3.
	res, err := FetchTablePage(L, link, TablePageReq{Page: 2, Size: 10})
	if err != nil {
		t.Fatalf("FetchTablePage: %v", err)
	}
	if res.LastPage != 3 {
		t.Errorf("last_page = %d, want 3", res.LastPage)
	}
	if len(res.Rows) != 10 {
		t.Fatalf("page 2 row count = %d, want 10", len(res.Rows))
	}
	if res.Rows[0]["id"] != int64(11) {
		t.Errorf("page 2 first id = %v, want 11", res.Rows[0]["id"])
	}
}

func TestFetchTablePageSortWhitelist(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 20)

	link := &TableLink{
		HandleID: "x",
		Query:    "SELECT id, name, qty FROM items",
		PageSize: 20,
		Columns:  []string{"id", "name", "qty", "active"}, // explicit whitelist
	}

	// Sort desc by id → first row id=20.
	res, err := FetchTablePage(L, link, TablePageReq{Sort: []SortSpec{{Field: "id", Dir: "DESC"}}})
	if err != nil {
		t.Fatalf("FetchTablePage: %v", err)
	}
	if res.Rows[0]["id"] != int64(20) {
		t.Errorf("desc first id = %v, want 20", res.Rows[0]["id"])
	}

	// Non-whitelisted field must be ignored (no SQL error, no injection).
	res2, err := FetchTablePage(L, link, TablePageReq{Sort: []SortSpec{{Field: "name); DROP TABLE items;--", Dir: "ASC"}}})
	if err != nil {
		t.Fatalf("FetchTablePage with injected sort: %v", err)
	}
	if len(res2.Rows) != 20 {
		t.Errorf("injected sort row count = %d, want 20 (untouched)", len(res2.Rows))
	}
}

func TestFetchTablePageFilterBoundParams(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 20)

	link := &TableLink{
		HandleID: "x",
		Query:    "SELECT id, name, qty FROM items",
		PageSize: 20,
		Columns:  []string{"id", "name", "qty", "active"},
	}

	// Filter qty = 10 → one row (id 1).
	res, err := FetchTablePage(L, link, TablePageReq{Filter: []FilterSpec{{Field: "qty", Op: "=", Value: "10"}}})
	if err != nil {
		t.Fatalf("FetchTablePage filter: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("qty=10 rows = %d, want 1", len(res.Rows))
	}

	// LIKE filter on name.
	res2, err := FetchTablePage(L, link, TablePageReq{Filter: []FilterSpec{{Field: "name", Op: "like", Value: "item1"}}})
	if err != nil {
		t.Fatalf("FetchTablePage like: %v", err)
	}
	// names item1, item10..item19 match "item1" → 11 rows.
	if len(res2.Rows) != 11 {
		t.Errorf("name LIKE item1 rows = %d, want 11", len(res2.Rows))
	}

	// Injected field name must be dropped.
	res3, err := FetchTablePage(L, link, TablePageReq{Filter: []FilterSpec{{Field: "id = 1 OR 1=1", Op: "=", Value: "x"}}})
	if err != nil {
		t.Fatalf("FetchTablePage injected filter: %v", err)
	}
	if len(res3.Rows) != 20 {
		t.Errorf("injected filter rows = %d, want 20 (untouched)", len(res3.Rows))
	}
}

func TestFetchTablePageBaseWhereAndCount(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	_ = newSQLiteHandle(t, 20)

	link := &TableLink{
		HandleID:   "x",
		Query:      "SELECT id, name, qty, active FROM items",
		PageSize:   10,
		Columns:    []string{"id", "name", "qty", "active"},
		Where:      map[string]interface{}{"active": 1},
		CountQuery: "SELECT COUNT(*) FROM items WHERE active = 1",
	}

	res, err := FetchTablePage(L, link, TablePageReq{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("FetchTablePage base-where: %v", err)
	}
	// 10 active rows (even ids), but there are only 2 pages of 10 with 10 rows → last_page=1.
	if res.LastPage != 1 {
		t.Errorf("last_page = %d, want 1", res.LastPage)
	}
	if len(res.Rows) != 10 {
		t.Errorf("active rows = %d, want 10", len(res.Rows))
	}
	// All returned rows must be active (even id).
	for _, r := range res.Rows {
		if r["id"].(int64)%2 != 0 {
			t.Errorf("got inactive row id=%v in active filter", r["id"])
		}
	}

	// Count derived (no count_query) must equal explicit count.
	link2 := &TableLink{
		HandleID: "x",
		Query:    "SELECT id, name, qty, active FROM items",
		PageSize: 10,
		Columns:  []string{"id", "name", "qty", "active"},
		Where:    map[string]interface{}{"active": 1},
	}
	res2, err := FetchTablePage(L, link2, TablePageReq{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("FetchTablePage derived count: %v", err)
	}
	if res2.LastPage != res.LastPage {
		t.Errorf("derived last_page = %d, explicit = %d", res2.LastPage, res.LastPage)
	}
}
