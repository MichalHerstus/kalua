-- serve-communication-plan demo: SOAP server (Phase 4).
--
-- Run:  KALUA serve soap_demo.lua --mode http
-- Then:  curl -X POST http://127.0.0.1:8080/soap \
--   -H "Content-Type: text/xml; charset=utf-8" \
--   -H "SOAPAction: GetUser" \
--   -d '<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><GetUser><userId>1</userId></GetUser></soap:Body></soap:Envelope>'

k.soap_operations = {
  GetUser = "soap_get_user",
  CreateUser = "soap_create_user",
}

function soap_get_user(body, headers)
  local userId = body.GetUser and body.GetUser.userId or "1"
  return {GetUserResponse = {user = {id = userId, name = "User " .. userId}}}
end

function soap_create_user(body, headers)
  local name = body.CreateUser and body.CreateUser.name or "New User"
  return {CreateUserResponse = {user = {id = "new", name = name}}}
end

function main()
  k.print("SOAP demo ready")
end