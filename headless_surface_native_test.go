//go:build !rust && !(js && wasm) && !android

package wgpu

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
	_ "github.com/gogpu/wgpu/hal/software"
)

const headlessTriangleWGSL = `
@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    if (index == 0u) { return vec4<f32>( 0.0,  0.8, 0.0, 1.0); }
    if (index == 1u) { return vec4<f32>(-0.8, -0.8, 0.0, 1.0); }
    return vec4<f32>(0.8, -0.8, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

type headlessSoftwareFixture struct {
	instance *Instance
	adapter  *Adapter
	device   *Device
	surface  *Surface
	width    uint32
	height   uint32
	format   TextureFormat
}

type emptyPixelReaderSurface struct {
	noop.Surface
}

func (*emptyPixelReaderSurface) ReadPixels() []byte { return nil }

func newHeadlessSoftwareFixture(t *testing.T, width, height uint32, format TextureFormat, configure bool) *headlessSoftwareFixture {
	t.Helper()

	instance, err := CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	surface, err := instance.CreateSurfaceFromTarget(HeadlessSurfaceTarget{})
	if err != nil {
		instance.Release()
		t.Fatalf("CreateSurfaceFromTarget: %v", err)
	}

	adapter, err := instance.RequestAdapter(&RequestAdapterOptions{
		CompatibleSurface:    surface,
		ForceFallbackAdapter: true,
	})
	if err != nil {
		surface.Release()
		instance.Release()
		t.Fatalf("RequestAdapter: %v", err)
	}
	if got := adapter.Info().DeviceType; got != gputypes.DeviceTypeCPU {
		adapter.Release()
		surface.Release()
		instance.Release()
		t.Fatalf("adapter device type = %v, want CPU software adapter", got)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		surface.Release()
		instance.Release()
		t.Fatalf("RequestDevice: %v", err)
	}

	fixture := &headlessSoftwareFixture{
		instance: instance,
		adapter:  adapter,
		device:   device,
		surface:  surface,
		width:    width,
		height:   height,
		format:   format,
	}
	t.Cleanup(func() {
		device.Release()
		surface.Release()
		adapter.Release()
		instance.Release()
	})

	if configure {
		fixture.configure(t)
	}
	return fixture
}

func (f *headlessSoftwareFixture) configure(t *testing.T) {
	t.Helper()
	if err := f.surface.Configure(f.device, &SurfaceConfiguration{
		Width:       f.width,
		Height:      f.height,
		Format:      f.format,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
}

func (f *headlessSoftwareFixture) beginFrame(t *testing.T, clearColor Color) (*SurfaceTexture, *TextureView, *CommandEncoder, *RenderPassEncoder) {
	t.Helper()

	texture, suboptimal, err := f.surface.GetCurrentTexture()
	if err != nil {
		t.Fatalf("GetCurrentTexture: %v", err)
	}
	if suboptimal {
		t.Fatal("headless software surface unexpectedly reported suboptimal")
	}
	view, err := texture.CreateView(nil)
	if err != nil {
		f.surface.DiscardTexture()
		t.Fatalf("CreateView: %v", err)
	}
	encoder, err := f.device.CreateCommandEncoder(&CommandEncoderDescriptor{Label: "headless-readback"})
	if err != nil {
		view.Release()
		f.surface.DiscardTexture()
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&RenderPassDescriptor{
		Label: "headless-readback",
		ColorAttachments: []RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: clearColor,
		}},
	})
	if err != nil {
		encoder.DiscardEncoding()
		view.Release()
		f.surface.DiscardTexture()
		t.Fatalf("BeginRenderPass: %v", err)
	}
	return texture, view, encoder, pass
}

func (f *headlessSoftwareFixture) submitAndPresent(t *testing.T, texture *SurfaceTexture, view *TextureView, encoder *CommandEncoder, pass *RenderPassEncoder) {
	t.Helper()

	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		view.Release()
		f.surface.DiscardTexture()
		t.Fatalf("RenderPass.End: %v", err)
	}
	commandBuffer, err := encoder.Finish()
	if err != nil {
		view.Release()
		f.surface.DiscardTexture()
		t.Fatalf("CommandEncoder.Finish: %v", err)
	}
	if _, err := f.device.Queue().Submit(commandBuffer); err != nil {
		commandBuffer.Release()
		view.Release()
		f.surface.DiscardTexture()
		t.Fatalf("Queue.Submit: %v", err)
	}
	view.Release()
	if err := f.surface.Present(texture); err != nil {
		t.Fatalf("Present: %v", err)
	}
}

func newConfiguredWrappedSurface(t *testing.T, raw hal.Surface) *Surface {
	t.Helper()

	device, err := NewDeviceFromHAL(&noop.Device{}, &noop.Queue{}, 0, DefaultLimits(), "readback-wrapper")
	if err != nil {
		t.Fatalf("NewDeviceFromHAL: %v", err)
	}
	surface := NewSurfaceFromHAL(raw, "readback-wrapper")
	// The wrapper owns an already-created raw surface. Set the same internal
	// creation fact that Instance-created surfaces carry before Configure.
	surface.surfaceCreated = true
	t.Cleanup(func() {
		surface.Unconfigure()
		surface.Release()
		device.Release()
	})
	if err := surface.Configure(device, &SurfaceConfiguration{
		Width:       2,
		Height:      2,
		Format:      TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	return surface
}

func TestHeadlessSurfaceClearReadback(t *testing.T) {
	const width, height = uint32(7), uint32(5)
	wantPixel := []byte{0xff, 0x00, 0x7f, 0xff}

	for _, format := range []TextureFormat{TextureFormatRGBA8Unorm, TextureFormatBGRA8Unorm} {
		t.Run(format.String(), func(t *testing.T) {
			fixture := newHeadlessSoftwareFixture(t, width, height, format, true)
			texture, view, encoder, pass := fixture.beginFrame(t, Color{R: 1, G: 0, B: 0.5, A: 1})
			fixture.submitAndPresent(t, texture, view, encoder, pass)

			pixels, err := fixture.surface.ReadPixels()
			if err != nil {
				t.Fatalf("ReadPixels: %v", err)
			}
			if want := int(width * height * 4); len(pixels) != want {
				t.Fatalf("ReadPixels length = %d, want %d", len(pixels), want)
			}
			for offset := 0; offset < len(pixels); offset += 4 {
				if !bytes.Equal(pixels[offset:offset+4], wantPixel) {
					t.Fatalf("pixel %d = %v, want RGBA %v", offset/4, pixels[offset:offset+4], wantPixel)
				}
			}

			pixels[0] = 0
			second, err := fixture.surface.ReadPixels()
			if err != nil {
				t.Fatalf("second ReadPixels: %v", err)
			}
			if !bytes.Equal(second[:4], wantPixel) {
				t.Fatalf("second snapshot begins %v after caller mutation, want %v", second[:4], wantPixel)
			}
		})
	}
}

func TestHeadlessSurfaceTriangleReadback(t *testing.T) {
	const width, height = uint32(32), uint32(32)
	fixture := newHeadlessSoftwareFixture(t, width, height, TextureFormatRGBA8Unorm, true)

	shader, err := fixture.device.CreateShaderModule(&ShaderModuleDescriptor{
		Label: "headless-triangle",
		WGSL:  headlessTriangleWGSL,
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	pipeline, err := fixture.device.CreateRenderPipeline(&RenderPipelineDescriptor{
		Label:  "headless-triangle",
		Vertex: VertexState{Module: shader, EntryPoint: "vs_main"},
		Fragment: &FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []ColorTargetState{{
				Format:    TextureFormatRGBA8Unorm,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xffffffff},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	texture, view, encoder, pass := fixture.beginFrame(t, Color{R: 0, G: 0, B: 1, A: 1})
	pass.SetPipeline(pipeline)
	pass.Draw(3, 1, 0, 0)
	fixture.submitAndPresent(t, texture, view, encoder, pass)

	pixels, err := fixture.surface.ReadPixels()
	if err != nil {
		t.Fatalf("ReadPixels: %v", err)
	}
	if want := int(width * height * 4); len(pixels) != want {
		t.Fatalf("ReadPixels length = %d, want %d", len(pixels), want)
	}

	assertPixel := func(x, y uint32, want []byte) {
		t.Helper()
		offset := int((y*width + x) * 4)
		if got := pixels[offset : offset+4]; !bytes.Equal(got, want) {
			t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
		}
	}
	assertPixel(width/2, height/2, []byte{0, 0xff, 0, 0xff})
	assertPixel(0, 0, []byte{0, 0, 0xff, 0xff})
	assertPixel(width-1, 0, []byte{0, 0, 0xff, 0xff})
	assertPixel(0, height-1, []byte{0, 0, 0xff, 0xff})
	assertPixel(width-1, height-1, []byte{0, 0, 0xff, 0xff})
}

func TestHeadlessSurfaceReadPixelsStateErrors(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		var surface *Surface
		if _, err := surface.ReadPixels(); !errors.Is(err, ErrReleased) {
			t.Fatalf("ReadPixels error = %v, want ErrReleased", err)
		}
	})

	t.Run("Unconfigured", func(t *testing.T) {
		fixture := newHeadlessSoftwareFixture(t, 2, 2, TextureFormatRGBA8Unorm, false)
		if _, err := fixture.surface.ReadPixels(); err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("ReadPixels error = %v, want not configured", err)
		}
	})

	t.Run("Acquired", func(t *testing.T) {
		fixture := newHeadlessSoftwareFixture(t, 2, 2, TextureFormatRGBA8Unorm, true)
		if _, _, err := fixture.surface.GetCurrentTexture(); err != nil {
			t.Fatalf("GetCurrentTexture: %v", err)
		}
		if _, err := fixture.surface.ReadPixels(); err == nil || !strings.Contains(err.Error(), "still acquired") {
			t.Fatalf("ReadPixels error = %v, want acquired-state error", err)
		}
		fixture.surface.DiscardTexture()
	})

	t.Run("AfterUnconfigure", func(t *testing.T) {
		fixture := newHeadlessSoftwareFixture(t, 2, 2, TextureFormatRGBA8Unorm, true)
		fixture.surface.Unconfigure()
		if _, err := fixture.surface.ReadPixels(); err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("ReadPixels error = %v, want not configured", err)
		}
	})

	t.Run("AfterRelease", func(t *testing.T) {
		fixture := newHeadlessSoftwareFixture(t, 2, 2, TextureFormatRGBA8Unorm, true)
		fixture.surface.Release()
		if _, err := fixture.surface.ReadPixels(); !errors.Is(err, ErrReleased) {
			t.Fatalf("ReadPixels error = %v, want ErrReleased", err)
		}
	})

	t.Run("UnsupportedBackend", func(t *testing.T) {
		surface := newConfiguredWrappedSurface(t, &noop.Surface{})
		if _, err := surface.ReadPixels(); err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("ReadPixels error = %v, want unsupported backend", err)
		}
	})

	t.Run("EmptySnapshot", func(t *testing.T) {
		surface := newConfiguredWrappedSurface(t, &emptyPixelReaderSurface{})
		if _, err := surface.ReadPixels(); err == nil || !strings.Contains(err.Error(), "no pixel data") {
			t.Fatalf("ReadPixels error = %v, want empty-snapshot failure", err)
		}
	})
}

func TestSurfaceWritePixelsNil(t *testing.T) {
	var surface *Surface
	if err := surface.WritePixels(nil, 0, 0); !errors.Is(err, ErrReleased) {
		t.Fatalf("WritePixels error = %v, want ErrReleased", err)
	}
}
