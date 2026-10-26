//go:build !wasm

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"kalua/internal/host"
)

// TestLogE2E verifies Phase 8 end-to-end through a real server:
//   - an HTTP request produces a klog row with correct status/duration_ms
//   - k.log.info from Lua lands with category 'app' and a JSON data object
//   - k.print goes to stdout and never appears as a klog row
func TestLogE2E(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)

	script := filepath.Join(dir, "app.lua")
	appSrc := `
function handle_http(req)
  k.print("stdout only", req.path)
  k.log.info("order placed", {order_id=42, total=99.5})
  k.log.warn("retry attempt", {attempt=3})
  return {status=200, body="/" .. req.path}
end
function main() end
`
	if err := os.WriteFile(script, []byte(appSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	logger := host.NewLoggerWriter(&stdout, &bytes.Buffer{}, false)
	cfg := Config{
		Host:        "127.0.0.1",
		Port:        0,
		Workers:     1,
		Mode:        "http",
		ScriptPath:  script,
		Verbose:     false,
		Logger:      logger,
		LogSQLite:   true,
		LogLevel:    "info",
		LogRetain:   time.Hour,
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()

	readyCtx, rc := context.WithTimeout(ctx, 8*time.Second)
	defer rc()
	select {
	case <-s.Ready():
	case err := <-runDone:
		t.Fatalf("server failed to boot: %v", err)
	case <-readyCtx.Done():
		t.Fatal("server did not become ready")
	}

	base := "http://" + s.HTTPAddr()
	resp, err := http.Get(base + "/ping")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Wait for the writer to flush (async batching).
	time.Sleep(400 * time.Millisecond)
	if s.klog == nil {
		t.Fatal("klog sink not opened")
	}
	s.klog.Flush()

	// Query rows directly.
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "klog.db"))
	if err != nil {
		t.Fatalf("open klog.db: %v", err)
	}
	defer sqlDB.Close()

	type row struct {
		category string
		message  string
		data     string
		worker   int
		remote   string
	}
	rows, err := sqlDB.Query("SELECT category, message, COALESCE(data,''), COALESCE(worker,0), COALESCE(remote_addr,'') FROM klog ORDER BY id")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.category, &r.message, &r.data, &r.worker, &r.remote); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) < 3 {
		t.Fatalf("expected >=3 rows (http + log.info + log.warn), got %d: %+v", len(got), got)
	}

	// Row 1: the HTTP request (category http, data has status/duration_ms).
	// (A startup lifecycle row may precede it.)
	var httpRow row
	foundHTTP := false
	for _, r := range got {
		if r.category == "http" {
			httpRow = r
			foundHTTP = true
			break
		}
	}
	if !foundHTTP {
		t.Fatalf("no http-category row: %+v", got)
	}
	if httpRow.remote == "" {
		t.Errorf("http row missing remote_addr: %+v", httpRow)
	}
	var hd struct {
		Status     int   `json:"status"`
		DurationMS int64 `json:"duration_ms"`
		Method     string
		Path       string
	}
	if err := json.Unmarshal([]byte(httpRow.data), &hd); err != nil {
		t.Fatalf("http data not JSON: %v (%q)", err, httpRow.data)
	}
	if hd.Status != 200 {
		t.Errorf("http status = %d, want 200", hd.Status)
	}
	if hd.DurationMS <= 0 {
		t.Errorf("duration_ms = %d, want > 0", hd.DurationMS)
	}
	if hd.Method != "GET" || hd.Path != "/ping" {
		t.Errorf("method/path = %s %s, want GET /ping", hd.Method, hd.Path)
	}

	// log.info from Lua → category app, JSON data with order_id.
	foundApp := false
	for _, r := range got {
		if r.category == "app" && r.message == "order placed" {
			foundApp = true
			var d struct {
				OrderID int     `json:"order_id"`
				Total   float64 `json:"total"`
			}
			if err := json.Unmarshal([]byte(r.data), &d); err != nil {
				t.Fatalf("app data not JSON: %v", err)
			}
			if d.OrderID != 42 || d.Total != 99.5 {
				t.Errorf("app data = %+v, want order_id 42 total 99.5", d)
			}
		}
	}
	if !foundApp {
		t.Error("log.info row not found")
	}

	// k.print must never produce a klog row (stdout only). Allowed categories:
	// http (request), app (k.log.*), lifecycle (server-startup marker).
	allowed := map[string]bool{"http": true, "app": true, "lifecycle": true}
	for _, r := range got {
		if !allowed[r.category] {
			t.Errorf("unexpected category %q (k.print leaked into klog?)", r.category)
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte("stdout only")) {
		t.Error("k.print output missing from stdout")
	}

	// Shutdown cleanly.
	cancel()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
		t.Fatal("server did not stop")
	}
}

// TestLogLevelGating verifies --log-level debug keeps info+warn but drops trace.
func TestLogLevelGating(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prev)

	script := filepath.Join(dir, "app.lua")
	appSrc := `
function handle_http(req)
  k.log.info("info-msg")
  k.log.warn("warn-msg")
  k.log.trace("trace-msg")
  return {status=200}
end
function main() end
`
	if err := os.WriteFile(script, []byte(appSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    1,
		Mode:       "http",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
		LogSQLite:  true,
		LogLevel:   "info",
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()
	select {
	case <-s.Ready():
	case <-time.After(8 * time.Second):
		t.Fatal("not ready")
	}
	http.Get("http://" + s.HTTPAddr())
	time.Sleep(300 * time.Millisecond)
	s.klog.Flush()

	sqlDB, _ := sql.Open("sqlite", filepath.Join(dir, "klog.db"))
	defer sqlDB.Close()
	var infoCnt, traceCnt int
	sqlDB.QueryRow(`SELECT count(*) FROM klog WHERE message='info-msg'`).Scan(&infoCnt)
	sqlDB.QueryRow(`SELECT count(*) FROM klog WHERE message='trace-msg'`).Scan(&traceCnt)
	if infoCnt != 1 {
		t.Errorf("info rows = %d, want 1", infoCnt)
	}
	if traceCnt != 0 {
		t.Errorf("trace rows = %d, want 0 (gated by --log-level info)", traceCnt)
	}
	cancel()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
		t.Fatal("did not stop")
	}
}
