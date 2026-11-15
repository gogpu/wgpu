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

// fixtureSites names the six allowlist entries of the fixture, in source order.
func fixtureSites(t *testing.T, src string) []string {
	t.Helper()
	var sites []string
	add := func(markers ...string) {
		for _, marker := range markers {
			for _, line := range linesWith(t, src, marker) {
				sites = append(sites, publicLayerFixturePath+":"+strconv.Itoa(line))
			}
		}
	}
	add("return func(c *config) { c.recorder = r }", "return func(c *config) { c.faults = f }",
		"defer r.mu.Unlock()", "for i := range")
	return sites
}

// allowlistBody renders exact-site entries, one per line.
func allowlistBody(sites []string) string {
	var b strings.Builder
	for _, site := range sites {
		b.WriteString(site)
		b.WriteString("\t")
		b.WriteString(publicLayerAllowReason)
		b.WriteString("\n")
	}
	return b.String()
}

// TestPublicLayerAllowlistExactSites proves the rule both ways on the fixture: without a list the file
// reports the six constructs, and with the exact-site list they all pass at phase final.
func TestPublicLayerAllowlistExactSites(t *testing.T) {
	src := publicLayerFixture()
	sites := fixtureSites(t, src)
	if len(sites) != 6 {
		t.Fatalf("the fixture yields %d sites, want the six the allowlist covers: %v", len(sites), sites)
	}
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  src,
		"internal/layoutcheck/" + allowlistFile: allowlistBody(sites),
	})

	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 0 {
		t.Fatalf("phase final reports listed sites: %v", rep.Failures)
	}

	empty := writeTree(t, map[string]string{publicLayerFixturePath: src})
	rep = &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, empty, true)
	if len(rep.Failures) != len(sites) {
		t.Fatalf("without a list the fixture reports %d findings, want the %d listed sites", len(rep.Failures), len(sites))
	}
}

// TestPublicLayerAllowlistExactSiteOnly proves an entry exempts its line only: a seventh finding on
// another line of the same listed file still fails at phase final.
func TestPublicLayerAllowlistExactSiteOnly(t *testing.T) {
	src := publicLayerFixture() + publicLayerExtra()
	seventh := linesWith(t, src, "for range t.Passes")[0]
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  src,
		"internal/layoutcheck/" + allowlistFile: allowlistBody(fixtureSites(t, src)),
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	if len(rep.Failures) != 1 {
		t.Fatalf("phase final reports %d findings, want the one unlisted site: %v", len(rep.Failures), rep.Failures)
	}
	if got := rep.Failures[0]; got.Line != seventh || got.File != publicLayerFixturePath {
		t.Fatalf("the failure names %s:%d, want %s:%d", got.File, got.Line, publicLayerFixturePath, seventh)
	}
}

// TestPublicLayerAllowlistRejectsFileWideEntry proves a file-wide entry is a format error, in every phase,
// and that it exempts nothing: the six constructs are still reported at phase final.
func TestPublicLayerAllowlistRejectsFileWideEntry(t *testing.T) {
	for _, entry := range []string{
		publicLayerFixturePath + "\tfile-wide, the retired form",
		publicLayerFixturePath + ":0\tzero is not a line",
		publicLayerFixturePath + ":x\tnot a number",
		publicLayerFixturePath + ":12",
	} {
		root := writeTree(t, map[string]string{"internal/layoutcheck/" + allowlistFile: entry + "\n"})
		if _, err := loadAllowlist(root); err == nil {
			t.Fatalf("loadAllowlist accepted %q", entry)
		}
	}

	src := publicLayerFixture()
	root := writeTree(t, map[string]string{
		publicLayerFixturePath:                  src,
		"internal/layoutcheck/" + allowlistFile: publicLayerFixturePath + "\tfile-wide, the retired form\n",
	})
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayer(rep, root, true)
	format, sites := 0, 0
	for _, f := range rep.Failures {
		switch f.Check {
		case publicLayerAllowCheck:
			format++
		case publicLayerCheck:
			sites++
		}
	}
	if format != 1 {
		t.Fatalf("a file-wide entry produced %d format failures, want one: %v", format, rep.Failures)
	}
	if sites != len(fixtureSites(t, src)) {
		t.Fatalf("a file-wide entry exempted something: %d site findings, want all %d", sites, len(fixtureSites(t, src)))
	}
}

// TestPublicLayerWGpuTestSites is the phase-final simulation on the real facade: the file holds exactly the
// six reviewed sites, every one of them is listed, and with the committed list the file is clean at final.
func TestPublicLayerWGpuTestSites(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, publicLayerFixturePath)
	rep := &Report{Summary: map[string]int{}}
	checkPublicLayerFile(rep, root, map[string]string{}, true, path)
	if len(rep.Failures) != 6 {
		lines := make([]string, 0, len(rep.Failures))
		for _, f := range rep.Failures {
			lines = append(lines, f.String())
		}
		t.Fatalf("the facade reports %d unexempted sites, want the six reviewed ones:\n%s", len(rep.Failures), strings.Join(lines, "\n"))
	}
	listed, err := loadAllowlist(root)
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	for _, f := range rep.Failures {
		if listed[f.File+":"+strconv.Itoa(f.Line)] == "" {
			t.Errorf("site %s:%d is not in the allowlist", f.File, f.Line)
		}
	}
	rep = &Report{Summary: map[string]int{}}
	checkPublicLayerFile(rep, root, listed, true, path)
	if len(rep.Failures) != 0 {
		t.Fatalf("phase final still reports listed sites: %v", rep.Failures)
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
	for _, want := range []string{"loop in peek", "closure in each"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the findings %q do not name %q", joined, want)
		}
	}
}

// TestPublicLayerAllowlistFile pins the committed list's shape: six entries, all the facade file, one
// shared reason. A seventh entry is a reviewed decision, not a drive-by exemption.
func TestPublicLayerAllowlistFile(t *testing.T) {
	listed, err := loadAllowlist(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	if len(listed) != 6 {
		t.Fatalf("the allowlist holds %d entries, want the six reviewed sites", len(listed))
	}
	for site, reason := range listed {
		if !strings.HasPrefix(site, publicLayerFixturePath+":") {
			t.Errorf("entry %q is outside %s; the list covers only the reviewed facade sites", site, publicLayerFixturePath)
		}
		if reason != publicLayerAllowReason {
			t.Errorf("entry %q carries a different reason", site)
		}
	}
}
