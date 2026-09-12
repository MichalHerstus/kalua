package builder

import (
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua/ast"
)

// PrintFunction renders an inline Lua function AST back to source text so that
// event-handler bodies survive the builder's JSON round-trip. Import captures
// handler bodies with this printer (into Control.Inline / Form.HandlerBodies);
// Export re-injects them so Save no longer drops hand-written or AI-generated
// handler logic.
func PrintFunction(fn *ast.FunctionExpr) string {
	if fn == nil {
		return "function() end"
	}
	var sb strings.Builder
	sb.WriteString("function(" + parList(fn) + ")\n")
	PrintStmts(&sb, fn.Stmts, 1)
	sb.WriteString("end")
	return sb.String()
}

func parList(fn *ast.FunctionExpr) string {
	if fn == nil || fn.ParList == nil {
		return ""
	}
	names := fn.ParList.Names
	if fn.ParList.HasVargs {
		names = append(names, "...")
	}
	return strings.Join(names, ", ")
}

func PrintStmts(sb *strings.Builder, stmts []ast.Stmt, indent int) {
	for _, s := range stmts {
		PrintStmt(sb, s, indent)
	}
}

func PrintStmt(sb *strings.Builder, s ast.Stmt, indent int) {
	if s == nil {
		return
	}
	pad := padOf(indent)
	switch n := s.(type) {
	case *ast.LocalAssignStmt:
		sb.WriteString(pad + "local " + strings.Join(n.Names, ", "))
		if len(n.Exprs) > 0 {
			sb.WriteString(" = " + joinExprs(n.Exprs))
		}
		sb.WriteString("\n")
	case *ast.AssignStmt:
		sb.WriteString(pad + joinExprs(n.Lhs) + " = " + joinExprs(n.Rhs) + "\n")
	case *ast.FuncCallStmt:
		sb.WriteString(pad + PrintExpr(n.Expr) + "\n")
	case *ast.DoBlockStmt:
		sb.WriteString(pad + "do\n")
		PrintStmts(sb, n.Stmts, indent+1)
		sb.WriteString(pad + "end\n")
	case *ast.WhileStmt:
		sb.WriteString(pad + "while " + PrintExpr(n.Condition) + " do\n")
		PrintStmts(sb, n.Stmts, indent+1)
		sb.WriteString(pad + "end\n")
	case *ast.RepeatStmt:
		sb.WriteString(pad + "repeat\n")
		PrintStmts(sb, n.Stmts, indent+1)
		sb.WriteString(pad + "until " + PrintExpr(n.Condition) + "\n")
	case *ast.IfStmt:
		printIf(sb, n, indent)
	case *ast.NumberForStmt:
		step := ""
		if n.Step != nil {
			step = ", " + PrintExpr(n.Step)
		}
		sb.WriteString(pad + "for " + n.Name + " = " + PrintExpr(n.Init) + ", " + PrintExpr(n.Limit) + step + " do\n")
		PrintStmts(sb, n.Stmts, indent+1)
		sb.WriteString(pad + "end\n")
	case *ast.GenericForStmt:
		sb.WriteString(pad + "for " + strings.Join(n.Names, ", ") + " in " + joinExprs(n.Exprs) + " do\n")
		PrintStmts(sb, n.Stmts, indent+1)
		sb.WriteString(pad + "end\n")
	case *ast.FuncDefStmt:
		sb.WriteString(pad + "function " + printFuncName(n.Name) + "(" + parList(n.Func) + ")\n")
		PrintStmts(sb, n.Func.Stmts, indent+1)
		sb.WriteString(pad + "end\n")
	case *ast.ReturnStmt:
		sb.WriteString(pad + "return " + joinExprs(n.Exprs) + "\n")
	case *ast.BreakStmt:
		sb.WriteString(pad + "break\n")
	case *ast.LabelStmt:
		sb.WriteString(pad + "::" + n.Name + "::\n")
	case *ast.GotoStmt:
		sb.WriteString(pad + "goto " + n.Label + "\n")
	}
}

// printIf renders an if/elseif/else chain. gopher-lua folds each elseif into a
// nested IfStmt stored as the sole element of the previous Else.
func printIf(sb *strings.Builder, n *ast.IfStmt, indent int) {
	pad := padOf(indent)
	for {
		sb.WriteString(pad + "if " + PrintExpr(n.Condition) + " then\n")
		PrintStmts(sb, n.Then, indent+1)
		if len(n.Else) == 0 {
			break
		}
		if len(n.Else) == 1 {
			if nested, ok := n.Else[0].(*ast.IfStmt); ok {
				sb.WriteString(pad + "elseif " + PrintExpr(nested.Condition) + " then\n")
				PrintStmts(sb, nested.Then, indent+1)
				n = nested
				continue
			}
		}
		sb.WriteString(pad + "else\n")
		PrintStmts(sb, n.Else, indent+1)
		break
	}
	sb.WriteString(pad + "end\n")
}

func printFuncName(fn *ast.FuncName) string {
	if fn == nil {
		return "?"
	}
	if fn.Method != "" && fn.Receiver != nil {
		return PrintExpr(fn.Receiver) + ":" + fn.Method
	}
	return PrintExpr(fn.Func)
}

// PrintExpr renders an expression AST back to Lua source. It is not a general
// Lua pretty-printer — it targets the statement/expression forms that appear
// inside form event-handler bodies (calls, arithmetic/comparison/logical
// operators, string concat, literals, tables, anonymous functions, gotos).
func PrintExpr(e ast.Expr) string {
	if e == nil {
		return "nil"
	}
	switch n := e.(type) {
	case *ast.StringExpr:
		return luaQuote(n.Value)
	case *ast.NumberExpr:
		return n.Value
	case *ast.TrueExpr:
		return "true"
	case *ast.FalseExpr:
		return "false"
	case *ast.NilExpr:
		return "nil"
	case *ast.IdentExpr:
		return n.Value
	case *ast.Comma3Expr:
		return "..."
	case *ast.AttrGetExpr:
		return printAttrGet(n)
	case *ast.TableExpr:
		return printTable(n)
	case *ast.FuncCallExpr:
		return printCall(n)
	case *ast.FunctionExpr:
		return PrintFunction(n)
	case *ast.LogicalOpExpr:
		return PrintExpr(n.Lhs) + " " + n.Operator + " " + PrintExpr(n.Rhs)
	case *ast.RelationalOpExpr:
		return PrintExpr(n.Lhs) + " " + n.Operator + " " + PrintExpr(n.Rhs)
	case *ast.StringConcatOpExpr:
		return PrintExpr(n.Lhs) + " .. " + PrintExpr(n.Rhs)
	case *ast.ArithmeticOpExpr:
		return PrintExpr(n.Lhs) + " " + n.Operator + " " + PrintExpr(n.Rhs)
	case *ast.UnaryMinusOpExpr:
		return " -" + unaryOperand(PrintExpr(n.Expr))
	case *ast.UnaryNotOpExpr:
		return "not " + unaryOperand(PrintExpr(n.Expr))
	case *ast.UnaryLenOpExpr:
		return "#" + unaryOperand(PrintExpr(n.Expr))
	}
	return "nil"
}

func printAttrGet(n *ast.AttrGetExpr) string {
	base := PrintExpr(n.Object)
	if n.Key == nil {
		return base
	}
	if k := keyName(n.Key); k != "" {
		if identRe.MatchString(k) {
			return base + "." + k
		}
		return base + "[" + luaQuote(k) + "]"
	}
	return base + "[" + PrintExpr(n.Key) + "]"
}

func printCall(n *ast.FuncCallExpr) string {
	var args []string
	for _, a := range n.Args {
		args = append(args, PrintExpr(a))
	}
	if n.Method != "" && n.Receiver != nil {
		return PrintExpr(n.Receiver) + ":" + n.Method + "(" + strings.Join(args, ", ") + ")"
	}
	return PrintExpr(n.Func) + "(" + strings.Join(args, ", ") + ")"
}

func printTable(t *ast.TableExpr) string {
	if t == nil || len(t.Fields) == 0 {
		return "{}"
	}
	allArray := true
	for _, f := range t.Fields {
		if f.Key != nil {
			allArray = false
			break
		}
	}
	if allArray {
		parts := make([]string, 0, len(t.Fields))
		for _, f := range t.Fields {
			parts = append(parts, PrintExpr(f.Value))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	var parts []string
	arrayIdx := 0
	for _, f := range t.Fields {
		if f == nil {
			arrayIdx++
			continue
		}
		if f.Key == nil {
			arrayIdx++
			parts = append(parts, fmt.Sprintf("[%d] = %s", arrayIdx, PrintExpr(f.Value)))
			continue
		}
		if k := keyName(f.Key); k != "" {
			parts = append(parts, luaKey(k)+" = "+PrintExpr(f.Value))
			continue
		}
		parts = append(parts, "["+PrintExpr(f.Key)+"] = "+PrintExpr(f.Value))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func joinExprs(exprs []ast.Expr) string {
	parts := make([]string, 0, len(exprs))
	for _, e := range exprs {
		parts = append(parts, PrintExpr(e))
	}
	return strings.Join(parts, ", ")
}

// unaryOperand wraps an operand so 'not' / '-' / '#' keep their precedence
// when the operand is a compound expression (binary ops, multi-arg calls emit
// spaces; simple atoms do not).
func unaryOperand(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' {
			return "(" + s + ")"
		}
	}
	return s
}

func padOf(indent int) string {
	var p strings.Builder
	for i := 0; i < indent; i++ {
		p.WriteString("  ")
	}
	return p.String()
}
