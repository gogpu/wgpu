//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Sampler represents a texture sampler.
type Sampler struct {
	hal      hal.Sampler
	device   *Device
	released bool
	// ref counts the sampler's owners: the application until Release, and
	// every bind group that binds it until that bind group is destroyed, as
	// Rust wgpu-core's BindGroup holds an Arc<Sampler>. The HAL sampler is
	// destroyed when the last one drops (ADR-056).
	ref *core.ResourceRef
	// Filter modes retained for bind-time float32-filterable validation (VAL-C21).
	magFilter    gputypes.FilterMode
	minFilter    gputypes.FilterMode
	mipmapFilter gputypes.FilterMode
}

// newSamplerRef returns the reference count of a HAL sampler: when its last
// owner drops it, the sampler is destroyed after the submissions in flight,
// as a bind group is.
func newSamplerRef(d *Device, halDevice hal.Device, halSampler hal.Sampler, label string) *core.ResourceRef {
	return core.NewResourceRef("Sampler:"+label, func() {
		dq := d.destroyQueue()
		if dq == nil {
			halDevice.DestroySampler(halSampler)
			return
		}
		dq.Defer(d.lastSubmissionIndex(), "Sampler", func() {
			halDevice.DestroySampler(halSampler)
		})
	})
}

// Release drops the application's reference to the sampler. The HAL sampler
// is destroyed once no bind group references it any more and the GPU has
// completed every submission that may use it: a sampler released after a bind
// group that binds it was recorded stays valid for that work (P1-CORE-020).
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
	if s.ref != nil {
		s.ref.Drop()
		return
	}

	// No reference count: destroy after the submissions in flight.
	halDevice := s.device.halDevice()
	if halDevice == nil {
		return
	}
	dq := s.device.destroyQueue()
	if dq == nil {
		halDevice.DestroySampler(s.hal)
		return
	}
	subIdx := s.device.lastSubmissionIndex()
	halSampler := s.hal
	dq.Defer(subIdx, "Sampler", func() {
		halDevice.DestroySampler(halSampler)
	})
}
