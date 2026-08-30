// Package bindings implements the §5.4 Web Service Run binding
// (k.webservice_run): a minimal SOAP 1.1/1.2 client. The profile table names
// the endpoint and operation; params become the XML payload of the body
// element. Returns {status, headers, body}.
package bindings

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yuin/gopher-lua"
)

// registerSoap installs k.webservice_run.
func registerSoap(e *Env) {
	// k.webservice_run(profile, params) -> {status, headers, body}
	// profile: {url, action, method, timeout_ms}; params: table → fields.
	e.register("webservice_run", "comm", func(L *lua.LState) int {
		profile := L.CheckTable(1)
		params := L.OptTable(2, L.NewTable())

		url := profile.RawGetString("url").String()
		action := profile.RawGetString("action").String()
		method := profile.RawGetString("method").String()
		if method == "" {
			method = "POST"
		}
		timeoutMs := profile.RawGetString("timeout_ms")
		timeout := 30 * time.Second
		if timeoutMs != lua.LNil {
			if n, ok := v(timeoutMs).(float64); ok && n > 0 {
				timeout = time.Duration(n) * time.Millisecond
			}
		}
		if url == "" || action == "" {
			L.RaiseError("webservice_run: profile requires url and action")
			return 0
		}

		return runBlocking(e, L, func() (interface{}, error) {
			envelope := soapEnvelope(action, params, L)
			client := &http.Client{Timeout: timeout}
			req, err := http.NewRequest(method, url, bytes.NewReader(envelope))
			if err != nil {
				return nil, fmt.Errorf("webservice error: %v", err)
			}
			req.Header.Set("Content-Type", "text/xml; charset=utf-8")
			req.Header.Set("SOAPAction", `"`+action+`"`)
			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("webservice error: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			headers := map[string]string{}
			for k, vs := range resp.Header {
				if len(vs) > 0 {
					headers[k] = vs[0]
				}
			}
			return map[string]interface{}{
				"status":  resp.StatusCode,
				"headers": headers,
				"body":    string(body),
			}, nil
		}, nil)
	})
}

// soapEnvelope builds a SOAP 1.1 envelope with the given target action as the
// body element name and params serialized as child elements.
func soapEnvelope(action string, params *lua.LTable, L *lua.LState) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`)
	sb.WriteString(`<soap:Body>`)
	sb.WriteString(`<` + soapName(action) + ` xmlns="http://tempuri.org/">`)
	writeSoapParams(&sb, params, L)
	sb.WriteString(`</` + soapName(action) + `>`)
	sb.WriteString(`</soap:Body></soap:Envelope>`)
	return []byte(sb.String())
}

// soapName keeps the leaf (non-namespaced) part of an action as the element.
func soapName(action string) string {
	if i := strings.LastIndexByte(action, '/'); i >= 0 {
		action = action[i+1:]
	}
	if i := strings.LastIndexByte(action, '#'); i >= 0 {
		action = action[i+1:]
	}
	r := strings.NewReplacer("(", "", ")", "", ".", "_", " ", "_")
	return r.Replace(action)
}

// writeSoapParams renders a Lua table as SOAP child elements. Numeric values
// become bare numbers; booleans true/false; other scalars as text; nested
// tables recurse.
func writeSoapParams(sb *strings.Builder, t *lua.LTable, L *lua.LState) {
	t.ForEach(func(k, val lua.LValue) {
		name := k.String()
		switch v := val.(type) {
		case *lua.LTable:
			sb.WriteString(`<` + name + `>`)
			writeSoapParams(sb, v, L)
			sb.WriteString(`</` + name + `>`)
		default:
			sb.WriteString(`<` + name + `>`)
			sb.WriteString(xmlEscape(val.String()))
			sb.WriteString(`</` + name + `>`)
		}
	})
}