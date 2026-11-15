// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"context"

	"github.com/gogpu/wgpu/internal/resource"
)

// MapPending is a handle to an in-flight Buffer.MapAsync. The implementation
// is pooled in internal/resource; this declaration is the public surface.
type MapPending struct {
	impl *resource.MapPending
}

// Status reports the current state of the pending map without blocking.
func (p *MapPending) Status() (ready bool, err error) {
	if p == nil {
		return true, ErrMapCanceled
	}
	return p.impl.Status()
}

// Wait blocks until the pending map resolves or ctx is canceled.
func (p *MapPending) Wait(ctx context.Context) error {
	if p == nil {
		return ErrMapCanceled
	}
	return p.impl.Wait(ctx)
}

// Release returns the handle to the implementation's pool without waiting.
func (p *MapPending) Release() {
	if p == nil {
		return
	}
	p.impl.Release()
}
