//go:build js && wasm

package wgpu

// Sampler represents a texture sampler.
type Sampler struct {
	released bool
}

// Release destroys the sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
}
