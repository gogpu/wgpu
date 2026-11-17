// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// flatVaryingShader carries integer vertex attributes to the fragment stage as
// flat varyings and writes them to an RGBA32Uint target: a vec4<u32> from a
// Uint32x4 attribute, an i32 from a Sint32 attribute and a u32 the vertex
// shader computes from vertex_index. The XORs fold the i32 and the u32 into
// two channels of the output, so all six words reach the target exactly.
const flatVaryingShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) @interpolate(flat) u: vec4<u32>,
    @location(1) @interpolate(flat) s: i32,
    @location(2) @interpolate(flat) k: u32,
}

@vertex
fn vs(@location(0) p: vec2f, @location(1) u: vec4<u32>, @location(2) s: i32,
      @builtin(vertex_index) vi: u32) -> VOut {
    return VOut(vec4f(p, 0.0, 1.0), u, s, 0xA5000000u + vi);
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<u32> {
    return vec4<u32>(in.u.x, in.u.y, in.u.z ^ bitcast<u32>(in.s), in.u.w ^ in.k);
}
`

// flatVaryingRecord is one vertex's integer attributes. Every word is above
// 2^24, and several are float NaN or infinity bit patterns, so a value that
// goes through float32 arithmetic, or a lane converted with float32(int), is
// caught.
type flatVaryingRecord struct {
	u [4]uint32
	s int32
}

var flatVaryingRecords = [6]flatVaryingRecord{
	{[4]uint32{0xDEADBEEF, 0xFFFFFFFF, 0x7F800001, 16777217}, -123456789},
	{[4]uint32{0x01000003, 0x7FC00000, 0xFF800001, 4000000000}, 2147483647},
	{[4]uint32{0x12345679, 0x80000001, 0x7F800000, 0xFFFFFFFE}, -2147483648},
	{[4]uint32{0x89ABCDEF, 0x7FFFFFFF, 0xFF7FFFFF, 0x01000001}, 16777219},
	{[4]uint32{0x0BADF00D, 0xFFC00001, 0x00800001, 0x7F7FFFFF}, -16777217},
	{[4]uint32{0xCAFEBABE, 0x80000000, 0xFFFFFFF0, 0x01000005}, 1},
}

// want returns the texel the fragment shader writes for record i, the
// provoking vertex of its triangle.
func (r flatVaryingRecord) want(i int) [4]uint32 {
	return [4]uint32{r.u[0], r.u[1], r.u[2] ^ uint32(r.s), r.u[3] ^ (0xA5000000 + uint32(i))}
}

// TestFlatIntegerVaryings draws a full-target quad as two triangles whose
// vertices carry different integer attributes, and checks that every pixel
// holds the exact words of its triangle's provoking vertex: the first vertex
// of the primitive (WGSL @interpolate(flat) defaults to sampling "first").
//
// On the 4x4 target the diagonal from (-1,-1) to (1,1) splits the pixels:
// x < y lies in the first triangle and x > y in the second. Pixels on the
// diagonal are left out, as either triangle may own them.
//
// The software backend held integer lanes as float32 (float32(int) on the way
// in, interpolated across the triangle), dropped scalar u32 varyings to 0,
// and took the third vertex of an odd triangle-strip triangle as the
// provoking one (P1-SW-019).
func TestFlatIntegerVaryings(t *testing.T) {
	positionsList := []float32{-1, -1, 1, -1, -1, 1, -1, 1, 1, -1, 1, 1}
	positionsStrip := []float32{-1, -1, 1, -1, -1, 1, 1, 1}
	cases := []struct {
		name      string
		topology  gputypes.PrimitiveTopology
		positions []float32
		vertices  uint32
		lower     int // record index of the provoking vertex for x < y
		upper     int // and for x > y
	}{
		// Triangles (0, 1, 2) and (3, 4, 5).
		{"list", gputypes.PrimitiveTopologyTriangleList, positionsList, 6, 0, 3},
		// Triangles (0, 1, 2) and (1, 2, 3): the second starts at vertex 1.
		{"strip", gputypes.PrimitiveTopologyTriangleStrip, positionsStrip, 4, 0, 1},
	}
	forEachBackendAdapter(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				texels := drawFlatVaryings(t, device, c.topology, c.positions, c.vertices)
				for y := range colorRigSize {
					for x := range colorRigSize {
						if x == y {
							continue
						}
						rec := c.upper
						if x < y {
							rec = c.lower
						}
						want := flatVaryingRecords[rec].want(rec)
						texel := texels[y*colorRigSize+x]
						var got [4]uint32
						for i := range got {
							got[i] = binary.LittleEndian.Uint32(texel[4*i:])
						}
						if got != want {
							t.Errorf("pixel (%d,%d) = %#08x, want %#08x (vertex %d)", x, y, got, want, rec)
						}
					}
				}
			})
		}
	})
}

// drawFlatVaryings draws vertices of the flat-varying scene with topology into
// a cleared RGBA32Uint target and returns its texels.
func drawFlatVaryings(t *testing.T, device *wgpu.Device, topology gputypes.PrimitiveTopology, positions []float32, vertices uint32) [][]byte {
	t.Helper()
	tex, view := newColorTexture(t, device, gputypes.TextureFormatRGBA32Uint, nil,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "flat-varyings", WGSL: flatVaryingShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "flat-varyings"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "flat-varyings",
		Layout: layout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs",
			Buffers: []gputypes.VertexBufferLayout{
				{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0},
				}},
				{ArrayStride: 20, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatUint32x4, ShaderLocation: 1},
					{Format: gputypes.VertexFormatSint32, Offset: 16, ShaderLocation: 2},
				}},
			},
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA32Uint, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: topology},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	words := make([]uint32, 0, 5*len(flatVaryingRecords))
	for _, r := range flatVaryingRecords {
		words = append(words, r.u[0], r.u[1], r.u[2], r.u[3], uint32(r.s))
	}
	positionBuf := newVertexBuffer(t, device, positions)
	attrBuf := newDrawArgsBuffer(t, device, wgpu.BufferUsageVertex, words)

	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View:    view,
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		pass.SetVertexBuffer(0, positionBuf, 0)
		pass.SetVertexBuffer(1, attrBuf, 0)
		pass.Draw(gputypes.DrawArgs{VertexCount: vertices, InstanceCount: 1})
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	})
	return readColorTexels(t, device, tex, 16)
}
