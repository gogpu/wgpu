// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"time"

	naga "github.com/gogpu/naga"
)

// Workgroup memory and barrier tests (P1-SW-025). WGSL (wgsl/index.bs,
// "Address Spaces" and "Synchronization Built-in Functions"): a variable in
// the workgroup address space is shared by all invocations of one workgroup
// and zero-initialized for each workgroup; workgroupBarrier makes every
// invocation of the workgroup wait until all of them have reached it, and the
// workgroup memory writes made before it are visible to the reads after it.

// compileNaga compiles wgsl with naga and parses the SPIR-V.
func compileNaga(t testing.TB, wgsl string) *Module {
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
	return m
}

// dispatchWords binds bufs[i] at @group(0) @binding(i), dispatches groups
// workgroups of "main" and copies the buffers back into bufs.
func dispatchWords(t testing.TB, m *Module, groups [3]uint32, bufs ...[]uint32) {
	t.Helper()
	raw := make([][]byte, len(bufs))
	ctx := &ExecutionContext{Buffers: make(map[BindingKey][]byte, len(bufs))}
	for i, buf := range bufs {
		raw[i] = make([]byte, 4*len(buf))
		for j, w := range buf {
			binary.LittleEndian.PutUint32(raw[i][j*4:], w)
		}
		ctx.Buffers[BindingKey{Group: 0, Binding: uint32(i)}] = raw[i]
	}
	if err := m.DispatchCompute("main", ctx, groups[0], groups[1], groups[2]); err != nil {
		t.Fatalf("DispatchCompute: %v", err)
	}
	for i, buf := range bufs {
		for j := range buf {
			buf[j] = binary.LittleEndian.Uint32(raw[i][j*4:])
		}
	}
}

// TestWorkgroupMemorySharedAcrossBarrier: invocation i writes slot i, waits
// at the barrier, then reads the slot its neighbor wrote.
func TestWorkgroupMemorySharedAcrossBarrier(t *testing.T) {
	const n = 64
	m := compileNaga(t, `
var<workgroup> wg: array<u32, 64>;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    wg[i] = i * 10u + 1u;
    workgroupBarrier();
    out[i] = wg[(i + 1u) % 64u];
}
`)
	out := make([]uint32, n)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, out)
	for i := range out {
		if want := uint32((i+1)%n)*10 + 1; out[i] != want {
			t.Errorf("out[%d] = %d, want %d", i, out[i], want)
		}
	}
}

// TestWorkgroupTreeReduction: 256 invocations sum their values in
// workgroup memory, halving the active range after each barrier.
func TestWorkgroupTreeReduction(t *testing.T) {
	m := compileNaga(t, `
var<workgroup> sdata: array<u32, 256>;
@group(0) @binding(0) var<storage, read> input: array<u32>;
@group(0) @binding(1) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(256)
fn main(@builtin(local_invocation_index) i: u32) {
    sdata[i] = input[i];
    workgroupBarrier();
    for (var s = 128u; s > 0u; s = s >> 1u) {
        if (i < s) {
            sdata[i] = sdata[i] + sdata[i + s];
        }
        workgroupBarrier();
    }
    if (i == 0u) {
        out[0] = sdata[0];
    }
}
`)
	input := make([]uint32, 256)
	var want uint32
	for i := range input {
		input[i] = uint32(i*7 + 3)
		want += input[i]
	}
	out := make([]uint32, 1)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, input, out)
	if out[0] != want {
		t.Errorf("sum = %d, want %d", out[0], want)
	}
}

// TestWorkgroupAtomicAdd: every invocation adds 1 to a workgroup counter.
// Each must see a distinct old value, and after the barrier all see the
// total. Two workgroups check that each starts from a zeroed counter.
func TestWorkgroupAtomicAdd(t *testing.T) {
	const n, groups = 64, 2
	m := compileNaga(t, `
var<workgroup> counter: atomic<u32>;
@group(0) @binding(0) var<storage, read_write> old: array<u32>;
@group(0) @binding(1) var<storage, read_write> total: array<u32>;
@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    old[gid.x] = atomicAdd(&counter, 1u);
    workgroupBarrier();
    total[gid.x] = atomicLoad(&counter);
}
`)
	old := make([]uint32, n*groups)
	total := make([]uint32, n*groups)
	dispatchWords(t, m, [3]uint32{groups, 1, 1}, old, total)
	for g := range groups {
		seen := slices.Clone(old[g*n : (g+1)*n])
		slices.Sort(seen)
		for i, v := range seen {
			if v != uint32(i) {
				t.Fatalf("workgroup %d: atomicAdd old values (sorted) = %v, want 0..%d", g, seen, n-1)
			}
		}
		for i := g * n; i < (g+1)*n; i++ {
			if total[i] != n {
				t.Errorf("workgroup %d invocation %d: counter after barrier = %d, want %d", g, i-g*n, total[i], n)
			}
		}
	}
}

// TestWorkgroupBarrierInHelperFunction: the barrier and the workgroup
// accesses are in a function the entry point calls, so an invocation stops
// at the barrier inside a call and resumes there.
func TestWorkgroupBarrierInHelperFunction(t *testing.T) {
	const n = 32
	m := compileNaga(t, `
var<workgroup> wg: array<u32, 32>;
var<private> bias: u32 = 5u;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
fn exchange(i: u32) -> u32 {
    wg[i] = i + bias;
    workgroupBarrier();
    return wg[31u - i];
}
@compute @workgroup_size(32)
fn main(@builtin(local_invocation_index) i: u32) {
    let v = exchange(i);
    out[i] = v * 2u;
}
`)
	out := make([]uint32, n)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, out)
	for i := range out {
		if want := (uint32(n-1-i) + 5) * 2; out[i] != want {
			t.Errorf("out[%d] = %d, want %d", i, out[i], want)
		}
	}
}

// TestWorkgroupStructMembers: a workgroup struct has no layout decorations
// (naga emits a layout-free type), and its members must not overlap.
func TestWorkgroupStructMembers(t *testing.T) {
	m := compileNaga(t, `
struct Tile {
    a: u32,
    b: vec2<f32>,
    c: array<u32, 4>,
    d: f32,
}
var<workgroup> tile: Tile;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(4)
fn main(@builtin(local_invocation_index) i: u32) {
    if (i == 0u) { tile.a = 11u; }
    if (i == 1u) { tile.b = vec2<f32>(2.5, 3.5); }
    tile.c[i] = 100u + i;
    if (i == 3u) { tile.d = 7.25; }
    workgroupBarrier();
    let t = tile;
    out[i] = t.c[3u - i];
    if (i == 0u) {
        out[4] = t.a;
        out[5] = bitcast<u32>(t.b.x);
        out[6] = bitcast<u32>(t.b.y);
        out[7] = bitcast<u32>(t.d);
    }
}
`)
	out := make([]uint32, 8)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, out)
	want := []uint32{103, 102, 101, 100, 11, math.Float32bits(2.5), math.Float32bits(3.5), math.Float32bits(7.25)}
	checkWords(t, "out", out, want)
}

// globalSumWGSL is a two-pass global sum like Born's globalSumShader: each
// of 256 invocations accumulates a grid-stride slice of the input, the
// workgroup tree-reduces the slices in workgroup memory, and invocation 0
// writes the workgroup's partial sum. The second pass runs the same shader
// over the partials in one workgroup. params[0] is the element count.
const globalSumWGSL = `
@group(0) @binding(0) var<storage, read> input: array<T>;
@group(0) @binding(1) var<storage, read_write> partials: array<T>;
@group(0) @binding(2) var<storage, read> params: array<u32>;
var<workgroup> sdata: array<T, 256>;
@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid: vec3<u32>,
        @builtin(local_invocation_id) lid: vec3<u32>,
        @builtin(workgroup_id) wid: vec3<u32>,
        @builtin(num_workgroups) nwg: vec3<u32>) {
    let n = params[0];
    var acc: T = T(0);
    for (var i = gid.x; i < n; i = i + 256u * nwg.x) {
        acc = acc + input[i];
    }
    sdata[lid.x] = acc;
    workgroupBarrier();
    for (var s = 128u; s > 0u; s = s >> 1u) {
        if (lid.x < s) {
            sdata[lid.x] = sdata[lid.x] + sdata[lid.x + s];
        }
        workgroupBarrier();
    }
    if (lid.x == 0u) {
        partials[wid.x] = sdata[0];
    }
}
`

// globalSum runs the two passes of globalSumWGSL over n copies of one.
func globalSum(t testing.TB, m *Module, n int, one uint32) uint32 {
	t.Helper()
	groups := (n + 255) / 256
	input := make([]uint32, n)
	for i := range input {
		input[i] = one
	}
	partials := make([]uint32, groups)
	dispatchWords(t, m, [3]uint32{uint32(groups), 1, 1}, input, partials, []uint32{uint32(n)})
	final := make([]uint32, 1)
	dispatchWords(t, m, [3]uint32{1, 1, 1}, partials, final, []uint32{uint32(groups)})
	return final[0]
}

// TestWorkgroupGlobalSumBornProbe is Born's probe: the sum of n ones. With
// unshared workgroup memory v0.34.4 returned the workgroup count (4, 16,
// 391) instead of n (1024, 4096, 100000).
func TestWorkgroupGlobalSumBornProbe(t *testing.T) {
	types := []struct {
		name   string
		one    uint32
		decode func(uint32) float64
	}{
		{"f32", math.Float32bits(1), func(w uint32) float64 { return float64(math.Float32frombits(w)) }},
		{"i32", 1, func(w uint32) float64 { return float64(int32(w)) }},
	}
	for _, ty := range types {
		m := compileNaga(t, replaceT(globalSumWGSL, ty.name))
		for _, n := range []int{1023, 1024, 4096, 100000} {
			start := time.Now()
			got := ty.decode(globalSum(t, m, n, ty.one))
			if n == 100000 {
				t.Logf("%s n=%d: %v", ty.name, n, time.Since(start))
			}
			if got != float64(n) {
				t.Errorf("%s: sum of %d ones = %v, want %d", ty.name, n, got, n)
			}
		}
	}
}

// replaceT substitutes the element type for T in globalSumWGSL.
func replaceT(src, ty string) string {
	out := make([]byte, 0, len(src)+64)
	for i := 0; i < len(src); i++ {
		if src[i] == 'T' && (i == 0 || !isIdent(src[i-1])) && (i+1 == len(src) || !isIdent(src[i+1])) {
			out = append(out, ty...)
			continue
		}
		out = append(out, src[i])
	}
	return string(out)
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// BenchmarkWorkgroupGlobalSum is the first pass of globalSumWGSL over 100000
// f32 ones (391 workgroups of 256 invocations).
func BenchmarkWorkgroupGlobalSum(b *testing.B) {
	m := compileNaga(b, replaceT(globalSumWGSL, "f32"))
	const n = 100000
	input := make([]uint32, n)
	for i := range input {
		input[i] = math.Float32bits(1)
	}
	partials := make([]uint32, (n+255)/256)
	params := []uint32{n}
	b.ReportAllocs()
	start := ExecutedInstructions()
	for b.Loop() {
		dispatchWords(b, m, [3]uint32{uint32(len(partials)), 1, 1}, input, partials, params)
	}
	reportInstrs(b, start)
}

// TestWorkgroupVectorTileWholeLoad: a workgroup array of vectors stored per
// element, loaded whole by another invocation after the barrier and read at a
// run-time component index (naga readback corpus vars/workgroup-tile-dynamic-rt).
func TestWorkgroupVectorTileWholeLoad(t *testing.T) {
	m := compileNaga(t, `
@group(0) @binding(0) var<storage, read> src: array<u32, 4>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;
var<workgroup> tile: array<vec4<u32>, 4>;
@compute @workgroup_size(4)
fn main(@builtin(local_invocation_index) i: u32) {
    tile[i] = vec4<u32>(4u * i, 4u * i + 1u, 4u * i + 2u, 4u * i + 3u);
    workgroupBarrier();
    let t = tile[3u - i];
    dst[i] = t[src[i]];
}
`)
	src := []uint32{1, 2, 3, 0}
	dst := []uint32{0xDEADBEEF, 0xDEADBEEF, 0xDEADBEEF, 0xDEADBEEF, 0xDEADBEEF}
	dispatchWords(t, m, [3]uint32{1, 1, 1}, src, dst)
	want := []uint32{13, 10, 7, 0, 0xDEADBEEF}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("dst[%d] = %#x, want %#x", i, dst[i], want[i])
		}
	}
}
