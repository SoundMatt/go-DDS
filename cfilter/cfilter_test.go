// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cfilter_test

//fusa:test REQ-CFILT-001
//fusa:test REQ-CFILT-002
//fusa:test REQ-CFILT-003
//fusa:test REQ-CFILT-004
//fusa:test REQ-CFILT-005

import (
	"testing"

	"github.com/SoundMatt/go-DDS/cfilter"
)

func TestParse_NumericComparison(t *testing.T) {
	e, err := cfilter.Parse("x > 42", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"x": 43}`)) {
		t.Error("expected x=43 > 42 to match")
	}
	if e.Eval([]byte(`{"x": 42}`)) {
		t.Error("expected x=42 > 42 to not match")
	}
	if e.Eval([]byte(`{"x": 41}`)) {
		t.Error("expected x=41 > 42 to not match")
	}
}

func TestParse_StringEquality(t *testing.T) {
	e, err := cfilter.Parse("status = 'active'", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"status": "active"}`)) {
		t.Error("expected status='active' to match")
	}
	if e.Eval([]byte(`{"status": "inactive"}`)) {
		t.Error("expected status='inactive' to not match")
	}
}

func TestParse_AndCombinator(t *testing.T) {
	e, err := cfilter.Parse("x > 42 AND status = 'active'", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := []struct {
		payload string
		want    bool
	}{
		{`{"x": 43, "status": "active"}`, true},
		{`{"x": 43, "status": "inactive"}`, false},
		{`{"x": 1, "status": "active"}`, false},
		{`{"x": 1, "status": "inactive"}`, false},
	}
	for _, c := range cases {
		if got := e.Eval([]byte(c.payload)); got != c.want {
			t.Errorf("Eval(%s) = %v, want %v", c.payload, got, c.want)
		}
	}
}

func TestParse_OrCombinator(t *testing.T) {
	e, err := cfilter.Parse("x < 0 OR x > 100", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"x": -1}`)) {
		t.Error("expected x=-1 to match")
	}
	if !e.Eval([]byte(`{"x": 101}`)) {
		t.Error("expected x=101 to match")
	}
	if e.Eval([]byte(`{"x": 50}`)) {
		t.Error("expected x=50 to not match")
	}
}

func TestParse_NotAndParens(t *testing.T) {
	e, err := cfilter.Parse("NOT (x > 10 AND y > 10)", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Eval([]byte(`{"x": 11, "y": 11}`)) {
		t.Error("expected NOT(11>10 AND 11>10) to be false")
	}
	if !e.Eval([]byte(`{"x": 1, "y": 11}`)) {
		t.Error("expected NOT(1>10 AND 11>10) to be true")
	}
}

func TestParse_Parameters(t *testing.T) {
	e, err := cfilter.Parse("status = %0 AND x > %1", []string{"active", "5"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"status": "active", "x": 6}`)) {
		t.Error("expected params to bind correctly and match")
	}
	if e.Eval([]byte(`{"status": "active", "x": 4}`)) {
		t.Error("expected x=4 not > %1=5 to not match")
	}
}

func TestParse_ParameterOutOfRange(t *testing.T) {
	if _, err := cfilter.Parse("x = %0", nil); err == nil {
		t.Error("expected error for out-of-range parameter")
	}
}

func TestParse_SyntaxErrors(t *testing.T) {
	cases := []string{
		"",
		"x >",
		"x > 1 AND",
		"(x > 1",
		"x >> 1",
		"x > 1)",
		"'unterminated",
		"x > $",
		"x = %",
	}
	for _, expr := range cases {
		if _, err := cfilter.Parse(expr, nil); err == nil {
			t.Errorf("Parse(%q) = nil error, want error", expr)
		}
	}
}

func TestParse_AllOperators(t *testing.T) {
	ops := map[string]struct {
		payload string
		want    bool
	}{
		"x = 5":  {`{"x":5}`, true},
		"x <> 5": {`{"x":5}`, false},
		"x != 5": {`{"x":6}`, true},
		"x > 5":  {`{"x":6}`, true},
		"x >= 5": {`{"x":5}`, true},
		"x < 5":  {`{"x":4}`, true},
		"x <= 5": {`{"x":5}`, true},
	}
	for expr, c := range ops {
		e, err := cfilter.Parse(expr, nil)
		if err != nil {
			t.Fatalf("Parse(%q): %v", expr, err)
		}
		if got := e.Eval([]byte(c.payload)); got != c.want {
			t.Errorf("Eval(%q against %q) = %v, want %v", expr, c.payload, got, c.want)
		}
	}
}

func TestParse_NegativeNumber(t *testing.T) {
	e, err := cfilter.Parse("x > -5", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"x": -1}`)) {
		t.Error("expected x=-1 > -5 to match")
	}
	if e.Eval([]byte(`{"x": -10}`)) {
		t.Error("expected x=-10 > -5 to not match")
	}
}

func TestEval_MissingFieldFailsClosed(t *testing.T) {
	e, err := cfilter.Parse("x > 42", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Eval([]byte(`{"y": 100}`)) {
		t.Error("expected missing field to fail closed (no match)")
	}
}

func TestEval_NonJSONPayloadFailsClosed(t *testing.T) {
	e, err := cfilter.Parse("x > 42", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Eval([]byte("not json")) {
		t.Error("expected non-JSON payload to fail closed (no match)")
	}
	if e.Eval([]byte(`[1,2,3]`)) {
		t.Error("expected non-object JSON payload to fail closed (no match)")
	}
}

func TestEval_TypeMismatchFailsClosed(t *testing.T) {
	e, err := cfilter.Parse("x > 42", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if e.Eval([]byte(`{"x": "not a number"}`)) {
		t.Error("expected string-vs-number comparison to fail closed (no match)")
	}
	if e.Eval([]byte(`{"x": true}`)) {
		t.Error("expected bool-vs-number comparison to fail closed (no match)")
	}
	if e.Eval([]byte(`{"x": null}`)) {
		t.Error("expected null-vs-number comparison to fail closed (no match)")
	}
}

func TestExpr_String(t *testing.T) {
	e, err := cfilter.Parse("x > 42", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := e.String(); got != "x > 42" {
		t.Errorf("String() = %q, want %q", got, "x > 42")
	}
}

func TestParse_EscapedQuoteInString(t *testing.T) {
	e, err := cfilter.Parse("name = 'O''Brien'", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"name": "O'Brien"}`)) {
		t.Error("expected escaped quote to decode correctly")
	}
}

func TestParse_ValueOnLeftOperand(t *testing.T) {
	e, err := cfilter.Parse("42 < x", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"x": 43}`)) {
		t.Error("expected 42 < x=43 to match")
	}
}

func TestParse_WhitespaceAndCaseInsensitiveKeywords(t *testing.T) {
	e, err := cfilter.Parse("  x>1   and   y<10  ", nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !e.Eval([]byte(`{"x": 2, "y": 5}`)) {
		t.Error("expected lowercase 'and' keyword and loose whitespace to parse")
	}
}
