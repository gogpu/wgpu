//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/resource"
)

// queueDeviceHost adapts *Device to the queue domain's DeviceHost seam
// (ADR-070 shape B, user decision 2026-11-16): the queue reaches the device's
// core state, its encoder pool, user command encoders and the stencil upload
// through it. The concrete device and pool stay in this package until S11
// moves them into internal/device; this adapter is the only place the
// destination's seam meets them.
//
// Why an adapter and not the Device itself: the seam's Poll (embedded from
// core.DeviceView) takes core's PollType and the public Device.Poll takes the
// root PollType, and Go forbids two methods of one type from sharing a name
// even with different parameter types; the seam's other methods would also
// have to be public methods on Device naming internal types, which §15.1
// forbids. The embedded value is the shared S1 adapter (device_view_native.go),
// cached on the device and reused by every child that holds it.
type queueDeviceHost struct {
	d *Device
	core.DeviceView
}

var _ queue.DeviceHost = queueDeviceHost{}

// newQueueDeviceHost returns the seam value of d, or an untyped nil for a nil
// device: the queue's nil checks must keep their meaning across the interface.
func newQueueDeviceHost(d *Device) queue.DeviceHost {
	if d == nil {
		return nil
	}
	return queueDeviceHost{d: d, DeviceView: d.seamView()}
}

// EncoderPool returns the device's HAL command encoder pool, or an untyped nil
// when the device has none: the queue's nil checks must hold across the
// interface (a nil *encoderPool boxed in it would defeat them).
func (h queueDeviceHost) EncoderPool() core.HalEncoderPool {
	if h.d.cmdEncoderPool == nil {
		return nil
	}
	return h.d.cmdEncoderPool
}

// CreateCommandEncoder creates the encoder the queue records its stencil blit
// in. It is the public creation path, so the encoder joins the device's
// recordings and carries its test hooks; the queue works on the implementation
// handle, which is what it submits and discards.
func (h queueDeviceHost) CreateCommandEncoder(label string) (*encoder.CommandEncoder, error) {
	enc, err := h.d.CreateCommandEncoder(&CommandEncoderDescriptor{Label: label})
	if err != nil {
		return nil, err
	}
	return enc.impl, nil
}

// NewStencilUpload creates the staging upload of size into the stencil of dst
// at mipLevel and origin. dst is an implementation handle; the payload still
// works on the public Texture wrapper (it stays in this package through S9),
// so the adapter maps the handle back to the wrapper the texture's Pub cache
// holds — the same handle keeps the same owner.
func (h queueDeviceHost) NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (encoder.Upload, error) {
	return h.d.newStencilUpload(texturePub(dst), mipLevel, Origin3D(origin), Extent3D(size))
}

// texturePub returns the public wrapper of t: the one the creating device
// cached in t.Pub, or a wrapper around t for a texture the frontend built
// without a public handle. The wrapper is a thin handle and owns nothing the
// implementation does not.
func texturePub(t *resource.Texture) *Texture {
	pub, _ := t.Pub.(*Texture)
	if pub == nil {
		pub = &Texture{impl: t}
	}
	return pub
}
