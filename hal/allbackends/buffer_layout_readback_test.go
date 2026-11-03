// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// bufferLayoutShader reads, writes and indexes vec3 arrays and a mat3x3
// struct member in storage buffers. Every expected word below comes from the
// WGSL memory layout (wgsl/index.bs "Memory Layout"):
//
//   - array<vec3<T>>: element i at byte 16*i, its word 3 padding;
//   - struct M { a: f32, m: mat3x3<f32>, t: f32 }: a at word 0, the columns
//     of m at words 4, 8 and 12 (column stride 16), t at word 16, size 80;
//   - a store to a composite never touches its padding ("Memory operations on
//     composites will not access padding memory locations", §Memory Locations).
const bufferLayoutShader = `
struct M { a: f32, m: mat3x3<f32>, t: f32 }

@group(0) @binding(0) var<storage, read_write> src: array<vec3<u32>>;
@group(0) @binding(1) var<storage, read_write> du: array<vec3<u32>>;
@group(0) @binding(2) var<storage, read_write> df: array<vec3<f32>>;
@group(0) @binding(3) var<storage, read_write> mats: M;
@group(0) @binding(4) var<storage, read_write> lanes: array<u32>;

@compute @workgroup_size(1)
fn main() {
    du[0] = src[1];
    du[1] = vec3<u32>(0xA1u, 0xA2u, 0xA3u);
    du[2] = src[0];
    du[2].y = 0xB2u;
    df[0] = vec3<f32>(1.5, 2.5, 3.5);
    df[1] = vec3<f32>(4.5, 5.5, 6.5);
    df[2].z = 9.5;
    mats.a = 0.25;
    mats.m = mat3x3<f32>(vec3<f32>(10.0, 11.0, 12.0), vec3<f32>(20.0, 21.0, 22.0), vec3<f32>(30.0, 31.0, 32.0));
    mats.m[1].y = 99.0;
    mats.t = 7.75;
    lanes[0] = arrayLength(&src);
    lanes[1] = src[2].z;
    let v = src[1];
    lanes[2] = v.x;
    lanes[3] = bitcast<u32>(mats.m[2].z);
}
`

const (
	layoutVecWords  = 12 // three vec3 elements
	layoutMatWords  = 20 // struct M
	layoutLaneWords = 4
)

// layoutUntouched is the initial value of word w of binding b; a word the
// shader does not write must still hold it.
func layoutUntouched(b, w int) uint32 { return 0x5EED0000 + uint32(b)<<8 + uint32(w) }

// layoutSrc is component c of src element i.
func layoutSrc(i, c int) uint32 { return 0x1000 + uint32(16*i+c) }

// TestBufferLayoutVec3Mat3x3 checks the in-bounds buffer layout of
// array<vec3<u32>>, array<vec3<f32>> and a mat3x3<f32> struct member through
// the public API on every backend: element and column strides of 16 bytes,
// arrayLength by stride, integer components kept integer, padding untouched.
func TestBufferLayoutVec3Mat3x3(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		sizes := []int{layoutVecWords, layoutVecWords, layoutVecWords, layoutMatWords, layoutLaneWords}
		inputs := make([][]uint32, len(sizes))
		for b, n := range sizes {
			inputs[b] = make([]uint32, n)
			for w := range n {
				inputs[b][w] = layoutUntouched(b, w)
			}
		}
		for i := range 3 {
			for c := range 3 {
				inputs[0][4*i+c] = layoutSrc(i, c)
			}
		}
		wantSrc := append([]uint32(nil), inputs[0]...)

		got, err := runPublicCompute(t, device, bufferLayoutShader, nil, inputs)
		if err != nil {
			t.Fatal(err)
		}

		checkLayoutWords(t, "src", got[0], wantSrc)
		checkLayoutWords(t, "du", got[1], expectLayoutDU())
		checkLayoutWords(t, "df", got[2], expectLayoutDF())
		checkLayoutWords(t, "mats", got[3], expectLayoutMats())
		checkLayoutWords(t, "lanes", got[4], []uint32{3, layoutSrc(2, 2), layoutSrc(1, 0), math.Float32bits(32)})
	})
}

func expectLayoutDU() []uint32 {
	want := untouchedWords(1, layoutVecWords)
	for c := range 3 {
		want[c] = layoutSrc(1, c)    // du[0] = src[1]
		want[4+c] = 0xA1 + uint32(c) // du[1]
		want[8+c] = layoutSrc(0, c)  // du[2] = src[0]
	}
	want[9] = 0xB2 // du[2].y
	return want
}

func expectLayoutDF() []uint32 {
	want := untouchedWords(2, layoutVecWords)
	for c, f := range []float32{1.5, 2.5, 3.5, 0, 4.5, 5.5, 6.5} {
		if c != 3 {
			want[c] = math.Float32bits(f)
		}
	}
	want[10] = math.Float32bits(9.5) // df[2].z
	return want
}

func expectLayoutMats() []uint32 {
	want := untouchedWords(3, layoutMatWords)
	want[0] = math.Float32bits(0.25)
	for col := range 3 {
		for row := range 3 {
			want[4+4*col+row] = math.Float32bits(float32(10*(col+1) + row))
		}
	}
	want[9] = math.Float32bits(99)    // mats.m[1].y
	want[16] = math.Float32bits(7.75) // mats.t
	return want
}

func untouchedWords(b, n int) []uint32 {
	w := make([]uint32, n)
	for i := range w {
		w[i] = layoutUntouched(b, i)
	}
	return w
}

func checkLayoutWords(t *testing.T, name string, got, want []uint32) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s word %d = %#x, want %#x", name, i, got[i], want[i])
		}
	}
}
