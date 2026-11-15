//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// Sampler represents a texture sampler.
type Sampler struct {
	hal      hal.Sampler
	device   core.DeviceView
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
	halDevice := s.device.HALDevice()
	if halDevice == nil {
		return
	}
	dq := s.device.DestroyQueue()
	if dq == nil {
		halDevice.DestroySampler(s.hal)
		return
	}
	subIdx := s.device.LastSubmissionIndex()
	halSampler := s.hal
	dq.Defer(subIdx, "Sampler", func() {
		halDevice.DestroySampler(halSampler)
	})
}
