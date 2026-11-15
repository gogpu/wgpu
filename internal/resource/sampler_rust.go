//go:build rust

package resource

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// Sampler represents a texture sampler.
// On Rust backend, this wraps go-webgpu/webgpu Sampler.
type Sampler struct {
	r        *rwgpu.Sampler
	released bool
}

// NewSampler builds a sampler over a wgpu-native sampler object.
func NewSampler(r *rwgpu.Sampler) *Sampler {
	return &Sampler{r: r}
}

// Rust returns the wgpu-native sampler object, or nil.
func (s *Sampler) Rust() *rwgpu.Sampler {
	if s == nil {
		return nil
	}
	return s.r
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
