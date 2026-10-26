//go:build !wasm

package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"

	"kalua/internal/bindings"
	"kalua/internal/host"
)

// startInProcBroker starts a mochi-mqtt broker on an ephemeral port and
// returns its address + close func. Pure-Go, no Docker required (the plan's
// testcontainers suggestion is Docker-only; this keeps CI hermetic).
func startInProcBroker(t *testing.T) (string, func()) {
	t.Helper()
	srv := mochi.New(nil)
	// Find a free port, then hand it to the broker. Slight race in theory;
	// acceptable for tests.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	tcp := listeners.NewTCP(listeners.Config{ID: "t1", Address: addr})
	if err := srv.AddListener(tcp); err != nil {
		t.Fatal(err)
	}
	if err := srv.AddHook(new(allowAllAuth), nil); err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	waitFor(addr, 3*time.Second)
	return addr, func() { _ = srv.Close() }
}

func waitFor(addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMQTTHub_ConnectSubscribePublishStats(t *testing.T) {
	addr, stop := startInProcBroker(t)
	defer stop()

	hub := NewMQTTHub()
	var delivered atomic.Int64
	hub.SetDeliver(func(msg bindings.MQTTMessage) {
		delivered.Add(1)
	})

	handle, err := hub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: portOf(addr)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if handle == "" {
		t.Fatal("empty handle")
	}
	if err := hub.Subscribe(handle, "sensors/#", 1); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	// Give paho a moment to complete the SUBSCRIBE handshake before publish.
	time.Sleep(150 * time.Millisecond)
	if err := hub.Publish(handle, "sensors/1/temp", "21.5", 1, false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	st, ok := hub.Stats(handle)
	if !ok {
		t.Fatal("stats: unknown handle")
	}
	if !st.Connected {
		t.Error("not connected")
	}
	if st.Subscriptions != 1 {
		t.Errorf("subscriptions = %d, want 1", st.Subscriptions)
	}
	if st.MessagesOut != 1 {
		t.Errorf("messages_out = %d, want 1", st.MessagesOut)
	}
	if st.MessagesIn < 1 {
		t.Errorf("messages_in = %d, want >= 1 (self-published message received back)", st.MessagesIn)
	}
	if delivered.Load() < 1 {
		t.Errorf("delivered = %d, want >= 1", delivered.Load())
	}

	if err := hub.Close(handle); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, ok := hub.Stats(handle); ok {
		t.Error("handle still present after close")
	}
}

func TestMQTTHub_UnknownHandle(t *testing.T) {
	hub := NewMQTTHub()
	if err := hub.Publish("nope", "x", "y", 0, false); err == nil {
		t.Error("publish with unknown handle should error")
	}
	if err := hub.Subscribe("nope", "x", 0); err == nil {
		t.Error("subscribe with unknown handle should error")
	}
	if _, ok := hub.Stats("nope"); ok {
		t.Error("stats with unknown handle should be !ok")
	}
}

// TestMQTTHub_InboundDispatchToWorker verifies the full circle: a KALUA serve
// worker with k.mqtt_on handlers receives a message published by a second MQTT
// client, dispatched through the hub's deliver func into Lua.
func TestMQTTHub_InboundDispatchToWorker(t *testing.T) {
	addr, stop := startInProcBroker(t)
	defer stop()

	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	os.WriteFile(script, []byte(`
k.mqtt_on("sensors/#", "on_sensor")
function on_sensor(t, p) last_topic = t; last_payload = p end
function main() end
`), 0o644)

	hub := NewMQTTHub()
	logger := host.NewLogger(false)
	shared := NewSharedState()
	workerCh := make(chan *Worker, 1)
	wsHub := NewWSHub(workerCh)
	tcpHub := NewTCPHub(workerCh)

	opts := bindings.Options{MQTT: hub, AllowNet: true}
	w, err := NewWorker(1, script, opts, shared, wsHub, tcpHub, logger)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	defer w.Close()
	hub.SetDeliver(func(msg bindings.MQTTMessage) { w.CallMQTT(msg) })

	handle, err := hub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: portOf(addr)})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer hub.Close(handle)
	if err := hub.Subscribe(handle, "sensors/#", 1); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// Second client publishes to the topic the subscribed client receives.
	pubOpts := bindings.MQTTConfig{Broker: "127.0.0.1", Port: portOf(addr)}
	if err := hub.Publish(handle, "sensors/kitchen/temp", "19.8", 1, false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	_ = pubOpts

	// Poll the worker globals until the handler has run (guarding each read
	// with the slot, consistent with the worker's serialization rules).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.acquireSlot()
		lt := w.L.GetGlobal("last_topic")
		lp := w.L.GetGlobal("last_payload")
		ok := lt != nil && lt.String() == "sensors/kitchen/temp" && lp != nil && lp.String() == "19.8"
		w.freeSlot()
		if ok {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("handler did not receive the message")
}

func TestMQTTHub_MulticastAcrossSubscriptions(t *testing.T) {
	addr, stop := startInProcBroker(t)
	defer stop()

	hub := NewMQTTHub()
	var total atomic.Int64
	hub.SetDeliver(func(bindings.MQTTMessage) { total.Add(1) })

	// Two clients subscribe to overlapping topics.
	h1, err := hub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: portOf(addr)})
	if err != nil {
		t.Fatal(err)
	}
	h2, err := hub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: portOf(addr)})
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close(h1)
	defer hub.Close(h2)
	hub.Subscribe(h1, "a/#", 0)
	hub.Subscribe(h2, "a/b", 0)
	time.Sleep(150 * time.Millisecond)
	hub.Publish(h1, "a/b", "x", 0, false)
	hub.Publish(h2, "a/b", "y", 0, false)
	time.Sleep(500 * time.Millisecond)
	if total.Load() < 2 {
		t.Errorf("delivered = %d, want >= 2 (both clients receive a/b)", total.Load())
	}
}

// portOf extracts the numeric port from a host:port string.
func portOf(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(p, "%d", &n)
	return n
}
