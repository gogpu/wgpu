//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"fmt"
	"strings"
)

// parseConstraint derives the set of build contexts in which a //go:build expression is true, using the
// graph tool's context rules (scripts/inversion-graph/README.md § "Build contexts", and scc.awk's ctxmask,
// which the graph tool applies to the same expressions): an expression is a set, not a bucket.
//
//   - a positive GOOS, architecture or plain tag holds in the native and rust contexts and never in wasm;
//     `rust` holds only under -tags rust; `js` and `wasm` only in the wasm context; `integration` is native
//     only.
//   - a negated `rust`, `js`, `wasm` or `integration` is the set complement over the three contexts.
//   - a negated GOOS, architecture or plain tag is native+rust: the abstraction cannot decide a GOOS (the
//     native and rust builds target an unspecified platform), so its negation is not the complement — it
//     holds in those builds too.
//   - a negated parenthesised subexpression that names such an undecidable atom is likewise native+rust;
//     one built only from `rust`/`js`/`wasm`/`integration` is the set complement.
//
// An expression whose set is empty admits no configured context. That is an error, never a mask: a zero
// mask would otherwise satisfy any golden row (see goldenSet.covers).
func parseConstraint(expr string) (contextMask, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "//go:build "))
	if expr == "" || expr == tokenAll {
		return ctxAll, nil
	}
	p := &constraintParser{s: expr}
	v, err := p.parseOr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return 0, fmt.Errorf("unexpected %q at %d in %q", p.s[p.pos:], p.pos, p.s)
	}
	if v.mask == 0 {
		return 0, fmt.Errorf("the constraint admits no configured context: %q", expr)
	}
	return v.mask, nil
}

// constraintValue is a parsed subexpression: its context set and whether it names an atom the abstraction
// cannot decide (a GOOS, an architecture or any tag other than rust/js/wasm/integration).
type constraintValue struct {
	mask  contextMask
	plain bool
}

// atomMask is the set of contexts in which a positive build atom is true.
func atomMask(name string) contextMask {
	switch name {
	case "js", tokenWasm:
		return ctxWasm
	case tokenRust:
		return ctxRust
	case atomIntegration:
		return ctxNative
	default:
		// A GOOS, an architecture or any other plain tag: true under native and -tags rust, never wasm.
		return ctxNative | ctxRust
	}
}

// negatedAtomMask is the set of contexts in which a negated build atom is true. A known atom is the set
// complement; a GOOS, an architecture or another plain tag cannot be decided by the abstraction, so its
// negation holds in native and rust as well and is never wasm.
func negatedAtomMask(name string) contextMask {
	switch name {
	case "js", tokenWasm:
		return ctxNative | ctxRust // complement of the wasm context
	case tokenRust:
		return ctxNative | ctxWasm // complement of the rust context
	case atomIntegration:
		return ctxRust | ctxWasm // complement of the native context
	default:
		return ctxNative | ctxRust
	}
}

// isPlainAtom reports whether a build atom is a GOOS, an architecture or another plain tag, which the
// three-context abstraction cannot decide.
func isPlainAtom(name string) bool {
	switch name {
	case "js", tokenWasm, tokenRust, atomIntegration:
		return false
	default:
		return true
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
func (p *constraintParser) parseOr() (constraintValue, error) {
	left, err := p.parseAnd()
	if err != nil {
		return constraintValue{}, err
	}
	for {
		p.skipSpace()
		if !strings.HasPrefix(p.s[p.pos:], "||") {
			return left, nil
		}
		p.pos += 2
		right, err := p.parseAnd()
		if err != nil {
			return constraintValue{}, err
		}
		left.mask |= right.mask
		left.plain = left.plain || right.plain
	}
}

// parseAnd parses `unary ('&&' unary)*`.
func (p *constraintParser) parseAnd() (constraintValue, error) {
	left, err := p.parseUnary()
	if err != nil {
		return constraintValue{}, err
	}
	for {
		p.skipSpace()
		if !strings.HasPrefix(p.s[p.pos:], "&&") {
			return left, nil
		}
		p.pos += 2
		right, err := p.parseUnary()
		if err != nil {
			return constraintValue{}, err
		}
		left.mask &= right.mask
		left.plain = left.plain || right.plain
	}
}

// parseUnary parses `'!' operand | '(' or ')' | atom`.
func (p *constraintParser) parseUnary() (constraintValue, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return constraintValue{}, fmt.Errorf("unexpected end of %q", p.s)
	}
	switch p.s[p.pos] {
	case '!':
		p.pos++
		return p.parseNegated()
	case '(':
		return p.parseGroup()
	default:
		return p.parseAtom()
	}
}

// parseNegated parses the operand of one `!` and returns the set of the whole negation.
func (p *constraintParser) parseNegated() (constraintValue, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return constraintValue{}, fmt.Errorf("unexpected end of %q", p.s)
	}
	switch p.s[p.pos] {
	case '!':
		// A double negation is its operand (a boolean identity).
		p.pos++
		return p.parseUnary()
	case '(':
		inner, err := p.parseGroup()
		if err != nil {
			return constraintValue{}, err
		}
		if inner.plain {
			// The negation of an undecidable atom cannot be decided either.
			return constraintValue{mask: ctxNative | ctxRust, plain: true}, nil
		}
		return constraintValue{mask: ctxAll &^ inner.mask}, nil
	default:
		return p.parseAtomName()
	}
}

// parseGroup parses `'(' or ')'`.
func (p *constraintParser) parseGroup() (constraintValue, error) {
	// p.pos points at '('.
	p.pos++
	v, err := p.parseOr()
	if err != nil {
		return constraintValue{}, err
	}
	p.skipSpace()
	if p.pos >= len(p.s) || p.s[p.pos] != ')' {
		return constraintValue{}, fmt.Errorf("missing ) in %q", p.s)
	}
	p.pos++
	return v, nil
}

// parseAtom parses one identifier and returns its positive set.
func (p *constraintParser) parseAtom() (constraintValue, error) {
	name, err := p.scanAtom()
	if err != nil {
		return constraintValue{}, err
	}
	return constraintValue{mask: atomMask(name), plain: isPlainAtom(name)}, nil
}

// parseAtomName parses one identifier and returns its negated set.
func (p *constraintParser) parseAtomName() (constraintValue, error) {
	name, err := p.scanAtom()
	if err != nil {
		return constraintValue{}, err
	}
	return constraintValue{mask: negatedAtomMask(name), plain: isPlainAtom(name)}, nil
}

// scanAtom reads one identifier.
func (p *constraintParser) scanAtom() (string, error) {
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == ' ' || c == '\t' || c == '(' || c == ')' || c == '!' || c == '&' || c == '|' {
			break
		}
		p.pos++
	}
	if start == p.pos {
		return "", fmt.Errorf("empty atom at %d in %q", start, p.s)
	}
	return p.s[start:p.pos], nil
}
