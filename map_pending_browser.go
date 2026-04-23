//go:build js && wasm

package wgpu

import "context"

// MapPending is a handle to an in-flight Buffer.MapAsync.
type MapPending struct{}

// Status returns the current state of the pending map.
func (p *MapPending) Status() (ready bool, err error) {
	panic("wgpu: browser backend not yet implemented")
}

// Wait blocks until the map resolves or ctx is canceled.
func (p *MapPending) Wait(ctx context.Context) error {
	panic("wgpu: browser backend not yet implemented")
}

// Release returns the handle to the pool.
func (p *MapPending) Release() {}
