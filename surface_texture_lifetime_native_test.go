//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/pendingwrites"
	surfacepkg "github.com/gogpu/wgpu/internal/surface"
)

// mockBatchingQueue is the batching HAL queue double for the pending-writes
// paths of this file. It lives here because the pending-writes domain and its
// own doubles moved to internal/pendingwrites (S8): a root test cannot share
// an unexported test type across the package boundary.
type mockBatchingQueue struct {
	noop.Queue
	submitCalls int
	submitErr   error
}

func (q *mockBatchingQueue) SupportsCommandBufferCopies() bool { return true }

func (q *mockBatchingQueue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	q.submitCalls++
	if q.submitErr != nil {
		return 0, q.submitErr
	}
	return q.Queue.Submit(commandBuffers)
}

type surfaceTextureLifetimeDevice struct {
	noop.Device
	destroyedTextures int
	destroyedViews    int
}

func (d *surfaceTextureLifetimeDevice) DestroyTexture(texture hal.Texture) {
	d.destroyedTextures++
	d.Device.DestroyTexture(texture)
}

func (d *surfaceTextureLifetimeDevice) DestroyTextureView(view hal.TextureView) {
	d.destroyedViews++
	d.Device.DestroyTextureView(view)
}

func newAcquiredSurfaceForLifetimeTest(t *testing.T) (*Surface, *SurfaceTexture, *Device, *surfaceTextureLifetimeDevice) {
	t.Helper()

	rawDevice := &surfaceTextureLifetimeDevice{}
	coreDevice := core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "surface-texture-lifetime-test")
	device := newTestDeviceWithQueue(coreDevice, nil, &noop.Queue{}, nil)

	rawSurface := &noop.Surface{}
	surface := &Surface{
		impl: surfacepkg.New(surfacepkg.Init{
			Core:           core.NewSurface(rawSurface, "surface-texture-lifetime-test"),
			Backend:        gputypes.BackendEmpty,
			SurfaceCreated: true,
		}),
	}
	config := &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopyDst,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := surface.Configure(device, config); err != nil {
		device.Release()
		t.Fatalf("surface.Configure: %v", err)
	}
	texture, _, err := surface.GetCurrentTexture()
	if err != nil {
		surface.Release()
		device.Release()
		t.Fatalf("surface.GetCurrentTexture: %v", err)
	}
	return surface, texture, device, rawDevice
}

func TestSurfaceTextureDerivedWrappersInvalidateAfterDiscard(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	texture := surfaceTexture.AsTexture()
	if texture == nil {
		t.Fatal("AsTexture returned nil for acquired texture")
	}
	texture.Release()
	if rawDevice.destroyedTextures != 0 {
		t.Fatalf("borrowed surface texture destruction count = %d, want 0", rawDevice.destroyedTextures)
	}
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	texture = surfaceTexture.AsTexture()
	if texture == nil {
		t.Fatal("second AsTexture returned nil while acquisition was active")
	}

	surface.DiscardTexture()
	if got := surfaceTexture.AsTexture(); got != nil {
		t.Fatal("AsTexture returned a wrapper after DiscardTexture")
	}
	if _, err := surfaceTexture.CreateView(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("CreateView after DiscardTexture = %v, want ErrReleased", err)
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after DiscardTexture = %v, want ErrReleased", err)
	}
	if err := device.Queue().WriteTexture(&ImageCopyTexture{Texture: texture}, nil, nil, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Queue.WriteTexture after DiscardTexture = %v, want ErrReleased", err)
	}

	view.Release()
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle after stale view release: %v", err)
	}
	if rawDevice.destroyedViews != 1 {
		t.Fatalf("stale surface view destruction count = %d, want 1", rawDevice.destroyedViews)
	}
	if view.Texture() != nil {
		t.Fatal("TextureView.Texture returned parent after stale view release")
	}
	surface.Release()
}

func TestPendingWriteTextureRejectsRetiredSurfaceAcquisition(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()
	defer surface.Release()

	// Install the batching path used by Vulkan, DX12, and Metal. The write is
	// valid when recorded, then its acquisition retires before Submit.
	rawQueue := &mockBatchingQueue{}
	pool := encoder.NewPool(rawDevice)
	replaceTestQueue(device, pool, rawQueue, pendingwrites.New(rawDevice, rawQueue, pool, nil))

	texture := surfaceTexture.AsTexture()
	err := device.Queue().WriteTexture(
		&ImageCopyTexture{Texture: texture},
		[]byte{1, 2, 3, 4},
		&ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1},
		&Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	)
	if err != nil {
		t.Fatalf("WriteTexture while acquired: %v", err)
	}
	if !device.queue.impl.Pending().HasPendingWork() {
		t.Fatal("WriteTexture did not create pending work")
	}

	surface.DiscardTexture()
	if _, err := device.Queue().Submit(); !errors.Is(err, ErrReleased) {
		t.Fatalf("Submit after acquisition retirement = %v, want ErrReleased", err)
	}
	if rawQueue.submitCalls != 0 {
		t.Fatalf("HAL Submit calls = %d, want 0", rawQueue.submitCalls)
	}
	if device.queue.impl.Pending().HasPendingWork() {
		t.Fatal("rejected pending batch remained active")
	}
	stats := device.queue.impl.Pending().BeltStats()
	if stats.ActiveChunks != 0 || stats.FreeChunks != 1 || stats.ClosedSubs != 0 {
		t.Fatalf("staging belt after rejection = %+v, want one recycled free chunk", stats)
	}
}

func TestPendingWriteTextureSubmitFailureRecyclesBatch(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()
	defer surface.Release()

	submitErr := errors.New("submit rejected")
	rawQueue := &mockBatchingQueue{submitErr: submitErr}
	pool := encoder.NewPool(rawDevice)
	replaceTestQueue(device, pool, rawQueue, pendingwrites.New(rawDevice, rawQueue, pool, nil))

	err := device.Queue().WriteTexture(
		&ImageCopyTexture{Texture: surfaceTexture.AsTexture()},
		[]byte{1, 2, 3, 4},
		&ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1},
		&Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	)
	if err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	if _, err := device.Queue().Submit(); !errors.Is(err, submitErr) {
		t.Fatalf("Submit = %v, want wrapped submit error", err)
	}
	if rawQueue.submitCalls != 1 {
		t.Fatalf("HAL Submit calls = %d, want 1", rawQueue.submitCalls)
	}
	if device.queue.impl.Pending().HasPendingWork() {
		t.Fatal("failed submitted batch remained active")
	}
	stats := device.queue.impl.Pending().BeltStats()
	if stats.ActiveChunks != 0 || stats.FreeChunks != 1 || stats.ClosedSubs != 0 {
		t.Fatalf("staging belt after failed submit = %+v, want one recycled free chunk", stats)
	}
}

func TestRetiredSurfaceWrappersFailAtPublicHALBoundaries(t *testing.T) {
	t.Run("render attachment", func(t *testing.T) {
		surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
		defer device.Release()
		defer surface.Release()

		view, err := surfaceTexture.CreateView(nil)
		if err != nil {
			t.Fatalf("CreateView: %v", err)
		}
		defer view.Release()
		surface.DiscardTexture()

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		defer enc.DiscardEncoding()
		_, err = enc.BeginRenderPass(&RenderPassDescriptor{
			ColorAttachments: []RenderPassColorAttachment{{
				View:    view,
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			}},
		})
		if !errors.Is(err, ErrReleased) {
			t.Fatalf("BeginRenderPass with retired view = %v, want ErrReleased", err)
		}
	})

	t.Run("texture copy", func(t *testing.T) {
		surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
		defer device.Release()
		defer surface.Release()

		texture := surfaceTexture.AsTexture()
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		surface.DiscardTexture()
		enc.CopyTextureToTexture(texture, texture, nil)
		if _, err := enc.Finish(); !errors.Is(err, ErrReleased) {
			t.Fatalf("Finish after copy from retired texture = %v, want ErrReleased", err)
		}
	})

	t.Run("bind group", func(t *testing.T) {
		surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
		defer device.Release()
		defer surface.Release()

		view, err := surfaceTexture.CreateView(nil)
		if err != nil {
			t.Fatalf("CreateView: %v", err)
		}
		defer view.Release()
		layout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
			Entries: []gputypes.BindGroupLayoutEntry{{
				Binding:    0,
				Visibility: ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeFloat,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			}},
		})
		if err != nil {
			t.Fatalf("CreateBindGroupLayout: %v", err)
		}
		defer layout.Release()
		surface.DiscardTexture()

		_, err = device.CreateBindGroup(&BindGroupDescriptor{
			Layout:  layout,
			Entries: []BindGroupEntry{{Binding: 0, TextureView: view}},
		})
		if !errors.Is(err, ErrReleased) {
			t.Fatalf("CreateBindGroup with retired view = %v, want ErrReleased", err)
		}
	})

	t.Run("submit", func(t *testing.T) {
		surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
		defer device.Release()
		defer surface.Release()

		view, err := surfaceTexture.CreateView(nil)
		if err != nil {
			t.Fatalf("CreateView: %v", err)
		}
		defer view.Release()
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := enc.BeginRenderPass(&RenderPassDescriptor{
			ColorAttachments: []RenderPassColorAttachment{{
				View:    view,
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("RenderPass.End: %v", err)
		}
		commandBuffer, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		defer commandBuffer.Release()

		rawQueue := &mockBatchingQueue{}
		replaceTestQueue(device, testDevicePool(device), rawQueue, nil)
		surface.DiscardTexture()
		if _, err := device.Queue().Submit(commandBuffer); !errors.Is(err, ErrSubmitTextureDestroyed) {
			t.Fatalf("Submit with retired attachment = %v, want ErrSubmitTextureDestroyed", err)
		}
		if rawQueue.submitCalls != 0 {
			t.Fatalf("HAL Submit calls = %d, want 0", rawQueue.submitCalls)
		}
	})
}

func TestTextureCopyRegionsRejectRetiredSurfaceAliases(t *testing.T) {
	surface, first, device, _ := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()
	defer surface.Release()

	retired := first.AsTexture()
	if err := surface.Present(first); err != nil {
		t.Fatalf("first Present: %v", err)
	}
	current, _, err := surface.GetCurrentTexture()
	if err != nil {
		t.Fatalf("second GetCurrentTexture: %v", err)
	}
	active := current.AsTexture()
	buffer, err := device.CreateBuffer(&BufferDescriptor{Size: 4, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()

	toBuffer, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder for buffer copy: %v", err)
	}
	toBuffer.CopyTextureToBuffer(active, buffer, []BufferTextureCopy{{
		TextureBase: ImageCopyTexture{Texture: retired},
		Size:        Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}})
	if _, err := toBuffer.Finish(); !errors.Is(err, ErrReleased) {
		t.Fatalf("CopyTextureToBuffer with retired region alias = %v, want ErrReleased", err)
	}

	toTexture, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder for texture copy: %v", err)
	}
	toTexture.CopyTextureToTexture(active, active, []TextureCopy{{
		Source:      ImageCopyTexture{Texture: retired},
		Destination: ImageCopyTexture{Texture: active},
		Size:        Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}})
	if _, err := toTexture.Finish(); !errors.Is(err, ErrReleased) {
		t.Fatalf("CopyTextureToTexture with retired region alias = %v, want ErrReleased", err)
	}

	surface.DiscardTexture()
}

func TestBufferCopyToSurfaceTextureHonorsAcquisitionLifetime(t *testing.T) {
	surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()
	defer surface.Release()

	buffer, err := device.CreateBuffer(&BufferDescriptor{
		Size:  256,
		Usage: gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()
	texture := surfaceTexture.AsTexture()
	region := []BufferTextureCopy{{
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
		TextureBase:  ImageCopyTexture{Texture: texture},
		Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}}

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToTexture(buffer, texture, region)
	commandBuffer, err := enc.Finish()
	if err != nil {
		t.Fatalf("active CopyBufferToTexture: %v", err)
	}
	commandBuffer.Release()

	surface.DiscardTexture()
	tests := []struct {
		name   string
		record func(*CommandEncoder)
	}{
		{
			name: "buffer to texture",
			record: func(encoder *CommandEncoder) {
				encoder.CopyBufferToTexture(buffer, texture, region)
			},
		},
		{
			name: "texture to buffer",
			record: func(encoder *CommandEncoder) {
				encoder.CopyTextureToBuffer(texture, buffer, nil)
			},
		},
		{
			name: "transition",
			record: func(encoder *CommandEncoder) {
				encoder.TransitionTextures([]TextureBarrier{{Texture: texture}})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			test.record(enc)
			if _, err := enc.Finish(); !errors.Is(err, ErrReleased) {
				t.Fatalf("Finish = %v, want ErrReleased", err)
			}
		})
	}
}

func TestSurfaceTextureDerivedWrappersInvalidateAfterPresentAndRelease(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	texture := surfaceTexture.AsTexture()
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	if err := surface.Present(surfaceTexture); err != nil {
		t.Fatalf("surface.Present: %v", err)
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after Present = %v, want ErrReleased", err)
	}
	view.Release()
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle after presented view release: %v", err)
	}
	if rawDevice.destroyedViews != 1 {
		t.Fatalf("stale presented view destruction count = %d, want 1", rawDevice.destroyedViews)
	}

	// A later acquisition is invalidated by surface teardown as well.
	surfaceTexture, _, err = surface.GetCurrentTexture()
	if err != nil {
		t.Fatalf("second GetCurrentTexture: %v", err)
	}
	texture = surfaceTexture.AsTexture()
	view, err = surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("second CreateView: %v", err)
	}
	surface.Release()
	if surfaceTexture.AsTexture() != nil {
		t.Fatal("AsTexture returned a wrapper after Surface.Release")
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after Surface.Release = %v, want ErrReleased", err)
	}
	view.Release()
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle after released-surface view release: %v", err)
	}
	if rawDevice.destroyedViews != 2 {
		t.Fatalf("stale released-surface view destruction count = %d, want 2", rawDevice.destroyedViews)
	}
}

// TestSurfacePresentRejectsTextureFromAnotherAcquisition moved to
// internal/surface with the implementation it exercises (ADR-070 S10).

func TestSurfaceTextureLeaseAcrossReconfigureAndUnconfigure(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	config := &SurfaceConfiguration{
		Width:       2,
		Height:      2,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopyDst,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := surface.Configure(device, config); !errors.Is(err, core.ErrSurfaceConfigureWhileAcquired) {
		t.Fatalf("Configure while acquired = %v, want ErrSurfaceConfigureWhileAcquired", err)
	}
	if err := surface.Present(surfaceTexture); err != nil {
		t.Fatalf("Present after rejected Configure: %v", err)
	}
	if err := surface.Configure(device, config); err != nil {
		t.Fatalf("Configure after Present: %v", err)
	}

	surfaceTexture, _, err := surface.GetCurrentTexture()
	if err != nil {
		t.Fatalf("GetCurrentTexture after reconfigure: %v", err)
	}
	texture := surfaceTexture.AsTexture()
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	surface.Unconfigure()
	if surfaceTexture.AsTexture() != nil {
		t.Fatal("AsTexture returned a wrapper after Unconfigure")
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after Unconfigure = %v, want ErrReleased", err)
	}
	if view.Texture() == nil {
		t.Fatal("TextureView.Texture did not preserve its parent after Unconfigure")
	}

	texture.Release()
	texture.Release()
	view.Release()
	view.Release()
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle after wrapper releases: %v", err)
	}
	if rawDevice.destroyedTextures != 0 || rawDevice.destroyedViews != 1 {
		t.Fatalf("borrowed wrapper releases: textures=%d, views=%d; want texture=0 view=1", rawDevice.destroyedTextures, rawDevice.destroyedViews)
	}
	surface.Release()
}

// TestSurfaceSwapchainTextureCaching moved to internal/surface with the
// implementation it exercises (ADR-070 S10).
func TestSurfaceTextureDerivedWrappersInvalidateAfterDeviceRelease(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)

	texture := surfaceTexture.AsTexture()
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}

	device.Release()
	if texture.impl.ResolveHAL() != nil {
		t.Fatal("surface texture wrapper remained usable after Device.Release")
	}
	if view.impl.ResolveHAL() != nil {
		t.Fatal("surface texture view remained usable after Device.Release")
	}
	if surfaceTexture.AsTexture() != nil {
		t.Fatal("AsTexture returned a wrapper after Device.Release")
	}
	view.Release()
	if rawDevice.destroyedViews != 0 {
		t.Fatalf("post-device-release view destruction count = %d, want 0", rawDevice.destroyedViews)
	}
	surface.Release()
}
