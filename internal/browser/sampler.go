//go:build js && wasm

package browser

import "syscall/js"

// Sampler wraps a browser GPUSampler.
type Sampler struct {
	// ref_ is the GPUSampler JavaScript object.
	ref_ js.Value
}

// NewSampler constructs a Sampler from a GPUSampler js.Value.
func NewSampler(ref js.Value) *Sampler {
	return &Sampler{ref_: ref}
}

// Ref returns the underlying GPUSampler js.Value.
func (s *Sampler) Ref() js.Value { return s.ref_ }
