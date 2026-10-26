-- serve-communication-plan demo: declarative REST routes + middleware +
-- structured logging (k.log.* lands in klog.db with --log-sqlite).
--
-- Run:  KALUA serve rest_api_demo.lua --log-sqlite --log-level info
-- Then: curl http://127.0.0.1:8080/api/users/42
--       curl -X POST http://127.0.0.1:8080/api/users -d '{"name":"Ada"}'
--       curl http://127.0.0.1:8080/stats

k.http_routes = {
  {method = "GET",  path = "/api/users/:id", handler = "get_user"},
  {method = "POST", path = "/api/users",     handler = "create_user"},
  {method = "GET",  path = "/stats",         handler = "show_stats"},
}

-- Middleware chain: runs before every matched route. Return nil to continue;
-- return a response (k.http_response_*) to short-circuit.
k.http_middleware = {"logging_mw", "auth_mw"}

function logging_mw(req)
  k.log.info("request", {method = req.method, path = req.path})
  return nil -- continue the chain
end

function auth_mw(req)
  local token = req.headers["Authorization"]
  if not token then
    return k.http_response_json({error = "unauthorized"}, 401)
  end
  req.user = "anonymous" -- pass data to the handler
  return nil             -- continue
end

function get_user(req)
  local id = tonumber(req.params.id)
  k.log.debug("get_user", {id = id, user = req.user})
  if id == nil or id <= 0 then
    return k.http_response_json({error = "bad id"}, 400)
  end
  return k.http_response_json({id = id, name = "User " .. id}, 200)
end

function create_user(req)
  local body = k.json_parse(req.body)
  if not body or not body.name then
    return k.http_response_json({error = "name required"}, 400)
  end
  k.log.info("user created", {name = body.name})
  return k.http_response_json({created = body.name}, 201)
end

function show_stats(req)
  local s = k.http_stats()
  s.log = k.log.stats()
  return k.http_response_json(s)
end

function main()
  k.print("REST demo running: GET /api/users/:id, POST /api/users")
end
