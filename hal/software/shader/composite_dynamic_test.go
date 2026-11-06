// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import "testing"

// TestCompositeInsertAndDynamicVectorOps runs OpVectorExtractDynamic,
// OpVectorInsertDynamic and OpCompositeInsert. They were skipped as
// unimplemented, so their results stayed undefined: naga lowers v[i] on a
// vector value to OpVectorExtractDynamic, and a store of that result never
// happened (naga readback corpus vars/workgroup-tile-dynamic-rt).
func TestCompositeInsertAndDynamicVectorOps(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}
	vec := ValVec4(10, 11, 12, 13)
	arr := ValArray([]Value{ValVec2(1, 2), ValVec2(3, 4)})
	tests := []struct {
		name string
		inst Instruction
		want Value
	}{
		{"extract dynamic", Instruction{Opcode: OpVectorExtractDynamic, Operands: []uint32{1, 3}}, ValFloat(12)},
		{"extract dynamic past the end", Instruction{Opcode: OpVectorExtractDynamic, Operands: []uint32{1, 5}}, ValFloat(0)},
		{"insert dynamic", Instruction{Opcode: OpVectorInsertDynamic, Operands: []uint32{1, 4, 6}}, ValVec4(10, 99, 12, 13)},
		{"insert dynamic past the end", Instruction{Opcode: OpVectorInsertDynamic, Operands: []uint32{1, 4, 5}}, vec},
		{"composite insert", Instruction{Opcode: OpCompositeInsert, Operands: []uint32{4, 2, 1, 0}}, ValArray([]Value{ValVec2(1, 2), ValVec2(99, 4)})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := tc.inst
			inst.ResultID = 10
			fn := &Function{
				Instructions: []Instruction{{Opcode: OpLabel, ResultID: 100}, inst, {Opcode: OpReturn}},
				Labels:       map[uint32]int{100: 0},
			}
			interp := &interpreter{
				module: m,
				fn:     fn,
				ep:     &EntryPoint{},
				ctx:    &ExecutionContext{},
				values: testMakeValues(map[uint32]any{1: vec, 2: arr, 3: ValUint(2), 4: ValFloat(99), 5: ValUint(4), 6: ValUint(1)}),
			}
			if err := interp.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := interp.values[10]; !deepValuesEqual(got, tc.want) {
				t.Errorf("result = %+v, want %+v", got, tc.want)
			}
			// The operands are values: the instruction must not change them.
			if !deepValuesEqual(interp.values[1], vec) || !deepValuesEqual(interp.values[2], arr) {
				t.Errorf("operand changed: vec %+v, arr %+v", interp.values[1], interp.values[2])
			}
		})
	}
}

// deepValuesEqual compares two values by tag and contents, recursing into
// arrays (valuesEqual compares arrays by reference).
func deepValuesEqual(a, b Value) bool {
	if a.Tag != b.Tag {
		return false
	}
	if a.Tag == TagArray {
		x, y := a.AsArray(), b.AsArray()
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if !deepValuesEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	return valuesEqual(a, b)
}
