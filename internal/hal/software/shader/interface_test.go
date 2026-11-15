// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"math"
	"slices"
	"testing"
)

const interfaceTestShader = `
@fragment
fn fs(@location(0) @interpolate(flat) u: vec4<u32>,
      @location(1) @interpolate(flat) s: i32,
      @location(2) @interpolate(flat) b: vec2<f32>,
      @location(3) c: vec3<f32>) -> @location(0) vec4<u32> {
    return u + vec4<u32>(bitcast<u32>(s)) + vec4<u32>(vec2<u32>(b), vec2<u32>(c.xy));
}
`

// inputAtLocation returns the Input variable of m decorated with location.
func inputAtLocation(t *testing.T, m *Module, location uint32) uint32 {
	t.Helper()
	for id, v := range m.Variables {
		if v.StorageClass != StorageClassInput {
			continue
		}
		if loc, ok := m.Decorations[decorationKey{TargetID: id, Decoration: DecorationLocation}]; ok && loc == location {
			return id
		}
	}
	t.Fatalf("no input at location %d", location)
	return 0
}

func lanesOf(words ...uint32) []float32 {
	lanes := make([]float32, len(words))
	for i, w := range words {
		lanes[i] = math.Float32frombits(w)
	}
	return lanes
}

func wordsOf(lanes []float32) []uint32 {
	words := make([]uint32, len(lanes))
	for i, l := range lanes {
		words[i] = math.Float32bits(l)
	}
	return words
}

// TestInterfaceValueLanes checks the interface lane transport (ADR-078 rule
// 7): an integer variable takes its lanes as words, exactly, NaN patterns
// included, and AppendInterfaceLanes gives the same lanes back; a float
// variable takes the lanes as values.
func TestInterfaceValueLanes(t *testing.T) {
	m := compileSoftwareSPIRV(t, interfaceTestShader)
	u := inputAtLocation(t, m, 0)
	s := inputAtLocation(t, m, 1)
	b := inputAtLocation(t, m, 2)
	c := inputAtLocation(t, m, 3)

	for _, tc := range []struct {
		name  string
		id    uint32
		words []uint32 // lanes as given
		kind  laneKind
		want  []uint32 // the value's words
	}{
		{"vec4<u32>", u, []uint32{0xDEADBEEF, 0xFFFFFFFF, 0x7F800001, 16777217}, kindUint,
			[]uint32{0xDEADBEEF, 0xFFFFFFFF, 0x7F800001, 16777217}},
		// Lanes past the type's components are ignored, missing ones are zero.
		{"vec4<u32> extra lane", u, []uint32{1, 2, 3, 4, 5}, kindUint, []uint32{1, 2, 3, 4}},
		{"vec4<u32> short", u, []uint32{0xFFC00001, 7}, kindUint, []uint32{0xFFC00001, 7, 0, 0}},
		{"i32", s, []uint32{0x80000000}, kindSint, []uint32{0x80000000}},
		{"vec2<f32>", b, []uint32{math.Float32bits(1.5), math.Float32bits(-2)}, kindFloat,
			[]uint32{math.Float32bits(1.5), math.Float32bits(-2)}},
		{"vec3<f32>", c, []uint32{math.Float32bits(0.25), 0, math.Float32bits(3)}, kindFloat,
			[]uint32{math.Float32bits(0.25), 0, math.Float32bits(3)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := m.InterfaceValue(tc.id, lanesOf(tc.words...))
			if k := vectorKind(v.Tag); k != tc.kind {
				t.Fatalf("kind = %v, want %v", k, tc.kind)
			}
			if n := vectorLanes(v.Tag); n != len(tc.want) {
				t.Fatalf("lanes = %d, want %d", n, len(tc.want))
			}
			back := wordsOf(AppendInterfaceLanes(nil, v))
			if !slices.Equal(back, tc.want) {
				t.Errorf("AppendInterfaceLanes = %#x, want %#x", back, tc.want)
			}
			var held []uint32
			if tc.kind == kindFloat {
				held = wordsOf(v.F[:len(tc.want)])
			} else {
				held = slices.Clone(v.U[:len(tc.want)])
			}
			if !slices.Equal(held, tc.want) {
				t.Errorf("value words = %#x, want %#x", held, tc.want)
			}
		})
	}
}

// TestInterpolatesFlat checks which interface variables the rasterizer must
// copy from the provoking vertex: Flat-decorated ones and every integer one.
func TestInterpolatesFlat(t *testing.T) {
	m := compileSoftwareSPIRV(t, interfaceTestShader)
	for loc, want := range []bool{true, true, true, false} {
		if got := m.InterpolatesFlat(inputAtLocation(t, m, uint32(loc))); got != want {
			t.Errorf("location %d: InterpolatesFlat = %v, want %v", loc, got, want)
		}
	}
}

// TestInterpolatesLinear checks that @interpolate(linear) inputs, and only
// they, are reported as interpolated linearly in screen space.
func TestInterpolatesLinear(t *testing.T) {
	m := compileSoftwareSPIRV(t, `
@fragment
fn fs(@location(0) p: vec2<f32>,
      @location(1) @interpolate(linear) l: vec2<f32>,
      @location(2) @interpolate(linear, sample) ls: f32,
      @location(3) @interpolate(perspective, centroid) pc: f32) -> @location(0) vec4<f32> {
    return vec4<f32>(p + l, ls, pc);
}
`)
	for loc, want := range []bool{false, true, true, false} {
		if got := m.InterpolatesLinear(inputAtLocation(t, m, uint32(loc))); got != want {
			t.Errorf("location %d: InterpolatesLinear = %v, want %v", loc, got, want)
		}
	}
}
