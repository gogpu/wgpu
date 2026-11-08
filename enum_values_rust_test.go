//go:build rust

package wgpu

import (
	"testing"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
)

// TestRustStencilOperationValues pins the -tags rust stencil operation
// constants to their gputypes values, which are the native build's values
// (descriptor.go), so the two builds cannot drift apart again.
func TestRustStencilOperationValues(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want gputypes.StencilOperation
	}{
		{"Keep", StencilOperationKeep, gputypes.StencilOperationKeep},
		{"Zero", StencilOperationZero, gputypes.StencilOperationZero},
		{"Replace", StencilOperationReplace, gputypes.StencilOperationReplace},
		{"Invert", StencilOperationInvert, gputypes.StencilOperationInvert},
		{"IncrementClamp", StencilOperationIncrementClamp, gputypes.StencilOperationIncrementClamp},
		{"DecrementClamp", StencilOperationDecrementClamp, gputypes.StencilOperationDecrementClamp},
		{"IncrementWrap", StencilOperationIncrementWrap, gputypes.StencilOperationIncrementWrap},
		{"DecrementWrap", StencilOperationDecrementWrap, gputypes.StencilOperationDecrementWrap},
	} {
		if c.got != c.want {
			t.Errorf("StencilOperation%s = %d, want the gputypes value %d", c.name, c.got, c.want)
		}
	}
}

// TestRustErrorFilterToNative checks that each error scope filter reaches
// wgpu-native as the webgpu.h member of the same name. ErrorFilter has no
// gputypes counterpart: its values are core's (Validation = 0), as on the
// native build, while webgpu.h numbers them from 1, so a plain cast pushed a
// scope for the wrong error type (OutOfMemory became Validation).
func TestRustErrorFilterToNative(t *testing.T) {
	for _, c := range []struct {
		filter ErrorFilter
		want   rwgpu.ErrorFilter
	}{
		{ErrorFilterValidation, rwgpu.ErrorFilterValidation},
		{ErrorFilterOutOfMemory, rwgpu.ErrorFilterOutOfMemory},
		{ErrorFilterInternal, rwgpu.ErrorFilterInternal},
	} {
		got, err := errorFilterToRust(c.filter)
		if err != nil {
			t.Errorf("%v: errorFilterToRust = %v, want wgpu-native filter %d", c.filter, err, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("%v: wgpu-native filter %d, want %d", c.filter, got, c.want)
		}
	}
	// An unmapped filter is an error, never a silent Validation fallback:
	// wgpu-native panics on an unknown filter and the panic aborts the process
	// across the FFI boundary (P3-API-025).
	for _, f := range []ErrorFilter{-1, 3, 42} {
		if _, err := errorFilterToRust(f); err == nil {
			t.Errorf("errorFilterToRust(%d) = nil error, want an error", int(f))
		}
	}
}

// TestRustMapModeValues pins the -tags rust MapMode constants, which have no
// gputypes counterpart and are cast to go-webgpu's type in MapAsync, to the
// go-webgpu (webgpu.h) values.
func TestRustMapModeValues(t *testing.T) {
	if rwgpu.MapMode(MapModeRead) != rwgpu.MapModeRead || rwgpu.MapMode(MapModeWrite) != rwgpu.MapModeWrite {
		t.Errorf("MapMode read/write = %d/%d, want the wgpu-native values %d/%d",
			MapModeRead, MapModeWrite, rwgpu.MapModeRead, rwgpu.MapModeWrite)
	}
}

// TestRustDepthStencilStencilOps checks that every stencil operation reaches
// wgpu-native as the same gputypes value. Before the fix the -tags rust
// build had its own StencilOperation numbering (Keep = 0) cast straight to
// go-webgpu's gputypes-based type, so every operation arrived shifted by one.
func TestRustDepthStencilStencilOps(t *testing.T) {
	ops := []gputypes.StencilOperation{
		gputypes.StencilOperationKeep, gputypes.StencilOperationZero, gputypes.StencilOperationReplace,
		gputypes.StencilOperationInvert, gputypes.StencilOperationIncrementClamp,
		gputypes.StencilOperationDecrementClamp, gputypes.StencilOperationIncrementWrap,
		gputypes.StencilOperationDecrementWrap,
	}
	for _, op := range ops {
		face := StencilFaceState{Compare: gputypes.CompareFunctionAlways, FailOp: op, DepthFailOp: op, PassOp: op}
		r := convertDepthStencilState(&DepthStencilState{
			Format:       gputypes.TextureFormatDepth24PlusStencil8,
			DepthCompare: gputypes.CompareFunctionAlways,
			StencilFront: face, StencilBack: face,
		})
		for name, f := range map[string]rwgpu.StencilFaceState{"front": r.StencilFront, "back": r.StencilBack} {
			if f.FailOp != op || f.DepthFailOp != op || f.PassOp != op {
				t.Errorf("%v %s: wgpu-native ops (%v, %v, %v), want %v", op, name, f.FailOp, f.DepthFailOp, f.PassOp, op)
			}
		}
	}
}
