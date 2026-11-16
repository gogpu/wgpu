//go:build !rust && !(js && wasm)

package wgpu

import (
	"sync"

	"github.com/gogpu/wgpu/internal/core"
)

// recordingRegistry holds the device's command encoders that are neither
// finished nor discarded, and its command buffers that are neither submitted
// nor released. Each holds ResourceRefs Clone()d while encoding; until it
// reaches the queue (or is released), nothing else drops them.
//
// Rust wgpu-core drops a command encoder's or command buffer's trackers when
// the object is dropped, and every resource keeps its device alive, so the
// device outlives them. Device.Release here destroys the device on request,
// so it first discards what this registry still holds: their references are
// dropped and the resources they kept alive are destroyed with the device
// instead of being left on it (VUID-vkDestroyDevice-device-05137 on Vulkan).
//
// The sets live behind a pointer so that Device, which embeds the registry by
// value, stays a comparable type, as it was before the registry existed.
type recordingRegistry struct {
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
func (r *recordingRegistry) setsLocked() *recordingSets {
	if r.sets == nil {
		r.sets = &recordingSets{
			encoders: make(map[*CommandEncoder]struct{}),
			buffers:  make(map[*CommandBuffer]struct{}),
		}
	}
	return r.sets
}

// AddEncoder records an encoder that has started recording.
func (r *recordingRegistry) AddEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().encoders[e] = struct{}{}
}

// RemoveEncoder forgets an encoder that finished or was discarded.
func (r *recordingRegistry) RemoveEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.setsLocked().encoders, e)
}

// AddBuffer records a command buffer awaiting submit or release.
func (r *recordingRegistry) AddBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().buffers[cb] = struct{}{}
}

// RemoveBuffer forgets a command buffer that was submitted or released.
func (r *recordingRegistry) RemoveBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.setsLocked().buffers, cb)
}

// take empties the registry and returns what it held.
func (r *recordingRegistry) take() ([]*CommandEncoder, []*CommandBuffer) {
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

// The registry implements the encoder seam's recordings interface declared in
// internal/core (ADR-070 shape B), instantiated here with the root encoder and
// command buffer types. S4's Move re-instantiates it over the internal/encoder
// types (the registry keeps naming them: a legal root -> internal/encoder
// edge) and the assertion follows that instantiation into the destination; the
// concrete registry itself moves to internal/device at S11.
var _ core.EncoderRecordings[*CommandEncoder, *CommandBuffer] = (*recordingRegistry)(nil)
