//go:build js && wasm

package browser

import "testing"

// TestBuildExtent3DExplicitZero checks that a copy extent reaches the browser
// with all three members, zero included. GPUExtent3DDict defaults height and
// depthOrArrayLayers to 1 (index.bs), so leaving out a zero turned an empty
// copy into a copy of one row or one layer; a zero copy size is an explicit
// value here, as on the native backends.
//
// It needs Node.js: GOOS=js GOARCH=wasm go test -c -o x.wasm, then
// node "$(go env GOROOT)/lib/wasm/wasm_exec_node.js" x.wasm -test.run Extent3D
func TestBuildExtent3DExplicitZero(t *testing.T) {
	for _, c := range []struct{ w, h, d uint32 }{
		{0, 0, 0}, {4, 0, 1}, {4, 4, 0}, {4, 4, 6},
	} {
		obj := BuildExtent3D(c.w, c.h, c.d)
		for key, want := range map[string]uint32{"width": c.w, "height": c.h, "depthOrArrayLayers": c.d} {
			v := obj.Get(key)
			if v.IsUndefined() {
				t.Errorf("%v: %s left out, want %d", c, key, want)
				continue
			}
			if got := uint32(v.Int()); got != want {
				t.Errorf("%v: %s = %d, want %d", c, key, got, want)
			}
		}
	}
}

// TestDepthStencilMasksZeroSent: zero stencil masks must reach the browser,
// which otherwise applies the default 0xFFFFFFFF.
func TestDepthStencilMasksZeroSent(t *testing.T) {
	for _, m := range []struct{ read, write uint32 }{{0, 0}, {0, 0xFF}, {0x0F, 0}} {
		d := DepthStencilStateJS{Format: "depth24plus-stencil8", StencilReadMask: m.read, StencilWriteMask: m.write}
		obj := d.ToJS()
		for key, want := range map[string]uint32{"stencilReadMask": m.read, "stencilWriteMask": m.write} {
			v := obj.Get(key)
			if v.IsUndefined() {
				t.Errorf("%v: %s left out", m, key)
				continue
			}
			if got := uint32(v.Int()); got != want {
				t.Errorf("%v: %s = %#x, want %#x", m, key, got, want)
			}
		}
	}
}

// TestColorTargetWriteMaskZeroSent: writeMask 0 (GPUColorWrite none) must
// reach the browser, which otherwise applies the default ALL.
func TestColorTargetWriteMaskZeroSent(t *testing.T) {
	for _, mask := range []uint32{0, 0x2, 0xF} {
		c := ColorTargetStateJS{Format: "rgba8unorm", WriteMask: mask}
		v := c.ToJS().Get("writeMask")
		if v.IsUndefined() {
			t.Errorf("writeMask %#x left out", mask)
			continue
		}
		if got := uint32(v.Int()); got != mask {
			t.Errorf("writeMask = %#x, want %#x", got, mask)
		}
	}
}
