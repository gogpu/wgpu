//go:build !(js && wasm)

package core

import (
	"sync"

	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

// DeviceTracker holds device-level resource usage trackers.
// During queue submit, each command buffer's per-resource usage scope is
// merged into this tracker, producing the barrier list that must be
// recorded before the command buffer executes.
//
// Every buffer and texture is inserted when it is created (NewBuffer,
// NewTexture), so a tracker index reused after a resource is freed starts
// from the new resource's initial state instead of the dead one's.
//
// All methods are safe for concurrent use: resources are created and freed
// from any goroutine while Queue.Submit merges command buffers. The lock is
// a leaf lock — no other lock is taken while it is held.
//
// This is the Go equivalent of Rust wgpu-core's DeviceTracker
// (device/resource.rs `trackers: Mutex<DeviceTracker>`). Covers both
// textures and buffers.
//
// Reference: wgpu-core track/mod.rs DeviceTracker
type DeviceTracker struct {
	mu       sync.Mutex
	textures *track.TextureTracker
	buffers  *track.BufferTracker
}

// NewDeviceTracker creates a device tracker with empty trackers.
func NewDeviceTracker() *DeviceTracker {
	return &DeviceTracker{
		textures: track.NewTextureTracker(),
		buffers:  track.NewBufferTracker(),
	}
}

// TextureUsage returns the device-level usage of a texture, or
// TextureUsesNone if the index is not tracked.
func (dt *DeviceTracker) TextureUsage(index track.TrackerIndex) track.TextureUses {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.textures.GetUsage(index)
}

// BufferUsage returns the device-level usage of a buffer, or BufferUsesNone
// if the index is not tracked.
func (dt *DeviceTracker) BufferUsage(index track.TrackerIndex) track.BufferUses {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.buffers.GetUsage(index)
}

// IsTextureTracked reports whether the texture index is tracked.
func (dt *DeviceTracker) IsTextureTracked(index track.TrackerIndex) bool {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.textures.IsTracked(index)
}

// IsBufferTracked reports whether the buffer index is tracked.
func (dt *DeviceTracker) IsBufferTracked(index track.TrackerIndex) bool {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.buffers.IsTracked(index)
}

// TextureCount returns the number of tracked textures.
func (dt *DeviceTracker) TextureCount() int {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.textures.Size()
}

// BufferCount returns the number of tracked buffers.
func (dt *DeviceTracker) BufferCount() int {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.buffers.Size()
}

// MergeTextureScope merges a command buffer's texture tracker (see
// CoreCommandBuffer.TextureScope) into the device tracker and returns the
// barriers that bring each texture from its device state into the state the
// command buffer first uses it in. Afterwards the device state of every
// texture is the state the command buffer leaves it in.
//
// Each returned TexturePendingTransition must be converted to a
// hal.TextureBarrier (via IntoHAL) and recorded into a command encoder
// before the command buffer executes.
//
// Reference: wgpu-core device/queue.rs pre_submit_for_command_buffers
func (dt *DeviceTracker) MergeTextureScope(cmdTracker *track.EncoderTextureTracker) []track.TexturePendingTransition {
	if cmdTracker == nil {
		return nil
	}
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.textures.MergeEncoder(cmdTracker)
}

// MergeBufferScope merges a command buffer's buffer tracker (see
// CoreCommandBuffer.BufferScope) into the device tracker and returns the
// barriers into each buffer's first usage in the command buffer. Afterwards
// the device state of every buffer is the command buffer's last usage.
//
// Each returned PendingTransition must be converted to a hal.BufferBarrier
// (via IntoHAL) and recorded into a command encoder before the command
// buffer executes.
//
// Reference: wgpu-core device/queue.rs pre_submit_for_command_buffers
func (dt *DeviceTracker) MergeBufferScope(cmdTracker *track.EncoderBufferTracker) []track.PendingTransition {
	if cmdTracker == nil {
		return nil
	}
	dt.mu.Lock()
	defer dt.mu.Unlock()
	return dt.buffers.MergeEncoder(cmdTracker)
}

// InsertBuffer registers a buffer in the device tracker with its initial
// usage, overwriting whatever state the index held before.
//
// Reference: wgpu-core track/buffer.rs DeviceBufferTracker::insert_single
func (dt *DeviceTracker) InsertBuffer(index track.TrackerIndex, usage track.BufferUses) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.buffers.InsertSingle(index, usage)
}

// RemoveBuffer removes a buffer from the device tracker.
func (dt *DeviceTracker) RemoveBuffer(index track.TrackerIndex) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.buffers.Remove(index)
}

// TrackPresentTexture records a swapchain texture's transition to Present
// state. This is called after all user command buffers are processed, to
// generate the final COLOR_ATTACHMENT_OPTIMAL -> PRESENT_SRC barrier.
//
// The trackerIndex must be a valid index for this texture in the device
// tracker. If the texture is not yet tracked, it will be added.
//
// Returns the barrier (from -> Present) or nil if no barrier is needed.
func (dt *DeviceTracker) TrackPresentTexture(trackerIndex track.TrackerIndex) *track.TexturePendingTransition {
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(trackerIndex, track.TextureUsesPresent)
	dt.mu.Lock()
	transitions := dt.textures.Merge(scope)
	dt.mu.Unlock()
	if len(transitions) == 0 {
		return nil
	}
	return &transitions[0]
}

// InsertTexture registers a texture in the device tracker with its initial
// usage, overwriting whatever state the index held before. Called when the
// texture is created (NewTexture) with TextureUsesUninitialized.
//
// Reference: wgpu-core track/texture.rs DeviceTextureTracker::insert_single
func (dt *DeviceTracker) InsertTexture(index track.TrackerIndex, usage track.TextureUses) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.textures.InsertSingle(index, usage)
}

// RemoveTexture removes a texture from the device tracker.
func (dt *DeviceTracker) RemoveTexture(index track.TrackerIndex) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.textures.Remove(index)
}

// BarrierCBFromTransitions creates a HAL command buffer containing texture
// barriers for the given transitions. The caller provides a HAL command
// encoder (from the encoder pool) and a function to resolve tracker indices
// to HAL textures.
//
// The caller MUST pass a non-empty transitions slice. Calling with an empty
// slice is a programming error (the caller should check len before calling).
//
// Returns the recorded command buffer, or an error if encoding fails.
// The caller must ensure the returned command buffer is included in the
// Submit call's command buffer list.
func BarrierCBFromTransitions(
	halEncoder hal.CommandEncoder,
	transitions []track.TexturePendingTransition,
	resolveTexture func(track.TrackerIndex) hal.Texture,
) (hal.CommandBuffer, error) {
	return BarrierCBFromAllTransitions(halEncoder, transitions, nil, resolveTexture, nil)
}

// BarrierCBFromAllTransitions creates a HAL command buffer containing both
// texture and buffer barriers. This is the generalized version that handles
// all resource types in a single preamble command buffer.
//
// Either textureTransitions or bufferTransitions (or both) must be non-empty.
//
// Returns the recorded command buffer, or an error if encoding fails.
func BarrierCBFromAllTransitions(
	halEncoder hal.CommandEncoder,
	textureTransitions []track.TexturePendingTransition,
	bufferTransitions []track.PendingTransition,
	resolveTexture func(track.TrackerIndex) hal.Texture,
	resolveBuffer func(track.TrackerIndex) hal.Buffer,
) (hal.CommandBuffer, error) {
	// Begin a short-lived encoding for the barrier commands
	if err := halEncoder.BeginEncoding("barrier-inject"); err != nil {
		return nil, err
	}

	// Emit texture barriers
	if len(textureTransitions) > 0 && resolveTexture != nil {
		barriers := make([]hal.TextureBarrier, 0, len(textureTransitions))
		for _, t := range textureTransitions {
			tex := resolveTexture(t.Index)
			if tex == nil {
				continue // destroyed texture — skip
			}
			barriers = append(barriers, t.IntoHAL(tex))
		}
		if len(barriers) > 0 {
			halEncoder.TransitionTextures(barriers)
		}
	}

	// Emit buffer barriers
	if len(bufferTransitions) > 0 && resolveBuffer != nil {
		barriers := make([]hal.BufferBarrier, 0, len(bufferTransitions))
		for _, t := range bufferTransitions {
			buf := resolveBuffer(t.Index)
			if buf == nil {
				continue // destroyed buffer — skip
			}
			barriers = append(barriers, t.IntoHAL(buf))
		}
		if len(barriers) > 0 {
			halEncoder.TransitionBuffers(barriers)
		}
	}

	return halEncoder.EndEncoding()
}
