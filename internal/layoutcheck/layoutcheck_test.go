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
