-- api_demo.lua
-- Serve-mode showcase exercising expression functions + k.shared.*
-- Run: KALUA serve testdata/apps/api_demo.lua --port 8080 --workers 2 --mode http,ws

-- Minimal main() - runs once at worker startup to initialize
function main()
  k.print("api_demo worker started")
end

-- Shared state keys
local SHARED_KEYS = {
  COUNTER = "demo:counter",
  VISITS = "demo:visits",
  LAST_REQ = "demo:last_request",
}

-- Optional: runs once at startup (first worker)
function init(config)
  k.print("api_demo init: starting up")
  k.shared.set(SHARED_KEYS.COUNTER, "0")
  k.shared.set(SHARED_KEYS.VISITS, "0")
  k.shared.set("demo:started_at", sys_date() .. " " .. sys_time())
end

-- Required for --mode http
function handle_http(req)
  -- Increment visit counter
  local visits = k.shared.incr(SHARED_KEYS.VISITS)

  -- Track last request
  k.shared.set(SHARED_KEYS.LAST_REQ, jsonencode({
    method = req.method,
    path = req.path,
    query = req.query,
    remote = req.remote_addr,
    time = sys_date() .. " " .. sys_time(),
  }))

  -- Route based on path
  if req.path == "/health" then
    return {json = {status = "ok", visits = visits}}
  elseif req.path == "/counter" then
    return handle_counter(req)
  elseif req.path == "/expr" then
    return handle_expr(req)
  elseif req.path == "/shared" then
    return handle_shared(req)
  elseif req.path == "/data" then
    return handle_data_formats(req)
  else
    return {status = 404, json = {error = "not found", path = req.path}}
  end
end

-- Counter endpoint: demonstrates k.shared.incr/get/set/del/keys
function handle_counter(req)
  local method = req.method
  local key = SHARED_KEYS.COUNTER

  if method == "GET" then
    local val = k.shared.get(key)
    return {json = {counter = tonum(val)}}
  elseif method == "POST" then
    local delta = 1
    if req.query and req.query.delta then
      delta = tonum(req.query.delta)
    end
    local newVal = k.shared.incr(key, delta)
    return {json = {counter = tonum(newVal), delta = delta}}
  elseif method == "DELETE" then
    k.shared.del(key)
    return {json = {deleted = true}}
  else
    return {status = 405, json = {error = "method not allowed"}}
  end
end

-- Expression functions demo endpoint
function handle_expr(req)
  local input = req.query.input or "hello world"
  local num = tonum(req.query.num) or 42

  local result = {
    input = input,
    num = num,
    string_ops = {
      upper = upper(input),
      lower = lower(input),
      length = length(input),
      trim = trim("  " .. input .. "  "),
      left = left(input, 5),
      right = right(input, 5),
      middle = middle(input, 3, 4),
      find = find(input, "world"),
      replace = replace(input, "world", "kalua"),
      base64_encode = base64_encode(input),
      base64_decode = base64_decode(base64_encode(input)),
      urlencode = urlencode(input .. " & more"),
      urldecode = urldecode(urlencode(input .. " & more")),
      jsonencode = jsonencode({msg = input, num = num}),
      jsondecode = jsondecode(jsonencode({msg = input, num = num})).msg,
      guid = guid(),
      mltext = mltext("line1", "line2", "line3"),
    },
    numeric_ops = {
      abs = abs(-num),
      round = round(num / 3, 2),
      floor = floor(num / 3),
      ceiling = ceiling(num / 3),
      power = power(num, 2),
      sqrt = sqrt(num),
      sin = sin(num),
      cos = cos(num),
      random = random(1, 100),
      int_part = int_part(num / 3),
      dec_part = dec_part(num / 3),
      bitwise_and = bitwise_and(num, 0xFF),
      bitwise_or = bitwise_or(num, 0xFF00),
      bitwise_xor = bitwise_xor(num, 0xFFFF),
      val = val(input),
      sum = sum({1, 2, 3, num}),
    },
    conditional_ops = {
      lookup_found = lookup("b", "a", 1, "b", 2, "c", 3),
      lookup_missing = lookup("z", "a", 1, "b", 2),
      yesno_true = yesno(1, "yes", "no"),
      yesno_false = yesno("", "yes", "no"),
      iif_true = iif(num > 10, "big", "small"),
      iif_false = iif(num < 0, "neg", "pos"),
    },
    datetime_ops = {
      sys_date = sys_date(),
      sys_time = sys_time(),
      today = sys_date(),
      add_7_days = add_days(sys_date(), 7),
      sub_3_days = subtract_days(sys_date(), 3),
      day = day(sys_date()),
      month = month(sys_date()),
      year = year(sys_date()),
      week_day = week_day(sys_date()), -- Sunday=1
      week_number = week_number(sys_date()),
      date_to_string = date_to_string(sys_date(), "%Y-%m-%d"),
      datetime_str = sys_date() .. " " .. sys_time(),
      time_to_string = time_to_string(sys_date() .. " " .. sys_time(), "%H:%M:%S"),
    },
    conversion_ops = {
      tostr = tostr(num),
      tonum = tonum("123.45"),
      todate = todate("2026-01-15"),
      strtodate = strtodate("15/01/2026", "%d/%m/%Y"),
      boolstr_true = boolstr(1),
      boolstr_false = boolstr(""),
    },
  }

  return {json = result}
end

-- Shared state inspection endpoint
function handle_shared(req)
  local keys = k.shared.keys("demo:*")
  local result = {}
  if keys then
    for _, key in ipairs(keys) do
      result[key] = k.shared.get(key)
    end
  end
  return {json = {keys = result}}
end

-- Data formats round-trip demo
function handle_data_formats(req)
  local sample = {
    users = {
      {id = 1, name = "Alice", active = true},
      {id = 2, name = "Bob", active = false},
    },
    meta = {version = 1, tags = {"demo", "kalua"}},
  }

  local result = {}

  -- JSON
  local jsonStr = jsonencode(sample)
  result.json = {
    original = sample,
    encoded = jsonStr,
    decoded = jsondecode(jsonStr),
    load_save = "see /data/json (file ops not shown)",
  }

  -- CSV (table of rows)
  local csvTable = {
    {"id", "name", "active"},
    {"1", "Alice", "true"},
    {"2", "Bob", "false"},
  }
  local csvStr = k.csv_string(csvTable, {header = true})
  result.csv = {
    original = csvTable,
    encoded = csvStr,
    parsed = k.csv_parse(csvStr, {header = true}),
  }

  -- INI
  local iniTable = {
    database = {host = "localhost", port = "5432"},
    app = {debug = "true"},
    _root = {version = "1.0"},
  }
  local iniStr = k.ini_string(iniTable)
  result.ini = {
    original = iniTable,
    encoded = iniStr,
    parsed = k.ini_parse(iniStr),
  }

  -- YAML
  local yamlStr = k.yaml_string(sample)
  result.yaml = {
    original = sample,
    encoded = yamlStr,
    decoded = k.yaml_parse(yamlStr),
  }

  -- XML
  local xmlTable = {
    _name = "demo",
    _attrs = {version = "1"},
    _children = {
      {_name = "user", _attrs = {id = "1"}, _children = {}, _text = "Alice"},
      {_name = "user", _attrs = {id = "2"}, _children = {}, _text = "Bob"},
    },
    _text = "",
  }
  local xmlStr = k.xml_save("xml_demo.xml", xmlTable)
  result.xml = {
    original = xmlTable,
    encoded = xmlStr,
    parsed = k.xml_load("xml_demo.xml"),
  }

  return {json = result}
end

-- Optional: runs once on SIGTERM/SIGINT before exit
function shutdown()
  k.print("api_demo shutdown: cleaning up")
  -- k.disconnect_db() if using DB
end

-- WebSocket handler (for --mode ws)
function handle_ws(msg)
  if msg.type == "open" then
    k.ws.send(msg.client_id, jsonencode({type = "welcome", msg = "connected to api_demo"}))
  elseif msg.type == "text" then
    -- Echo with metadata
    local data = jsondecode(msg.data)
    local reply = {
      type = "echo",
      original = data,
      server_time = sys_date() .. " " .. sys_time(),
      client_id = msg.client_id,
    }
    return jsonencode(reply) -- string return = echo back to sender
  elseif msg.type == "close" then
    k.print("WS client disconnected:", msg.client_id)
  end
end

-- TCP handler (for --mode tcp)
function handle_tcp(msg)
  if msg.type == "open" then
    k.print("TCP client connected:", msg.client_id)
  elseif msg.type == "text" or msg.type == "data" then
    local reply = "ECHO: " .. msg.data .. " (at " .. sys_time() .. ")"
    return reply
  elseif msg.type == "close" then
    k.print("TCP client disconnected:", msg.client_id)
  end
end