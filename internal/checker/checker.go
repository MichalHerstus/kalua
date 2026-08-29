// Package checker performs static analysis on a KALUA script at load time:
// - reports unknown k.* references
// - verifies main() is defined
//
// Called by CLI `check` and by the run path before execution.
package checker

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"

	"kalua/internal/bindings"
)

type Result struct {
	Errors []string
}

func Check(src, name string) Result {
	chunk, err := parse.Parse(strings.NewReader(src), name)
	if err != nil {
		return Result{Errors: []string{fmt.Sprintf("%s: %v", name, err)}}
	}
	var res Result
	w := walker{src: src, name: name, known: bindings.Known(), res: &res}
	w.walk(chunk)
	if !w.hasMain {
		res.Errors = append(res.Errors, fmt.Sprintf("%s: missing required function main()", name))
	}
	return res
}

type walker struct {
	src     string
	name    string
	known   map[string]bool
	res     *Result
	hasMain bool
}

func (w *walker) walk(stmts []ast.Stmt) {
	for _, s := range stmts {
		w.walkStmt(s)
	}
}

func (w *walker) walkStmt(s ast.Stmt) {
	switch n := s.(type) {
	case *ast.FuncDefStmt:
		w.checkFuncDef(n)
		w.walk(n.Func.Stmts)
	case *ast.LocalAssignStmt:
		for _, e := range n.Exprs {
			w.walkExpr(e)
		}
	case *ast.AssignStmt:
		for _, e := range n.Rhs {
			w.walkExpr(e)
		}
	case *ast.FuncCallStmt:
		w.walkExpr(n.Expr)
	case *ast.DoBlockStmt:
		w.walk(n.Stmts)
	case *ast.WhileStmt:
		w.walkExpr(n.Condition)
		w.walk(n.Stmts)
	case *ast.RepeatStmt:
		w.walk(n.Stmts)
		w.walkExpr(n.Condition)
	case *ast.IfStmt:
		w.walkExpr(n.Condition)
		w.walk(n.Then)
		if n.Else != nil {
			w.walk(n.Else)
		}
	case *ast.NumberForStmt:
		w.walkExpr(n.Init)
		w.walkExpr(n.Limit)
		if n.Step != nil {
			w.walkExpr(n.Step)
		}
		w.walk(n.Stmts)
	case *ast.GenericForStmt:
		for _, e := range n.Exprs {
			w.walkExpr(e)
		}
		w.walk(n.Stmts)
	case *ast.ReturnStmt:
		for _, e := range n.Exprs {
			w.walkExpr(e)
		}
	case *ast.LabelStmt, *ast.GotoStmt, *ast.BreakStmt:
		// nothing to recurse
	}
}

func (w *walker) checkFuncDef(f *ast.FuncDefStmt) {
	name := funcName(f.Name)
	if name == "main" && f.Func != nil {
		w.hasMain = true
	}
}

func funcName(fn *ast.FuncName) string {
	if fn == nil {
		return ""
	}
	if fn.Func != nil {
		if ident, ok := fn.Func.(*ast.IdentExpr); ok {
			return ident.Value
		}
	}
	if fn.Method != "" {
		return fn.Method
	}
	return ""
}

func (w *walker) walkExpr(e ast.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *ast.IdentExpr:
		// bare ident; not a k.* access
	case *ast.AttrGetExpr:
		// obj.key or obj["key"] — check for k.<name>
		w.checkNestedAttrAccess(n)
		w.walkExpr(n.Object)
		w.walkExpr(n.Key)
	case *ast.FuncCallExpr:
		w.walkExpr(n.Func)
		for _, a := range n.Args {
			w.walkExpr(a)
		}
	case *ast.TableExpr:
		for _, f := range n.Fields {
			if f.Key != nil {
				w.walkExpr(f.Key)
			}
			w.walkExpr(f.Value)
		}
	case *ast.FunctionExpr:
		w.walk(n.Stmts)
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
	case *ast.NumberExpr, *ast.StringExpr, *ast.NilExpr,
		*ast.TrueExpr, *ast.FalseExpr, *ast.Comma3Expr:
		// literals
	}
}

// checkNestedAttrAccess checks for nested k.* access like k.form.new
func (w *walker) checkNestedAttrAccess(e *ast.AttrGetExpr) {
	var path []string
	current := e
	for {
		// Get the key name
		var keyName string
		switch k := current.Key.(type) {
		case *ast.IdentExpr:
			keyName = k.Value
		case *ast.StringExpr:
			keyName = k.Value
		default:
			return // dynamic key, can't check
		}
		path = append([]string{keyName}, path...) // prepend

		// Check if object is another AttrGetExpr
		if obj, ok := current.Object.(*ast.AttrGetExpr); ok {
			current = obj
			continue
		}
		// Check if object is the base "k"
		if ident, ok := current.Object.(*ast.IdentExpr); ok && ident.Value == "k" {
			// We have a full path like k.form.new
			fullName := ""
			for i, p := range path {
				if i > 0 {
					fullName += "."
				}
				fullName += p
			}
			if !w.known[fullName] {
				w.res.Errors = append(w.res.Errors,
					fmt.Sprintf("%s: unknown k.%s (not implemented)", w.name, fullName))
			}
			return
		}
		// Not a k.* chain
		return
	}
}

func (w *walker) checkTableAccess(obj, key ast.Expr) {
	// obj is k (IdentExpr "k") and key is IdentExpr or StringExpr with known name
	ident, ok := obj.(*ast.IdentExpr)
	if !ok || ident.Value != "k" {
		return
	}
	var name string
	switch k := key.(type) {
	case *ast.IdentExpr:
		name = k.Value
	case *ast.StringExpr:
		name = k.Value
	default:
		return // dynamic key, can't check
	}
	if !w.known[name] {
		w.res.Errors = append(w.res.Errors,
			fmt.Sprintf("%s: unknown k.%s (not implemented)", w.name, name))
	}
}

// checkNestedTableAccess checks for nested k.* access like k.form.new
func (w *walker) checkNestedTableAccess(obj ast.Expr, path []string) {
	// Walk the chain: k -> form -> new
	// obj should be an AttrGetExpr chain
	current := obj
	for _, part := range path {
		attr, ok := current.(*ast.AttrGetExpr)
		if !ok {
			return
		}
		// Check if the key is an IdentExpr or StringExpr
		var keyName string
		switch k := attr.Key.(type) {
		case *ast.IdentExpr:
			keyName = k.Value
		case *ast.StringExpr:
			keyName = k.Value
		default:
			return // dynamic key
		}
		if keyName != part {
			return // path doesn't match
		}
		current = attr.Object
	}
	// At this point, we've matched the full path
	// The final object should be "k"
	if ident, ok := current.(*ast.IdentExpr); ok && ident.Value == "k" {
		fullName := ""
		for i, p := range path {
			if i > 0 {
				fullName += "."
			}
			fullName += p
		}
		if !w.known[fullName] {
			w.res.Errors = append(w.res.Errors,
				fmt.Sprintf("%s: unknown k.%s (not implemented)", w.name, fullName))
		}
	}
}
