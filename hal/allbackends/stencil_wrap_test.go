// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The stencil wrap tests pin the increment-wrap and decrement-wrap operations
// and the way the stencil write mask applies to their result on the combined
// Depth24PlusStencil8 format: the parity behavior gg's EvenOdd stroke
// construct builds on.
//
// Expected values, from the specs:
//
//   - index.bs "GPUStencilOperation": "increment-wrap" "Increments the current
//     stencil value, wrapping to zero if the value exceeds the maximum
//     representable value of the ... stencil aspect"; "decrement-wrap"
//     "Decrement the current stencil value, wrapping to the maximum
//     representable value of the ... stencil aspect if the value goes below
//     0". The stencil aspect is 8 bits, so the maximum is 255.
//   - index.bs "GPUDepthStencilState": "stencilWriteMask: Bitmask controlling
//     which ... stencil value bits are written to when performing stencil
//     operations", and "stencilFront"/"stencilBack" apply to front- and
//     back-facing primitives.
//   - Vulkan, VkStencilOp: VK_STENCIL_OP_INCREMENT_AND_WRAP "increments the
//     current value and wraps to 0 when the maximum value would have been
//     exceeded"; VK_STENCIL_OP_DECREMENT_AND_WRAP "decrements the current
//     value and wraps to the maximum possible value when the value would go
//     below 0"; "For purposes of increment and decrement, the stencil bits are
//     considered as an unsigned integer."
//   - Vulkan, "Depth/Stencil Operations" (Stencil Test): if the stencil test
//     passes and the depth test passes, passOp is used; the result is stored as
//     sa = (sa & ~sw) | (sg & sw), where sw is the face's writeMask.
//   - D3D12, D3D12_STENCIL_OP: D3D12_STENCIL_OP_INCR "Increment the stencil
//     value by 1, and wrap the result if necessary"; D3D12_STENCIL_OP_DECR
//     "Decrement the stencil value by 1, and wrap the result if necessary";
//     D3D12_DEPTH_STENCILOP_DESC::StencilWriteMask masks the result.
//   - index.bs "GPUStencilOperation"/"GPUFrontFace"/"Polygon Rasterization":
//     with frontFace "ccw" a polygon whose area in framebuffer coordinates is
//     positive is front-facing; those coordinates start at the top-left corner
//     with y growing down (index.bs "Coordinates").

const (
	stencilWrapSize        = 16
	stencilWrapBytesPerRow = 256 // the copy alignment; the 16 stencil texels of a row fit
)

var stencilWrapExtent = wgpu.Extent3D{Width: stencilWrapSize, Height: stencilWrapSize, DepthOrArrayLayers: 1}

// stencilWrapShader draws one triangle twice: vertices 0-2 are front-facing,
// 3-5 the same triangle in the reverse order, so back-facing with frontFace
// "ccw". In the 16x16 target it covers framebuffer x < 8 on every row: the
// edge at x = 8 falls between two columns of pixel centers, and the two other
// edges leave the target, so the covered set is exactly the left half.
const stencilWrapShader = `
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    var p = array<vec2f, 6>(
        vec2f(-3.0, 0.0), vec2f(0.0, -3.0), vec2f(0.0, 3.0),
        vec2f(0.0, 3.0), vec2f(0.0, -3.0), vec2f(-3.0, 0.0),
    );
    return vec4f(p[i], 0.0, 1.0);
}
@fragment fn fs() -> @location(0) vec4f { return vec4f(0.0, 1.0, 0.0, 1.0); }
`

// TestStencilWrap draws a triangle into a stencil cleared to a known value and
// reads the stencil aspect back. Every covered texel must hold the value the
// operation produces, and every uncovered one must keep the clear value: a
// backend that runs no stencil operation, or that writes the whole target,
// fails here.
func TestStencilWrap(t *testing.T) {
	forEachOverrideBackend(t, gputypes.DownlevelFlagsDepthTextureAndBufferCopies, func(t *testing.T, device *wgpu.Device) {
		t.Run("increment-wrap-full-mask", func(t *testing.T) {
			// 255 is the maximum of the 8-bit stencil aspect: a front-facing
			// increment wraps it to 0.
			got := drawStencilWrap(t, device, stencilWrapCase{
				clear: 255, passOp: wgpu.StencilOperationIncrementWrap, writeMask: 0xFF,
			})
			checkStencilWrap(t, got, 0, 255)
		})
		t.Run("decrement-wrap-full-mask", func(t *testing.T) {
			// 0 is the minimum: a back-facing decrement wraps it to 255 and
			// stencilBack, not stencilFront, supplies the operation.
			got := drawStencilWrap(t, device, stencilWrapCase{
				clear: 0, passOp: wgpu.StencilOperationDecrementWrap, writeMask: 0xFF, back: true,
			})
			checkStencilWrap(t, got, 255, 0)
		})
		t.Run("increment-wrap-one-bit-mask", func(t *testing.T) {
			t.Run("clear-0", func(t *testing.T) {
				// 0 increments to 1, and bit 0 is the bit the mask keeps.
				got := drawStencilWrap(t, device, stencilWrapCase{
					clear: 0, passOp: wgpu.StencilOperationIncrementWrap, writeMask: 0x01,
				})
				checkStencilWrap(t, got, 1, 0)
			})
			t.Run("clear-1", func(t *testing.T) {
				// 1 increments to 2: the carry lands in bit 1, which the write
				// mask drops, so the stored value is (1 & ^0x01) | (2 & 0x01) = 0.
				got := drawStencilWrap(t, device, stencilWrapCase{
					clear: 1, passOp: wgpu.StencilOperationIncrementWrap, writeMask: 0x01,
				})
				checkStencilWrap(t, got, 0, 1)
			})
		})
		t.Run("decrement-wrap-one-bit-mask", func(t *testing.T) {
			// 0 decrements to 255, whose bit 0 is 1.
			got := drawStencilWrap(t, device, stencilWrapCase{
				clear: 0, passOp: wgpu.StencilOperationDecrementWrap, writeMask: 0x01, back: true,
			})
			checkStencilWrap(t, got, 1, 0)
		})
	})
}

// stencilWrapCovers reports whether stencilWrapShader's triangle covers the
// texel in column x. It covers every row of the left half.
func stencilWrapCovers(x int) bool { return x < stencilWrapSize/2 }

// checkStencilWrap asserts that every covered texel reads op and every
// uncovered texel still holds the value the pass cleared the stencil to.
func checkStencilWrap(t *testing.T, got []byte, op, clearValue byte) {
	t.Helper()
	for y := 0; y < stencilWrapSize; y++ {
		row := got[y*stencilWrapBytesPerRow:]
		for x := 0; x < stencilWrapSize; x++ {
			want := clearValue
			if stencilWrapCovers(x) {
				want = op
			}
			if v := row[x]; v != want {
				t.Fatalf("texel (%d,%d) = %d, want %d (covered = %v, clear = %d, op = %d)",
					x, y, v, want, stencilWrapCovers(x), clearValue, op)
			}
		}
	}
}

// stencilWrapCase is one draw into a stencil target cleared to clear.
type stencilWrapCase struct {
	clear     uint32
	passOp    gputypes.StencilOperation
	writeMask uint32
	back      bool // draw the back-facing copy of the triangle
}

// drawStencilWrap clears the stencil aspect of a new 16x16
// Depth24PlusStencil8 texture to c.clear, draws the triangle with compare
// "always" so that every covered fragment reaches c.passOp, and returns the
// stencil aspect read back through CopyTextureToBuffer. The operation is put
// on stencilFront, or on stencilBack for the back-facing copy, whose other
// face keeps the stencil.
func drawStencilWrap(t *testing.T, device *wgpu.Device, c stencilWrapCase) []byte {
	t.Helper()
	ds, dsView := newStencilWrapTexture(t, device, "stencil-wrap", gputypes.TextureFormatDepth24PlusStencil8,
		wgpu.TextureUsageRenderAttachment|wgpu.TextureUsageCopySrc)
	_, colorView := newStencilWrapTexture(t, device, "stencil-wrap-color", gputypes.TextureFormatRGBA8Unorm,
		wgpu.TextureUsageRenderAttachment)

	keep := wgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionAlways,
		FailOp:  wgpu.StencilOperationKeep, DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationKeep,
	}
	op := keep
	op.PassOp = c.passOp
	front, back := keep, keep
	if c.back {
		back = op
	} else {
		front = op
	}
	pipeline := newStencilWrapPipeline(t, device, front, back, c.writeMask)

	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{A: 1},
			}},
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View:        dsView,
				DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
				StencilLoadOp: gputypes.LoadOpClear, StencilStoreOp: gputypes.StoreOpStore,
				StencilClearValue: c.clear,
			},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		pass.SetPipeline(pipeline)
		args := gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1}
		if c.back {
			args.FirstVertex = 3
		}
		pass.Draw(args)
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
	})
	return readStencilWrap(t, device, ds)
}

// newStencilWrapPipeline draws stencilWrapShader's triangle with compare
// "always" on both faces, the given stencil face states and write mask.
func newStencilWrapPipeline(t *testing.T, device *wgpu.Device, front, back wgpu.StencilFaceState,
	writeMask uint32) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "stencil-wrap", WGSL: stencilWrapShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "stencil-wrap"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "stencil-wrap",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList, FrontFace: gputypes.FrontFaceCCW, CullMode: gputypes.CullModeNone,
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format:          gputypes.TextureFormatDepth24PlusStencil8,
			DepthCompare:    gputypes.CompareFunctionAlways,
			StencilFront:    front,
			StencilBack:     back,
			StencilReadMask: 0xFF, StencilWriteMask: writeMask,
		},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// newStencilWrapTexture creates a 16x16 texture of format with a view.
func newStencilWrapTexture(t *testing.T, device *wgpu.Device, label string, format gputypes.TextureFormat,
	usage gputypes.TextureUsage) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: label, Size: stencilWrapExtent, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: format, Usage: usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture %s: %v", label, err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView %s: %v", label, err)
	}
	t.Cleanup(view.Release)
	return tex, view
}

// readStencilWrap copies the stencil aspect of tex into a buffer at
// stencilWrapBytesPerRow and returns the buffer's bytes.
func readStencilWrap(t *testing.T, device *wgpu.Device, tex *wgpu.Texture) []byte {
	t.Helper()
	bufSize := uint64(stencilWrapBytesPerRow * stencilWrapSize)
	staging := newPublicBuffer(t, device, bufSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: stencilWrapBytesPerRow, RowsPerImage: stencilWrapSize},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectStencilOnly},
			Size:         stencilWrapExtent,
		}})
	})
	return mapStaging(t, staging, bufSize)
}
