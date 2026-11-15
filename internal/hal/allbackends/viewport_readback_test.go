// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The viewport tests draw a quad that covers all of NDC and check that the
// viewport maps it (index.bs "Rasterization"):
//
//	framebufferCoords = (vp.x + 0.5*(ndc.x+1)*vp.width, vp.y + 0.5*(1-ndc.y)*vp.height)
//	depth             = vp.minDepth + ndc.z*(vp.maxDepth - vp.minDepth)
//
// so it covers exactly the viewport rectangle (clipped to the target and the
// scissor), its varyings run across the viewport, not the target, and its
// depth lies in the viewport's depth range. The software backend recorded
// SetViewport and never read it: it mapped every draw to the whole target
// with depth 0..1 (P1-SW-036).

const viewportShader = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) uv: vec2f,
}

fn corner(i: u32) -> vec2f {
    var c = array<vec2f, 4>(vec2f(-1.0, -1.0), vec2f(1.0, -1.0), vec2f(-1.0, 1.0), vec2f(1.0, 1.0));
    return c[i];
}

// ndc.z runs from 0 at the left edge to 1 at the right, so the depth range
// mapping shows in every column.
fn vout(p: vec2f) -> VOut {
    return VOut(vec4f(p, p.x * 0.5 + 0.5, 1.0), p * 0.5 + vec2f(0.5));
}

@vertex
fn vs_list(@builtin(vertex_index) i: u32) -> VOut {
    var order = array<u32, 6>(0u, 1u, 2u, 2u, 1u, 3u);
    return vout(corner(order[i]));
}

@vertex
fn vs_corner(@builtin(vertex_index) i: u32) -> VOut {
    return vout(corner(i));
}

@vertex
fn vs_buffer(@location(0) p: vec2f) -> VOut {
    return vout(p);
}

@fragment
fn fs_frag(in: VOut) -> @location(0) vec4f {
    return vec4f(in.pos.xyz, in.uv.x);
}

@fragment
fn fs_quad(in: VOut) -> @location(0) vec4f {
    return vec4f(in.pos.xy, dpdxFine(in.uv.x), dpdyFine(in.uv.y));
}

struct MOut {
    @location(0) a: vec4f,
    @location(1) b: vec4f,
}

@fragment
fn fs_mrt(in: VOut) -> MOut {
    return MOut(vec4f(in.pos.xyz, in.uv.x), vec4f(in.uv, 0.5, 1.0));
}

@fragment
fn fs_const() -> @location(0) vec4f {
    return vec4f(0.5, 0.25, 0.125, 1.0);
}
`

// viewportTolerance bounds the difference from the exact value: positions,
// depths and varyings of an 8-pixel target with w = 1 are exact to well under
// 1e-4 on every backend.
const viewportTolerance = 1e-4

// viewportCase is one draw of the quad.
type viewportCase struct {
	name    string
	vp      gputypes.Viewport
	scissor *gputypes.ScissorRect
	fs      string // fragment entry point; "" draws depth only
	targets int
	draw    string // "draw", "indexed", "indirect" or "vertex-buffer"
}

var (
	// viewportOffset lies inside the target, away from every edge.
	viewportOffset = gputypes.Viewport{X: 2, Y: 1, Width: 4, Height: 6, MinDepth: 0.25, MaxDepth: 0.75}
	// viewportOutside extends past the target's left and bottom edges.
	viewportOutside = gputypes.Viewport{X: -2, Y: 3, Width: 8, Height: 8, MinDepth: 0, MaxDepth: 0.5}
)

// TestViewportMapping checks the viewport in every draw path of the software
// backend: a per-fragment shader, a shader with derivatives (quads), multiple
// render targets, a constant color, a depth-only pass, indexed and vertex
// buffer draws, a viewport cut by a scissor and one partly outside the target.
func TestViewportMapping(t *testing.T) {
	cases := []viewportCase{
		{name: "fragment", vp: viewportOffset, fs: "fs_frag", targets: 1, draw: "draw"},
		{name: "quad", vp: viewportOffset, fs: "fs_quad", targets: 1, draw: "draw"},
		{name: "mrt", vp: viewportOffset, fs: "fs_mrt", targets: 2, draw: "draw"},
		{name: "constant", vp: viewportOffset, fs: "fs_const", targets: 1, draw: "draw"},
		{name: "depth-only", vp: viewportOffset, draw: "draw"},
		{name: "indexed", vp: viewportOffset, fs: "fs_frag", targets: 1, draw: "indexed"},
		{name: "vertex-buffer", vp: viewportOffset, fs: "fs_frag", targets: 1, draw: "vertex-buffer"},
		{name: "scissor", vp: viewportOffset, scissor: &gputypes.ScissorRect{X: 0, Y: 0, Width: 4, Height: 8},
			fs: "fs_frag", targets: 1, draw: "draw"},
		{name: "outside", vp: viewportOutside, fs: "fs_frag", targets: 1, draw: "draw"},
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) { runViewportCase(t, device, c) })
		}
	})
}

// TestViewportMappingIndirect is TestViewportMapping's indirect draw, which
// needs IndirectExecution.
func TestViewportMappingIndirect(t *testing.T) {
	c := viewportCase{name: "indirect", vp: viewportOffset, fs: "fs_frag", targets: 1, draw: "indirect"}
	forEachOverrideBackend(t, gputypes.DownlevelFlagsIndirectExecution, func(t *testing.T, device *wgpu.Device) {
		runViewportCase(t, device, c)
	})
}

func runViewportCase(t *testing.T, device *wgpu.Device, c viewportCase) {
	t.Helper()
	scene := rigScene{size: 8, wgsl: viewportShader, vsEntry: "vs_list", fsEntry: c.fs, targets: c.targets}
	setState := func(pass *wgpu.RenderPassEncoder) {
		pass.SetViewport(c.vp)
		if c.scissor != nil {
			pass.SetScissorRect(*c.scissor)
		}
	}
	switch c.draw {
	case "draw":
		scene.draw = func(_ *testing.T, _ *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			setState(pass)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		}
	case "indexed":
		scene.vsEntry = "vs_corner"
		scene.draw = func(t *testing.T, device *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			setState(pass)
			pass.SetIndexBuffer(newDrawArgsBuffer(t, device, wgpu.BufferUsageIndex, []uint32{0, 1, 2, 2, 1, 3}),
				gputypes.IndexFormatUint32, 0)
			pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 6, InstanceCount: 1})
		}
	case "indirect":
		scene.draw = func(t *testing.T, device *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			setState(pass)
			pass.DrawIndirect(newDrawArgsBuffer(t, device, wgpu.BufferUsageIndirect, []uint32{6, 1, 0, 0}), 0)
		}
	case "vertex-buffer":
		scene.vsEntry = "vs_buffer"
		scene.buffers = []gputypes.VertexBufferLayout{{
			ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0}},
		}}
		scene.draw = func(t *testing.T, device *wgpu.Device, pass *wgpu.RenderPassEncoder) {
			setState(pass)
			pass.SetVertexBuffer(0, newVertexBuffer(t, device, []float32{-1, -1, 1, -1, -1, 1, -1, 1, 1, -1, 1, 1}), 0)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		}
	default:
		t.Fatalf("unknown draw %q", c.draw)
	}
	img := scene.run(t, device)
	background := [4]float64{rigClearColor, rigClearColor, rigClearColor, rigClearColor}
	vp := c.vp
	for y := range scene.size {
		for x := range scene.size {
			px, py := float64(x)+0.5, float64(y)+0.5
			inside := px >= float64(vp.X) && px < float64(vp.X+vp.Width) &&
				py >= float64(vp.Y) && py < float64(vp.Y+vp.Height)
			if s := c.scissor; s != nil {
				inside = inside && x >= int(s.X) && x < int(s.X+s.Width) && y >= int(s.Y) && y < int(s.Y+s.Height)
			}
			if !inside {
				for i := range c.targets {
					checkTexel(t, "color", x, y, img.at(i, x, y), background, 0)
				}
				checkDepth(t, x, y, img.depthAt(x, y), 1, 0)
				continue
			}
			u := (px - float64(vp.X)) / float64(vp.Width)
			v := 1 - (py-float64(vp.Y))/float64(vp.Height)
			depth := float64(vp.MinDepth) + u*float64(vp.MaxDepth-vp.MinDepth)
			checkDepth(t, x, y, img.depthAt(x, y), depth, viewportTolerance)
			switch c.fs {
			case "fs_frag":
				checkTexel(t, "color", x, y, img.at(0, x, y), [4]float64{px, py, depth, u}, viewportTolerance)
			case "fs_quad":
				want := [4]float64{px, py, 1 / float64(vp.Width), -1 / float64(vp.Height)}
				checkTexel(t, "color", x, y, img.at(0, x, y), want, viewportTolerance)
			case "fs_mrt":
				checkTexel(t, "color 0", x, y, img.at(0, x, y), [4]float64{px, py, depth, u}, viewportTolerance)
				checkTexel(t, "color 1", x, y, img.at(1, x, y), [4]float64{u, v, 0.5, 1}, viewportTolerance)
			case "fs_const":
				checkTexel(t, "color", x, y, img.at(0, x, y), [4]float64{0.5, 0.25, 0.125, 1}, 0)
			}
		}
	}
}
