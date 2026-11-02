// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

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

// TestIntLaneOps checks that the bit ops apply per lane of an integer vector
// in either form, keeping signed scalars signed.
func TestIntLaneOps(t *testing.T) {
	if got := mapIntLanes(ValVec2(7, 255), false, bitCount); got.Tag != TagVec2 || got.F[0] != 3 || got.F[1] != 8 {
		t.Errorf("countOneBits(vec2(7, 255)) = %v %v, want (3, 8)", got.Tag, got.F)
	}
	arr := ValArray([]Value{ValUint(1), ValUint(0x80000000)})
	got := mapIntLanes(arr, false, bitCount)
	if a := got.AsArray(); len(a) != 2 || a[0].U[0] != 1 || a[1].U[0] != 1 {
		t.Errorf("countOneBits(array) = %v", got)
	}
	if got := mapIntLanes(ValInt(-1), true, bitCount); got.Tag != TagInt32 || got.U[0] != 32 {
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

// TestLaneBitsSaturates checks the conversion of a float-held integer lane
// back to its bits, saturated where float32 rounds past the type's range.
func TestLaneBitsSaturates(t *testing.T) {
	tests := []struct {
		f      float32
		signed bool
		want   uint32
	}{
		{0xA1, false, 0xA1},
		{1 << 24, false, 1 << 24},
		{math.MaxUint32, false, math.MaxUint32}, // float32 rounds it to 2^32
		{-3, false, 0},
		{-5, true, 0xFFFFFFFB},
		{math.MaxInt32, true, math.MaxInt32}, // float32 rounds it to 2^31
		{math.MinInt32, true, 0x80000000},
		{float32(math.NaN()), false, 0},
	}
	for _, tc := range tests {
		if got := laneBits(tc.f, tc.signed); got != tc.want {
			t.Errorf("laneBits(%v, signed=%v) = %#x, want %#x", tc.f, tc.signed, got, tc.want)
		}
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
