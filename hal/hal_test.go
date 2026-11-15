package hal

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// scalarKinds are the only field kinds this package may declare: handles are
// uintptr, QueueFamilyIndex is uint32, and BackendKind is uint8-based.
var scalarKinds = map[reflect.Kind]bool{
	reflect.Uintptr: true,
	reflect.Uint32:  true,
	reflect.Uint8:   true,
}

func fieldShape(t reflect.Type) [][2]string {
	shape := make([][2]string, t.NumField())
	for i := range shape {
		shape[i] = [2]string{t.Field(i).Name, t.Field(i).Type.String()}
	}
	return shape
}

func checkShape(t *testing.T, typ reflect.Type, want [][2]string) {
	t.Helper()
	if got := fieldShape(typ); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s fields = %v, want %v", typ, got, want)
	}
	if !typ.Comparable() {
		t.Fatalf("%s is not comparable", typ)
	}
}

// TestHandlesShape pins the exact field set and types of Handles and its five
// per-backend structs: the package is frozen at v1.0, so a new or retyped field
// is a change someone has to make on purpose.
func TestHandlesShape(t *testing.T) {
	checkShape(t, reflect.TypeOf(Handles{}), [][2]string{
		{"Kind", "hal.BackendKind"},
		{"Vulkan", "hal.VulkanHandles"},
		{"DX12", "hal.DX12Handles"},
		{"Metal", "hal.MetalHandles"},
		{"GLES", "hal.GLESHandles"},
		{"WebGPU", "hal.WebGPUHandles"},
	})
	if got := reflect.TypeOf(Handles{}).Field(0); got.Name != "Kind" {
		t.Fatalf("Handles field 0 = %q, want Kind", got.Name)
	}

	uintptrFields := func(names ...string) [][2]string {
		shape := make([][2]string, len(names))
		for i, name := range names {
			shape[i] = [2]string{name, "uintptr"}
		}
		return shape
	}

	checkShape(t, reflect.TypeOf(VulkanHandles{}), [][2]string{
		{"Instance", "uintptr"},
		{"PhysicalDevice", "uintptr"},
		{"Device", "uintptr"},
		{"Queue", "uintptr"},
		{"QueueFamilyIndex", "uint32"},
		{"GetInstanceProcAddr", "uintptr"},
		{"GetDeviceProcAddr", "uintptr"},
		{"Surface", "uintptr"},
	})
	checkShape(t, reflect.TypeOf(DX12Handles{}), uintptrFields("Factory", "Adapter", "Device", "Queue", "SwapChain"))
	checkShape(t, reflect.TypeOf(MetalHandles{}), uintptrFields("Device", "CommandQueue", "Layer"))
	checkShape(t, reflect.TypeOf(GLESHandles{}), uintptrFields("EGLDisplay", "EGLContext", "EGLConfig", "EGLSurface", "HGLRC", "HDC"))
	checkShape(t, reflect.TypeOf(WebGPUHandles{}), uintptrFields("Instance", "Adapter", "Device", "Queue"))

	// Every field of a per-backend struct is a scalar; no pointer, slice, map
	// or interface may ever appear there.
	for _, typ := range []reflect.Type{
		reflect.TypeOf(VulkanHandles{}),
		reflect.TypeOf(DX12Handles{}),
		reflect.TypeOf(MetalHandles{}),
		reflect.TypeOf(GLESHandles{}),
		reflect.TypeOf(WebGPUHandles{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			if f := typ.Field(i); !scalarKinds[f.Type.Kind()] {
				t.Errorf("%s.%s has kind %v, want a scalar", typ, f.Name, f.Type.Kind())
			}
		}
	}
}

// TestKindValues pins the nine Kind values. A renumbering silently relabels
// every release and compiled consumer; the values are a recorded decision.
func TestKindValues(t *testing.T) {
	tests := []struct {
		name string
		kind BackendKind
		want uint8
	}{
		{"KindUnknown", KindUnknown, 0},
		{"KindVulkan", KindVulkan, 1},
		{"KindMetal", KindMetal, 2},
		{"KindDX12", KindDX12, 3},
		{"KindGLES", KindGLES, 4},
		{"KindBrowser", KindBrowser, 5},
		{"KindSoftware", KindSoftware, 6},
		{"KindNoop", KindNoop, 7},
		{"KindWebGPU", KindWebGPU, 8},
	}
	seen := make(map[BackendKind]string, len(tests))
	for _, test := range tests {
		if uint8(test.kind) != test.want {
			t.Errorf("%s = %d, want %d", test.name, uint8(test.kind), test.want)
		}
		if prev, ok := seen[test.kind]; ok {
			t.Errorf("%s and %s share the value %d", prev, test.name, uint8(test.kind))
		}
		seen[test.kind] = test.name
	}
}

// TestZeroHandles checks that the zero value is valid, comparable and names
// nothing.
func TestZeroHandles(t *testing.T) {
	var got Handles
	if got.Kind != KindUnknown {
		t.Fatalf("zero Handles Kind = %d, want KindUnknown", got.Kind)
	}
	if got != (Handles{}) {
		t.Fatal("zero Handles is not equal to Handles{}")
	}
	if got.Vulkan != (VulkanHandles{}) || got.DX12 != (DX12Handles{}) ||
		got.Metal != (MetalHandles{}) || got.GLES != (GLESHandles{}) ||
		got.WebGPU != (WebGPUHandles{}) {
		t.Fatal("zero Handles carries a non-zero backend struct")
	}
}

// TestPackageImportsNothing enforces the declaration-only contract: this
// package may not import anything at all, so it can never name an internal
// type and can never gain a dependency that would not advance with v1.0.
func TestPackageImportsNothing(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files++
		if len(file.Imports) != 0 {
			t.Errorf("%s imports %d package(s), want 0", name, len(file.Imports))
		}
	}
	if files == 0 {
		t.Fatal("no non-test .go file found in the package dir")
	}
}
