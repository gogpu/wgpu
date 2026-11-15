//go:build !rust && !(js && wasm)

package resource

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

// NewSampler builds a sampler over a HAL object and the shared device view.
func NewSampler(hal hal.Sampler, device core.DeviceView, ref *core.ResourceRef, mag, min, mipmap gputypes.FilterMode) *Sampler {
	return &Sampler{hal: hal, device: device, ref: ref, magFilter: mag, minFilter: min, mipmapFilter: mipmap}
}

// HAL returns the HAL sampler, or nil.
func (s *Sampler) HAL() hal.Sampler {
	if s == nil {
		return nil
	}
	return s.hal
}

// Ref returns the sampler's reference count, or nil for one without it.
func (s *Sampler) Ref() *core.ResourceRef {
	if s == nil {
		return nil
	}
	return s.ref
}

// MagFilter returns the sampler's magnification filter mode.
func (s *Sampler) MagFilter() gputypes.FilterMode {
	if s == nil {
		return 0
	}
	return s.magFilter
}

// MinFilter returns the sampler's minification filter mode.
func (s *Sampler) MinFilter() gputypes.FilterMode {
	if s == nil {
		return 0
	}
	return s.minFilter
}

// MipmapFilter returns the sampler's mipmap filter mode.
func (s *Sampler) MipmapFilter() gputypes.FilterMode {
	if s == nil {
		return 0
	}
	return s.mipmapFilter
}

// IsReleased reports whether Release has run on the sampler.
func (s *Sampler) IsReleased() bool {
	return s != nil && s.released
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
