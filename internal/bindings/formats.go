// Package bindings implements the §5.10 data-format actions beyond JSON:
// CSV, INI, YAML and XML document load/save/parse/string. Every format follows
// the shared convention (k.<fmt>_load / _save / _parse / _string) with a
// 16 MiB file cap and atomic saves (see json.go for the JSON half).
//
// Element→table convention shared by all XML bindings (spec §5.10):
//
//	{ _name="tag", _attrs={name="value",...}, _children={ element,... }, _text="..." }
//
// xml_load/xml_save and the §5.4 getter family both expose this shape via
// nodeToTable / tableToNode.
package bindings

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"

	"kalua/internal/common"
)

// stripBOM removes a UTF-8 BOM if present.
func stripBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// readerForFile is a shared body for k.<fmt>_load: resolve+read bounded input
// and hand the bytes to a decode func. Runs on the session worker (async).
func loadVia(e *Env, L *lua.LState, path string, opts *lua.LTable, decode func([]byte) (interface{}, error), conv func(*lua.LState, interface{}) lua.LValue) int {
	return runBlocking(e, L, func() (interface{}, error) {
		resolved, err := e.resolvePath(path)
		if err != nil {
			return nil, err
		}
		fi, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("file error: cannot stat %s: %w", path, err)
		}
		if fi.Size() > e.maxFileSize {
			return nil, fmt.Errorf("file error: %s exceeds max file size (%d bytes)", path, e.maxFileSize)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, fmt.Errorf("file error: cannot read %s: %w", path, err)
		}
		return decode(stripBOM(data))
	}, func(L *lua.LState, v interface{}) lua.LValue {
		if conv != nil {
			return conv(L, v)
		}
		// Some decoders build Lua tables on the caller's state and return them
		// as interface{}; pass those through untouched instead of re-converting.
		if lv, ok := v.(lua.LValue); ok {
			return lv
		}
		return common.DefaultConv(L, v)
	})
}

// saveVia is the async body for k.<fmt>_save: serialize on the caller (never
// touches Lua from the worker), then atomically write.
func saveVia(e *Env, L *lua.LState, path string, text string) int {
	data := []byte(text)
	return runBlocking(e, L, func() (interface{}, error) {
		resolved, err := e.resolvePath(path)
		if err != nil {
			return nil, err
		}
		if err := writeFileAtomic(resolved, data); err != nil {
			return nil, fmt.Errorf("file error: cannot write %s: %w", path, err)
		}
		return nil, nil
	}, nil)
}

// registerFormats installs the §5.10 CSV / INI / YAML / XML document actions.
func registerFormats(e *Env) {
	registerCSV(e)
	registerINI(e)
	registerYAML(e)
	registerXMLFormats(e)
}

// -------- CSV --------

// registerCSV installs k.csv_parse/string/load/save plus table conversions.
func registerCSV(e *Env) {
	// k.csv_parse(text[, opts]) -> list of row-arrays, or row-maps when header=true
	e.register("csv_parse", "formats", func(L *lua.LState) int {
		text := L.CheckString(1)
		opts := L.OptTable(2, L.NewTable())
		tbl, err := parseCSV(L, e, []byte(text), opts)
		if err != nil {
			L.RaiseError("csv error: %v", err)
			return 0
		}
		L.Push(tbl)
		return 1
	})

	// k.csv_string(data[, opts]) -> CSV text
	e.register("csv_string", "formats", func(L *lua.LState) int {
		data := L.CheckTable(1)
		opts := L.OptTable(2, L.NewTable())
		text, err := csvString(e, data, opts)
		if err != nil {
			L.RaiseError("csv error: %v", err)
			return 0
		}
		L.Push(lua.LString(text))
		return 1
	})

	// k.csv_load(path[, opts]) (async)
	e.register("csv_load", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		opts := L.OptTable(2, L.NewTable())
		return loadVia(e, L, path, opts, func(data []byte) (interface{}, error) {
			return parseCSV(L, e, data, opts) // caller-side: never from worker
		}, nil)
	})

	// k.csv_save(path, data[, opts]) (async)
	e.register("csv_save", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		data := L.CheckTable(2)
		opts := L.OptTable(3, L.NewTable())
		text, err := csvString(e, data, opts)
		if err != nil {
			L.RaiseError("csv error: %v", err)
			return 0
		}
		return saveVia(e, L, path, text)
	})
}

// csvOpts pulls header/sep/quote from an opts table.
func csvOpts(opts *lua.LTable) (header bool, sep rune, quote rune) {
	header = false
	sep = ','
	quote = '"'
	if opts == nil {
		return
	}
	if v := opts.RawGetString("header"); v != lua.LNil {
		header = lua.LVAsBool(v)
	}
	if v := opts.RawGetString("sep"); v != lua.LNil && v.Type() == lua.LTString {
		s := v.String()
		if len(s) > 0 {
			sep = rune(s[0])
		}
	}
	if v := opts.RawGetString("quote"); v != lua.LNil && v.Type() == lua.LTString {
		s := v.String()
		if len(s) > 0 {
			quote = rune(s[0])
		}
	}
	return
}

// parseCSV decodes CSV bytes into a Lua table. header=true yields a list of
// row-maps keyed by the first row; otherwise a list of row-arrays. All cells
// are strings (the documented mapping; coerce for numbers).
func parseCSV(L *lua.LState, e *Env, data []byte, opts *lua.LTable) (*lua.LTable, error) {
	header, sep, quote := csvOpts(opts)
	r := csv.NewReader(bytes.NewReader(stripBOM(data)))
	r.Comma = sep
	r.LazyQuotes = true
	if quote != '"' && quote != 0 {
		r.LazyQuotes = true
	}
	raw, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	out := L.NewTable()
	if len(raw) == 0 {
		return out, nil
	}
	colNames := make([]string, 0)
	if header {
		for _, c := range raw[0] {
			colNames = append(colNames, c)
		}
		raw = raw[1:]
	}
	for _, row := range raw {
		rowTbl := L.NewTable()
		if header {
			for i, cell := range row {
				name := ""
				if i < len(colNames) {
					name = colNames[i]
				} else {
					name = strconv.Itoa(i + 1)
				}
				rowTbl.RawSetString(name, lua.LString(cell))
			}
		} else {
			for i, cell := range row {
				rowTbl.RawSetInt(i+1, lua.LString(cell))
			}
		}
		out.RawSetInt(out.Len()+1, rowTbl)
	}
	return out, nil
}

// csvString serializes a Lua table to CSV. With header=true, the first row
// (a table of column names) names the output columns and each following row is
// a field→map lookup; otherwise every row is a sequence.
func csvString(e *Env, data *lua.LTable, opts *lua.LTable) (string, error) {
	header, sep, quote := csvOpts(opts)

	var rows []string
	var colNames []string
	if header {
		first := data.RawGetInt(1)
		if firstTbl, ok := first.(*lua.LTable); ok {
			names := tableStringKeys(firstTbl)
			colNames = names
			headerLine := make([]string, len(colNames))
			for i, n := range colNames {
				headerLine[i] = n
			}
			rows = append(rows, encodeCSVRow(headerLine, sep, quote))
		}
	}
	n := data.Len()
	start := 1
	if header {
		start = 2
	}
	for i := start; i <= n; i++ {
		row := data.RawGetInt(i)
		rowTbl, ok := row.(*lua.LTable)
		if !ok {
			continue
		}
		var rowCells []string
		if header && len(colNames) > 0 {
			for _, c := range colNames {
				rowCells = append(rowCells, rowTbl.RawGetString(c).String())
			}
		} else {
			keys := make([]int, 0)
			rowTbl.ForEach(func(k, _ lua.LValue) {
				if num, ok := k.(lua.LNumber); ok {
					keys = append(keys, int(num))
				}
			})
			sort.Ints(keys)
			for _, k := range keys {
				rowCells = append(rowCells, rowTbl.RawGetInt(k).String())
			}
		}
		rows = append(rows, encodeCSVRow(rowCells, sep, quote))
	}
	return strings.Join(rows, "\n") + "\n", nil
}

func encodeCSVRow(cells []string, sep, quote rune) string {
	var sb strings.Builder
	for i, c := range cells {
		if i > 0 {
			sb.WriteRune(sep)
		}
		needsQuote := strings.ContainsRune(c, sep) || strings.ContainsRune(c, quote) ||
			strings.ContainsAny(c, "\r\n")
		if needsQuote {
			sb.WriteRune(quote)
			sb.WriteString(strings.ReplaceAll(c, string(quote), string(quote)+string(quote)))
			sb.WriteRune(quote)
		} else {
			sb.WriteString(c)
		}
	}
	return sb.String()
}

var _ = []lua.LValue{nil}

// -------- INI --------

// registerINI installs k.ini_parse/string/load/save and the single-key
// k.ini_read / k.ini_write helpers (spec §5.5).
func registerINI(e *Env) {
	e.register("ini_parse", "formats", func(L *lua.LState) int {
		text := L.CheckString(1)
		tbl, err := parseINI(L, []byte(text))
		if err != nil {
			L.RaiseError("ini error: %v", err)
			return 0
		}
		L.Push(tbl)
		return 1
	})

	e.register("ini_string", "formats", func(L *lua.LState) int {
		data := L.CheckTable(1)
		text, err := iniString(e, data)
		if err != nil {
			L.RaiseError("ini error: %v", err)
			return 0
		}
		L.Push(lua.LString(text))
		return 1
	})

	e.register("ini_load", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		return loadVia(e, L, path, nil, func(data []byte) (interface{}, error) {
			return parseINI(L, data)
		}, nil)
	})

	e.register("ini_save", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		data := L.CheckTable(2)
		text, err := iniString(e, data)
		if err != nil {
			L.RaiseError("ini error: %v", err)
			return 0
		}
		return saveVia(e, L, path, text)
	})

	// k.ini_read(path, section, key) -> string (Kalipso parity, §5.5)
	e.register("ini_read", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		section := L.CheckString(2)
		key := L.CheckString(3)
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(resolved)
			if err != nil {
				return nil, fmt.Errorf("file error: cannot read %s: %w", path, err)
			}
			tbl, err := parseINI(L, stripBOM(data))
			if err != nil {
				return nil, err
			}
			v := tbl.RawGetString(section)
			if v == lua.LNil {
				return "", nil
			}
			secTbl, ok := v.(*lua.LTable)
			if !ok {
				return "", nil
			}
			val := secTbl.RawGetString(key)
			if val == lua.LNil {
				return "", nil
			}
			return val.String(), nil
		}, nil)
	})

	// k.ini_write(path, section, key, value) (async, atomic)
	e.register("ini_write", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		section := L.CheckString(2)
		key := L.CheckString(3)
		value := luaToString(L, 4)
		return runBlocking(e, L, func() (interface{}, error) {
			resolved, err := e.resolvePath(path)
			if err != nil {
				return nil, err
			}
			root := L.NewTable()
			if data, err := os.ReadFile(resolved); err == nil {
				root, _ = parseINI(L, stripBOM(data))
			}
			v := root.RawGetString(section)
			secTbl, ok := v.(*lua.LTable)
			if !ok {
				secTbl = L.NewTable()
				root.RawSetString(section, secTbl)
			}
			secTbl.RawSetString(key, lua.LString(value))
			text, err := iniString(e, root)
			if err != nil {
				return nil, err
			}
			if err := writeFileAtomic(resolved, []byte(text)); err != nil {
				return nil, fmt.Errorf("file error: cannot write %s: %w", path, err)
			}
			return nil, nil
		}, nil)
	})
}

// parseINI parses INI text into {section={key=value,...}, _root={key=value}}.
// Keys are case-preserved; values are strings.
func parseINI(L *lua.LState, data []byte) (*lua.LTable, error) {
	root := L.NewTable()
	rootTbl := L.NewTable()
	root.RawSetString("_root", rootTbl)

	current := rootTbl
	lines := strings.Split(string(stripBOM(data)), "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec := strings.TrimSpace(line[1 : len(line)-1])
			secTbl := L.NewTable()
			root.RawSetString(sec, secTbl)
			current = secTbl
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		current.RawSetString(k, lua.LString(v))
	}
	return root, nil
}

// iniString serializes a Lua table as INI. Section tables are emitted under
// [section]; "_root" holds the leading no-section keys.
func iniString(e *Env, data *lua.LTable) (string, error) {
	var sb strings.Builder

	writePairs := func(tbl *lua.LTable) {
		keys := tableStringKeys(tbl)
		sort.Strings(keys)
		for _, k := range keys {
			v := tbl.RawGetString(k)
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(strings.ReplaceAll(v.String(), "\n", "\\n"))
			sb.WriteString("\n")
		}
	}

	// root (no-section) keys first
	if v := data.RawGetString("_root"); v != lua.LNil {
		if rootTbl, ok := v.(*lua.LTable); ok {
			writePairs(rootTbl)
		}
	}

	secKeys := tableStringKeys(data)
	sort.Strings(secKeys)
	for _, sec := range secKeys {
		if sec == "_root" {
			continue
		}
		v := data.RawGetString(sec)
		secTbl, ok := v.(*lua.LTable)
		if !ok {
			continue
		}
		sb.WriteString("[")
		sb.WriteString(sec)
		sb.WriteString("]\n")
		writePairs(secTbl)
	}
	return sb.String(), nil
}

// -------- YAML --------

// registerYAML installs k.yaml_parse/string/load/save. Value mapping matches
// JSON: object→table, array→sequence, null→K.NULL.
func registerYAML(e *Env) {
	e.register("yaml_parse", "formats", func(L *lua.LState) int {
		text := L.CheckString(1)
		v, err := parseYAML(L, e, []byte(text))
		if err != nil {
			L.RaiseError("yaml error: %v", err)
			return 0
		}
		if arr, ok := v.([]interface{}); ok && isYAMLDocs(arr) {
			out := L.NewTable()
			for i, item := range arr {
				out.RawSetInt(i+1, convertJSONGo(L, e, item))
			}
			L.Push(out)
			return 1
		}
		L.Push(convertJSONGo(L, e, v))
		return 1
	})

	e.register("yaml_string", "formats", func(L *lua.LState) int {
		text, err := yamlString(e, L.Get(1))
		if err != nil {
			L.RaiseError("yaml error: %v", err)
			return 0
		}
		L.Push(lua.LString(text))
		return 1
	})

	e.register("yaml_load", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		return loadVia(e, L, path, nil, func(data []byte) (interface{}, error) {
			return parseYAML(L, e, data)
		}, func(L *lua.LState, v interface{}) lua.LValue {
			if arr, ok := v.([]interface{}); ok && isYAMLDocs(arr) {
				out := L.NewTable()
				for i, item := range arr {
					out.RawSetInt(i+1, convertJSONGo(L, e, item))
				}
				return out
			}
			return convertJSONGo(L, e, v)
		})
	})

	e.register("yaml_save", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		text, err := yamlString(e, L.Get(2))
		if err != nil {
			L.RaiseError("yaml error: %v", err)
			return 0
		}
		return saveVia(e, L, path, text)
	})
}

// isYAMLDocs reports whether a YAML multi-document decode produced a list of
// documents (heuristic: every element is a map or non-scalar).
func isYAMLDocs(arr []interface{}) bool {
	if len(arr) < 1 {
		return false
	}
	allMaps := true
	for _, item := range arr {
		if _, ok := item.(map[string]interface{}); !ok {
			allMaps = false
			break
		}
	}
	return allMaps
}

// parseYAML decodes YAML into a Go tree (maps/arrays/scalars). Multi-document
// input returns a []interface{} of documents.
func parseYAML(L *lua.LState, e *Env, data []byte) (interface{}, error) {
	dec := yaml.NewDecoder(bytes.NewReader(stripBOM(data)))
	var docs []interface{}
	for {
		var doc interface{}
		err := dec.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		docs = append(docs, normalizeYAML(doc))
	}
	switch len(docs) {
	case 0:
		return nil, nil
	case 1:
		return docs[0], nil
	default:
		return docs, nil
	}
}

// normalizeYAML converts typed YAML maps into map[string]interface{} and
// normalizes time values to RFC3339 strings (yaml timestamps cannot round-trip
// through JSON's convertJSONGo otherwise).
func normalizeYAML(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = normalizeYAML(val)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[fmt.Sprintf("%v", k)] = normalizeYAML(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = normalizeYAML(val)
		}
		return out
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return v
	}
}

// yamlString converts a Lua value tree to YAML via JSON-like mapping.
func yamlString(e *Env, v lua.LValue) (string, error) {
	goVal, err := luaTreeToGo(e, v, map[*lua.LTable]bool{})
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(goVal)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// luaTreeToGo converts a Lua value tree to a plain Go tree, mirroring the
// JSON value-mapping (see json.go) but for arbitrary tables.
func luaTreeToGo(e *Env, v lua.LValue, seen map[*lua.LTable]bool) (interface{}, error) {
	switch lv := v.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(lv), nil
	case lua.LNumber:
		return float64(lv), nil
	case lua.LString:
		return string(lv), nil
	case *lua.LTable:
		if lv == e.kNULL {
			return nil, nil
		}
		if lv == e.kNULL || seen[lv] {
			return nil, fmt.Errorf("circular reference")
		}
		seen[lv] = true
		defer delete(seen, lv)

		if isArrayTable(lv) {
			n := lv.Len()
			out := make([]interface{}, 0, n)
			for i := 1; i <= n; i++ {
				item, err := luaTreeToGo(e, lv.RawGetInt(i), seen)
				if err != nil {
					return nil, err
				}
				out = append(out, item)
			}
			return out, nil
		}
		keys := tableStringKeys(lv)
		sort.Strings(keys)
		out := make(map[string]interface{}, len(keys))
		for _, k := range keys {
			item, err := luaTreeToGo(e, lv.RawGetString(k), seen)
			if err != nil {
				return nil, err
			}
			out[k] = item
		}
		return out, nil
	default:
		return lv.String(), nil
	}
}

// -------- XML (document load/save) --------

// registerXMLFormats installs k.xml_load / k.xml_save alongside the §5.4
// getter family (xml_parse + xml_root/child/... in xml.go).
func registerXMLFormats(e *Env) {
	// k.xml_load(path) -> document root table {_name,_attrs,_children,_text}
	e.register("xml_load", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		return loadVia(e, L, path, nil, func(data []byte) (interface{}, error) {
			root, err := parseXML(string(stripBOM(data)))
			if err != nil {
				return nil, err
			}
			return root, nil
		}, func(L *lua.LState, v interface{}) lua.LValue {
			node, ok := v.(*xmlNode)
			if !ok || node == nil {
				return lua.LNil
			}
			return nodeTable(L, node)
		})
	})

	// k.xml_save(path, table) (async; table shape = nodeTable convention)
	e.register("xml_save", "formats", func(L *lua.LState) int {
		path := L.CheckString(1)
		data := L.CheckTable(2)
		var sb strings.Builder
		if err := writeXMLNode(&sb, data, 0); err != nil {
			L.RaiseError("xml_save error: %v", err)
			return 0
		}
		return saveVia(e, L, path, sb.String())
	})
}

// nodeTable serializes an xmlNode as the documented element table shape.
func nodeTable(L *lua.LState, node *xmlNode) *lua.LTable {
	tbl := L.NewTable()
	tbl.RawSetString("_name", lua.LString(node.Name))
	if len(node.Attrs) > 0 {
		attrTbl := L.NewTable()
		for k, v := range node.Attrs {
			attrTbl.RawSetString(k, lua.LString(v))
		}
		tbl.RawSetString("_attrs", attrTbl)
	}
	if len(node.Children) > 0 {
		childrenTbl := L.NewTable()
		for i, ch := range node.Children {
			childrenTbl.RawSetInt(i+1, nodeTable(L, ch))
		}
		tbl.RawSetString("_children", childrenTbl)
	}
	if node.Content != "" {
		tbl.RawSetString("_text", lua.LString(node.Content))
	}
	return tbl
}

// writeXMLNode renders an element table as XML. Accepts the documented shape
// (string key "_name"/"_attrs"/"_children"/"_text") and render helpers.
func writeXMLNode(sb *strings.Builder, node *lua.LTable, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("nesting too deep")
	}
	name := node.RawGetString("_name").String()
	if name == "" {
		// Fall back to any string key as tag name.
		keys := tableStringKeys(node)
		for _, k := range keys {
			if k != "_attrs" && k != "_children" && k != "_text" && k != "_name" {
				name = k
				break
			}
		}
	}
	if name == "" {
		return fmt.Errorf("xml_save: element has no name")
	}
	indent := strings.Repeat("  ", depth)
	sb.WriteString(indent)
	sb.WriteString("<")
	sb.WriteString(xmlEscape(name))

	text := node.RawGetString("_text").String()
	if attrs := node.RawGetString("_attrs"); attrs != lua.LNil {
		if attrTbl, ok := attrs.(*lua.LTable); ok {
			for _, k := range tableStringKeys(attrTbl) {
				sb.WriteString(" ")
				sb.WriteString(xmlEscape(k))
				sb.WriteString(`="`)
				sb.WriteString(xmlAttrEscape(attrTbl.RawGetString(k).String()))
				sb.WriteString(`"`)
			}
		}
	}

	children := node.RawGetString("_children")
	var childTbls []*lua.LTable
	if children != lua.LNil {
		if cTbl, ok := children.(*lua.LTable); ok {
			n := cTbl.Len()
			for i := 1; i <= n; i++ {
				if ct, ok := cTbl.RawGetInt(i).(*lua.LTable); ok {
					childTbls = append(childTbls, ct)
				}
			}
		}
	}

	if len(childTbls) == 0 && text == "" {
		sb.WriteString("/>\n")
		return nil
	}
	sb.WriteString(">")
	if len(childTbls) == 0 {
		sb.WriteString(xmlEscape(text))
		sb.WriteString("</")
		sb.WriteString(xmlEscape(name))
		sb.WriteString(">\n")
		return nil
	}
	if text != "" {
		sb.WriteString(xmlEscape(text))
		sb.WriteString("\n")
	}
	for _, ch := range childTbls {
		if err := writeXMLNode(sb, ch, depth+1); err != nil {
			return err
		}
	}
	sb.WriteString(indent)
	sb.WriteString("</")
	sb.WriteString(xmlEscape(name))
	sb.WriteString(">\n")
	return nil
}

// xmlAttrEscape escapes attribute text (quotes and ampersands) only.
func xmlAttrEscape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			sb.WriteString("&amp;")
		case '"':
			sb.WriteString("&quot;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}

// tableStringKeys returns the sorted string keys of a Lua table (numeric keys
// are stringified). Used for deterministic INI and CSV output.
func tableStringKeys(t *lua.LTable) []string {
	var keys []string
	t.ForEach(func(k, _ lua.LValue) {
		if s, ok := k.(lua.LString); ok {
			keys = append(keys, string(s))
		} else if n, ok := k.(lua.LNumber); ok {
			keys = append(keys, strconv.Itoa(int(n)))
		}
	})
	sort.Strings(keys)
	return keys
}