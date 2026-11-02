package wgpu_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
// written directly in the argument list of syscall.Syscall*, syscall.SyscallN or
// the standard library's (*syscall.Proc).Call / (*syscall.LazyProc).Call keeps
// the object alive and in place (unsafe.Pointer rule 4, //go:uintptrescapes).
// That holds only for a direct call on the concrete type: the same Call reached
// through an interface or a func value loses the directive. goffi wrappers,
// objc MsgSend and struct fields read by C are ordinary Go code.
// golang.org/x/sys/windows (*Proc).Call and (*LazyProc).Call carry the
// directive too (x/sys v0.47.0 windows/dll_windows.go:156, :305); this check
// accepts only the standard library procs, which is stricter than needed, so a
// direct x/sys Call is reported and wgpu code uses syscall procs instead. Any other conversion of an
// address must pin the object with runtime.Pinner in the same function, or be
// listed in ffiPointerAllowed with a reason.
//
// The check is syntactic. It recognizes the address-of form (&x, &x[i], &x.f);
// a pointer variable converted to uintptr is not checked. A Call receiver counts
// as a stdlib proc only when the same package declares it with type
// *syscall.Proc or *syscall.LazyProc, or initializes it with NewProc, FindProc or
// MustFindProc on a DLL from syscall.NewLazyDLL, LoadDLL or MustLoadDLL.
func TestFFIPointersArePinned(t *testing.T) {
	var violations []string
	for _, root := range []string{"hal", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			found, err := unpinnedFFIAddressesInDir(path)
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

// unpinnedFFIAddressesInDir checks the non-test Go files of one package
// directory, whatever their build tags.
func unpinnedFFIAddressesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		paths = append(paths, filepath.ToSlash(path))
	}
	procs := syscallProcNames(fset, files)
	var found []string
	for i, f := range files {
		found = append(found, unpinnedFFIAddresses(fset, f, paths[i], procs)...)
	}
	return found, nil
}

// syscallProcNames returns the names of variables and struct fields in files
// that hold a standard library *syscall.Proc or *syscall.LazyProc.
func syscallProcNames(fset *token.FileSet, files []*ast.File) map[string]bool {
	isType := func(e ast.Expr, names ...string) bool {
		s := exprString(fset, e)
		for _, n := range names {
			if s == n {
				return true
			}
		}
		return false
	}
	dlls := map[string]bool{}
	procs := map[string]bool{}
	// Typed declarations: var x *syscall.Proc, struct fields, var dll *syscall.DLL.
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			var names []*ast.Ident
			var typ ast.Expr
			switch v := n.(type) {
			case *ast.ValueSpec:
				names, typ = v.Names, v.Type
			case *ast.Field:
				names, typ = v.Names, v.Type
			default:
				return true
			}
			if typ == nil {
				return true
			}
			for _, id := range names {
				switch {
				case isType(typ, "*syscall.Proc", "*syscall.LazyProc"):
					procs[id.Name] = true
				case isType(typ, "*syscall.DLL", "*syscall.LazyDLL"):
					dlls[id.Name] = true
				}
			}
			return true
		})
	}
	// Initialized declarations and assignments: dll = syscall.NewLazyDLL(...),
	// proc = dll.NewProc(...), proc := dll.NewProc(...).
	record := func(name string, value ast.Expr) {
		call, ok := value.(*ast.CallExpr)
		if !ok {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return
		}
		switch {
		case x.Name == "syscall" && (sel.Sel.Name == "NewLazyDLL" || sel.Sel.Name == "LoadDLL" || sel.Sel.Name == "MustLoadDLL"):
			dlls[name] = true
		case dlls[x.Name] && (sel.Sel.Name == "NewProc" || sel.Sel.Name == "FindProc" || sel.Sel.Name == "MustFindProc"):
			procs[name] = true
		}
	}
	for pass := 0; pass < 2; pass++ {
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.ValueSpec:
					for i := range v.Values {
						if i < len(v.Names) {
							record(v.Names[i].Name, v.Values[i])
						}
					}
				case *ast.AssignStmt:
					// Multi-value forms (proc, err = dll.FindProc(...)) bind the
					// first result to the proc.
					if len(v.Rhs) == 1 && len(v.Lhs) >= 1 {
						if id, ok := v.Lhs[0].(*ast.Ident); ok {
							record(id.Name, v.Rhs[0])
						}
					}
				}
				return true
			})
		}
	}
	return procs
}

func unpinnedFFIAddresses(fset *token.FileSet, file *ast.File, rel string, procs map[string]bool) []string {
	var found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		pinned := pinnedExpressions(fset, fn.Body)
		raceAlias := raceImportName(file)
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
			if call, ok := stack[len(stack)-2].(*ast.CallExpr); ok &&
				(keepsUintptrReferent(call, procs) || isRaceInstrumentation(call, raceAlias)) {
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
	return found
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
// compiler keeps alive and unmoved: syscall.Syscall*, or Call on a receiver that
// procs records as a standard library *syscall.Proc / *syscall.LazyProc.
func keepsUintptrReferent(call *ast.CallExpr, procs map[string]bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "syscall" && strings.HasPrefix(sel.Sel.Name, "Syscall") {
		return true
	}
	if sel.Sel.Name != "Call" {
		return false
	}
	switch recv := sel.X.(type) {
	case *ast.Ident:
		return procs[recv.Name]
	case *ast.SelectorExpr:
		return procs[recv.Sel.Name]
	}
	return false
}

// racePackagePath is the package CI's racedetector imports into every file it
// instruments (racedetector cmd/racedetector/instrument, RacePackageImportPath).
const racePackagePath = "github.com/kolkov/racedetector/race"

// raceImportName returns the local name file uses for racePackagePath, or ""
// when file does not import it.
func raceImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != racePackagePath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "race"
	}
	return ""
}

// isRaceInstrumentation reports whether call is one the race detector inserted
// to record an access. It reads the address as a number and never hands it to
// foreign code, so the referent needs no pin.
func isRaceInstrumentation(call *ast.CallExpr, raceAlias string) bool {
	if raceAlias == "" {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == raceAlias
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

// TestFFIPointerGuardRejectsIndirectCall checks the guard itself: a Call through
// an interface, or on a golang.org/x/sys/windows proc, is not accepted as a
// stdlib syscall even though the method name matches.
func TestFFIPointerGuardRejectsIndirectCall(t *testing.T) {
	const src = `package p

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type caller interface{ Call(a ...uintptr) (uintptr, uintptr, error) }

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	procOK   = user32.NewProc("GetDC")
	xsys     = windows.NewLazySystemDLL("user32.dll")
	procXSys = xsys.NewProc("GetDC")
)

func direct(x int) { procOK.Call(uintptr(unsafe.Pointer(&x))) }

func viaInterface(c caller, x int) { c.Call(uintptr(unsafe.Pointer(&x))) }

func viaXSys(x int) { procXSys.Call(uintptr(unsafe.Pointer(&x))) }
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := unpinnedFFIAddressesInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	funcs := make([]string, 0, len(found))
	for _, f := range found {
		parts := strings.Split(f, ":")
		funcs = append(funcs, parts[len(parts)-2])
	}
	sort.Strings(funcs)
	if got, want := strings.Join(funcs, ","), "viaInterface,viaXSys"; got != want {
		t.Fatalf("flagged functions = %q, want %q (direct stdlib Call must pass)", got, want)
	}
}

// TestFFIPointerGuardIgnoresRaceInstrumentation checks the guard on source as
// CI's racedetector rewrites it: every memory access gains a
// race.RaceRead/RaceWrite(uintptr(unsafe.Pointer(&x))) call. Those calls only
// record the address, so they are not FFI; an unpinned FFI address next to
// them is still reported.
func TestFFIPointerGuardIgnoresRaceInstrumentation(t *testing.T) {
	const src = `package p

import (
	"unsafe"

	race "github.com/kolkov/racedetector/race"
)

func ffi(a uintptr) {}

func instrumented(x int) {
	race.RaceRead(uintptr(unsafe.Pointer(&x)))
	race.RaceWrite(uintptr(unsafe.Pointer(&x)))
}

func unpinned(x int) {
	race.RaceRead(uintptr(unsafe.Pointer(&x)))
	ffi(uintptr(unsafe.Pointer(&x)))
}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := unpinnedFFIAddressesInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || !strings.HasSuffix(found[0], "p.go:unpinned:&x") {
		t.Fatalf("found = %q, want only the ffi call in unpinned", found)
	}
}
