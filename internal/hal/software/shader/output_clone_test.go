//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import "testing"

// TestArrayOutputSurvivesNextInvocation pins the one escape boundary of the
// invocation arena (#419 lever 2 review, gap 1): collectOutputs must deep-copy
// an array output, because beginInvocation reuses the arena for the next
// invocation. Without the clone in collectOutputs this test corrupts 1.5/2.5
// into 99/99 — proven by the reviewer on a throwaway copy with the guard
// removed.
func TestArrayOutputSurvivesNextInvocation(t *testing.T) {
	m := &Module{Variables: map[uint32]*VariableInfo{
		7: {StorageClass: StorageClassOutput},
	}}
	ep := &EntryPoint{InterfaceIDs: []uint32{7}}
	interp := &interpreter{module: m, ep: ep, values: make([]Value, 16)}

	interp.beginInvocation()
	arr := interp.allocArray(2)
	arr[0], arr[1] = ValFloat(1.5), ValFloat(2.5)
	interp.values[7] = ValPointer(interp.allocPointer(ValArray(arr)))
	outputs := make(map[uint32]Value)
	interp.collectOutputs(outputs)

	// The next invocation reuses the same arena buffers.
	interp.beginInvocation()
	over := interp.allocArray(2)
	over[0], over[1] = ValFloat(99), ValFloat(99)

	got := outputs[7].AsArray()
	if len(got) != 2 || got[0].AsFloat32() != 1.5 || got[1].AsFloat32() != 2.5 {
		t.Fatalf("array output corrupted by the next invocation: %v", got)
	}
}
