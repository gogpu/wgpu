// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	naga "github.com/gogpu/naga"
)

// Buffer layout tests (P1-SW-022). The expected words come from the WGSL
// memory layout rules (wgsl/index.bs "Memory Layout"), which naga lowers to
// SPIR-V ArrayStride, Offset and MatrixStride decorations:
//
//   - array<vec3<T>>: stride roundUp(16, 12) = 16, so word 3 of each element
//     is padding that no access may touch;
//   - mat3x3<f32>: three vec3 columns at 16-byte steps (MatrixStride 16);
//   - arrayLength of a runtime array: truncate((binding size - offset) / stride).

// layoutSentinel marks a word no store may change.
func layoutSentinel(word int) uint32 { return 0xDEAD0000 + uint32(word) }

// dispatchNagaBuffers compiles wgsl with naga, binds bufs[i] at
// @group(0) @binding(i) and dispatches one workgroup of "main". The buffers
// are updated in place.
func dispatchNagaBuffers(t *testing.T, wgsl string, bufs ...[]uint32) {
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
	raw := make([][]byte, len(bufs))
	ctx := &ExecutionContext{Buffers: make(map[BindingKey][]byte, len(bufs))}
	for i, buf := range bufs {
		raw[i] = make([]byte, 4*len(buf))
		for j, w := range buf {
			binary.LittleEndian.PutUint32(raw[i][j*4:], w)
		}
		ctx.Buffers[BindingKey{Group: 0, Binding: uint32(i)}] = raw[i]
	}
	if err := m.DispatchCompute("main", ctx, 1, 1, 1); err != nil {
		t.Fatalf("DispatchCompute: %v", err)
	}
	for i, buf := range bufs {
		for j := range buf {
			buf[j] = binary.LittleEndian.Uint32(raw[i][j*4:])
		}
	}
}

// sentinelWords returns n words filled with layoutSentinel.
func sentinelWords(n int) []uint32 {
	w := make([]uint32, n)
	for i := range w {
		w[i] = layoutSentinel(i)
	}
	return w
}

// checkWords compares got with want word by word.
func checkWords(t *testing.T, name string, got, want []uint32) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s word %d = %#x, want %#x", name, i, got[i], want[i])
		}
	}
}

func f32w(f float32) uint32 { return math.Float32bits(f) }

// TestBufferLayoutArrayLengthUsesArrayStride checks arrayLength against the
// array stride, not the element size: a vec3 element is 12 bytes, its stride 16.
func TestBufferLayoutArrayLengthUsesArrayStride(t *testing.T) {
	const top = `
@group(0) @binding(0) var<storage, read_write> a: array<vec3<u32>>;
@group(0) @binding(1) var<storage, read_write> n: array<u32>;
@compute @workgroup_size(1)
fn main() { n[0] = arrayLength(&a); }
`
	const member = `
struct B { head: vec4<f32>, tail: array<vec3<f32>> }
@group(0) @binding(0) var<storage, read_write> a: B;
@group(0) @binding(1) var<storage, read_write> n: array<u32>;
@compute @workgroup_size(1)
fn main() { n[0] = arrayLength(&a.tail); }
`
	tests := []struct {
		name  string
		wgsl  string
		words int
		want  uint32
	}{
		{"28 bytes", top, 7, 1},
		{"32 bytes", top, 8, 2},
		{"44 bytes", top, 11, 2},
		{"48 bytes", top, 12, 3},
		{"member at 16, 44 bytes", member, 11, 1},
		{"member at 16, 48 bytes", member, 12, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := []uint32{0xFFFFFFFF}
			dispatchNagaBuffers(t, tc.wgsl, sentinelWords(tc.words), out)
			if out[0] != tc.want {
				t.Errorf("arrayLength = %d, want %d", out[0], tc.want)
			}
		})
	}
}

// TestBufferLayoutVec3F32Array reads, writes and indexes array<vec3<f32>>
// elements at 16-byte steps and leaves every padding word untouched.
func TestBufferLayoutVec3F32Array(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var<storage, read_write> src: array<vec3<f32>>;
@group(0) @binding(1) var<storage, read_write> dst: array<vec3<f32>>;
@group(0) @binding(2) var<storage, read_write> lanes: array<f32>;
@compute @workgroup_size(1)
fn main() {
    dst[0] = src[0];
    dst[1] = vec3<f32>(7.0, 8.0, 9.0);
    dst[2] = src[1];
    dst[2].y = 42.0;
    lanes[0] = src[2].z;
    lanes[1] = src[1].x;
}
`
	// Element i, component c holds 10*i + c + 1; word 4*i + 3 is padding.
	src := sentinelWords(12)
	for i := range 3 {
		for c := range 3 {
			src[4*i+c] = f32w(float32(10*i + c + 1))
		}
	}
	dst := sentinelWords(12)
	lanes := sentinelWords(2)
	wantSrc := append([]uint32(nil), src...)
	dispatchNagaBuffers(t, wgsl, src, dst, lanes)

	want := sentinelWords(12)
	copy(want[0:3], wantSrc[0:3])
	want[4], want[5], want[6] = f32w(7), f32w(8), f32w(9)
	want[8], want[9], want[10] = wantSrc[4], f32w(42), wantSrc[6]
	checkWords(t, "dst", dst, want)
	checkWords(t, "src", src, wantSrc)
	checkWords(t, "lanes", lanes, []uint32{f32w(23), f32w(11)})
}

// TestBufferLayoutVec3IntArray stores integer vectors and their components
// as integers at 16-byte steps: a vec3<u32> or vec3<i32> store must not
// write the components as f32 bits.
func TestBufferLayoutVec3IntArray(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var<storage, read_write> src: array<vec3<u32>>;
@group(0) @binding(1) var<storage, read_write> dst: array<vec3<u32>>;
@group(0) @binding(2) var<storage, read_write> lanes: array<u32>;
@group(0) @binding(3) var<storage, read_write> idst: array<vec3<i32>>;
@group(0) @binding(4) var<storage, read_write> ilanes: array<i32>;
@compute @workgroup_size(1)
fn main() {
    dst[0] = src[1];
    dst[1] = vec3<u32>(0xA1u, 0xA2u, 0xA3u);
    dst[2] = src[2];
    dst[2].x = 0xBEEFu;
    lanes[0] = src[2].y;
    lanes[1] = src[0].z;
    let s = idst[0];
    idst[1] = vec3<i32>(-5, 6, -7);
    idst[2] = s;
    ilanes[0] = s.y;
}
`
	// Element i, component c holds 0x1000 + 16*i + c; word 4*i + 3 is padding.
	src := sentinelWords(12)
	for i := range 3 {
		for c := range 3 {
			src[4*i+c] = 0x1000 + uint32(16*i+c)
		}
	}
	dst := sentinelWords(12)
	lanes := sentinelWords(2)
	idst := sentinelWords(12)
	neg := []int32{-100, -200, 300}
	for c, v := range neg {
		idst[c] = uint32(v)
	}
	ilanes := sentinelWords(1)
	dispatchNagaBuffers(t, wgsl, src, dst, lanes, idst, ilanes)

	want := sentinelWords(12)
	want[0], want[1], want[2] = 0x1010, 0x1011, 0x1012
	want[4], want[5], want[6] = 0xA1, 0xA2, 0xA3
	want[8], want[9], want[10] = 0xBEEF, 0x1021, 0x1022
	checkWords(t, "dst", dst, want)
	checkWords(t, "lanes", lanes, []uint32{0x1021, 0x1002})

	wantI := sentinelWords(12)
	for c, v := range neg {
		wantI[c] = uint32(v)
		wantI[8+c] = uint32(v)
	}
	for c, v := range []int32{-5, 6, -7} {
		wantI[4+c] = uint32(v)
	}
	checkWords(t, "idst", idst, wantI)
	checkWords(t, "ilanes", ilanes, []uint32{uint32(neg[1])})
}

// TestBufferLayoutMat3x3Member reads and writes a mat3x3<f32> struct member
// and an array of them, in a uniform and a storage buffer: columns at
// 16-byte steps (MatrixStride 16), array elements 48 bytes apart.
//
// struct S layout (WGSL): a @0, m @16 (cols 16, 32, 48), n @64 (n[0] cols
// 64, 80, 96; n[1] cols 112, 128, 144), t @160, size 176.
func TestBufferLayoutMat3x3Member(t *testing.T) {
	const wgsl = `
struct S { a: f32, m: mat3x3<f32>, n: array<mat3x3<f32>, 2>, t: f32 }
@group(0) @binding(0) var<uniform> u: S;
@group(0) @binding(1) var<storage, read_write> s: S;
@group(0) @binding(2) var<storage, read_write> lanes: array<f32>;
@compute @workgroup_size(1)
fn main() {
    s.m = u.m;
    s.n[1] = u.n[0];
    s.n[0][2] = vec3<f32>(70.0, 71.0, 72.0);
    s.t = u.t;
    lanes[0] = u.m[2].z;
    lanes[1] = u.n[1][1].x;
    lanes[2] = s.m[1].y;
}
`
	const words = 44
	u := make([]uint32, words)
	for w := range u {
		u[w] = f32w(float32(w) + 0.5)
	}
	s := sentinelWords(words)
	lanes := sentinelWords(3)
	dispatchNagaBuffers(t, wgsl, u, s, lanes)

	want := sentinelWords(words)
	for c := range 3 {
		for r := range 3 {
			want[4+4*c+r] = u[4+4*c+r]   // s.m = u.m
			want[28+4*c+r] = u[16+4*c+r] // s.n[1] = u.n[0]
		}
	}
	want[24], want[25], want[26] = f32w(70), f32w(71), f32w(72) // s.n[0][2]
	want[40] = u[40]                                            // s.t
	checkWords(t, "s", s, want)
	checkWords(t, "lanes", lanes, []uint32{u[14], u[32], u[9]})
}

// TestBufferLayoutUniformIntVector reads integer vector members of a
// uniform buffer and stores their components to a u32 storage buffer: the
// interpreter holds the loaded components as float32, and the store must
// write the integers, not their f32 bits (41 came out as 0x42240000).
func TestBufferLayoutUniformIntVector(t *testing.T) {
	const wgsl = `
struct U { rect: vec4<f32>, tag: vec4<u32>, itag: vec4<i32> }
@group(0) @binding(0) var<uniform> u: U;
@group(0) @binding(1) var<storage, read_write> out: array<u32>;
@group(0) @binding(2) var<storage, read_write> iout: array<i32>;
@compute @workgroup_size(1)
fn main() {
    out[0] = u.tag.x;
    out[1] = u.tag.w;
    let t = u.tag;
    out[2] = t.y;
    iout[0] = u.itag.z;
}
`
	u := make([]uint32, 12)
	copy(u[4:8], []uint32{41, 0xFFFFFF, 7, 0x123456})
	minus9 := int32(-9)
	u[10] = uint32(minus9)
	out := sentinelWords(3)
	iout := sentinelWords(1)
	dispatchNagaBuffers(t, wgsl, u, out, iout)
	checkWords(t, "out", out, []uint32{41, 0x123456, 0xFFFFFF})
	checkWords(t, "iout", iout, []uint32{uint32(minus9)})
}

// TestSharesWorkgroupMemoryAcrossBarrierNeedsBarrier: the same module with
// its OpControlBarrier instructions removed no longer shares workgroup
// memory across a barrier (naga always emits one after zero-initializing
// workgroup memory, so this variant cannot come from WGSL).
func TestSharesWorkgroupMemoryAcrossBarrierNeedsBarrier(t *testing.T) {
	spirvBytes, err := naga.Compile(`
var<workgroup> wg: array<u32, 64>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    wg[i] = i;
    workgroupBarrier();
    out[i] = wg[(i + 1u) % 64u];
}
`)
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
	if !m.SharesWorkgroupMemoryAcrossBarrier("main") {
		t.Fatal("with its barriers: SharesWorkgroupMemoryAcrossBarrier = false, want true")
	}
	for _, fn := range m.FunctionsByID {
		fn.Instructions = slices.DeleteFunc(fn.Instructions, func(inst Instruction) bool { return inst.Opcode == OpControlBarrier })
	}
	if m.SharesWorkgroupMemoryAcrossBarrier("main") {
		t.Error("without barriers: SharesWorkgroupMemoryAcrossBarrier = true, want false")
	}
}
