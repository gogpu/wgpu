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

// These tests check WGSL's discard semantics beyond "the fragment writes no
// color" (TestFragmentDiscard): discard turns the invocation into a helper
// invocation (WGSL "Discard Statement"). Statements before it keep their
// effects; a helper writes nothing to storage ("Fragment Shaders and Helper
// Invocations"); and the quad keeps computing derivatives. Vulkan gets these
// semantics from OpDemoteToHelperInvocation where the device enables
// shaderDemoteToHelperInvocation (P2-VK-018). With OpKill the first two hold
// as well, and the derivative after a neighbor's discard is undefined, so no
// readback can prove OpKill wrong; the derivative case asserts what WGSL
// requires.

// discardStorageWGSL draws colorRigSize² pixels; every fragment marks its
// pixel in the "before" half of marks, fragments in odd columns then discard,
// and the rest mark the "after" half.
const discardStorageWGSL = `
@group(0) @binding(0) var<storage, read_write> marks: array<u32, 32>;

struct VOut {
    @builtin(position) pos: vec4<f32>,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0));
    return VOut(vec4<f32>(p[i], 0.0, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let x = u32(in.pos.x);
    let y = u32(in.pos.y);
    let pixel = y * 4u + x;
    marks[pixel] = pixel + 1u;
    if (x & 1u) == 1u {
        discard;
    }
    marks[16u + pixel] = pixel + 1u;
    return vec4<f32>(1.0, 1.0, 1.0, 1.0);
}
`

const discardStorageFill = 0xDEADBEEF

// TestDiscardStorageWrites checks that a store before discard lands and a
// store after it does not: marks[p] = p+1 for every pixel, marks[16+p] = p+1
// only for even columns, the sentinel elsewhere.
func TestDiscardStorageWrites(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsFragmentStorage, func(t *testing.T, device *wgpu.Device) {
		const words = 32
		marks := newPublicBuffer(t, device, words*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
		fill := make([]uint32, words)
		for i := range fill {
			fill[i] = discardStorageFill
		}
		writeWords(t, device, marks, fill...)
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}})
		group, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: marks, Size: words * 4},
		}})
		if err != nil {
			t.Fatalf("CreateBindGroup: %v", err)
		}
		t.Cleanup(group.Release)

		_, view := newColorTexture(t, device, gputypes.TextureFormatRGBA8Unorm, nil, wgpu.TextureUsageRenderAttachment)
		colorPass{
			view:   view,
			wgsl:   discardStorageWGSL,
			state:  gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			layout: bgl,
			group:  group,
		}.run(t, device)

		staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
		enc := newEncoder(t, device)
		enc.CopyBufferToBuffer(marks, 0, staging, 0, words*4)
		submit(t, device, enc)

		want := make([]uint32, words)
		for p := range 16 {
			want[p] = uint32(p + 1)
			want[16+p] = discardStorageFill
			if p%colorRigSize%2 == 0 {
				want[16+p] = uint32(p + 1)
			}
		}
		checkWords(t, "marks (before discard, after discard)", readPublicBuffer(t, staging, words), want)
	})
}

// TestDerivativesAfterDiscard discards the bottom-right lane of every 2x2
// quad and takes fine derivatives of x*x and y*y in the other three after
// it. The discarded lane still runs as a helper invocation, so each
// derivative is the exact difference between the quad's two columns or rows:
// 2*xq+2 and 2*yq+2 for the quad's even corner (xq, yq), as in
// TestQuadDerivatives. Discarded pixels keep the clear color.
func TestDerivativesAfterDiscard(t *testing.T) {
	body := "vec4<f32>(dpdxFine(x * x), dpdyFine(y * y), 1.0, 1.0)"
	wgsl := derivativeShaderAfterDiscard(body)
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		tex, view := newColorTexture(t, device, gputypes.TextureFormatRGBA32Float, nil,
			wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
		colorPass{
			view:  view,
			wgsl:  wgsl,
			state: gputypes.ColorTargetState{Format: gputypes.TextureFormatRGBA32Float, WriteMask: gputypes.ColorWriteMaskAll},
			clear: gputypes.Color{R: -1, G: -1, B: -1, A: -1},
		}.run(t, device)
		texels := readColorTexels(t, device, tex, 16)
		for y := range colorRigSize {
			for x := range colorRigSize {
				got := decodeRGBA32F(texels[y*colorRigSize+x])
				want := [4]float64{float64(2*(x&^1) + 2), float64(2*(y&^1) + 2), 1, 1}
				if x%2 == 1 && y%2 == 1 {
					want = [4]float64{-1, -1, -1, -1}
				}
				for ch := range 4 {
					if math.Abs(got[ch]-want[ch]) > derivativeTolerance {
						t.Errorf("pixel (%d, %d) channel %d = %g, want %g; pixel %v", x, y, ch, got[ch], want[ch], got)
					}
				}
			}
		}
	})
}

// derivativeShaderAfterDiscard is derivativeShader with a discard of the
// bottom-right lane of each quad before the return expression body.
func derivativeShaderAfterDiscard(body string) string {
	return `
struct VOut {
    @builtin(position) pos: vec4<f32>,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0));
    return VOut(vec4<f32>(p[i], 0.0, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let x = in.pos.x;
    let y = in.pos.y;
    if (u32(x) & 1u) == 1u && (u32(y) & 1u) == 1u {
        discard;
    }
    return ` + body + `;
}
`
}
