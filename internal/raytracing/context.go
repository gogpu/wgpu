package raytracing

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// DeviceContext provides access to device-level operations needed by the
// ray tracing package. This interface breaks the import cycle between
// internal/raytracing/ and core/ — core.Device implements this interface
// and passes itself when calling RT functions.
//
// Pattern: same as ssa.Frontend in Go compiler (internal package defines
// callback interface, parent implements it).
type DeviceContext interface {
	// CreateBuffer creates a GPU buffer for scratch or instance data.
	CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error)

	// DestroyBuffer releases a GPU buffer.
	DestroyBuffer(buffer hal.Buffer)

	// HALDevice returns the underlying HAL device for acceleration
	// structure operations (create, destroy, build size queries).
	HALDevice() hal.Device

	// DeviceFeatures returns the features enabled on this device.
	// Used to validate that RT features are present before operations.
	DeviceFeatures() gputypes.Features

	// DeviceLimits returns the resource limits of this device.
	// Used to validate geometry counts, instance counts, etc.
	DeviceLimits() gputypes.Limits

	// DeviceAlignments returns the alignment requirements of this device.
	// Used for scratch buffer alignment during AS builds.
	DeviceAlignments() hal.Alignments
}
