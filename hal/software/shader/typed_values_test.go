// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/spirv"
)

// Tests for ADR-078: integer and boolean vectors hold their exact lanes.
//
// Rows with a wrongToday output document a defect of the float-lane value
// model and the ADR-078 migration step that fixes it. expectCurrent passes
// while the interpreter still gives that wrong output and fails the moment
// it gives the right one, so the step that fixes a row must flip it to a
// plain assertion. Rows without wrongToday are right today and must stay
// right through every step.

// compileSoftwareSPIRV compiles wgsl with the SPIR-V options the software
// backend uses (hal/software spirvOptions: SPIR-V 1.3, Restrict index,
// buffer and image load checks, Unchecked binding arrays) and parses it.
// The options are repeated here because this package cannot import
// hal/software.
func compileSoftwareSPIRV(t testing.TB, wgsl string) *Module {
	t.Helper()
	spirvBytes, err := naga.CompileWithOptions(wgsl, naga.CompileOptions{
		SPIRV: spirv.Options{
			Version: spirv.Version1_3,
			BoundsCheckPolicies: backend.BoundsCheckPolicies{
				Index:        backend.BoundsCheckRestrict,
				Buffer:       backend.BoundsCheckRestrict,
				ImageLoad:    backend.BoundsCheckRestrict,
				BindingArray: backend.BoundsCheckUnchecked,
			},
		},
	})
	if err != nil {
		t.Fatalf("naga.CompileWithOptions: %v", err)
	}
	words := make([]uint32, len(spirvBytes)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
	}
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
	return m
}

// expectCurrent checks a row that is wrong today: it passes while got equals
// the measured wrong output and fails once got equals want ("fixed early")
// or anything else.
func expectCurrent(t *testing.T, got, wrongToday, want []uint32, step int) {
	t.Helper()
	switch {
	case slices.Equal(got, wrongToday):
	case slices.Equal(got, want):
		t.Errorf("fixed early: flip to want %v (step %d)", want, step)
	default:
		t.Errorf("got %v, neither today's wrong output %v nor want %v (step %d)", got, wrongToday, want, step)
	}
}

// typedCase is one compute shader: src at @binding(0), dst at @binding(1),
// an optional texture at @binding(2) and sampler at @binding(3).
type typedCase struct {
	name       string
	wgsl       string
	src        []uint32
	dstLen     int
	want       []uint32
	wrongToday []uint32 // the output today; nil when today is already right
	step       int      // the ADR-078 migration step that fixes the row
	tex        *Texture2D
	smp        *Sampler
}

func wordsToBytes(words []uint32) []byte {
	raw := make([]byte, 4*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint32(raw[i*4:], w)
	}
	return raw
}

func bytesToWords(raw []byte) []uint32 {
	words := make([]uint32, len(raw)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(raw[i*4:])
	}
	return words
}

// typedContext binds the case's resources.
func typedContext(c *typedCase) (*ExecutionContext, []byte) {
	src := c.src
	if len(src) == 0 {
		src = []uint32{0}
	}
	dst := make([]byte, 4*c.dstLen)
	ctx := &ExecutionContext{Buffers: map[BindingKey][]byte{
		{Group: 0, Binding: 0}: wordsToBytes(src),
		{Group: 0, Binding: 1}: dst,
	}}
	if c.tex != nil {
		ctx.Textures = map[BindingKey]*Texture2D{{Group: 0, Binding: 2}: c.tex}
	}
	if c.smp != nil {
		ctx.Samplers = map[BindingKey]*Sampler{{Group: 0, Binding: 3}: c.smp}
	}
	return ctx, dst
}

func runTypedCase(t *testing.T, c *typedCase) []uint32 {
	t.Helper()
	m := compileSoftwareSPIRV(t, c.wgsl)
	ctx, dst := typedContext(c)
	if err := m.DispatchCompute("main", ctx, 1, 1, 1); err != nil {
		t.Fatalf("DispatchCompute: %v", err)
	}
	return bytesToWords(dst)
}

func checkTypedCases(t *testing.T, cases []typedCase) {
	t.Helper()
	for i := range cases {
		c := &cases[i]
		t.Run(c.name, func(t *testing.T) {
			got := runTypedCase(t, c)
			if c.wrongToday != nil {
				expectCurrent(t, got, c.wrongToday, c.want, c.step)
				return
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

const typedHeader = `
@group(0) @binding(0) var<storage, read> src: array<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;
`

const typedHeaderTex = typedHeader + `
@group(0) @binding(2) var tex: texture_2d<f32>;
`

const typedHeaderSampled = typedHeaderTex + `
@group(0) @binding(3) var smp: sampler;
`

// gridTexture is an RGBA8 texture whose texel (x, y) is (10x, 10y, x+y, 255).
func gridTexture(w, h uint32) *Texture2D {
	data := make([]byte, 0, 4*w*h)
	for y := range h {
		for x := range w {
			data = append(data, byte(10*x), byte(10*y), byte(x+y), 255)
		}
	}
	return &Texture2D{Width: w, Height: h, Data: data}
}

func f32bits(f float32) uint32 { return f32w(f) }

// TestTypedValuesWrongToday holds the defects of ADR-078's Context section,
// measured on the float-lane interpreter.
func TestTypedValuesWrongToday(t *testing.T) {
	checkTypedCases(t, []typedCase{
		{
			name: "uvec2_add_runtime",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let b = vec2<u32>(src[2], src[3]);
	let c = a + b;
	dst[0] = c.x; dst[1] = c.y;
}`,
			src: []uint32{5, 9, 10, 18}, dstLen: 2, want: []uint32{15, 27}, step: 3,
		},
		{
			name: "uvec2_times_const",
			wgsl: typedHeader + `const K = vec2<u32>(3u, 4u);
@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let c = a * K;
	dst[0] = c.x; dst[1] = c.y;
}`,
			src: []uint32{5, 7}, dstLen: 2, want: []uint32{15, 28}, step: 3,
		},
		{
			name: "ivec2_div",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<i32>(i32(src[0]), i32(src[1]));
	let b = vec2<i32>(i32(src[2]), i32(src[3]));
	let c = a / b;
	dst[0] = bitcast<u32>(c.x); dst[1] = bitcast<u32>(c.y);
}`,
			src: []uint32{10, 21, 2, 3}, dstLen: 2, want: []uint32{5, 7}, step: 4,
		},
		{
			name: "uvec2_equal_bvec2",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let b = vec2<u32>(src[2], src[3]);
	let e = a == b;
	dst[0] = select(0u, 1u, e.x); dst[1] = select(0u, 1u, e.y);
}`,
			src: []uint32{1, 2, 1, 3}, dstLen: 2, want: []uint32{1, 0}, step: 4,
		},
		{
			name: "fvec2_less_bvec2",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<f32>(f32(src[0]), f32(src[1]));
	let b = vec2<f32>(f32(src[2]), f32(src[3]));
	let l = a < b;
	dst[0] = select(0u, 1u, l.x); dst[1] = select(0u, 1u, l.y);
}`,
			src: []uint32{1, 5, 2, 3}, dstLen: 2, want: []uint32{1, 0}, step: 4,
		},
		{
			name: "const_vec4_times_vec4",
			wgsl: typedHeader + `const K = vec4<f32>(1.0, 2.0, 3.0, 4.0);
@compute @workgroup_size(1) fn main() {
	let v = vec4<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]));
	let p = K * v;
	let q = v * K;
	dst[0] = u32(p.x); dst[1] = u32(p.y); dst[2] = u32(p.z); dst[3] = u32(p.w);
	dst[4] = u32(q.x); dst[5] = u32(q.y); dst[6] = u32(q.z); dst[7] = u32(q.w);
}`,
			src: []uint32{2, 3, 4, 5}, dstLen: 8, want: []uint32{2, 6, 12, 20, 2, 6, 12, 20}, step: 3,
		},
		{
			name: "vec_times_matrix",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let m = mat2x2<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]));
	let v = vec2<f32>(f32(src[4]), f32(src[5]));
	let r = v * m;
	dst[0] = u32(r.x); dst[1] = u32(r.y);
}`,
			src: []uint32{1, 0, 0, 1, 2, 3}, dstLen: 2, want: []uint32{2, 3}, step: 6,
		},
		{
			name: "uvec2_swizzle_add",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let s = a.yx + a;
	dst[0] = s.x; dst[1] = s.y;
}`,
			src: []uint32{5, 7}, dstLen: 2, want: []uint32{12, 12}, step: 3,
		},
		{
			name: "u32_above_2p24_through_vector",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let v = vec2<u32>(src[0], 1u);
	dst[0] = v.x; dst[1] = v.y;
}`,
			src: []uint32{1<<24 + 1}, dstLen: 2, want: []uint32{1<<24 + 1, 1}, step: 3,
		},
		{
			name: "bitcast_vec2_into_construct",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let f = vec2<f32>(f32(src[0]), f32(src[1]));
	let v = vec4<u32>(bitcast<vec2<u32>>(f), 5u, 7u);
	dst[0] = v.x; dst[1] = v.y; dst[2] = v.z; dst[3] = v.w;
}`,
			src: []uint32{1, 2}, dstLen: 4, want: []uint32{f32bits(1), f32bits(2), 5, 7}, step: 3,
		},
		{
			name: "all",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let b = vec2<u32>(src[2], src[3]);
	dst[0] = select(0u, 1u, all(a == b));
	dst[1] = select(0u, 1u, all(a == a.yx));
}`,
			src: []uint32{1, 2, 1, 2}, dstLen: 2, want: []uint32{1, 0}, step: 7,
		},
		{
			name: "any",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	dst[0] = select(0u, 1u, any(a == vec2<u32>(src[2], src[1])));
	dst[1] = select(0u, 1u, any(a == vec2<u32>(src[2], src[2])));
}`,
			src: []uint32{1, 2, 3}, dstLen: 2, want: []uint32{1, 0}, step: 7,
		},
		{
			name: "texture_dimensions_minus_one",
			wgsl: typedHeaderTex + `@compute @workgroup_size(1) fn main() {
	let d = textureDimensions(tex) - vec2<u32>(1u);
	dst[0] = d.x; dst[1] = d.y;
}`,
			dstLen: 2, want: []uint32{6, 8}, step: 3, tex: gridTexture(7, 9),
		},
		{
			name: "module_const_array_of_vec4_dynamic_index",
			wgsl: typedHeader + `const TABLE = array<vec4<f32>, 3>(
	vec4<f32>(1.0, 2.0, 3.0, 4.0),
	vec4<f32>(3.0, -1.0, 0.0, 1.0),
	vec4<f32>(5.0, 6.0, 7.0, 8.0));
@compute @workgroup_size(1) fn main() {
	let v = TABLE[src[0]];
	dst[0] = bitcast<u32>(i32(v.x)); dst[1] = bitcast<u32>(i32(v.y));
	dst[2] = bitcast<u32>(i32(v.z)); dst[3] = bitcast<u32>(i32(v.w));
}`,
			src: []uint32{1}, dstLen: 4, want: []uint32{3, 0xFFFFFFFF, 0, 1}, step: 3,
		},
		{
			name: "uniform_uvec4_above_2p24",
			wgsl: `@group(0) @binding(0) var<uniform> u: vec4<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;
@compute @workgroup_size(1) fn main() {
	let v = u;
	dst[0] = v.x; dst[1] = v.y; dst[2] = v.z; dst[3] = v.w;
}`,
			src:    []uint32{1<<24 + 1, 0xFFFFFFFF, 0x80000001, 3},
			dstLen: 4, want: []uint32{1<<24 + 1, 0xFFFFFFFF, 0x80000001, 3}, step: 3,
		},
	})
}

// TestTypedValuesAtomics covers the atomic read-modify-write builtins
// (OpAtomicAnd, OpAtomicOr, OpAtomicXor since step 6).
func TestTypedValuesAtomics(t *testing.T) {
	checkTypedCases(t, []typedCase{{
		name: "atomic_and_or_xor",
		wgsl: `@group(0) @binding(0) var<storage, read> src: array<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<atomic<u32>>;
@compute @workgroup_size(1) fn main() {
	atomicStore(&dst[0], src[0]);
	atomicStore(&dst[1], src[0]);
	atomicStore(&dst[2], src[0]);
	atomicOr(&dst[0], src[1]);
	atomicAnd(&dst[1], src[1]);
	atomicXor(&dst[2], src[1]);
	atomicAdd(&dst[3], src[2]);
}`,
		src: []uint32{1, 6, 0x80000001}, dstLen: 4,
		want: []uint32{7, 0, 7, 0x80000001}, step: 6,
	}})
}

// TestTypedValuesRightToday holds what the float-lane interpreter computes
// right; every migration step must keep it right.
func TestTypedValuesRightToday(t *testing.T) {
	checkTypedCases(t, []typedCase{
		{
			name: "fvec4_mul",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec4<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]));
	let c = a * vec4<f32>(f32(src[4]));
	dst[0] = u32(c.x); dst[1] = u32(c.y); dst[2] = u32(c.z); dst[3] = u32(c.w);
}`,
			src: []uint32{1, 2, 3, 4, 3}, dstLen: 4, want: []uint32{3, 6, 9, 12},
		},
		{
			name: "loop_10",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var s = 0u;
	for (var i = 0u; i < src[0]; i++) { s += 1u; }
	dst[0] = s;
}`,
			src: []uint32{10}, dstLen: 1, want: []uint32{10},
		},
		{
			name: "loop_70000",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var s = 0u;
	for (var i = 0u; i < src[0]; i++) { s += 1u; }
	dst[0] = s;
}`,
			src: []uint32{70000}, dstLen: 1, want: []uint32{70000},
		},
		{
			name: "vector_component_store",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var v = vec4<u32>(src[0], src[1], src[2], src[3]);
	v.y = 100u;
	v[src[4]] = 50u;
	dst[0] = v.x; dst[1] = v.y; dst[2] = v.z; dst[3] = v.w;
}`,
			src: []uint32{1, 2, 3, 4, 3}, dstLen: 4, want: []uint32{1, 100, 3, 50},
		},
		{
			name: "bitcast_scalar",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	dst[0] = bitcast<u32>(f32(src[0]));
	dst[1] = bitcast<u32>(-i32(src[0]));
}`,
			src: []uint32{1}, dstLen: 2, want: []uint32{f32bits(1), 0xFFFFFFFF},
		},
		{
			name: "array_u32_sum",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var arr = array<u32, 2>(src[0], src[1]);
	dst[0] = arr[0] + arr[1];
}`,
			src: []uint32{0x80000000, 0x7FFFFFFF}, dstLen: 1, want: []uint32{0xFFFFFFFF},
		},
		{
			name: "texture_dimensions",
			wgsl: typedHeaderTex + `@compute @workgroup_size(1) fn main() {
	let d = textureDimensions(tex);
	dst[0] = d.x; dst[1] = d.y;
}`,
			dstLen: 2, want: []uint32{7, 9}, tex: gridTexture(7, 9),
		},
		{
			name: "workgroup_zero_init",
			wgsl: typedHeader + `var<workgroup> w: array<u32, 4>;
@compute @workgroup_size(1) fn main() {
	dst[0] = w[0] + w[3] + src[0];
}`,
			src: []uint32{0}, dstLen: 1, want: []uint32{0},
		},
	})
}

// TestTypedValuesExact covers the lane-wise integer, boolean and comparison
// operations ADR-078 types, with values a float lane cannot hold.
func TestTypedValuesExact(t *testing.T) {
	checkTypedCases(t, []typedCase{
		{
			name: "uvec2_max_u32",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let m = max(vec2<u32>(src[0], src[1]), vec2<u32>(src[2], src[3]));
	dst[0] = m.x; dst[1] = m.y;
}`,
			src: []uint32{0xFFFFFFFF, 1, 2, 0xFFFFFFFE}, dstLen: 2, want: []uint32{0xFFFFFFFF, 0xFFFFFFFE},
			step: 4,
		},
		{
			name: "uvec2_sub_mul_shift",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let b = vec2<u32>(src[2], src[3]);
	let d = a - b;
	let m = a * b;
	let l = a << b;
	let r = a >> b;
	dst[0] = d.x; dst[1] = d.y; dst[2] = m.x; dst[3] = m.y;
	dst[4] = l.x; dst[5] = l.y; dst[6] = r.x; dst[7] = r.y;
}`,
			src:    []uint32{0x80000001, 3, 2, 1},
			dstLen: 8,
			want:   []uint32{0x7FFFFFFF, 2, 2, 3, 4, 6, 0x20000000, 1},
			step:   4,
		},
		{
			name: "bvec2_construct",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let b = vec2<bool>(src[0] > 1u, src[1] > 1u);
	dst[0] = select(0u, 1u, b.x); dst[1] = select(0u, 1u, b.y);
}`,
			src: []uint32{2, 0}, dstLen: 2, want: []uint32{1, 0}, step: 3,
		},
		{
			name: "select_bvec3",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let r = select(vec3<u32>(src[0], src[1], src[2]), vec3<u32>(src[3], src[4], src[5]),
		vec3<bool>(src[6] == 1u, src[7] == 1u, src[8] == 1u));
	dst[0] = r.x; dst[1] = r.y; dst[2] = r.z;
}`,
			src:    []uint32{1, 1<<24 + 1, 3, 0xFFFFFFFF, 5, 0x80000001, 1, 0, 1},
			dstLen: 3, want: []uint32{0xFFFFFFFF, 1<<24 + 1, 0x80000001}, step: 3,
		},
		{
			name: "fvec4_comparisons",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec4<f32>(f32(src[0]), f32(src[1]), f32(src[2]), f32(src[3]));
	let b = vec4<f32>(f32(src[4]));
	let one = vec4<u32>(1u);
	let zero = vec4<u32>(0u);
	let le = select(zero, one, a <= b);
	let gt = select(zero, one, a > b);
	let ge = select(zero, one, a >= b);
	let eq = select(zero, one, a == b);
	let ne = select(zero, one, a != b);
	dst[0] = le.x; dst[1] = le.y; dst[2] = le.z; dst[3] = le.w;
	dst[4] = gt.x; dst[5] = gt.y; dst[6] = gt.z; dst[7] = gt.w;
	dst[8] = ge.x; dst[9] = ge.y; dst[10] = ge.z; dst[11] = ge.w;
	dst[12] = eq.x; dst[13] = eq.y; dst[14] = eq.z; dst[15] = eq.w;
	dst[16] = ne.x; dst[17] = ne.y; dst[18] = ne.z; dst[19] = ne.w;
}`,
			src:    []uint32{1, 2, 3, 4, 2},
			dstLen: 20,
			want: []uint32{
				1, 1, 0, 0,
				0, 0, 1, 1,
				0, 1, 1, 1,
				0, 1, 0, 0,
				1, 0, 1, 1,
			},
			step: 4,
		},
		{
			// WGSL §8.7 (index.bs "division", "Remainder"): x / 0 = x,
			// INT_MIN / -1 = INT_MIN, x % 0 = 0, INT_MIN % -1 = 0; naga emits
			// the guards as helper functions.
			name: "i32_div_rem_scalar",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	for (var i = 0u; i < 4u; i++) {
		let a = i32(src[i]);
		let b = i32(src[4u + i]);
		dst[i] = bitcast<u32>(a / b);
		dst[4u + i] = bitcast<u32>(a % b);
	}
}`,
			src:    []uint32{7, 0x80000000, 0xFFFFFFF9, 9, 0, 0xFFFFFFFF, 2, 4},
			dstLen: 8,
			want:   []uint32{7, 0x80000000, 0xFFFFFFFD, 2, 0, 0, 0xFFFFFFFF, 1},
		},
		{
			name: "i32_div_rem_vector",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec4<i32>(i32(src[0]), i32(src[1]), i32(src[2]), i32(src[3]));
	let b = vec4<i32>(i32(src[4]), i32(src[5]), i32(src[6]), i32(src[7]));
	let q = bitcast<vec4<u32>>(a / b);
	let r = bitcast<vec4<u32>>(a % b);
	dst[0] = q.x; dst[1] = q.y; dst[2] = q.z; dst[3] = q.w;
	dst[4] = r.x; dst[5] = r.y; dst[6] = r.z; dst[7] = r.w;
}`,
			src:    []uint32{7, 0x80000000, 0xFFFFFFF9, 9, 0, 0xFFFFFFFF, 2, 4},
			dstLen: 8,
			want:   []uint32{7, 0x80000000, 0xFFFFFFFD, 2, 0, 0, 0xFFFFFFFF, 1},
			step:   4,
		},
		{
			name: "u32_div_rem_by_zero",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let a = vec2<u32>(src[0], src[1]);
	let b = vec2<u32>(src[2], src[3]);
	let q = a / b;
	let r = a % b;
	dst[0] = q.x; dst[1] = q.y; dst[2] = r.x; dst[3] = r.y;
	dst[4] = src[0] / src[2]; dst[5] = src[0] % src[2];
}`,
			src:    []uint32{0xFFFFFFFF, 10, 0, 3},
			dstLen: 6,
			want:   []uint32{0xFFFFFFFF, 3, 0, 1, 0xFFFFFFFF, 0},
			step:   4,
		},
	})
}

// TestTypedValuesGGControls runs WGSL taken from gg's shaders (gg is MIT;
// files and lines at gg commit 974122e) through compute, as controls of
// float-vector code that must not change.
func TestTypedValuesGGControls(t *testing.T) {
	checkTypedCases(t, []typedCase{
		{
			// internal/gpu/shaders/glyph_mask.wgsl:105-109, apply_mask_gamma.
			name: "glyph_mask_apply_mask_gamma",
			wgsl: typedHeader + `const MASK_GAMMA_MAX_CONTRAST: f32 = 0.5;
fn apply_mask_gamma(alpha: f32, text_color: vec3<f32>) -> f32 {
	let luminance = dot(text_color, vec3<f32>(0.299, 0.587, 0.114));
	let contrast = luminance * MASK_GAMMA_MAX_CONTRAST;
	return alpha + (1.0 - alpha) * contrast * alpha;
}
@compute @workgroup_size(1) fn main() {
	let c = vec3<f32>(f32(src[0]), f32(src[1]), f32(src[2])) / 255.0;
	let a = apply_mask_gamma(f32(src[3]) / 100.0, c);
	dst[0] = u32(a * 1000.0 + 0.5);
}`,
			src: []uint32{255, 255, 255, 50}, dstLen: 1, want: []uint32{625},
		},
		{
			// internal/gpu/shaders/msdf_text.wgsl:161, the screen texture size;
			// fwidth(in.tex_coord) is replaced by a runtime vec2 in compute.
			name: "msdf_screen_tex_size",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	let w = vec2<f32>(1.0 / f32(src[0]), 1.0 / f32(src[1]));
	let screen_tex_size = vec2<f32>(1.0, 1.0) / w;
	dst[0] = u32(screen_tex_size.x + 0.5); dst[1] = u32(screen_tex_size.y + 0.5);
}`,
			src: []uint32{4, 2}, dstLen: 2, want: []uint32{4, 2},
		},
		{
			// internal/gpu/shaders/blit.wgsl:20-35 (also composite.wgsl:71-84,
			// blend.wgsl:61-74): function-local arrays of vec2 indexed by the
			// vertex index.
			name: "blit_fullscreen_triangle_arrays",
			wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var positions = array<vec2<f32>, 3>(
		vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	var uvs = array<vec2<f32>, 3>(
		vec2<f32>(0.0, 1.0), vec2<f32>(2.0, 1.0), vec2<f32>(0.0, -1.0));
	for (var k = 0u; k < 3u; k++) {
		let idx = src[k];
		let p = positions[idx];
		let uv = uvs[idx];
		dst[4u * k + 0u] = bitcast<u32>(i32(p.x));
		dst[4u * k + 1u] = bitcast<u32>(i32(p.y));
		dst[4u * k + 2u] = bitcast<u32>(i32(uv.x));
		dst[4u * k + 3u] = bitcast<u32>(i32(uv.y));
	}
}`,
			src:    []uint32{0, 1, 2},
			dstLen: 12,
			want: []uint32{
				0xFFFFFFFF, 0xFFFFFFFF, 0, 1,
				3, 0xFFFFFFFF, 2, 1,
				0xFFFFFFFF, 3, 0, 0xFFFFFFFF,
			},
		},
	})
}

// TestTypedValuesImages covers image operands built from integer vectors.
func TestTypedValuesImages(t *testing.T) {
	clamp := &Sampler{WrapU: WrapClampToEdge, WrapV: WrapClampToEdge, LodMaxClamp: LodClampNone}
	checkTypedCases(t, []typedCase{
		{
			name: "texture_load_ivec2_runtime",
			wgsl: typedHeaderTex + `@compute @workgroup_size(1) fn main() {
	let t = textureLoad(tex, vec2<i32>(i32(src[0]), i32(src[1])), 0);
	dst[0] = u32(t.x * 255.0 + 0.5); dst[1] = u32(t.y * 255.0 + 0.5); dst[2] = u32(t.z * 255.0 + 0.5);
}`,
			src: []uint32{3, 5}, dstLen: 3, want: []uint32{30, 50, 8}, tex: gridTexture(7, 9),
		},
		{
			name: "texture_load_uvec2_runtime",
			wgsl: typedHeaderTex + `@compute @workgroup_size(1) fn main() {
	let t = textureLoad(tex, vec2<u32>(src[0], src[1]), 0);
	dst[0] = u32(t.x * 255.0 + 0.5); dst[1] = u32(t.y * 255.0 + 0.5); dst[2] = u32(t.z * 255.0 + 0.5);
}`,
			src: []uint32{6, 8}, dstLen: 3, want: []uint32{60, 80, 14}, tex: gridTexture(7, 9),
		},
		{
			name: "texture_sample_level_const_offset",
			wgsl: typedHeaderSampled + `@compute @workgroup_size(1) fn main() {
	let uv = vec2<f32>((f32(src[0]) + 0.5) / 7.0, (f32(src[1]) + 0.5) / 9.0);
	let t = textureSampleLevel(tex, smp, uv, 0.0, vec2<i32>(1, -1));
	dst[0] = u32(t.x * 255.0 + 0.5); dst[1] = u32(t.y * 255.0 + 0.5);
}`,
			src: []uint32{2, 3}, dstLen: 2, want: []uint32{30, 20}, tex: gridTexture(7, 9), smp: clamp,
		},
	})
}

// TestTypedValuesTextureStoreRuntimeCoord stores at an integer vector
// coordinate computed at runtime.
func TestTypedValuesTextureStoreRuntimeCoord(t *testing.T) {
	m := compileSoftwareSPIRV(t, `@group(0) @binding(0) var<storage, read> src: array<u32>;
@group(0) @binding(2) var img: texture_storage_2d<rgba8unorm, write>;
@compute @workgroup_size(1) fn main() {
	let c = vec2<i32>(i32(src[0]), i32(src[1])) + vec2<i32>(1, 2);
	textureStore(img, c, vec4<f32>(1.0, 0.5, 0.0, 1.0));
}`)
	img := &Texture2D{Width: 4, Height: 4, Data: make([]byte, 4*4*4),
		Format: uint32(gputypes.TextureFormatRGBA8Unorm), BytesPerPixel: 4}
	ctx := &ExecutionContext{
		Buffers:  map[BindingKey][]byte{{Group: 0, Binding: 0}: wordsToBytes([]uint32{2, 1})},
		Textures: map[BindingKey]*Texture2D{{Group: 0, Binding: 2}: img},
	}
	if err := m.DispatchCompute("main", ctx, 1, 1, 1); err != nil {
		t.Fatalf("DispatchCompute: %v", err)
	}
	// One word per texel, R in the low byte: (1, 0.5, 0, 1) is 0xFF0080FF.
	const texel = 0xFF0080FF
	want := make([]uint32, 16)
	want[3*4+3] = texel // (2, 1) + (1, 2)
	if got := bytesToWords(img.Data); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestTypedValuesWorkgroupPrologue runs naga's workgroup zero-initializing
// prologue (if all(local_invocation_id == vec3(0)) { w = zero }; barrier)
// with several invocations and workgroups: every workgroup must start from
// zeroed memory, whatever the previous one left in it.
func TestTypedValuesWorkgroupPrologue(t *testing.T) {
	m := compileSoftwareSPIRV(t, `@group(0) @binding(1) var<storage, read_write> dst: array<u32>;
var<workgroup> w: array<u32, 4>;
@compute @workgroup_size(4) fn main(@builtin(local_invocation_index) li: u32,
		@builtin(workgroup_id) wg: vec3<u32>) {
	let before = w[li];
	workgroupBarrier();
	w[li] = li + 10u + wg.x * 100u;
	workgroupBarrier();
	dst[wg.x * 8u + li] = w[(li + 1u) % 4u];
	dst[wg.x * 8u + 4u + li] = before;
}`)
	dst := make([]uint32, 16)
	dispatchWords(t, m, [3]uint32{2, 1, 1}, []uint32{0}, dst)
	want := make([]uint32, 16)
	for g := range uint32(2) {
		for li := range uint32(4) {
			want[g*8+li] = (li+1)%4 + 10 + g*100
		}
	}
	if !slices.Equal(dst, want) {
		t.Errorf("got %v, want %v", dst, want)
	}
}

// reportInstrs reports the SPIR-V instructions executed per op since start;
// only a build with -tags swinstrcount counts them.
func reportInstrs(b *testing.B, start uint64) {
	b.Helper()
	if countInstr {
		b.ReportMetric(float64(ExecutedInstructions()-start)/float64(b.N), "instrs/op")
	}
}

func benchTypedCase(b *testing.B, c *typedCase) {
	b.Helper()
	m := compileSoftwareSPIRV(b, c.wgsl)
	ctx, _ := typedContext(c)
	b.ReportAllocs()
	b.ResetTimer()
	start := ExecutedInstructions()
	for b.Loop() {
		if err := m.DispatchCompute("main", ctx, 1, 1, 1); err != nil {
			b.Fatal(err)
		}
	}
	reportInstrs(b, start)
}

// BenchmarkLoopBounded runs a 64-iteration loop under naga's loop bound
// (a vec2<u32> counter, ADR-078 rule 5).
func BenchmarkLoopBounded(b *testing.B) {
	benchTypedCase(b, &typedCase{
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var s = 0u;
	for (var i = 0u; i < src[4]; i++) { s += src[i % 4u]; }
	dst[0] = s;
}`,
		src: []uint32{1, 2, 3, 4, 64}, dstLen: 1,
	})
}

// BenchmarkIntVecArith hashes a vec4<u32> (integer vector arithmetic).
func BenchmarkIntVecArith(b *testing.B) {
	benchTypedCase(b, &typedCase{
		wgsl: typedHeader + `@compute @workgroup_size(1) fn main() {
	var h = vec4<u32>(src[0], src[1], src[2], src[3]);
	for (var i = 0u; i < 16u; i++) {
		h = h * 1664525u + vec4<u32>(1013904223u);
		h = h ^ (h >> vec4<u32>(16u));
	}
	dst[0] = h.x; dst[1] = h.y; dst[2] = h.z; dst[3] = h.w;
}`,
		src: []uint32{1, 2, 3, 4}, dstLen: 4,
	})
}

// BenchmarkUniformUVec4 loads a uniform vec4<u32> and stores its lanes.
func BenchmarkUniformUVec4(b *testing.B) {
	benchTypedCase(b, &typedCase{
		wgsl: `@group(0) @binding(0) var<uniform> u: vec4<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;
@compute @workgroup_size(1) fn main() {
	let v = u + vec4<u32>(1u);
	dst[0] = v.x; dst[1] = v.y; dst[2] = v.z; dst[3] = v.w;
}`,
		src: []uint32{1, 2, 3, 4}, dstLen: 4,
	})
}
