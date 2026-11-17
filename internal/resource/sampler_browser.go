//go:build js && wasm

package resource

import "github.com/gogpu/wgpu/internal/browser"

// Sampler represents a texture sampler.
type Sampler struct {
	browser  *browser.Sampler
	released bool
}

// NewSampler builds a sampler over a browser sampler object.
func NewSampler(b *browser.Sampler) *Sampler {
	return &Sampler{browser: b}
}

// Browser returns the browser sampler object, or nil.
func (s *Sampler) Browser() *browser.Sampler {
	if s == nil {
		return nil
	}
	return s.browser
}

// Release destroys the sampler.
func (s *Sampler) Release() {
	if s.released {
		return
	}
	s.released = true
}
