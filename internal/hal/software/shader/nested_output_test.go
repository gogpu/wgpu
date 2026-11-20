//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Authored by gpuexpert as the independent probe for the #419 lever-2 gate;
// adopted through the PR flow (provenance recorded: the validating model did
// not author the implementation under test).

package shader

import "testing"

// TestNestedArrayOutputSurvivesNextInvocation is gpuexpert's independent probe
// of the collectOutputs escape guard: a STRUCT output whose member is an array
// (e.g. an output block with clip distances inside). cloneValue must recurse —
// a shallow copy of the outer backing would leave the inner TagArray element
// pointing into the invocation arena, which the next invocation overwrites.
// The upstream guard test pins only a flat array; this pins the recursion.
func TestNestedArrayOutputSurvivesNextInvocation(t *testing.T) {
	m := &Module{Variables: map[uint32]*VariableInfo{
		7: {StorageClass: StorageClassOutput},
	}}
	ep := &EntryPoint{InterfaceIDs: []uint32{7}}
	interp := &interpreter{module: m, ep: ep, values: make([]Value, 16)}

	interp.beginInvocation()
	inner := interp.allocArray(2)
	inner[0], inner[1] = ValFloat(3.5), ValFloat(4.5)
	outer := interp.allocArray(2)
	outer[0] = ValFloat(1.5)
	outer[1] = ValArray(inner) // struct member that is itself an array
	interp.values[7] = ValPointer(interp.allocPointer(ValArray(outer)))
	outputs := make(map[uint32]Value)
	interp.collectOutputs(outputs)

	// The next invocation reuses the same arena slots (inner at 0..1, outer
	// at 2..3); overwrite all four.
	interp.beginInvocation()
	over := interp.allocArray(4)
	for i := range over {
		over[i] = ValFloat(99)
	}

	got := outputs[7].AsArray()
	if len(got) != 2 || got[0].AsFloat32() != 1.5 {
		t.Fatalf("outer output corrupted by the next invocation: %v", got)
	}
	nested := got[1].AsArray()
	if len(nested) != 2 || nested[0].AsFloat32() != 3.5 || nested[1].AsFloat32() != 4.5 {
		t.Fatalf("nested array member corrupted by the next invocation (shallow clone?): %v", nested)
	}
}
