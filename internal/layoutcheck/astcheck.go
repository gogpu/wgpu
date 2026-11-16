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
	"strconv"
	"strings"
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

// parseFile parses one file.
func parseFile(path string) (*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
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

// allowlistFile is the explicit public-layer allowlist, relative to internal/layoutcheck.
const allowlistFile = "publiclayer_allow.txt"

// allowlistRel is the allowlist's module-relative path, used in every error message so a reader chasing a
// CI annotation opens the file that exists.
const allowlistRel = dirInternal + "/layoutcheck/" + allowlistFile

// loadAllowlist reads the explicit public-layer allowlist: `path:line<TAB>reason` lines. The rule is
// exact-site, so an entry exempts the one finding at that file and line and nothing else; the retired
// file-wide form (`path<TAB>reason`) is a format error, never an exemption. A missing file is an empty
// allowlist: the list is optional, and its absence fails closed (more findings, never fewer). A malformed
// line is returned as an error while the well-formed entries stay in the map, so a typo neither exempts
// anything by accident nor costs the entries that are right.
func loadAllowlist(root string) (map[string]string, error) {
	out := map[string]string{}
	data, err := readTreeFile(filepath.Join(root, dirInternal, "layoutcheck", allowlistFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, fmt.Errorf("%s: %w", allowlistRel, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, reason, err := parseAllowEntry(line)
		if err != nil {
			return out, fmt.Errorf("%s:%d: %w", allowlistRel, i+1, err)
		}
		out[key] = reason
	}
	return out, nil
}

// parseAllowEntry splits one allowlist line into its exact-site key and its reason.
func parseAllowEntry(line string) (key, reason string, err error) {
	fields := strings.SplitN(line, "\t", 2)
	key = strings.TrimSpace(fields[0])
	if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
		return "", "", fmt.Errorf("%q: want `path:line<TAB>reason`", key)
	}
	colon := strings.LastIndexByte(key, ':')
	if colon <= 0 {
		return "", "", fmt.Errorf("%q: want `path:line<TAB>reason`; a file-wide entry exempts nothing", key)
	}
	n, convErr := strconv.Atoi(key[colon+1:])
	if convErr != nil || n < 1 {
		return "", "", fmt.Errorf("%q: %q is not a 1-based line number", key, key[colon+1:])
	}
	return key, strings.TrimSpace(fields[1]), nil
}

// checkPublicLayer applies the public-layer rule of the layout report §2: a public package holds
// declarations and pure projections only. The conservative first form fails on the constructs that can
// never be a projection — a loop, a switch, defer/go, a closure, a map literal — and on a package-scope
// map. Generated files (*_gen.go) are projections by construction and are skipped. The rule is a note
// until the claims phase turns final, when it fails (§15.2).
//
// An allowlist entry is one exact site, `path:line<TAB>reason`: it exempts the finding reported at that
// file and that line, and no other. A second finding in the same file still fails at phase final, and a
// new site — even on another line of a listed file — reds the gate, which is the point of the form: the
// exemption names a line somebody reviewed, never a file that becomes a no-go area.
//
// The six current entries all name wgputest/wgputest.go, and they exist for one reason: Option is
// func(*config). The facade's With* must build closures over config, and Trace/Counts must guard their
// mutex (the clone deep-copies the variable-length records). Those sites predate this window; the
// API-level cleanup that removes them from the facade is carded before v1.0.
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
		// The allowlist is part of the gate's configuration, so a malformed entry fails in every phase.
		// The well-formed entries still apply: the list never exempts more than it says.
		rep.addFailf(publicLayerAllowCheck, "", "", 0, "%v", err)
	}
	for _, dir := range publicPackages {
		for _, path := range goFiles(filepath.Join(root, dir), false) {
			checkPublicLayerFile(rep, root, allow, final, path)
		}
	}
}

// checkPublicLayerFile applies the rule to one file of a public package.
func checkPublicLayerFile(rep *Report, root string, allow map[string]string, final bool, path string) {
	rel := relPath(root, path)
	if strings.HasSuffix(path, "_gen.go") {
		return
	}
	f, fset, err := parseFile(path)
	if err != nil {
		reportLayerf(rep, final, "ast-parse", rel, 0, "%v", err)
		return
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			checkLayerGenDecl(rep, allow, final, rel, d, fset)
		case *ast.FuncDecl:
			checkLayerFunc(rep, allow, final, rel, d, fset)
		}
	}
}

// reportLayerSitef records one public-layer finding unless the allowlist names this exact file and line.
func reportLayerSitef(rep *Report, allow map[string]string, final bool, rel string, line int, format string, args ...any) {
	if allow[fmt.Sprintf("%s:%d", rel, line)] != "" {
		return
	}
	reportLayerf(rep, final, publicLayerCheck, rel, line, format, args...)
}

// reportLayerf records one public-layer finding: a note while the phase is open, a failure at final.
func reportLayerf(rep *Report, final bool, check, file string, line int, format string, args ...any) {
	if final {
		rep.addFailf(check, "", file, line, format, args...)
		return
	}
	rep.addNotef(check, file, line, format, args...)
}

// checkLayerGenDecl reports a package-scope map, which a public package never holds.
func checkLayerGenDecl(rep *Report, allow map[string]string, final bool, rel string, d *ast.GenDecl, fset *token.FileSet) {
	if d.Tok != token.VAR {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if _, isMap := vs.Type.(*ast.MapType); isMap {
			reportLayerSitef(rep, allow, final, rel, fset.Position(vs.Pos()).Line,
				"package-scope map in a public package; maps and registries belong to internal/**")
		}
	}
}

// checkLayerFunc reports every construct of a public method that is logic rather than a projection.
func checkLayerFunc(rep *Report, allow map[string]string, final bool, rel string, d *ast.FuncDecl, fset *token.FileSet) {
	if d.Body == nil {
		return
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if msg := layerViolation(n, d.Name.Name); msg != "" {
			reportLayerSitef(rep, allow, final, rel, fset.Position(n.Pos()).Line, "%s", msg)
		}
		return true
	})
}

// publicLayerCheck is the check name of the public-layer rule.
const publicLayerCheck = "public-layer"

// publicLayerAllowCheck is the check name of the allowlist format rule.
const publicLayerAllowCheck = "public-layer-allow"

// layerViolation describes the construct if it may not appear in a public method, and "" otherwise.
func layerViolation(n ast.Node, funcName string) string {
	switch node := n.(type) {
	case *ast.ForStmt:
		return fmt.Sprintf("loop in %s; a public method is one delegation, no logic", funcName)
	case *ast.RangeStmt:
		return fmt.Sprintf("range loop in %s; a public method is one delegation, no logic", funcName)
	case *ast.SwitchStmt:
		return fmt.Sprintf("switch in %s; a public method is one delegation, no logic", funcName)
	case *ast.TypeSwitchStmt:
		return fmt.Sprintf("type switch in %s; a public method is one delegation, no logic", funcName)
	case *ast.SelectStmt:
		return fmt.Sprintf("select in %s; a public method is one delegation, no logic", funcName)
	case *ast.DeferStmt:
		return fmt.Sprintf("defer in %s; cleanup registration belongs to internal/**", funcName)
	case *ast.GoStmt:
		return fmt.Sprintf("go statement in %s; concurrency belongs to internal/**", funcName)
	case *ast.FuncLit:
		return fmt.Sprintf("closure in %s; a public method is one delegation, no logic", funcName)
	case *ast.CompositeLit:
		if _, isMap := node.Type.(*ast.MapType); isMap {
			return fmt.Sprintf("map literal in %s; maps and registries belong to internal/**", funcName)
		}
	}
	return ""
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
