//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// TestContextParity fails when the WGL (context.go) and EGL (context_linux.go)
// implementations of *Context expose different exported methods or different
// signatures. The shared GLES resource code compiles against both, so a binding
// added to one platform only, or with a different parameter type, would break
// the other platform's build or silently diverge. Parsing the sources lets the
// check run on every OS, not only on the platform whose file is compiled.
func TestContextParity(t *testing.T) {
	wgl := contextMethods(t, "context.go")
	egl := contextMethods(t, "context_linux.go")

	for _, name := range sortedKeys(wgl) {
		eglSig, ok := egl[name]
		switch {
		case !ok:
			t.Errorf("(*Context).%s exists in context.go (WGL) but not in context_linux.go (EGL)", name)
		case eglSig != wgl[name]:
			t.Errorf("(*Context).%s signature differs:\n  WGL: %s\n  EGL: %s", name, wgl[name], eglSig)
		}
	}
	for _, name := range sortedKeys(egl) {
		if _, ok := wgl[name]; !ok {
			t.Errorf("(*Context).%s exists in context_linux.go (EGL) but not in context.go (WGL)", name)
		}
	}
}

func TestIsESVersionString(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"OpenGL ES 3.2 V@490.0", true},
		{"OpenGL ES 3.0 Mesa 24.0.5", true},
		{"4.6.0 NVIDIA 560.94", false},
		{"4.5 (Core Profile) Mesa 24.0.5", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isESVersionString(tt.version); got != tt.want {
			t.Errorf("isESVersionString(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

// contextMethods returns exported (*Context) methods of file mapped to their
// signature with parameter names removed, e.g. "(uint32, int, unsafe.Pointer, uint32)".
func contextMethods(t *testing.T, file string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	methods := make(map[string]string)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !fn.Name.IsExported() || !isContextReceiver(fn.Recv) {
			continue
		}
		methods[fn.Name.Name] = signature(t, fset, fn.Type)
	}
	if len(methods) == 0 {
		t.Fatalf("%s: no exported (*Context) methods found", file)
	}
	return methods
}

func isContextReceiver(recv *ast.FieldList) bool {
	if len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Context"
}

func signature(t *testing.T, fset *token.FileSet, ft *ast.FuncType) string {
	t.Helper()
	return "(" + fieldTypes(t, fset, ft.Params) + ") (" + fieldTypes(t, fset, ft.Results) + ")"
}

// fieldTypes prints one type per parameter, so "a, b int32" and
// "a int32, b int32" compare equal and parameter names are ignored.
func fieldTypes(t *testing.T, fset *token.FileSet, fl *ast.FieldList) string {
	t.Helper()
	if fl == nil {
		return ""
	}
	var parts []string
	for _, field := range fl.List {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, field.Type); err != nil {
			t.Fatalf("print type: %v", err)
		}
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			parts = append(parts, buf.String())
		}
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
