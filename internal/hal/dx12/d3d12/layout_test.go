// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package d3d12

import (
	"testing"
	"unsafe"
)

// unionLayout is the x64 layout of a d3d12.h struct that holds an anonymous
// union, which the Go binding models as a Union byte array.
type unionLayout struct {
	name        string
	size, align uintptr // sizeof and alignof of the whole struct
	unionOffset uintptr
	unionSize   uintptr
}

// TestUnionStructLayout checks every Go struct that models a C union with a
// byte array against d3d12.h (Windows SDK 10.0.26100.0, x64).
//
// Two properties matter. The size and union offset must match C, or the
// runtime reads past the Go value (or reads the wrong fields), and arrays of
// the struct get the wrong stride. The alignment must match C too: a
// [N]byte union has alignment 1, so without an 8-byte member the Go struct
// is only 4-aligned while the C union holds pointers or UINT64s. A local of
// such a struct can then sit at a 4-aligned address, and the cast of
// &x.Union[0] to a pointer-holding struct is misaligned (checkptr fatal).
//
// Expected values, the C union's largest member in brackets:
//   - D3D12_RESOURCE_BARRIER: TRANSITION_BARRIER (pointer + 3 UINT) = 24, align 8
//   - D3D12_SHADER_RESOURCE_VIEW_DESC: BUFFER_SRV / TEX2D_ARRAY_SRV = 24, align 8
//   - D3D12_UNORDERED_ACCESS_VIEW_DESC: BUFFER_UAV (two UINT64) = 32, align 8
//   - D3D12_RENDER_TARGET_VIEW_DESC: BUFFER_RTV (UINT64 + UINT) / TEX2D_ARRAY_RTV = 16, align 8
//   - D3D12_DEPTH_STENCIL_VIEW_DESC: TEX1D/TEX2D_ARRAY_DSV (3 UINT) = 12, align 4
//   - D3D12_TEXTURE_COPY_LOCATION: PLACED_SUBRESOURCE_FOOTPRINT = 32, align 8
//   - D3D12_INDIRECT_ARGUMENT_DESC: Constant (3 UINT) = 12, align 4
//   - D3D12_RENDER_PASS_BEGINNING_ACCESS: CLEAR_PARAMETERS (D3D12_CLEAR_VALUE) = 20, align 4
//   - D3D12_RENDER_PASS_ENDING_ACCESS: RESOLVE_PARAMETERS (3 pointers) = 48, align 8
//   - D3D12_RAYTRACING_GEOMETRY_DESC: TRIANGLES_DESC = 48, align 8
//   - D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS: GPU VA / pointer = 8, align 8
func TestUnionStructLayout(t *testing.T) {
	tests := []struct {
		want unionLayout
		got  unionLayout
	}{
		{
			unionLayout{"D3D12_RESOURCE_BARRIER", 32, 8, 8, 24},
			layoutOf(D3D12_RESOURCE_BARRIER{}, unsafe.Offsetof(D3D12_RESOURCE_BARRIER{}.Union), unsafe.Sizeof(D3D12_RESOURCE_BARRIER{}.Union)),
		},
		{
			unionLayout{"D3D12_SHADER_RESOURCE_VIEW_DESC", 40, 8, 16, 24},
			layoutOf(D3D12_SHADER_RESOURCE_VIEW_DESC{}, unsafe.Offsetof(D3D12_SHADER_RESOURCE_VIEW_DESC{}.Union), unsafe.Sizeof(D3D12_SHADER_RESOURCE_VIEW_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_UNORDERED_ACCESS_VIEW_DESC", 40, 8, 8, 32},
			layoutOf(D3D12_UNORDERED_ACCESS_VIEW_DESC{}, unsafe.Offsetof(D3D12_UNORDERED_ACCESS_VIEW_DESC{}.Union), unsafe.Sizeof(D3D12_UNORDERED_ACCESS_VIEW_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_RENDER_TARGET_VIEW_DESC", 24, 8, 8, 16},
			layoutOf(D3D12_RENDER_TARGET_VIEW_DESC{}, unsafe.Offsetof(D3D12_RENDER_TARGET_VIEW_DESC{}.Union), unsafe.Sizeof(D3D12_RENDER_TARGET_VIEW_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_DEPTH_STENCIL_VIEW_DESC", 24, 4, 12, 12},
			layoutOf(D3D12_DEPTH_STENCIL_VIEW_DESC{}, unsafe.Offsetof(D3D12_DEPTH_STENCIL_VIEW_DESC{}.Union), unsafe.Sizeof(D3D12_DEPTH_STENCIL_VIEW_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_TEXTURE_COPY_LOCATION", 48, 8, 16, 32},
			layoutOf(D3D12_TEXTURE_COPY_LOCATION{}, unsafe.Offsetof(D3D12_TEXTURE_COPY_LOCATION{}.Union), unsafe.Sizeof(D3D12_TEXTURE_COPY_LOCATION{}.Union)),
		},
		{
			unionLayout{"D3D12_INDIRECT_ARGUMENT_DESC", 16, 4, 4, 12},
			layoutOf(D3D12_INDIRECT_ARGUMENT_DESC{}, unsafe.Offsetof(D3D12_INDIRECT_ARGUMENT_DESC{}.Union), unsafe.Sizeof(D3D12_INDIRECT_ARGUMENT_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_RENDER_PASS_BEGINNING_ACCESS", 24, 4, 4, 20},
			layoutOf(D3D12_RENDER_PASS_BEGINNING_ACCESS{}, unsafe.Offsetof(D3D12_RENDER_PASS_BEGINNING_ACCESS{}.Union), unsafe.Sizeof(D3D12_RENDER_PASS_BEGINNING_ACCESS{}.Union)),
		},
		{
			unionLayout{"D3D12_RENDER_PASS_ENDING_ACCESS", 56, 8, 8, 48},
			layoutOf(D3D12_RENDER_PASS_ENDING_ACCESS{}, unsafe.Offsetof(D3D12_RENDER_PASS_ENDING_ACCESS{}.Union), unsafe.Sizeof(D3D12_RENDER_PASS_ENDING_ACCESS{}.Union)),
		},
		{
			unionLayout{"D3D12_RAYTRACING_GEOMETRY_DESC", 56, 8, 8, 48},
			layoutOf(D3D12_RAYTRACING_GEOMETRY_DESC{}, unsafe.Offsetof(D3D12_RAYTRACING_GEOMETRY_DESC{}.Union), unsafe.Sizeof(D3D12_RAYTRACING_GEOMETRY_DESC{}.Union)),
		},
		{
			unionLayout{"D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS", 24, 8, 16, 8},
			layoutOf(D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS{}, unsafe.Offsetof(D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS{}.Union), unsafe.Sizeof(D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS{}.Union)),
		},
	}
	for _, tt := range tests {
		got := tt.got
		got.name = tt.want.name
		if got != tt.want {
			t.Errorf("%s layout = {size %d, align %d, union offset %d, union size %d}, want {%d, %d, %d, %d}",
				tt.want.name, got.size, got.align, got.unionOffset, got.unionSize,
				tt.want.size, tt.want.align, tt.want.unionOffset, tt.want.unionSize)
		}
	}
}

// layoutOf collects the layout of v; the union offset and size are passed in
// because unsafe.Offsetof needs a field selector at the call site.
func layoutOf[T any](v T, unionOffset, unionSize uintptr) unionLayout {
	return unionLayout{size: unsafe.Sizeof(v), align: unsafe.Alignof(v), unionOffset: unionOffset, unionSize: unionSize}
}

// TestUnionVariantLayout checks the C structs that the binding writes into a
// Union through a pointer cast. Each must fit the union, and the Go type must
// match d3d12.h, or the cast writes the wrong bytes.
func TestUnionVariantLayout(t *testing.T) {
	tests := []struct {
		name        string
		size, align uintptr
		wantSize    uintptr
		unionSize   uintptr
	}{
		{"D3D12_RESOURCE_TRANSITION_BARRIER", unsafe.Sizeof(D3D12_RESOURCE_TRANSITION_BARRIER{}), unsafe.Alignof(D3D12_RESOURCE_TRANSITION_BARRIER{}), 24, unsafe.Sizeof(D3D12_RESOURCE_BARRIER{}.Union)},
		{"D3D12_RESOURCE_ALIASING_BARRIER", unsafe.Sizeof(D3D12_RESOURCE_ALIASING_BARRIER{}), unsafe.Alignof(D3D12_RESOURCE_ALIASING_BARRIER{}), 16, unsafe.Sizeof(D3D12_RESOURCE_BARRIER{}.Union)},
		{"D3D12_RESOURCE_UAV_BARRIER", unsafe.Sizeof(D3D12_RESOURCE_UAV_BARRIER{}), unsafe.Alignof(D3D12_RESOURCE_UAV_BARRIER{}), 8, unsafe.Sizeof(D3D12_RESOURCE_BARRIER{}.Union)},
		{"D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC", unsafe.Sizeof(D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC{}), unsafe.Alignof(D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC{}), 48, unsafe.Sizeof(D3D12_RAYTRACING_GEOMETRY_DESC{}.Union)},
		{"D3D12_RAYTRACING_GEOMETRY_AABBS_DESC", unsafe.Sizeof(D3D12_RAYTRACING_GEOMETRY_AABBS_DESC{}), unsafe.Alignof(D3D12_RAYTRACING_GEOMETRY_AABBS_DESC{}), 24, unsafe.Sizeof(D3D12_RAYTRACING_GEOMETRY_DESC{}.Union)},
	}
	for _, tt := range tests {
		if tt.size != tt.wantSize {
			t.Errorf("%s size = %d, want %d", tt.name, tt.size, tt.wantSize)
		}
		if tt.size > tt.unionSize {
			t.Errorf("%s (%d bytes) does not fit its union (%d bytes)", tt.name, tt.size, tt.unionSize)
		}
		if tt.align != 8 {
			t.Errorf("%s align = %d, want 8", tt.name, tt.align)
		}
	}
}

// TestUnionCastAlignment builds the structs the way callers do (locals and
// slice elements) and casts the union the way the binding does. Under
// -race (which enables checkptr) a misaligned cast is a fatal error, so this
// test is the runtime counterpart of TestUnionStructLayout.
func TestUnionCastAlignment(t *testing.T) {
	var res ID3D12Resource
	barriers := []D3D12_RESOURCE_BARRIER{
		NewTransitionBarrier(&res, D3D12_RESOURCE_STATE_COMMON, D3D12_RESOURCE_STATE_COPY_DEST, 0),
		NewUAVBarrier(&res),
		NewAliasingBarrier(&res, &res),
	}
	for i := range barriers {
		if addr := uintptr(unsafe.Pointer(&barriers[i].Union[0])); addr%8 != 0 {
			t.Errorf("barrier %d union at %#x, not 8-aligned", i, addr)
		}
	}

	geoms := make([]D3D12_RAYTRACING_GEOMETRY_DESC, 3)
	for i := range geoms {
		geoms[i].SetTriangles(D3D12_RAYTRACING_GEOMETRY_TRIANGLES_DESC{VertexCount: uint32(i + 1)})
	}
	var inputs D3D12_BUILD_RAYTRACING_ACCELERATION_STRUCTURE_INPUTS
	inputs.SetGeometryDescs(unsafe.Pointer(&geoms[0]))
	inputs.SetInstanceDescs(0x1000)
	if got := *(*uint64)(unsafe.Pointer(&inputs.Union[0])); got != 0x1000 {
		t.Errorf("InstanceDescs = %#x, want 0x1000", got)
	}
}
