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
//   - Vulkan: with robustBufferAccess2 (enabled wherever supported, as in
//     Rust) the driver discards the write and the read returns 0. Without
//     it naga's Restrict policy clamps the index to the last element in the
//     shader, so the write must land in a[3] and the read must return a[3]'s
//     old value. Both are checked exactly.
//   - DX12: a runtime-sized array is bounded by the descriptor's raw buffer
//     view, which D3D12 itself contains: the write is discarded and the read
//     returns 0 (as in Rust wgpu-hal, whose naga HLSL restrict_indexing
//     covers fixed-size arrays, vectors and matrices only).
//   - Metal: naga's Restrict policy clamps the index in the MSL, bounded by
//     the buffer sizes the HAL passes, so it is checked exactly as on Vulkan
//     without robustBufferAccess2.
//   - GLES: the GLSL policies are Unchecked, as in Rust; the context's
//     robust buffer access (EGL as in Rust, WGL beyond Rust) makes the
//     driver contain the access. Without a robust context nothing guarantees
//     containment, so the test skips there instead of passing by luck.
//   - Software: the interpreter discards the write and reads 0.
func TestStorageBufferOutOfBoundsContained(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, checkStorageOOB)
}

func checkStorageOOB(t *testing.T, b overrideBackend, device *wgpu.Device) {
	skipWithoutRobustGL(t, b, device)
	got := runStorageOOB(t, device, oobShader, oobBindWords*4)
	t.Logf("a = %#x, b = %#x", got[:oobBindWords], got[oobB:oobB+oobBindWords])
	checkOOBContained(t, got)
	if b.backend == gputypes.BackendMetal {
		checkOOBRestricted(t, got)
	}
	if b.backend == gputypes.BackendVulkan && !b.software {
		robust, ok := device.HalDevice().(interface{ RobustBufferAccess2() bool })
		if !ok {
			t.Fatalf("Vulkan device %T does not report RobustBufferAccess2", device.HalDevice())
		}
		if robust.RobustBufferAccess2() {
			checkOOBDiscarded(t, got)
		} else {
			checkOOBRestricted(t, got)
		}
	}
}

// skipWithoutRobustGL skips a GLES device whose context has no robust buffer
// access: nothing then guarantees containment, so the test would pass or
// fail by luck.
func skipWithoutRobustGL(t *testing.T, b overrideBackend, device *wgpu.Device) {
	t.Helper()
	if b.backend != gputypes.BackendGL {
		return
	}
	robust, ok := device.HalDevice().(interface{ RobustAccess() bool })
	if !ok {
		t.Fatalf("GLES device %T does not report RobustAccess", device.HalDevice())
	}
	if !robust.RobustAccess() {
		t.Skip("GL context without robust buffer access: containment is not guaranteed")
	}
}

// runStorageOOB fills one storage buffer with oobFill + i per word, binds
// bytes [0, bindSizeA) as binding a and words oobB..oobB+3 as binding b,
// dispatches shader once and returns the whole buffer.
func runStorageOOB(t *testing.T, device *wgpu.Device, shader string, bindSizeA uint64) []uint32 {
	t.Helper()
	bgl, pipeline, err := newPublicComputePipeline(t, device, shader, nil, 2)
	if err != nil {
		t.Fatal(err)
	}

	const size = oobWords * 4
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
			{Binding: 0, Buffer: storage, Offset: 0, Size: bindSizeA},
			{Binding: 1, Buffer: storage, Offset: oobB * 4, Size: oobBindWords * 4},
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

	return readPublicBuffer(t, staging, oobWords)
}

// The vec3 case: a is array<vec3<u32>>, stride 16 and element size 12, bound
// with 28 bytes (7 words). WGSL defines its length as
// truncate(28 / stride) = 1 (wgsl/index.bs, NRuntime), so only a[0] is in
// bounds. The 12 bytes after it still lie inside the binding, which is where
// a bound by element size instead of stride would clamp to: the test accepts
// either clamp and requires arrayLength to follow the spec.
const (
	oobVec3BindWords = 7
	oobVec3Length    = 1        // truncate(28 / 16)
	oobVec3LenWord   = oobB     // b[0] = arrayLength(&a)
	oobVec3ReadWord  = oobB + 2 // b[2] = the x of the out-of-bounds read
)

// a[16] is b[0..2] and a[17] is words 68..70, past b, when unchecked.
var oobVec3Shader = fmt.Sprintf(`
@group(0) @binding(0) var<storage, read_write> a: array<vec3<u32>>;
@group(0) @binding(1) var<storage, read_write> b: array<u32, 4>;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let r = a[16u + gid.x];
    a[17u + gid.x] = vec3<u32>(%du, %du, %du);
    b[0] = arrayLength(&a);
    b[2] = r.x;
}
`, oobWritten, oobWritten+1, oobWritten+2)

// TestStorageBufferOutOfBoundsContainedVec3 checks containment and the WGSL
// array length for a runtime-sized array whose element is smaller than its
// stride, with a binding that ends inside the last element's padding.
func TestStorageBufferOutOfBoundsContainedVec3(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		skipWithoutRobustGL(t, b, device)
		got := runStorageOOB(t, device, oobVec3Shader, oobVec3BindWords*4)
		t.Logf("a = %#x, b = %#x", got[:oobVec3BindWords], got[oobB:oobB+oobBindWords])
		if got[oobVec3LenWord] != oobVec3Length {
			t.Errorf("arrayLength = %d, want %d: truncate(binding size / stride)", got[oobVec3LenWord], oobVec3Length)
		}
		for i, w := range got {
			old := oobFill + uint32(i)
			switch {
			case i == oobVec3LenWord:
			case i == oobVec3ReadWord:
				if w != 0 && (w < oobFill || w >= oobFill+oobVec3BindWords) {
					t.Errorf("the out-of-bounds read returned %#x: neither zero nor a value of a", w)
				}
			case i < oobVec3BindWords:
				if w != old && (w < oobWritten || w > oobWritten+2) {
					t.Errorf("a word %d = %#x: neither unchanged nor a written value", i, w)
				}
			case w != old:
				t.Errorf("word %d = %#x, want %#x (unchanged): the out-of-bounds access escaped a", i, w, old)
			}
		}
	})
}

// checkOOBDiscarded checks robustBufferAccess2 for a storage buffer, whose
// robustStorageBufferAccessSizeAlignment is at most 4: the write is
// discarded and the read returns zero.
func checkOOBDiscarded(t *testing.T, got []uint32) {
	for i := range oobBindWords {
		if got[i] == oobWritten {
			t.Errorf("a[%d] = %#x: robustBufferAccess2 must discard the write", i, got[i])
		}
	}
	if got[oobStore] != 0 {
		t.Errorf("the read of a[%d] returned %#x, want 0 under robustBufferAccess2", oobRead, got[oobStore])
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
