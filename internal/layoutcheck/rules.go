//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// modulePath is this module's import path, spelled as the concatenation of two literals: the graph tool
// (scripts/inversion-graph/graph.sh) scans every tracked line for the module path and counts a quoted
// match as an import, so a single literal would turn this declaration into a false internal→root edge.
// The checker must not appear in the graph it checks.
const modulePath = moduleHost + moduleName

const (
	moduleHost = "github.com/gogpu"
	moduleName = "/wgpu"
)

// importRec is one parsed import statement of the tree.
type importRec struct {
	file string
	line int
	dst  string // module-relative destination: "wgpu", "hal", "internal/core", "wgputest", ... or ""
	cls  string // "stdlib", "external" or "module"
	kind string // "N", "INT" or "EXT"
}

// srcPackage is the package a file belongs to, relative to the module root ("wgpu" for the root).
func srcPackage(file string) string {
	dir := filepath.ToSlash(filepath.Dir(file))
	if dir == "." {
		return pkgRoot
	}
	return dir
}

// quotedPath matches the quoted import path in an imports.tsv row.
var quotedPath = regexp.MustCompile(`"([^"]+)"`)

// classify maps an absolute import path to the rule namespace.
func classify(path string) (dst, cls string) {
	switch {
	case path == modulePath:
		return pkgRoot, "module"
	case strings.HasPrefix(path, modulePath+"/"):
		return strings.TrimPrefix(path, modulePath+"/"), "module"
	default:
		first := path
		if i := strings.IndexByte(first, '/'); i >= 0 {
			first = first[:i]
		}
		if strings.Contains(first, ".") {
			return "", "external"
		}
		return "", "stdlib"
	}
}

// loadImports parses imports.tsv and testkind.tsv into records.
func loadImports(outDir string) ([]importRec, error) {
	kinds, err := loadTestKinds(outDir)
	if err != nil {
		return nil, err
	}
	data, err := readTreeFile(filepath.Join(outDir, "imports.tsv"))
	if err != nil {
		return nil, err
	}
	var recs []importRec
	for _, line := range strings.Split(string(data), "\n") {
		if rec, ok := parseImportRow(line, kinds); ok {
			recs = append(recs, rec)
		}
	}
	return recs, nil
}

// loadTestKinds reads testkind.tsv: a file's package is INT (internal test) or EXT (external test).
func loadTestKinds(outDir string) (map[string]string, error) {
	data, err := readTreeFile(filepath.Join(outDir, "testkind.tsv"))
	if err != nil {
		return nil, err
	}
	kinds := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 2 {
			kinds[fields[0]] = fields[1]
		}
	}
	return kinds, nil
}

// parseImportRow turns one imports.tsv row (`file<TAB>line<TAB>content`) into a record. A content line
// that starts with `//` or `/*` is a comment, not an import: package documentation may quote an import
// example (internal/hal/allbackends/doc.go does). This is parse.awk's rule for the same rows; the two
// readers must agree on what an import is.
func parseImportRow(line string, kinds map[string]string) (importRec, bool) {
	fields := strings.SplitN(line, "\t", 3)
	if len(fields) < 3 {
		return importRec{}, false
	}
	content := fields[2]
	trimmed := strings.TrimLeft(content, " \t")
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
		return importRec{}, false
	}
	m := quotedPath.FindStringSubmatch(content)
	if m == nil {
		return importRec{}, false
	}
	lineNo, err := strconv.Atoi(fields[1])
	if err != nil {
		return importRec{}, false
	}
	dst, cls := classify(m[1])
	kind := kinds[fields[0]]
	if kind == "" {
		kind = kindNonTest
	}
	return importRec{file: fields[0], line: lineNo, dst: dst, cls: cls, kind: kind}, true
}

// checkImports applies the import rules R2–R7, R9 and R10 (ADR-070 R1–R12; the layout report's older
// wording is reconciled with the §15 contract, which is the standard: external test packages are
// test-only consumers and their layering finds are notes — the graph tool records them as edges-ext and
// ADR-070 §15.10 fails a white-box child test, not an external one).
func checkImports(rep *Report, outDir string) {
	recs, err := loadImports(outDir)
	if err != nil {
		rep.addFailf("imports", "", "scripts/inversion-graph/imports.tsv", 0, "read: %v", err)
		return
	}
	checkImportRecords(rep, recs)
}

// checkImportRecords applies every import rule to every record. One function per rule keeps each rule's
// nesting and complexity on its own, and lets the tests drive the rules with records directly.
func checkImportRecords(rep *Report, recs []importRec) {
	for _, rec := range recs {
		src := srcPackage(rec.file)
		blackBox := rec.kind == kindExtTest
		checkThinHalImport(rep, src, rec)
		checkWgpuTestImport(rep, src, rec, blackBox)
		checkRootWgpuTestImport(rep, src, rec, blackBox)
		checkInternalImport(rep, src, rec, blackBox)
		checkHALCoreImport(rep, src, rec, blackBox)
		checkCoreExtractionImport(rep, src, rec)
		checkDescriptorMirrorImport(rep, src, rec)
		checkExampleImport(rep, src, rec)
	}
}

// checkThinHalImport applies R2: the thin hal imports stdlib only.
func checkThinHalImport(rep *Report, src string, rec importRec) {
	if src != pkgHal || rec.kind != kindNonTest || rec.cls == "stdlib" {
		return
	}
	rep.addFailf("R2", "", rec.file, rec.line, "thin hal imports %q; stdlib only", rec.dst)
}

// checkWgpuTestImport applies R3: the wgputest facade holds no implementation (its imports of internal
// packages live in wgputest/internal/**, which may name only the root, gputypes, internal/hal and the
// bridge).
func checkWgpuTestImport(rep *Report, src string, rec importRec, blackBox bool) {
	if src == pkgWgpuTest && !blackBox && inTree(rec.dst, dirInternal) && !inTree(rec.dst, dirWgpuTestInternal) {
		rep.addFailf("R3", "", rec.file, rec.line, "wgputest facade imports %q; move it to wgputest/internal/**", rec.dst)
	}
	if !inTree(src, dirWgpuTestInternal) || !inTree(rec.dst, dirInternal) {
		return
	}
	if inTree(rec.dst, dirInternalHal) || rec.dst == dirWgpuTestBridge {
		return
	}
	rep.addFailf("R3", "", rec.file, rec.line,
		"wgputest/internal imports %q; allowed: root, gputypes, internal/hal, internal/wgputestbridge", rec.dst)
}

// checkRootWgpuTestImport applies R4-ext (the layout report's R4, lead-approved 2026-11-15): a root
// non-test file never imports wgputest; a white-box test in package wgpu never imports it either (Go's
// import-cycle rule forbids it, and the rule states it); an external test package in the root directory
// may — it is a separate test-only package — and the gate reports it as a note.
func checkRootWgpuTestImport(rep *Report, src string, rec importRec, blackBox bool) {
	if src != pkgRoot || !inTree(rec.dst, pkgWgpuTest) {
		return
	}
	if blackBox {
		rep.addNotef("R4-ext", rec.file, rec.line,
			"external test package of the root imports %q (test-only; a white-box package wgpu cannot)", rec.dst)
		return
	}
	rep.addFailf("R4", "", rec.file, rec.line, "the root imports %q; the wgputest bridge is the only seam", rec.dst)
}

// checkInternalImport applies R5: an internal package never imports the root, wgputest, cmd/** or
// examples/**. The public thin `hal` is the one exception (ADR-070 §15.1, amended 2026-11-15): it is a
// declarations-only leaf that the internal code names behind the four public HAL() accessors, and the
// edge cannot create a cycle.
func checkInternalImport(rep *Report, src string, rec importRec, blackBox bool) {
	if !inTree(src, dirInternal) || !r5BannedDestination(rec.dst) {
		return
	}
	if blackBox {
		rep.addNotef("R5-ext", rec.file, rec.line,
			"external test package of %s imports %q (test-only; §15.10 fails white-box only)", src, rec.dst)
		return
	}
	rep.addFailf("R5", "", rec.file, rec.line, "%s imports %q; internal packages never import it", src, rec.dst)
}

// r5BannedDestination reports whether an internal package may not import dst. The thin hal is
// deliberately absent: it is the amendment's single exception.
func r5BannedDestination(dst string) bool {
	return dst == pkgRoot || inTree(dst, pkgWgpuTest) || inTree(dst, dirCmd) || inTree(dst, dirExamples)
}

// checkHALCoreImport applies R6: the HAL never imports internal/core.
func checkHALCoreImport(rep *Report, src string, rec importRec, blackBox bool) {
	if !inTree(src, dirInternalHal) || rec.dst != dirInternalCore {
		return
	}
	if blackBox {
		rep.addNotef("R6-ext", rec.file, rec.line, "external test package of %s imports internal/core", src)
		return
	}
	rep.addFailf("R6", "", rec.file, rec.line, "%s imports internal/core", src)
}

// checkCoreExtractionImport applies R7: internal/core never imports an extraction package.
func checkCoreExtractionImport(rep *Report, src string, rec importRec) {
	if !inTree(src, dirInternalCore) || !extractionPackages[rec.dst] {
		return
	}
	rep.addFailf("R7", "", rec.file, rec.line, "%s imports the extraction package %q", src, rec.dst)
}

// checkDescriptorMirrorImport applies R9: no shared descriptor package.
func checkDescriptorMirrorImport(rep *Report, src string, rec importRec) {
	if !inTree(rec.dst, dirInternalDesc) {
		return
	}
	rep.addFailf("R9", "", rec.file, rec.line,
		"%s imports the shared descriptor package %q; mirrors are package-local", src, rec.dst)
}

// checkExampleImport applies R10: examples construct through the public path; the internal registration
// helper is the one documented exception (docs/ARCHITECTURE.md).
func checkExampleImport(rep *Report, src string, rec importRec) {
	if !inTree(src, dirExamples) || !inTree(rec.dst, dirInternal) || rec.dst == dirInternalAb {
		return
	}
	rep.addFailf("R10", "", rec.file, rec.line,
		"%s imports %q; construct through the public path (MIGRATION-MAP §4)", src, rec.dst)
}
