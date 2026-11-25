// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

// TestTlasInstanceToBytesLayout pins the byte layout of one instance against
// MTLIndirectAccelerationStructureInstanceDescriptor (sizeof 72, accelerationStructureID
// at offset 64, checked against the SDK header). Every field gets a different
// value so a shifted field shows up.
func TestTlasInstanceToBytesLayout(t *testing.T) {
	in := hal.TlasInstance{
		Transform:                      [12]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		CustomData:                     0xA1A2A3A4,
		Mask:                           0x5A,
		BlasAddress:                    0x1122334455667788,
		ShaderBindingTableRecordOffset: 0xB1B2B3B4,
	}
	b := tlasInstanceToBytes(in)
	if len(b) != 72 {
		t.Fatalf("instance is %d bytes, want 72", len(b))
	}
	// The row-major 3x4 matrix is stored as four packed float3 columns.
	wantCols := [12]float32{1, 5, 9, 2, 6, 10, 3, 7, 11, 4, 8, 12}
	for i, w := range wantCols {
		got := math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
		if got != w {
			t.Errorf("matrix word %d = %v, want %v", i, got, w)
		}
	}
	u32 := func(off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
	if u32(48) != 0 {
		t.Errorf("options = %#x, want 0", u32(48))
	}
	if u32(52) != 0x5A {
		t.Errorf("mask = %#x, want 0x5a", u32(52))
	}
	if u32(56) != 0xB1B2B3B4 {
		t.Errorf("intersectionFunctionTableOffset = %#x", u32(56))
	}
	if u32(60) != 0xA1A2A3A4 {
		t.Errorf("userID = %#x", u32(60))
	}
	if got := binary.LittleEndian.Uint64(b[64:]); got != 0x1122334455667788 {
		t.Errorf("accelerationStructureID = %#x", got)
	}
}
