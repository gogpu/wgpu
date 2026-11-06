// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"math"
	"testing"
)

// Tests for the numeric instructions ADR-078 step 6 executes, each compiled
// from WGSL by naga: OpVectorTimesMatrix, OpIsNan, OpIsInf, OpQuantizeToF16,
// OpSDotKHR and OpUDotKHR. Before step 6 the interpreter returned zero for
// all of them.

func TestVectorTimesMatrix(t *testing.T) {
	checkTypedCases(t, []typedCase{
		{
			// vec3 * mat2x3 is a row vector times two columns: (v·c0, v·c1).
			name: "vec3_times_mat2x3",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let m = mat2x3<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]), f32(src[4]), f32(src[5]));
	let v = vec3<f32>(f32(src[6]), f32(src[7]), f32(src[8]));
	let r = v * m;
	dst[0] = u32(r.x); dst[1] = u32(r.y);
}`,
			src: []uint32{1, 2, 3, 4, 5, 6, 1, 1, 2}, dstLen: 2, want: []uint32{9, 21},
		},
		{
			name: "vec2_times_mat3x2",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let m = mat3x2<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]), f32(src[4]), f32(src[5]));
	let v = vec2<f32>(f32(src[6]), f32(src[7]));
	let r = v * m;
	dst[0] = u32(r.x); dst[1] = u32(r.y); dst[2] = u32(r.z);
}`,
			src: []uint32{1, 2, 3, 4, 5, 6, 1, 2}, dstLen: 3, want: []uint32{5, 11, 17},
		},
		{
			// A constant matrix: r[i] is v dotted with column i.
			name: "vec4_times_const_mat4x4",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let m = mat4x4<f32>(
		1.0, 2.0, 3.0, 4.0,
		5.0, 6.0, 7.0, 8.0,
		9.0, 10.0, 11.0, 12.0,
		13.0, 14.0, 15.0, 16.0);
	let v = vec4<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]));
	let r = v * m;
	dst[0] = u32(r.x); dst[1] = u32(r.y); dst[2] = u32(r.z); dst[3] = u32(r.w);
}`,
			src: []uint32{1, 0, 2, 1}, dstLen: 4, want: []uint32{11, 27, 43, 59},
		},
	})
}

func TestFloatClassification(t *testing.T) {
	nan, inf, ninf, one := uint32(0x7FC00000), uint32(0x7F800000), uint32(0xFF800000), f32bits(1)
	checkTypedCases(t, []typedCase{
		{
			// Bit 0 is isnan, bit 1 is isinf, lane by lane.
			name: "vec4",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let v = bitcast<vec4<f32>>(vec4<u32>(src[0], src[1], src[2], src[3]));
	let n = isnan(v);
	let i = isinf(v);
	let r = select(vec4<u32>(0u), vec4<u32>(1u), n) | select(vec4<u32>(0u), vec4<u32>(2u), i);
	dst[0] = r.x; dst[1] = r.y; dst[2] = r.z; dst[3] = r.w;
}`,
			src: []uint32{nan, inf, ninf, one}, dstLen: 4, want: []uint32{1, 2, 2, 0},
		},
		{
			name: "scalar",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = bitcast<f32>(src[0]);
	let b = bitcast<f32>(src[1]);
	dst[0] = select(0u, 1u, isnan(a)); dst[1] = select(0u, 1u, isinf(a));
	dst[2] = select(0u, 1u, isnan(b)); dst[3] = select(0u, 1u, isinf(b));
}`,
			src: []uint32{nan, one}, dstLen: 4, want: []uint32{1, 0, 0, 0},
		},
	})
}

func TestQuantizeToF16(t *testing.T) {
	src := []uint32{
		f32bits(1),   // exact
		0x3F801000,   // 1 + 2^-11: a tie, rounds to the even 1
		0x3F803000,   // 1 + 3*2^-11: a tie, rounds to the even 1 + 2^-9
		0x477FE000,   // 65504, the largest finite f16
		0x477FF000,   // 65520 rounds to 65536: out of range, +inf
		0xC77FF000,   // -65520: -inf
		0x80000000,   // -0 stays -0
		0x38000000,   // 2^-15 is below the smallest normal f16: +0
		0xB8000000,   // -2^-15: -0
		0x38800000,   // 2^-14, the smallest normal f16, is kept
		0x7F800000,   // +inf
		f32bits(0.1), // 0.1 to f16 is 0x2E66, back to f32 0x3DCCC000
	}
	want := []uint32{
		f32bits(1), 0x3F800000, 0x3F804000, 0x477FE000, 0x7F800000, 0xFF800000,
		0x80000000, 0x00000000, 0x80000000, 0x38800000, 0x7F800000, 0x3DCCC000,
	}
	checkTypedCases(t, []typedCase{{
		name: "scalar",
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	for (var i = 0u; i < arrayLength(&src); i++) {
		dst[i] = bitcast<u32>(quantizeToF16(bitcast<f32>(src[i])));
	}
}`,
		src: src, dstLen: len(src), want: want,
	}, {
		name: "vec2",
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let q = quantizeToF16(bitcast<vec2<f32>>(vec2<u32>(src[0], src[1])));
	dst[0] = bitcast<u32>(q.x); dst[1] = bitcast<u32>(q.y);
}`,
		src: []uint32{0x3F803000, 0x477FF000}, dstLen: 2, want: []uint32{0x3F804000, 0x7F800000},
	}})
}

// TestQuantizeToF16NaN checks that a NaN stays a NaN (its payload is not
// specified).
func TestQuantizeToF16NaN(t *testing.T) {
	for _, bits := range []uint32{0x7FC00000, 0xFFC00001, 0x7F800001} {
		if got := quantizeToF16(math.Float32frombits(bits)); !math.IsNaN(float64(got)) {
			t.Errorf("quantizeToF16(%#08x) = %v, want NaN", bits, got)
		}
	}
}

func TestPackedIntegerDot(t *testing.T) {
	// a holds the bytes 1, -2, 3, -128 and b the bytes 5, 6, -7, -1, low
	// byte first. Signed: 5 - 12 - 21 + 128 = 100. Unsigned: 1*5 + 254*6 +
	// 3*249 + 128*255 = 34916. All 0xFF bytes: 4 signed, 4*255*255 unsigned.
	checkTypedCases(t, []typedCase{{
		name: "dot4_packed",
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	dst[0] = bitcast<u32>(dot4I8Packed(src[0], src[1]));
	dst[1] = dot4U8Packed(src[0], src[1]);
	dst[2] = bitcast<u32>(dot4I8Packed(src[2], src[2]));
	dst[3] = dot4U8Packed(src[2], src[2]);
}`,
		src: []uint32{0x8003FE01, 0xFFF90605, 0xFFFFFFFF}, dstLen: 4, want: []uint32{100, 34916, 4, 260100},
	}, {
		// Integer dot products of vectors wrap modulo 2^32.
		name: "integer_vector_dot_wraps",
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = bitcast<vec2<i32>>(vec2<u32>(src[0], src[1]));
	let b = bitcast<vec2<i32>>(vec2<u32>(src[2], src[3]));
	dst[0] = bitcast<u32>(dot(a, b));
	dst[1] = dot(vec2<u32>(src[4], src[5]), vec2<u32>(src[6], src[7]));
}`,
		src:    []uint32{0x40000000, 3, 4, 0xFFFFFFFB, 0x80000000, 1, 2, 7},
		dstLen: 2, want: []uint32{0xFFFFFFF1, 7},
	}})
}
