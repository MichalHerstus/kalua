//go:build !wasm

package bindings

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua"
)

// registerMQTT installs the Phase 2 MQTT client bindings in serve mode. The
// hub is server-owned; nil disables the group (k.mqtt_* raise a clear error).
func registerMQTT(e *Env, hub MQTTHub) {
	// k.mqtt_on(pattern, fn_name) — register an inbound handler. A message on
	// topic T invokes every handler whose pattern matches T (MQTT wildcards
	// "+" single level, "#" multi level, including the shared-prefix forms).
	e.register("mqtt_on", "comm", func(L *lua.LState) int {
		pattern := L.CheckString(1)
		fn := L.CheckString(2)
		if err := validateMQTTPattern(pattern); err != nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("mqtt_on: %v", err))
		}
		e.mqttHandlers = append(e.mqttHandlers, mqttHandler{pattern: pattern, fn: fn})
		return 0
	})

	if hub == nil {
		// Serve mode without an MQTT hub: the *bindings* exist (scripts load
		// and check passes) but every wire call raises a runtime error.
		noHub := func(name string) lua.LGFunction {
			return func(L *lua.LState) int {
				L.RaiseError("k.mqtt_%s not available: no MQTT hub configured", name)
				return 0
			}
		}
		for _, name := range []string{"connect", "subscribe", "publish", "unsubscribe", "close", "stats"} {
			e.register("mqtt_"+name, "comm", noHub(name))
		}
		return
	}

	// k.mqtt_connect{broker=, port=, user=, password=, client_id=, tls=} -> handle
	e.register("mqtt_connect", "comm", func(L *lua.LState) int {
		if !e.allowNet {
			return e.fail(L, KErrorPermission, "k.mqtt_connect: network egress disabled (serve with --allow-net)")
		}
		var cfg MQTTConfig
		opts := L.CheckTable(1)
		cfg.Broker = lua.LVAsString(opts.RawGetString("broker"))
		cfg.Port = int(lua.LVAsNumber(opts.RawGetString("port")))
		cfg.User = lua.LVAsString(opts.RawGetString("user"))
		cfg.Password = lua.LVAsString(opts.RawGetString("password"))
		cfg.ClientID = lua.LVAsString(opts.RawGetString("client_id"))
		if t := opts.RawGetString("tls"); t != lua.LNil {
			cfg.TLS = lua.LVAsBool(t)
		}
		if cfg.Broker == "" {
			return e.fail(L, KErrorInvalidParam, "mqtt_connect: broker is required")
		}
		if cfg.Port == 0 && !strings.Contains(cfg.Broker, "://") {
			cfg.Port = 1883
		}
		handle, err := hub.Connect(cfg)
		if err != nil {
			return e.fail(L, KErrorComm, fmt.Sprintf("mqtt_connect: %v", err))
		}
		L.Push(lua.LString(handle))
		return 1
	})

	// k.mqtt_subscribe(handle, topic[, qos])
	e.register("mqtt_subscribe", "comm", func(L *lua.LState) int {
		handle := L.CheckString(1)
		topic := L.CheckString(2)
		qos := byte(L.OptInt(3, 0))
		if err := validateMQTTQoS(qos); err != nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("mqtt_subscribe: %v", err))
		}
		if err := hub.Subscribe(handle, topic, qos); err != nil {
			return e.fail(L, KErrorComm, fmt.Sprintf("mqtt_subscribe: %v", err))
		}
		return 0
	})

	// k.mqtt_publish(handle, topic, payload[, qos[, retain]])
	e.register("mqtt_publish", "comm", func(L *lua.LState) int {
		handle := L.CheckString(1)
		topic := L.CheckString(2)
		payload := luaToString(L, 3)
		qos := byte(L.OptInt(4, 0))
		retain := L.OptBool(5, false)
		if err := validateMQTTQoS(qos); err != nil {
			return e.fail(L, KErrorInvalidParam, fmt.Sprintf("mqtt_publish: %v", err))
		}
		if err := hub.Publish(handle, topic, payload, qos, retain); err != nil {
			return e.fail(L, KErrorComm, fmt.Sprintf("mqtt_publish: %v", err))
		}
		return 0
	})

	// k.mqtt_unsubscribe(handle, topic)
	e.register("mqtt_unsubscribe", "comm", func(L *lua.LState) int {
		handle := L.CheckString(1)
		topic := L.CheckString(2)
		if err := hub.Unsubscribe(handle, topic); err != nil {
			return e.fail(L, KErrorComm, fmt.Sprintf("mqtt_unsubscribe: %v", err))
		}
		return 0
	})

	// k.mqtt_close(handle)
	e.register("mqtt_close", "comm", func(L *lua.LState) int {
		handle := L.CheckString(1)
		if err := hub.Close(handle); err != nil {
			return e.fail(L, KErrorComm, fmt.Sprintf("mqtt_close: %v", err))
		}
		return 0
	})

	// k.mqtt_stats(handle) -> {connected, subscriptions, messages_in, messages_out}
	e.register("mqtt_stats", "comm", func(L *lua.LState) int {
		handle := L.CheckString(1)
		st, ok := hub.Stats(handle)
		if !ok {
			return e.fail(L, KErrorNotFound, fmt.Sprintf("mqtt_stats: unknown handle %s", handle))
		}
		tbl := L.NewTable()
		tbl.RawSetString("connected", lua.LBool(st.Connected))
		tbl.RawSetString("subscriptions", lua.LNumber(st.Subscriptions))
		tbl.RawSetString("messages_in", lua.LNumber(st.MessagesIn))
		tbl.RawSetString("messages_out", lua.LNumber(st.MessagesOut))
		L.Push(tbl)
		return 1
	})
}

// DispatchMQTT invokes every registered k.mqtt_on handler whose pattern matches
// the inbound topic, calling fn(topic, payload) in a fresh coroutine per
// handler. Runs under the worker slot already; a handler error is reported via
// the Kalipso error hook and does not abort the other handlers.
func (e *Env) DispatchMQTT(L *lua.LState, msg MQTTMessage) {
	for _, h := range e.mqttHandlers {
		if !mqttTopicMatch(h.pattern, msg.Topic) {
			continue
		}
		fn := L.GetGlobal(h.fn)
		lf, ok := fn.(*lua.LFunction)
		if !ok {
			e.FireError(L, KErrorGeneric, fmt.Sprintf("mqtt handler %q not found", h.fn))
			continue
		}
		co, cancel := L.NewThread()
		st, err, _ := L.Resume(co, lf, lua.LString(msg.Topic), lua.LString(msg.Payload))
		if cancel != nil && st != lua.ResumeOK {
			cancel()
		}
		if st == lua.ResumeError {
			e.FireError(L, KErrorGeneric, err.Error())
		}
	}
}

// validateMQTTPattern accepts a non-empty topic filter. "/" alone is legal
// (matches a single empty level). "#" must be a whole final segment; "+" must
// be a whole segment.
func validateMQTTPattern(p string) error {
	if p == "" {
		return fmt.Errorf("empty topic filter")
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if s == "#" {
			if i != len(segs)-1 {
				return fmt.Errorf("wildcard # must be the final level")
			}
			continue
		}
		if strings.Contains(s, "#") {
			return fmt.Errorf("wildcard # must be a whole level")
		}
		if strings.Contains(s, "+") && s != "+" {
			return fmt.Errorf("wildcard + must be a whole level")
		}
	}
	return nil
}

// validateMQTTQoS ensures qos is 0, 1 or 2.
func validateMQTTQoS(q byte) error {
	if q > 2 {
		return fmt.Errorf("qos must be 0, 1 or 2 (got %d)", q)
	}
	return nil
}

// mqttTopicMatch reports whether an MQTT topic filter matches a concrete
// topic per the MQTT 3.1.1 spec: "+" matches exactly one level, "#" matches
// the remaining levels (it must be the final segment of the filter). Empty
// filter matches only the empty topic.
func mqttTopicMatch(filter, topic string) bool {
	if filter == topic {
		return true
	}
	fSegs := strings.Split(filter, "/")
	tSegs := strings.Split(topic, "/")
	for i, f := range fSegs {
		if f == "#" {
			return true // remaining levels (including none)
		}
		if i >= len(tSegs) {
			return false
		}
		if f == "+" {
			continue
		}
		if f != tSegs[i] {
			return false
		}
	}
	return len(fSegs) == len(tSegs)
}
