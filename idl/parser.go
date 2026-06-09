// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package idl

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ── Lexer ─────────────────────────────────────────────────────────────────────

type tokenKind int

const (
	tokEOF         tokenKind = iota
	tokIdent                 // identifier or keyword
	tokNumber                // integer literal (used for array sizes)
	tokLBrace                // {
	tokRBrace                // }
	tokLAngle                // <
	tokRAngle                // >
	tokLBracket              // [
	tokRBracket              // ]
	tokSemi                  // ;
	tokComma                 // ,
	tokDoubleColon           // ::
	tokAt                    // @
)

type token struct {
	kind tokenKind
	val  string
	line int
}

type lexer struct {
	src  []rune
	pos  int
	line int
}

func newLexer(src string) *lexer {
	return &lexer{src: []rune(src), line: 1}
}

func (l *lexer) peek() (rune, bool) {
	if l.pos >= len(l.src) {
		return 0, false
	}
	return l.src[l.pos], true
}

func (l *lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
	}
	return r
}

func (l *lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		r, ok := l.peek()
		if !ok {
			break
		}
		if unicode.IsSpace(r) {
			l.advance()
			continue
		}
		// Single-line comment
		if r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		// Block comment
		if r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			l.pos += 2
			for l.pos+1 < len(l.src) {
				if l.src[l.pos] == '*' && l.src[l.pos+1] == '/' {
					l.pos += 2
					break
				}
				if l.src[l.pos] == '\n' {
					l.line++
				}
				l.pos++
			}
			continue
		}
		break
	}
}

func (l *lexer) next() token {
	l.skipWhitespaceAndComments()
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, line: l.line}
	}
	r := l.src[l.pos]
	ln := l.line
	switch r {
	case '{':
		l.advance()
		return token{kind: tokLBrace, val: "{", line: ln}
	case '}':
		l.advance()
		return token{kind: tokRBrace, val: "}", line: ln}
	case '<':
		l.advance()
		return token{kind: tokLAngle, val: "<", line: ln}
	case '>':
		l.advance()
		return token{kind: tokRAngle, val: ">", line: ln}
	case '[':
		l.advance()
		return token{kind: tokLBracket, val: "[", line: ln}
	case ']':
		l.advance()
		return token{kind: tokRBracket, val: "]", line: ln}
	case ';':
		l.advance()
		return token{kind: tokSemi, val: ";", line: ln}
	case ',':
		l.advance()
		return token{kind: tokComma, val: ",", line: ln}
	case ':':
		// :: scope-resolution operator (IDL qualified names)
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == ':' {
			l.advance()
			l.advance()
			return token{kind: tokDoubleColon, val: "::", line: ln}
		}
		// single colon — skip
		l.advance()
		return l.next()
	case '@':
		l.advance()
		return token{kind: tokAt, val: "@", line: ln}
	}
	if unicode.IsDigit(r) {
		var b strings.Builder
		for l.pos < len(l.src) {
			ch := l.src[l.pos]
			if unicode.IsDigit(ch) {
				b.WriteRune(ch)
				l.pos++
			} else {
				break
			}
		}
		return token{kind: tokNumber, val: b.String(), line: ln}
	}
	if unicode.IsLetter(r) || r == '_' {
		var b strings.Builder
		for l.pos < len(l.src) {
			ch := l.src[l.pos]
			if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
				b.WriteRune(ch)
				l.pos++
			} else {
				break
			}
		}
		return token{kind: tokIdent, val: b.String(), line: ln}
	}
	// Unknown character — skip
	l.advance()
	return l.next()
}

// ── Parser ────────────────────────────────────────────────────────────────────

type parser struct {
	lx     *lexer
	cur    token
	peeked bool
}

func newParser(src string) *parser {
	p := &parser{lx: newLexer(src)}
	return p
}

func (p *parser) peek() token {
	if !p.peeked {
		p.cur = p.lx.next()
		p.peeked = true
	}
	return p.cur
}

func (p *parser) consume() token {
	t := p.peek()
	p.peeked = false
	return t
}

func (p *parser) expect(kind tokenKind, desc string) (token, error) {
	t := p.consume()
	if t.kind != kind {
		return t, fmt.Errorf("idl: line %d: expected %s, got %q", t.line, desc, t.val)
	}
	return t, nil
}

// parseModule parses the top-level scope or a named module body.
// When name == "" it reads until EOF; otherwise it reads until '}'.
func (p *parser) parseModule() (*Module, error) {
	m := &Module{}
	for {
		t := p.peek()
		if t.kind == tokEOF || t.kind == tokRBrace {
			break
		}
		if t.kind != tokIdent {
			p.consume()
			continue
		}
		switch t.val {
		case "module":
			p.consume()
			sub, err := p.parseNamedModule()
			if err != nil {
				return nil, err
			}
			m.Modules = append(m.Modules, sub)
		case "struct":
			p.consume()
			s, err := p.parseStruct()
			if err != nil {
				return nil, err
			}
			m.Structs = append(m.Structs, *s)
		case "enum":
			p.consume()
			e, err := p.parseEnum()
			if err != nil {
				return nil, err
			}
			m.Enums = append(m.Enums, *e)
		case "typedef":
			p.consume()
			td, err := p.parseTypedef()
			if err != nil {
				return nil, err
			}
			m.Typedefs = append(m.Typedefs, *td)
		default:
			// Unknown keyword or annotation — skip to next ';' or '}'
			p.consume()
			for {
				t2 := p.peek()
				if t2.kind == tokEOF || t2.kind == tokSemi || t2.kind == tokRBrace {
					if t2.kind == tokSemi {
						p.consume()
					}
					break
				}
				p.consume()
			}
		}
	}
	return m, nil
}

func (p *parser) parseNamedModule() (*Module, error) {
	nameTok, err := p.expect(tokIdent, "module name")
	if err != nil {
		return nil, err
	}
	lbrace, braceErr := p.expect(tokLBrace, "{")
	_ = lbrace
	if braceErr != nil {
		return nil, braceErr
	}
	m, err := p.parseModule()
	if err != nil {
		return nil, err
	}
	m.Name = nameTok.val
	rbrace, rbErr := p.expect(tokRBrace, "}")
	_ = rbrace
	if rbErr != nil {
		return nil, rbErr
	}
	// Optional trailing semicolon
	if p.peek().kind == tokSemi {
		p.consume()
	}
	return m, nil
}

func (p *parser) parseStruct() (*Struct, error) {
	nameTok, err := p.expect(tokIdent, "struct name")
	if err != nil {
		return nil, err
	}
	slbrace, err := p.expect(tokLBrace, "{")
	_ = slbrace
	if err != nil {
		return nil, err
	}
	s := &Struct{Name: nameTok.val}
	for {
		t := p.peek()
		if t.kind == tokRBrace || t.kind == tokEOF {
			break
		}
		// Consume leading annotations: @key, @optional, @id(...), etc.
		isKey := false
		for p.peek().kind == tokAt {
			p.consume() // @
			annot := p.consume()
			if annot.val == "key" {
				isKey = true
			}
			// Ignore annotation arguments: @id(N) → consume (N)
			if p.peek().kind == tokLBrace {
				for p.peek().kind != tokRBrace && p.peek().kind != tokEOF {
					p.consume()
				}
				if p.peek().kind == tokRBrace {
					p.consume()
				}
			}
		}
		typeSpec, ferr := p.parseTypeSpec()
		if ferr != nil {
			return nil, ferr
		}
		fieldName, ferr := p.expect(tokIdent, "field name")
		if ferr != nil {
			return nil, ferr
		}
		// Optional single array dimension: T name[N]
		if p.peek().kind == tokLBracket {
			p.consume() // [
			sizeTok := p.consume()
			if sizeTok.kind != tokNumber {
				return nil, fmt.Errorf("idl: line %d: expected array size, got %q", sizeTok.line, sizeTok.val)
			}
			if _, ferr = p.expect(tokRBracket, "]"); ferr != nil {
				return nil, ferr
			}
			size, _ := strconv.Atoi(sizeTok.val)
			elem := typeSpec
			typeSpec = TypeSpec{Kind: KindArray, ElemType: &elem, ArraySize: size}
		}
		semi, ferr := p.expect(tokSemi, ";")
		_ = semi
		if ferr != nil {
			return nil, ferr
		}
		s.Fields = append(s.Fields, Field{Name: fieldName.val, Type: typeSpec, Key: isKey})
	}
	srbrace, err := p.expect(tokRBrace, "}")
	_ = srbrace
	if err != nil {
		return nil, err
	}
	ssemi, err := p.expect(tokSemi, ";")
	_ = ssemi
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (p *parser) parseEnum() (*Enum, error) {
	nameTok, err := p.expect(tokIdent, "enum name")
	if err != nil {
		return nil, err
	}
	_, err = p.expect(tokLBrace, "{")
	if err != nil {
		return nil, err
	}
	e := &Enum{Name: nameTok.val}
	for {
		t := p.peek()
		if t.kind == tokRBrace || t.kind == tokEOF {
			break
		}
		if t.kind == tokIdent {
			p.consume()
			e.Values = append(e.Values, t.val)
		}
		if p.peek().kind == tokComma {
			p.consume()
		}
	}
	if _, err = p.expect(tokRBrace, "}"); err != nil {
		return nil, err
	}
	if _, err = p.expect(tokSemi, ";"); err != nil {
		return nil, err
	}
	return e, nil
}

func (p *parser) parseTypedef() (*Typedef, error) {
	underlying, err := p.parseTypeSpec()
	if err != nil {
		return nil, err
	}
	nameTok, err := p.expect(tokIdent, "typedef alias name")
	if err != nil {
		return nil, err
	}
	if _, err = p.expect(tokSemi, ";"); err != nil {
		return nil, err
	}
	return &Typedef{Name: nameTok.val, Type: underlying}, nil
}

func (p *parser) parseTypeSpec() (TypeSpec, error) {
	t := p.consume()
	if t.kind != tokIdent {
		return TypeSpec{}, fmt.Errorf("idl: line %d: expected type name, got %q", t.line, t.val)
	}
	switch t.val {
	case "boolean":
		return TypeSpec{Kind: KindBoolean}, nil
	case "octet":
		return TypeSpec{Kind: KindOctet}, nil
	case "float":
		return TypeSpec{Kind: KindFloat}, nil
	case "double":
		return TypeSpec{Kind: KindDouble}, nil
	case "string":
		// Consume optional bounded string: string<N>
		if p.peek().kind == tokLAngle {
			p.consume() // <
			p.consume() // bound value — ignored
			if _, err := p.expect(tokRAngle, ">"); err != nil {
				return TypeSpec{}, err
			}
		}
		return TypeSpec{Kind: KindString}, nil
	case "short":
		return TypeSpec{Kind: KindShort}, nil
	case "long":
		// could be "long long"
		if p.peek().val == "long" {
			p.consume()
			return TypeSpec{Kind: KindLongLong}, nil
		}
		return TypeSpec{Kind: KindLong}, nil
	case "unsigned":
		next := p.consume()
		if next.kind != tokIdent {
			return TypeSpec{}, fmt.Errorf("idl: line %d: expected type after 'unsigned', got %q", next.line, next.val)
		}
		switch next.val {
		case "short":
			return TypeSpec{Kind: KindUShort}, nil
		case "long":
			if p.peek().val == "long" {
				p.consume()
				return TypeSpec{Kind: KindULongLong}, nil
			}
			return TypeSpec{Kind: KindULong}, nil
		default:
			return TypeSpec{}, fmt.Errorf("idl: line %d: unknown unsigned type %q", next.line, next.val)
		}
	case "sequence":
		langle, err := p.expect(tokLAngle, "<")
		_ = langle
		if err != nil {
			return TypeSpec{}, err
		}
		elem, err := p.parseTypeSpec()
		if err != nil {
			return TypeSpec{}, err
		}
		// Optional bound: sequence<T, N>
		if p.peek().kind == tokComma {
			p.consume()
			p.consume() // bound value — ignored
		}
		rangle, err := p.expect(tokRAngle, ">")
		_ = rangle
		if err != nil {
			return TypeSpec{}, err
		}
		return TypeSpec{Kind: KindSequence, ElemType: &elem}, nil
	default:
		// Named type — may be qualified: Module::Struct or bare Struct/Enum name.
		// Consume any leading :: segments to form the full qualified name.
		name := t.val
		for p.peek().kind == tokDoubleColon {
			p.consume() // ::
			next := p.consume()
			if next.kind != tokIdent {
				return TypeSpec{}, fmt.Errorf("idl: line %d: expected name after '::', got %q", next.line, next.val)
			}
			name += "::" + next.val
		}
		// Defer struct-vs-enum resolution to the code generator (both use RefName).
		return TypeSpec{Kind: KindStruct, RefName: name}, nil
	}
}

// ── Utilities ─────────────────────────────────────────────────────────────────

// Ensure utf8 import is used (referenced only in test, kept for completeness).
var _ = utf8.RuneLen
