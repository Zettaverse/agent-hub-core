package flow

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// This file implements a small, safe expression language used by condition
// nodes. Supported: string/number/bool/null literals, identifiers with field
// access (result, result.field), the `.length` property, unary `!`, comparison
// operators == != > < >= <=, and logical && || with short-circuit evaluation.

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokBool
	tokNull
	tokAnd // &&
	tokOr  // ||
	tokNot // !
	tokEq  // ==
	tokNeq // !=
	tokGt  // >
	tokLt  // <
	tokGe  // >=
	tokLe  // <=
	tokDot // .
	tokLParen
	tokRParen
)

type token struct {
	kind tokenKind
	text string
	num  float64
}

// evalExpression evaluates expr with the given variable bindings.
func evalExpression(expr string, vars map[string]any) (any, error) {
	tokens, err := lex(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, vars: vars}
	v, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("expr: unexpected token %q", p.peek().text)
	}
	return v, nil
}

func lex(src string) ([]token, error) {
	var tokens []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '&':
			if i+1 < len(src) && src[i+1] == '&' {
				tokens = append(tokens, token{kind: tokAnd, text: "&&"})
				i += 2
			} else {
				return nil, fmt.Errorf("expr: unexpected '&' at %d", i)
			}
		case c == '|':
			if i+1 < len(src) && src[i+1] == '|' {
				tokens = append(tokens, token{kind: tokOr, text: "||"})
				i += 2
			} else {
				return nil, fmt.Errorf("expr: unexpected '|' at %d", i)
			}
		case c == '=':
			if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, token{kind: tokEq, text: "=="})
				i += 2
			} else {
				return nil, fmt.Errorf("expr: unexpected '=' at %d", i)
			}
		case c == '!':
			if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, token{kind: tokNeq, text: "!="})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokNot, text: "!"})
				i++
			}
		case c == '>':
			if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, token{kind: tokGe, text: ">="})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokGt, text: ">"})
				i++
			}
		case c == '<':
			if i+1 < len(src) && src[i+1] == '=' {
				tokens = append(tokens, token{kind: tokLe, text: "<="})
				i += 2
			} else {
				tokens = append(tokens, token{kind: tokLt, text: "<"})
				i++
			}
		case c == '.':
			tokens = append(tokens, token{kind: tokDot, text: "."})
			i++
		case c == '(':
			tokens = append(tokens, token{kind: tokLParen, text: "("})
			i++
		case c == ')':
			tokens = append(tokens, token{kind: tokRParen, text: ")"})
			i++
		case c == '"' || c == '\'':
			quote := c
			j := i + 1
			var sb strings.Builder
			for j < len(src) && src[j] != quote {
				if src[j] == '\\' && j+1 < len(src) {
					j++
				}
				sb.WriteByte(src[j])
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("expr: unterminated string at %d", i)
			}
			tokens = append(tokens, token{kind: tokString, text: sb.String()})
			i = j + 1
		case unicode.IsDigit(rune(c)) || (c == '-' && i+1 < len(src) && unicode.IsDigit(rune(src[i+1]))):
			j := i
			if src[j] == '-' {
				j++
			}
			for j < len(src) && (unicode.IsDigit(rune(src[j])) || src[j] == '.') {
				j++
			}
			num, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("expr: invalid number %q", src[i:j])
			}
			tokens = append(tokens, token{kind: tokNumber, text: src[i:j], num: num})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			word := src[i:j]
			switch word {
			case "true":
				tokens = append(tokens, token{kind: tokBool, text: word, num: 1})
			case "false":
				tokens = append(tokens, token{kind: tokBool, text: word, num: 0})
			case "null":
				tokens = append(tokens, token{kind: tokNull, text: word})
			default:
				tokens = append(tokens, token{kind: tokIdent, text: word})
			}
			i = j
		default:
			return nil, fmt.Errorf("expr: unexpected character %q at %d", c, i)
		}
	}
	tokens = append(tokens, token{kind: tokEOF})
	return tokens, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

type parser struct {
	tokens []token
	pos    int
	vars   map[string]any
}

func (p *parser) peek() token {
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		if truthy(left) {
			// Short-circuit: the whole OR chain is truthy; skip remaining
			// operands without evaluating them.
			for p.peek().kind == tokOr {
				p.next()
				p.skipAnd()
			}
			return left, nil
		}
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = right
	}
	return left, nil
}

func (p *parser) parseAnd() (any, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		if !truthy(left) {
			// Short-circuit: the whole AND chain is falsy; skip remaining
			// operands without evaluating them.
			for p.peek().kind == tokAnd {
				p.next()
				p.skipUnary()
			}
			return left, nil
		}
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = right
	}
	return left, nil
}

// skipAnd consumes one and-expression (a sequence of unary expressions joined
// by &&) without evaluating it. It stops at a top-level ||, ) or EOF.
func (p *parser) skipAnd() {
	p.skipUnary()
	for p.peek().kind == tokAnd {
		p.next()
		p.skipUnary()
	}
}

// skipUnary consumes one unary/comparison/primary expression without
// evaluating it. It stops at &&, ||, ) or EOF at the top level.
func (p *parser) skipUnary() {
	if p.peek().kind == tokNot {
		p.next()
		p.skipUnary()
		return
	}
	p.skipPrimary()
	if isComparison(p.peek().kind) {
		p.next()
		p.skipPrimary()
	}
}

// skipPrimary consumes a single literal, identifier path, or parenthesized
// expression without evaluating it.
func (p *parser) skipPrimary() {
	t := p.peek()
	switch t.kind {
	case tokLParen:
		p.next()
		depth := 1
		for depth > 0 {
			cur := p.next()
			if cur.kind == tokEOF {
				return
			}
			if cur.kind == tokLParen {
				depth++
			}
			if cur.kind == tokRParen {
				depth--
			}
		}
	default:
		if t.kind == tokEOF {
			return
		}
		p.next()
		for p.peek().kind == tokDot {
			p.next() // .
			p.next() // field name
		}
	}
}

func isComparison(k tokenKind) bool {
	switch k {
	case tokEq, tokNeq, tokGt, tokLt, tokGe, tokLe:
		return true
	}
	return false
}

func (p *parser) parseUnary() (any, error) {
	if p.peek().kind == tokNot {
		p.next()
		v, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return !truthy(v), nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (any, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	switch p.peek().kind {
	case tokEq, tokNeq, tokGt, tokLt, tokGe, tokLe:
		op := p.next()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return compare(op.kind, left, right)
	}
	return left, nil
}

func (p *parser) parsePrimary() (any, error) {
	switch t := p.peek(); t.kind {
	case tokString:
		p.next()
		return t.text, nil
	case tokNumber:
		p.next()
		return t.num, nil
	case tokBool:
		p.next()
		return t.num == 1, nil
	case tokNull:
		p.next()
		return nil, nil
	case tokLParen:
		p.next()
		v, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("expr: missing closing parenthesis")
		}
		p.next()
		return v, nil
	case tokIdent:
		p.next()
		return p.parseFieldAccess(t.text)
	default:
		return nil, fmt.Errorf("expr: unexpected token %q", t.text)
	}
}

func (p *parser) parseFieldAccess(root string) (any, error) {
	val, ok := p.vars[root]
	if !ok {
		return nil, fmt.Errorf("expr: unknown variable %q", root)
	}
	for p.peek().kind == tokDot {
		p.next()
		fieldTok := p.next()
		if fieldTok.kind != tokIdent {
			return nil, fmt.Errorf("expr: expected field name after '.'")
		}
		val = field(val, fieldTok.text)
	}
	return val, nil
}

func field(v any, name string) any {
	if name == "length" {
		return lengthOf(v)
	}
	switch m := v.(type) {
	case map[string]any:
		return m[name]
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			item := rv.MapIndex(reflect.ValueOf(name))
			if item.IsValid() {
				return item.Interface()
			}
		}
		return nil
	}
}

func lengthOf(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case string:
		return len(x)
	case []any:
		return len(x)
	case map[string]any:
		return len(x)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return rv.Len()
	}
	return 0
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	}
	return true
}

func compare(op tokenKind, a, b any) (any, error) {
	switch op {
	case tokEq:
		return deepEqual(a, b), nil
	case tokNeq:
		return !deepEqual(a, b), nil
	}
	// Ordering comparisons: numeric if both are numbers, otherwise strings.
	an, aok := toFloat(a)
	bn, bok := toFloat(b)
	if aok && bok {
		return orderFloat(op, an, bn), nil
	}
	as, asok := a.(string)
	bs, bsok := b.(string)
	if asok && bsok {
		return orderString(op, as, bs), nil
	}
	return nil, fmt.Errorf("expr: cannot order values of incompatible types")
}

func orderFloat(op tokenKind, a, b float64) bool {
	switch op {
	case tokGt:
		return a > b
	case tokLt:
		return a < b
	case tokGe:
		return a >= b
	case tokLe:
		return a <= b
	}
	return false
}

func orderString(op tokenKind, a, b string) bool {
	switch op {
	case tokGt:
		return a > b
	case tokLt:
		return a < b
	case tokGe:
		return a >= b
	case tokLe:
		return a <= b
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func deepEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	return reflect.DeepEqual(a, b)
}
