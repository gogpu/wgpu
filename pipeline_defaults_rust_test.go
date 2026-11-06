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

// TestConvertTextureDescZeroDefaults: wgpu-native takes the texture
// descriptor literally, so zero MipLevelCount, SampleCount, Dimension,
// Size.Height and Size.DepthOrArrayLayers must arrive as the WebGPU defaults
// (1, 1, 2d, 1, 1); explicit values pass unchanged.
func TestConvertTextureDescZeroDefaults(t *testing.T) {
	r := convertTextureDesc(&TextureDescriptor{Size: Extent3D{Width: 16}, Format: gputypes.TextureFormatRGBA8Unorm})
	if want := (rwgpu.Extent3D{Width: 16, Height: 1, DepthOrArrayLayers: 1}); r.Size != want {
		t.Errorf("wgpu-native size %+v, want %+v", r.Size, want)
	}
	if r.MipLevelCount != 1 || r.SampleCount != 1 || r.Dimension != gputypes.TextureDimension2D {
		t.Errorf("wgpu-native mips %d, samples %d, dimension %v, want 1, 1, 2d", r.MipLevelCount, r.SampleCount, r.Dimension)
	}
	r = convertTextureDesc(&TextureDescriptor{Size: Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 2}, MipLevelCount: 3, SampleCount: 4, Dimension: gputypes.TextureDimension3D})
	if r.Size.DepthOrArrayLayers != 2 || r.MipLevelCount != 3 || r.SampleCount != 4 || r.Dimension != gputypes.TextureDimension3D {
		t.Errorf("explicit values changed: %+v", r)
	}
}

// TestConvertRenderPipelineDescSampleMask: wgpu-native takes the sample mask
// literally, so a zero mask must arrive as every sample.
func TestConvertRenderPipelineDescSampleMask(t *testing.T) {
	for _, c := range []struct {
		in   uint64
		want uint32
	}{{0, 0xFFFFFFFF}, {0x1, 0x1}, {0xFFFFFFFF, 0xFFFFFFFF}} {
		r := convertRenderPipelineDesc(&RenderPipelineDescriptor{Multisample: gputypes.MultisampleState{Count: 1, Mask: c.in}})
		if r.Multisample.Mask != c.want {
			t.Errorf("Mask %#x: wgpu-native %#x, want %#x", c.in, r.Multisample.Mask, c.want)
		}
	}
}
