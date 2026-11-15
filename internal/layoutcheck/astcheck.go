//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// publicPackages are the directories of the three public packages.
var publicPackages = []string{"", pkgHal, pkgWgpuTest}

// childDirectories are the extraction packages. A check that needs a child skips a directory that does not
// exist yet: before its step creates the package the rule is vacuous, not failing.
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

// checkChildren enforces the child rules: no package-scope `var _`, no child→device import, and no
// white-box child test importing device (§15.6, §15.10).
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

// checkChildDeviceImport enforces §15.3: a child never imports internal/device; the seam is declared in
// the child and typed by core.
func checkChildDeviceImport(rep *Report, dir, rel string, f *ast.File, fset *token.FileSet) {
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

// loadAllowlist reads the explicit public-layer allowlist (`path<TAB>reason` lines). A missing file is an
// empty allowlist: the list is optional, and its absence fails closed (more findings, never fewer).
func loadAllowlist(root string) map[string]string {
	out := map[string]string{}
	data, err := readTreeFile(filepath.Join(root, dirInternal, "layoutcheck", "publiclayer_allow.txt"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		reason := ""
		if len(fields) > 1 {
			reason = fields[1]
		}
		out[strings.TrimSpace(fields[0])] = reason
	}
	return out
}

// checkPublicLayer applies the public-layer rule of the layout report §2: a public package holds
// declarations and pure projections only. The conservative first form fails on the constructs that can
// never be a projection — a loop, a switch, defer/go, a closure, a map literal — and on a package-scope
// map. Generated files (*_gen.go) are projections by construction and are skipped, as is every file on the
// explicit allowlist. The rule is a note until the claims phase turns final, when it fails (§15.2).
func checkPublicLayer(rep *Report, root string, final bool) {
	allow := loadAllowlist(root)
	for _, dir := range publicPackages {
		for _, path := range goFiles(filepath.Join(root, dir), false) {
			checkPublicLayerFile(rep, root, allow, final, path)
		}
	}
}

// checkPublicLayerFile applies the rule to one file of a public package.
func checkPublicLayerFile(rep *Report, root string, allow map[string]string, final bool, path string) {
	rel := relPath(root, path)
	if strings.HasSuffix(path, "_gen.go") || allow[rel] != "" {
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
			checkLayerGenDecl(rep, final, rel, d, fset)
		case *ast.FuncDecl:
			checkLayerFunc(rep, final, rel, d, fset)
		}
	}
}

// reportLayerf records one public-layer finding: a note while the phase is open, a failure at final.
func reportLayerf(rep *Report, final bool, check, file string, line int, format string, args ...any) {
	if final {
		rep.addFailf(check, "", file, line, format, args...)
		return
	}
	rep.addNotef(check, "", file, line, format, args...)
}

// checkLayerGenDecl reports a package-scope map, which a public package never holds.
func checkLayerGenDecl(rep *Report, final bool, rel string, d *ast.GenDecl, fset *token.FileSet) {
	if d.Tok != token.VAR {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if _, isMap := vs.Type.(*ast.MapType); isMap {
			reportLayerf(rep, final, publicLayerCheck, rel, fset.Position(vs.Pos()).Line,
				"package-scope map in a public package; maps and registries belong to internal/**")
		}
	}
}

// checkLayerFunc reports every construct of a public method that is logic rather than a projection.
func checkLayerFunc(rep *Report, final bool, rel string, d *ast.FuncDecl, fset *token.FileSet) {
	if d.Body == nil {
		return
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if msg := layerViolation(n, d.Name.Name); msg != "" {
			reportLayerf(rep, final, publicLayerCheck, rel, fset.Position(n.Pos()).Line, "%s", msg)
		}
		return true
	})
}

// publicLayerCheck is the check name of the public-layer rule.
const publicLayerCheck = "public-layer"

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
