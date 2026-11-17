//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestConstraintContexts pins the build-context table of scripts/inversion-graph/README.md § "Build
// contexts": an expression is a set over native, rust and wasm, never a single bucket.
func TestConstraintContexts(t *testing.T) {
	cases := []struct {
		name    string
		expr    string
		want    contextMask
		wantErr string
	}{
		{"no tag", "", ctxAll, ""},
		{"all token", "all", ctxAll, ""},
		{"not js and wasm", "!(js && wasm)", ctxNative | ctxRust, ""},
		{"not rust and not js-wasm", "!rust && !(js && wasm)", ctxNative, ""},
		{"integration tag", "integration", ctxNative, ""},
		{"goos with not rust", "windows && !rust", ctxNative, ""},
		{"goos alone", "windows", ctxNative | ctxRust, ""},
		{"goos and arch", "android && arm64", ctxNative | ctxRust, ""},
		{"other plain tag", "swinstrcount", ctxNative | ctxRust, ""},
		{"rust tag", "rust", ctxRust, ""},
		{"rust and goos", "rust && windows", ctxRust, ""},
		{"js and wasm", "js && wasm", ctxWasm, ""},
		{"rust or js-wasm", "rust || (js && wasm)", ctxRust | ctxWasm, ""},
		{"not rust", "!rust", ctxNative | ctxWasm, ""},
		{"or of goos", "(windows || linux) && !(js && wasm)", ctxNative | ctxRust, ""},
		{"integration on linux", "integration && linux && !(js && wasm)", ctxNative, ""},
		{"de morgan", "!js || !wasm", ctxNative | ctxRust, ""},
		{"negated goos", "!android", ctxNative | ctxRust, ""},
		{"negated plain tag", "!swinstrcount", ctxNative | ctxRust, ""},
		{"negated rust", "!rust", ctxNative | ctxWasm, ""},
		{"negated js", "!js", ctxNative | ctxRust, ""},
		{"negated wasm", "!wasm", ctxNative | ctxRust, ""},
		{"negated integration", "!integration", ctxRust | ctxWasm, ""},
		{"negated group of known atoms", "!(js && wasm)", ctxNative | ctxRust, ""},
		{"negated group naming a goos is undecidable", "!(windows && !wgpu_nosoftware)", ctxNative | ctxRust, ""},
		{"double negation", "!!rust", ctxRust, ""},
		{"empty set", "wasm && rust", 0, "admits no configured context"},
		{"build line prefix", "//go:build windows", ctxNative | ctxRust, ""},
		{"missing paren", "(windows", 0, "missing )"},
		{"trailing token", "windows )", 0, "unexpected"},
		{"dangling and", "windows &&", 0, "unexpected end"},
		{"dangling or", "!rust ||", 0, "unexpected end"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConstraint(tc.expr)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseConstraint(%q) = %v, want an error containing %q", tc.expr, got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseConstraint(%q) error = %q, want it to contain %q", tc.expr, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseConstraint(%q): %v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("parseConstraint(%q) = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}
}

// TestConstraintTreeExpressions proves parseConstraint agrees with the graph tool's own per-context
// inclusion (scc.awk's ctxmask) on every distinct build constraint in the tree, taken from the tool's
// own tags.tsv. tags.tsv holds 43 distinct values; 42 are build constraints, and the 43rd is a source
// line in cmd/vk-gen/main.go that writes a "//go:build" string for generated code ("tags" records it as
// the empty string because the line begins with tabs). That file imports no module package, so it never
// becomes a record or an edge (records.tsv has no cmd/vk-gen row) and is not a constraint of the graph.
func TestConstraintTreeExpressions(t *testing.T) {
	cases := []struct {
		expr string
		want contextMask // the tool's ctxmask result
	}{
		{`!android && !(js && wasm) && !rust`, ctxNative},
		{`!rust && !(js && wasm)`, ctxNative},
		{`!rust && !(js && wasm) && !(((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !wgpu_nosoftware)`, ctxNative},
		{`!rust && !(js && wasm) && !android`, ctxNative},
		{`!rust && !(js && wasm) && wgpu_nosoftware`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && wgpu_nosoftware`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nogles`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_novulkan`, ctxNative},
		{`darwin && (amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nometal`, ctxNative},
		{`darwin && (amd64 || arm64) && wgpu_moltenvk && !rust && !(js && wasm) && !wgpu_novulkan`, ctxNative},
		{`integration && !(js && wasm)`, ctxNative},
		{`integration && !rust && !(js && wasm)`, ctxNative},
		{`integration && darwin && !rust && !(js && wasm)`, ctxNative},
		{`integration && linux && !(js && wasm)`, ctxNative},
		{`integration && linux && !(js && wasm) && wgpu_gles_force_es`, ctxNative},
		{`linux && !rust`, ctxNative},
		{`windows && (386 || amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nodx12`, ctxNative},
		{`rust`, ctxRust},
		{`rust && android`, ctxRust},
		{`rust && darwin`, ctxRust},
		{`rust && linux && !android`, ctxRust},
		{`rust && windows`, ctxRust},
		{`!(js && wasm)`, ctxNative | ctxRust},
		{`!android && !(js && wasm)`, ctxNative | ctxRust},
		{`!android && !js && !(js && wasm)`, ctxNative | ctxRust},
		{`!swinstrcount`, ctxNative | ctxRust},
		{`!windows && !linux && !darwin && !(js && wasm)`, ctxNative | ctxRust},
		{`(linux || windows) && !(js && wasm)`, ctxNative | ctxRust},
		{`(windows || linux) && !(js && wasm)`, ctxNative | ctxRust},
		{`android && arm64`, ctxNative | ctxRust},
		{`darwin && !(js && wasm)`, ctxNative | ctxRust},
		{`linux`, ctxNative | ctxRust},
		{`linux && !(js && wasm)`, ctxNative | ctxRust},
		{`linux && !(js && wasm) && !wgpu_gles_force_es`, ctxNative | ctxRust},
		{`linux && !(js && wasm) && wgpu_gles_force_es`, ctxNative | ctxRust},
		{`linux && !android && !(js && wasm)`, ctxNative | ctxRust},
		{`swinstrcount`, ctxNative | ctxRust},
		{`windows`, ctxNative | ctxRust},
		{`windows && !(js && wasm)`, ctxNative | ctxRust},
		{`js && wasm`, ctxWasm},
		{`rust || (js && wasm)`, ctxRust | ctxWasm},
	}
	if len(cases) != 42 {
		t.Fatalf("the tree table holds %d rows, want the 42 real constraints of tags.tsv", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			got, err := parseConstraint(tc.expr)
			if err != nil {
				t.Fatalf("parseConstraint(%q): %v", tc.expr, err)
			}
			if got != tc.want {
				t.Fatalf("parseConstraint(%q) = %q, want the tool's %q", tc.expr, got, tc.want)
			}
		})
	}
}

// TestConstraintTreeZeroFixes pins the 14 tree constraints that evaluated to mask 0 under the old
// atom-complement model (validator BLOCKER 2). Each now yields a non-zero set that matches the graph
// tool, so no generated edge of these files is silently accepted by the subset predicate.
func TestConstraintTreeZeroFixes(t *testing.T) {
	cases := []struct {
		expr string
		want contextMask
	}{
		{`!android && !(js && wasm)`, ctxNative | ctxRust},
		{`!android && !(js && wasm) && !rust`, ctxNative},
		{`!android && !js && !(js && wasm)`, ctxNative | ctxRust},
		{`!rust && !(js && wasm) && !android`, ctxNative},
		{`!windows && !linux && !darwin && !(js && wasm)`, ctxNative | ctxRust},
		{`((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nogles`, ctxNative},
		{`((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_novulkan`, ctxNative},
		{`darwin && (amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nometal`, ctxNative},
		{`darwin && (amd64 || arm64) && wgpu_moltenvk && !rust && !(js && wasm) && !wgpu_novulkan`, ctxNative},
		{`linux && !(js && wasm) && !wgpu_gles_force_es`, ctxNative | ctxRust},
		{`linux && !android && !(js && wasm)`, ctxNative | ctxRust},
		{`rust && linux && !android`, ctxRust},
		{`windows && (386 || amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nodx12`, ctxNative},
	}
	if len(cases) != 14 {
		t.Fatalf("the zero-fix table holds %d rows, want the 14 the validator named", len(cases))
	}
	for _, tc := range cases {
		got, err := parseConstraint(tc.expr)
		if err != nil {
			t.Fatalf("parseConstraint(%q): %v", tc.expr, err)
		}
		if got == 0 {
			t.Fatalf("parseConstraint(%q) still yields an empty mask", tc.expr)
		}
		if got != tc.want {
			t.Fatalf("parseConstraint(%q) = %q, want the tool's %q", tc.expr, got, tc.want)
		}
	}
}

// TestConstraintEmptySetIsAnError proves an expression that admits no configured context is a hard error
// quoting the expression, never a mask 0 that a golden row would vacuously cover.
func TestConstraintEmptySetIsAnError(t *testing.T) {
	for _, expr := range []string{"wasm && rust", "rust && !rust", "js && (386 || amd64)"} {
		got, err := parseConstraint(expr)
		if err == nil {
			t.Fatalf("parseConstraint(%q) = %q, want an error for an empty context set", expr, got)
		}
		if !strings.Contains(err.Error(), expr) {
			t.Fatalf("parseConstraint(%q) error = %q, want it to quote the expression", expr, err)
		}
	}
}

// TestGoldenTokens pins the hand-written golden vocabulary (README § "Build contexts": the B side uses
// exactly all, native, rust, wasm and native+rust) and that String round-trips it.
func TestGoldenTokens(t *testing.T) {
	cases := []struct {
		token string
		want  contextMask
	}{
		{"all", ctxAll},
		{"native", ctxNative},
		{"rust", ctxRust},
		{"wasm", ctxWasm},
		{"native+rust", ctxNative | ctxRust},
	}
	for _, tc := range cases {
		got, ok := goldenToken(tc.token)
		if !ok {
			t.Errorf("goldenToken(%q) rejected a golden token", tc.token)
			continue
		}
		if got != tc.want {
			t.Errorf("goldenToken(%q) = %q, want %q", tc.token, got, tc.want)
		}
		if got.String() != tc.token {
			t.Errorf("contextMask(%q).String() = %q, want %q", tc.token, got.String(), tc.token)
		}
	}
	for _, bad := range []string{"", "native+wasm", "rust+wasm", "everything"} {
		if _, ok := goldenToken(bad); ok {
			t.Errorf("goldenToken(%q) accepted a token outside the golden vocabulary", bad)
		}
	}
}

// Fixture paths and rows shared by the claims and subset tests.
const (
	goldenTargetFile   = "scripts/inversion-graph/target-edges.tsv"
	goldenInvertedFile = "scripts/inversion-graph/inverted-edges.tsv"
	claimsPath         = "scripts/inversion-graph/claims.tsv"

	claimsHeader = "golden\tsrc\tdst\tcontext\tstep\n"

	rowDeviceQueue    = "internal/device\tinternal/queue\tall\tN\tdevice_native.go:24\n"
	rowSurfaceRes     = "internal/surface\tinternal/resource\tnative+rust\tN\tsurface_native.go:46\n"
	rowQueueDeviceInv = "internal/queue\tinternal/device\tall\tN\tqueue_native.go:27\n"

	claimDeviceQueue = "target\tinternal/device\tinternal/queue\tall\tS11\n"
)

// writeTree writes files under a fresh temporary directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// failureContains reports whether some finding renders with the given substring.
func failureContains(findings []Finding, substr string) bool {
	for _, f := range findings {
		if strings.Contains(f.String(), substr) {
			return true
		}
	}
	return false
}

// claimCase is one claims.tsv fixture.
type claimCase struct {
	name        string
	target      string // target-edges.tsv data rows
	inverted    string // inverted-edges.tsv data rows
	claims      string // full claims.tsv content
	wantLoadErr string // substring expected from loadClaims; empty means it must parse
	wantFails   []string
}

// TestClaimsFixtures covers the claim side of the §15.2 predicate: a claim names a golden row, no row is
// claimed twice, and phase final leaves no row unclaimed.
func TestClaimsFixtures(t *testing.T) {
	cases := []claimCase{
		{
			name:     "valid open phase",
			target:   rowDeviceQueue,
			inverted: rowQueueDeviceInv,
			claims:   "phase: open\n" + claimsHeader + claimDeviceQueue,
		},
		{
			name:      "duplicate claim",
			target:    rowDeviceQueue,
			inverted:  rowQueueDeviceInv,
			claims:    "phase: open\n" + claimsHeader + claimDeviceQueue + "target\tinternal/device\tinternal/queue\tall\tS9\n",
			wantFails: []string{"row claimed 2 times"},
		},
		{
			name:      "claim names no golden row",
			target:    rowDeviceQueue,
			inverted:  rowQueueDeviceInv,
			claims:    "phase: open\n" + claimsHeader + "target\tinternal/device\tinternal/surface\tall\tS11\n",
			wantFails: []string{"names no golden row"},
		},
		{
			name:      "final phase with an unclaimed row",
			target:    rowDeviceQueue,
			claims:    "phase: final\n" + claimsHeader,
			wantFails: []string{"unclaimed at phase final"},
		},
		{
			name:        "malformed line",
			target:      rowDeviceQueue,
			claims:      "phase: open\n" + claimsHeader + "target\tinternal/device\tall\n",
			wantLoadErr: "want 5 tab-separated columns",
		},
		{
			name:        "step out of range",
			target:      rowDeviceQueue,
			claims:      "phase: open\n" + claimsHeader + "target\tinternal/device\tinternal/queue\tall\tS14\n",
			wantLoadErr: "not S1..S13",
		},
		{
			name:        "unknown context",
			target:      rowDeviceQueue,
			claims:      "phase: open\n" + claimsHeader + "target\tinternal/device\tinternal/queue\tnative+wasm\tS11\n",
			wantLoadErr: "unknown context",
		},
		{
			name:        "missing phase line",
			target:      rowDeviceQueue,
			claims:      claimsHeader + claimDeviceQueue,
			wantLoadErr: "neither open nor final",
		},
		{
			name:        "missing header line",
			target:      rowDeviceQueue,
			claims:      "phase: open\n" + claimDeviceQueue,
			wantLoadErr: "column header line is missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runClaimCase(t, tc) })
	}
}

// runClaimCase builds the fixture, parses it and checks the claim predicate.
func runClaimCase(t *testing.T, tc claimCase) {
	t.Helper()
	root := writeTree(t, map[string]string{
		goldenTargetFile:   "# target rows\n" + tc.target,
		goldenInvertedFile: "# inverted rows\n" + tc.inverted,
		claimsPath:         tc.claims,
	})
	golden, err := loadGolden(root)
	if err != nil {
		t.Fatalf("loadGolden: %v", err)
	}
	claims, err := loadClaims(root)
	if tc.wantLoadErr != "" {
		if err == nil {
			t.Fatalf("loadClaims accepted a malformed file; want an error containing %q", tc.wantLoadErr)
		}
		if !strings.Contains(err.Error(), tc.wantLoadErr) {
			t.Fatalf("loadClaims error = %q, want it to contain %q", err, tc.wantLoadErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("loadClaims: %v", err)
	}
	rep := &Report{Summary: map[string]int{}}
	checkClaims(rep, golden, claims)
	if len(rep.Failures) != len(tc.wantFails) {
		t.Fatalf("checkClaims produced %d failures, want %d: %v", len(rep.Failures), len(tc.wantFails), rep.Failures)
	}
	for _, want := range tc.wantFails {
		if !failureContains(rep.Failures, want) {
			t.Fatalf("no failure contains %q; got %v", want, rep.Failures)
		}
	}
}

// subsetCase is one edges-a.tsv fixture against a golden.
type subsetCase struct {
	name       string
	target     string // target-edges.tsv data rows
	inverted   string // inverted-edges.tsv data rows
	edgesA     string // edges-a.tsv content
	wantFails  []string
	wantBEdges int
}

// TestSubset covers the subset half of §15.2: every generated edge of an extraction package exists in the
// golden for at least the contexts it is generated in; the golden may declare more, never less.
func TestSubset(t *testing.T) {
	cases := []subsetCase{
		{
			name:       "generated edge covered",
			target:     rowDeviceQueue,
			edgesA:     "internal/device\tinternal/queue\tall\tN\n",
			wantBEdges: 1,
		},
		{
			name:       "generated edge not covered",
			target:     rowDeviceQueue,
			edgesA:     "internal/device\tinternal/surface\tall\tN\n",
			wantFails:  []string{"not covered by any golden target row"},
			wantBEdges: 1,
		},
		{
			name:       "golden row narrower than the generated contexts",
			target:     "internal/device\tinternal/queue\tnative\tN\tdevice_native.go:24\n",
			edgesA:     "internal/device\tinternal/queue\t//go:build !(js && wasm)\tN\n",
			wantFails:  []string{"not covered by any golden target row"},
			wantBEdges: 1,
		},
		{
			name:       "inverted row does not cover a generated edge",
			target:     rowSurfaceRes,
			inverted:   rowDeviceQueue,
			edgesA:     "internal/device\tinternal/queue\tall\tN\n",
			wantFails:  []string{"not covered by any golden target row"},
			wantBEdges: 1,
		},
		{
			name:       "non-extraction source ignored",
			target:     rowDeviceQueue,
			edgesA:     "wgpu\tinternal/device\tall\tN\n",
			wantBEdges: 0,
		},
		{
			name:       "malformed row",
			target:     rowDeviceQueue,
			edgesA:     "internal/device\tinternal/queue\n",
			wantFails:  []string{"malformed row"},
			wantBEdges: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runSubsetCase(t, tc) })
	}
}

// runSubsetCase builds the golden and the edges-a.tsv fixture and checks the subset predicate.
func runSubsetCase(t *testing.T, tc subsetCase) {
	t.Helper()
	root := writeTree(t, map[string]string{
		goldenTargetFile:   "# target rows\n" + tc.target,
		goldenInvertedFile: "# inverted rows\n" + tc.inverted,
	})
	golden, err := loadGolden(root)
	if err != nil {
		t.Fatalf("loadGolden: %v", err)
	}
	out := writeTree(t, map[string]string{"edges-a.tsv": tc.edgesA})
	rep := &Report{Summary: map[string]int{}}
	checkSubset(rep, out, golden)
	if len(rep.Failures) != len(tc.wantFails) {
		t.Fatalf("checkSubset produced %d failures, want %d: %v", len(rep.Failures), len(tc.wantFails), rep.Failures)
	}
	for _, want := range tc.wantFails {
		if !failureContains(rep.Failures, want) {
			t.Fatalf("no failure contains %q; got %v", want, rep.Failures)
		}
	}
	if got := rep.Summary["b-edges"]; got != tc.wantBEdges {
		t.Fatalf("summary b-edges = %d, want %d", got, tc.wantBEdges)
	}
}

// importRow renders one imports.tsv row: file<TAB>line<TAB>content.
func importRow(file string, line int, content string) string {
	return file + "\t" + strconv.Itoa(line) + "\t" + content + "\n"
}

// TestImportCommentVsImport proves the reader agrees with parse.awk on what an import is: the doc-comment
// example that internal/hal/allbackends/doc.go carries counts as zero imports, and a real import in the
// same file still counts as one. On the pre-fix reader the comment became a record of
// internal/hal/allbackends importing the root package.
func TestImportCommentVsImport(t *testing.T) {
	docFile := dirInternalHal + "/allbackends/doc.go"
	comment := "//\t\t\"" + modulePath + "\""
	realImport := "\t_ \"" + modulePath + "/" + dirInternalHal + "\""
	out := writeTree(t, map[string]string{
		"testkind.tsv": docFile + "\t" + kindNonTest + "\n",
		"imports.tsv":  importRow(docFile, 32, comment) + importRow(docFile, 7, realImport),
	})

	recs, err := loadImports(out)
	if err != nil {
		t.Fatalf("loadImports: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("loadImports returned %d records, want 1 (the doc comment is not an import): %+v", len(recs), recs)
	}
	got := recs[0]
	if got.dst != dirInternalHal || got.cls != "module" || got.kind != kindNonTest || got.line != 7 {
		t.Fatalf("record = %+v, want dst=%q cls=module kind=%q line=7", got, dirInternalHal, kindNonTest)
	}
}

// TestImportRules pins the import rules with one record per case. The internal cases are the amended R5
// (ADR-070 §15.1, 2026-11-15): internal code may name the public thin hal, and nothing else the ban lists.
func TestImportRules(t *testing.T) {
	cases := []struct {
		name string
		file string
		dst  string
		kind string
		want []string
	}{
		{"internal imports the thin hal", "internal/core/hal_interop.go", pkgHal, kindNonTest, nil},
		{"internal imports the root", "internal/core/x.go", pkgRoot, kindNonTest, []string{"R5"}},
		{"internal imports wgputest", "internal/core/x.go", pkgWgpuTest, kindNonTest, []string{"R5"}},
		{"internal imports a wgputest package", "internal/core/x.go", pkgWgpuTest + "/fake", kindNonTest, []string{"R5"}},
		{"internal imports cmd", "internal/core/x.go", dirCmd + "/vk-test", kindNonTest, []string{"R5"}},
		{"internal imports examples", "internal/core/x.go", dirExamples + "/triangle", kindNonTest, []string{"R5"}},
		{"hal backend imports the thin hal", "internal/hal/vulkan/interop.go", pkgHal, kindNonTest, nil},
		{"wgputest/internal imports internal/hal", dirWgpuTestInternal + "/device/x.go", dirInternalHal, kindNonTest, nil},
		{"wgputest/internal imports the bridge", dirWgpuTestInternal + "/device/x.go", dirWgpuTestBridge, kindNonTest, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runImportRuleCase(t, tc.file, tc.dst, tc.kind, tc.want)
		})
	}
}

// runImportRuleCase drives the rule set with one record and checks its failures.
func runImportRuleCase(t *testing.T, file, dst, kind string, want []string) {
	t.Helper()
	rep := &Report{Summary: map[string]int{}}
	checkImportRecords(rep, []importRec{{file: file, dst: dst, cls: "module", kind: kind}})
	if len(rep.Failures) != len(want) {
		t.Fatalf("checkImportRecords produced %d failures, want %d: %v", len(rep.Failures), len(want), rep.Failures)
	}
	for _, sub := range want {
		if !failureContains(rep.Failures, sub) {
			t.Fatalf("no failure contains %q; got %v", sub, rep.Failures)
		}
	}
}

// TestRootWgpuTestImportExt proves R4-ext both ways: an external test package in the root directory may
// import wgputest and is reported as a note, while a root non-test file and a white-box (INT) test in
// package wgpu still fail. The white-box import is impossible in Go (it would be an import cycle); the
// rule states it anyway.
func TestRootWgpuTestImportExt(t *testing.T) {
	ext := &Report{Summary: map[string]int{}}
	checkImportRecords(ext, []importRec{{file: "queue_bench_test.go", dst: pkgWgpuTest, cls: "module", kind: kindExtTest}})
	if len(ext.Failures) != 0 {
		t.Fatalf("an external root test importing wgputest produced failures: %v", ext.Failures)
	}
	if !failureContains(ext.Notes, "R4-ext") {
		t.Fatalf("no R4-ext note for the external root test: %v", ext.Notes)
	}

	for _, tc := range []struct {
		name string
		kind string
	}{
		{"non-test root file", kindNonTest},
		{"white-box root test", kindIntTest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := &Report{Summary: map[string]int{}}
			checkImportRecords(rep, []importRec{{file: "queue_bench_test.go", dst: pkgWgpuTest, cls: "module", kind: tc.kind}})
			if len(rep.Failures) != 1 || !failureContains(rep.Failures, "R4") {
				t.Fatalf("%s importing wgputest: failures = %v, want one R4 finding", tc.name, rep.Failures)
			}
		})
	}
}

// TestStencilblitChildRules proves internal/stencilblit is a child: a planted stencilblit→internal/device
// import produces child-imports-device. It also pins that the package is deliberately not an extraction
// package (its stencilblit→shaderir edge is A-side, absent from the golden; the subset predicate covers
// extraction packages only).
func TestStencilblitChildRules(t *testing.T) {
	if extractionPackages["internal/stencilblit"] {
		t.Fatal("internal/stencilblit must stay out of extractionPackages")
	}
	src := "//go:build !(js && wasm)\n\npackage stencilblit\n\nimport _ \"" + modulePath + "/" + dirInternalDevice + "\"\n"
	root := writeTree(t, map[string]string{"internal/stencilblit/zz_probe.go": src})
	rep := &Report{Summary: map[string]int{}}
	checkChildren(rep, root)
	if !failureContains(rep.Failures, "child-imports-device") {
		t.Fatalf("a stencilblit→internal/device import produced no child-imports-device finding: %v", rep.Failures)
	}
}

// TestInstanceIsParentOfDevice pins the amended child→device rule in both
// directions (ADR-070 §15.10, amended 2026-11-16): the instance is the device's
// parent — it constructs and releases the device, and the golden's
// internal/instance → internal/device (all) row declares that edge — so a
// planted instance→device import produces no finding. Every other child is
// still a child: the same import in internal/resource reds with
// child-imports-device. The negative control is the point; an exemption without
// it is a hole.
func TestInstanceIsParentOfDevice(t *testing.T) {
	childSource := func(pkg string) string {
		return "//go:build !(js && wasm)\n\npackage " + pkg + "\n\nimport _ \"" + modulePath + "/" + dirInternalDevice + "\"\n"
	}
	root := writeTree(t, map[string]string{
		"internal/instance/zz_probe.go": childSource("instance"),
		"internal/resource/zz_probe.go": childSource("resource"),
	})
	rep := &Report{Summary: map[string]int{}}
	checkChildren(rep, root)

	if failureContains(rep.Failures, "internal/instance/zz_probe.go") {
		t.Fatalf("the device's parent was reported for importing internal/device: %v", rep.Failures)
	}
	if !failureContains(rep.Failures, "internal/resource/zz_probe.go") {
		t.Fatalf("a device child importing internal/device produced no finding: %v", rep.Failures)
	}
	if len(rep.Failures) != 1 {
		t.Fatalf("checkChildren produced %d failures, want the one device-child finding: %v", len(rep.Failures), rep.Failures)
	}
}

// TestCoversEmptyMask proves the vacuous pass is gone: the old predicate `row.mask&0 == 0` matched every
// row, so covers(...,0) was true for any pair. An empty mask now covers nothing.
func TestCoversEmptyMask(t *testing.T) {
	golden := &goldenSet{byPair: map[pairKey][]goldenRow{
		{src: "internal/device", dst: "internal/surface"}: {
			{kind: kindTarget, src: "internal/device", dst: "internal/surface", mask: ctxAll},
		},
	}}
	if golden.covers("internal/device", "internal/surface", 0) {
		t.Fatal("covers(...,0) = true; an empty mask must not cover any row")
	}
	if !golden.covers("internal/device", "internal/surface", ctxNative) {
		t.Fatal("covers(...,native) = false for an all-context row")
	}
}

// TestSubsetEmptyMaskFailsClosed proves the same at the gate: a generated edge whose expression admits no
// configured context is a hard failure naming the expression, not a silent cover. Under the old code the
// mask 0 made covers true and the edge passed; it is red now.
func TestSubsetEmptyMaskFailsClosed(t *testing.T) {
	root := writeTree(t, map[string]string{
		goldenTargetFile:   "internal/device\tinternal/surface\tnative\tN\tprobe.go:9\n",
		goldenInvertedFile: "# none\n",
	})
	golden, err := loadGolden(root)
	if err != nil {
		t.Fatalf("loadGolden: %v", err)
	}
	out := writeTree(t, map[string]string{
		"edges-a.tsv": "internal/device\tinternal/surface\t//go:build wasm && rust\tN\n",
	})
	rep := &Report{Summary: map[string]int{}}
	checkSubset(rep, out, golden)
	if !failureContains(rep.Failures, "admits no configured context") {
		t.Fatalf("an edge with an empty context set was not rejected: %v", rep.Failures)
	}
}

// TestGoldenRowUnparseable proves a golden row whose context is not in the vocabulary is a hard failure,
// not a row with the zero mask.
func TestGoldenRowUnparseable(t *testing.T) {
	root := writeTree(t, map[string]string{
		goldenTargetFile:   "internal/device\tinternal/surface\tbogus\tN\tprobe.go:9\n",
		goldenInvertedFile: "# none\n",
	})
	if _, err := loadGolden(root); err == nil || !strings.Contains(err.Error(), "unknown context") {
		t.Fatalf("loadGolden on an unparseable context = %v, want an unknown-context error", err)
	}
}

// TestSCCEmptyContextFailsClosed proves a context with zero edges is a failed scan, not a silent pass: the
// floors are totals, so a whole context can be lost under them. A genuine cycle still fails with the cycle
// message, and the two are distinct findings.
func TestSCCEmptyContextFailsClosed(t *testing.T) {
	empty := writeTree(t, map[string]string{"scc-after.txt": "== context native — 3 edges ==\n" +
		"  acyclic (no SCC > 1)\n" +
		"== context rust — 0 edges ==\n" +
		"  (none)\n" +
		"== total cycles: 0 ==\n"})
	rep := &Report{Summary: map[string]int{}}
	checkSCCAfter(rep, empty)
	if len(rep.Failures) != 1 {
		t.Fatalf("an empty context produced %d failures, want the one failed-scan finding: %v", len(rep.Failures), rep.Failures)
	}
	if got := rep.Failures[0]; got.Check != "graph-context-empty" || !strings.Contains(got.Message, "context has no edges") {
		t.Fatalf("an empty context reported %s: %q, want graph-context-empty with the no-edges message", got.Check, got.Message)
	}

	bad := writeTree(t, map[string]string{"scc-after.txt": "== context native — 3 edges ==\n" +
		"  CYCLE (2 packages): a <-> b\n" +
		"== total cycles: 1 ==\n"})
	rep = &Report{Summary: map[string]int{}}
	checkSCCAfter(rep, bad)
	if len(rep.Failures) == 0 {
		t.Fatal("a real cycle produced no failure")
	}
	for _, f := range rep.Failures {
		if f.Check == "graph-context-empty" {
			t.Fatalf("a cycle was reported as an empty context: %v", f)
		}
	}
}

// TestRunConfigErrors proves the gate cannot report success when it cannot run at all.
func TestRunConfigErrors(t *testing.T) {
	if _, err := Run(Config{}); err == nil {
		t.Fatal("Run with an empty Root returned no error")
	}
	if _, err := Run(Config{Root: t.TempDir()}); err == nil {
		t.Fatal("Run on a directory without go.mod returned no error")
	}
}

// TestRunMissingClaims proves a missing claims file is a finding with the phase falling back to open, not
// a crash: the window still reports the graph and the syntax-tree checks.
func TestRunMissingClaims(t *testing.T) {
	root := writeTree(t, map[string]string{"go.mod": "module example.com/fixture\n\ngo 1.25\n"})
	rep, err := Run(Config{Root: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Phase != "open" {
		t.Fatalf("phase = %q, want the open fallback", rep.Phase)
	}
	if !failureContains(rep.Failures, "claims.tsv") {
		t.Fatalf("no claims finding in %v", rep.Failures)
	}
}

// requireFromEnv reports whether the comma-separated WGPU_REQUIRE list names the given requirement.
func requireFromEnv(name string) bool {
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.TrimSpace(item) == name {
			return true
		}
	}
	return false
}

// TestRunGate runs the real gate over this module's worktree: the graph tool, its floors and stages, the
// acyclic-after predicate, the golden subset and claims, R1-R12 and the syntax-tree rules. Failures are
// listed one per line; the tolerated findings are logged as the inventory they are.
func TestRunGate(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		if requireFromEnv("layoutcheck") {
			t.Fatalf("WGPU_REQUIRE=layoutcheck, but sh is not available: %v", err)
		}
		t.Skipf("sh is not available; set WGPU_REQUIRE=layoutcheck to make this a failure: %v", err)
	}
	rep, err := Run(Config{Root: filepath.Join("..", ".."), Worktree: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Logf("phase: %s", rep.Phase)
	logSummary(t, rep.Summary)
	logNotes(t, rep.Notes)
	if len(rep.Failures) == 0 {
		t.Logf("gate: 0 failures, %d notes", len(rep.Notes))
		return
	}
	lines := make([]string, 0, len(rep.Failures))
	for _, f := range rep.Failures {
		lines = append(lines, f.String())
	}
	t.Fatalf("gate: %d failures:\n%s", len(rep.Failures), strings.Join(lines, "\n"))
}

// logSummary logs the graph's counters in a stable order.
func logSummary(t *testing.T, summary map[string]int) {
	t.Helper()
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("summary[%s] = %d", k, summary[k])
	}
}

// logNotes logs the tolerated findings grouped by check: the count, the files affected and the first five
// of each, which is the pre-existing-tree inventory the lead reads.
func logNotes(t *testing.T, notes []Finding) {
	t.Helper()
	groups := map[string][]Finding{}
	var order []string
	for _, n := range notes {
		if _, ok := groups[n.Check]; !ok {
			order = append(order, n.Check)
		}
		groups[n.Check] = append(groups[n.Check], n)
	}
	sort.Strings(order)
	for _, check := range order {
		group := groups[check]
		files := noteFiles(group)
		t.Logf("notes[%s]: %d findings in %d files: %s", check, len(group), len(files), strings.Join(files, ", "))
		for i, n := range group {
			if i == 5 {
				t.Logf("notes[%s]: ... %d more", check, len(group)-5)
				break
			}
			t.Logf("notes[%s] %s", check, n.String())
		}
	}
}

// noteFiles returns the distinct files of a note group, sorted.
func noteFiles(group []Finding) []string {
	seen := make(map[string]bool, len(group))
	files := make([]string, 0, len(group))
	for _, n := range group {
		if n.File == "" || seen[n.File] {
			continue
		}
		seen[n.File] = true
		files = append(files, n.File)
	}
	sort.Strings(files)
	return files
}

// The public-layer allowlist tests. publicLayerFixturePath is the file the six reviewed sites live in;
// the fixture below has the same shape and lets the tests move lines without touching the real facade.
const publicLayerFixturePath = "wgputest/wgputest.go"

// publicLayerAllowReason is the one reason the committed list gives for all six sites.
const publicLayerAllowReason = "Option is func(*config); the facade's With* must build closures over config, " +
	"and Trace/Counts must guard their mutex; predates the window; API-level cleanup carded before v1.0"

// publicLayerFixture is a stand-in for the facade: the two Option closures, the two mutex defers and the
// two clone ranges, which are exactly the constructs the allowlist covers.
func publicLayerFixture() string {
	return `package wgputest

type config struct {
	recorder *Recorder
	faults   Faults
}

type Faults struct{}

type Option func(*config)

func WithRecorder(r *Recorder) Option {
	return func(c *config) { c.recorder = r }
}

func WithFaults(f Faults) Option {
	return func(c *config) { c.faults = f }
}

type Mutex struct{}

func (m *Mutex) Lock() {}

func (m *Mutex) Unlock() {}

type Recorder struct {
	mu    Mutex
	trace Trace
}

func (r *Recorder) Trace() Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.trace.clone()
}

func (r *Recorder) Counts() Counts {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Counts{}
}

type Counts struct{}

type Bg struct{}

type Pass struct{}

type Trace struct {
	BindGroups []Bg
	Passes     []Pass
}

func (t *Trace) clone() Trace {
	out := *t
	out.BindGroups = make([]Bg, len(t.BindGroups))
	for i := range t.BindGroups {
		out.BindGroups[i] = t.BindGroups[i]
	}
	out.Passes = make([]Pass, len(t.Passes))
	for i := range t.Passes {
		out.Passes[i] = t.Passes[i]
	}
	return out
}
`
}

// publicLayerExtra is a seventh construct on a line no allowlist entry names.
func publicLayerExtra() string {
	return `
func (t *Trace) Extra() int {
	for range t.Passes {
		return 1
	}
	return 0
}
`
}

// linesWith returns the 1-based line of every line in src that contains marker, in order.
func linesWith(t *testing.T, src, marker string) []int {
	t.Helper()
	var lines []int
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			lines = append(lines, i+1)
		}
	}
	if len(lines) == 0 {
		t.Fatalf("marker %q not found in the fixture", marker)
	}
	return lines
}

// waiverRow renders one waiver data row: path, symbol, construct, reason, owner, card, expires.
func waiverRow(path, symbol, construct, reason, owner, card, expires string) string {
	return strings.Join([]string{path, symbol, construct, reason, owner, card, expires}, "\t")
}

// waiverFile renders a waiver file: a limit header and the given rows.
func waiverFile(limit int, rows ...string) string {
	var b strings.Builder
	b.WriteString("limit: " + strconv.Itoa(limit) + "\n")
	for _, row := range rows {
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

// fixtureWaiverRows names the six constructs of the fixture, in source order: the two Option closures, the
// two mutex defers and the two ranges in clone. The symbols are receiver-qualified where the declaration is a
// method (Trace, Counts and clone), matching the key the checker builds (declSymbol).
func fixtureWaiverRows() []string {
	return []string{
		waiverRow(publicLayerFixturePath, "WithRecorder", "closure", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
		waiverRow(publicLayerFixturePath, "WithFaults", "closure", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
		waiverRow(publicLayerFixturePath, "(*Recorder).Trace", "defer", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
		waiverRow(publicLayerFixturePath, "(*Recorder).Counts", "defer", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
		waiverRow(publicLayerFixturePath, "(*Trace).clone", "range", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
		waiverRow(publicLayerFixturePath, "(*Trace).clone", "range#2", publicLayerAllowReason, "wgpu", "P2-API-032", "permanent"),
	}
}

// TestPublicLayerWaiverExactSites proves the rule both ways on the fixture: without a list the file reports
// the six constructs, and with the six symbol-keyed waivers they all pass at phase final, every one consumed.
func TestPublicLayerWaiverExactSites(t *testing.T) {
	src := publicLayerFixture()
	rows := fixtureWaiverRows()
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  src,
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})

	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 0 {
		t.Fatalf("phase final reports waived sites or a stale waiver: %v", rep.Failures)
	}

	empty := writeTree(t, map[string]string{publicLayerFixturePath: src})
	rep = &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, empty, true)
	if len(rep.Failures) != len(rows) {
		t.Fatalf("without a list the fixture reports %d findings, want the %d waived sites", len(rep.Failures), len(rows))
	}
}

// TestPublicLayerWaiverExactSiteOnly proves a waiver exempts its key only: a seventh construct (the Extra
// range) in the same file still fails at phase final, and the checker names its live line as evidence.
func TestPublicLayerWaiverExactSiteOnly(t *testing.T) {
	src := publicLayerFixture() + publicLayerExtra()
	extra := linesWith(t, src, "for range t.Passes")[0]
	rows := fixtureWaiverRows()
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  src,
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 1 {
		t.Fatalf("phase final reports %d findings, want the one unwaived construct: %v", len(rep.Failures), rep.Failures)
	}
	if got := rep.Failures[0]; got.Line != extra || got.File != publicLayerFixturePath {
		t.Fatalf("the failure names %s:%d, want %s:%d", got.File, got.Line, publicLayerFixturePath, extra)
	}
}

// malformedWaiverCases are rows that must be format errors in every phase.
func malformedWaiverCases() []struct{ name, row string } {
	return []struct{ name, row string }{
		{"too few columns", publicLayerFixturePath + "\tclone\trange\ttoo few"},
		{"empty reason", waiverRow(publicLayerFixturePath, "clone", "range", "", "wgpu", "P2-API-032", "permanent")},
		{"empty owner", waiverRow(publicLayerFixturePath, "clone", "range", "r", "", "P2-API-032", "permanent")},
		{"empty card", waiverRow(publicLayerFixturePath, "clone", "range", "r", "wgpu", "", "permanent")},
		{"bad construct", waiverRow(publicLayerFixturePath, "clone", "loop", "r", "wgpu", "P2-API-032", "permanent")},
		{"bad ordinal", waiverRow(publicLayerFixturePath, "clone", "range#1", "r", "wgpu", "P2-API-032", "permanent")},
		{"bad expires", waiverRow(publicLayerFixturePath, "clone", "range", "r", "wgpu", "P2-API-032", "2026-13-40")},
		{"missing expires", waiverRow(publicLayerFixturePath, "clone", "range", "r", "wgpu", "P2-API-032", "")},
	}
}

// TestPublicLayerWaiverFormatErrorsRed proves every malformed-row class is a format failure in both phases:
// a bad column count, an empty reason/owner/card, an unknown construct, a bad ordinal, and a bad or missing
// expires.
func TestPublicLayerWaiverFormatErrorsRed(t *testing.T) {
	for _, tc := range malformedWaiverCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"internal/layoutcheck/" + allowlistFile: waiverFile(1, tc.row)})
			if _, err := loadAllowlist(root); err == nil {
				t.Fatalf("loadAllowlist accepted %q", tc.row)
			}
			for _, final := range []bool{false, true} {
				rep := &Report{Summary: map[string]int{}}
				checkPublicLayer(rep, root, final)
				if !failureContains(rep.Failures, publicLayerAllowCheck) {
					t.Fatalf("phase final=%v did not red the format rule: %v", final, rep.Failures)
				}
			}
		})
	}
}

// TestPublicLayerWaiverMalformedExemptsNothing proves a malformed row exempts nothing: a row that would
// name a live construct but carries six columns is a format error and the constructs are all still reported.
func TestPublicLayerWaiverMalformedExemptsNothing(t *testing.T) {
	bad := strings.Join([]string{publicLayerFixturePath, "clone", "range", "reason", "wgpu", "P2-API-032"}, "\t")
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  publicLayerFixture(),
		"internal/layoutcheck/" + allowlistFile: waiverFile(1, bad),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	sites := 0
	for _, f := range rep.Failures {
		if f.Check == publicLayerCheck {
			sites++
		}
	}
	if sites != len(fixtureWaiverRows()) {
		t.Fatalf("a malformed row exempted something: %d site findings, want all %d", sites, len(fixtureWaiverRows()))
	}
}

// TestPublicLayerWaiverDuplicateKeyReds proves a duplicate key is a format error in both phases.
func TestPublicLayerWaiverDuplicateKeyReds(t *testing.T) {
	row := waiverRow(publicLayerFixturePath, "clone", "range", "r", "wgpu", "P2-API-032", "permanent")
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  publicLayerFixture(),
		"internal/layoutcheck/" + allowlistFile: waiverFile(2, row, row),
	})
	if _, err := loadAllowlist(root); err == nil {
		t.Fatalf("loadAllowlist accepted a duplicate key")
	}
	for _, final := range []bool{false, true} {
		rep := &Report{Summary: map[string]int{}}
		checkPublicLayer(rep, root, final)
		if !failureContains(rep.Failures, publicLayerAllowCheck) {
			t.Fatalf("phase final=%v did not red the duplicate key: %v", final, rep.Failures)
		}
	}
}

// TestPublicLayerWaiverUnusedEntryReds proves a waiver whose key matches no finding is a failure in every
// phase: F1's consumed set and F4's "every entry fires" in one mechanism.
func TestPublicLayerWaiverUnusedEntryReds(t *testing.T) {
	rows := fixtureWaiverRows()
	rows = append(rows, waiverRow(publicLayerFixturePath, "NoSuchSymbol", "range", "not a live construct", "wgpu", "P2-API-032", "permanent"))
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  publicLayerFixture(),
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})
	for _, final := range []bool{false, true} {
		rep := &Report{Summary: map[string]int{}}
		checkPublicLayer(rep, root, final)
		if !failureContains(rep.Failures, publicLayerUnusedCheck) {
			t.Fatalf("phase final=%v did not red the unused waiver: %v", final, rep.Failures)
		}
	}
}

// TestPublicLayerWaiverOverLimitReds proves an entry count above the `limit:` header fails in both phases.
func TestPublicLayerWaiverOverLimitReds(t *testing.T) {
	rows := fixtureWaiverRows()
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  publicLayerFixture(),
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows)-1, rows...),
	})
	for _, final := range []bool{false, true} {
		rep := &Report{Summary: map[string]int{}}
		checkPublicLayer(rep, root, final)
		if !failureContains(rep.Failures, "exceed the recorded limit") {
			t.Fatalf("phase final=%v did not red the over-limit list: %v", final, rep.Failures)
		}
	}
}

// TestPublicLayerWaiverLineShiftKeepsEntry is the F2 fix: an edit above a construct shifts its line, and a
// symbol-keyed waiver still exempts it. A line-keyed entry would have gone stale here.
func TestPublicLayerWaiverLineShiftKeepsEntry(t *testing.T) {
	src := publicLayerFixture()
	before := linesWith(t, src, "return func(c *config) { c.recorder = r }")[0]
	shifted := strings.Replace(src, "package wgputest\n", "package wgputest\n\n// an edit above every construct\n\nvar shiftPad = 0\n", 1)
	after := linesWith(t, shifted, "return func(c *config) { c.recorder = r }")[0]
	if after <= before {
		t.Fatalf("the fixture did not shift a construct line (%d -> %d)", before, after)
	}
	rows := fixtureWaiverRows()
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  shifted,
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 0 {
		t.Fatalf("a waiver went stale after a line shift: %v", rep.Failures)
	}
}

// TestPublicLayerWaiverDifferentConstructNotExempted proves the key carries the construct kind: a switch
// waiver does not exempt a range on the same symbol, and is itself reported unused.
func TestPublicLayerWaiverDifferentConstructNotExempted(t *testing.T) {
	rows := fixtureWaiverRows()
	rows = append(rows, waiverRow(publicLayerFixturePath, "(*Trace).Extra", "switch", "wrong kind", "wgpu", "P2-API-032", "permanent"))
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  publicLayerFixture() + publicLayerExtra(),
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if !failureContains(rep.Failures, "range loop in (*Trace).Extra") {
		t.Fatalf("a switch waiver exempted a range: %v", rep.Failures)
	}
	if !failureContains(rep.Failures, publicLayerUnusedCheck) {
		t.Fatalf("the wrong-kind waiver was not reported unused: %v", rep.Failures)
	}
}

// TestPublicLayerWaiverSameNameDifferentReceiver is the Amendment-3 P1 regression (consolidated deep review).
// descriptor.go declares eighteen toHAL methods, and the key's symbol used to be the bare method name, so all
// of them shared one key space and one type's reviewed waiver exempted a construct inserted in another. The
// fixture plants M on two receivers: a key on (*A).M must exempt only A's construct, and the bare name M must
// exempt neither, because no declaration's symbol is the bare name any more.
func TestPublicLayerWaiverSameNameDifferentReceiver(t *testing.T) {
	const fixture = "package wgpu\n\n" +
		"type A struct{ N []int }\n\n" +
		"type B struct{ N []int }\n\n" +
		"func (a *A) M() int {\n" +
		"\tfor range a.N {\n" +
		"\t\treturn 1\n" +
		"\t}\n" +
		"\treturn 0\n" +
		"}\n\n" +
		"func (b *B) M() int {\n" +
		"\tfor range b.N {\n" +
		"\t\treturn 1\n" +
		"\t}\n" +
		"\treturn 0\n" +
		"}\n"
	aLine := linesWith(t, fixture, "for range a.N")[0]
	bLine := linesWith(t, fixture, "for range b.N")[0]

	sites := func(findings []Finding) []Finding {
		out := make([]Finding, 0, len(findings))
		for _, f := range findings {
			if f.Check == publicLayerCheck {
				out = append(out, f)
			}
		}
		return out
	}

	t.Run("a qualified key exempts only its receiver", func(t *testing.T) {
		rows := []string{waiverRow("same_receiver.go", "(*A).M", "range",
			"the A receiver's projection", "wgpu", "P2-API-032", "permanent")}
		root := writeTree(t, map[string]string{
			"same_receiver.go":                      fixture,
			"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
		})
		rep := &Report{Summary: map[string]int{}}
		checkPublicLayer(rep, root, true)
		got := sites(rep.Failures)
		if len(got) != 1 || got[0].Line != bLine {
			t.Fatalf("a key on (*A).M reports %v, want only (*B).M's range at line %d (A's is line %d)", got, bLine, aLine)
		}
	})

	t.Run("the bare method name is no key", func(t *testing.T) {
		rows := []string{waiverRow("same_receiver.go", "M", "range",
			"the old bare-name shape", "wgpu", "P2-API-032", "permanent")}
		root := writeTree(t, map[string]string{
			"same_receiver.go":                      fixture,
			"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
		})
		rep := &Report{Summary: map[string]int{}}
		checkPublicLayer(rep, root, true)
		if got := sites(rep.Failures); len(got) != 2 {
			t.Fatalf("a bare-name key exempted a method: %d site findings, want both receivers: %v", len(got), got)
		}
		if !failureContains(rep.Failures, publicLayerUnusedCheck) {
			t.Fatalf("the bare-name waiver was not reported unused: %v", rep.Failures)
		}
	})
}

// TestPublicLayerWaiverRepeatedInit pins the documented ordinal scheme for the one declaration name that may
// legally repeat in a file: two func init() bodies each carry a range. A waiver on the first body's construct
// (init, range) must leave the second body's construct (init, range#2) reported — the per-file, per-kind
// ordinal in AST order is what keeps two init bodies from sharing one slot.
func TestPublicLayerWaiverRepeatedInit(t *testing.T) {
	const fixture = "package wgpu\n\n" +
		"var firstPad = 0\n\n" +
		"func init() {\n" +
		"\tfor range []int{1} {\n" +
		"\t\tfirstPad = 1\n" +
		"\t}\n" +
		"}\n\n" +
		"var secondPad = 0\n\n" +
		"func init() {\n" +
		"\tfor range []int{2} {\n" +
		"\t\tsecondPad = 2\n" +
		"\t}\n" +
		"}\n"
	first := linesWith(t, fixture, "for range []int{1}")[0]
	second := linesWith(t, fixture, "for range []int{2}")[0]
	rows := []string{waiverRow("two_inits.go", "init", "range",
		"the first init's registration loop", "wgpu", "P2-API-032", "permanent")}
	root := writeTree(t, map[string]string{
		"two_inits.go":                          fixture,
		"internal/layoutcheck/" + allowlistFile: waiverFile(len(rows), rows...),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	var got []Finding
	for _, f := range rep.Failures {
		if f.Check == publicLayerCheck {
			got = append(got, f)
		}
	}
	if len(got) != 1 || got[0].Line != second {
		t.Fatalf("a waiver on the first init reports %v, want only the second init's range at line %d (first is %d)",
			got, second, first)
	}
}

// TestPublicLayerGeneratedMarker covers F3: the skip is ast.IsGenerated (the standard generated marker
// before the first non-comment text), never the *_gen.go suffix. A hand-written x_gen.go is checked; a
// marked file is skipped; a marker placed after code does not exempt.
func TestPublicLayerGeneratedMarker(t *testing.T) {
	const banned = "func Gen() int {\n\tfor range []int{1} {\n\t\treturn 1\n\t}\n\treturn 0\n}\n"
	cases := []struct {
		name      string
		file      string
		wantFails int
	}{
		{
			name:      "hand-written x_gen.go is checked",
			file:      "package wgpu\n\n" + banned,
			wantFails: 1,
		},
		{
			name:      "marker before the package skips",
			file:      "// Code generated by test. DO NOT EDIT.\n\npackage wgpu\n\n" + banned,
			wantFails: 0,
		},
		{
			name:      "marker after code does not exempt",
			file:      "package wgpu\n\n// Code generated by test. DO NOT EDIT.\n\n" + banned,
			wantFails: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeTree(t, map[string]string{"x_gen.go": tc.file})
			rep := &Report{Summary: map[string]int{}}
			checkPublicLayer(rep, root, true)
			sites := 0
			for _, f := range rep.Failures {
				if f.Check == publicLayerCheck {
					sites++
				}
			}
			if sites != tc.wantFails {
				t.Fatalf("%s: %d public-layer findings, want %d: %v", tc.name, sites, tc.wantFails, rep.Failures)
			}
		})
	}
}

// TestPublicLayerWGpuTestSites is the phase-final simulation on the real facade: the file holds exactly the
// six reviewed constructs, every one is waived, and with the committed file the file is clean at final.
func TestPublicLayerWGpuTestSites(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, publicLayerFixturePath)
	findings := collectPublicLayerFile(&Report{Summary: map[string]int{}}, root, true, path)
	if len(findings) != 6 {
		lines := make([]string, 0, len(findings))
		for _, f := range findings {
			lines = append(lines, f.key)
		}
		t.Fatalf("the facade holds %d constructs, want the six waived ones:\n%s", len(findings), strings.Join(lines, "\n"))
	}
	listed, err := loadAllowlist(root)
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	for _, f := range findings {
		if !listed.lookup(f.key) {
			t.Errorf("construct %q at %s:%d is not in the waiver file", f.key, f.rel, f.line)
		}
	}
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayerFile(rep, root, listed, true, path)
	if len(rep.Failures) != 0 {
		t.Fatalf("phase final still reports waived constructs: %v", rep.Failures)
	}
}

// TestPublicLayerUnexportedMethodLogic plants the shape the rule must read: a loop and a closure inside
// unexported methods of a public package. The detector is about the package's code, not its exported
// surface, so both red at phase final with the method named.
func TestPublicLayerUnexportedMethodLogic(t *testing.T) {
	src := `package wgputest

type Pass struct{}

type Trace struct {
	Passes []Pass
}

func (t *Trace) peek() int {
	for range t.Passes {
		return 1
	}
	return 0
}

func (t *Trace) each(f func(Pass)) {
	f = func(p Pass) {}
	_ = f
}
`
	root := writeTree(t, map[string]string{publicLayerFixturePath: src})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 2 {
		t.Fatalf("phase final reports %d findings, want the loop and the closure: %v", len(rep.Failures), rep.Failures)
	}
	messages := make([]string, 0, len(rep.Failures))
	for _, f := range rep.Failures {
		messages = append(messages, f.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, want := range []string{"loop in (*Trace).peek", "closure in (*Trace).each"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the findings %q do not name %q", joined, want)
		}
	}
}

// publicLayerMapFixture declares the banned package-scope map in every form the language offers: the
// explicit type, an inferred composite literal, the inferred make call with and without a size, the
// parenthesized literal (gofmt keeps the parentheses) and a conversion to a map type.
func publicLayerMapFixture() string {
	return `package wgputest

var explicit map[string]int

var inferredLiteral = map[string]int{"a": 1}

var inferredMake = make(map[string]int)

var inferredMakeSized = make(map[string]int, 4)

var parenthesizedLiteral = (map[string]int{"b": 2})

var convertedMap = map[string]int(nil)
`
}

// TestPublicLayerPackageScopeMapForms proves the ban reads every form of the construct. Before F-A2 of the
// S13 trial review only the explicit type was flagged: the inferred initializers — the most idiomatic way to
// write the declaration — passed silently. Each declaration now reports one finding at its own line, and
// the delta check's residuals (the parenthesized literal and the map conversion) are covered here.
func TestPublicLayerPackageScopeMapForms(t *testing.T) {
	src := publicLayerMapFixture()
	root := writeTree(t, map[string]string{publicLayerFixturePath: src})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)

	want := make([]int, 0, 6)
	want = append(want, linesWith(t, src, "var explicit map[string]int")...)
	want = append(want, linesWith(t, src, "var inferredLiteral")...)
	want = append(want, linesWith(t, src, "var inferredMake ")...)
	want = append(want, linesWith(t, src, "var inferredMakeSized")...)
	want = append(want, linesWith(t, src, "var parenthesizedLiteral")...)
	want = append(want, linesWith(t, src, "var convertedMap")...)
	if len(rep.Failures) != len(want) {
		t.Fatalf("phase final reports %d findings, want the %d map declarations: %v", len(rep.Failures), len(want), rep.Failures)
	}
	flagged := map[int]bool{}
	for _, f := range rep.Failures {
		if f.File != publicLayerFixturePath || !strings.Contains(f.Message, "package-scope map") {
			t.Fatalf("the finding %q does not describe a package-scope map in %s", f.Message, f.File)
		}
		flagged[f.Line] = true
	}
	for _, line := range want {
		if !flagged[line] {
			t.Errorf("the map declaration at %s:%d is not flagged", publicLayerFixturePath, line)
		}
	}
}

// TestPublicLayerWaiverFile pins the committed waiver file's shape: the recorded limit, the entry count, and
// the card groups the review approved (6 wgputest P2-API-032, 31 conversion P2-API-034/P2-API-035, 4 factory
// ADR-070 §15.13). A changed count is a reviewed decision, not a drive-by edit.
func TestPublicLayerWaiverFile(t *testing.T) {
	listed, err := loadAllowlist(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	if !listed.hasLimit || listed.limit != 41 {
		t.Fatalf("the waiver file records limit %d (present=%v), want 41", listed.limit, listed.hasLimit)
	}
	if len(listed.entries) != listed.limit {
		t.Fatalf("the waiver file holds %d entries, want the recorded limit %d", len(listed.entries), listed.limit)
	}
	groups := map[string]int{}
	for _, e := range listed.entries {
		groups[e.card]++
		if e.owner != "wgpu" {
			t.Errorf("entry %q has owner %q, want wgpu", allowKey(e.path, e.symbol, e.construct), e.owner)
		}
	}
	want := map[string]int{"P2-API-032": 6, "P2-API-034/P2-API-035": 31, "ADR-070 §15.13": 4}
	for card, n := range want {
		if groups[card] != n {
			t.Errorf("card %q carries %d entries, want %d (groups: %v)", card, groups[card], n, groups)
		}
	}
}
