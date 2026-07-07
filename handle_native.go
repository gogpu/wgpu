//go:build !rust && !(js && wasm)

package wgpu

import (
	"unsafe"

	"github.com/gogpu/gpucontext"
)

// DeviceToHandle wraps a *Device as a gpucontext.Device opaque handle.
// The pointer is preserved as an unsafe.Pointer so cross-package consumers
// can type-assert or call Device.Pointer() without importing wgpu directly.
func DeviceToHandle(d *Device) gpucontext.Device {
	return gpucontext.NewDevice(unsafe.Pointer(d)) //nolint:gosec // G103: intentional unsafe for opaque handle
}

// DeviceFromHandle extracts a *Device from a gpucontext.Device handle
// previously created by DeviceToHandle. Returns nil if the handle is nil or
// does not hold a *Device.
func DeviceFromHandle(h gpucontext.Device) *Device {
	if h.IsNil() {
		return nil
	}
	return (*Device)(h.Pointer()) //nolint:gosec // G103: intentional unsafe for opaque handle
}

// QueueToHandle wraps a *Queue as a gpucontext.Queue opaque handle.
func QueueToHandle(q *Queue) gpucontext.Queue {
	return gpucontext.NewQueue(unsafe.Pointer(q)) //nolint:gosec // G103: intentional unsafe for opaque handle
}

// QueueFromHandle extracts a *Queue from a gpucontext.Queue handle
// previously created by QueueToHandle. Returns nil if the handle is nil.
func QueueFromHandle(h gpucontext.Queue) *Queue {
	if h.IsNil() {
		return nil
	}
	return (*Queue)(h.Pointer()) //nolint:gosec // G103: intentional unsafe for opaque handle
}

// AdapterToHandle wraps an *Adapter as a gpucontext.Adapter opaque handle.
func AdapterToHandle(a *Adapter) gpucontext.Adapter {
	return gpucontext.NewAdapter(unsafe.Pointer(a)) //nolint:gosec // G103: intentional unsafe for opaque handle
}

// AdapterFromHandle extracts an *Adapter from a gpucontext.Adapter handle
// previously created by AdapterToHandle. Returns nil if the handle is nil.
func AdapterFromHandle(h gpucontext.Adapter) *Adapter {
	if h.IsNil() {
		return nil
	}
	return (*Adapter)(h.Pointer()) //nolint:gosec // G103: intentional unsafe for opaque handle
}
