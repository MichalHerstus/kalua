//go:build !wasm

package server

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
)

// SOAPServer handles SOAP request processing for serve mode.
type SOAPServer struct {
	mu         sync.RWMutex
	operations map[string]*soapOperation
	stats      SOAPStats
	L          *lua.LState
}

// SOAPStats tracks SOAP request metrics.
type SOAPStats struct {
	RequestsTotal int64
	FaultsTotal   int64
	LatencySumMs  int64
	LatencyCount  int64
}

// soapOperation represents a SOAP operation mapping.
type soapOperation struct {
	actionName string
	handlerFn  string
}

// SOAP envelope structures for parsing
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Header  *soapHeader
	Body    soapBody
}

type soapHeader struct {
	XMLName xml.Name `xml:"Header"`
	Items   []xml.Token `xml:",any"`
}

type soapBody struct {
	XMLName xml.Name `xml:"Body"`
	Items   []xml.Token `xml:",any"`
}

// soapFault represents a SOAP fault response
type soapFault struct {
	XMLName   xml.Name `xml:"Fault"`
	FaultCode   string `xml:"faultcode"`
	FaultString string `xml:"faultstring"`
	Detail      string `xml:"detail,omitempty"`
}

type soapResponseEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapResponseBody
}

type soapResponseBody struct {
	XMLName xml.Name `xml:"Body"`
	Content any      `xml:",any"`
}

// newSOAPServer creates a new SOAP server instance.
func newSOAPServer() *SOAPServer {
	return &SOAPServer{
		operations: make(map[string]*soapOperation),
	}
}

// BuildOperations reads k.soap_operations table and registers operations.
func (s *SOAPServer) BuildOperations(L *lua.LState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	opsTable := L.GetGlobal("k").(*lua.LTable).RawGetString("soap_operations")
	if opsTable == lua.LNil {
		return nil // No operations defined
	}

	tbl, ok := opsTable.(*lua.LTable)
	if !ok {
		return errors.New("k.soap_operations must be a table")
	}

	tbl.ForEach(func(key, value lua.LValue) {
		actionName := key.String()
		if fn, ok := value.(lua.LString); ok {
			s.operations[actionName] = &soapOperation{
				actionName: actionName,
				handlerFn:  string(fn),
			}
		}
	})

	return nil
}

// Execute processes a SOAP request and returns the response.
func (s *SOAPServer) Execute(ctx context.Context, req *SOAPRequest) *SOAPResponse {
	startTime := time.Now()
	defer func() {
		latency := time.Since(startTime).Milliseconds()
		s.mu.Lock()
		s.stats.LatencySumMs += latency
		s.stats.LatencyCount++
		s.mu.Unlock()
	}()

	s.mu.Lock()
	s.stats.RequestsTotal++
	s.mu.Unlock()

	// Parse SOAP envelope
	env, err := parseSOAPEnvelope(req.Body)
	if err != nil {
		s.mu.Lock()
		s.stats.FaultsTotal++
		s.mu.Unlock()
		return &SOAPResponse{
			Fault: &soapFault{
				FaultCode:   "Client",
				FaultString: fmt.Sprintf("Invalid SOAP envelope: %v", err),
			},
		}
	}

	// Find the operation
	action := extractSOAPAction(req.HTTPHeaders, env)
	op, ok := s.operations[action]
	if !ok {
		s.mu.Lock()
		s.stats.FaultsTotal++
		s.mu.Unlock()
		return &SOAPResponse{
			Fault: &soapFault{
				FaultCode:   "Client",
				FaultString: fmt.Sprintf("Unknown operation: %s", action),
			},
		}
	}

	// Parse body content
	bodyContent, err := parseSOAPBody(env.Body)
	if err != nil {
		s.mu.Lock()
		s.stats.FaultsTotal++
		s.mu.Unlock()
		return &SOAPResponse{
			Fault: &soapFault{
				FaultCode:   "Client",
				FaultString: fmt.Sprintf("Invalid SOAP body: %v", err),
			},
		}
	}

	// Parse headers
	headers := make(map[string]any)
	if env.Header != nil {
		for _, item := range env.Header.Items {
			_ = item // suppress unused warning
			// Simplified header parsing
		}
	}

	// Call Lua handler using stored L state
	result, err := s.callLuaSOAPHandler(op.handlerFn, bodyContent, map[string]any{
		"headers": headers,
	})
	if err != nil {
		s.mu.Lock()
		s.stats.FaultsTotal++
		s.mu.Unlock()
		return &SOAPResponse{
			Fault: &soapFault{
				FaultCode:   "Server",
				FaultString: err.Error(),
			},
		}
	}

	// Build response envelope
	respEnv := buildSOAPResponse(result)
	xmlBytes, err := xml.MarshalIndent(respEnv, "", "  ")
	if err != nil {
		return &SOAPResponse{
			Fault: &soapFault{
				FaultCode:   "Server",
				FaultString: "Failed to marshal response",
			},
		}
	}

	return &SOAPResponse{
		XML:     xmlBytes,
		Content: result,
	}
}

// GetStats returns the current SOAP statistics.
func (s *SOAPServer) GetStats() SOAPStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// SOAP request/response types
type SOAPRequest struct {
	Body        string
	HTTPHeaders http.Header
}

type SOAPResponse struct {
	XML      []byte
	Content  any
	Fault    *soapFault
}

// parseSOAPEnvelope parses a SOAP envelope from XML bytes.
func parseSOAPEnvelope(data string) (*soapEnvelope, error) {
	decoder := xml.NewDecoder(strings.NewReader(data))
	decoder.Strict = false

	var env soapEnvelope
	if err := decoder.Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}

// extractSOAPAction extracts the SOAPAction from HTTP headers or envelope.
func extractSOAPAction(headers http.Header, env *soapEnvelope) string {
	// Check HTTP SOAPAction header
	if action := headers.Get("SOAPAction"); action != "" {
		// Remove quotes if present
		action = strings.Trim(action, `"`)
		return action
	}

	// Try to get action from first body element
	if len(env.Body.Items) > 0 {
		// Simplified: assume first element name is action
		return "unknown"
	}

	return "unknown"
}

// parseSOAPBody extracts the body content as a map.
func parseSOAPBody(body soapBody) (map[string]any, error) {
	result := make(map[string]any)

	for _, item := range body.Items {
		if se, ok := item.(xml.StartElement); ok {
			// Simple approach: re-marshal the element and unmarshal
			if data, err := xml.Marshal(se); err == nil {
				var val map[string]any
				if err := xml.Unmarshal(data, &val); err == nil {
					result[se.Name.Local] = val
				}
			}
		}
	}

	return result, nil
}

// callLuaSOAPHandler calls the Lua handler function using the stored L state.
func (s *SOAPServer) callLuaSOAPHandler(fnName string, body map[string]any, context map[string]any) (any, error) {
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
	for k, v := range body {
		argsTbl.RawSetString(k, toLuaValue(L, v))
	}
	L.Push(argsTbl)

	// Push context as table
	ctxTbl := L.NewTable()
	for k, v := range context {
		ctxTbl.RawSetString(k, toLuaValue(L, v))
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

	// Return the first result
	ret := L.Get(n)
	L.SetTop(n - 1)

	return luaValueToGo(ret), nil
}

// toLuaValue is defined in graphql.go
// // toLuaValue is defined in graphql.go

// luaValueToGo and luaTableToGo are defined in graphql.go


// buildSOAPResponse builds a SOAP response envelope from result.
func buildSOAPResponse(result any) *soapResponseEnvelope {
	return &soapResponseEnvelope{
		Body: soapResponseBody{Content: result},
	}
}

// SOAPRequestHandler handles /soap endpoint requests.
func (s *SOAPServer) SOAPRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	resp := s.Execute(r.Context(), &SOAPRequest{
		Body:        string(body),
		HTTPHeaders: r.Header,
	})

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	if resp.Fault != nil {
		w.WriteHeader(http.StatusInternalServerError)
		faultEnv := buildSOAPFaultResponse(resp.Fault)
		xml.NewEncoder(w).Encode(faultEnv)
		return
	}

	w.Write(resp.XML)
}

// buildSOAPFaultResponse builds a fault response envelope.
func buildSOAPFaultResponse(fault *soapFault) *soapResponseEnvelope {
	return &soapResponseEnvelope{
		Body: soapResponseBody{Content: fault},
	}
}

// SOAPStatsHandler returns the stats for k.soap_stats()
func (s *SOAPServer) SOAPStatsHandler() SOAPStats {
	return s.GetStats()
}