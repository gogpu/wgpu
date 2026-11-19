//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// BenchmarkManySmallQuads720p measures the shape of a UI compositing many
// small elements: a 1280x720 pass of 512 draws, each a 16x16 quad placed by
// its own SetViewport. Every draw rasterizes through a pipeline of the pass's
// extent, so this reports what a draw's target buffers cost (#419).
func BenchmarkManySmallQuads720p(b *testing.B) {
	const (
		w, h = 1280, 720
		quad = 16 // the on-screen size of each quad, set by its viewport
		cols = 32
		rows = 16 // cols*rows = 512 draws
	)

	backend := NewBackend()
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()
	dev := openDev.Device.(*Device)

	_, srcView := newShaderTexture(b, dev, w, h, gputypes.TextureFormatRGBA8Unorm, [4]byte{200, 50, 10, 255})
	_, dstView := newShaderTexture(b, dev, w, h, gputypes.TextureFormatBGRA8Unorm, [4]byte{})

	// One quad covering the whole screen; each draw's viewport clips it to its
	// 16x16 cell.
	uniforms := make([]byte, 32)
	for i, f := range []float32{0, 0, w, h, w, h, 1, 1} { // rect, screen, alpha, premultiplied
		binary.LittleEndian.PutUint32(uniforms[i*4:], math.Float32bits(f))
	}
	ubuf, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: 32, Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst})
	if err != nil {
		b.Fatalf("CreateBuffer: %v", err)
	}
	ubuf.(*Buffer).WriteData(0, uniforms)
	samp, err := dev.CreateSampler(&hal.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
		MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear,
	})
	if err != nil {
		b.Fatalf("CreateSampler: %v", err)
	}
	srcOver := &gputypes.BlendState{
		Color: gputypes.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha},
		Alpha: gputypes.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha},
	}
	pipeline := newShaderPipeline(b, dev, positionedQuadWGSL, gputypes.TextureFormatBGRA8Unorm, srcOver)
	uniformGroup := newShaderBindGroup(b, dev, gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.BufferBinding{Buffer: ubuf.NativeHandle(), Size: 32}})
	texGroup := newShaderBindGroup(b, dev,
		gputypes.BindGroupEntry{Binding: 0, Resource: gputypes.SamplerBinding{Sampler: samp.NativeHandle()}},
		gputypes.BindGroupEntry{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
		if err != nil {
			b.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
			ColorAttachments: []hal.RenderPassColorAttachment{{
				View: dstView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{A: 1},
			}},
		})
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, uniformGroup, nil)
		pass.SetBindGroup(1, texGroup, nil)
		for i := 0; i < cols*rows; i++ {
			pass.SetViewport(gputypes.Viewport{
				X:        float32(i%cols) * (w / cols),
				Y:        float32(i/cols) * (h / rows),
				Width:    quad,
				Height:   quad,
				MinDepth: 0,
				MaxDepth: 1,
			})
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		}
		pass.End()
		submit(b, enc)
	}
}
