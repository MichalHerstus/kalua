package builder

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"
)

// Import parses a Kalipso-style Lua app and extracts EVERY form definition
// (k.form.new + k.ctrl.<type> + k.form.on + k.form.show calls) into a
// multi-form builder Document, in declaration order. Import is structure
// extraction only: non-serializable values (function bodies, DB handles,
// references) are recorded as notes, surrounding non-form code — and any
// k.form.* / k.ctrl.* call that is not a direct top-level statement — is left
// untouched. For each owned statement the importer records its [start,end]
// line span (Form.Lines) so the save path can surgically replace a form's
// block without touching anything else (rebuild.go). k.form.on handler
// statements are captured verbatim (exact source text) so re-emission never
// loses or reformats a hand-written handler.
func Import(src, fileName string) (*Document, error) {
	imp := &importer{
		byName:   map[string]*formDef{},
		srcLines: strings.Split(src, "\n"),
	}
	stmts, err := parse.Parse(strings.NewReader(src), fileName)
	if err != nil {
		return nil, err
	}
	for _, s := range stmts {
		imp.walkStmt(s)
	}
	if len(imp.forms) == 0 {
		return nil, fmt.Errorf("no k.form.new call found")
	}
	imp.finish()
	var forms []*Form
	for _, fd := range imp.forms {
		forms = append(forms, &Form{
			Name:          fd.name,
			Title:         fd.title,
			Layout:        fd.layout,
			Align:         fd.align,
			Gap:           fd.gap,
			Cells:         fd.cells,
			Controls:      fd.controls,
			Handlers:      fd.handlers,
			HandlerBodies: fd.bodies,
			Notes:         fd.notes,
			Lines:         fd.lines,
			Indent:        fd.indent,
			HasShow:       fd.hasShow,
		})
	}
	return &Document{
		Version:    DocVersion,
		ActiveForm: forms[0].Name,
		Forms:      forms,
		Notes:      imp.dnotes,
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
	handlers map[string][]string
	bodies   map[string]string
	notes    []string
	starts   []int    // owned statement start lines (1-based)
	endsMax  []int    // owned statement max descendant line (before bracket-close expansion)
	keys     []string // parallel body key ("@form.event" / "ctrl.event"/"")
	lines    [][]int
	indent   string
	hasShow  bool
}

type importer struct {
	forms       []*formDef
	byName      map[string]*formDef
	dnotes      []string
	bounds      []int // statement starts + block end lines (splice boundaries)
	srcLines    []string
	curStmtLine int
	directCall  bool // true while walking a bare FuncCallStmt statement
}

func (im *importer) dnote(f string, args ...any) {
	im.dnotes = append(im.dnotes, fmt.Sprintf(f, args...))
}

func (fd *formDef) note(f string, args ...any) {
	fd.notes = append(fd.notes, fmt.Sprintf(f, args...))
}

// bound records a splice safety boundary: a statement start or a block
// closing "end" line that the span expansion must never cross.
func (im *importer) bound(ln int) {
	if ln > 0 {
		im.bounds = append(im.bounds, ln)
	}
}

func (im *importer) walkStmt(s ast.Stmt) {
	if s == nil {
		return
	}
	im.curStmtLine = s.Line()
	im.bound(s.Line())
	im.bound(s.LastLine())
	switch n := s.(type) {
	case *ast.FuncDefStmt:
		im.directCall = false
		im.walkExpr(n.Func)
	case *ast.LocalAssignStmt:
		im.directCall = false
		for _, e := range n.Exprs {
			im.walkExpr(e)
		}
	case *ast.AssignStmt:
		im.directCall = false
		for _, e := range n.Rhs {
			im.walkExpr(e)
		}
	case *ast.FuncCallStmt:
		im.directCall = true
		im.walkExpr(n.Expr)
	case *ast.DoBlockStmt:
		im.directCall = false
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.WhileStmt:
		im.directCall = false
		im.walkExpr(n.Condition)
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.RepeatStmt:
		im.directCall = false
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
		im.walkExpr(n.Condition)
	case *ast.IfStmt:
		im.directCall = false
		im.walkExpr(n.Condition)
		for _, st := range n.Then {
			im.walkStmt(st)
		}
		for _, st := range n.Else {
			im.walkStmt(st)
		}
	case *ast.NumberForStmt:
		im.directCall = false
		im.walkExpr(n.Init)
		im.walkExpr(n.Limit)
		if n.Step != nil {
			im.walkExpr(n.Step)
		}
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.GenericForStmt:
		im.directCall = false
		for _, e := range n.Exprs {
			im.walkExpr(e)
		}
		for _, st := range n.Stmts {
			im.walkStmt(st)
		}
	case *ast.ReturnStmt:
		im.directCall = false
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
			if f == nil {
				continue
			}
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

// checkCall handles a direct function call: k.form.new / k.ctrl.<type> /
// k.form.on / k.form.show. Only bare call statements are owned (their spans
// are recorded so they can be regenerated); calls wrapped in another
// expression are skipped with a note so the source stays untouched.
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
	case len(path) == 3 && path[1] == "form" && path[2] == "show":
		im.importFormShow(n)
	case len(path) == 3 && path[1] == "ctrl" && contains(Types, path[2]):
		im.importControl(n, path[2])
	}
}

// spanEnd returns the deepest source line reached by the call expression
// (before bracket-close expansion), derived from the AST's line positions.
func (im *importer) spanEnd(n *ast.FuncCallExpr) int {
	var mx int = im.curStmtLine
	im.collectMaxExpr(n, &mx)
	return mx
}

func (im *importer) collectMaxExpr(e ast.Expr, mx *int) {
	if e == nil {
		return
	}
	if e.Line() > *mx {
		*mx = e.Line()
	}
	if e.LastLine() > *mx {
		*mx = e.LastLine()
	}
	switch n := e.(type) {
	case *ast.FuncCallExpr:
		im.collectMaxExpr(n.Func, mx)
		im.collectMaxExpr(n.Receiver, mx)
		for _, a := range n.Args {
			im.collectMaxExpr(a, mx)
		}
	case *ast.AttrGetExpr:
		im.collectMaxExpr(n.Object, mx)
		im.collectMaxExpr(n.Key, mx)
	case *ast.TableExpr:
		for _, f := range n.Fields {
			if f == nil {
				continue
			}
			im.collectMaxExpr(f.Key, mx)
			im.collectMaxExpr(f.Value, mx)
		}
	case *ast.FunctionExpr:
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
	case *ast.LogicalOpExpr:
		im.collectMaxExpr(n.Lhs, mx)
		im.collectMaxExpr(n.Rhs, mx)
	case *ast.RelationalOpExpr:
		im.collectMaxExpr(n.Lhs, mx)
		im.collectMaxExpr(n.Rhs, mx)
	case *ast.StringConcatOpExpr:
		im.collectMaxExpr(n.Lhs, mx)
		im.collectMaxExpr(n.Rhs, mx)
	case *ast.ArithmeticOpExpr:
		im.collectMaxExpr(n.Lhs, mx)
		im.collectMaxExpr(n.Rhs, mx)
	case *ast.UnaryMinusOpExpr:
		im.collectMaxExpr(n.Expr, mx)
	case *ast.UnaryNotOpExpr:
		im.collectMaxExpr(n.Expr, mx)
	case *ast.UnaryLenOpExpr:
		im.collectMaxExpr(n.Expr, mx)
	}
}

func (im *importer) collectMaxStmt(s ast.Stmt, mx *int) {
	if s == nil {
		return
	}
	if s.Line() > *mx {
		*mx = s.Line()
	}
	if s.LastLine() > *mx {
		*mx = s.LastLine()
	}
	switch n := s.(type) {
	case *ast.FuncDefStmt:
		im.collectMaxExpr(n.Func, mx)
	case *ast.LocalAssignStmt:
		for _, e := range n.Exprs {
			im.collectMaxExpr(e, mx)
		}
	case *ast.AssignStmt:
		for _, e := range n.Rhs {
			im.collectMaxExpr(e, mx)
		}
	case *ast.FuncCallStmt:
		im.collectMaxExpr(n.Expr, mx)
	case *ast.DoBlockStmt:
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
	case *ast.WhileStmt:
		im.collectMaxExpr(n.Condition, mx)
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
	case *ast.RepeatStmt:
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
		im.collectMaxExpr(n.Condition, mx)
	case *ast.IfStmt:
		im.collectMaxExpr(n.Condition, mx)
		for _, st := range n.Then {
			im.collectMaxStmt(st, mx)
		}
		for _, st := range n.Else {
			im.collectMaxStmt(st, mx)
		}
	case *ast.NumberForStmt:
		im.collectMaxExpr(n.Init, mx)
		im.collectMaxExpr(n.Limit, mx)
		if n.Step != nil {
			im.collectMaxExpr(n.Step, mx)
		}
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
	case *ast.GenericForStmt:
		for _, e := range n.Exprs {
			im.collectMaxExpr(e, mx)
		}
		for _, st := range n.Stmts {
			im.collectMaxStmt(st, mx)
		}
	case *ast.ReturnStmt:
		for _, e := range n.Exprs {
			im.collectMaxExpr(e, mx)
		}
	}
}

// recordSpan tags the current statement as owned by fd. Returns false when the
// call is not a bare statement (then it is left untouched, not regenerated).
func (im *importer) recordSpan(fd *formDef, key string, maxLine int) bool {
	if !im.directCall {
		return false
	}
	fd.starts = append(fd.starts, im.curStmtLine)
	fd.endsMax = append(fd.endsMax, maxLine)
	fd.keys = append(fd.keys, key)
	return true
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
		im.dnote("k.form.new with non-literal name skipped")
		return
	}
	if im.byName[name] != nil {
		return // second k.form.new for an imported form is left untouched
	}
	f := &formDef{
		name:     name,
		layout:   "vertical",
		align:    "left",
		handlers: map[string][]string{},
		bodies:   map[string]string{},
	}
	if len(n.Args) >= 2 {
		if opts, ok := n.Args[1].(*ast.TableExpr); ok {
			f.title, f.layout, f.align, f.gap, f.cells = formOptsToJSON(opts, &f.notes)
		}
	}
	if !im.recordSpan(f, "", im.spanEnd(n)) {
		im.dnote("k.form.new(%q) is not a direct call; skipped", name)
		return
	}
	f.indent = im.lineIndent(im.curStmtLine)
	im.byName[name] = f
	im.forms = append(im.forms, f)
}

func (im *importer) importControl(n *ast.FuncCallExpr, ctrlType string) {
	if len(n.Args) < 2 {
		return
	}
	formName := exprName(n.Args[0])
	if formName == "" {
		im.dnote("k.ctrl.%s with non-literal form name skipped", ctrlType)
		return
	}
	fd := im.byName[formName]
	if fd == nil {
		im.dnote("k.ctrl.%s targets form %q (not imported); skipping", ctrlType, formName)
		return
	}
	name := exprName(n.Args[1])
	if name == "" {
		im.dnote("k.ctrl.%s with non-literal control name skipped", ctrlType)
		return
	}
	ctrl := &Control{Name: name, Type: ctrlType, Opts: map[string]any{}, Inline: map[string]string{}}
	if len(n.Args) >= 3 {
		if opts, ok := n.Args[2].(*ast.TableExpr); ok {
			ctrl.Opts, _ = optsToJSON(opts, &fd.notes, &ctrl.Inline)
		}
	}
	// The runtime (and the builder GUI) render a label control's text from the
	// "text" option; some authors write "label" instead. Canonicalize so the
	// builder document, export, and preview all agree on "text".
	if ctrlType == "label" {
		if _, hasText := ctrl.Opts["text"]; !hasText {
			if lv, hasLabel := ctrl.Opts["label"]; hasLabel {
				ctrl.Opts["text"] = lv
				delete(ctrl.Opts, "label")
			}
		}
	}
	if !im.recordSpan(fd, "", im.spanEnd(n)) {
		im.dnote("k.ctrl.%s(%q, %q) is not a direct call; skipped", ctrlType, formName, name)
		return
	}
	fd.controls = append(fd.controls, ctrl)
}

func (im *importer) importFormShow(n *ast.FuncCallExpr) {
	if len(n.Args) < 1 {
		return
	}
	name := exprName(n.Args[0])
	fd := im.byName[name]
	if fd == nil {
		return // dynamic or unknown show is left untouched
	}
	if fd.hasShow {
		return // only the first literal k.form.show is owned
	}
	fd.hasShow = true
	im.recordSpan(fd, "", im.spanEnd(n))
}

func (im *importer) importFormOn(n *ast.FuncCallExpr) {
	if len(n.Args) < 3 {
		return
	}
	formName := exprName(n.Args[0])
	if formName == "" {
		return
	}
	fd := im.byName[formName]
	if fd == nil {
		return // handler for an unimported/dynamic form is left untouched
	}
	// 3-arg form: k.form.on(name, event, fn) — form-level handler, stored
	// under the "@form" key so export re-emits the 3-arg form and so
	// auto-generated stubs never overwrite a real lifecycle handler.
	if len(n.Args) == 3 {
		event := exprName(n.Args[1])
		if event == "" {
			return
		}
		fd.handlers["@form"] = append(fd.handlers["@form"], event)
		im.recordSpan(fd, "@form."+event, im.spanEnd(n))
		return
	}
	if len(n.Args) < 4 {
		return
	}
	ctrlName := exprName(n.Args[1])
	event := exprName(n.Args[2])
	if ctrlName == "" || event == "" {
		return
	}
	fd.handlers[ctrlName] = append(fd.handlers[ctrlName], event)
	im.recordSpan(fd, ctrlName+"."+event, im.spanEnd(n))
}

// finish computes each owned statement's [start,end] line span (expanding a
// multi-line statement to its closing bracket line, bounded by the next
// statement or block "end" line) and captures k.form.on handler statements
// verbatim from the original source.
func (im *importer) finish() {
	sort.Ints(im.bounds)
	boundAfter := func(ln int) int {
		for _, b := range im.bounds {
			if b > ln {
				return b
			}
		}
		return len(im.srcLines) + 1
	}
	for _, fd := range im.forms {
		var lines [][]int
		n := len(fd.starts)
		for i := 0; i < n; i++ {
			start := fd.starts[i]
			end := start
			if fd.endsMax[i] > start {
				// multi-line statement: extend past the closing bracket line,
				// but never into the next statement or a block "end".
				end = boundAfter(fd.endsMax[i]) - 1
				if end < fd.endsMax[i] {
					end = fd.endsMax[i]
				}
				if end > len(im.srcLines) {
					end = len(im.srcLines)
				}
			}
			lines = append(lines, []int{start, end})
			if k := fd.keys[i]; k != "" {
				fd.bodies[k] = im.verbatim(start, end)
			}
		}
		fd.lines = lines
	}
}

// verbatim returns the exact source text of lines [start..end] (1-based).
func (im *importer) verbatim(start, end int) string {
	if start < 1 {
		start = 1
	}
	if end > len(im.srcLines) {
		end = len(im.srcLines)
	}
	if end < start {
		return ""
	}
	var parts []string
	for i := start; i <= end; i++ {
		parts = append(parts, im.srcLines[i-1])
	}
	return strings.Join(parts, "\n")
}

// lineIndent returns the leading whitespace of the given 1-based source line.
func (im *importer) lineIndent(ln int) string {
	if ln < 1 || ln > len(im.srcLines) {
		return ""
	}
	s := im.srcLines[ln-1]
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return s[:i]
		}
	}
	return ""
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
		*notes = append(*notes, "function value kept in source (not editable in the builder)")
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
func optsToJSON(t *ast.TableExpr, notes *[]string, inline *map[string]string) (map[string]any, []string) {
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
		if fn, ok := f.Value.(*ast.FunctionExpr); ok {
			// Capture the handler body so export can re-inject it instead of
			// dropping the control's behavior.
			if inline != nil {
				(*inline)[k] = PrintFunction(fn)
			}
			*notes = append(*notes, fmt.Sprintf("event handler %q preserved in source (not editable in the builder; re-export keeps it)", k))
			continue
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
