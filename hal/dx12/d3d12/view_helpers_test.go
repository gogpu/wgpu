// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package d3d12

import (
	"encoding/binary"
	"testing"
)

func TestSetTexture2DArrayEncodesPlaneSlice(t *testing.T) {
	var desc D3D12_SHADER_RESOURCE_VIEW_DESC
	desc.SetTexture2DArray(2, 3, 4, 5, 1, 0)

	if got := binary.LittleEndian.Uint32(desc.Union[16:20]); got != 1 {
		t.Fatalf("plane slice = %d, want 1", got)
	}
}

// TestRTVTexture2DArrayEncodesAllFields checks D3D12_TEX2D_ARRAY_RTV
// (MipSlice, FirstArraySlice, ArraySize, PlaneSlice). PlaneSlice used to be
// dropped because the Go union was 4 bytes short of the C union.
func TestRTVTexture2DArrayEncodesAllFields(t *testing.T) {
	var desc D3D12_RENDER_TARGET_VIEW_DESC
	desc.SetTexture2DArray(2, 3, 4, 1)

	le := binary.LittleEndian
	got := [4]uint32{le.Uint32(desc.Union[0:4]), le.Uint32(desc.Union[4:8]), le.Uint32(desc.Union[8:12]), le.Uint32(desc.Union[12:16])}
	if want := [4]uint32{2, 3, 4, 1}; got != want {
		t.Fatalf("TEX2D_ARRAY_RTV = %v, want %v", got, want)
	}
}

// TestDSVTexture2DArrayEncodesArraySize checks D3D12_TEX2D_ARRAY_DSV
// (MipSlice, FirstArraySlice, ArraySize). ArraySize used to be dropped
// because the Go union was 4 bytes short of the C union, so the runtime read
// it from whatever followed the Go value.
func TestDSVTexture2DArrayEncodesArraySize(t *testing.T) {
	var desc D3D12_DEPTH_STENCIL_VIEW_DESC
	desc.SetTexture2DArray(2, 3, 4)

	le := binary.LittleEndian
	got := [3]uint32{le.Uint32(desc.Union[0:4]), le.Uint32(desc.Union[4:8]), le.Uint32(desc.Union[8:12])}
	if want := [3]uint32{2, 3, 4}; got != want {
		t.Fatalf("TEX2D_ARRAY_DSV = %v, want %v", got, want)
	}
}

// TestUAVTextureHelpersEncodeHeaderLayout checks the texture members of the
// D3D12_UNORDERED_ACCESS_VIEW_DESC union against d3d12.h: D3D12_TEX1D_UAV
// (MipSlice), D3D12_TEX2D_UAV (MipSlice, PlaneSlice), D3D12_TEX2D_ARRAY_UAV
// (MipSlice, FirstArraySlice, ArraySize, PlaneSlice) and D3D12_TEX3D_UAV
// (MipSlice, FirstWSlice, WSize).
func TestUAVTextureHelpersEncodeHeaderLayout(t *testing.T) {
	le := binary.LittleEndian
	words := func(desc *D3D12_UNORDERED_ACCESS_VIEW_DESC) [4]uint32 {
		return [4]uint32{le.Uint32(desc.Union[0:4]), le.Uint32(desc.Union[4:8]), le.Uint32(desc.Union[8:12]), le.Uint32(desc.Union[12:16])}
	}
	tests := []struct {
		name string
		set  func(*D3D12_UNORDERED_ACCESS_VIEW_DESC)
		dim  D3D12_UAV_DIMENSION
		want [4]uint32
	}{
		{"1D", func(d *D3D12_UNORDERED_ACCESS_VIEW_DESC) { d.SetTexture1D(3) }, D3D12_UAV_DIMENSION_TEXTURE1D, [4]uint32{3, 0, 0, 0}},
		{"2D", func(d *D3D12_UNORDERED_ACCESS_VIEW_DESC) { d.SetTexture2D(2, 1) }, D3D12_UAV_DIMENSION_TEXTURE2D, [4]uint32{2, 1, 0, 0}},
		{"2DArray", func(d *D3D12_UNORDERED_ACCESS_VIEW_DESC) { d.SetTexture2DArray(2, 3, 4, 1) }, D3D12_UAV_DIMENSION_TEXTURE2DARRAY, [4]uint32{2, 3, 4, 1}},
		{"3D", func(d *D3D12_UNORDERED_ACCESS_VIEW_DESC) { d.SetTexture3D(1, 0, ^uint32(0)) }, D3D12_UAV_DIMENSION_TEXTURE3D, [4]uint32{1, 0, ^uint32(0), 0}},
	}
	for _, tt := range tests {
		var desc D3D12_UNORDERED_ACCESS_VIEW_DESC
		tt.set(&desc)
		if desc.ViewDimension != tt.dim {
			t.Errorf("%s: ViewDimension = %d, want %d", tt.name, desc.ViewDimension, tt.dim)
		}
		if got := words(&desc); got != tt.want {
			t.Errorf("%s: union = %v, want %v", tt.name, got, tt.want)
		}
	}
}
