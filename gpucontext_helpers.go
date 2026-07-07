package wgpu

import (
	"unsafe"

	"github.com/gogpu/gpucontext"
)

// Handle conversion helpers — isolate unsafe.Pointer from consumers.
// Consumers write wgpu.DeviceFromHandle(dev) instead of (*wgpu.Device)(dev.Pointer()).
// DIP: wgpu (implementation) depends on gpucontext (abstraction),
// like database/sql depends on database/sql/driver.

// DeviceFromHandle extracts *Device from a gpucontext.Device handle.
func DeviceFromHandle(h gpucontext.Device) *Device {
	if h.IsNil() {
		return nil
	}
	return (*Device)(h.Pointer())
}

// QueueFromHandle extracts *Queue from a gpucontext.Queue handle.
func QueueFromHandle(h gpucontext.Queue) *Queue {
	if h.IsNil() {
		return nil
	}
	return (*Queue)(h.Pointer())
}

// AdapterFromHandle extracts *Adapter from a gpucontext.Adapter handle.
func AdapterFromHandle(h gpucontext.Adapter) *Adapter {
	if h.IsNil() {
		return nil
	}
	return (*Adapter)(h.Pointer())
}

// SurfaceFromHandle extracts *Surface from a gpucontext.Surface handle.
func SurfaceFromHandle(h gpucontext.Surface) *Surface {
	if h.IsNil() {
		return nil
	}
	return (*Surface)(h.Pointer())
}

// InstanceFromHandle extracts *Instance from a gpucontext.Instance handle.
func InstanceFromHandle(h gpucontext.Instance) *Instance {
	if h.IsNil() {
		return nil
	}
	return (*Instance)(h.Pointer())
}

// DeviceToHandle wraps *Device into a gpucontext.Device handle.
func DeviceToHandle(d *Device) gpucontext.Device {
	return gpucontext.NewDevice(unsafe.Pointer(d)) //nolint:gosec // ADR-018 opaque handle
}

// QueueToHandle wraps *Queue into a gpucontext.Queue handle.
func QueueToHandle(q *Queue) gpucontext.Queue {
	return gpucontext.NewQueue(unsafe.Pointer(q)) //nolint:gosec // ADR-018 opaque handle
}

// AdapterToHandle wraps *Adapter into a gpucontext.Adapter handle.
func AdapterToHandle(a *Adapter) gpucontext.Adapter {
	return gpucontext.NewAdapter(unsafe.Pointer(a)) //nolint:gosec // ADR-018 opaque handle
}

// SurfaceToHandle wraps *Surface into a gpucontext.Surface handle.
func SurfaceToHandle(s *Surface) gpucontext.Surface {
	return gpucontext.NewSurface(unsafe.Pointer(s)) //nolint:gosec // ADR-018 opaque handle
}

// InstanceToHandle wraps *Instance into a gpucontext.Instance handle.
func InstanceToHandle(i *Instance) gpucontext.Instance {
	return gpucontext.NewInstance(unsafe.Pointer(i)) //nolint:gosec // ADR-018 opaque handle
}
