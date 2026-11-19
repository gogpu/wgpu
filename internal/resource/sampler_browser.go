//go:build js && wasm

package resource

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/browser"
)

// Sampler represents a texture sampler.
type Sampler struct {
	browser *browser.Sampler
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewSampler builds a sampler over a browser sampler object.
func NewSampler(b *browser.Sampler) *Sampler {
	return &Sampler{browser: b, released: new(atomic.Bool)}
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
	if s.released == nil || !s.released.CompareAndSwap(false, true) {
		return
	}
}
