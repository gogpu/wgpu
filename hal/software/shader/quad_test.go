//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"testing"

	naga "github.com/gogpu/naga"
)

// parseNaga compiles wgsl with naga and parses the SPIR-V.
func parseNaga(t testing.TB, wgsl string) *Module {
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

// locationVars returns the Input and the Output variables of entry point
// entry by @location.
func locationVars(t *testing.T, m *Module, entry string) (in, out map[int]uint32) {
	t.Helper()
	ep, ok := m.EntryPoints[entry]
	if !ok {
		t.Fatalf("no entry point %q", entry)
	}
	in, out = map[int]uint32{}, map[int]uint32{}
	for _, id := range ep.InterfaceIDs {
		vi, ok := m.Variables[id]
		if !ok || m.GetLocation(id) < 0 {
			continue
		}
		switch vi.StorageClass {
		case StorageClassInput:
			in[m.GetLocation(id)] = id
		case StorageClassOutput:
			out[m.GetLocation(id)] = id
		}
	}
	return in, out
}

// quadTestShader takes derivatives of @location(0) in a function the entry
// point calls, counts each lane's invocations in a storage buffer and
// discards lane 3 after the derivatives.
const quadTestShader = `
@group(0) @binding(0) var<storage, read_write> hits: array<atomic<u32>, 4>;

fn derivs(v: f32) -> vec4<f32> {
    return vec4<f32>(dpdxFine(v), dpdyFine(v), dpdxCoarse(v), dpdyCoarse(v));
}

@fragment
fn main(@location(0) v: f32, @location(1) @interpolate(flat) lane: u32) -> @location(0) vec4<f32> {
    atomicAdd(&hits[lane], 1u);
    let d = derivs(v);
    if lane == 3u {
        discard;
    }
    return d;
}

@fragment
fn plain(@location(0) v: f32) -> @location(0) vec4<f32> {
    return vec4<f32>(v);
}
`

// BenchmarkExecuteQuad runs one quad of a fragment shader with fine and
// coarse derivatives, the input and output maps reused as the software
// backend's draw reuses them; allocs/op is per quad of four fragments.
func BenchmarkExecuteQuad(b *testing.B) {
	m := parseNaga(b, `
@fragment
fn main(@location(0) v: vec2<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(dpdx(v.x), dpdy(v.y), fwidth(v.x * v.y), dpdyCoarse(v.x));
}`)
	ep := m.EntryPoints["main"]
	var inID uint32
	for _, id := range ep.InterfaceIDs {
		if vi, ok := m.Variables[id]; ok && vi.StorageClass == StorageClassInput {
			inID = id
		}
	}
	ctx := &ExecutionContext{}
	var q Quad
	for lane := range QuadLanes {
		q.Inputs[lane] = map[uint32]Value{inID: ValVec2(float32(lane&1), float32(lane>>1))}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.ExecuteQuad("main", ctx, &q); err != nil {
			b.Fatal(err)
		}
	}
}

// TestExecuteQuad checks ExecuteQuad's derivatives (fine: per row and
// column; coarse: the top row and the left column), that a helper lane runs
// for its neighbors but neither writes memory nor returns outputs, that a
// discarding lane is reported, and that NeedsQuads follows calls.
func TestExecuteQuad(t *testing.T) {
	m := parseNaga(t, quadTestShader)
	if !m.NeedsQuads("main") {
		t.Error("NeedsQuads(main) = false: its callee takes derivatives")
	}
	if m.NeedsQuads("plain") {
		t.Error("NeedsQuads(plain) = true: it has no derivative")
	}

	in, out := locationVars(t, m, "main")
	hits := make([]byte, 16)
	ctx := &ExecutionContext{Buffers: map[BindingKey][]byte{{Group: 0, Binding: 0}: hits}}
	values := [QuadLanes]float32{1, 3, 10, 30} // lanes (0,0) (1,0) (0,1) (1,1)
	var q Quad
	for lane := range QuadLanes {
		q.Inputs[lane] = map[uint32]Value{in[0]: ValFloat(values[lane]), in[1]: ValUint(uint32(lane))}
	}
	q.Helpers = 1 << 2

	// Twice, so the second run reuses the output maps the first made.
	for run := range 2 {
		clear(hits)
		if err := m.ExecuteQuad("main", ctx, &q); err != nil {
			t.Fatalf("run %d: ExecuteQuad: %v", run, err)
		}
		if q.Discarded != 1<<3 {
			t.Errorf("run %d: Discarded = %04b, want 1000", run, q.Discarded)
		}
		for lane, want := range [QuadLanes]uint32{1, 1, 0, 1} {
			if got := binary.LittleEndian.Uint32(hits[lane*4:]); got != want {
				t.Errorf("run %d: lane %d wrote hits = %d, want %d", run, lane, got, want)
			}
		}
		wants := [QuadLanes]*[4]float32{
			{2, 9, 2, 9},  // row 0: 3-1; column 0: 10-1
			{2, 27, 2, 9}, // column 1: 30-3
			nil,           // helper
			nil,           // discarded
		}
		for lane, want := range wants {
			got, ok := q.Outputs[lane][out[0]]
			if want == nil {
				if ok {
					t.Errorf("run %d: lane %d returned %v, want no output", run, lane, got.F)
				}
				continue
			}
			if !ok || got.F != *want {
				t.Errorf("run %d: lane %d = %v (ok %v), want %v", run, lane, got.F, ok, *want)
			}
		}
	}
}
