//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// Sampler represents a texture sampler.
// On Rust backend, this wraps go-webgpu/webgpu Sampler.
type Sampler struct {
	r        *rwgpu.Sampler
	device   *Device
	released bool
}

// Release destroys the sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
	if s.r != nil {
		s.r.Release()
	}
}
