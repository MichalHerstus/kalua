-- serve-communication-plan demo: MQTT client (Phase 2).
--
-- Run:  KALUA serve mqtt_demo.lua --mode http --allow-net
-- Then publish from another client, e.g.:
--   mosquitto_pub -h 127.0.0.1 -t sensors/kitchen/temp -m 19.8
-- The on_sensor handler logs each inbound message (see k.log.stats / klog.db).

k.mqtt_on("sensors/#", "on_sensor")

local handle = nil

function main()
  -- Connect in main() so the handle is ready before the first request.
  handle = k.mqtt_connect{
    broker = "127.0.0.1",
    port = 1883,
    client_id = "kalua-demo",
  }
  if not handle then
    k.print("MQTT connect failed (start serve with --allow-net): " .. tostring(ERRORMSG))
    return
  end
  k.mqtt_subscribe(handle, "sensors/#", 1)
  k.print("subscribed to sensors/#")
end

function on_sensor(topic, payload)
  k.log.info("mqtt", {topic = topic, payload = payload})
  k.print("mqtt " .. topic .. " = " .. payload)
end

function handle_http(req)
  if req.path == "/stats" then
    local st = k.mqtt_stats(handle)
    st.topic = "sensors/#"
    return {json = st}
  end
  -- Publish from an HTTP call: /pub?topic=...&payload=...
  local topic = req.query.topic or "sensors/demo"
  local payload = req.query.payload or "1"
  local ok = k.mqtt_publish(handle, topic, payload, 1)
  return {json = {ok = ok, topic = topic, payload = payload}}
end
