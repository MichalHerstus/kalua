//go:build !wasm

package server

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/yuin/gopher-lua"

	"kalua/internal/bindings"
	"kalua/internal/host"
)

// TestMqttE2E_ServerRoundTrip boots a full serve Server (http mode); a second
// paho client publishes to a topic the script subscribed to via the hub, and
// the Lua k.mqtt_on handler must observe it. Exercises Options.MQTT wiring,
// the hub → worker deliver path (wired by startWorkers), and DispatchMQTT.
func TestMqttE2E_ServerRoundTrip(t *testing.T) {
	addr, stop := startInProcBroker(t)
	defer stop()
	port := portOf(addr)

	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	os.WriteFile(script, []byte(`
k.mqtt_on("e2e/#", "on_e2e")
function on_e2e(t, p) e2e_topic = t; e2e_payload = p end
function main() end
`), 0o644)

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    1,
		Mode:       "http",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
		AllowNet:   true,
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()
	select {
	case <-s.Ready():
	case err := <-runDone:
		t.Fatalf("boot failed: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("server not ready")
	}

	handle, err := s.mqttHub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: port, ClientID: "e2e-sub"})
	if err != nil {
		t.Fatalf("hub connect: %v", err)
	}
	defer s.mqttHub.Close(handle)
	if err := s.mqttHub.Subscribe(handle, "e2e/#", 1); err != nil {
		t.Fatalf("hub subscribe: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let SUBSCRIBE reach the broker

	// A second client publishes to the subscribed topic.
	pub := mqtt.NewClient(mqtt.NewClientOptions().
		AddBroker("tcp://127.0.0.1:" + strconv.Itoa(port)).
		SetClientID("e2e-pub"))
	if tok := pub.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("pub connect: %v", tok.Error())
	}
	defer pub.Disconnect(100)
	if tok := pub.Publish("e2e/1", 1, false, "hello-from-second-client"); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("pub publish: %v", tok.Error())
	}

	// Poll the (single) worker's globals until the handler ran.
	s.workerMu.RLock()
	workers := append([]*Worker{}, s.workers...)
	s.workerMu.RUnlock()
	if len(workers) == 0 {
		t.Fatal("no workers")
	}
	w := workers[0]
	deadline := time.Now().Add(5 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		w.acquireSlot()
		topicV := w.L.GetGlobal("e2e_topic")
		payloadV := w.L.GetGlobal("e2e_payload")
		w.freeSlot()
		if topicV != nil && topicV.String() == "e2e/1" &&
			payloadV != nil && payloadV.String() == "hello-from-second-client" {
			seen = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !seen {
		t.Fatal("e2e message never reached the Lua handler")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
		t.Fatal("server did not stop")
	}
}

// TestMqttE2E_MultiWorkerDispatch verifies the AutoReload-safe dispatch: after
// a hot reload, the hub's deliver closure (which leases workers dynamically)
// reaches the NEW pool — inbound MQTT messages keep flowing post-reload.
func TestMqttE2E_MultiWorkerDispatch(t *testing.T) {
	addr, stop := startInProcBroker(t)
	defer stop()
	port := portOf(addr)

	dir := t.TempDir()
	script := filepath.Join(dir, "app.lua")
	os.WriteFile(script, []byte(`
k.mqtt_on("auto/#", "on_auto")
function on_auto(t, p) auto_hits = (auto_hits or 0) + 1 end
function main() end
`), 0o644)

	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		Workers:    2,
		Mode:       "http",
		ScriptPath: script,
		Logger:     host.NewLogger(false),
		AllowNet:   true,
	}
	s := NewServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()
	select {
	case <-s.Ready():
	case err := <-runDone:
		t.Fatalf("boot failed: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("not ready")
	}

	handle, err := s.mqttHub.Connect(bindings.MQTTConfig{Broker: "127.0.0.1", Port: port, ClientID: "auto-sub"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer s.mqttHub.Close(handle)
	if err := s.mqttHub.Subscribe(handle, "auto/#", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish #1 before reload.
	pub := mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://127.0.0.1:" + strconv.Itoa(port)).SetClientID("auto-pub"))
	if tok := pub.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("pub connect: %v", tok.Error())
	}
	defer pub.Disconnect(100)

	totalHits := func() int64 {
		var total int64
		s.workerMu.RLock()
		for _, w := range s.workers {
			w.acquireSlot()
			v := w.L.GetGlobal("auto_hits")
			if n, ok := v.(lua.LNumber); ok {
				total += int64(n)
			}
			w.freeSlot()
		}
		s.workerMu.RUnlock()
		return total
	}

	// waitForValue polls fn until it returns >= want (with a deadline).
	waitForValue := func(fn func() int64, want int64) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if fn() >= want {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("did not reach %d (got %d)", want, fn())
	}

	if tok := pub.Publish("auto/room", 0, false, "1"); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("pub publish: %v", tok.Error())
	}
	waitForValue(totalHits, 1)

	// Reload the server (SIGHUP path). The hub deliver closure survives and
	// must now reach the fresh pool: a publish after the swap must land on a
	// NEW worker (the freshly built pool starts with auto_hits cleared).
	if err := s.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if tok := pub.Publish("auto/room", 0, false, "2"); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("pub publish after reload: %v", tok.Error())
	}
	waitForValue(totalNewHits(s), 1)

	cancel()
	select {
	case <-runDone:
	case <-time.After(8 * time.Second):
		t.Fatal("did not stop")
	}
}

// totalNewHits sums auto_hits across the server's CURRENT worker pool only
// (used after a hot reload, where the superseded pool is no longer counted).
func totalNewHits(s *Server) func() int64 {
	return func() int64 {
		var total int64
		s.workerMu.RLock()
		ws := append([]*Worker{}, s.workers...)
		s.workerMu.RUnlock()
		for _, w := range ws {
			w.acquireSlot()
			if v, ok := w.L.GetGlobal("auto_hits").(lua.LNumber); ok {
				total += int64(v)
			}
			w.freeSlot()
		}
		return total
	}
}
