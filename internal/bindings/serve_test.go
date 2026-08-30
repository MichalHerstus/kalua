package bindings

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"kalua/internal/vm"
)

// fakeStore is an in-memory SharedStore for tests (avoids importing
// internal/server, which already depends on this package).
type fakeStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: make(map[string]string)}
}

func (s *fakeStore) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *fakeStore) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

func (s *fakeStore) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *fakeStore) Keys(pattern string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for k := range s.data {
		if pattern == "*" || pattern == "" || strings.HasPrefix(k, strings.TrimSuffix(pattern, "*")) {
			out = append(out, k)
		}
	}
	return out
}

func (s *fakeStore) Incr(key string, delta int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, err := strconv.ParseInt(s.data[key], 10, 64)
	if err != nil {
		val = 0
	}
	val += delta
	s.data[key] = strconv.FormatInt(val, 10)
	return val
}

// runServeLua executes a Lua chunk in a fresh serve-mode VM sharing the given
// store, returning any runtime error.
func runServeLua(t *testing.T, store SharedStore, src string) error {
	t.Helper()
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	SetupServe(L, app, Options{}, store, nil, nil, nil)
	return L.DoString(src)
}

func TestShared_JSONRoundTrip(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end

k.shared.set("str", "hello")
check(k.shared.get("str") == "hello", "string roundtrip")

k.shared.set("num", 42)
check(k.shared.get("num") == 42, "number roundtrip")

k.shared.set("bool", true)
check(k.shared.get("bool") == true, "bool roundtrip")

k.shared.set("obj", {a = 1, b = "x"})
local obj = k.shared.get("obj")
check(obj.a == 1 and obj.b == "x", "table roundtrip")

k.shared.set("arr", {10, 20, 30})
local arr = k.shared.get("arr")
check(arr[1] == 10 and arr[3] == 30, "array roundtrip")

k.shared.set("null", K.NULL)
check(K.is_null(k.shared.get("null")), "null roundtrip")

k.shared.set("nested", {inner = {{y = true}}})
local n = k.shared.get("nested")
check(n.inner[1].y == true, "nested roundtrip")

k.shared.incr("counter", 5)
k.shared.incr("counter", 2)
check(k.shared.get("counter") == 7, "incr returns number")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("shared JSON round-trip failed: %v", err)
	}
}

func TestShared_LegacyRawString(t *testing.T) {
	store := newFakeStore()
	store.Set("legacy", "plain text")
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
check(k.shared.get("legacy") == "plain text", "legacy raw string preserved")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("legacy raw string failed: %v", err)
	}
}

func TestShared_KeysAndDel(t *testing.T) {
	store := newFakeStore()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
k.shared.set("foo:1", 1)
k.shared.set("foo:2", 2)
k.shared.set("bar:1", 3)
local keys = k.shared.keys("foo:*")
check(#keys == 2, "keys count")
k.shared.del("foo:1")
check(k.shared.get("foo:1") == "", "del leaves missing key empty")
`
	if err := runServeLua(t, store, src); err != nil {
		t.Fatalf("shared keys/del failed: %v", err)
	}
}