-- serve-mode HTTP callback
function main() end

function handle_http(req)
  return {status = 200, body = "hello from KALUA"}
end
