package format

import (
	"bytes"
	"strings"

	"github.com/yuin/gopher-lua/parse"
)

// tokComment is the normalized Type used for comment items in the printer's
// prev-item state (real comment tokens carry parse.commentToken, which is
// unexported).
const tokComment = -2

// indentUnit is the padding added per structural level.
const indentUnit = "  "

type printer struct {
	out bytes.Buffer

	// Structural depth (each unit adds one indentUnit line):
	// blockDepth counts then/do/repeat/function bodies; bracketDepth counts
	// {, (, [. elseif pops the then-body slot and its own `then` re-pushes,
	// so a whole if/elseif/else chain shares a single slot.
	blockDepth   int
	bracketDepth int
	fnParens     int // open `function` parameter lists awaiting their ')'

	prev           parse.FormatterToken
	prevMinusUnary bool // the last emitted '-' token was unary
	first          bool
	lastLine       int // end line of the last emitted item
}

func (p *printer) indent() string { return strings.Repeat(indentUnit, p.blockDepth+p.bracketDepth) }

// print renders the token stream to its canonical form.
func (p *printer) print(toks []parse.FormatterToken) string {
	p.first = true
	for _, t := range toks {
		// Structural mutations happen before emit so closing keywords/braces
		// align with their opener, and so indentation reflects the post-close
		// (or post-open) depth for the item itself.
		switch t.Type {
		case parse.TThen, parse.TDo, parse.TRepeat:
			p.blockDepth++
		case parse.TEnd, parse.TUntil, parse.TElseIf, parse.TElse:
			if p.blockDepth > 0 {
				p.blockDepth--
			}
		case parse.TFunction:
			p.fnParens++
		case '(', '[', '{':
			p.bracketDepth++
		case ')':
			if p.bracketDepth > 0 {
				p.bracketDepth--
			}
			if p.fnParens > 0 {
				p.fnParens--
				p.blockDepth++
			}
		case ']', '}':
			if p.bracketDepth > 0 {
				p.bracketDepth--
			}
		}
		p.emit(t)
		// The else body reuses the if's block slot, so the depth is restored
		// only after the `else` keyword itself has been emitted at the outer
		// level.
		if t.Type == parse.TElse {
			p.blockDepth++
		}
	}
	out := p.out.String()
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// emit writes one item with normalized surrounding whitespace.
func (p *printer) emit(t parse.FormatterToken) {
	newLine := false
	if !p.first {
		newLine = forcedNewline(t) || t.Line > p.lastLine
	}

	if newLine {
		if t.Line-p.lastLine >= 2 && !suppressBlank(t) && !isOpener(p.prev.Type) {
			p.out.WriteByte('\n')
		}
		p.out.WriteByte('\n')
		p.out.WriteString(p.indent())
	} else if !p.first && p.space(t) {
		p.out.WriteByte(' ')
	}

	p.out.WriteString(text(t))
	p.lastLine = endLine(t)
	if t.IsComment {
		p.prev = parse.FormatterToken{Type: tokComment, Line: t.Line}
		p.prevMinusUnary = false
	} else {
		prevOperand := isOperandEnd(p.prev)
		p.prev = t
		p.prevMinusUnary = t.Type == '-' && !prevOperand
	}
	p.first = false
}

// space reports whether a single space separates the previous item and t on
// the same line.
func (p *printer) space(t parse.FormatterToken) bool {
	return spaceNeeded(p.prev, t, p.prevMinusUnary)
}

// spaceNeeded reports whether a single space separates prev and cur on the
// same line (no space is emitted for delimiters/selectors/unary operators per
// gofmt-style rules; everything else gets one).
func spaceNeeded(prev, cur parse.FormatterToken, prevMinusUnary bool) bool {
	// No space before delimiters / selectors / closers.
	switch cur.Type {
	case ',', ';', ')', ']', '}', '.', ':', parse.T2Colon:
		return false
	}

	// No space after brackets, selectors, or unary operators.
	switch prev.Type {
	case '(', '[', '{', ':', '.', parse.T2Colon, '#':
		return false
	case '-':
		return !prevMinusUnary // binary minus: space; unary minus: bind tight
	}

	// No space before a call/index/open that directly follows an operand, a
	// function literal's parameter list, or a unary operator that binds tight.
	switch cur.Type {
	case '(', '{', '[':
		if isOperandEnd(prev) || prev.Type == parse.TFunction || prev.Type == '#' ||
			(prev.Type == '-' && prevMinusUnary) {
			return false
		}
		return true
	}

	return true
}

// isOperandEnd reports whether tok ends a value (an operand), i.e. a following
// '-' must be a binary operator (rather than unary minus) and a '(' a call.
func isOperandEnd(t parse.FormatterToken) bool {
	switch t.Type {
	case parse.TIdent, parse.TNumber, parse.TString, parse.TTrue, parse.TFalse, parse.TNil:
		return true
	}
	return t.Type == ')' || t.Type == ']' || t.Type == '}'
}

// isOpener reports whether a blank line right after prev should be dropped.
func isOpener(typ int) bool {
	switch typ {
	case '{', '(', '[', parse.TThen, parse.TDo, parse.TRepeat:
		return true
	}
	return false
}

// suppressBlank reports whether a blank line right before cur should be
// dropped (gofmt collapses blanks adjacent to a block boundary).
func suppressBlank(t parse.FormatterToken) bool {
	switch t.Type {
	case parse.TEnd, parse.TElse, parse.TElseIf, parse.TUntil, ')', ']', '}':
		return true
	}
	return false
}

// forcedNewline reports whether cur must start a new line regardless of the
// source layout (block terminators always do).
func forcedNewline(t parse.FormatterToken) bool {
	switch t.Type {
	case parse.TEnd, parse.TElse, parse.TElseIf, parse.TUntil:
		return true
	}
	return false
}

// text returns the exact source text to emit for an item.
func text(t parse.FormatterToken) string { return t.Raw }

// endLine is the source line the last byte of t is on (accounting for
// embedded newlines in long strings and block comments).
func endLine(t parse.FormatterToken) int {
	return t.Line + strings.Count(t.Raw, "\n")
}
