//go:build !wasm

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"

	"github.com/yuin/gopher-lua"
)

// GraphQLServer handles GraphQL execution for serve mode.
// It parses a Lua-defined schema table into an executable GraphQL schema.
type GraphQLServer struct {
	mu        sync.RWMutex
	schema    *graphqlSchema
	contextFn func(*HTTPRequest) map[string]any // optional per-request context
	stats     GraphQLStats
	L         *lua.LState // Lua state for calling resolvers
}

// GraphQLStats tracks GraphQL request metrics.
type GraphQLStats struct {
	QueriesTotal     int64
	MutationsTotal   int64
	ErrorsTotal      int64
	LatencySumMs     int64
	LatencyCount     int64
}

// graphqlSchema holds the parsed GraphQL schema with Lua resolver functions.
type graphqlSchema struct {
	astSchema       *ast.SchemaDocument
	validatorSchema *ast.Schema
	resolvers       map[string]map[string]*luaResolver // typeName -> fieldName -> resolver
	operations      map[string]*ast.OperationDefinition // query/mutation names
}

// luaResolver wraps a Lua function to be called as a GraphQL field resolver.
type luaResolver struct {
	fnName    string
	fieldName string
	typeName  string
}

// newGraphQLServer creates a new GraphQL server instance.
func newGraphQLServer() *GraphQLServer {
	return &GraphQLServer{
		schema: nil,
		stats:  GraphQLStats{},
	}
}

// BuildSchema constructs the GraphQL schema from the k.graphql_schema table.
// Called during SetupServe to register the schema before any requests arrive.
func (s *GraphQLServer) BuildSchema(L *lua.LState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.L = L // Store the Lua state for resolver calls

	schemaTable := L.GetGlobal("k").(*lua.LTable).RawGetString("graphql_schema")
	if schemaTable == lua.LNil {
		return nil // No schema defined
	}

	tbl, ok := schemaTable.(*lua.LTable)
	if !ok {
		return errors.New("k.graphql_schema must be a table")
	}

	gqlSchema := &graphqlSchema{
		resolvers:  make(map[string]map[string]*luaResolver),
		operations: make(map[string]*ast.OperationDefinition),
	}

	// Parse types table
	typesTbl := tbl.RawGetString("types")
	if typesTbl == lua.LNil {
		return errors.New("k.graphql_schema.types is required")
	}

	typesLuaTbl, ok := typesTbl.(*lua.LTable)
	if !ok {
		return errors.New("k.graphql_schema.types must be a table")
	}

	// Build SDL from Lua table
	sdl := buildSDLFromLua(typesLuaTbl)
	if sdl == "" {
		return errors.New("no types defined in k.graphql_schema.types")
	}

	// Parse SDL with gqlparser
	src := &ast.Source{Input: sdl, Name: "schema.graphql"}
	doc, err := parser.ParseSchema(src)
	if err != nil {
		return fmt.Errorf("failed to parse GraphQL schema: %v", err)
	}

	// Schema is valid if it parses successfully (parser.ParseSchema validates it)

	// Extract resolvers from the types table
	for _, typeName := range getLuaTableKeys(typesLuaTbl) {
		typeTbl := typesLuaTbl.RawGetString(typeName)
		if typeTbl == lua.LNil {
			continue
		}
		if typeLuaTbl, ok := typeTbl.(*lua.LTable); ok {
			fieldsTbl := typeLuaTbl.RawGetString("fields")
			if fieldsTbl == lua.LNil {
				continue
			}
			if fieldsLuaTbl, ok := fieldsTbl.(*lua.LTable); ok {
				for _, fieldName := range getLuaTableKeys(fieldsLuaTbl) {
					fieldDef := fieldsLuaTbl.RawGetString(fieldName)
					if fieldDef == lua.LNil {
						continue
					}
					if fieldLuaTbl, ok := fieldDef.(*lua.LTable); ok {
						resolve := fieldLuaTbl.RawGetString("resolve")
						if resolve == lua.LNil {
							continue
						}
						if fn, ok := resolve.(lua.LString); ok {
							if gqlSchema.resolvers[typeName] == nil {
								gqlSchema.resolvers[typeName] = make(map[string]*luaResolver)
							}
							gqlSchema.resolvers[typeName][fieldName] = &luaResolver{
								fnName:    string(fn),
								fieldName: fieldName,
								typeName:  typeName,
							}
						}
					}
				}
			}
		}
	}

	// Extract operations (Query, Mutation, Subscription)
	for _, opName := range []string{"Query", "Mutation", "Subscription"} {
		opTbl := typesLuaTbl.RawGetString(opName)
		if opTbl != lua.LNil {
			if opLuaTbl, ok := opTbl.(*lua.LTable); ok {
				fieldsTbl := opLuaTbl.RawGetString("fields")
				if fieldsTbl != lua.LNil {
					if fieldsLuaTbl, ok := fieldsTbl.(*lua.LTable); ok {
						for _, fieldName := range getLuaTableKeys(fieldsLuaTbl) {
							fieldDef := fieldsLuaTbl.RawGetString(fieldName)
							if fieldDef == lua.LNil {
								continue
							}
							if fieldLuaTbl, ok := fieldDef.(*lua.LTable); ok {
								resolve := fieldLuaTbl.RawGetString("resolve")
								if resolve == lua.LNil {
									continue
								}
								if fn, ok := resolve.(lua.LString); ok {
									if gqlSchema.resolvers[opName] == nil {
										gqlSchema.resolvers[opName] = make(map[string]*luaResolver)
									}
									gqlSchema.resolvers[opName][fieldName] = &luaResolver{
										fnName:    string(fn),
										fieldName: fieldName,
										typeName:  opName,
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Parse query operations from the schema SDL for operation lookup
	astSrc := &ast.Source{Input: sdl, Name: "schema.graphql"}
	astDoc, err := parser.ParseQuery(astSrc)
	if err == nil {
		for _, opDef := range astDoc.Operations {
			if opDef.Name != "" {
				gqlSchema.operations[opDef.Name] = opDef
			}
		}
	}

	gqlSchema.astSchema = doc
	s.schema = gqlSchema
	return nil
}

// SetContextFunc sets the optional per-request context initializer.
// Called from SetupServe when k.graphql_context is defined.
func (s *GraphQLServer) SetContextFn(fn func(*HTTPRequest) map[string]any) {
	s.contextFn = fn
}

// Execute runs a GraphQL query/mutation and returns the response.
func (s *GraphQLServer) Execute(ctx context.Context, req *GraphQLRequest) *GraphQLResponse {
	startTime := time.Now()
	defer func() {
		latency := time.Since(startTime).Milliseconds()
		s.mu.Lock()
		s.stats.LatencySumMs += latency
		s.stats.LatencyCount++
		s.mu.Unlock()
	}()

	if s.schema == nil {
		s.mu.Lock()
		s.stats.ErrorsTotal++
		s.mu.Unlock()
		return &GraphQLResponse{
			Errors: []gqlerror.Error{{Message: "no GraphQL schema defined"}},
		}
	}

	// Parse the query
	querySrc := &ast.Source{Input: req.Query, Name: "query.graphql"}
	queryAST, err := parser.ParseQuery(querySrc)
	if err != nil {
		s.mu.Lock()
		s.stats.ErrorsTotal++
		s.mu.Unlock()
		return &GraphQLResponse{
			Errors: []gqlerror.Error{{Message: fmt.Sprintf("parse error: %v", err)}},
		}
	}

	// Validate query
	if errs := validator.Validate(s.schema.validatorSchema, queryAST); len(errs) > 0 {
		s.mu.Lock()
		s.stats.ErrorsTotal++
		s.mu.Unlock()
		var gqlErrs []gqlerror.Error
		for _, e := range errs {
			gqlErrs = append(gqlErrs, gqlerror.Error{Message: e.Message})
		}
		return &GraphQLResponse{Errors: gqlErrs}
	}

	// Build context for resolvers
	resolverCtx := &resolverContext{
		ctx:      ctx,
		schema:   s.schema,
		variables: req.Variables,
	}

	// Execute the operation
	var opDef *ast.OperationDefinition
	if req.OperationName != "" {
		opDef = s.schema.operations[req.OperationName]
	} else {
		// Find the first operation
		for _, op := range queryAST.Operations {
			if op.Name != "" {
				opDef = op
				break
			}
		}
	}

	if opDef == nil {
		return &GraphQLResponse{
			Errors: []gqlerror.Error{{Message: "no operation found"}},
		}
	}

	// Execute based on operation type
	var data interface{}
	var errs []gqlerror.Error

	switch opDef.Operation {
	case ast.Query:
		s.mu.Lock()
		s.stats.QueriesTotal++
		s.mu.Unlock()
		data, errs = s.executeSelectionSet(ctx, nil, opDef.SelectionSet, resolverCtx, s.schema.resolvers["Query"])
	case ast.Mutation:
		s.mu.Lock()
		s.stats.MutationsTotal++
		s.mu.Unlock()
		data, errs = s.executeSelectionSet(ctx, nil, opDef.SelectionSet, resolverCtx, s.schema.resolvers["Mutation"])
	case ast.Subscription:
		// Subscriptions not yet implemented
		errs = []gqlerror.Error{{Message: "subscriptions not supported yet"}}
	default:
		errs = []gqlerror.Error{{Message: "unknown operation type"}}
	}

	resp := &GraphQLResponse{Data: data}
	if len(errs) > 0 {
		resp.Errors = errs
		s.mu.Lock()
		s.stats.ErrorsTotal++
		s.mu.Unlock()
	}
	return resp
}

// GetStats returns the current GraphQL statistics.
func (s *GraphQLServer) GetStats() GraphQLStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// resolverContext carries execution context for resolvers.
type resolverContext struct {
	ctx       context.Context
	schema    *graphqlSchema
	variables map[string]any
}

// executeSelectionSet executes a GraphQL selection set against a root value.
func (s *GraphQLServer) executeSelectionSet(ctx context.Context, rootValue any, selSet ast.SelectionSet, rctx *resolverContext, resolvers map[string]*luaResolver) (map[string]any, []gqlerror.Error) {
	result := make(map[string]any)
	var errs []gqlerror.Error

	for _, sel := range selSet {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue // skip fragments, inline fragments for now
		}

		resolver, ok := resolvers[field.Name]
		if !ok {
			errs = append(errs, gqlerror.Error{
				Message:   fmt.Sprintf("no resolver for field %s", field.Name),
				Locations: []gqlerror.Location{{Line: field.Position.Line, Column: field.Position.Column}},
			})
			continue
		}

		// Prepare arguments
		args := make(map[string]any)
		for _, arg := range field.Arguments {
			args[arg.Name] = coerceValue(arg.Value)
		}

		// Call Lua resolver
		val, err := s.callLuaResolver(resolver.fnName, args, ctx)
		if err != nil {
			errs = append(errs, gqlerror.Error{
				Message:   err.Error(),
				Locations: []gqlerror.Location{{Line: field.Position.Line, Column: field.Position.Column}},
			})
			continue
		}

		// If the field has sub-selections, recurse
		if field.SelectionSet != nil {
			if mapVal, ok := val.(map[string]any); ok {
				nestedVal, nestedErrs := s.executeSelectionSet(ctx, mapVal, field.SelectionSet, rctx, resolvers)
				if len(nestedErrs) > 0 {
					errs = append(errs, nestedErrs...)
				}
				result[field.Alias] = nestedVal
			} else if sliceVal, ok := val.([]any); ok {
				// Handle list of objects
				var nestedResults []map[string]any
				for _, item := range sliceVal {
					if itemMap, ok := item.(map[string]any); ok {
						nestedVal, nestedErrs := s.executeSelectionSet(ctx, itemMap, field.SelectionSet, rctx, resolvers)
						if len(nestedErrs) > 0 {
							errs = append(errs, nestedErrs...)
						}
						nestedResults = append(nestedResults, nestedVal)
					}
				}
				result[field.Alias] = nestedResults
			}
		} else {
			result[field.Alias] = val
		}
	}

	return result, errs
}

// coerceValue converts a gqlparser AST value to a Go value.
func coerceValue(val *ast.Value) any {
	switch val.Kind {
	case ast.IntValue:
		i, _ := strconv.Atoi(val.Raw)
		return i
	case ast.FloatValue:
		f, _ := strconv.ParseFloat(val.Raw, 64)
		return f
	case ast.StringValue, ast.BlockValue:
		return val.Raw
	case ast.BooleanValue:
		return val.Raw == "true"
	case ast.NullValue:
		return nil
	case ast.EnumValue:
		return val.Raw
	case ast.ListValue:
		var arr []any
		for _, child := range val.Children {
			arr = append(arr, coerceValue(child.Value))
		}
		return arr
	case ast.ObjectValue:
		obj := make(map[string]any)
		for _, child := range val.Children {
			obj[child.Name] = coerceValue(child.Value)
		}
		return obj
	default:
		return nil
	}
}

// callLuaResolver calls a Lua resolver function by name with arguments and context.
// Returns the result value and any error.
func (s *GraphQLServer) callLuaResolver(fnName string, args map[string]any, ctx context.Context) (any, error) {
	if s.L == nil {
		return nil, errors.New("no Lua state available")
	}

	L := s.L

	// Push the function
	fn := L.GetGlobal(fnName)
	if fn == lua.LNil {
		return nil, errors.New("resolver function not found: " + fnName)
	}
	lf, ok := fn.(*lua.LFunction)
	if !ok {
		return nil, errors.New("resolver is not a function: " + fnName)
	}

	L.Push(lf)

	// Push arguments as a table
	argsTbl := L.NewTable()
	for k, v := range args {
		argsTbl.RawSetString(k, toLuaValue(L, v))
	}
	L.Push(argsTbl)

	// Push context
	ctxTbl := L.NewTable()
	if ctx != nil {
		// Add context values if needed
	}
	L.Push(ctxTbl)

	// Call the function with 2 arguments (args, context)
	err := L.PCall(2, lua.MultRet, nil)
	if err != nil {
		return nil, fmt.Errorf("resolver %s error: %v", fnName, err)
	}

	// Get results
	n := L.GetTop()
	if n == 0 {
		return nil, nil
	}

	// For now, return the first result
	ret := L.Get(n)
	L.SetTop(n - 1)

	return luaValueToGo(ret), nil
}

// toLuaValue converts a Go value to a Lua value.
func toLuaValue(L *lua.LState, val any) lua.LValue {
	switch v := val.(type) {
	case string:
		return lua.LString(v)
	case int:
		return lua.LNumber(v)
	case int64:
		return lua.LNumber(v)
	case float64:
		return lua.LNumber(v)
	case bool:
		return lua.LBool(v)
	case map[string]any:
		tbl := L.NewTable()
		for k, v := range v {
			tbl.RawSetString(k, toLuaValue(L, v))
		}
		return tbl
	case []any:
		tbl := L.NewTable()
		for i, v := range v {
			tbl.RawSetInt(i+1, toLuaValue(L, v))
		}
		return tbl
	case nil:
		return lua.LNil
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// buildSDLFromLua converts the Lua types table to GraphQL SDL string.
func buildSDLFromLua(typesTbl *lua.LTable) string {
	var sb strings.Builder
	typesTbl.ForEach(func(name lua.LValue, value lua.LValue) {
		typeName := name.String()
		if typeTbl, ok := value.(*lua.LTable); ok {
			sb.WriteString(fmt.Sprintf("type %s {\n", typeName))
			fieldsTbl := typeTbl.RawGetString("fields")
			if fieldsTbl != lua.LNil {
				if fieldsLuaTbl, ok := fieldsTbl.(*lua.LTable); ok {
					fieldsLuaTbl.ForEach(func(fname lua.LValue, fdef lua.LValue) {
						fieldName := fname.String()
						if fieldLuaTbl, ok := fdef.(*lua.LTable); ok {
							typeVal := fieldLuaTbl.RawGetString("type")
							if typeVal != lua.LNil {
								sb.WriteString(fmt.Sprintf("  %s: %s\n", fieldName, typeVal.String()))
							}
						}
					})
				}
			}
			sb.WriteString("}\n\n")
		}
	})
	return sb.String()
}

// getLuaTableKeys returns all string keys from a Lua table.
func getLuaTableKeys(tbl *lua.LTable) []string {
	var keys []string
	tbl.ForEach(func(k lua.LValue, v lua.LValue) {
		if str, ok := k.(lua.LString); ok {
			keys = append(keys, string(str))
		}
	})
	return keys
}

// GraphQL request/response types
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]any         `json:"variables,omitempty"`
	OperationName string                 `json:"operationName,omitempty"`
}

type GraphQLResponse struct {
	Data   interface{}         `json:"data,omitempty"`
	Errors []gqlerror.Error    `json:"errors,omitempty"`
}

// headerToMap converts http.Header to map[string]string by taking the first value.
func headerToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// GraphQLRequestHandler handles /graphql endpoint requests.
func (s *GraphQLServer) GraphQLRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GraphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Build context with optional per-request context
	ctx := r.Context()
	if s.contextFn != nil {
		httpReq := &HTTPRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			Headers:     headerToMap(r.Header),
			Body:        "",
			RemoteAddr:  r.RemoteAddr,
			TLS:         r.TLS != nil,
		}
		if ctxData := s.contextFn(httpReq); ctxData != nil {
			ctx = context.WithValue(ctx, "graphqlContext", ctxData)
		}
	}

	resp := s.Execute(ctx, &GraphQLRequest{
		Query:         req.Query,
		Variables:     req.Variables,
		OperationName: req.OperationName,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GraphQLStatsHandler returns the stats for k.graphql_stats()
func (s *GraphQLServer) GraphQLStatsHandler() GraphQLStats {
	return s.GetStats()
}