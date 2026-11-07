//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// drawCullCase draws, in one draw call into an 8x8 target, a triangle that is
// counter-clockwise in NDC (left half) and one that is clockwise in NDC
// (right half), with the given cull mode and front face, and reports which of
// them were drawn. Per the WebGPU spec the facing is decided in framebuffer
// coordinates (y down), where an NDC-counter-clockwise triangle has a positive
// spec area, i.e. it is front-facing for FrontFaceCCW.
func drawCullCase(t *testing.T, cull gputypes.CullMode, front gputypes.FrontFace) (left, right bool) {
	t.Helper()
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	verts := []float32{
		// Left, counter-clockwise in NDC.
		-1, -1, 0,
		0, -1, 0,
		-1, 1, 0,
		// Right, clockwise in NDC.
		1, -1, 0,
		0, -1, 0,
		1, 1, 0,
	}
	const stride = 12
	vbData := make([]byte, len(verts)*4)
	for i, v := range verts {
		writeFloat32(vbData, i*4, v)
	}
	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "cull",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: stride,
				StepMode:    gputypes.VertexStepModeVertex,
				Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
				},
			}},
		},
		Primitive: gputypes.PrimitiveState{
			Topology:  gputypes.PrimitiveTopologyTriangleList,
			FrontFace: front,
			CullMode:  cull,
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:       dstView,
			LoadOp:     gputypes.LoadOpClear,
			ClearValue: gputypes.Color{},
		}},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, 0)
	pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	pass.End()
	submit(t, enc)

	data := dstTex.(*Texture).GetData()
	drawn := func(x, y int) bool { return data[(y*8+x)*4] != 0 }
	return drawn(1, 4), drawn(6, 4)
}

// TestDrawCullModeAndFrontFace is the P1-SW-014 regression: the software
// backend ignored PrimitiveState.CullMode/FrontFace (every triangle was
// drawn), and its winding test had the sign of the WebGPU spec area inverted,
// so wiring the setters alone would have culled the wrong face.
func TestDrawCullModeAndFrontFace(t *testing.T) {
	tests := []struct {
		name        string
		cull        gputypes.CullMode
		front       gputypes.FrontFace
		left, right bool
	}{
		{"none", gputypes.CullModeNone, gputypes.FrontFaceCCW, true, true},
		{"back-ccw", gputypes.CullModeBack, gputypes.FrontFaceCCW, true, false},
		{"front-ccw", gputypes.CullModeFront, gputypes.FrontFaceCCW, false, true},
		{"back-cw", gputypes.CullModeBack, gputypes.FrontFaceCW, false, true},
		{"front-cw", gputypes.CullModeFront, gputypes.FrontFaceCW, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, right := drawCullCase(t, tt.cull, tt.front)
			if left != tt.left || right != tt.right {
				t.Errorf("drawn (left CCW, right CW) = (%v, %v), want (%v, %v)", left, right, tt.left, tt.right)
			}
		})
	}
}
