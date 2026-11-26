package expr

import (
	"fmt"
	"strings"
	"unicode"
)

// AST ------------------------------------------------------------------------

type node interface{ isNode() }

type litNode struct {
	groovy string
	str    *string // set for string literals
}

type refNode struct{ path []string }

type notNode struct{ x node }

type parenNode struct{ x node }

type binNode struct {
	op   string
	l, r node
}

type callNode struct {
	name string
	args []node
}

func (*litNode) isNode()   {}
func (*refNode) isNode()   {}
func (*notNode) isNode()   {}
func (*parenNode) isNode() {}
func (*binNode) isNode()   {}
func (*callNode) isNode()  {}

// Lexer ----------------------------------------------------------------------

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tNumber
	tOp
)

type token struct {
	kind tokKind
	val  string
}

func lex(src string) ([]token, error) {
	var toks []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '\'':
			var b strings.Builder
			i++
			for {
				if i >= len(rs) {
					return nil, fmt.Errorf("unterminated string")
				}
				if rs[i] == '\'' {
					if i+1 < len(rs) && rs[i+1] == '\'' { // '' escapes a quote
						b.WriteRune('\'')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteRune(rs[i])
				i++
			}
			toks = append(toks, token{tString, b.String()})
		case unicode.IsDigit(c) || (c == '-' && i+1 < len(rs) && unicode.IsDigit(rs[i+1])):
			j := i + 1
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'x' || unicode.Is(unicode.ASCII_Hex_Digit, rs[j])) {
				j++
			}
			toks = append(toks, token{tNumber, string(rs[i:j])})
			i = j
		case unicode.IsLetter(c) || c == '_':
			j := i + 1
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '-') {
				j++
			}
			toks = append(toks, token{tIdent, string(rs[i:j])})
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				toks = append(toks, token{tOp, two})
				i += 2
				continue
			}
			if strings.ContainsRune("!<>().,[]*", c) {
				toks = append(toks, token{tOp, string(c)})
				i++
				continue
			}
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}
	return append(toks, token{kind: tEOF}), nil
}

// Parser ---------------------------------------------------------------------

type parser struct {
	toks []token
	pos  int
}

func parse(src string) (node, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 1 {
		return nil, fmt.Errorf("empty expression")
	}
	p := &parser{toks: toks}
	n, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected %q", p.peek().val)
	}
	return n, nil
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) isOp(ops ...string) bool {
	t := p.peek()
	if t.kind != tOp {
		return false
	}
	for _, o := range ops {
		if t.val == o {
			return true
		}
	}
	return false
}

func (p *parser) expect(op string) error {
	if !p.isOp(op) {
		return fmt.Errorf("expected %q, got %q", op, p.peek().val)
	}
	p.next()
	return nil
}

func (p *parser) binary(sub func() (node, error), ops ...string) (node, error) {
	l, err := sub()
	if err != nil {
		return nil, err
	}
	for p.isOp(ops...) {
		op := p.next().val
		r, err := sub()
		if err != nil {
			return nil, err
		}
		l = &binNode{op: op, l: l, r: r}
	}
	return l, nil
}

func (p *parser) or() (node, error)  { return p.binary(p.and, "||") }
func (p *parser) and() (node, error) { return p.binary(p.eq, "&&") }
func (p *parser) eq() (node, error)  { return p.binary(p.cmp, "==", "!=") }
func (p *parser) cmp() (node, error) { return p.binary(p.unary, "<", "<=", ">", ">=") }

func (p *parser) unary() (node, error) {
	if p.isOp("!") {
		p.next()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return &notNode{x: x}, nil
	}
	return p.primary()
}

func (p *parser) primary() (node, error) {
	t := p.next()
	switch t.kind {
	case tString:
		s := t.val
		return &litNode{groovy: QuoteString(s), str: &s}, nil
	case tNumber:
		return &litNode{groovy: t.val}, nil
	case tOp:
		if t.val == "(" {
			x, err := p.or()
			if err != nil {
				return nil, err
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return &parenNode{x: x}, nil
		}
		return nil, fmt.Errorf("unexpected %q", t.val)
	case tIdent:
		switch t.val {
		case "true", "false", "null":
			return &litNode{groovy: t.val}, nil
		}
		if p.isOp("(") {
			p.next()
			var args []node
			for !p.isOp(")") {
				a, err := p.or()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if !p.isOp(",") {
					break
				}
				p.next()
			}
			if err := p.expect(")"); err != nil {
				return nil, err
			}
			return &callNode{name: t.val, args: args}, nil
		}
		path := []string{t.val}
		for {
			switch {
			case p.isOp("."):
				p.next()
				id := p.next()
				if id.kind != tIdent && (id.kind != tOp || id.val != "*") {
					return nil, fmt.Errorf("expected property name after '.'")
				}
				path = append(path, id.val)
			case p.isOp("["):
				p.next()
				key := p.next()
				if key.kind != tString && key.kind != tNumber {
					return nil, fmt.Errorf("only literal index access is supported")
				}
				if err := p.expect("]"); err != nil {
					return nil, err
				}
				path = append(path, key.val)
			default:
				return &refNode{path: path}, nil
			}
		}
	}
	return nil, fmt.Errorf("unexpected end of expression")
}
