-- serve-mode callbacks: HTTP + WebSocket + TCP
function main() end

function handle_http(req)
  return {status = 200, body = "hello from KALUA (HTTP)"}
end

function handle_ws(msg)
  if msg.type == "text" then
    return "echo:" .. msg.data
  end
end

function handle_tcp(msg)
  if msg.type == "text" then
    return "tcp-echo:" .. msg.data
  end
end
