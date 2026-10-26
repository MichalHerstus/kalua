package bindings

import (
	"sync"
	"testing"

	"kalua/internal/vm"
)

// fakeMQTTHub is an in-memory MQTTHub for bindings-level tests.
type fakeMQTTHub struct {
	mu      sync.Mutex
	conns   map[string]bool // handle -> connected
	subs    map[string][]string
	pubs    []string // "topic|payload"
	stats   map[string]MQTTStats
	connect func(MQTTConfig) (string, error)
}

func newFakeMQTTHub() *fakeMQTTHub {
	return &fakeMQTTHub{
		conns: make(map[string]bool),
		subs:  make(map[string][]string),
		stats: make(map[string]MQTTStats),
	}
}

func (h *fakeMQTTHub) Connect(cfg MQTTConfig) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.connect != nil {
		handle, err := h.connect(cfg)
		if err != nil {
			return "", err
		}
		h.conns[handle] = true
		return handle, nil
	}
	handle := "fake-handle"
	h.conns[handle] = true
	return handle, nil
}

func (h *fakeMQTTHub) Subscribe(handle, topic string, qos byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[handle] = append(h.subs[handle], topic)
	return nil
}

func (h *fakeMQTTHub) Publish(handle, topic, payload string, qos byte, retain bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pubs = append(h.pubs, handle+"|"+topic+"|"+payload)
	return nil
}

func (h *fakeMQTTHub) Unsubscribe(handle, topic string) error { return nil }
func (h *fakeMQTTHub) Close(handle string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, handle)
	return nil
}

func (h *fakeMQTTHub) Stats(handle string) (MQTTStats, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st, ok := h.stats[handle]; ok {
		return st, true
	}
	if _, ok := h.conns[handle]; ok {
		return MQTTStats{Connected: true, Subscriptions: 1}, true
	}
	return MQTTStats{}, false
}

// runServeMqttLua runs a Lua chunk in a serve-mode VM wired to a fake hub.
func runServeMqttLua(t *testing.T, hub MQTTHub, allowNet bool, src string) error {
	t.Helper()
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	SetupServe(L, app, Options{MQTT: hub, AllowNet: allowNet}, nil, nil, nil, nil)
	return L.DoString(src)
}

func TestMQTT_BindingsWireCalls(t *testing.T) {
	hub := newFakeMQTTHub()
	src := `
local function check(cond, msg)
  if not cond then error("FAIL: " .. msg) end
end
local h = k.mqtt_connect{broker="broker.local", port=1883}
check(h ~= nil, "connect returned handle")
k.mqtt_subscribe(h, "sensors/#", 1)
k.mqtt_publish(h, "sensors/1/temp", "21.5", 1, true)
local st = k.mqtt_stats(h)
check(st.connected == true, "connected")
check(st.subscriptions == 1, "subscriptions")
k.mqtt_close(h)
`
	if err := runServeMqttLua(t, hub, true, src); err != nil {
		t.Fatalf("mqtt wire calls failed: %v", err)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.subs["fake-handle"]) != 1 || hub.subs["fake-handle"][0] != "sensors/#" {
		t.Errorf("subs = %v", hub.subs)
	}
	if len(hub.pubs) != 1 || hub.pubs[0] != "fake-handle|sensors/1/temp|21.5" {
		t.Errorf("pubs = %v", hub.pubs)
	}
	if _, ok := hub.conns["fake-handle"]; ok {
		t.Error("close did not disconnect")
	}
}

func TestMQTT_ConnectGatedByAllowNet(t *testing.T) {
	hub := newFakeMQTTHub()
	src := `
local h = k.mqtt_connect{broker="broker.local"}
assert(h == nil, "connect must be refused without --allow-net")
assert(ERRORCODE == -20, "expected permission error (K_ERROR_PERMISSION=-20), got " .. tostring(ERRORCODE))
`
	if err := runServeMqttLua(t, hub, false, src); err != nil {
		t.Fatalf("gated connect failed: %v", err)
	}
}

func TestMQTT_NoHubRaises(t *testing.T) {
	// No hub configured (Options.MQTT == nil): bindings exist but wire calls
	// raise a runtime error.
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	SetupServe(L, app, Options{AllowNet: true}, nil, nil, nil, nil)
	if err := L.DoString(`k.mqtt_connect{broker="b"}`); err == nil {
		t.Fatal("expected runtime error without MQTT hub")
	}
}

func TestMQTT_OnHandlerDispatch(t *testing.T) {
	// k.mqtt_on registers handlers; DispatchMQTT invokes matching ones with
	// (topic, payload) in registration order.
	src := `
k.mqtt_on("sensors/+/temp", "on_temp")
k.mqtt_on("sensors/#", "on_any")
calls = {}
function on_temp(t, p) table.insert(calls, "temp:" .. t .. ":" .. p) end
function on_any(t, p) table.insert(calls, "any:" .. t) end
`
	L := vm.New()
	defer L.Close()
	app := vm.NewApp(L)
	e := SetupServe(L, app, Options{AllowNet: true}, nil, nil, nil, nil)
	if err := L.DoString(src); err != nil {
		t.Fatalf("setup: %v", err)
	}
	e.DispatchMQTT(L, MQTTMessage{Topic: "sensors/1/temp", Payload: "21"})
	// Both patterns match sensors/1/temp → two calls.
	_ = L.GetGlobal("calls")
	var n int
	L.GetField(L.GetGlobal("calls"), "n")
	// count via table length in Lua
	L.Push(L.GetGlobal("calls"))
	_ = n
	if err := L.DoString(`assert(#calls == 2, "expected 2 handler calls, got " .. #calls)`); err != nil {
		t.Fatalf("dispatch calls: %v", err)
	}
	if err := L.DoString(`assert(calls[1] == "temp:sensors/1/temp:21", "first handler")`); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := L.DoString(`assert(calls[2] == "any:sensors/1/temp", "second handler")`); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestMQTT_TopicMatchUnit(t *testing.T) {
	cases := []struct {
		filter, topic string
		want          bool
	}{
		{"sensors/#", "sensors/1/temp", true},
		{"sensors/#", "sensors", true}, // # matches zero remaining levels
		{"sensors/+/temp", "sensors/1/temp", true},
		{"sensors/+/temp", "sensors/a/b/temp", false},
		{"sensors/+/temp", "sensors/temp", false},
		{"sensors/+/temp", "sensors/1/hum", false},
		{"#", "anything/at/all", true},
		{"+/+/+", "a/b/c", true},
		{"a/b/c", "a/b/c", true},
		{"a/b/c", "a/b/d", false},
		{"a/#", "a", true},
	}
	for _, c := range cases {
		if got := mqttTopicMatch(c.filter, c.topic); got != c.want {
			t.Errorf("mqttTopicMatch(%q, %q) = %v, want %v", c.filter, c.topic, got, c.want)
		}
	}
}

func TestMQTT_PatternValidation(t *testing.T) {
	ok := []string{"a/b", "a/+/b", "a/#", "#", "+", "sensors/1/temp"}
	for _, p := range ok {
		if err := validateMQTTPattern(p); err != nil {
			t.Errorf("validateMQTTPattern(%q) error: %v", p, err)
		}
	}
	bad := []string{"", "a/#/b", "a/b#", "a+b"}
	for _, p := range bad {
		if err := validateMQTTPattern(p); err == nil {
			t.Errorf("validateMQTTPattern(%q) should error", p)
		}
	}
}
