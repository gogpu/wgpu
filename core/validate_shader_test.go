//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/wgpu/internal/shaderir"
)

// TestWorkgroupStorageSize pins WebGPU's workgroupStorageUsed: each
// workgroup variable the entry point uses statically, callees included,
// rounded up to 16 bytes; variables it does not use and other address
// spaces do not count.
func TestWorkgroupStorageSize(t *testing.T) {
	t.Parallel()
	const source = `
var<workgroup> scalar: f32;
var<workgroup> vec3s: array<vec3<f32>, 3>;
var<workgroup> tile: array<vec4<f32>, 64>;
var<workgroup> other: array<u32, 100>;
var<private> p: array<u32, 1000>;
fn helper() { tile[0] = vec4<f32>(1.0); }
@compute @workgroup_size(1) fn a() { scalar = 1.0; vec3s[0] = vec3<f32>(f32(p[0])); }
@compute @workgroup_size(1) fn b() { helper(); }
@compute @workgroup_size(1) fn c() { other[0] = 1u; }
@compute @workgroup_size(1) fn d() {}
`
	module, err := shaderir.Lower(source)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	tests := []struct {
		entryPoint string
		want       uint64
	}{
		{"a", 16 + 48}, // f32 → 16; array<vec3<f32>, 3> has stride 16 → 48
		{"b", 1024},    // through helper()
		{"c", 400},
		{"d", 0},
	}
	for i, tt := range tests {
		if got := module.IR.EntryPoints[i].Name; got != tt.entryPoint {
			t.Fatalf("entry point %d is %q, want %q", i, got, tt.entryPoint)
		}
		if got := WorkgroupStorageSize(module.IR, module.Info.EntryPoints[i].GlobalUses); got != tt.want {
			t.Errorf("%s: WorkgroupStorageSize = %d, want %d", tt.entryPoint, got, tt.want)
		}
	}
	if got := WorkgroupStorageSize(nil, nil); got != 0 {
		t.Errorf("nil module: %d, want 0", got)
	}
}
