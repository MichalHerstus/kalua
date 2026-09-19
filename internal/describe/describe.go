// Package describe extracts a high-level structural overview of a KALUA
// script (entry points, forms, and API surface) from its AST, so agents can
// orient without reading the whole file. It is shared between the `KALUA
// describe` CLI command and the MCP tools.
package describe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"

	"kalua/internal/bindings"
)

// Result is the machine-readable `describe` output.
type Result struct {
	OK         bool            `json:"ok"`
	Entry      string          `json:"entry"`    // "run" | "serve" | ""
	Main       bool            `json:"main"`     // has main()
	Handlers   []string        `json:"handlers"` // init/shutdown/handle_* present
	Forms      []Form          `json:"forms,omitempty"`
	KUsage     map[string]int  `json:"k_usage,omitempty"` // known k.path → call count
	KCallCount int             `json:"k_calls"`           // total k.* call expressions
	Lines      int             `json:"lines"`             // source line count
	Statements int             `json:"statements"`        // top-level statement count
	Toplevel   []string        `json:"toplevel_functions,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// Form is one k.form.* call that targets a specific form.
type Form struct {
	Op    string `json:"op"` // "new" | "show"
	Form  string `json:"form,omitempty"`
	Title string `json:"title,omitempty"`
}

type walker struct {
	res    *Result
	search map[string][]string // known k.a.b → [a b]
}

// Scan walks the parsed AST of src and fills in structural facts.
func Scan(src, name string, res *Result) {
	stmts, err := parse.Parse(strings.NewReader(src), name)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return
	}
	res.Statements = len(stmts)

	// Build the search index: every known dotted binding maps its full path
	// (k.form.new) to its segments so we can recognize and count API calls.
	w := &walker{res: res}
	w.search = map[string][]string{}
	for full := range bindings.Known() {
		parts := strings.Split(full, ".")
		if len(parts) >= 1 && parts[0] != "" {
			w.search["k."+full] = append([]string{"k"}, parts...)
		}
	}
	w.walkStmts(stmts, true)
	sort.Strings(res.Toplevel)
	sort.Strings(res.Handlers)
}

func (w *walker) walkStmts(stmts []ast.Stmt, top bool) {
	for _, s := range stmts {
		w.walkStmt(s, top)
	}
}

func (w *walker) walkStmt(s ast.Stmt, top bool) {
	switch n := s.(type) {
	case *ast.FuncDefStmt:
		fn := funcDefName(n)
		if fn != "" {
			if top {
				w.res.Toplevel = append(w.res.Toplevel, fn)
			}
			switch fn {
			case "init", "handle_http", "handle_ws", "handle_tcp", "shutdown":
				w.res.Handlers = append(w.res.Handlers, fn)
			}
			w.res.Main = w.res.Main || fn == "main"
		}
		if n.Func != nil {
			w.walkStmts(n.Func.Stmts, false)
		}
	case *ast.FuncCallStmt:
		w.walkExpr(n.Expr)
	case *ast.AssignStmt:
		for _, ex := range n.Rhs {
			w.walkExpr(ex)
		}
	case *ast.LocalAssignStmt:
		for _, ex := range n.Exprs {
			w.walkExpr(ex)
		}
	case *ast.ReturnStmt:
		for _, ex := range n.Exprs {
			w.walkExpr(ex)
		}
	case *ast.DoBlockStmt:
		w.walkStmts(n.Stmts, false)
	case *ast.WhileStmt:
		w.walkExpr(n.Condition)
		w.walkStmts(n.Stmts, false)
	case *ast.RepeatStmt:
		w.walkStmts(n.Stmts, false)
		w.walkExpr(n.Condition)
	case *ast.IfStmt:
		w.walkExpr(n.Condition)
		w.walkStmts(n.Then, false)
		if n.Else != nil {
			w.walkStmts(n.Else, false)
		}
	case *ast.NumberForStmt:
		w.walkExpr(n.Init)
		w.walkExpr(n.Limit)
		if n.Step != nil {
			w.walkExpr(n.Step)
		}
		w.walkStmts(n.Stmts, false)
	case *ast.GenericForStmt:
		for _, ex := range n.Exprs {
			w.walkExpr(ex)
		}
		w.walkStmts(n.Stmts, false)
	}
}

func (w *walker) walkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *ast.FuncCallExpr:
		// Recognize known k.* bindings by their identifier path.
		if path := identPath(n.Func); len(path) >= 2 && path[0] == "k" {
			if full := w.longestKnown(path); full != "" {
				w.res.KUsage[full]++
				w.res.KCallCount++
				w.trackFormCall(full, n)
			}
		}
		w.walkExpr(n.Func)
		for _, a := range n.Args {
			w.walkExpr(a)
		}
	case *ast.AttrGetExpr:
		w.walkExpr(n.Object)
		w.walkExpr(n.Key)
	case *ast.TableExpr:
		for _, f := range n.Fields {
			if f.Key != nil {
				w.walkExpr(f.Key)
			}
			w.walkExpr(f.Value)
		}
	case *ast.FunctionExpr:
		w.walkStmts(n.Stmts, false)
	case *ast.LogicalOpExpr:
		w.walkExpr(n.Lhs)
		w.walkExpr(n.Rhs)
	case *ast.RelationalOpExpr:
		w.walkExpr(n.Lhs)
		w.walkExpr(n.Rhs)
	case *ast.StringConcatOpExpr:
		w.walkExpr(n.Lhs)
		w.walkExpr(n.Rhs)
	case *ast.ArithmeticOpExpr:
		w.walkExpr(n.Lhs)
		w.walkExpr(n.Rhs)
	case *ast.UnaryMinusOpExpr:
		w.walkExpr(n.Expr)
	case *ast.UnaryNotOpExpr:
		w.walkExpr(n.Expr)
	case *ast.UnaryLenOpExpr:
		w.walkExpr(n.Expr)
	}
}

// longestKnown returns the longest known k.* binding that prefixes the given
// path, or "" when none matches. Counting the longest known prefix (e.g.
// k.form.new over k.form) keeps the usage map at the exact API boundary.
func (w *walker) longestKnown(path []string) string {
	if len(path) < 2 || path[0] != "k" {
		return ""
	}
	for i := len(path); i >= 2; i-- {
		cand := strings.Join(path[:i], ".")
		if _, ok := w.search[cand]; ok {
			return cand
		}
	}
	return ""
}

// trackFormCall records k.form.new/show calls with their literal form name.
func (w *walker) trackFormCall(full string, call *ast.FuncCallExpr) {
	parts := strings.Split(full, ".")
	if len(parts) < 3 || parts[1] != "form" {
		return
	}
	op := parts[2]
	if op != "new" && op != "show" {
		return
	}
	f := Form{Op: op}
	if len(call.Args) > 0 {
		if se, ok := call.Args[0].(*ast.StringExpr); ok {
			f.Form = se.Value
		}
	}
	if op == "new" && len(call.Args) > 1 {
		if opts, ok := call.Args[1].(*ast.TableExpr); ok {
			for _, fl := range opts.Fields {
				k := ""
				switch ke := fl.Key.(type) {
				case *ast.IdentExpr:
					k = ke.Value
				case *ast.StringExpr:
					k = ke.Value
				}
				if k == "title" {
					if ve, ok := fl.Value.(*ast.StringExpr); ok {
						f.Title = ve.Value
					}
				}
			}
		}
	}
	w.res.Forms = append(w.res.Forms, f)
}

// funcDefName resolves a FuncDefStmt's name (plain ident, or method).
func funcDefName(n *ast.FuncDefStmt) string {
	if n == nil || n.Name == nil {
		return ""
	}
	if n.Name.Func != nil {
		if id, ok := n.Name.Func.(*ast.IdentExpr); ok {
			return id.Value
		}
	}
	return n.Name.Method
}

// identPath resolves an expression to its dotted identifier path, e.g.
// k.form.new → ["k","form","new"]. Returns nil when the expression is not a
// pure identifier chain.
func identPath(e ast.Expr) []string {
	if id, ok := e.(*ast.IdentExpr); ok {
		return []string{id.Value}
	}
	a, ok := e.(*ast.AttrGetExpr)
	if !ok {
		return nil
	}
	var chain []string
	cur := a
	for {
		var key string
		switch k := cur.Key.(type) {
		case *ast.IdentExpr:
			key = k.Value
		case *ast.StringExpr:
			key = k.Value
		default:
			return nil
		}
		chain = append([]string{key}, chain...)
		obj, isAttr := cur.Object.(*ast.AttrGetExpr)
		if !isAttr {
			if id, ok := cur.Object.(*ast.IdentExpr); ok {
				chain = append([]string{id.Value}, chain...)
			} else {
				return nil
			}
			break
		}
		cur = obj
	}
	return chain
}

// Print writes a human-readable description of res for the given script.
func Print(res Result, script string) {
	fmt.Printf("%s: %s\n", script, modeLabel(res.Entry))
	if res.Main {
		fmt.Println("  entry: main()")
	}
	if len(res.Handlers) > 0 {
		fmt.Printf("  handlers: %s\n", strings.Join(res.Handlers, ", "))
	}
	fmt.Printf("  source: %d lines, %d top-level statements\n", res.Lines, res.Statements)
	if len(res.Forms) > 0 {
		fmt.Printf("  forms: %s\n", strings.Join(describeForms(res.Forms), ", "))
	}
	if len(res.Toplevel) > 0 {
		fmt.Printf("  top-level functions: %s\n", strings.Join(res.Toplevel, ", "))
	}
	if len(res.KUsage) > 0 {
		fmt.Printf("  k.* API (%d calls):\n", res.KCallCount)
		for _, k := range sortedKeys(res.KUsage) {
			fmt.Printf("    %s x%d\n", k, res.KUsage[k])
		}
	}
}

func modeLabel(m string) string {
	switch m {
	case "run":
		return "run-mode app"
	case "serve":
		return "serve-mode app"
	}
	return "no entry point"
}

func describeForms(forms []Form) []string {
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		if f.Form == "" {
			out = append(out, f.Op+"()")
			continue
		}
		label := f.Form + " (" + f.Op + ")"
		if f.Title != "" {
			label += " — " + f.Title
		}
		out = append(out, label)
	}
	return out
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}