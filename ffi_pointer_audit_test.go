package wgpu_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// ffiPointerAllowed lists address-to-uintptr conversions that are safe without a
// runtime.Pinner, keyed by "file:function:expression", with the reason.
var ffiPointerAllowed = map[string]string{
	// Package-level arrays never move, and Wayland keeps the listener pointer
	// for the lifetime of the proxy, so it has to be static memory anyway.
	"hal/software/blit_wayland.go:obtainWlShm:&registryListenerFuncs[0]":          "package-level array",
	"hal/software/blit_wayland.go:waylandCreateShmBuffer:&bufferListenerFuncs[0]": "package-level array",
	// debugNameBuf is a field of the heap-allocated Device: heap objects do not
	// move, and the KeepAlive after the call keeps it reachable.
	"hal/vulkan/debug.go:setObjectName:&d.debugNameBuf[0]": "heap field kept alive",
}

// TestFFIPointersArePinned guards P1-VK-012. A uintptr holding the address of a
// Go object is invisible to the GC and to the stack copier. Only a conversion
// written directly in the argument list of a syscall.Syscall*, syscall.SyscallN
// or a standard library (*syscall.Proc).Call / (*syscall.LazyProc).Call keeps
// the object alive and in place (unsafe.Pointer rule 4, //go:uintptrescapes).
// golang.org/x/sys/windows Proc.Call has no //go:uintptrescapes, and goffi
// wrappers, objc MsgSend and struct fields read by C are ordinary Go code. Any
// other conversion of an address must pin the object with runtime.Pinner in
// the same function, or be listed in ffiPointerAllowed with a reason.
//
// The check is syntactic: it recognizes the address-of form (&x, &x[i], &x.f).
// A pointer variable converted to uintptr is not checked.
func TestFFIPointersArePinned(t *testing.T) {
	var violations []string
	for _, root := range []string{"hal", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			found, err := unpinnedFFIAddresses(path)
			if err != nil {
				return err
			}
			violations = append(violations, found...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range violations {
		t.Errorf("Go address passed as uintptr without runtime.Pinner (see P1-VK-012): %s", v)
	}
}

func unpinnedFFIAddresses(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	xsysWindows := false
	for _, imp := range file.Imports {
		if imp.Path.Value == `"golang.org/x/sys/windows"` {
			xsysWindows = true
		}
	}
	rel := filepath.ToSlash(path)
	var found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		pinned := pinnedExpressions(fset, fn.Body)
		var stack []ast.Node
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			addr, ok := addressToUintptr(n)
			if !ok {
				return true
			}
			if call, ok := stack[len(stack)-2].(*ast.CallExpr); ok && keepsUintptrReferent(call, xsysWindows) {
				return true
			}
			text := exprString(fset, addr)
			if pinned[text] {
				return true
			}
			key := rel + ":" + fn.Name.Name + ":" + text
			if _, ok := ffiPointerAllowed[key]; ok {
				return true
			}
			found = append(found, fset.Position(n.Pos()).String()+" "+key)
			return true
		})
	}
	return found, nil
}

// addressToUintptr matches uintptr(unsafe.Pointer(&x...)) and returns &x....
func addressToUintptr(n ast.Node) (ast.Expr, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "uintptr" {
		return nil, false
	}
	inner, ok := call.Args[0].(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 {
		return nil, false
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Pointer" {
		return nil, false
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "unsafe" {
		return nil, false
	}
	addr, ok := inner.Args[0].(*ast.UnaryExpr)
	if !ok || addr.Op != token.AND {
		return nil, false
	}
	return addr, true
}

// keepsUintptrReferent reports whether call is one whose uintptr arguments the
// compiler keeps alive and unmoved: syscall.Syscall*, and Proc.Call from the
// standard library (not from golang.org/x/sys/windows).
func keepsUintptrReferent(call *ast.CallExpr, xsysWindows bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "syscall" && strings.HasPrefix(sel.Sel.Name, "Syscall") {
		return true
	}
	return sel.Sel.Name == "Call" && !xsysWindows
}

// pinnedExpressions collects the arguments of every X.Pin(arg) call in body.
func pinnedExpressions(fset *token.FileSet, body *ast.BlockStmt) map[string]bool {
	pinned := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Pin" {
			pinned[exprString(fset, call.Args[0])] = true
		}
		return true
	})
	return pinned
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, fset, e)
	return b.String()
}
