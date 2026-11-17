//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package layoutcheck enforces the ADR-070 layout contract on this module: the target import graph
// (scripts/inversion-graph), the golden rows and their claims, R1–R12, the public-layer rule and the
// §15.10 fail list.
//
// The graph tool is the source of truth for packages and edges (ADR-070 §14.1: `scripts/inversion-graph/`
// is tracked and is what CI runs). This package runs it, parses its outputs and adds the checks that need
// the Go syntax tree. It never rewrites anything.
//
// The check is phase-aware during the v0.35.0 window: claims.tsv's `phase: open` means golden rows are
// still being claimed by the extraction steps, and the public-layer rule reports rather than fails. The
// S13 sweep sets `phase: final`, and from then an unclaimed row and a public-layer violation fail the
// gate (ADR-070 §15.2, §15.10).
package layoutcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config selects the tree to check.
type Config struct {
	// Root is the module root (the directory holding go.mod).
	Root string
	// Worktree asks graph.sh for the working tree of tracked files instead of HEAD. CI leaves it
	// false: the checked tree is the committed one.
	Worktree bool
}

// Finding is one check result.
type Finding struct {
	Check   string
	Golden  string // golden row or claim involved, when there is one
	File    string
	Line    int
	Message string
}

// String renders a finding for a test log or a CI annotation.
func (f Finding) String() string {
	loc := f.File
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	if loc == "" {
		return fmt.Sprintf("%s: %s", f.Check, f.Message)
	}
	if f.Golden != "" {
		return fmt.Sprintf("%s: %s [%s]: %s", f.Check, loc, f.Golden, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.Check, loc, f.Message)
}

// Report is the full result: failures fail the gate, notes are recorded findings that the current
// phase tolerates (ADR-070 §15.10's phase split).
type Report struct {
	Failures []Finding
	Notes    []Finding

	// Summary carries the graph's counters for the S0.3/S13 records, keyed by the summary.txt label.
	Summary map[string]int
	// Phase is the claims phase the run observed: "open" or "final".
	Phase string
}

// addFailf records a failing finding.
func (r *Report) addFailf(check, golden, file string, line int, format string, args ...any) {
	r.Failures = append(r.Failures, Finding{Check: check, Golden: golden, File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// addNotef records a tolerated finding. A note names no golden row: every current note is a layering or
// public-layer observation, not a claim on a design row.
func (r *Report) addNotef(check, file string, line int, format string, args ...any) {
	r.Notes = append(r.Notes, Finding{Check: check, File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// Run checks the tree and returns the report. A returned error means the check could not run at all
// (for example the module root is missing); a failed check is a Finding, never an error.
func Run(cfg Config) (*Report, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("layoutcheck: Config.Root is empty")
	}
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return nil, fmt.Errorf("layoutcheck: %s does not hold go.mod: %w", abs, err)
	}

	rep := &Report{Summary: map[string]int{}}

	claims, claimsErr := loadClaims(abs)
	if claimsErr != nil {
		rep.addFailf("claims", "", "scripts/inversion-graph/claims.tsv", 0, "%v", claimsErr)
		rep.Phase = phaseOpen
	} else {
		rep.Phase = claims.phase
	}

	outDir, err := runGraph(abs, cfg.Worktree)
	if err != nil {
		rep.addFailf("graph", "", "scripts/inversion-graph/graph.sh", 0, "%v", err)
		return rep, nil
	}
	defer cleanupDir(outDir)

	checkStages(rep, outDir)
	checkSummary(rep, outDir)
	checkSCCAfter(rep, outDir)

	golden, goldenErr := loadGolden(abs)
	if goldenErr != nil {
		rep.addFailf("golden", "", "scripts/inversion-graph", 0, "%v", goldenErr)
		return rep, nil
	}
	checkSubset(rep, outDir, golden)
	if claimsErr == nil {
		checkClaims(rep, golden, claims)
	}

	checkImports(rep, outDir)
	checkAST(rep, abs, rep.Phase)
	checkPublicGoldens(rep, abs)

	return rep, nil
}

// extractionPackages are the destination packages of the root extraction. A generated edge whose source
// is one of these is a B-side edge and must exist in the golden (ADR-070 §15.2).
//
// internal/stencilblit is a child of §15.1/§15.4 but is deliberately absent: its existing
// stencilblit→shaderir edge is A-side (in the tree, not in the golden — the golden holds only the planned
// B-side edges), so requiring the golden to cover every generated stencilblit edge would fail on an edge
// the inversion does not create. The golden's three stencilblit rows (queue→stencilblit,
// encoder→stencilblit, stencilblit→hal) are B-side additions, checked as rows by the claims side, not as a
// source subset. The package still takes the child rules (astcheck.go's childDirectories).
var extractionPackages = map[string]bool{
	"internal/instance":      true,
	"internal/device":        true,
	"internal/resource":      true,
	"internal/pipeline":      true,
	"internal/encoder":       true,
	"internal/staging":       true,
	"internal/pendingwrites": true,
	"internal/queue":         true,
	"internal/surface":       true,
	"internal/logging":       true,
}

// contextMask is a set of the three build contexts the graph is checked in.
type contextMask uint8

const (
	ctxNative contextMask = 1 << iota
	ctxRust
	ctxWasm

	ctxAll = ctxNative | ctxRust | ctxWasm
)

// String renders a context set as a golden token.
func (m contextMask) String() string {
	switch m {
	case ctxAll:
		return tokenAll
	case ctxNative:
		return tokenNative
	case ctxRust:
		return tokenRust
	case ctxWasm:
		return tokenWasm
	case ctxNative | ctxRust:
		return tokenNativeRust
	case ctxNative | ctxWasm:
		return "native+wasm"
	case ctxRust | ctxWasm:
		return "rust+wasm"
	default:
		return fmt.Sprintf("ctx(%d)", uint8(m))
	}
}

// goldenToken maps the hand-written context column to a mask.
func goldenToken(s string) (contextMask, bool) {
	switch strings.TrimSpace(s) {
	case tokenAll:
		return ctxAll, true
	case tokenNative:
		return ctxNative, true
	case tokenRust:
		return ctxRust, true
	case tokenWasm:
		return ctxWasm, true
	case tokenNativeRust:
		return ctxNative | ctxRust, true
	default:
		return 0, false
	}
}
