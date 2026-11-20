//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Authored by gpuexpert as the independent probe for the #419 lever-2 gate;
// adopted through the PR flow (provenance recorded: the validating model did
// not author the implementation under test).

package shader

import "testing"

// TestCompositeLiveAcrossBarrierSurvivesResume is gpuexpert's independent
// probe of the invocation arena's suspension lifetime: a composite SSA value
// built BEFORE a workgroup barrier must stay intact when the invocation
// resumes after it. The arena is shared by the whole call tree and must NOT
// be reset on resume — only a new entry-point invocation (beginInvocation)
// may reuse the buffers. A post-barrier composite then lands in fresh slots.
//
// Proposed pinning test for the #419 lever-2 gate: the existing workgroup
// suite keeps its pre-barrier state in workgroup memory (raw bytes), so no
// upstream test fails when run() wrongly resets the arena on every resume
// (verified by mutation: beginInvocation at the top of run() leaves the whole
// shader suite green). This test goes red under that mutation.
func TestCompositeLiveAcrossBarrierSurvivesResume(t *testing.T) {
	m := compileNaga(t, `
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(4)
fn main(@builtin(local_invocation_index) i: u32) {
    let before = array<u32, 4>(10u + i, 20u + i, 30u + i, 40u + i);
    workgroupBarrier();
    let after = array<u32, 4>(99u, 99u, 99u, 99u);
    out[i] = before[0u] + after[0u];
}
`)
	out := make([]uint32, 4)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, out)
	want := []uint32{109, 110, 111, 112}
	checkWords(t, "out", out, want)
}
