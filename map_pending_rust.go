//go:build rust

package wgpu

import (
	"context"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// MapPending is a handle to an in-flight Buffer.MapAsync on the Rust backend.
// On Rust backend, this wraps go-webgpu/webgpu MapPending which uses
// wgpu-native's callback mechanism.
type MapPending struct {
	r   *rwgpu.MapPending
	buf *Buffer
}

// Status returns the current state of the pending map without blocking.
func (p *MapPending) Status() (ready bool, err error) {
	if p == nil || p.r == nil {
		return true, ErrMapCanceled
	}
	return p.r.Status()
}

// Wait blocks until the pending map resolves or ctx is canceled.
func (p *MapPending) Wait(ctx context.Context) error {
	if p == nil || p.r == nil {
		return ErrMapCanceled
	}
	return p.r.Wait(ctx)
}

// Release discards the MapPending handle.
func (p *MapPending) Release() {
	if p == nil {
		return
	}
	if p.r != nil {
		p.r.Release()
	}
	p.buf = nil
	p.r = nil
}
