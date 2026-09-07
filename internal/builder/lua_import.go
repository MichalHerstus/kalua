package builder

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"
)

// Import parses a Kalipso-style Lua app and extracts its form definition
// (k.form.new + k.ctrl.<type> + k.form.on calls) into a builder Document.
// Import is structure extraction only: the first k.form.new in the file is
// used, non-serializable values (function bodies, DB handles, references)
// are recorded as notes, and surrounding non-form code is left untouched.
func Import(src, fileName string) (*Document, error) {
	imp := &importer{handlers: map[string][]string{}}
	stmts, err := parse.Parse(strings.NewReader(src), fileName)
	if err != nil {
		return nil, err
	}
	for _, s := range stmts {
		imp.walkStmt(s)
	}
	if imp.form == nil {
		return nil, fmt.Errorf("no k.form.new call found")
	}
	return &Document{
		Version: DocVersion,
		Form: &Form{
			Name:     imp.form.name,
			Title:    imp.form.title,
			Layout:   imp.form.layout,
			Align:    imp.form.align,
			Gap:      imp.form.gap,
			Cells:    imp.form.cells,
			Controls: imp.form.controls,
			Handlers: imp.handlers,
			Notes:    imp.notes,
		},
	}, nil
}

type formDef struct {
	name     string
	title    string
	layout   string
	align    string
	gap      *int
	cells    []*CellDef
	controls []*Control
}

type importer struct {
	form      *formDef
	formCount int
	handlers  map[string][]string
	notes     []string
}

func (im *importer) note(f string, args ...any) {
	im.notes = append(im.notes, fmt.Sprintf(f, args...))
}

func (im *importer) walkStmt(s ast.Stmt) {
	switch n := s.(type) {
	case *ast.FuncDefStmt:
		im.walkExpr(n.Func)
	case *ast.LocalAssignStmt:
		for _, e := range n.Exprs {
			im.walkExpr(e)
		}
	case *ast.AssignStmt:
		for _, e := range n.Rhs {
			im.walkExpr(e)
		}
	case *ast.FuncCallStmt:
		im.walkExpr(n.Expr)
	case *ast.DoBlockStmt:
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.WhileStmt:
		im.walkExpr(n.Condition)
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.RepeatStmt:
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
		im.walkExpr(n.Condition)
	case *ast.IfStmt:
		im.walkExpr(n.Condition)
		for _, st := range n.Then {
			im.walkStmt(st)
		}
		for _, st := range n.Else {
			im.walkStmt(st)
		}
	case *ast.NumberForStmt:
		im.walkExpr(n.Init)
		im.walkExpr(n.Limit)
		if n.Step != nil {
			im.walkExpr(n.Step)
		}
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.GenericForStmt:
		for _, e := range n.Exprs {
			im.walkExpr(e)
		}
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.ReturnStmt:
		for _, e := range n.Exprs {
			im.walkExpr(e)
		}
	}
}

func (im *importer) walkExpr(e ast.Expr) {
	switch n := e.(type) {
	case *ast.FuncCallExpr:
		im.checkCall(n)
		if n.Func != nil {
			im.walkExpr(n.Func)
		}
		if n.Receiver != nil {
			im.walkExpr(n.Receiver)
		}
		for _, a := range n.Args {
			im.walkExpr(a)
		}
	case *ast.AttrGetExpr:
		im.walkExpr(n.Object)
		if n.Key != nil {
			im.walkExpr(n.Key)
		}
	case *ast.TableExpr:
		for _, f := range n.Fields {
			if f.Key != nil {
				im.walkExpr(f.Key)
			}
			im.walkExpr(f.Value)
		}
	case *ast.FunctionExpr:
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.LogicalOpExpr:
		im.walkExpr(n.Lhs)
		im.walkExpr(n.Rhs)
	case *ast.RelationalOpExpr:
		im.walkExpr(n.Lhs)
		im.walkExpr(n.Rhs)
	case *ast.StringConcatOpExpr:
		im.walkExpr(n.Lhs)
		im.walkExpr(n.Rhs)
	case *ast.ArithmeticOpExpr:
		im.walkExpr(n.Lhs)
		im.walkExpr(n.Rhs)
	case *ast.UnaryMinusOpExpr:
		im.walkExpr(n.Expr)
	case *ast.UnaryNotOpExpr:
		im.walkExpr(n.Expr)
	case *ast.UnaryLenOpExpr:
		im.walkExpr(n.Expr)
	}
}

// checkCall handles a function call: k.form.new / k.ctrl.<type> / k.form.on.
func (im *importer) checkCall(n *ast.FuncCallExpr) {
	path := callPath(n)
	if len(path) == 0 || path[0] != "k" {
		return
	}
	switch {
	case len(path) == 3 && path[1] == "form" && path[2] == "new":
		im.importFormNew(n)
	case len(path) == 3 && path[1] == "form" && path[2] == "on":
		im.importFormOn(n)
	case len(path) == 3 && path[1] == "ctrl" && contains(Types, path[2]):
		im.importControl(n, path[2])
	}
}

// callPath resolves a function-call callee to its dotted path (e.g.
// "k.form.new" → [k form new]), including ':method' form. Returns nil when the
// callee is not a statically-resolvable chain.
func callPath(n *ast.FuncCallExpr) []string {
	if n.Receiver != nil {
		return append(attrChain(n.Receiver), n.Method)
	}
	return attrChain(n.Func)
}

// attrChain unwraps AttrGetExpr chains to their dotted segments.
func attrChain(e ast.Expr) []string {
	switch n := e.(type) {
	case *ast.IdentExpr:
		return []string{n.Value}
	case *ast.AttrGetExpr:
		key := keyName(n.Key)
		if key == "" {
			return nil
		}
		parent := attrChain(n.Object)
		if parent == nil {
			return nil
		}
		return append(parent, key)
	}
	return nil
}

func keyName(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.IdentExpr:
		return n.Value
	case *ast.StringExpr:
		return n.Value
	}
	return ""
}

func (im *importer) importFormNew(n *ast.FuncCallExpr) {
	if len(n.Args) < 1 {
		return
	}
	name := exprName(n.Args[0])
	if name == "" {
		im.note("k.form.new with non-literal name skipped")
		return
	}
	if im.form != nil {
		im.formCount++
		im.note("additional form %q found; only the first form is imported", name)
		return
	}
	f := &formDef{name: name, layout: "vertical", align: "left"}
	if len(n.Args) >= 2 {
		if opts, ok := n.Args[1].(*ast.TableExpr); ok {
			f.title, f.layout, f.align, f.gap, f.cells = formOptsToJSON(opts, &im.notes)
		}
	}
	im.form = f
}

func (im *importer) importControl(n *ast.FuncCallExpr, ctrlType string) {
	if len(n.Args) < 2 {
		return
	}
	formName := exprName(n.Args[0])
	if formName == "" {
		im.note("k.ctrl.%s with non-literal form name skipped", ctrlType)
		return
	}
	if im.form == nil || formName != im.form.name {
		im.note("k.ctrl.%s targets form %q (not imported); skipping", ctrlType, formName)
		return
	}
	name := exprName(n.Args[1])
	if name == "" {
		im.note("k.ctrl.%s with non-literal control name skipped", ctrlType)
		return
	}
	ctrl := &Control{Name: name, Type: ctrlType, Opts: map[string]any{}}
	if len(n.Args) >= 3 {
		if opts, ok := n.Args[2].(*ast.TableExpr); ok {
			ctrl.Opts, _ = optsToJSON(opts, &im.notes)
		}
	}
	im.form.controls = append(im.form.controls, ctrl)
}

func (im *importer) importFormOn(n *ast.FuncCallExpr) {
	if len(n.Args) < 4 {
		return
	}
	formName := exprName(n.Args[0])
	if formName == "" || (im.form != nil && formName != im.form.name) {
		return
	}
	ctrlName := exprName(n.Args[1])
	event := exprName(n.Args[2])
	if ctrlName == "" || event == "" {
		return
	}
	im.handlers[ctrlName] = append(im.handlers[ctrlName], event)
}

// exprName returns a literal string from an expr (StringExpr, or an IdentExpr
// used as a constant). Empty when the value is not statically available.
func exprName(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.StringExpr:
		return n.Value
	case *ast.IdentExpr:
		return n.Value
	}
	return ""
}

// exprToJSON converts a literal AST expression into a JSON-ready value.
// Non-literal or non-serializable nodes become nil with a note.
func exprToJSON(e ast.Expr, notes *[]string) any {
	if e == nil {
		return nil
	}
	switch n := e.(type) {
	case *ast.StringExpr:
		return n.Value
	case *ast.NumberExpr:
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return n.Value
		}
		return f
	case *ast.TrueExpr:
		return true
	case *ast.FalseExpr:
		return false
	case *ast.NilExpr:
		return nil
	case *ast.TableExpr:
		return tableToJSON(n, notes)
	case *ast.UnaryMinusOpExpr:
		if num, ok := n.Expr.(*ast.NumberExpr); ok {
			f, err := strconv.ParseFloat(num.Value, 64)
			if err == nil {
				return -f
			}
		}
		*notes = append(*notes, "unsupported negative expression skipped")
		return nil
	case *ast.IdentExpr:
		*notes = append(*notes, fmt.Sprintf("expression %q is not a literal; skipped", n.Value))
		return nil
	case *ast.FunctionExpr:
		*notes = append(*notes, "function body cannot be serialized; skipped")
		return nil
	default:
		*notes = append(*notes, fmt.Sprintf("non-literal value skipped (%T)", e))
		return nil
	}
}

// tableToJSON converts a Lua table literal to JSON: all-array fields → JSON
// array; otherwise a string-keyed JSON object.
func tableToJSON(t *ast.TableExpr, notes *[]string) any {
	if t == nil || len(t.Fields) == 0 {
		return map[string]any{}
	}
	allArray := true
	for _, f := range t.Fields {
		if f.Key != nil {
			allArray = false
			break
		}
	}
	if allArray {
		arr := make([]any, 0, len(t.Fields))
		for _, f := range t.Fields {
			arr = append(arr, exprToJSON(f.Value, notes))
		}
		return arr
	}
	m := map[string]any{}
	for i, f := range t.Fields {
		if f.Key == nil {
			m[fmt.Sprintf("%d", i+1)] = exprToJSON(f.Value, notes)
			continue
		}
		k := keyName(f.Key)
		if k == "" {
			continue
		}
		m[k] = exprToJSON(f.Value, notes)
	}
	return m
}

// optsToJSON converts a control opts table literal. The `items` key is
// normalized to the ordered [{key,display}] representation.
func optsToJSON(t *ast.TableExpr, notes *[]string) (map[string]any, []string) {
	out := map[string]any{}
	if t == nil {
		return out, nil
	}
	for _, f := range t.Fields {
		k := keyName(f.Key)
		if k == "" {
			continue
		}
		if k == "items" {
			if tbl, ok := f.Value.(*ast.TableExpr); ok {
				out[k] = itemsToOrdered(tbl)
				continue
			}
		}
		out[k] = exprToJSON(f.Value, notes)
	}
	return out, nil
}

// itemsToOrdered converts an items map literal into the ordered representation
// [{key, display}] preserving source order.
func itemsToOrdered(t *ast.TableExpr) []any {
	arr := make([]any, 0, len(t.Fields))
	for _, f := range t.Fields {
		k := keyName(f.Key)
		if k == "" {
			continue
		}
		arr = append(arr, map[string]any{
			"key":     k,
			"display": exprToJSON(f.Value, nil),
		})
	}
	return arr
}

// formOptsToJSON extracts title/layout/align/gap/cells from a k.form.new
// opts table literal.
func formOptsToJSON(t *ast.TableExpr, notes *[]string) (title, layout, align string, gap *int, cells []*CellDef) {
	layout, align = "vertical", "left"
	if t == nil {
		return
	}
	for _, f := range t.Fields {
		k := keyName(f.Key)
		switch k {
		case "title":
			title = exprString(f.Value)
		case "layout":
			layout = exprString(f.Value)
			if layout == "" {
				layout = "vertical"
			}
		case "align":
			align = exprString(f.Value)
			if align == "" {
				align = "left"
			}
		case "gap":
			if n := exprNumber(f.Value); n != nil {
				gap = n
			}
		case "cells":
			if tbl, ok := f.Value.(*ast.TableExpr); ok {
				cells = cellsToList(tbl)
			}
		}
	}
	return
}

// cellsToList normalizes both source forms of the cells option (array of
// {id=...} entries, or map {id={...}}) into an ordered []*CellDef. The array
// form preserves the declared order (the runtime's canonical representation);
// the map form falls back to lexical id order.
func cellsToList(t *ast.TableExpr) []*CellDef {
	var cells []*CellDef
	if t == nil {
		return cells
	}
	for _, f := range t.Fields {
		cellTbl, ok := f.Value.(*ast.TableExpr)
		if !ok {
			continue
		}
		if f.Key != nil {
			id := keyName(f.Key)
			if id != "" {
				d := cellToDef(cellTbl)
				d.Id = id
				cells = append(cells, d)
			}
			continue
		}
		// array form: read the id field from the entry itself
		id := ""
		for _, cf := range cellTbl.Fields {
			if keyName(cf.Key) == "id" {
				id = exprString(cf.Value)
				break
			}
		}
		if id != "" {
			d := cellToDef(cellTbl)
			d.Id = id
			cells = append(cells, d)
		}
	}
	return cells
}

func cellToDef(t *ast.TableExpr) *CellDef {
	c := &CellDef{}
	for _, f := range t.Fields {
		switch keyName(f.Key) {
		case "width":
			if n := exprNumber(f.Value); n != nil {
				c.Width = *n
			}
		case "bg", "background":
			c.Bg = exprString(f.Value)
		case "align":
			c.Align = exprString(f.Value)
		case "border":
			if bt, ok := f.Value.(*ast.TableExpr); ok {
				c.Border = &Border{}
				for _, bf := range bt.Fields {
					switch keyName(bf.Key) {
					case "width":
						if n := exprNumber(bf.Value); n != nil {
							c.Border.Width = *n
						}
					case "color":
						c.Border.Color = exprString(bf.Value)
					}
				}
			}
		}
	}
	return c
}

func exprString(e ast.Expr) string {
	if s, ok := e.(*ast.StringExpr); ok {
		return s.Value
	}
	return ""
}

func exprNumber(e ast.Expr) *int {
	switch n := e.(type) {
	case *ast.NumberExpr:
		f, err := strconv.Atoi(n.Value)
		if err != nil {
			return nil
		}
		return &f
	case *ast.UnaryMinusOpExpr:
		if num, ok := n.Expr.(*ast.NumberExpr); ok {
			f, err := strconv.Atoi(num.Value)
			if err != nil {
				return nil
			}
			f = -f
			return &f
		}
	}
	return nil
}

// SortControlNames orders the control list by name for deterministic JSON.
func sortControls(ctrls []*Control) {
	sort.Slice(ctrls, func(i, j int) bool {
		return strings.Compare(ctrls[i].Name, ctrls[j].Name) < 0
	})
}
