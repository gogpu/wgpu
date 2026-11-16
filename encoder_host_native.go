//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// encoderDeviceHost adapts *Device to the encoder domain's DeviceHost seam
// (ADR-070 shape B, user decision 2026-11-16): the encoder reaches the pool,
// the recordings registry and the stencil upload through it. The concrete
// pool and registry stay in this package until S11 moves them with the Device
// into internal/device; the adapter is the only place the destination's seam
// meets them.
type encoderDeviceHost struct{ d *Device }

var _ encoder.DeviceHost = encoderDeviceHost{}

// Recordings returns the device's recordings registry.
func (h encoderDeviceHost) Recordings() core.EncoderRecordings[*encoder.CommandEncoder, *encoder.CommandBuffer] {
	return &h.d.recordings
}

// EncoderPool returns the device's HAL command encoder pool, or an untyped nil
// when the device has none: the encoder's nil checks must hold across the
// interface (a nil *encoderPool boxed in it would defeat them).
func (h encoderDeviceHost) EncoderPool() core.HalEncoderPool {
	if h.d.cmdEncoderPool == nil {
		return nil
	}
	return h.d.cmdEncoderPool
}

// NewStencilUpload creates the staging upload of size into the stencil of dst
// at mipLevel and origin: the payload moved to internal/stencilblit at S9, so
// the host hands it the seam's implementation handle and the stencil host —
// the only place the payload meets the device's creation paths.
func (h encoderDeviceHost) NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (*stencilblit.Upload, error) {
	return stencilblit.NewUpload(newStencilDeviceHost(h.d), dst, mipLevel, origin, size)
}

// Features returns the device's enabled features: the render pass reads them
// for its draw feature gates (MultiDrawIndirect and MultiDrawIndirectCount),
// as the pre-move pass read them from *Device directly.
func (h encoderDeviceHost) Features() gputypes.Features {
	return h.d.Features()
}
