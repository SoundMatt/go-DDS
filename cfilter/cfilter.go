// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package cfilter implements the server-side content-filter predicate
// language used by DDS Content-Filtered Topics (ROADMAP.md, Milestone 15
// "Cloud-Native Runtime" — "Content-Filtered Topics"; see
// dds.NewFilteredSubscriber).
//
// The grammar is a small subset of the OMG DDS-SQL filter expression
// grammar (DDS spec §7.1.3): comparison predicates on JSON object fields,
// combined with AND / OR / NOT and parentheses, plus %0, %1, ... parameter
// placeholders bound at compile time from a separate params slice — the
// same expr/params split used by DDS ContentFilteredTopic.
//
//	expr   = "x > 42 AND status = %0"
//	params = []string{"active"}
//
//	field      = identifier (letters, digits, '_'; must not start with a digit)
//	value      = number | 'single-quoted string' | %N parameter
//	comparison = field op value | value op field
//	op         = "=" | "<>" | "!=" | ">" | ">=" | "<" | "<="
//	predicate  = comparison
//	           | NOT predicate
//	           | predicate AND predicate
//	           | predicate OR predicate
//	           | "(" predicate ")"
//
// A sample's Payload is evaluated by first JSON-decoding it into a
// map[string]any and resolving field identifiers against that map. Fields
// absent from the object, a Payload that does not decode as a JSON object,
// or a comparison between operands of incompatible types (e.g. a number
// against a string) make that comparison — and therefore the whole
// predicate, unless satisfied by an unrelated OR branch — evaluate false.
// Content filtering is a network-load optimisation, never a correctness
// requirement, so failing closed (drop rather than over-deliver) is the
// safe default. Non-JSON payloads are simply never matched; use
// dds.WithFilter for arbitrary binary content instead.
//
// This package has no dependency on the dds package or any transport
// backend so that mock, rtps, and shmem can all import it directly and
// share one implementation of the predicate language — the same
// "identical semantics across backends" convention already used for topic
// wildcard matching (see e.g. rtps.TopicMatches) and QoS enforcement.
package cfilter

//fusa:req REQ-CFILT-001
//fusa:req REQ-CFILT-002
//fusa:req REQ-CFILT-003
//fusa:req REQ-CFILT-004
//fusa:req REQ-CFILT-005

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Expr is a compiled content-filter predicate produced by Parse.
// An *Expr is safe for concurrent use from multiple goroutines: Eval only
// reads the immutable state built by Parse.
type Expr struct {
	src  string
	eval func(fields map[string]any) bool
}

// String returns the original, unparsed expression text.
func (e *Expr) String() string { return e.src }

// Eval reports whether payload satisfies the compiled predicate. payload is
// decoded as a JSON object; a decode failure or a non-object top-level JSON
// value evaluates to false (see package doc).
func (e *Expr) Eval(payload []byte) bool {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		return false
	}
	return e.eval(fields)
}

// Parse compiles expr, a DDS-SQL-like predicate (see package doc), binding
// any %0, %1, ... placeholders to the corresponding params entries. Parse
// returns an error if expr is syntactically invalid or references a
// parameter index outside params.
func Parse(expr string, params []string) (*Expr, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, fmt.Errorf("cfilter: %w", err)
	}
	p := &parser{toks: toks, params: params, src: expr}
	fn, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("cfilter: %w", err)
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("cfilter: unexpected token %q in %q", p.toks[p.pos].text, expr)
	}
	return &Expr{src: expr, eval: fn}, nil
}

// ── Tokenizer ────────────────────────────────────────────────────────────────

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokNumber
	tokString
	tokParam
	tokOp
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
)

type token struct {
	kind tokKind
	text string
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func tokenize(s string) ([]token, error) {
	var toks []token
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{tokLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tokRParen, ")"})
			i++
		case c == '\'':
			j := i + 1
			var sb strings.Builder
			closed := false
			for j < n {
				if s[j] == '\'' {
					if j+1 < n && s[j+1] == '\'' { // '' == escaped literal quote
						sb.WriteByte('\'')
						j += 2
						continue
					}
					closed = true
					break
				}
				sb.WriteByte(s[j])
				j++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated string literal in %q", s)
			}
			toks = append(toks, token{tokString, sb.String()})
			i = j + 1
		case c == '%':
			j := i + 1
			for j < n && isDigit(s[j]) {
				j++
			}
			if j == i+1 {
				return nil, fmt.Errorf("malformed parameter placeholder in %q", s)
			}
			toks = append(toks, token{tokParam, s[i+1 : j]})
			i = j
		case c == '!' && i+1 < n && s[i+1] == '=':
			toks = append(toks, token{tokOp, "!="})
			i += 2
		case c == '<' && i+1 < n && s[i+1] == '>':
			toks = append(toks, token{tokOp, "<>"})
			i += 2
		case c == '<' && i+1 < n && s[i+1] == '=':
			toks = append(toks, token{tokOp, "<="})
			i += 2
		case c == '<':
			toks = append(toks, token{tokOp, "<"})
			i++
		case c == '>' && i+1 < n && s[i+1] == '=':
			toks = append(toks, token{tokOp, ">="})
			i += 2
		case c == '>':
			toks = append(toks, token{tokOp, ">"})
			i++
		case c == '=':
			toks = append(toks, token{tokOp, "="})
			i++
		case (c == '-' || c == '+') && i+1 < n && isDigit(s[i+1]):
			j := i + 1
			for j < n && (isDigit(s[j]) || s[j] == '.') {
				j++
			}
			toks = append(toks, token{tokNumber, s[i:j]})
			i = j
		case isDigit(c):
			j := i + 1
			for j < n && (isDigit(s[j]) || s[j] == '.') {
				j++
			}
			toks = append(toks, token{tokNumber, s[i:j]})
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(s[j]) {
				j++
			}
			word := s[i:j]
			switch strings.ToUpper(word) {
			case "AND":
				toks = append(toks, token{tokAnd, word})
			case "OR":
				toks = append(toks, token{tokOr, word})
			case "NOT":
				toks = append(toks, token{tokNot, word})
			default:
				toks = append(toks, token{tokIdent, word})
			}
			i = j
		default:
			return nil, fmt.Errorf("unexpected character %q in %q", string(c), s)
		}
	}
	return toks, nil
}

// ── Parser ───────────────────────────────────────────────────────────────────

type parser struct {
	toks   []token
	pos    int
	params []string
	src    string
}

func (p *parser) peek() token {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return token{tokEOF, ""}
}

func (p *parser) next() token {
	t := p.peek()
	p.pos++
	return t
}

func (p *parser) parseOr() (func(map[string]any) bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(f map[string]any) bool { return l(f) || r(f) }
	}
	return left, nil
}

func (p *parser) parseAnd() (func(map[string]any) bool, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(f map[string]any) bool { return l(f) && r(f) }
	}
	return left, nil
}

func (p *parser) parseUnary() (func(map[string]any) bool, error) {
	switch p.peek().kind {
	case tokNot:
		p.next()
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(f map[string]any) bool { return !inner(f) }, nil
	case tokLParen:
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("expected ')' in %q", p.src)
		}
		p.next()
		return inner, nil
	default:
		return p.parseComparison()
	}
}

func (p *parser) parseComparison() (func(map[string]any) bool, error) {
	left, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	opTok := p.peek()
	if opTok.kind != tokOp {
		return nil, fmt.Errorf("expected comparison operator, got %q in %q", opTok.text, p.src)
	}
	p.next()
	right, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	op := opTok.text
	return func(f map[string]any) bool {
		lv, lok := left(f)
		rv, rok := right(f)
		if !lok || !rok {
			return false
		}
		return compare(lv, rv, op)
	}, nil
}

// operand resolves to (value, present) given the JSON-decoded field map.
type operand func(map[string]any) (any, bool)

func (p *parser) parseOperand() (operand, error) {
	t := p.next()
	switch t.kind {
	case tokIdent:
		name := t.text
		return func(f map[string]any) (any, bool) {
			v, ok := f[name]
			return v, ok
		}, nil
	case tokNumber:
		v, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q in %q", t.text, p.src)
		}
		return func(map[string]any) (any, bool) { return v, true }, nil
	case tokString:
		v := t.text
		return func(map[string]any) (any, bool) { return v, true }, nil
	case tokParam:
		idx, err := strconv.Atoi(t.text)
		if err != nil {
			return nil, fmt.Errorf("invalid parameter %%%s in %q", t.text, p.src)
		}
		if idx < 0 || idx >= len(p.params) {
			return nil, fmt.Errorf("parameter %%%d out of range (have %d params) in %q", idx, len(p.params), p.src)
		}
		v := paramValue(p.params[idx])
		return func(map[string]any) (any, bool) { return v, true }, nil
	default:
		return nil, fmt.Errorf("expected operand, got %q in %q", t.text, p.src)
	}
}

// paramValue distinguishes a %N parameter's bound text (params are declared
// as []string, matching NewFilteredSubscriber's signature) from a plain
// quoted string literal in expr: a parameter is polymorphic — normalizeParam
// coerces it to a number when its text parses as one, so "x > %0" with
// params=["5"] compares numerically — whereas a literal like 'active' is
// always a string, matching DDS-SQL's typed-literal convention.
type paramValue string

// normalizeParam resolves a paramValue to its effective comparison type: a
// float64 when the parameter text parses as a number, otherwise the plain
// string. Non-parameter operands pass through unchanged.
func normalizeParam(v any) any {
	pv, ok := v.(paramValue)
	if !ok {
		return v
	}
	if f, err := strconv.ParseFloat(string(pv), 64); err == nil {
		return f
	}
	return string(pv)
}

// compare evaluates lv op rv. Numeric comparison applies when both operands
// are JSON numbers (decoded as float64) or numeric parameters; string
// comparison applies when both are strings. Any other type combination
// (bool, nil, nested object/array, or a number-vs-string mismatch) is not
// comparable and evaluates false for every operator (see package doc, "fail
// closed").
func compare(lv, rv any, op string) bool {
	lv, rv = normalizeParam(lv), normalizeParam(rv)
	if lf, lok := lv.(float64); lok {
		if rf, rok := rv.(float64); rok {
			return compareOrdered(lf, rf, op)
		}
		return false
	}
	if ls, lok := lv.(string); lok {
		if rs, rok := rv.(string); rok {
			return compareOrdered(ls, rs, op)
		}
		return false
	}
	return false
}

// orderedValue permits comparison and equality checks over both numeric and
// string operands via a single generic implementation.
type orderedValue interface {
	~float64 | ~string
}

func compareOrdered[T orderedValue](l, r T, op string) bool {
	switch op {
	case "=":
		return l == r
	case "<>", "!=":
		return l != r
	case ">":
		return l > r
	case ">=":
		return l >= r
	case "<":
		return l < r
	case "<=":
		return l <= r
	default:
		return false
	}
}
