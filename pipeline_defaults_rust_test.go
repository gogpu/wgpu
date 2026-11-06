//go:build rust

package wgpu

import (
	"testing"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
)

// TestConvertRenderPipelineDescSpecDefaults checks that the -tags rust
// conversion sends wgpu-native the WebGPU defaults for zero stencil faces and
// zero blend components, as the native build sends them to the HAL, and that
// the caller's descriptor is unchanged.
func TestConvertRenderPipelineDescSpecDefaults(t *testing.T) {
	blend := &gputypes.BlendState{}
	desc := &RenderPipelineDescriptor{
		DepthStencil: &DepthStencilState{
			Format:            gputypes.TextureFormatDepth24PlusStencil8,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionLess,
		},
		Fragment: &FragmentState{
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, Blend: blend, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	}

	r := convertRenderPipelineDesc(desc)

	wantFace := rwgpu.StencilFaceState{
		Compare: gputypes.CompareFunctionAlways,
		FailOp:  gputypes.StencilOperationKeep, DepthFailOp: gputypes.StencilOperationKeep, PassOp: gputypes.StencilOperationKeep,
	}
	if r.DepthStencil.StencilFront != wantFace || r.DepthStencil.StencilBack != wantFace {
		t.Errorf("wgpu-native stencil faces %+v, %+v, want %+v", r.DepthStencil.StencilFront, r.DepthStencil.StencilBack, wantFace)
	}
	if r.DepthStencil.DepthCompare != gputypes.CompareFunctionLess {
		t.Errorf("wgpu-native depth compare %v, want the explicit Less", r.DepthStencil.DepthCompare)
	}
	wantBlend := rwgpu.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd}
	if b := r.Fragment.Targets[0].Blend; b == nil || b.Color != wantBlend || b.Alpha != wantBlend {
		t.Errorf("wgpu-native blend %+v, want color and alpha %+v", b, wantBlend)
	}
	if *blend != (gputypes.BlendState{}) || desc.DepthStencil.StencilFront != (StencilFaceState{}) {
		t.Error("the conversion changed the caller's descriptor")
	}
}
