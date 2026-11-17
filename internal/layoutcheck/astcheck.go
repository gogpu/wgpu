//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// publicPackages are the directories of the three public packages.
var publicPackages = []string{"", pkgHal, pkgWgpuTest}

// childDirectories are the extraction packages. A check that needs a child skips a directory that does not
// exist yet: before its step creates the package the rule is vacuous, not failing. internal/stencilblit is
// a child of §15.1/§15.4 but not an extraction package: its existing stencilblit→shaderir edge is A-side
// (present in the tree, absent from the golden), and the golden's three stencilblit rows are B-side
// additions. It therefore takes the child rules (var _, child→device, white-box test) but not the subset
// predicate, which covers extraction packages only (see extractionPackages).
var childDirectories = []string{
	"internal/instance",
	"internal/device",
	"internal/resource",
	"internal/pipeline",
	"internal/encoder",
	"internal/staging",
	"internal/pendingwrites",
	"internal/queue",
	"internal/surface",
	"internal/logging",
	"internal/stencilblit",
}

// checkAST applies the syntax-tree checks: the public alias ban, the child rules and the public-layer rule
// (ADR-070 §15.1, §15.6, §15.10; the layout report's public-layer rule).
func checkAST(rep *Report, root, phase string) {
	checkPublicAliases(rep, root)
	checkChildren(rep, root)
	checkPublicLayer(rep, root, phase == phaseFinal)
}

// goFiles lists the .go files of one directory.
func goFiles(dir string, includeTests bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // a directory the tree does not hold yet: the rule that needs it is vacuous
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if !includeTests && strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// parseFile parses one file. Comments are kept because the public-layer skip reads the standard generated
// marker with ast.IsGenerated.
func parseFile(path string) (*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ParseComments)
	return f, fset, err
}

// relPath renders a path relative to the module root, slash-separated.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// checkPublicAliases enforces §15.1: a public package holds no `type X = ...`.
func checkPublicAliases(rep *Report, root string) {
	for _, dir := range publicPackages {
		for _, path := range goFiles(filepath.Join(root, dir), false) {
			f, fset, err := parseFile(path)
			if err != nil {
				rep.addFailf("ast-parse", "", relPath(root, path), 0, "%v", err)
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok || !ts.Assign.IsValid() {
					return true
				}
				rep.addFailf("public-alias", "", relPath(root, path), fset.Position(ts.Pos()).Line,
					"type %s = ...; a public package holds defined types only (§15.1)", ts.Name.Name)
				return true
			})
		}
	}
}

// importsPath reports whether a file imports the given path.
func importsPath(f *ast.File, path string) bool {
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"`+path+`"` {
			return true
		}
	}
	return false
}

// checkChildren enforces the child rules: no package-scope `var _`, no
// device-child→device import, and no white-box child test importing device
// (§15.6, §15.10).
func checkChildren(rep *Report, root string) {
	for _, dir := range childDirectories {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		checkChildDirectory(rep, root, dir, abs)
	}
}

// checkChildDirectory applies the child rules to every file of one child package.
func checkChildDirectory(rep *Report, root, dir, abs string) {
	for _, path := range goFiles(abs, true) {
		f, fset, err := parseFile(path)
		if err != nil {
			rep.addFailf("ast-parse", "", relPath(root, path), 0, "%v", err)
			continue
		}
		rel := relPath(root, path)
		if strings.HasSuffix(path, "_test.go") {
			checkWhiteBoxChildTest(rep, dir, rel, f, fset)
			continue
		}
		checkChildAssertions(rep, dir, rel, f, fset)
		checkChildDeviceImport(rep, dir, rel, f, fset)
	}
}

// checkChildAssertions enforces §15.6: a child declares no package-scope `var _`.
func checkChildAssertions(rep *Report, dir, rel string, f *ast.File, fset *token.FileSet) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			checkValueSpecAssertion(rep, dir, rel, spec, fset)
		}
	}
}

// checkValueSpecAssertion reports every `_` name of one var declaration.
func checkValueSpecAssertion(rep *Report, dir, rel string, spec ast.Spec, fset *token.FileSet) {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok {
		return
	}
	for _, name := range vs.Names {
		if name.Name == "_" {
			rep.addFailf("var-assertion-in-child", "", rel, fset.Position(name.Pos()).Line,
				"%s declares var _ at package scope; the compile check is the call-site assignment (§15.6)", dir)
		}
	}
}

// checkChildDeviceImport enforces §15.3 for a child of the device: the seam is
// declared in the child and typed by core, so a child never imports
// internal/device.
//
// The instance is the device's parent, not its child (ADR-070 §15.10, amended
// 2026-11-16): it constructs the device (device.New, SetInstance) and releases
// it before its own core instance, and the golden's internal/instance →
// internal/device (all) row declares that edge as designed. The rule therefore
// names the direction — a child of the device never imports it — instead of
// exempting a package: the parent/child distinction is the rule's own purpose,
// and it cannot be joined by another child. The both-directions test
// (TestInstanceIsParentOfDevice) keeps the teeth: internal/resource or any other
// child importing internal/device still fails.
func checkChildDeviceImport(rep *Report, dir, rel string, f *ast.File, fset *token.FileSet) {
	if dir == dirInternalInstance {
		return
	}
	if !importsPath(f, modulePath+"/"+dirInternalDevice) {
		return
	}
	rep.addFailf("child-imports-device", "", rel, fset.Position(f.Pos()).Line,
		"%s imports internal/device; the seam is declared in the child and typed by core (§15.3)", dir)
}

// checkWhiteBoxChildTest enforces §15.10: a white-box test of a child drives the seam with a fake and
// never imports internal/device.
func checkWhiteBoxChildTest(rep *Report, dir, rel string, f *ast.File, fset *token.FileSet) {
	if f.Name.Name != filepath.Base(dir) || !importsPath(f, modulePath+"/"+dirInternalDevice) {
		return
	}
	rep.addFailf("whitebox-child-test-imports-device", "", rel, fset.Position(f.Pos()).Line,
		"a white-box test of %s imports internal/device; drive the seam with a fake (§15.10)", dir)
}

// allowlistFile is the explicit public-layer waiver file, relative to internal/layoutcheck.
const allowlistFile = "publiclayer_allow.txt"

// allowlistRel is the waiver file's module-relative path, used in every error message so a reader chasing a
// CI annotation opens the file that exists.
const allowlistRel = dirInternal + "/layoutcheck/" + allowlistFile

// allowEntry is one parsed waiver row. The key is path + symbol + construct, where symbol is the enclosing
// declaration (receiver-qualified for a method, see declSymbol); the other columns carry the review evidence
// the gate prints and the CI audit reads (owner, card, expires).
type allowEntry struct {
	path      string
	symbol    string
	construct string
	reason    string
	owner     string
	card      string
	expires   string
}

// allowlist is the parsed waiver file: the entries by key and the recorded count limit.
type allowlist struct {
	entries  map[string]allowEntry
	limit    int
	hasLimit bool
}

// allowKey builds the waiver key: path + symbol + construct, the symbol being the enclosing declaration as
// declSymbol renders it. The LINE is deliberately not part of it: an edit above a flagged construct shifts its
// line, and a line-keyed entry would then either go stale undetected or exempt whatever lands on the old line
// (PRACTICES-CONSENSUS O1; eco-architect F2). The checker prints the live file:line as evidence in its report
// instead.
func allowKey(path, symbol, construct string) string {
	return path + "\t" + symbol + "\t" + construct
}

// lookup reports whether the waiver file names key. A nil or empty list exempts nothing.
func (a *allowlist) lookup(key string) bool {
	if a == nil {
		return false
	}
	_, ok := a.entries[key]
	return ok
}

// allowlistColumns is the data-row column count: path, symbol, construct, reason, owner, card, expires.
const allowlistColumns = 7

// constructKinds are the construct identifiers a waiver key may carry. They mirror the rule's construct ban.
// `for` is a plain loop (ForStmt); it is its own kind so a plain loop is never exempted by a range entry.
var constructKinds = map[string]bool{
	"for":        true,
	"range":      true,
	"switch":     true,
	"typeswitch": true,
	"select":     true,
	"defer":      true,
	"go":         true,
	"closure":    true,
	"mapliteral": true,
	"packagemap": true,
}

// loadAllowlist reads and validates the waiver file. A missing file is an empty list that exempts nothing and
// is not an error (the rule then fails closed: more findings, never fewer). A present file must carry one
// `limit: N` header and well-formed data rows. A malformed row, a duplicate key, an empty reason or a bad
// expires field is a format error that fails in every phase, while the well-formed rows still apply, so a typo
// never exempts more than a reviewed entry and never costs the entries that are right.
func loadAllowlist(root string) (*allowlist, error) {
	out := &allowlist{entries: map[string]allowEntry{}}
	data, err := readTreeFile(filepath.Join(root, dirInternal, "layoutcheck", allowlistFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, fmt.Errorf("%s: %w", allowlistRel, err)
	}
	var errs []error
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if err := loadAllowline(out, line); err != nil {
			errs = append(errs, fmt.Errorf("%s:%d: %w", allowlistRel, i+1, err))
		}
	}
	if !out.hasLimit {
		errs = append(errs, fmt.Errorf("%s: missing the `limit: N` header; the count is gated, not implied", allowlistRel))
	}
	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

// loadAllowline parses one non-comment line: the single `limit:` header or one data row.
func loadAllowline(out *allowlist, line string) error {
	if strings.HasPrefix(strings.TrimSpace(line), "limit:") {
		return loadLimitLine(out, line)
	}
	entry, err := parseAllowEntry(line)
	if err != nil {
		return err
	}
	key := allowKey(entry.path, entry.symbol, entry.construct)
	if _, dup := out.entries[key]; dup {
		return fmt.Errorf("duplicate key %q; one key, one reviewed entry", key)
	}
	out.entries[key] = entry
	return nil
}

// loadLimitLine parses the single `limit: N` header.
func loadLimitLine(out *allowlist, line string) error {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "limit:"))
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 {
		return fmt.Errorf("limit: %q is not a non-negative integer", rest)
	}
	if out.hasLimit {
		return fmt.Errorf("duplicate `limit:` header")
	}
	out.limit = n
	out.hasLimit = true
	return nil
}

// parseAllowEntry splits one data row into its seven columns and validates them.
func parseAllowEntry(line string) (allowEntry, error) {
	fields := strings.Split(line, "\t")
	if len(fields) != allowlistColumns {
		return allowEntry{}, fmt.Errorf("want %d tab-separated columns (path, symbol, construct, reason, owner, card, expires); got %d", allowlistColumns, len(fields))
	}
	e := allowEntry{
		path:      strings.TrimSpace(fields[0]),
		symbol:    strings.TrimSpace(fields[1]),
		construct: strings.TrimSpace(fields[2]),
		reason:    strings.TrimSpace(fields[3]),
		owner:     strings.TrimSpace(fields[4]),
		card:      strings.TrimSpace(fields[5]),
		expires:   strings.TrimSpace(fields[6]),
	}
	switch {
	case e.path == "":
		return allowEntry{}, fmt.Errorf("empty path")
	case e.symbol == "":
		return allowEntry{}, fmt.Errorf("empty symbol; init and package-scope names are symbols too")
	case !validConstruct(e.construct):
		return allowEntry{}, fmt.Errorf("construct %q is not a kind or kind#N with N >= 2", e.construct)
	case e.reason == "":
		return allowEntry{}, fmt.Errorf("empty reason; a waiver states why the construct belongs where it is")
	case e.owner == "":
		return allowEntry{}, fmt.Errorf("empty owner")
	case e.card == "":
		return allowEntry{}, fmt.Errorf("empty card; name the card or the ADR section")
	case !validExpires(e.expires):
		return allowEntry{}, fmt.Errorf("expires %q is neither YYYY-MM-DD nor permanent", e.expires)
	}
	return e, nil
}

// validConstruct accepts a construct kind, or kind#N with N >= 2 (the first occurrence carries no ordinal).
func validConstruct(s string) bool {
	base := s
	if i := strings.LastIndexByte(s, '#'); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n < 2 {
			return false
		}
		base = s[:i]
	}
	return constructKinds[base]
}

// validExpires checks the FORMAT only: `YYYY-MM-DD` or `permanent`. The clock lives in the CI audit step
// (tools/waiveraudit), never here: a date-based unit failure would red the tree overnight with no push behind
// it, and this checker must answer the same on this tree next month (PRACTICES-CONSENSUS Amendment 2.2).
func validExpires(s string) bool {
	if s == "permanent" {
		return true
	}
	if len(s) != len("2006-01-02") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 4 || i == 7 {
			if s[i] != '-' {
				return false
			}
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// checkPublicLayer applies the public-layer rule of the layout report §2: a public package holds
// declarations and pure projections only. The conservative first form fails on the constructs that can
// never be a projection — a loop, a range, a switch, defer/go, a closure, a map literal — and on a
// package-scope map. A file is skipped only when ast.IsGenerated reports true (the standard
// `^// Code generated .* DO NOT EDIT\.$` marker before the first non-comment text, parsed with
// parser.ParseComments); the `*_gen.go` suffix is never a skip, so a hand-written *_gen.go is checked like
// any other file and the suffix stays reserved for generator output (PRACTICES-CONSENSUS O5; eco-architect
// F3). The rule is a note until the claims phase turns final, when it fails (§15.2).
//
// A waiver names one construct, never a site or a file. Its key is `path<TAB>symbol<TAB>construct[#n]`: the
// enclosing declaration — receiver-qualified for a method (`(*T).M` for a pointer receiver, `(T).M` for a
// value one), the bare name for a package-level function and init — plus the construct kind, with an ordinal
// when one symbol carries several constructs of the same kind. The receiver is part of the symbol because a
// public package holds many methods of one name on different types (descriptor.go's eighteen toHAL), and a
// bare method name would let one type's waiver exempt another's construct (Amendment 3). The line is not the
// key, so an unrelated edit above a construct does not invalidate the review, and a different construct
// landing on the old line is not exempted (PRACTICES-CONSENSUS O1; eco-architect F2).
//
// The waiver list is a reviewed set, not a no-go area. An entry whose key matches no finding is a failure in
// every phase — the "consumed set" F1 asked for and F4's "every entry fires" in one mechanism — and the list
// count may not exceed the `limit:` header. The CI audit (tools/waiveraudit) forbids growth against the PR
// base and expires dated entries; content swaps are caught here by "every entry fires" and in review by the
// owner/reason diff. A missing waiver file is an empty list and fails closed.
//
// The rule is a construct ban, not a proof that a public package's code is thin. Its known bypass is logic
// written with none of the flagged constructs: a Lock/append/Unlock helper, or a method value where a
// closure used to be. A method value is not a closure, so rewriting closures as method values removes the
// findings without moving the code out of the package (wgputest/observer.go's eleven record callbacks took
// that shape). What the rule does read is unexported declarations as well as exported ones: a loop or a
// closure inside an unexported method reds the same as in an exported one (pinned by
// TestPublicLayerUnexportedMethodLogic). The flat-body bypass is the limit of this conservative form,
// stated here rather than discovered later.
func checkPublicLayer(rep *Report, root string, final bool) {
	allow, err := loadAllowlist(root)
	if err != nil {
		// The waiver list is part of the gate's configuration, so a malformed row fails in every phase.
		// The well-formed rows still apply: the list never exempts more than it says.
		rep.addFailf(publicLayerAllowCheck, "", allowlistRel, 0, "%v", err)
	}
	if allow.hasLimit && len(allow.entries) > allow.limit {
		rep.addFailf(publicLayerAllowCheck, "", allowlistRel, 0,
			"%d entries exceed the recorded limit %d; a lead-approved change is a dated amendment, never a silent edit",
			len(allow.entries), allow.limit)
	}
	consumed := map[string]bool{}
	for _, f := range collectPublicLayerFindings(rep, root, final) {
		if allow.lookup(f.key) {
			consumed[f.key] = true
			continue
		}
		reportLayerf(rep, final, publicLayerCheck, f.rel, f.line, "%s", f.message)
	}
	if rep.Summary != nil {
		// The counters make "every entry fires" visible in the gate summary: the two numbers are equal
		// exactly when every reviewed waiver matched a live construct.
		rep.Summary["public-layer-waivers"] = len(allow.entries)
		rep.Summary["public-layer-waivers-consumed"] = len(consumed)
	}
	reportUnusedWaivers(rep, allow, consumed)
}

// reportUnusedWaivers fails on every waiver whose key matched no finding. This is the consumed-set failure
// eco-architect F1 found missing: without it a renamed or moved construct leaves a waiver that exempts
// nothing, and the ADR would record a guarantee the checker does not make. It fails in every phase.
func reportUnusedWaivers(rep *Report, allow *allowlist, consumed map[string]bool) {
	if allow == nil {
		return
	}
	stale := make([]string, 0, len(allow.entries))
	for key := range allow.entries {
		if !consumed[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		e := allow.entries[key]
		rep.addFailf(publicLayerUnusedCheck, "", e.path, 0,
			"waiver %q matches no finding: stale or unused (owner %s, reason: %s)", key, e.owner, e.reason)
	}
}

// layerFinding is one construct the public-layer rule flags, with its symbol-keyed waiver key.
type layerFinding struct {
	rel     string
	line    int
	key     string
	message string
}

// collectPublicLayerFindings walks the three public packages and returns every flagged construct, unwaived
// or not. The caller does the waiver matching so the consumed set is known in one place.
func collectPublicLayerFindings(rep *Report, root string, final bool) []layerFinding {
	var out []layerFinding
	for _, dir := range publicPackages {
		for _, path := range goFiles(filepath.Join(root, dir), false) {
			out = append(out, collectPublicLayerFile(rep, root, final, path)...)
		}
	}
	return out
}

// checkPublicLayerFile reports the unwaived findings of one file: the entry point the unit tests use.
func checkPublicLayerFile(rep *Report, root string, allow *allowlist, final bool, path string) {
	for _, f := range collectPublicLayerFile(rep, root, final, path) {
		if allow.lookup(f.key) {
			continue
		}
		reportLayerf(rep, final, publicLayerCheck, f.rel, f.line, "%s", f.message)
	}
}

// collectPublicLayerFile applies the rule to one file of a public package. A generated file — one carrying
// the standard `^// Code generated .* DO NOT EDIT\.$` marker before the first non-comment text — is skipped
// via ast.IsGenerated. The suffix is not consulted: a hand-written *_gen.go is checked like any other file
// (eco-architect F3).
func collectPublicLayerFile(rep *Report, root string, final bool, path string) []layerFinding {
	rel := relPath(root, path)
	f, fset, err := parseFile(path)
	if err != nil {
		reportLayerf(rep, final, "ast-parse", rel, 0, "%v", err)
		return nil
	}
	if ast.IsGenerated(f) {
		return nil
	}
	counters := map[string]int{}
	var out []layerFinding
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			out = append(out, collectLayerGenDecl(rel, d, fset, counters)...)
		case *ast.FuncDecl:
			out = append(out, collectLayerFunc(rel, d, fset, counters)...)
		}
	}
	return out
}

// reportLayerf records one public-layer finding: a note while the phase is open, a failure at final.
func reportLayerf(rep *Report, final bool, check, file string, line int, format string, args ...any) {
	if final {
		rep.addFailf(check, "", file, line, format, args...)
		return
	}
	rep.addNotef(check, file, line, format, args...)
}

// collectLayerGenDecl collects a package-scope map, which a public package never holds. The variable name is
// the symbol-less construct's defined identifier.
func collectLayerGenDecl(rel string, d *ast.GenDecl, fset *token.FileSet, counters map[string]int) []layerFinding {
	if d.Tok != token.VAR {
		return nil
	}
	var out []layerFinding
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if !isPackageMap(vs) {
			continue
		}
		out = append(out, layerFinding{
			rel:     rel,
			line:    fset.Position(vs.Pos()).Line,
			key:     countingKey(counters, rel, packageMapSymbol(vs), "packagemap"),
			message: "package-scope map in a public package; maps and registries belong to internal/**",
		})
	}
	return out
}

// isPackageMap reports whether a var spec declares a map, in any of the three forms the ban covers: the
// explicit type (`var x map[K]V`), an inferred composite literal (`var x = map[K]V{...}`) and an inferred
// make call (`var x = make(map[K]V)` or `make(map[K]V, n)`). Reading only the explicit type let the inferred
// forms — the most idiomatic way to write the declaration — pass silently (F-A2 of the S13 trial review).
// The inferred forms are read through parentheses and map conversions too; isMapInitializer states the
// closed set and the two forms that deliberately stay out of scope. The values are read only when the type
// is omitted, because a typed spec names the map itself and carries no initializer.
func isPackageMap(vs *ast.ValueSpec) bool {
	if _, isMap := vs.Type.(*ast.MapType); isMap {
		return true
	}
	if vs.Type != nil {
		return false
	}
	for _, value := range vs.Values {
		if isMapInitializer(value) {
			return true
		}
	}
	return false
}

// isMapInitializer reports whether an inferred initializer produces a map: a composite literal whose type is
// `map[K]V`, a make call for a map type, a conversion to a map type (`map[K]V(nil)`), or any of those
// parenthesized (gofmt keeps the parentheses, so the parenthesized literal is a stable bypass if it passes).
// The closed set is deliberate: a named map type (`type R map[K]V; var r R`) cannot be recognized without
// go/types and stays out of scope for this AST rule, and `new(map[K]V)` declares a pointer variable, not a map.
func isMapInitializer(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return isMapInitializer(v.X)
	case *ast.CompositeLit:
		_, isMap := v.Type.(*ast.MapType)
		return isMap
	case *ast.CallExpr:
		if _, isMapConversion := v.Fun.(*ast.MapType); isMapConversion {
			return true
		}
		return isMakeMapCall(v)
	}
	return false
}

// isMakeMapCall reports whether a call is `make(map[K]V)` or `make(map[K]V, n)`. The builtin is matched by the
// plain identifier "make": a package that shadows make with its own function is a conservative false
// positive, accepted here because the rule must never exempt the idiomatic form of the banned declaration.
func isMakeMapCall(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "make" {
		return false
	}
	_, isMap := call.Args[0].(*ast.MapType)
	return isMap
}

// packageMapSymbol names the declaration a package-scope map belongs to: its first variable name.
func packageMapSymbol(vs *ast.ValueSpec) string {
	if len(vs.Names) > 0 {
		return vs.Names[0].Name
	}
	return ""
}

// collectLayerFunc collects every construct of a public method that is logic rather than a projection. The
// waiver symbol is the enclosing declaration (declSymbol), receiver-qualified for a method: descriptor.go
// declares eighteen toHAL methods, so a bare method name would share one key space across all of them and one
// type's reviewed waiver would exempt a construct inserted in another (Amendment 3, the consolidated deep
// review's P1).
func collectLayerFunc(rel string, d *ast.FuncDecl, fset *token.FileSet, counters map[string]int) []layerFinding {
	if d.Body == nil {
		return nil
	}
	symbol := declSymbol(d)
	var out []layerFinding
	ast.Inspect(d.Body, func(n ast.Node) bool {
		kind, msg := layerViolation(n, symbol)
		if kind == "" {
			return true
		}
		out = append(out, layerFinding{
			rel:     rel,
			line:    fset.Position(n.Pos()).Line,
			key:     countingKey(counters, rel, symbol, kind),
			message: msg,
		})
		return true
	})
	return out
}

// declSymbol names a declaration for the waiver key: receiver-qualified for a method — `(*T).M` for a
// pointer receiver, `(T).M` for a value one — and the bare name for a package-level function and for init.
// The receiver is part of the identity because a public package declares many methods of the same name on
// different types (descriptor.go's eighteen toHAL): a bare method name would let one type's waiver exempt a
// construct inserted in another (Amendment 3; PRACTICES-CONSENSUS O1's symbol, corrected, not reinterpreted).
// init may legally repeat in one file; countingKey's per-file ordinal, assigned per construct kind in AST
// order, keeps two init bodies apart.
func declSymbol(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	return receiverPrefix(d.Recv.List[0].Type) + "." + d.Name.Name
}

// receiverPrefix renders a method receiver's type as the key prefix: `(*T)` for a pointer receiver and `(T)`
// for a value receiver. Parentheses around the whole receiver are unwrapped first, so `(*T)` and `*T` agree;
// the star is read before the type name is unwrapped, so the pointer and value forms stay distinct keys.
func receiverPrefix(expr ast.Expr) string {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = p.X
	}
	if _, isPtr := expr.(*ast.StarExpr); isPtr {
		return "(*" + receiverTypeName(expr) + ")"
	}
	return "(" + receiverTypeName(expr) + ")"
}

// receiverTypeName returns a receiver's bare type name, unwrapping a leading star and parentheses: `T`,
// `*T`, `(T)` and `(*T)` all yield `T`, and an instantiated generic receiver yields its base type name. Go
// admits only these receiver forms (a defined type of the same package), so the switch is exhaustive.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.ParenExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // T[P]
		return receiverTypeName(t.X)
	case *ast.IndexListExpr: // T[P1, P2]
		return receiverTypeName(t.X)
	}
	return ""
}

// countingKey assigns the waiver key: path + symbol + construct, with `#2`, `#3`… when the same symbol
// carries several flagged constructs of the same kind. The counter lives per file (collectPublicLayerFile),
// so ordinals are assigned per file and per construct kind in AST order; the symbol is the enclosing
// declaration from declSymbol.
func countingKey(counters map[string]int, path, symbol, construct string) string {
	id := allowKey(path, symbol, construct)
	counters[id]++
	if n := counters[id]; n > 1 {
		return id + "#" + strconv.Itoa(n)
	}
	return id
}

// publicLayerCheck is the check name of the public-layer rule.
const publicLayerCheck = "public-layer"

// publicLayerAllowCheck is the check name of the waiver-format and limit rules.
const publicLayerAllowCheck = "public-layer-allow"

// publicLayerUnusedCheck is the check name of the stale/unused-waiver rule (eco-architect F1/F4).
const publicLayerUnusedCheck = "public-layer-unused"

// layerViolation describes the construct if it may not appear in a public method, and returns the empty kind
// otherwise. The kind is the waiver construct identifier; the second return is the finding text, which names
// the enclosing declaration (symbol, receiver-qualified for a method) so the reader sees which type's method
// was flagged.
func layerViolation(n ast.Node, symbol string) (kind, msg string) {
	switch node := n.(type) {
	case *ast.ForStmt:
		return "for", fmt.Sprintf("loop in %s; a public method is one delegation, no logic", symbol)
	case *ast.RangeStmt:
		return "range", fmt.Sprintf("range loop in %s; a public method is one delegation, no logic", symbol)
	case *ast.SwitchStmt:
		return "switch", fmt.Sprintf("switch in %s; a public method is one delegation, no logic", symbol)
	case *ast.TypeSwitchStmt:
		return "typeswitch", fmt.Sprintf("type switch in %s; a public method is one delegation, no logic", symbol)
	case *ast.SelectStmt:
		return "select", fmt.Sprintf("select in %s; a public method is one delegation, no logic", symbol)
	case *ast.DeferStmt:
		return "defer", fmt.Sprintf("defer in %s; cleanup registration belongs to internal/**", symbol)
	case *ast.GoStmt:
		return "go", fmt.Sprintf("go statement in %s; concurrency belongs to internal/**", symbol)
	case *ast.FuncLit:
		return "closure", fmt.Sprintf("closure in %s; a public method is one delegation, no logic", symbol)
	case *ast.CompositeLit:
		if _, isMap := node.Type.(*ast.MapType); isMap {
			return "mapliteral", fmt.Sprintf("map literal in %s; maps and registries belong to internal/**", symbol)
		}
	}
	return "", ""
}

// checkPublicGoldens re-reads the signature-level API goldens: a public signature that names an internal
// package is a failure in every context (§15.1; the A5 acid test).
func checkPublicGoldens(rep *Report, root string) {
	dir := filepath.Join(root, "tools", "apigolden", "testdata")
	for _, name := range []string{"api_windows.txt", "api_rust.txt", "api_wasm.txt"} {
		rel := "tools/apigolden/testdata/" + name
		data, err := readTreeFile(filepath.Join(dir, name))
		if err != nil {
			rep.addFailf("public-golden", "", rel, 0, "read: %v", err)
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "internal/") {
				rep.addFailf("public-golden", "", rel, i+1, "a public signature names an internal package: %s", strings.TrimSpace(line))
			}
		}
	}
}
