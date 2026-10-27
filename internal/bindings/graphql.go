//go:build !wasm

package bindings

import (
	"github.com/yuin/gopher-lua"
)

// GraphQLConfig holds configuration for the GraphQL server.
type GraphQLConfig struct {
	SchemaTable string // name of the schema table (default: k.graphql_schema)
}

// graphqlContextKey is the context key for the per-request GraphQL context.
type graphqlContextKey struct{}

// registerGraphQL installs the GraphQL server in serve mode.
// It registers k.graphql_context to set a per-request context function.
func registerGraphQL(e *Env) {
	k := e.L.GetGlobal("k").(*lua.LTable)

	// k.graphql_context(fn) - sets a per-request context initializer
	// The function receives the HTTP request table and returns a context table
	// that will be available to resolvers as the 'context' argument.
	k.RawSetString("graphql_context", e.L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		e.L.SetGlobal("_graphql_context_fn", fn)
		return 0
	}))

	// The actual GraphQL schema building happens in the server when the
	// script defines k.graphql_schema. The server will parse it and register
	// the /graphql endpoint.
}