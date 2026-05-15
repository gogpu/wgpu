//go:build js && wasm

package wgpu

import "github.com/gogpu/wgpu/internal/browser"

// Sampler represents a texture sampler.
type Sampler struct {
	browser  *browser.Sampler
	released bool
}

// Release destroys the sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
}
