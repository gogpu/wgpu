//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

// DeviceTracker holds device-level resource usage trackers.
// During queue submit, each command buffer's per-resource usage scope is
// merged into this tracker, producing the barrier list that must be
// recorded before the command buffer executes.
//
// This is the Go equivalent of Rust wgpu-core's DeviceTracker
// (device/resource.rs). Covers both textures and buffers.
//
// Reference: wgpu-core track/mod.rs DeviceTracker
type DeviceTracker struct {
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

// Textures returns the device-level texture tracker.
func (dt *DeviceTracker) Textures() *track.TextureTracker {
	return dt.textures
}

// Buffers returns the device-level buffer tracker.
func (dt *DeviceTracker) Buffers() *track.BufferTracker {
	return dt.buffers
}

// MergeTextureScope merges a command buffer's texture usage scope into the
// device tracker and returns any barriers that need to be emitted.
//
// Each returned TexturePendingTransition must be converted to a
// hal.TextureBarrier (via IntoHAL) and recorded into a command encoder
// before the command buffer executes.
//
// Reference: wgpu-core device/queue.rs pre_submit_for_command_buffers
func (dt *DeviceTracker) MergeTextureScope(scope *track.TextureUsageScope) []track.TexturePendingTransition {
	if scope == nil {
		return nil
	}
	return dt.textures.Merge(scope)
}

// MergeBufferScope merges a command buffer's buffer usage scope into the
// device tracker and returns any barriers that need to be emitted.
//
// Each returned PendingTransition must be converted to a hal.BufferBarrier
// (via IntoHAL) and recorded into a command encoder before the command
// buffer executes.
//
// Reference: wgpu-core device/queue.rs pre_submit_for_command_buffers
func (dt *DeviceTracker) MergeBufferScope(scope *track.BufferUsageScope) []track.PendingTransition {
	if scope == nil {
		return nil
	}
	return dt.buffers.Merge(scope)
}

// InsertBuffer registers a buffer in the device tracker with its initial usage.
func (dt *DeviceTracker) InsertBuffer(index track.TrackerIndex, usage track.BufferUses) {
	dt.buffers.InsertSingle(index, usage)
}

// RemoveBuffer removes a buffer from the device tracker.
func (dt *DeviceTracker) RemoveBuffer(index track.TrackerIndex) {
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
	transitions := dt.textures.Merge(scope)
	if len(transitions) == 0 {
		return nil
	}
	return &transitions[0]
}

// InsertTexture registers a texture in the device tracker with its initial
// usage. Called when a texture is first used (e.g., acquired from swapchain).
func (dt *DeviceTracker) InsertTexture(index track.TrackerIndex, usage track.TextureUses) {
	dt.textures.InsertSingle(index, usage)
}

// RemoveTexture removes a texture from the device tracker.
// Called when a texture is destroyed.
func (dt *DeviceTracker) RemoveTexture(index track.TrackerIndex) {
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
