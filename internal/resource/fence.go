//go:build !rust && !(js && wasm)

package resource

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// Fence is a GPU synchronization primitive.
// Fences allow CPU-GPU synchronization by signaling when submitted work completes.
//
// Fences are created via Device.CreateFence and should be released via Release()
// when no longer needed.
type Fence struct {
	hal      hal.Fence
	device   core.DeviceView
	released bool
}

// NewFence builds a fence over a HAL object and the shared device view.
func NewFence(halFence hal.Fence, device core.DeviceView) *Fence {
	return &Fence{hal: halFence, device: device}
}

// HAL returns the HAL fence, or nil.
func (f *Fence) HAL() hal.Fence {
	if f == nil {
		return nil
	}
	return f.hal
}

// IsReleased reports whether Release has run on the fence.
func (f *Fence) IsReleased() bool {
	return f != nil && f.released
}

// Release destroys the fence.
// After this call, the fence must not be used.
func (f *Fence) Release() {
	if f.released {
		return
	}
	f.released = true
	halDevice := f.device.HALDevice()
	if halDevice != nil {
		halDevice.DestroyFence(f.hal)
	}
}
