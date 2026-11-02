// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The storage layout: one buffer holds binding a at word 0 and binding b at
// word 64 (byte 256, the default minStorageBufferOffsetAlignment), 4 words
// each. The out-of-bounds indices point into b, so an unchecked access hits b
// deterministically instead of whatever memory follows a.
const (
	oobWords     = 128
	oobBindWords = 4
	oobB         = 64       // first word of binding b
	oobWrite     = oobB + 1 // a[65] is b[1] when unchecked
	oobRead      = oobB + 2 // a[66] is b[2] when unchecked
	oobStore     = oobB + 2 // b[2], where the shader stores what it read
	oobLastA     = 3        // the last element of a, where Restrict clamps
	oobFill      = 0x1000   // word i starts as oobFill + i
	oobWritten   = uint32(0xA1)
)

// oobShader reads and writes the runtime-sized array a past its end. Both
// indices are dynamic, so neither can be folded or rejected at compile time.
// Under the Restrict bounds-check policy both accesses clamp to a[3]: r gets
// a[3]'s old value, then the write lands in a[3].
var oobShader = fmt.Sprintf(`
@group(0) @binding(0) var<storage, read_write> a: array<u32>;
@group(0) @binding(1) var<storage, read_write> b: array<u32, 4>;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let r = a[%du + gid.x];
    a[%du + gid.x] = %du;
    b[%d] = r;
}
`, oobRead, oobWrite, oobWritten, oobStore-oobB)

// TestStorageBufferOutOfBoundsContained checks WebGPU's out-of-bounds
// containment for storage buffers (#370) on every backend and on each adapter
// the power preferences reach: a shader access past the end of a binding must
// not touch memory outside it. WebGPU lets the write be discarded or land
// inside the binding, and the read return zero or a value from inside it.
//
// How each backend contains it differs:
//   - Vulkan: naga's Restrict policy clamps the index to the last element
//     in the shader (we enable no robustBufferAccess), so the write must land
//     in a[3] and the read must return a[3]'s old value. This is checked
//     exactly.
//   - DX12: a runtime-sized array is bounded by the descriptor's raw buffer
//     view, which D3D12 itself contains: the write is discarded and the read
//     returns 0 (as in Rust wgpu-hal, whose naga HLSL restrict_indexing
//     covers fixed-size arrays, vectors and matrices only).
//   - GLES: the GLSL policies are Unchecked, as in Rust; containment comes
//     from the driver.
//   - Software: the interpreter discards the write and reads 0.
func TestStorageBufferOutOfBoundsContained(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, checkStorageOOB)
}

func checkStorageOOB(t *testing.T, b overrideBackend, device *wgpu.Device) {
	bgl, pipeline, err := newPublicComputePipeline(t, device, oobShader, nil, 2)
	if err != nil {
		t.Fatal(err)
	}

	const size, bindSize = oobWords * 4, oobBindWords * 4
	storage := newPublicBuffer(t, device, size, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	staging := newPublicBuffer(t, device, size, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	initial := make([]byte, size)
	for i := 0; i < oobWords; i++ {
		binary.LittleEndian.PutUint32(initial[i*4:], oobFill+uint32(i))
	}
	if err := device.Queue().WriteBuffer(storage, 0, initial); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "oob", Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: storage, Offset: 0, Size: bindSize},
			{Binding: 1, Buffer: storage, Offset: oobB * 4, Size: bindSize},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyBufferToBuffer(storage, 0, staging, 0, size)
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got := readPublicBuffer(t, staging, oobWords)
	t.Logf("a = %#x, b = %#x", got[:oobBindWords], got[oobB:oobB+oobBindWords])
	checkOOBContained(t, got)
	if b.backend == gputypes.BackendVulkan && !b.software {
		checkOOBRestricted(t, got)
	}
}

// checkOOBContained checks what WebGPU requires of an out-of-bounds access:
// the write lands inside a or nowhere, the read returns zero or a value from
// inside a, and no other word changes.
func checkOOBContained(t *testing.T, got []uint32) {
	if got[oobWrite] != oobFill+oobWrite {
		t.Errorf("the write to a[%d] landed in binding b (word %d = %#x): not contained", oobWrite, oobWrite, got[oobWrite])
	}
	writes := 0
	for i, w := range got {
		old := oobFill + uint32(i)
		switch {
		case i < oobBindWords && w == oobWritten:
			writes++
		case i == oobStore:
			if w != 0 && (w < oobFill || w >= oobFill+oobBindWords) {
				t.Errorf("the read of a[%d] returned %#x: neither zero nor a value of a", oobRead, w)
			}
		case w != old:
			t.Errorf("word %d = %#x, want %#x (unchanged)", i, w, old)
		}
	}
	if writes > 1 {
		t.Errorf("the write to a[%d] landed in %d words of a", oobWrite, writes)
	}
}

// checkOOBRestricted checks naga's Restrict policy for a runtime-sized array:
// both accesses clamp to the last element, so the write lands in a[3] and the
// read returns a[3]'s old value.
func checkOOBRestricted(t *testing.T, got []uint32) {
	if got[oobLastA] != oobWritten {
		t.Errorf("a[%d] = %#x, want %#x: the write was not clamped to the last element", oobLastA, got[oobLastA], oobWritten)
	}
	if want := uint32(oobFill + oobLastA); got[oobStore] != want {
		t.Errorf("the read of a[%d] returned %#x, want %#x: not clamped to the last element", oobRead, got[oobStore], want)
	}
}
