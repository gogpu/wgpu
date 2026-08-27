//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/raytracing"
)

// deviceRTContext adapts core.Device to the raytracing.DeviceContext interface.
// This breaks the import cycle: internal/raytracing defines the interface,
// core.Device implements it via this thin adapter.
type deviceRTContext struct {
	device *Device
	guard  SnatchGuard
}

func (c *deviceRTContext) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	raw := c.device.Raw(c.guard)
	if raw == nil {
		return nil, ErrDeviceLost
	}
	return raw.CreateBuffer(desc)
}

func (c *deviceRTContext) DestroyBuffer(buffer hal.Buffer) {
	raw := c.device.Raw(c.guard)
	if raw == nil {
		return
	}
	raw.DestroyBuffer(buffer)
}

func (c *deviceRTContext) HALDevice() hal.Device {
	return c.device.Raw(c.guard)
}

func (c *deviceRTContext) DeviceFeatures() gputypes.Features {
	return c.device.Features
}

func (c *deviceRTContext) DeviceLimits() gputypes.Limits {
	return c.device.Limits
}

func (c *deviceRTContext) DeviceAlignments() hal.Alignments {
	if c.device.adapter != nil && c.device.adapter.halCapabilities != nil {
		return c.device.adapter.halCapabilities.AlignmentsMask
	}
	return hal.Alignments{}
}

// Compile-time check.
var _ raytracing.DeviceContext = (*deviceRTContext)(nil)
