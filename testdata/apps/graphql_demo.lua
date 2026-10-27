-- serve-communication-plan demo: GraphQL server (Phase 3).
--
-- Run:  KALUA serve graphql_demo.lua --mode http
-- Then: curl -X POST http://127.0.0.1:8080/graphql -H "Content-Type: application/json" -d '{"query":"{ user(id: 1) { id name email } }"}'

k.graphql_schema = {
  types = {
    User = {fields = {
      id = {type = "ID"},
      name = "String",
      email = "String",
    }},
    Query = {fields = {
      user = {type = "User", args = {id = "ID"}, resolve = "resolve_user"},
      users = {type = "[User]", resolve = "resolve_users"},
    }},
    Mutation = {fields = {
      createUser = {type = "User", args = {name = "String", email = "String"}, resolve = "create_user"},
    }},
  }
}

-- Resolvers receive (args, context, info)
function resolve_user(args, ctx, info)
  -- In a real app, this would query a database
  -- For demo, return mock data
  if args.id == "1" then
    return {id = "1", name = "Alice", email = "alice@example.com"}
  elseif args.id == "2" then
    return {id = "2", name = "Bob", email = "bob@example.com"}
  end
  return nil
end

function resolve_users(args, ctx, info)
  return {
    {id = "1", name = "Alice", email = "alice@example.com"},
    {id = "2", name = "Bob", email = "bob@example.com"},
    {id = "3", name = "Charlie", email = "charlie@example.com"},
  }
end

function create_user(args, ctx, info)
  -- In a real app, this would insert into a database
  return {id = "4", name = args.name, email = args.email}
end

function main()
  k.print("GraphQL demo ready")
end