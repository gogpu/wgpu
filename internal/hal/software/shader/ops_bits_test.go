// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// TestBitFieldOps checks OpBitFieldInsert, OpBitFieldSExtract and
// OpBitFieldUExtract against the WGSL insertBits / extractBits definitions.
func TestBitFieldOps(t *testing.T) {
	if got := bitFieldInsert(0xFFFF0000, 0xAB, 4, 8); got != 0xFFFF0AB0 {
		t.Errorf("insertBits = %#x, want 0xffff0ab0", got)
	}
	if got := bitFieldInsert(0x12345678, 0xFFFFFFFF, 0, 32); got != 0xFFFFFFFF {
		t.Errorf("insertBits count 32 = %#x, want 0xffffffff", got)
	}
	if got := bitFieldInsert(0x12345678, 1, 3, 0); got != 0x12345678 {
		t.Errorf("insertBits count 0 = %#x, want base", got)
	}
	if got := bitFieldExtract(0x00000F70, 4, 8, false); got != 0xF7 {
		t.Errorf("extractBits u32 = %#x, want 0xf7", got)
	}
	if got := bitFieldExtract(0x00000F70, 4, 8, true); got != 0xFFFFFFF7 {
		t.Errorf("extractBits i32 = %#x, want 0xfffffff7 (sign-extended)", got)
	}
	if got := bitFieldExtract(0x80000000, 31, 4, false); got != 1 {
		t.Errorf("extractBits past bit 31 = %#x, want 1", got)
	}
}

// TestIntLaneOps checks that the bit ops apply per lane of an integer vector,
// give the result shape's kind, and keep signed scalars signed. A float
// operand reads as its bits: the raw-word model has no float-held integers.
func TestIntLaneOps(t *testing.T) {
	uvec2 := shapeOf(kindUint, 2)
	if got := mapIntLanes(uvec2, ValUVec(2, [4]uint32{7, 0xFFFFFFFF}), bitCount); got.Tag != TagUVec2 || got.U[0] != 3 || got.U[1] != 32 {
		t.Errorf("countOneBits(uvec2(7, 0xFFFFFFFF)) = %+v, want (3, 32)", got)
	}
	// 1.0 is 0x3F800000 (7 bits set); -0.0 is 0x80000000 (1 bit set).
	if got := mapIntLanes(uvec2, ValVec2(1, float32(math.Copysign(0, -1))), bitCount); got.Tag != TagUVec2 || got.U[0] != 7 || got.U[1] != 1 {
		t.Errorf("countOneBits(vec2(1, -0)) = %+v, want uvec2(7, 1) from the bits", got)
	}
	if got := mapIntLanes(0, ValInt(-1), bitCount); got.Tag != TagInt32 || got.U[0] != 32 {
		t.Errorf("countOneBits(-1) = %v %v, want i32 32", got.Tag, got.U)
	}
}

// TestFloatNotEqual checks the ordered and unordered inequalities with NaN.
func TestFloatNotEqual(t *testing.T) {
	nan := float32(math.NaN())
	cases := []struct {
		a, b          float32
		ordered, want bool
	}{
		{1, 2, true, true},
		{1, 1, true, false},
		{nan, 1, true, false},
		{nan, 1, false, true},
		{1, 1, false, false},
	}
	for _, c := range cases {
		if got := floatNotEqual(c.a, c.b, c.ordered); got != c.want {
			t.Errorf("floatNotEqual(%v, %v, ordered=%v) = %v, want %v", c.a, c.b, c.ordered, got, c.want)
		}
	}
}

// TestArrayLength checks OpArrayLength (arrayLength in WGSL) on a runtime
// array after a struct header: the element count the binding's size leaves.
// It read no value before, which naga's Restrict index clamps depend on.
func TestArrayLength(t *testing.T) {
	const wgsl = `
struct Data { header: vec4<u32>, items: array<u32> }
@group(0) @binding(0) var<storage, read> src: Data;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

@compute @workgroup_size(1)
fn main() {
    dst[0] = arrayLength(&src.items);
}
`
	got := runNagaCompute(t, wgsl, make([]uint32, 4+6), 1)
	if got[0] != 6 {
		t.Errorf("arrayLength = %d, want 6", got[0])
	}
}

// TestBitcastTagsResultByType checks that OpBitcast reinterprets the bits
// and tags the result by its type: bitcast<u32>(f) stored to a u32 keeps
// the f32 bits, bitcast<f32>(u) takes part in float arithmetic, and a
// bitcast vector or a lane of an integer vector keeps every bit.
func TestBitcastTagsResultByType(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var<storage, read_write> f: array<f32>;
@group(0) @binding(1) var<storage, read_write> u: array<u32>;
@group(0) @binding(2) var<storage, read_write> uv: array<vec2<u32>>;
@group(0) @binding(3) var<storage, read_write> fv: array<vec2<f32>>;
@compute @workgroup_size(1)
fn main() {
    u[0] = bitcast<u32>(f[0]);
    f[1] = bitcast<f32>(u[1]) + 1.0;
    uv[0] = bitcast<vec2<u32>>(vec2<f32>(f[2], f[3]));
    let iv = vec2<i32>(i32(u[2]), 7);
    u[3] = bitcast<u32>(iv.x);
    fv[0] = bitcast<vec2<f32>>(vec2<u32>(u[4], u[5]));
}
`
	f := []uint32{math.Float32bits(1.5), 0, 0x80000000, math.Float32bits(1)}
	u := []uint32{0, math.Float32bits(2), 0xFFFFFFFB, 0, 1, 0x00800000}
	uv := make([]uint32, 2)
	fv := make([]uint32, 2)
	dispatchNagaBuffers(t, wgsl, f, u, uv, fv)

	checkWords(t, "u", u, []uint32{0x3FC00000, math.Float32bits(2), 0xFFFFFFFB, 0xFFFFFFFB, 1, 0x00800000})
	checkWords(t, "f", f, []uint32{math.Float32bits(1.5), math.Float32bits(3), 0x80000000, math.Float32bits(1)})
	checkWords(t, "uv", uv, []uint32{0x80000000, 0x3F800000})
	checkWords(t, "fv", fv, []uint32{1, 0x00800000})
}

// TestConvertVectorsComponentWise checks that OpConvertFToS, OpConvertFToU,
// OpConvertSToF and OpConvertUToF convert every component of a vector (they
// collapsed it to one scalar 0), and that float to integer follows WGSL:
// truncate toward zero, saturating at the nearest value float32 can hold
// (wgsl/index.bs "Floating point conversion": 1e10 -> 0xFFFFFF00u).
func TestConvertVectorsComponentWise(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var<storage, read_write> f: array<f32>;
@group(0) @binding(1) var<storage, read_write> o: array<i32>;
@group(0) @binding(2) var<storage, read_write> fo: array<f32>;
@compute @workgroup_size(1)
fn main() {
    let iv = vec2<i32>(vec2<f32>(f[0], f[1]));
    o[0] = iv.x;
    o[1] = iv.y;
    let uv = vec2<u32>(vec2<f32>(f[2], f[3]));
    o[2] = bitcast<i32>(uv.x);
    o[3] = bitcast<i32>(uv.y);
    let sat = vec2<i32>(vec2<f32>(f[3], -f[3]));
    o[4] = sat.x;
    o[5] = sat.y;
    let back = vec2<f32>(iv);
    fo[0] = back.x;
    fo[1] = back.y;
    let ub = vec2<f32>(vec2<u32>(u32(f[4]), 9u));
    fo[2] = ub.x;
    fo[3] = ub.y;
}
`
	f := []uint32{math.Float32bits(3.75), math.Float32bits(-2.5), math.Float32bits(7.9), math.Float32bits(1e10), math.Float32bits(5)}
	o := make([]uint32, 6)
	fo := make([]uint32, 4)
	dispatchNagaBuffers(t, wgsl, f, o, fo)

	neg2, minI32 := int32(-2), int32(math.MinInt32)
	checkWords(t, "o", o, []uint32{3, uint32(neg2), 7, 0xFFFFFF00, 0x7FFFFF80, uint32(minI32)})
	checkWords(t, "fo", fo, []uint32{math.Float32bits(3), math.Float32bits(-2), math.Float32bits(5), math.Float32bits(9)})
}
