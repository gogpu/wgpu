// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"testing"

	naga "github.com/gogpu/naga"
)

// runNagaCompute compiles wgsl with naga, dispatches one workgroup of its
// "main" with src as binding 0 and returns binding 1 (dstWords words).
func runNagaCompute(t *testing.T, wgsl string, src []uint32, dstWords int) []uint32 {
	t.Helper()
	spirvBytes, err := naga.Compile(wgsl)
	if err != nil {
		t.Fatalf("naga.Compile: %v", err)
	}
	words := make([]uint32, len(spirvBytes)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
	}
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
	srcBuf := make([]byte, 4*len(src))
	for i, w := range src {
		binary.LittleEndian.PutUint32(srcBuf[i*4:], w)
	}
	dstBuf := make([]byte, 4*dstWords)
	for i := range dstWords {
		binary.LittleEndian.PutUint32(dstBuf[i*4:], 0xFEEDFACE)
	}
	ctx := &ExecutionContext{Buffers: map[BindingKey][]byte{{Group: 0, Binding: 0}: srcBuf, {Group: 0, Binding: 1}: dstBuf}}
	if err := m.DispatchCompute("main", ctx, 1, 1, 1); err != nil {
		t.Fatalf("DispatchCompute: %v", err)
	}
	out := make([]uint32, dstWords)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(dstBuf[i*4:])
	}
	return out
}

// TestMatrixTimesVectorSizes checks matCxR * vecC = vecR for 2- and
// 3-component shapes (naga's readback corpus uniforms/mat-cx2-rt; P1-SW-013):
// only vec4 operands were multiplied, so m * vec2(1, 1) returned (1, 1).
func TestMatrixTimesVectorSizes(t *testing.T) {
	m2 := ValArray([]Value{ValVec2(1, 2), ValVec2(3, 4)})
	if got := matrixTimesVector(m2, ValVec2(1, 1)); got.Tag != TagVec2 || got.F[0] != 4 || got.F[1] != 6 {
		t.Errorf("mat2x2 * (1, 1) = %v %v, want vec2(4, 6)", got.Tag, got.F)
	}
	m3x2 := ValArray([]Value{ValVec2(1, 2), ValVec2(3, 4), ValVec2(5, 6)})
	if got := matrixTimesVector(m3x2, ValVec3(1, 0, 2)); got.Tag != TagVec2 || got.F[0] != 11 || got.F[1] != 14 {
		t.Errorf("mat3x2 * (1, 0, 2) = %v %v, want vec2(11, 14)", got.Tag, got.F)
	}
	m2x3 := ValArray([]Value{ValVec3(1, 2, 3), ValVec3(4, 5, 6)})
	if got := matrixTimesVector(m2x3, ValVec2(1, 1)); got.Tag != TagVec3 || got.F != [4]float32{5, 7, 9, 0} {
		t.Errorf("mat2x3 * (1, 1) = %v %v, want vec3(5, 7, 9)", got.Tag, got.F)
	}
}

// TestFunctionVariableStructInitializer checks a function variable whose
// initializer is a struct OpConstantNull and whose members are then
// written: the null struct was a scalar 0, so the member stores went nowhere.
func TestFunctionVariableStructInitializer(t *testing.T) {
	const wgsl = `
struct P { a: u32, b: u32 }
@group(0) @binding(0) var<storage, read> src: array<u32, 2>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

@compute @workgroup_size(1)
fn main() {
    var p = P(src[0], src[1]);
    p.b += 10u;
    dst[0] = p.a;
    dst[1] = p.b;
}
`
	got := runNagaCompute(t, wgsl, []uint32{3, 4}, 2)
	if got[0] != 3 || got[1] != 14 {
		t.Errorf("dst = %v, want [3 14]", got)
	}
	types := map[uint32]*TypeInfo{1: {Kind: TypeInt}, 2: {Kind: TypeStruct, MemberIDs: []uint32{1, 1}}}
	if z := zeroValue(types, 2); z.Tag != TagArray || len(z.AsArray()) != 2 {
		t.Errorf("zeroValue(struct) = tag %v, want an array of 2 members", z.Tag)
	}
}

// TestPrivateVariablesInitialized checks var<private> globals read before any
// write: zero without an initializer, the initializer otherwise (naga's
// readback corpus vars/uninit-read-rt; P1-SW-013). Private globals were never
// set up, so loads produced no value and the stores wrote nothing.
func TestPrivateVariablesInitialized(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var<storage, read> src: array<u32, 1>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

var<private> p: i32;
var<private> q: u32 = 9u;

@compute @workgroup_size(1)
fn main() {
    if src[0] == 7u {
        p = 6;
        q = 4u;
    }
    dst[0] = bitcast<u32>(p);
    dst[1] = q;
}
`
	got := runNagaCompute(t, wgsl, []uint32{0}, 2)
	if got[0] != 0 || got[1] != 9 {
		t.Errorf("dst = %v, want [0 9]", got)
	}
}
