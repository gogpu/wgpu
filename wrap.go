//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pendingwrites"
)

// newDeviceFromHAL is the constructor behind the wgputest factory and the
// test hooks. The public NewDeviceFromHAL leaves the API in v0.35.0 (ADR-070
// §15: no internal type in a public signature); a consumer that needs a device
// around HAL objects asks the backend or the wgputest seam instead.
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
	// The Rust backend doesn't go through the adapter registry, so we
	// create a minimal adapter that satisfies the core.Device requirements.
	coreAdapter := &core.Adapter{
		Features: features,
		Limits:   limits,
	}

	coreDevice := core.NewDevice(halDevice, coreAdapter, features, limits, label)

	// Single shared encoder pool for both user command encoders and PendingWrites.
	pool := newEncoderPool(halDevice)

	q := newQueue(halQueue, pendingwrites.New(halDevice, halQueue, pool, coreDevice.Tracker()))

	coreDevice.SetAssociatedQueue(&core.Queue{Label: label + " Queue"})

	device := &Device{
		core:           coreDevice,
		queue:          q,
		cmdEncoderPool: pool,
	}
	q.impl.SetDevice(newQueueDeviceHost(device))

	return device
}
