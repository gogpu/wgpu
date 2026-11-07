//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// subRenderWGSL fills the left half of the target with green.
const subRenderWGSL = `
@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
    var p = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(0.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(-1.0, 1.0), vec2<f32>(0.0, -1.0), vec2<f32>(0.0, 1.0));
    return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

// TestRenderIntoLayerAndMipViews renders, clear red plus a green left half,
// into a view of array layer 1 at mip level 0 and into a view of mip level
// 1 of layer 0, with a depth attachment on layer 1 of a depth array cleared
// to 0.25: each pass must cover its own level's extent and write only its
// own subresource. The old backend rendered every view into layer 0 of mip
// level 0, and a view above mip level 0 into nothing.
func TestRenderIntoLayerAndMipViews(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	const w, h = 8, 4
	tex := newSubTexture(t, dev, gputypes.TextureDimension2D, w, h, 2, 2, gputypes.TextureFormatRGBA8Unorm)
	for level := range uint32(2) {
		lw, lh, n := subExtent(gputypes.TextureDimension2D, w, h, 2, level)
		fillLevel(t, q, tex, level, 0, lw, lh, n, func(_, _, _ uint32) [4]byte { return subSentinel })
	}
	depth := newSubTexture(t, dev, gputypes.TextureDimension2D, w, h, 2, 1, gputypes.TextureFormatDepth32Float)
	depthSentinel := make([]byte, w*h*2*4)
	for i := 0; i < len(depthSentinel); i += 4 {
		binary.LittleEndian.PutUint32(depthSentinel[i:], math.Float32bits(0.75))
	}
	if err := q.WriteTexture(&hal.ImageCopyTexture{Texture: depth}, depthSentinel,
		&hal.ImageDataLayout{BytesPerRow: w * 4, RowsPerImage: h}, &hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 2}); err != nil {
		t.Fatalf("WriteTexture depth: %v", err)
	}

	pipeline := newShaderPipeline(t, dev, subRenderWGSL, gputypes.TextureFormatRGBA8Unorm, nil)
	layer1 := newSubView(t, dev, tex, hal.TextureViewDescriptor{
		Dimension: gputypes.TextureViewDimension2D, BaseArrayLayer: 1, ArrayLayerCount: 1, MipLevelCount: 1,
	})
	mip1 := newSubView(t, dev, tex, hal.TextureViewDescriptor{
		Dimension: gputypes.TextureViewDimension2D, BaseMipLevel: 1, MipLevelCount: 1, ArrayLayerCount: 1,
	})
	depthLayer1 := newSubView(t, dev, depth, hal.TextureViewDescriptor{
		Dimension: gputypes.TextureViewDimension2D, BaseArrayLayer: 1, ArrayLayerCount: 1,
	})
	for _, v := range []struct {
		view  hal.TextureView
		depth hal.TextureView
	}{{layer1, depthLayer1}, {mip1, nil}} {
		enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		desc := &hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{{
			View: v.view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, A: 1},
		}}}
		if v.depth != nil {
			desc.DepthStencilAttachment = &hal.RenderPassDepthStencilAttachment{
				View: v.depth, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 0.25,
			}
		}
		pass := enc.BeginRenderPass(desc)
		pass.SetPipeline(pipeline)
		pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		pass.End()
		submit(t, enc)
	}

	halfGreen := func(lw uint32) func(x, y uint32) [4]byte {
		return func(x, _ uint32) [4]byte {
			if x < lw/2 {
				return [4]byte{0, 255, 0, 255}
			}
			return [4]byte{255, 0, 0, 255}
		}
	}
	checkImage(t, readImage(t, dev, tex, 0, 1, w, h, 4), 0, 1, w, h, halfGreen(w))
	checkImage(t, readImage(t, dev, tex, 1, 0, w/2, h/2, 4), 1, 0, w/2, h/2, halfGreen(w/2))
	checkImage(t, readImage(t, dev, tex, 0, 0, w, h, 4), 0, 0, w, h, func(_, _ uint32) [4]byte { return subSentinel })
	checkImage(t, readImage(t, dev, tex, 1, 1, w/2, h/2, 4), 1, 1, w/2, h/2, func(_, _ uint32) [4]byte { return subSentinel })

	for z, want := range []float32{0.75, 0.25} {
		data := readImage(t, dev, depth, 0, uint32(z), w, h, 4)
		for i := 0; i < len(data); i += 4 {
			if got := math.Float32frombits(binary.LittleEndian.Uint32(data[i:])); got != want {
				t.Fatalf("depth layer %d texel %d = %v, want %v", z, i/4, got, want)
			}
		}
	}
}
