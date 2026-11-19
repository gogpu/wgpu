//go:build rust

package resource

import (
	"sync/atomic"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// Sampler represents a texture sampler.
// On Rust backend, this wraps go-webgpu/webgpu Sampler.
type Sampler struct {
	r *rwgpu.Sampler
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewSampler builds a sampler over a wgpu-native sampler object.
func NewSampler(r *rwgpu.Sampler) *Sampler {
	return &Sampler{r: r, released: new(atomic.Bool)}
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
	if s.released == nil || !s.released.CompareAndSwap(false, true) {
		return
	}
	if s.r != nil {
		s.r.Release()
	}
}
