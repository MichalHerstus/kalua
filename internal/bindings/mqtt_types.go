package bindings

// MQTT types shared between native and WASM builds. The MQTTHub interface is
// implemented by internal/server (native only); on WASM Options.MQTT stays
// nil and the k.mqtt_* bindings are not registered, so only the type
// references need to exist here.

// MQTTHub defines the interface the serve server implements for k.mqtt_*
// (serve-communication-plan.md Phase 2). Implemented by server.MQTTHub; the
// interface lives here so bindings never import internal/server (cycle).
type MQTTHub interface {
	// Connect establishes a client connection. Returns a handle string.
	Connect(cfg MQTTConfig) (string, error)
	// Subscribe registers interest in topic (with MQTT wildcards) at qos.
	Subscribe(handle, topic string, qos byte) error
	// Publish sends payload to topic at qos, optionally retaining it.
	Publish(handle, topic, payload string, qos byte, retain bool) error
	// Unsubscribe removes interest in topic.
	Unsubscribe(handle, topic string) error
	// Close disconnects the client identified by handle.
	Close(handle string) error
	// Stats returns per-client counters and connection state.
	Stats(handle string) (MQTTStats, bool)
}

// MQTTConfig carries k.mqtt_connect options.
type MQTTConfig struct {
	Broker   string // host or full URL ("mqtt://host:port")
	Port     int
	User     string
	Password string
	ClientID string
	TLS      bool
}

// MQTTMessage is an inbound publish delivered to a k.mqtt_on handler.
type MQTTMessage struct {
	Topic   string
	Payload string
}

// MQTTStats is a snapshot of one MQTT client connection.
type MQTTStats struct {
	Connected     bool
	Subscriptions int
	MessagesIn    int64
	MessagesOut   int64
}

// mqttHandler is one registered k.mqtt_on(pattern, fn) entry. fn is a global
// function name resolved at dispatch time (so it works across hot reloads).
type mqttHandler struct {
	pattern string
	fn      string
}
