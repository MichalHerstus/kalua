//go:build !wasm

package server

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"kalua/internal/bindings"
)

// MQTTHub owns outbound MQTT client connections for serve mode
// (serve-communication-plan.md Phase 2). Each k.mqtt_connect creates one paho
// client; inbound messages are pushed to a per-client channel and delivered to
// every worker's registered k.mqtt_on handlers via the worker pool. The hub
// is shared across workers (like WSHub/TCPHub) and survives hot reloads.
type MQTTHub struct {
	mu       sync.RWMutex
	conns    map[string]*mqttClient
	deliver  func(bindings.MQTTMessage) // set by the Server to reach workers
	stopCh   chan struct{}
}

type mqttClient struct {
	handle       string
	client       mqtt.Client
	messagesIn   atomic.Int64
	messagesOut  atomic.Int64
	subMu        sync.Mutex
	subscriptions map[string]byte // topic -> qos
	msgCh        chan bindings.MQTTMessage
}

// NewMQTTHub creates an empty hub.
func NewMQTTHub() *MQTTHub {
	return &MQTTHub{
		conns:  make(map[string]*mqttClient),
		stopCh: make(chan struct{}),
	}
}

// SetDeliver wires the inbound-message pump to the worker pool. Called by the
// Server once workers exist (and re-set after each hot reload so new workers
// are reached).
func (h *MQTTHub) SetDeliver(fn func(bindings.MQTTMessage)) {
	h.mu.Lock()
	h.deliver = fn
	h.mu.Unlock()
}

// Connect establishes a client connection per k.mqtt_connect options and
// returns a handle for the other k.mqtt_* calls.
func (h *MQTTHub) Connect(cfg bindings.MQTTConfig) (string, error) {
	broker := cfg.Broker
	if !strings.Contains(broker, "://") {
		scheme := "tcp"
		if cfg.TLS {
			scheme = "ssl"
		}
		port := cfg.Port
		if port == 0 {
			port = 1883
		}
		broker = fmt.Sprintf("%s://%s:%d", scheme, cfg.Broker, port)
	}
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = randomID("kalua-mqtt")
	}
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetConnectTimeout(10 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetAutoReconnect(true).
		SetOrderMatters(false)
	if cfg.User != "" {
		opts.SetUsername(cfg.User)
	}
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}

	c := &mqttClient{
		handle:        randomID("mqtt"),
		msgCh:         make(chan bindings.MQTTMessage, 256),
		subscriptions: make(map[string]byte),
	}
	// The paho default handler fires for every message on a subscribed topic
	// (we always subscribe with a nil callback). Push into a bounded channel;
	// a per-client pump goroutine delivers to workers (backpressure: a busy
	// pool drops the newest message rather than blocking the paho reader).
	opts.SetDefaultPublishHandler(func(_ mqtt.Client, m mqtt.Message) {
		c.messagesIn.Add(1)
		select {
		case c.msgCh <- bindings.MQTTMessage{Topic: m.Topic(), Payload: string(m.Payload())}:
		default:
		}
	})

	client := mqtt.NewClient(opts)
	tok := client.Connect()
	if !tok.WaitTimeout(15 * time.Second) {
		return "", fmt.Errorf("connect to %s: timeout", broker)
	}
	if err := tok.Error(); err != nil {
		return "", fmt.Errorf("connect to %s: %v", broker, err)
	}
	c.client = client

	h.mu.Lock()
	h.conns[c.handle] = c
	deliver := h.deliver
	h.mu.Unlock()

	// Start the per-client pump delivering inbound messages to workers.
	go c.pump(h, deliver)
	return c.handle, nil
}

// pump forwards inbound messages to the worker dispatcher until the client is
// closed. The deliver func is read under the hub lock each iteration so a
// SetDeliver (initial wiring or hot reload) is always observed; it reaches
// any worker (round-robin via leaseWorker), which matches topics against that
// worker's k.mqtt_on handlers.
func (c *mqttClient) pump(h *MQTTHub, _ func(bindings.MQTTMessage)) {
	for msg := range c.msgCh {
		h.mu.RLock()
		deliver := h.deliver
		h.mu.RUnlock()
		if deliver != nil {
			deliver(msg)
		}
	}
}

// Subscribe registers interest in a topic filter at the given QoS.
func (h *MQTTHub) Subscribe(handle, topic string, qos byte) error {
	c, ok := h.get(handle)
	if !ok {
		return fmt.Errorf("unknown handle %s", handle)
	}
	tok := c.client.Subscribe(topic, qos, nil)
	if !tok.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("subscribe %s: timeout", topic)
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("subscribe %s: %v", topic, err)
	}
	c.subMu.Lock()
	c.subscriptions[topic] = qos
	c.subMu.Unlock()
	return nil
}

// Publish sends a payload to a topic.
func (h *MQTTHub) Publish(handle, topic, payload string, qos byte, retain bool) error {
	c, ok := h.get(handle)
	if !ok {
		return fmt.Errorf("unknown handle %s", handle)
	}
	tok := c.client.Publish(topic, qos, retain, payload)
	if !tok.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("publish %s: timeout", topic)
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("publish %s: %v", topic, err)
	}
	c.messagesOut.Add(1)
	return nil
}

// Unsubscribe removes interest in a topic filter.
func (h *MQTTHub) Unsubscribe(handle, topic string) error {
	c, ok := h.get(handle)
	if !ok {
		return fmt.Errorf("unknown handle %s", handle)
	}
	tok := c.client.Unsubscribe(topic)
	if !tok.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("unsubscribe %s: timeout", topic)
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("unsubscribe %s: %v", topic, err)
	}
	c.subMu.Lock()
	delete(c.subscriptions, topic)
	c.subMu.Unlock()
	return nil
}

// Close disconnects a client and stops its inbound pump.
func (h *MQTTHub) Close(handle string) error {
	c, ok := h.get(handle)
	if !ok {
		return fmt.Errorf("unknown handle %s", handle)
	}
	h.mu.Lock()
	delete(h.conns, handle)
	h.mu.Unlock()
	close(c.msgCh)
	c.client.Disconnect(250)
	return nil
}

// Stats returns per-client counters and connection state.
func (h *MQTTHub) Stats(handle string) (bindings.MQTTStats, bool) {
	c, ok := h.get(handle)
	if !ok {
		return bindings.MQTTStats{}, false
	}
	c.subMu.Lock()
	n := len(c.subscriptions)
	c.subMu.Unlock()
	return bindings.MQTTStats{
		Connected:     c.client.IsConnected(),
		Subscriptions: n,
		MessagesIn:    c.messagesIn.Load(),
		MessagesOut:   c.messagesOut.Load(),
	}, true
}

func (h *MQTTHub) get(handle string) (*mqttClient, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.conns[handle]
	return c, ok
}

// CloseAll disconnects every client (server shutdown).
func (h *MQTTHub) CloseAll() {
	h.mu.Lock()
	conns := make([]*mqttClient, 0, len(h.conns))
	for _, c := range h.conns {
		conns = append(conns, c)
	}
	h.conns = make(map[string]*mqttClient)
	h.mu.Unlock()
	for _, c := range conns {
		close(c.msgCh)
		c.client.Disconnect(250)
	}
}
