//go:build software

package software

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

func TestBackendRegistration(t *testing.T) {
	backend := API{}
	if backend.Variant() != types.BackendEmpty {
		t.Errorf("Expected BackendEmpty, got %v", backend.Variant())
	}
}

func TestInstanceCreation(t *testing.T) {
	backend := API{}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	if instance == nil {
		t.Fatal("Instance is nil")
	}
	instance.Destroy()
}

func TestAdapterEnumeration(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("No adapters found")
	}

	adapter := adapters[0]
	if adapter.Info.Name != "Software Renderer" {
		t.Errorf("Expected 'Software Renderer', got %s", adapter.Info.Name)
	}
	if adapter.Info.DeviceType != types.DeviceTypeCPU {
		t.Errorf("Expected DeviceTypeCPU, got %v", adapter.Info.DeviceType)
	}
}

func TestDeviceCreation(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter

	openDev, err := adapter.Open(0, types.DefaultLimits())
	if err != nil {
		t.Fatalf("Failed to open device: %v", err)
	}
	if openDev.Device == nil {
		t.Fatal("Device is nil")
	}
	if openDev.Queue == nil {
		t.Fatal("Queue is nil")
	}

	openDev.Device.Destroy()
}

func TestBufferCreation(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	// Create buffer
	buffer, err := openDev.Device.CreateBuffer(&hal.BufferDescriptor{
		Label: "Test Buffer",
		Size:  1024,
		Usage: types.BufferUsageCopyDst | types.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("Failed to create buffer: %v", err)
	}
	if buffer == nil {
		t.Fatal("Buffer is nil")
	}

	// Verify buffer has data storage
	buf, ok := buffer.(*Buffer)
	if !ok {
		t.Fatal("Buffer is not *Buffer type")
	}
	if len(buf.data) != 1024 {
		t.Errorf("Expected buffer size 1024, got %d", len(buf.data))
	}

	openDev.Device.DestroyBuffer(buffer)
}

func TestBufferWriteRead(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	buffer, _ := openDev.Device.CreateBuffer(&hal.BufferDescriptor{
		Size:  256,
		Usage: types.BufferUsageCopyDst,
	})
	defer openDev.Device.DestroyBuffer(buffer)

	// Write data via queue
	testData := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	openDev.Queue.WriteBuffer(buffer, 0, testData)

	// Read data back
	buf := buffer.(*Buffer)
	data := buf.GetData()

	// Verify first 8 bytes
	for i := 0; i < len(testData); i++ {
		if data[i] != testData[i] {
			t.Errorf("Byte %d: expected %d, got %d", i, testData[i], data[i])
		}
	}
}

func TestTextureCreation(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	texture, err := openDev.Device.CreateTexture(&hal.TextureDescriptor{
		Label: "Test Texture",
		Size: hal.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     types.TextureDimension2D,
		Format:        types.TextureFormatRGBA8Unorm,
		Usage:         types.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("Failed to create texture: %v", err)
	}
	if texture == nil {
		t.Fatal("Texture is nil")
	}

	// Verify texture has data storage
	tex, ok := texture.(*Texture)
	if !ok {
		t.Fatal("Texture is not *Texture type")
	}
	expectedSize := 256 * 256 * 1 * 4 // width * height * depth * 4 bytes per pixel
	if len(tex.data) != expectedSize {
		t.Errorf("Expected texture size %d, got %d", expectedSize, len(tex.data))
	}

	openDev.Device.DestroyTexture(texture)
}

func TestTextureClear(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	texture, _ := openDev.Device.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{
			Width:              16,
			Height:             16,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     types.TextureDimension2D,
		Format:        types.TextureFormatRGBA8Unorm,
		Usage:         types.TextureUsageRenderAttachment,
	})
	defer openDev.Device.DestroyTexture(texture)

	tex := texture.(*Texture)

	// Clear to red
	tex.Clear(types.Color{R: 1.0, G: 0.0, B: 0.0, A: 1.0})

	data := tex.GetData()

	// Check first pixel (RGBA order)
	if data[0] != 255 || data[1] != 0 || data[2] != 0 || data[3] != 255 {
		t.Errorf("Expected red pixel (255,0,0,255), got (%d,%d,%d,%d)", data[0], data[1], data[2], data[3])
	}
}

func TestSurfaceConfiguration(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	surface, err := instance.CreateSurface(0, 0)
	if err != nil {
		t.Fatalf("Failed to create surface: %v", err)
	}
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	// Configure surface
	err = surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      types.TextureFormatBGRA8Unorm,
		Usage:       types.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeImmediate,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	})
	if err != nil {
		t.Fatalf("Failed to configure surface: %v", err)
	}

	// Verify surface configuration
	surf := surface.(*Surface)
	if surf.width != 800 || surf.height != 600 {
		t.Errorf("Expected size 800x600, got %dx%d", surf.width, surf.height)
	}
	if len(surf.framebuffer) != 800*600*4 {
		t.Errorf("Expected framebuffer size %d, got %d", 800*600*4, len(surf.framebuffer))
	}

	surface.Unconfigure(openDev.Device)
}

func TestSurfaceFramebufferReadback(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:       100,
		Height:      100,
		Format:      types.TextureFormatRGBA8Unorm,
		Usage:       types.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeImmediate,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	})

	surf := surface.(*Surface)
	framebuffer := surf.GetFramebuffer()

	if len(framebuffer) != 100*100*4 {
		t.Errorf("Expected framebuffer size %d, got %d", 100*100*4, len(framebuffer))
	}

	surface.Unconfigure(openDev.Device)
}

func TestComputePipelineNotSupported(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, types.DefaultLimits())
	defer openDev.Device.Destroy()

	_, err := openDev.Device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "Test Compute",
	})
	if err == nil {
		t.Error("Expected error for compute pipeline creation, got nil")
	}
}
