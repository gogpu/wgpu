//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pendingwrites"
)

// newDeviceFromHAL is the constructor behind the wgputest factory and the
// test hooks. The public NewDeviceFromHAL leaves the API in v0.35.0 (ADR-070
// §15: no internal type in a public signature); a consumer that needs a device
// around HAL objects asks the backend or the wgputest seam instead.
//
// It is the root bridge of the S11 Move: the core device, the queue and the
// pending-write batch are built here, where internal/pendingwrites is a legal
// import, and the implementation is handed the three objects it owns. Building
// them inside internal/device would add a device -> pendingwrites edge the
// golden does not cover (ADR-070 S11 §8.5).
//
// Callers must pass non-nil HAL objects.
func newDeviceFromHAL(
	halDevice hal.Device,
	halQueue hal.Queue,
	features gputypes.Features,
	limits gputypes.Limits,
	label string,
) *Device {
	// Create a core.Adapter stub for the core.Device constructor.
	coreAdapter := &core.Adapter{
		Features: features,
		Limits:   limits,
	}

	coreDevice := core.NewDevice(halDevice, coreAdapter, features, limits, label)

	// Single shared encoder pool for both user command encoders and PendingWrites.
	pool := encoder.NewPool(halDevice)

	q := newQueue(halQueue, pendingwrites.New(halDevice, halQueue, pool, coreDevice.Tracker()))

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	impl := device.New(coreDevice, q.impl, pool)
	dev := &Device{impl: impl, queue: q}
	q.impl.SetDevice(impl)

	return dev
}
