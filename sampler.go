package wgpu

import "github.com/gogpu/wgpu/hal"

// Sampler represents a texture sampler.
type Sampler struct {
	hal      hal.Sampler
	device   *Device
	released bool
}

// Release destroys the sampler. Destruction is deferred until the GPU
// completes any submission that may reference this sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true

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
