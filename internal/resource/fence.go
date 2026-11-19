//go:build !rust && !(js && wasm)

package resource

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// Fence is a GPU synchronization primitive.
// Fences allow CPU-GPU synchronization by signaling when submitted work completes.
//
// Fences are created via Device.CreateFence and should be released via Release()
// when no longer needed.
type Fence struct {
	hal    hal.Fence
	device core.DeviceView
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewFence builds a fence over a HAL object and the shared device view.
func NewFence(halFence hal.Fence, device core.DeviceView) *Fence {
	return &Fence{hal: halFence, device: device, released: new(atomic.Bool)}
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
	return f != nil && f.released != nil && f.released.Load()
}

// Release destroys the fence.
// After this call, the fence must not be used.
func (f *Fence) Release() {
	if f.released == nil || !f.released.CompareAndSwap(false, true) {
		return
	}
	halDevice := f.device.HALDevice()
	if halDevice != nil {
		halDevice.DestroyFence(f.hal)
	}
}
