package wgpu

import "github.com/gogpu/wgpu/hal"

// Fence is a GPU synchronization primitive.
// Fences allow CPU-GPU synchronization by signaling when submitted work completes.
//
// Fences are created via Device.CreateFence and should be released via Release()
// when no longer needed.
type Fence struct {
	hal      hal.Fence
	device   *Device
	released bool
}

// Release destroys the fence.
// After this call, the fence must not be used.
func (f *Fence) Release() {
	if f.released {
		return
	}
	f.released = true
	halDevice := f.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyFence(f.hal)
	}
}
