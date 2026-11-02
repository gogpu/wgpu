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
