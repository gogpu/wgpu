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
