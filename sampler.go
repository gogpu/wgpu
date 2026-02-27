package wgpu

import "github.com/gogpu/wgpu/hal"

// Sampler represents a texture sampler.
type Sampler struct {
	hal      hal.Sampler
	device   *Device
	released bool
}

// Release destroys the sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
	halDevice := s.device.halDevice()
	if halDevice != nil {
		halDevice.DestroySampler(s.hal)
	}
}
