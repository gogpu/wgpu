//go:build !(js && wasm)

package track

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TextureUses represents internal texture usage states for tracking.
// These are more granular than gputypes.TextureUsage for precise barrier
// insertion and usage conflict detection.
//
// The flags mirror Rust wgpu-types TextureUses (wgpu-types/src/texture.rs:215).
// Two composite constants partition the flags:
//   - Ordered  — hardware guarantees access order; no barrier when state unchanged
//   - Exclusive — require sole access; barrier always needed on transition
//
// Reference: wgpu-core track/texture.rs (ResourceUses impl for TextureUses)
type TextureUses uint32

// Texture usage flags for state tracking.
const (
	TextureUsesNone              TextureUses = 0
	TextureUsesUninitialized     TextureUses = 1 << 0 // Unknown/junk contents
	TextureUsesPresent           TextureUses = 1 << 1 // Ready for surface presentation
	TextureUsesCopySrc           TextureUses = 1 << 2 // Source of a hardware copy
	TextureUsesCopyDst           TextureUses = 1 << 3 // Destination of a hardware copy
	TextureUsesResource          TextureUses = 1 << 4 // Read-only sampled or fetched
	TextureUsesColorTarget       TextureUses = 1 << 5 // Render pass color target
	TextureUsesDepthStencilRead  TextureUses = 1 << 6 // Read-only depth/stencil
	TextureUsesDepthStencilWrite TextureUses = 1 << 7 // Read-write depth/stencil
	TextureUsesStorageRead       TextureUses = 1 << 8 // Storage texture read-only
	TextureUsesStorageWrite      TextureUses = 1 << 9 // Storage texture write-only
)

// textureUsesInclusive is the combination of states that may coexist.
// Reference: wgpu-types TextureUses::INCLUSIVE
const textureUsesInclusive = TextureUsesCopySrc | TextureUsesResource |
	TextureUsesDepthStencilRead | TextureUsesStorageRead

// textureUsesExclusive is the combination of states that require sole access.
// Reference: wgpu-types TextureUses::EXCLUSIVE
const textureUsesExclusive = TextureUsesCopyDst | TextureUsesColorTarget |
	TextureUsesDepthStencilWrite | TextureUsesStorageWrite | TextureUsesPresent

// textureUsesOrdered is the combination of usages guaranteed to be ordered
// by hardware. If a texture stays in an ordered state between operations, no
// barrier is needed.
// Reference: wgpu-types TextureUses::ORDERED
const textureUsesOrdered = textureUsesInclusive | TextureUsesColorTarget |
	TextureUsesDepthStencilWrite | TextureUsesStorageRead

// IsReadOnly returns true if the usage contains only read-only operations.
func (u TextureUses) IsReadOnly() bool {
	return u&textureUsesExclusive == 0
}

// IsEmpty returns true if no usage flags are set.
func (u TextureUses) IsEmpty() bool {
	return u == TextureUsesNone
}

// Contains returns true if all flags in other are present in u.
func (u TextureUses) Contains(other TextureUses) bool {
	return u&other == other
}

// AllOrdered returns true if all set flags are in the ordered set.
// When all usages are ordered, hardware guarantees access ordering.
// Reference: wgpu-core track/mod.rs ResourceUses::all_ordered()
func (u TextureUses) AllOrdered() bool {
	return u&^textureUsesOrdered == 0
}

// IsExclusive returns true if any exclusive usage flag is set.
// Reference: wgpu-core track/mod.rs ResourceUses::any_exclusive()
func (u TextureUses) IsExclusive() bool {
	return u&textureUsesExclusive != 0
}

// IsCompatible returns true if two usages can coexist without a barrier.
// Read-only (inclusive) usages are compatible with each other.
// Any exclusive usage requires sole access unless the exact same flag.
//
// Implements the Rust wgpu-core rule: any(inclusive) XOR one(exclusive).
// Reference: wgpu-core track/mod.rs invalid_resource_state()
func (u TextureUses) IsCompatible(other TextureUses) bool {
	if u.IsEmpty() || other.IsEmpty() {
		return true
	}
	combined := u | other
	// If any exclusive bit is set in the combined state, only a single
	// usage flag may be active (power of two = exactly one bit set).
	if combined.IsExclusive() {
		return isPowerOfTwo(uint32(combined))
	}
	// All inclusive — compatible.
	return true
}

// isPowerOfTwo returns true when exactly one bit is set.
func isPowerOfTwo(v uint32) bool {
	return v != 0 && (v&(v-1)) == 0
}

// SkipBarrier returns true if transitioning from old to new does NOT
// require a barrier.
//
// A barrier can be skipped when:
//  1. The state did not change, AND
//  2. All usages in the state are ordered by hardware
//
// Reference: wgpu-core track/mod.rs skip_barrier()
func SkipBarrier(old, next TextureUses) bool {
	return old == next && old.AllOrdered()
}

// ToTextureUsage converts internal uses to gputypes.TextureUsage for HAL.
func (u TextureUses) ToTextureUsage() gputypes.TextureUsage {
	var result gputypes.TextureUsage

	if u&TextureUsesCopySrc != 0 {
		result |= gputypes.TextureUsageCopySrc
	}
	if u&TextureUsesCopyDst != 0 {
		result |= gputypes.TextureUsageCopyDst
	}
	if u&(TextureUsesResource|TextureUsesDepthStencilRead) != 0 {
		result |= gputypes.TextureUsageTextureBinding
	}
	if u&(TextureUsesStorageRead|TextureUsesStorageWrite) != 0 {
		result |= gputypes.TextureUsageStorageBinding
	}
	if u&(TextureUsesColorTarget|TextureUsesDepthStencilWrite) != 0 {
		result |= gputypes.TextureUsageRenderAttachment
	}

	return result
}

// TextureState holds the tracked state for a single texture.
type TextureState struct {
	usage TextureUses
}

// Usage returns the current usage.
func (s TextureState) Usage() TextureUses {
	return s.usage
}

// TextureTracker tracks texture usage states for a device.
// It is the device-level source of truth: each submitted command buffer's
// usage scope is merged into this tracker, which produces the barrier list.
//
// This mirrors Rust wgpu-core's DeviceTracker for textures
// (simplified to whole-texture granularity; per-subresource tracking is
// deferred to a future phase).
//
// Reference: wgpu-core track/texture.rs TextureTracker
type TextureTracker struct {
	states   []TextureState   // States indexed by TrackerIndex
	metadata ResourceMetadata // Tracks which indices are valid
}

// NewTextureTracker creates a new texture tracker.
func NewTextureTracker() *TextureTracker {
	return &TextureTracker{
		states:   make([]TextureState, 0, 64),
		metadata: NewResourceMetadata(),
	}
}

// InsertSingle tracks a new texture with initial usage.
func (t *TextureTracker) InsertSingle(index TrackerIndex, usage TextureUses) {
	t.ensureSize(int(index) + 1)
	t.states[index] = TextureState{usage: usage}
	t.metadata.SetOwned(index, true)
}

// Remove stops tracking a texture.
func (t *TextureTracker) Remove(index TrackerIndex) {
	if int(index) < len(t.states) {
		t.states[index] = TextureState{}
		t.metadata.SetOwned(index, false)
	}
}

// GetUsage returns the current usage of a texture.
func (t *TextureTracker) GetUsage(index TrackerIndex) TextureUses {
	if int(index) < len(t.states) && t.metadata.IsOwned(index) {
		return t.states[index].usage
	}
	return TextureUsesNone
}

// SetUsage updates the usage of a tracked texture.
func (t *TextureTracker) SetUsage(index TrackerIndex, usage TextureUses) {
	if int(index) < len(t.states) && t.metadata.IsOwned(index) {
		t.states[index].usage = usage
	}
}

// IsTracked returns true if the texture is being tracked.
func (t *TextureTracker) IsTracked(index TrackerIndex) bool {
	return int(index) < len(t.states) && t.metadata.IsOwned(index)
}

// Size returns the number of tracked textures.
func (t *TextureTracker) Size() int {
	return t.metadata.Count()
}

// ensureSize grows the state vector if needed.
func (t *TextureTracker) ensureSize(size int) {
	for len(t.states) < size {
		t.states = append(t.states, TextureState{})
	}
}

// Merge merges usage from scope into tracker, returning needed transitions.
// Called during queue submit to synchronize command buffer state with device
// state. Each returned TexturePendingTransition should be converted to a
// hal.TextureBarrier and emitted before the corresponding command buffer.
//
// Reference: wgpu-core track/texture.rs TextureTracker::set_from_usage_scope
func (t *TextureTracker) Merge(scope *TextureUsageScope) []TexturePendingTransition {
	var transitions []TexturePendingTransition

	for i := range scope.states {
		if i < 0 || i > int(^TrackerIndex(0)-1) {
			continue
		}
		index := TrackerIndex(i)
		if !scope.metadata.IsOwned(index) {
			continue
		}

		newUsage := scope.states[i].usage
		oldUsage := t.GetUsage(index)

		// If texture not tracked in device, add it
		if !t.IsTracked(index) {
			t.InsertSingle(index, newUsage)
			continue
		}

		// Check if transition is needed using the skip_barrier rule
		if SkipBarrier(oldUsage, newUsage) {
			continue
		}

		// State changed or not all ordered — emit a barrier
		transitions = append(transitions, TexturePendingTransition{
			Index: index,
			Usage: TextureStateTransition{
				From: oldUsage,
				To:   newUsage,
			},
		})
		t.states[index].usage = newUsage
	}

	return transitions
}

// TextureUsageScope tracks texture usage within a command buffer or pass.
// Each command buffer has its own scope that gets merged into the device
// tracker on submit.
//
// Reference: wgpu-core track/texture.rs TextureUsageScope
type TextureUsageScope struct {
	states   []TextureState
	metadata ResourceMetadata
}

// NewTextureUsageScope creates a new usage scope.
func NewTextureUsageScope() *TextureUsageScope {
	return &TextureUsageScope{
		states:   make([]TextureState, 0, 32),
		metadata: NewResourceMetadata(),
	}
}

// SetUsage sets the usage for a texture in this scope.
// Returns error if the texture already has an incompatible usage.
//
// Reference: wgpu-core track/texture.rs TextureUsageScope::merge_single
func (s *TextureUsageScope) SetUsage(index TrackerIndex, usage TextureUses) error {
	s.ensureSize(int(index) + 1)

	if s.metadata.IsOwned(index) {
		existing := s.states[index].usage
		combined := existing | usage
		if !combined.IsCompatible(combined) {
			return &TextureUsageConflictError{
				Index:    index,
				Existing: existing,
				New:      usage,
			}
		}
		// Merge usages if compatible
		s.states[index].usage = combined
	} else {
		s.states[index] = TextureState{usage: usage}
		s.metadata.SetOwned(index, true)
	}

	return nil
}

// ReplaceUsage replaces the usage recorded for a texture. It is used after an
// explicit texture transition, where the scope must describe the state after
// the transition rather than merge incompatible before/after usages.
func (s *TextureUsageScope) ReplaceUsage(index TrackerIndex, usage TextureUses) {
	s.ensureSize(int(index) + 1)
	s.states[index] = TextureState{usage: usage}
	s.metadata.SetOwned(index, true)
}

// GetUsage returns the current usage in this scope.
func (s *TextureUsageScope) GetUsage(index TrackerIndex) TextureUses {
	if int(index) < len(s.states) && s.metadata.IsOwned(index) {
		return s.states[index].usage
	}
	return TextureUsesNone
}

// IsUsed returns true if the texture is used in this scope.
func (s *TextureUsageScope) IsUsed(index TrackerIndex) bool {
	return int(index) < len(s.states) && s.metadata.IsOwned(index)
}

// IsEmpty returns true if no textures are tracked in this scope.
func (s *TextureUsageScope) IsEmpty() bool {
	return s.metadata.Count() == 0
}

// Clear resets the scope for reuse.
func (s *TextureUsageScope) Clear() {
	s.states = s.states[:0]
	s.metadata.Clear()
}

// ensureSize grows the state vector if needed.
func (s *TextureUsageScope) ensureSize(size int) {
	for len(s.states) < size {
		s.states = append(s.states, TextureState{})
	}
}

// TexturePendingTransition represents a texture state transition that needs
// a barrier. Produced by TextureTracker.Merge() when a command buffer's
// usage scope requires a different state than the device tracker's current
// state.
//
// Reference: wgpu-core track/mod.rs PendingTransition<TextureUses>
type TexturePendingTransition struct {
	Index TrackerIndex
	Usage TextureStateTransition
}

// TextureStateTransition represents a from-to state change for a texture.
type TextureStateTransition struct {
	From TextureUses
	To   TextureUses
}

// NeedsBarrier returns true if this transition requires a barrier.
func (t TextureStateTransition) NeedsBarrier() bool {
	return !SkipBarrier(t.From, t.To)
}

// IntoHAL converts a pending transition to a HAL texture barrier.
// The caller provides the hal.Texture handle.
func (p TexturePendingTransition) IntoHAL(texture hal.Texture) hal.TextureBarrier {
	return hal.TextureBarrier{
		Texture: texture,
		Usage: hal.TextureUsageTransition{
			OldUsage: p.Usage.From.ToTextureUsage(),
			NewUsage: p.Usage.To.ToTextureUsage(),
		},
	}
}

// TextureUsageConflictError is returned when incompatible texture usages
// are detected within the same scope.
type TextureUsageConflictError struct {
	Index    TrackerIndex
	Existing TextureUses
	New      TextureUses
}

// Error implements the error interface.
func (e *TextureUsageConflictError) Error() string {
	return fmt.Sprintf("track: texture %d usage conflict: existing %d incompatible with %d",
		e.Index, e.Existing, e.New)
}
