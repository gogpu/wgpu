//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// goldenRow is one hand-reviewed design row: an edge the extraction creates (target) or one the
// inversion removes (inverted). ADR-070 §15.2: the golden is design intent, never regenerated from the
// tree.
type goldenRow struct {
	kind    string // "target" or "inverted"
	src     string
	dst     string
	context string
	mask    contextMask
	reason  string
}

// goldenKey identifies a golden row for the claims file.
type goldenKey struct {
	kind    string
	src     string
	dst     string
	context string
}

// pairKey indexes golden rows by their endpoints.
type pairKey struct {
	src string
	dst string
}

// goldenSet is the parsed golden.
type goldenSet struct {
	rows   []goldenRow
	byKey  map[goldenKey]goldenRow
	byPair map[pairKey][]goldenRow
}

// loadGolden parses scripts/inversion-graph/target-edges.tsv and inverted-edges.tsv.
func loadGolden(root string) (*goldenSet, error) {
	dir := filepath.Join(root, "scripts", "inversion-graph")
	g := &goldenSet{byKey: map[goldenKey]goldenRow{}, byPair: map[pairKey][]goldenRow{}}
	for _, src := range []struct {
		file string
		kind string
	}{
		{"target-edges.tsv", kindTarget},
		{"inverted-edges.tsv", kindInverted},
	} {
		rows, err := readGoldenFile(filepath.Join(dir, src.file), src.kind)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := goldenKey{kind: row.kind, src: row.src, dst: row.dst, context: row.context}
			if _, dup := g.byKey[key]; dup {
				return nil, fmt.Errorf("%s: duplicate golden row %s→%s (%s)", src.file, row.src, row.dst, row.context)
			}
			g.rows = append(g.rows, row)
			g.byKey[key] = row
			p := pairKey{src: row.src, dst: row.dst}
			g.byPair[p] = append(g.byPair[p], row)
		}
	}
	if len(g.rows) == 0 {
		return nil, fmt.Errorf("the golden is empty")
	}
	return g, nil
}

// readGoldenFile parses one tab-separated golden file. Five columns: src, dst, context, class, reason.
func readGoldenFile(path, kind string) ([]goldenRow, error) {
	data, err := readTreeFile(path)
	if err != nil {
		return nil, err
	}
	var rows []goldenRow
	for i, line := range strings.Split(string(data), "\n") {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			return nil, fmt.Errorf("%s:%d: want 5 tab-separated columns, got %d", path, lineNo, len(fields))
		}
		mask, ok := goldenToken(fields[2])
		if !ok {
			return nil, fmt.Errorf("%s:%d: unknown context %q", path, lineNo, fields[2])
		}
		if mask == 0 {
			return nil, fmt.Errorf("%s:%d: context %q admits no configured context", path, lineNo, fields[2])
		}
		rows = append(rows, goldenRow{
			kind:    kind,
			src:     strings.TrimSpace(fields[0]),
			dst:     strings.TrimSpace(fields[1]),
			context: strings.TrimSpace(fields[2]),
			mask:    mask,
			reason:  fields[4],
		})
	}
	return rows, nil
}

// covers reports whether some golden target row covers (src, dst) in every context of mask. The golden
// may declare an edge more broadly than the tree generates it; the reverse is the failure. An empty mask
// covers nothing: `row.mask&0 == 0` would make every row a match, the vacuous pass the context model must
// never allow (parseConstraint reports an empty set as an error, and this guard keeps the predicate honest
// even if a caller passes 0).
func (g *goldenSet) covers(src, dst string, mask contextMask) bool {
	if mask == 0 {
		return false
	}
	for _, row := range g.byPair[pairKey{src: src, dst: dst}] {
		if row.kind == kindTarget && row.mask&mask == mask {
			return true
		}
	}
	return false
}

// claimRow is one line of claims.tsv.
type claimRow struct {
	key  goldenKey
	step string
	line int
}

// claimsFile is the parsed claims.tsv.
type claimsFile struct {
	phase string
	rows  []claimRow
}

// stepName matches S1..S13.
var stepName = regexp.MustCompile(`^S(?:[1-9]|1[0-3])$`)

// loadClaims parses scripts/inversion-graph/claims.tsv (format: README.md § claims.tsv).
func loadClaims(root string) (*claimsFile, error) {
	path := filepath.Join(root, "scripts", "inversion-graph", "claims.tsv")
	data, err := readTreeFile(path)
	if err != nil {
		return nil, err
	}

	cf := &claimsFile{}
	header := false
	for i, line := range strings.Split(string(data), "\n") {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "phase:") {
			cf.phase = strings.TrimSpace(strings.TrimPrefix(trimmed, "phase:"))
			continue
		}
		if trimmed == "golden\tsrc\tdst\tcontext\tstep" {
			header = true
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			return nil, fmt.Errorf("%s:%d: want 5 tab-separated columns, got %d", path, lineNo, len(fields))
		}
		kind := strings.TrimSpace(fields[0])
		if kind != kindTarget && kind != kindInverted {
			return nil, fmt.Errorf("%s:%d: golden kind %q", path, lineNo, kind)
		}
		context := strings.TrimSpace(fields[3])
		if _, ok := goldenToken(context); !ok {
			return nil, fmt.Errorf("%s:%d: unknown context %q", path, lineNo, context)
		}
		step := strings.TrimSpace(fields[4])
		if !stepName.MatchString(step) {
			return nil, fmt.Errorf("%s:%d: step %q is not S1..S13", path, lineNo, step)
		}
		cf.rows = append(cf.rows, claimRow{
			key:  goldenKey{kind: kind, src: strings.TrimSpace(fields[1]), dst: strings.TrimSpace(fields[2]), context: context},
			step: step,
			line: lineNo,
		})
	}
	if cf.phase != phaseOpen && cf.phase != phaseFinal {
		return nil, fmt.Errorf("%s: phase %q is neither open nor final", path, cf.phase)
	}
	if !header {
		return nil, fmt.Errorf("%s: the column header line is missing", path)
	}
	return cf, nil
}

// checkClaims enforces the claim side of the §15.2 predicate: a claim names a golden row; no row is
// claimed twice; once the phase is final every golden row is claimed.
func checkClaims(rep *Report, golden *goldenSet, cf *claimsFile) {
	seen := map[goldenKey][]claimRow{}
	for _, row := range cf.rows {
		if _, ok := golden.byKey[row.key]; !ok {
			rep.addFailf("claims", row.key.src+"→"+row.key.dst, "scripts/inversion-graph/claims.tsv", row.line,
				"claim (%s, %s) names no golden row", row.key.kind, row.key.context)
			continue
		}
		seen[row.key] = append(seen[row.key], row)
	}
	for key, rows := range seen {
		if len(rows) > 1 {
			rep.addFailf("claims", key.src+"→"+key.dst, "scripts/inversion-graph/claims.tsv", rows[1].line,
				"row claimed %d times (%s and %s)", len(rows), rows[0].step, rows[1].step)
		}
	}
	if cf.phase == phaseFinal {
		for key := range golden.byKey {
			if len(seen[key]) == 0 {
				rep.addFailf("claims", key.src+"→"+key.dst, "scripts/inversion-graph/claims.tsv", 0,
					"golden row (%s, %s) is unclaimed at phase final", key.kind, key.context)
			}
		}
	}
	rep.Summary["claims"] = len(cf.rows)
	rep.Summary["golden-rows"] = len(golden.rows)
}
