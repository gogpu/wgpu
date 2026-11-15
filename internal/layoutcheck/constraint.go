//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"fmt"
	"strings"
)

// parseConstraint derives the set of build contexts in which a //go:build expression is true, using the
// graph tool's context rules (scripts/inversion-graph/README.md § "Build contexts"): an expression is a
// set, not a bucket. A GOOS or architecture atom holds in the native and rust contexts and can never be
// wasm; `rust` holds only under -tags rust; `js` and `wasm` only in the wasm context; a plain tag holds
// native+rust; `integration` is native only.
func parseConstraint(expr string) (contextMask, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "//go:build "))
	if expr == "" || expr == tokenAll {
		return ctxAll, nil
	}
	p := &constraintParser{s: expr}
	mask, err := p.parseOr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return 0, fmt.Errorf("unexpected %q at %d in %q", p.s[p.pos:], p.pos, p.s)
	}
	return mask, nil
}

// atomMask is the set of contexts in which a build atom is true.
func atomMask(name string) contextMask {
	switch name {
	case "js", tokenWasm:
		return ctxWasm
	case tokenRust:
		return ctxRust
	case "integration":
		return ctxNative
	default:
		// A GOOS, an architecture or any other plain tag: true under native and -tags rust, never wasm.
		return ctxNative | ctxRust
	}
}

type constraintParser struct {
	s   string
	pos int
}

func (p *constraintParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

// parseOr parses `and ('||' and)*`.
func (p *constraintParser) parseOr() (contextMask, error) {
	left, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if !strings.HasPrefix(p.s[p.pos:], "||") {
			return left, nil
		}
		p.pos += 2
		right, err := p.parseAnd()
		if err != nil {
			return 0, err
		}
		left |= right
	}
}

// parseAnd parses `unary ('&&' unary)*`.
func (p *constraintParser) parseAnd() (contextMask, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if !strings.HasPrefix(p.s[p.pos:], "&&") {
			return left, nil
		}
		p.pos += 2
		right, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		left &= right
	}
}

// parseUnary parses `'!' unary | '(' or ')' | atom`.
func (p *constraintParser) parseUnary() (contextMask, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("unexpected end of %q", p.s)
	}
	switch p.s[p.pos] {
	case '!':
		p.pos++
		mask, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return ctxAll &^ mask, nil
	case '(':
		p.pos++
		mask, err := p.parseOr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing ) in %q", p.s)
		}
		p.pos++
		return mask, nil
	default:
		return p.parseAtom()
	}
}

// parseAtom parses one identifier.
func (p *constraintParser) parseAtom() (contextMask, error) {
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == ' ' || c == '\t' || c == '(' || c == ')' || c == '!' || c == '&' || c == '|' {
			break
		}
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("empty atom at %d in %q", start, p.s)
	}
	return atomMask(p.s[start:p.pos]), nil
}
