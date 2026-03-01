package software

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestBackendRegistration(t *testing.T) {
	backend := API{}
	if backend.Variant() != gputypes.BackendEmpty {
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
	if adapter.Info.DeviceType != gputypes.DeviceTypeCPU {
		t.Errorf("Expected DeviceTypeCPU, got %v", adapter.Info.DeviceType)
	}
}

func TestDeviceCreation(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter

	openDev, err := adapter.Open(0, gputypes.DefaultLimits())
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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	// Create buffer
	buffer, err := openDev.Device.CreateBuffer(&hal.BufferDescriptor{
		Label: "Test Buffer",
		Size:  1024,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageCopySrc,
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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	buffer, _ := openDev.Device.CreateBuffer(&hal.BufferDescriptor{
		Size:  256,
		Usage: gputypes.BufferUsageCopyDst,
	})
	defer openDev.Device.DestroyBuffer(buffer)

	// Write data via queue
	testData := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if err := openDev.Queue.WriteBuffer(buffer, 0, testData); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}

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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
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
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	texture, _ := openDev.Device.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{
			Width:              16,
			Height:             16,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	defer openDev.Device.DestroyTexture(texture)

	tex := texture.(*Texture)

	// Clear to red
	tex.Clear(gputypes.Color{R: 1.0, G: 0.0, B: 0.0, A: 1.0})

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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	// Configure surface
	err = surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:       100,
		Height:      100,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
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
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	_, err := openDev.Device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "Test Compute",
	})
	if err == nil {
		t.Fatal("expected error for compute pipeline creation, got nil")
	}
	if !errors.Is(err, ErrComputeNotSupported) {
		t.Errorf("expected ErrComputeNotSupported, got: %v", err)
	}
}

func TestComputePipelineNotSupportedNilDescriptor(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	adapter := adapters[0].Adapter
	openDev, _ := adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	_, err := openDev.Device.CreateComputePipeline(nil)
	if err == nil {
		t.Fatal("expected error for nil compute pipeline descriptor, got nil")
	}
	if !errors.Is(err, ErrComputeNotSupported) {
		t.Errorf("expected ErrComputeNotSupported, got: %v", err)
	}
}

func TestAdapterDownlevelNoCompute(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no adapters found")
	}

	caps := adapters[0].Capabilities
	if caps.DownlevelCapabilities.Flags&hal.DownlevelFlagsComputeShaders != 0 {
		t.Error("software backend should not report compute shader support")
	}
}

// =============================================================================
// Device Resource Tests
// =============================================================================

func createSoftwareDevice(t *testing.T) (*Device, hal.Queue, func()) {
	t.Helper()
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	dev := openDev.Device.(*Device)
	cleanup := func() {
		openDev.Device.Destroy()
		instance.Destroy()
	}
	return dev, openDev.Queue, cleanup
}

func TestDeviceCreateTextureView(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 32, Height: 32, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	if err != nil {
		t.Fatalf("CreateTexture failed: %v", err)
	}
	defer dev.DestroyTexture(tex)

	view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView failed: %v", err)
	}
	if view == nil {
		t.Fatal("TextureView is nil")
	}
	defer dev.DestroyTextureView(view)

	// View should reference the original texture
	tv, ok := view.(*TextureView)
	if !ok {
		t.Fatal("expected *TextureView")
	}
	if tv.texture == nil {
		t.Fatal("TextureView.texture is nil")
	}
}

func TestDeviceCreateTextureViewNonSoftwareTexture(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Pass a non-software texture (using noop Resource as mock)
	view, err := dev.CreateTextureView(&Resource{}, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView with non-software texture should not error: %v", err)
	}
	// Should return a plain Resource
	_, isResource := view.(*Resource)
	if !isResource {
		t.Error("expected *Resource for non-software texture")
	}
}

func TestDeviceCreateSampler(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	sampler, err := dev.CreateSampler(&hal.SamplerDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateSampler failed: %v", err)
	}
	if sampler == nil {
		t.Fatal("sampler is nil")
	}
	dev.DestroySampler(sampler) // no-op, should not panic
}

func TestDeviceCreateBindGroupLayout(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	bgl, err := dev.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout failed: %v", err)
	}
	if bgl == nil {
		t.Fatal("BindGroupLayout is nil")
	}
	dev.DestroyBindGroupLayout(bgl)
}

func TestDeviceCreateBindGroup(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateBindGroup failed: %v", err)
	}
	if bg == nil {
		t.Fatal("BindGroup is nil")
	}
	dev.DestroyBindGroup(bg)
}

func TestDeviceCreatePipelineLayout(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	pl, err := dev.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreatePipelineLayout failed: %v", err)
	}
	if pl == nil {
		t.Fatal("PipelineLayout is nil")
	}
	dev.DestroyPipelineLayout(pl)
}

func TestDeviceCreateShaderModule(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}
	if sm == nil {
		t.Fatal("ShaderModule is nil")
	}
	dev.DestroyShaderModule(sm)
}

func TestDeviceCreateRenderPipeline(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	rp, err := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateRenderPipeline failed: %v", err)
	}
	if rp == nil {
		t.Fatal("RenderPipeline is nil")
	}
	dev.DestroyRenderPipeline(rp)
}

func TestDeviceCreateQuerySet(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	qs, err := dev.CreateQuerySet(&hal.QuerySetDescriptor{})
	if err == nil {
		t.Fatal("CreateQuerySet should return error for software backend")
	}
	if qs != nil {
		t.Fatal("CreateQuerySet should return nil")
	}
}

func TestDeviceCreateRenderBundleEncoder(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, err := dev.CreateRenderBundleEncoder(&hal.RenderBundleEncoderDescriptor{})
	if err == nil {
		t.Fatal("CreateRenderBundleEncoder should return error for software backend")
	}
	if enc != nil {
		t.Fatal("CreateRenderBundleEncoder should return nil")
	}
}

func TestDeviceCreateCommandEncoder(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}
	if enc == nil {
		t.Fatal("CommandEncoder is nil")
	}
}

func TestDeviceFenceOperations(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	fence, err := dev.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence failed: %v", err)
	}
	defer dev.DestroyFence(fence)

	// Initial status should be unsignaled
	signaled, err := dev.GetFenceStatus(fence)
	if err != nil {
		t.Fatalf("GetFenceStatus failed: %v", err)
	}
	if signaled {
		t.Error("fence should not be signaled initially")
	}

	// Signal the fence
	f := fence.(*Fence)
	f.value.Store(1)

	// Now should be signaled
	signaled, err = dev.GetFenceStatus(fence)
	if err != nil {
		t.Fatalf("GetFenceStatus failed: %v", err)
	}
	if !signaled {
		t.Error("fence should be signaled after store")
	}

	// Wait should return true (already at value)
	ok, err := dev.Wait(fence, 1, 0)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !ok {
		t.Error("Wait should return true when fence has reached value")
	}

	// Wait for a higher value should return false
	ok, err = dev.Wait(fence, 100, 0)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if ok {
		t.Error("Wait should return false when fence hasn't reached value")
	}

	// Reset fence
	err = dev.ResetFence(fence)
	if err != nil {
		t.Fatalf("ResetFence failed: %v", err)
	}
	if f.value.Load() != 0 {
		t.Error("fence value should be 0 after reset")
	}
}

func TestDeviceFenceWithNonSoftwareFence(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Use nil fence -- tests type assertion fallback paths
	var fake hal.Fence

	ok, err := dev.Wait(fake, 0, 0)
	if err != nil {
		t.Errorf("Wait with nil fence should not error: %v", err)
	}
	if !ok {
		t.Error("Wait with nil fence should return true (fallback)")
	}

	signaled, err := dev.GetFenceStatus(fake)
	if err != nil {
		t.Errorf("GetFenceStatus with nil fence should not error: %v", err)
	}
	if signaled {
		t.Error("GetFenceStatus with nil fence should return false")
	}

	err = dev.ResetFence(fake)
	if err != nil {
		t.Errorf("ResetFence with nil fence should not error: %v", err)
	}
}

func TestDeviceWaitIdle(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	if err := dev.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle failed: %v", err)
	}
}

func TestDeviceDestroyNoOps(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// All Destroy methods are no-ops; should not panic with nil
	dev.DestroyBuffer(nil)
	dev.DestroyTexture(nil)
	dev.DestroyTextureView(nil)
	dev.DestroySampler(nil)
	dev.DestroyBindGroupLayout(nil)
	dev.DestroyBindGroup(nil)
	dev.DestroyPipelineLayout(nil)
	dev.DestroyShaderModule(nil)
	dev.DestroyRenderPipeline(nil)
	dev.DestroyComputePipeline(nil)
	dev.DestroyQuerySet(nil)
	dev.DestroyRenderBundle(nil)
	dev.DestroyFence(nil)
	dev.FreeCommandBuffer(nil)
}

// =============================================================================
// Command Encoder Tests
// =============================================================================

func TestCommandEncoderBeginEnd(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	if err := enc.BeginEncoding("test-pass"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}

	cmdBuf, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}
	if cmdBuf == nil {
		t.Fatal("EndEncoding returned nil")
	}
}

func TestCommandEncoderDiscardAndReset(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	_ = enc.BeginEncoding("test")
	enc.DiscardEncoding() // should not panic

	enc.ResetAll(nil)
	enc.ResetAll([]hal.CommandBuffer{})
}

func TestCommandEncoderTransitions(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	enc.TransitionBuffers(nil)
	enc.TransitionBuffers([]hal.BufferBarrier{{}})
	enc.TransitionTextures(nil)
	enc.TransitionTextures([]hal.TextureBarrier{{}})
}

func TestCommandEncoderClearBuffer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256, Usage: gputypes.BufferUsageCopyDst})
	defer dev.DestroyBuffer(buf)

	// Write some data first
	b := buf.(*Buffer)
	b.WriteData(0, []byte{1, 2, 3, 4, 5, 6, 7, 8})

	// Clear the buffer
	enc.ClearBuffer(buf, 0, 8)

	// Verify cleared
	data := b.GetData()
	for i := 0; i < 8; i++ {
		if data[i] != 0 {
			t.Errorf("byte %d = %d, want 0 after clear", i, data[i])
		}
	}
}

func TestCommandEncoderCopyBufferToBuffer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	srcBuf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256})
	dstBuf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256})
	defer dev.DestroyBuffer(srcBuf)
	defer dev.DestroyBuffer(dstBuf)

	// Write test data
	src := srcBuf.(*Buffer)
	src.WriteData(0, []byte{10, 20, 30, 40})

	enc.CopyBufferToBuffer(srcBuf, dstBuf, []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 0, Size: 4},
	})

	dst := dstBuf.(*Buffer)
	data := dst.GetData()
	for i := 0; i < 4; i++ {
		expected := byte((i + 1) * 10)
		if data[i] != expected {
			t.Errorf("byte %d = %d, want %d", i, data[i], expected)
		}
	}
}

func TestCommandEncoderCopyBufferToTexture(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyBuffer(buf)
	defer dev.DestroyTexture(tex)

	// Write 16 bytes (2x2 RGBA)
	srcBuf := buf.(*Buffer)
	srcBuf.WriteData(0, []byte{
		255, 0, 0, 255, // R
		0, 255, 0, 255, // G
		0, 0, 255, 255, // B
		255, 255, 0, 255, // Y
	})

	enc.CopyBufferToTexture(buf, tex, []hal.BufferTextureCopy{
		{
			BufferLayout: hal.ImageDataLayout{Offset: 0},
			Size:         hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		},
	})

	// Verify texture has data
	dstTex := tex.(*Texture)
	data := dstTex.GetData()
	if data[0] != 255 || data[1] != 0 || data[2] != 0 || data[3] != 255 {
		t.Errorf("first pixel = (%d,%d,%d,%d), want (255,0,0,255)", data[0], data[1], data[2], data[3])
	}
}

func TestCommandEncoderCopyTextureToBuffer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	})
	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyTexture(tex)
	defer dev.DestroyBuffer(buf)

	// Write data to texture
	srcTex := tex.(*Texture)
	srcTex.WriteData(0, []byte{100, 200, 150, 255, 50, 75, 25, 128})

	enc.CopyTextureToBuffer(tex, buf, []hal.BufferTextureCopy{
		{
			BufferLayout: hal.ImageDataLayout{Offset: 0},
			Size:         hal.Extent3D{Width: 2, Height: 1, DepthOrArrayLayers: 1},
		},
	})

	dstBuf := buf.(*Buffer)
	data := dstBuf.GetData()
	if data[0] != 100 || data[1] != 200 {
		t.Errorf("copied data = (%d,%d,...), want (100,200,...)", data[0], data[1])
	}
}

func TestCommandEncoderCopyTextureToTexture(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	tex1, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	tex2, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyTexture(tex1)
	defer dev.DestroyTexture(tex2)

	// Write data to source
	srcTex := tex1.(*Texture)
	srcTex.Clear(gputypes.Color{R: 0.5, G: 0.25, B: 0.75, A: 1.0})

	enc.CopyTextureToTexture(tex1, tex2, []hal.TextureCopy{
		{Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}},
	})

	dstTex := tex2.(*Texture)
	data := dstTex.GetData()
	// 0.5 * 255 = 127
	if data[0] != 127 {
		t.Errorf("first byte = %d, want ~127", data[0])
	}
}

func TestCommandEncoderCopyWithNonSoftwareBuffers(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})

	// Use Resource (non-Buffer/non-Texture) -- should not panic, just return
	r := &Resource{}
	enc.ClearBuffer(r, 0, 64)                          // non-Buffer, no-op
	enc.CopyBufferToBuffer(r, r, []hal.BufferCopy{{}}) // non-Buffer, no-op
	enc.CopyBufferToTexture(r, r, []hal.BufferTextureCopy{{}})
	enc.CopyTextureToBuffer(r, r, []hal.BufferTextureCopy{{}})
	enc.CopyTextureToTexture(r, r, []hal.TextureCopy{{}})
}

func TestCommandEncoderResolveQuerySet(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	enc.ResolveQuerySet(nil, 0, 1, buf, 0) // no-op
}

// =============================================================================
// Render Pass Encoder Tests
// =============================================================================

func TestRenderPassEncoderEnd(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:  hal.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 1},
		Usage: gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(tex)

	view, _ := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(view)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1},
			},
		},
	})

	pass.End()

	// Verify that the clear color was applied
	underlyingTex := tex.(*Texture)
	data := underlyingTex.GetData()
	if data[0] != 255 || data[1] != 0 || data[2] != 0 || data[3] != 255 {
		t.Errorf("after clear: pixel = (%d,%d,%d,%d), want (255,0,0,255)", data[0], data[1], data[2], data[3])
	}
}

func TestRenderPassEncoderWithDepthStencil(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	colorTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	depthTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyTexture(colorTex)
	defer dev.DestroyTexture(depthTex)

	colorView, _ := dev.CreateTextureView(colorTex, &hal.TextureViewDescriptor{})
	depthView, _ := dev.CreateTextureView(depthTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(colorView)
	defer dev.DestroyTextureView(depthView)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: colorView, LoadOp: gputypes.LoadOpClear},
		},
		DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{
			View:            depthView,
			DepthLoadOp:     gputypes.LoadOpClear,
			DepthClearValue: 1.0,
		},
	})
	pass.End() // should not panic
}

func TestRenderPassEncoderAllNoOps(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 256})
	defer dev.DestroyBuffer(buf)

	// All no-ops
	pass.SetPipeline(nil)
	pass.SetBindGroup(0, nil, nil)
	pass.SetVertexBuffer(0, buf, 0)
	pass.SetIndexBuffer(buf, gputypes.IndexFormatUint16, 0)
	pass.SetViewport(0, 0, 800, 600, 0, 1)
	pass.SetScissorRect(0, 0, 800, 600)
	pass.SetBlendConstant(&gputypes.Color{R: 1, G: 1, B: 1, A: 1})
	pass.SetStencilReference(0xFF)
	pass.Draw(3, 1, 0, 0)
	pass.DrawIndexed(6, 1, 0, 0, 0)
	pass.DrawIndirect(buf, 0)
	pass.DrawIndexedIndirect(buf, 0)
	pass.ExecuteBundle(nil)
	pass.End()
}

// =============================================================================
// Compute Pass Encoder Tests
// =============================================================================

func TestComputePassEncoderAllNoOps(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "test-cp"})

	pass.SetPipeline(nil)
	pass.SetBindGroup(0, nil, nil)
	pass.Dispatch(1, 1, 1)
	pass.DispatchIndirect(nil, 0)
	pass.End()
}

// =============================================================================
// Surface Tests
// =============================================================================

func TestSurfaceZeroArea(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	// Width=0 should return ErrZeroArea
	err := surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:  0,
		Height: 600,
	})
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Errorf("expected ErrZeroArea, got %v", err)
	}

	// Height=0 should return ErrZeroArea
	err = surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:  800,
		Height: 0,
	})
	if !errors.Is(err, hal.ErrZeroArea) {
		t.Errorf("expected ErrZeroArea for zero height, got %v", err)
	}
}

func TestSurfaceAcquireTexture(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()

	surface.Configure(openDev.Device, &hal.SurfaceConfiguration{
		Width:       100,
		Height:      100,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		PresentMode: hal.PresentModeImmediate,
	})

	acquired, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture failed: %v", err)
	}
	if acquired == nil {
		t.Fatal("acquired is nil")
	}
	if acquired.Texture == nil {
		t.Fatal("acquired.Texture is nil")
	}
	if acquired.Suboptimal {
		t.Error("expected Suboptimal=false")
	}

	// Discard should not panic
	surface.DiscardTexture(acquired.Texture)
}

func TestSurfaceGetFramebufferNil(t *testing.T) {
	s := &Surface{}
	fb := s.GetFramebuffer()
	if fb != nil {
		t.Error("GetFramebuffer should return nil for unconfigured surface")
	}
}

// =============================================================================
// Resource Tests
// =============================================================================

func TestResourceNativeHandle(t *testing.T) {
	r := &Resource{}
	if r.NativeHandle() != 0 {
		t.Error("Resource.NativeHandle should return 0")
	}

	b := &Buffer{data: make([]byte, 64)}
	if b.NativeHandle() != 0 {
		t.Error("Buffer.NativeHandle should return 0")
	}

	tex := &Texture{data: make([]byte, 64)}
	if tex.NativeHandle() != 0 {
		t.Error("Texture.NativeHandle should return 0")
	}

	v := &TextureView{}
	if v.NativeHandle() != 0 {
		t.Error("TextureView.NativeHandle should return 0")
	}
}

func TestBufferGetWriteData(t *testing.T) {
	buf := &Buffer{data: make([]byte, 16)}

	// Write some data
	buf.WriteData(4, []byte{0xAA, 0xBB, 0xCC})

	// Read it back
	data := buf.GetData()
	if data[4] != 0xAA || data[5] != 0xBB || data[6] != 0xCC {
		t.Errorf("WriteData/GetData mismatch: got (%x,%x,%x)", data[4], data[5], data[6])
	}

	// GetData should return a copy (modifying it should not affect buffer)
	data[4] = 0xFF
	original := buf.GetData()
	if original[4] != 0xAA {
		t.Error("GetData should return a copy, not the original slice")
	}
}

func TestTextureGetWriteData(t *testing.T) {
	tex := &Texture{data: make([]byte, 32)}

	tex.WriteData(0, []byte{10, 20, 30, 40})

	data := tex.GetData()
	if data[0] != 10 || data[1] != 20 || data[2] != 30 || data[3] != 40 {
		t.Errorf("WriteData/GetData mismatch: got (%d,%d,%d,%d)", data[0], data[1], data[2], data[3])
	}

	// GetData should return a copy
	data[0] = 0xFF
	original := tex.GetData()
	if original[0] != 10 {
		t.Error("GetData should return a copy, not the original slice")
	}
}
