//go:build !wasm

package bindings

import (
	"path/filepath"
	"testing"

	"kalua/internal/klog"
)

// TestSQLLoggerInterface ensures the SQLLogger adapter satisfies the host
// Logger surface and gates by level.
func TestSQLLoggerInterface(t *testing.T) {
	db, err := klog.Open(klog.Config{
		Path:  filepath.Join(t.TempDir(), "sqllogger.db"),
		Level: klog.LevelInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sq := NewSQLLogger(db)
	var _ interface {
		Printf(string, ...any)
		Errorf(string, ...any)
		Warnf(string, ...any)
		Tracef(string, ...any)
	} = sq

	sq.Printf("printed-info")
	sq.Errorf("printed-error")
	sq.Tracef("dropped-trace") // below MinLevel=info? no — trace is above (less severe)
	db.Flush()

	st := db.Stats()
	// info (2) and error (0) are ≤ info threshold; trace (4) > info → dropped.
	if st.Rows != 2 {
		t.Fatalf("rows = %d, want 2 (info+error kept, trace dropped)", st.Rows)
	}
}
