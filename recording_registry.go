//go:build !rust && !(js && wasm)

package wgpu

import "sync"

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

func (r *recordingRegistry) addEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().encoders[e] = struct{}{}
}

func (r *recordingRegistry) removeEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.setsLocked().encoders, e)
}

func (r *recordingRegistry) addBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.setsLocked().buffers[cb] = struct{}{}
}

func (r *recordingRegistry) removeBuffer(cb *CommandBuffer) {
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

// discardRecordings discards every command encoder and releases every
// command buffer still in the registry. Called by Device.Release while the
// backend device is alive, before the deferred destructions are flushed, so
// the resources they referenced are destroyed with the rest.
func (d *Device) discardRecordings() {
	encoders, buffers := d.recordings.take()
	for _, e := range encoders {
		e.DiscardEncoding()
	}
	for _, cb := range buffers {
		cb.Release()
	}
}
