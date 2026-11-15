//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// stageFiles are the graph tool outputs the gate reads. A stage that produced nothing fails the check
// (ADR-070 §15.10: "a stage whose output is empty").
var stageFiles = []string{
	"imports.tsv",
	"testkind.tsv",
	"records.tsv",
	"edges-a.tsv",
	"edges-after.tsv",
	"pairs.tsv",
	"scc-after.txt",
	"summary.txt",
}

// runGraph executes scripts/inversion-graph/graph.sh into a temporary directory and returns that
// directory. The tool enforces its floors and exits non-zero on a partial scan, so its exit status is the
// first check.
func runGraph(root string, worktree bool) (string, error) {
	dir, err := os.MkdirTemp("", "layoutcheck-graph-")
	if err != nil {
		return "", err
	}
	args := []string{"scripts/inversion-graph/graph.sh"}
	if worktree {
		args = append(args, "--worktree")
	}
	// The two paths are handed to a POSIX shell pipeline, and graph.sh passes the out-dir to awk -v,
	// where a backslash starts an escape sequence: a native Windows path loses its separators in that
	// layer and the stage reads nothing. Slash-separated paths survive on every host.
	args = append(args, filepath.ToSlash(root), filepath.ToSlash(dir))
	cmd := exec.Command("sh", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		cleanupDir(dir)
		return "", fmt.Errorf("graph.sh: %w\n%s", err, tailLines(string(out), 8))
	}
	return dir, nil
}

// checkStages requires every stage output to exist and be non-empty.
func checkStages(rep *Report, outDir string) {
	for _, name := range stageFiles {
		info, err := os.Stat(filepath.Join(outDir, name))
		if err != nil {
			rep.addFailf("graph-stage", "", "scripts/inversion-graph/"+name, 0, "stage output missing: %v", err)
			continue
		}
		if info.Size() == 0 {
			rep.addFailf("graph-stage", "", "scripts/inversion-graph/"+name, 0, "stage output is empty")
		}
	}
}

// summaryLine matches `label : 123   (floor 100)` in summary.txt.
var summaryLine = regexp.MustCompile(`^(.+?)\s*:\s*(\d+)\s+\(floor (\d+)\)$`)

// checkSummary reads summary.txt, records its counters and enforces the floors independently of the
// tool's own exit status (ADR-070 §15.2).
func checkSummary(rep *Report, outDir string) {
	data, err := readTreeFile(filepath.Join(outDir, "summary.txt"))
	if err != nil {
		rep.addFailf("graph-floors", "", "scripts/inversion-graph/summary.txt", 0, "read: %v", err)
		return
	}
	floors := 0
	for _, line := range strings.Split(string(data), "\n") {
		m := summaryLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		floor, _ := strconv.Atoi(m[3])
		rep.Summary[m[1]] = n
		floors++
		if n < floor {
			rep.addFailf("graph-floors", "", "scripts/inversion-graph/summary.txt", 0, "%s = %d is below the floor %d", m[1], n, floor)
		}
	}
	if floors < 5 {
		rep.addFailf("graph-floors", "", "scripts/inversion-graph/summary.txt", 0, "only %d floored counters found; the scan is partial", floors)
	}
}

// checkSCCAfter requires the post-inversion graph to be acyclic in every context (ADR-070 §15.2).
func checkSCCAfter(rep *Report, outDir string) {
	data, err := readTreeFile(filepath.Join(outDir, "scc-after.txt"))
	if err != nil {
		rep.addFailf("graph-scc", "", "scripts/inversion-graph/scc-after.txt", 0, "read: %v", err)
		return
	}
	lines := strings.Split(string(data), "\n")
	totalSeen := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "== context"):
			checkContextAcyclic(rep, trimmed, lines[i+1:])
		case strings.HasPrefix(trimmed, "== total cycles:"):
			totalSeen = true
			if trimmed != "== total cycles: 0 ==" {
				rep.addFailf("graph-scc", "", "scripts/inversion-graph/scc-after.txt", 0, "%s", trimmed)
			}
		}
	}
	if !totalSeen {
		rep.addFailf("graph-scc", "", "scripts/inversion-graph/scc-after.txt", 0, "no total-cycles line; the run is incomplete")
	}
}

// checkContextAcyclic reports a context whose next non-empty line is not the acyclic marker.
func checkContextAcyclic(rep *Report, contextLine string, rest []string) {
	for _, line := range rest {
		next := strings.TrimSpace(line)
		if next == "" {
			continue
		}
		if !strings.Contains(next, "acyclic") {
			rep.addFailf("graph-scc", "", "scripts/inversion-graph/scc-after.txt", 0, "%s: not acyclic (%s)", contextLine, next)
		}
		return
	}
}

// checkSubset enforces the §15.2 subset rule: every generated B-side edge (an edge whose source is one of
// the extraction packages) must exist in the golden for at least the contexts it is generated in.
func checkSubset(rep *Report, outDir string, golden *goldenSet) {
	data, err := readTreeFile(filepath.Join(outDir, "edges-a.tsv"))
	if err != nil {
		rep.addFailf("golden-subset", "", "scripts/inversion-graph/edges-a.tsv", 0, "read: %v", err)
		return
	}
	seen := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			rep.addFailf("golden-subset", "", "scripts/inversion-graph/edges-a.tsv", 0, "malformed row %q", line)
			continue
		}
		src, dst, expr := f[0], f[1], f[2]
		if !extractionPackages[src] {
			continue
		}
		seen++
		mask, err := parseConstraint(expr)
		if err != nil {
			rep.addFailf("golden-subset", "", "scripts/inversion-graph/edges-a.tsv", 0, "edge %s→%s: %v", src, dst, err)
			continue
		}
		if !golden.covers(src, dst, mask) {
			rep.addFailf("golden-subset", src+"→"+dst, "scripts/inversion-graph/target-edges.tsv", 0,
				"generated edge %s→%s (%s) is not covered by any golden target row", src, dst, mask)
		}
	}
	rep.Summary["b-edges"] = seen
}

// tailLines keeps the last n lines of a command's output for an error message.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
