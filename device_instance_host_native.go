//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/surface"
)

// deviceInstanceHost adapts the root Instance to device.InstanceHost, the
// device domain's seam view of the instance (ADR-070 S11). It exists because
// the instance stays root until S12: the seam may not name *Instance, and the
// per-surface retire must reach *surface.Surface, so the root host holds both
// pointers and projects.
//
// The host is built once per device, by Instance.adoptDevice, and lives as
// long as the device. It holds its own device backward, which setInstance
// fills at that moment; nothing else writes either field.
type deviceInstanceHost struct {
	i *Instance
	d *Device
}

var _ device.InstanceHost = (*deviceInstanceHost)(nil)

// newDeviceInstanceHost builds the host for the given instance. The device
// pointer is bound by setInstance, which is also the only caller.
func newDeviceInstanceHost(i *Instance) *deviceInstanceHost {
	return &deviceInstanceHost{i: i}
}

// SurfacesForDevice returns the surface implementations configured against the
// device behind view, as a teardown snapshot.
//
// The scan walks the instance's surface registry under its lock and filters by
// the core device identity, exactly as the pre-seam teardown scan did:
// Surface.Configure rejects a released device before assigning its device, and
// Device.Release marks itself released before reaching here, so the
// association is stable for the snapshot even though Surface has no second
// ownership lock.
func (h *deviceInstanceHost) SurfacesForDevice(view core.DeviceView) []*surface.Surface {
	if h == nil || h.i == nil || view == nil {
		return nil
	}
	coreDevice := view.Core()
	h.i.mu.Lock()
	defer h.i.mu.Unlock()
	surfaces := make([]*surface.Surface, 0, len(h.i.surfaces))
	for s := range h.i.surfaces {
		if s != nil && s.impl != nil && s.impl.SameDevice(coreDevice) {
			surfaces = append(surfaces, s.impl)
		}
	}
	return surfaces
}

// UnregisterDevice removes the device from the instance's registry. The
// registry is keyed by the root wrapper, which the host holds; the view adds
// nothing to the removal, so it is not read.
func (h *deviceInstanceHost) UnregisterDevice(_ core.DeviceView) {
	if h == nil || h.i == nil || h.d == nil {
		return
	}
	h.i.unregisterDevice(h.d)
}
