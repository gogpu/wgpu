//go:build !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gogpu/wgpu"
)

// =============================================================================
// Fence lifecycle tests — Device.CreateFence, DestroyFence, ResetFence,
// GetFenceStatus, WaitForFence
// Covers device_native.go lines 747-820, fence.go lines 1-30
// =============================================================================

func TestDeviceCreateFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	if fence == nil {
		t.Fatal("CreateFence returned nil")
	}
	fence.Release()
}

func TestDeviceCreateFenceReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	fence, err := device.CreateFence()
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateFence after Release: got %v, want ErrReleased", err)
	}
	if fence != nil {
		t.Error("CreateFence after Release should return nil")
	}
}

func TestFenceReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	// First release succeeds.
	fence.Release()
	// Second release is a no-op (idempotent).
	fence.Release()
}

func TestDeviceDestroyFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	// DestroyFence is the deprecated path that calls fence.Release().
	device.DestroyFence(fence)
	// Second call should not panic.
	device.DestroyFence(fence)
}

func TestDeviceDestroyFenceNil(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// DestroyFence(nil) should not panic.
	device.DestroyFence(nil)
}

func TestDeviceResetFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	// ResetFence on a valid fence should succeed.
	if err := device.ResetFence(fence); err != nil {
		t.Fatalf("ResetFence: %v", err)
	}
}

func TestDeviceResetFenceReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	device.Release()

	err = device.ResetFence(fence)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("ResetFence after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceResetFenceNilFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	err := device.ResetFence(nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("ResetFence(nil): got %v, want ErrReleased", err)
	}
}

func TestDeviceResetFenceReleasedFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	fence.Release()
	err = device.ResetFence(fence)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("ResetFence on released fence: got %v, want ErrReleased", err)
	}
}

func TestDeviceGetFenceStatus(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	// GetFenceStatus on a freshly created fence should not error.
	_, err = device.GetFenceStatus(fence)
	if err != nil {
		t.Fatalf("GetFenceStatus: %v", err)
	}
}

func TestDeviceGetFenceStatusReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	device.Release()

	_, err = device.GetFenceStatus(fence)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("GetFenceStatus after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceGetFenceStatusNilFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	_, err := device.GetFenceStatus(nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("GetFenceStatus(nil): got %v, want ErrReleased", err)
	}
}

func TestDeviceGetFenceStatusReleasedFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	fence.Release()

	_, err = device.GetFenceStatus(fence)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("GetFenceStatus on released fence: got %v, want ErrReleased", err)
	}
}

func TestDeviceWaitForFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	// WaitForFence with a short timeout should return without error.
	_, err = device.WaitForFence(fence, 0, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForFence: %v", err)
	}
}

func TestDeviceWaitForFenceReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer fence.Release()

	device.Release()

	_, err = device.WaitForFence(fence, 0, time.Millisecond)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("WaitForFence after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceWaitForFenceNilFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	_, err := device.WaitForFence(nil, 0, time.Millisecond)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("WaitForFence(nil): got %v, want ErrReleased", err)
	}
}

func TestDeviceWaitForFenceReleasedFence(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}

	fence.Release()

	_, err = device.WaitForFence(fence, 0, time.Millisecond)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("WaitForFence on released fence: got %v, want ErrReleased", err)
	}
}

// =============================================================================
// FreeCommandBuffer — Device.FreeCommandBuffer
// Covers device_native.go lines 822-838
// =============================================================================

func TestDeviceFreeCommandBufferNil(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// FreeCommandBuffer(nil) should not panic.
	device.FreeCommandBuffer(nil)
}

func TestDeviceFreeCommandBufferReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	// FreeCommandBuffer on released device should not panic.
	device.FreeCommandBuffer(nil)
}

func TestDeviceFreeCommandBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	cmdBuf, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// FreeCommandBuffer should not panic on a valid command buffer.
	device.FreeCommandBuffer(cmdBuf)
}

// =============================================================================
// Released device extended tests — fence operations on released device
// Covers device_native.go fence guard clauses
// =============================================================================

func TestReleasedDeviceFenceOperations(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"CreateFence", func() error {
			_, err := device.CreateFence()
			return err
		}},
		{"ResetFence_nil", func() error {
			return device.ResetFence(nil)
		}},
		{"GetFenceStatus_nil", func() error {
			_, err := device.GetFenceStatus(nil)
			return err
		}},
		{"WaitForFence_nil", func() error {
			_, err := device.WaitForFence(nil, 0, time.Millisecond)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, wgpu.ErrReleased) {
				t.Errorf("got %v, want ErrReleased", err)
			}
		})
	}
}

// =============================================================================
// Device.Poll tests — Covers device_native.go lines 942-961
// =============================================================================

func TestDevicePollOnNilDevice(t *testing.T) {
	// Ensure Poll on a nil-ish device does not panic.
	var device *wgpu.Device
	result := device.Poll(wgpu.PollPoll)
	if result {
		t.Error("Poll on nil device should return false")
	}
}

func TestDevicePollWait(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// Poll(PollWait) with no pending operations should return quickly.
	result := device.Poll(wgpu.PollWait)
	// No maps in flight, expect false.
	if result {
		t.Error("PollWait with no pending maps should return false")
	}
}

func TestDevicePollPoll(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// Poll(PollPoll) with no pending operations should return immediately.
	result := device.Poll(wgpu.PollPoll)
	if result {
		t.Error("PollPoll with no pending maps should return false")
	}
}

// =============================================================================
// Device.CreatePipelineLayout validation
// Covers device_native.go lines 286-331
// =============================================================================

func TestDeviceCreatePipelineLayoutReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label: "released-layout",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreatePipelineLayout after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceCreatePipelineLayoutNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	_, err := device.CreatePipelineLayout(nil)
	if err == nil {
		t.Fatal("CreatePipelineLayout(nil) should return error")
	}
}

// =============================================================================
// Device.CreateBindGroup validation
// Covers device_native.go lines 334-436
// =============================================================================

func TestDeviceCreateBindGroupReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label: "released-bg",
	})
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateBindGroup after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceCreateBindGroupNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	_, err := device.CreateBindGroup(nil)
	if err == nil {
		t.Fatal("CreateBindGroup(nil) should return error")
	}
}

// =============================================================================
// Device.CreateTextureView validation
// Covers device_native.go lines 126-157
// =============================================================================

func TestDeviceCreateTextureViewReleasedDevice(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	_, err := device.CreateTextureView(nil, nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("CreateTextureView after Release: got %v, want ErrReleased", err)
	}
}

func TestDeviceCreateTextureViewNilTexture(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	_, err := device.CreateTextureView(nil, nil)
	if err == nil {
		t.Fatal("CreateTextureView(nil texture) should return error")
	}
}

func TestDeviceCreateTextureView(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "view-texture",
		Size:          wgpu.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	if view == nil {
		t.Fatal("CreateTextureView returned nil")
	}
	view.Release()
}

func TestDeviceCreateTextureViewWithDescriptor(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "view-desc-texture",
		Size:          wgpu.Extent3D{Width: 32, Height: 32, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label:           "test-view",
		Format:          wgpu.TextureFormatRGBA8Unorm,
		BaseMipLevel:    0,
		MipLevelCount:   1,
		BaseArrayLayer:  0,
		ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView with desc: %v", err)
	}
	if view == nil {
		t.Fatal("CreateTextureView with desc returned nil")
	}
	view.Release()
}

// =============================================================================
// Wrap tests — NewDeviceFromHAL, NewSurfaceFromHAL, etc.
// Covers wrap.go lines 19-109
// =============================================================================

func TestNewDeviceFromHALNilDevice(t *testing.T) {
	_, err := wgpu.NewDeviceFromHAL(nil, nil, 0, wgpu.DefaultLimits(), "test")
	if err == nil {
		t.Fatal("NewDeviceFromHAL(nil device) should fail")
	}
}

func TestHalDeviceOnNilQueue(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// HalQueue returns nil when queue is not HAL-backed. On mock device
	// the queue may be nil.
	_ = device.HalQueue()
}
