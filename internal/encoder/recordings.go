//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package encoder

// This file holds the device's recordings registry. It arrived here from the
// root package with the device in the S11 Move (ADR-070 §15.1:
// encoder.Recordings). Its old root comment said it "moves to internal/device
// at S11"; the ADR's destination is this package, and the type is the
// internal/encoder instantiation of internal/core's EncoderRecordings seam.

import (
	"sync"
)

// Recordings holds the device's command encoders that are neither finished
// nor discarded, and its command buffers that are neither submitted nor
// released. Each holds ResourceRefs Clone()d while encoding; until it reaches
// the queue (or is released), nothing else drops them.
//
// Rust wgpu-core drops a command encoder's or command buffer's trackers when
// the object is dropped, and every resource keeps its device alive, so the
// device outlives them. Device.Release here destroys the device on request,
// so it first discards what this registry still holds: their references are
// dropped and the resources they kept alive are destroyed with the device
// instead of being left on it (VUID-vkDestroyDevice-device-05137 on Vulkan).
//
// The sets live behind a pointer so a zero value is usable and the struct
// stays copyable in the same way it was before the registry existed.
type Recordings struct {
	mu   sync.Mutex
	sets *recordingSets
}

// recordingSets holds the registry's contents; created on first use.
type recordingSets struct {
	encoders map[*CommandEncoder]struct{}
	buffers  map[*CommandBuffer]struct{}
}

// setsLocked returns the registry's sets, creating them on first use.
// The caller holds r.mu.
func (r *Recordings) setsLocked() *recordingSets {
	if r.sets == nil {
		r.sets = &recordingSets{
			encoders: make(map[*CommandEncoder]struct{}),
			buffers:  make(map[*CommandBuffer]struct{}),
		}
	}
	return r.sets
}

// AddEncoder records an encoder that has started recording.
func (r *Recordings) AddEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().encoders[e] = struct{}{}
}

// RemoveEncoder forgets an encoder that finished or was discarded.
func (r *Recordings) RemoveEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.setsLocked().encoders, e)
}

// AddBuffer records a command buffer awaiting submit or release.
func (r *Recordings) AddBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().buffers[cb] = struct{}{}
}

// RemoveBuffer forgets a command buffer that was submitted or released.
func (r *Recordings) RemoveBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.setsLocked().buffers, cb)
}

// Take empties the registry and returns what it held.
func (r *Recordings) Take() ([]*CommandEncoder, []*CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sets := r.setsLocked()
	encoders := make([]*CommandEncoder, 0, len(sets.encoders))
	for e := range sets.encoders {
		encoders = append(encoders, e)
	}
	buffers := make([]*CommandBuffer, 0, len(sets.buffers))
	for cb := range sets.buffers {
		buffers = append(buffers, cb)
	}
	clear(sets.encoders)
	clear(sets.buffers)
	return encoders, buffers
}
