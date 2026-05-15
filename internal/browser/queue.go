//go:build js && wasm

package browser

import "syscall/js"

// Queue wraps a browser GPUQueue.
//
// Phase 1 only holds the js.Value reference. Queue methods (submit, writeBuffer,
// writeTexture) will be implemented in Phase 3.
type Queue struct {
	// ref_ is the GPUQueue JavaScript object.
	ref_ js.Value
}

// NewQueue constructs a Queue from a GPUQueue js.Value.
func NewQueue(ref js.Value) *Queue {
	return &Queue{ref_: ref}
}

// Ref returns the underlying GPUQueue js.Value.
func (q *Queue) Ref() js.Value {
	return q.ref_
}
