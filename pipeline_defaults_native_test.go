//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestRenderPipelineToHALSpecDefaults checks that the native conversion hands
// the HAL the WebGPU defaults for zero stencil faces and zero blend
// components. The HAL takes these values literally: DX12 enabled the stencil
// test with compare NEVER for a zero face and drew nothing.
func TestRenderPipelineToHALSpecDefaults(t *testing.T) {
	desc := &RenderPipelineDescriptor{
		DepthStencil: &DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionLess,
		},
		Fragment: &FragmentState{
			Module: &ShaderModule{},
			Targets: []gputypes.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				Blend:     &gputypes.BlendState{},
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
	}

	h := desc.toHAL()

	wantFace := StencilFaceState{
		Compare: gputypes.CompareFunctionAlways,
		FailOp:  gputypes.StencilOperationKeep, DepthFailOp: gputypes.StencilOperationKeep, PassOp: gputypes.StencilOperationKeep,
	}.toHAL()
	if h.DepthStencil.StencilFront != wantFace || h.DepthStencil.StencilBack != wantFace {
		t.Errorf("HAL stencil faces %+v, %+v, want %+v", h.DepthStencil.StencilFront, h.DepthStencil.StencilBack, wantFace)
	}
	if h.DepthStencil.DepthCompare != gputypes.CompareFunctionLess {
		t.Errorf("HAL depth compare %v, want the explicit Less", h.DepthStencil.DepthCompare)
	}
	wantBlend := gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd}
	if b := h.Fragment.Targets[0].Blend; b == nil || b.Color != wantBlend || b.Alpha != wantBlend {
		t.Errorf("HAL blend %+v, want color and alpha %+v", b, wantBlend)
	}
	if *desc.Fragment.Targets[0].Blend != (gputypes.BlendState{}) {
		t.Error("toHAL changed the caller's blend state")
	}
}

// TestRenderPipelineToHALSampleMask: a zero sample mask reaches the HAL as
// every sample; any other mask unchanged.
func TestRenderPipelineToHALSampleMask(t *testing.T) {
	for _, c := range []struct{ in, want uint64 }{{0, 0xFFFFFFFF}, {0x1, 0x1}, {0xFFFFFFFF, 0xFFFFFFFF}} {
		desc := &RenderPipelineDescriptor{Multisample: gputypes.MultisampleState{Count: 1, Mask: c.in}}
		if got := desc.toHAL().Multisample.Mask; got != c.want {
			t.Errorf("Mask %#x: HAL %#x, want %#x", c.in, got, c.want)
		}
	}
}
