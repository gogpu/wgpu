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
type recordingRegistry struct {
	mu       sync.Mutex
	encoders map[*CommandEncoder]struct{}
	buffers  map[*CommandBuffer]struct{}
}

func (r *recordingRegistry) addEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.encoders == nil {
		r.encoders = make(map[*CommandEncoder]struct{})
	}
	r.encoders[e] = struct{}{}
}

func (r *recordingRegistry) removeEncoder(e *CommandEncoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.encoders, e)
}

func (r *recordingRegistry) addBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buffers == nil {
		r.buffers = make(map[*CommandBuffer]struct{})
	}
	r.buffers[cb] = struct{}{}
}

func (r *recordingRegistry) removeBuffer(cb *CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buffers, cb)
}

// take empties the registry and returns what it held.
func (r *recordingRegistry) take() ([]*CommandEncoder, []*CommandBuffer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	encoders := make([]*CommandEncoder, 0, len(r.encoders))
	for e := range r.encoders {
		encoders = append(encoders, e)
	}
	buffers := make([]*CommandBuffer, 0, len(r.buffers))
	for cb := range r.buffers {
		buffers = append(buffers, cb)
	}
	clear(r.encoders)
	clear(r.buffers)
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
