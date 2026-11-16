//go:build !(js && wasm)

package track

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
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
//
// The HAL barrier type has no depth-stencil states, so the three states of a
// depth/stencil attachment are encoded in the public bits, and the backends
// read them together with the texture format (TextureUsesFromUsage is the
// inverse):
//
//   - DepthStencilWrite: RenderAttachment
//   - DepthStencilRead (read-only attachment): RenderAttachment | TextureBinding
//   - Resource (sampled): TextureBinding
//
// Before this split DepthStencilRead became TextureBinding alone, so a
// read-only depth attachment moved to the sampled state (P1-DX12-056). Rust
// passes the exact uses (hal::TextureUses); a precise HAL barrier type is
// planned with the HAL interface rework.
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
	if u&TextureUsesDepthStencilRead != 0 {
		result |= gputypes.TextureUsageRenderAttachment
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
//
// Deprecated: the trackers keep a texture's usage per mip level and array
// layer and no longer use TextureState; TextureTracker.GetUsage answers a
// texture's usage.
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
// Each texture has one usage while every subresource shares it, and one per
// mip level and array layer once they differ, so a barrier moves each
// subresource from its own state.
//
// Reference: wgpu-core track/texture.rs TextureTracker
type TextureTracker struct {
	states   []textureState   // States indexed by TrackerIndex
	metadata ResourceMetadata // Tracks which indices are valid
}

// NewTextureTracker creates a new texture tracker.
func NewTextureTracker() *TextureTracker {
	return &TextureTracker{
		states:   make([]textureState, 0, 64),
		metadata: NewResourceMetadata(),
	}
}

// InsertSingle tracks a new texture with initial usage for every
// subresource.
func (t *TextureTracker) InsertSingle(index TrackerIndex, usage TextureUses) {
	t.ensureSize(int(index) + 1)
	t.states[index].reset(usage)
	t.metadata.SetOwned(index, true)
}

// Remove stops tracking a texture.
func (t *TextureTracker) Remove(index TrackerIndex) {
	if int(index) < len(t.states) {
		t.states[index].reset(TextureUsesNone)
		t.metadata.SetOwned(index, false)
	}
}

// GetUsage returns the usage every subresource of a texture shares, or
// TextureUsesNone if their usages differ or the texture is not tracked.
func (t *TextureTracker) GetUsage(index TrackerIndex) TextureUses {
	if t.IsTracked(index) {
		return t.states[index].wholeUsage()
	}
	return TextureUsesNone
}

// SetUsage gives every subresource of a tracked texture usage.
func (t *TextureTracker) SetUsage(index TrackerIndex, usage TextureUses) {
	if t.IsTracked(index) {
		t.states[index].reset(usage)
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

// Merge merges usage from scope into tracker, returning needed transitions.
// Each subresource the scope uses moves from its own state to the scope's
// usage. A texture not tracked yet is inserted as MergeEncoder inserts it.
// Each returned TexturePendingTransition should be converted to a
// hal.TextureBarrier and emitted before the corresponding command buffer.
//
// Reference: wgpu-core track/texture.rs TextureTracker::set_from_usage_scope
func (t *TextureTracker) Merge(scope *TextureUsageScope) []TexturePendingTransition {
	var transitions textureTransitionList
	for i := range scope.states {
		index, valid := trackerIndexAt(i)
		if valid && scope.metadata.IsOwned(index) {
			t.mergeTexture(&transitions, index, &scope.states[i], &scope.states[i])
		}
	}
	return transitions
}

// SetSingle moves every subresource of a texture to usage and returns the
// barrier from its current state, unless SkipBarrier allows omitting it. An
// untracked index is treated as TextureUsesUninitialized.
//
// Deprecated: a texture whose subresources are in different states needs a
// barrier for each, and SetSingle returns only the first; use AppendSingle.
func (t *TextureTracker) SetSingle(index TrackerIndex, usage TextureUses) (TexturePendingTransition, bool) {
	var storage [1]TexturePendingTransition
	transitions := t.AppendSingle(storage[:0], index, TextureSelector{}, usage)
	if len(transitions) == 0 {
		return TexturePendingTransition{}, false
	}
	return transitions[0], true
}

// AppendSingle moves the subresources selector selects to usage and appends
// the barriers from their current states to transitions, but for those
// SkipBarrier allows omitting. Used by queue writes, which change device
// state outside any command buffer. An untracked index is treated as
// TextureUsesUninitialized.
//
// Reference: wgpu-core track/texture.rs DeviceTextureTracker::set_single
func (t *TextureTracker) AppendSingle(transitions []TexturePendingTransition, index TrackerIndex, selector TextureSelector, usage TextureUses) []TexturePendingTransition {
	if !t.IsTracked(index) {
		t.InsertSingle(index, TextureUsesUninitialized)
	}
	pendingTransitions := textureTransitionList(transitions)
	t.states[index].set(&pendingTransitions, index, selector, usage)
	return pendingTransitions
}

// MergeEncoder merges a submitted command buffer's tracker into the device
// tracker. For every subresource the command buffer uses, a transition from
// the device's current state to the command buffer's start state is
// returned (unless SkipBarrier allows omitting it), and the device state
// becomes the command buffer's end state. A texture the device does not
// track yet is inserted with the end state, with no barrier, when the
// command buffer first uses every subresource alike; otherwise its
// subresources start Uninitialized.
//
// Reference: wgpu-core track/texture.rs DeviceTextureTracker::set_from_tracker_and_drain_transitions
func (t *TextureTracker) MergeEncoder(enc *EncoderTextureTracker) []TexturePendingTransition {
	if enc == nil {
		return nil
	}
	var transitions textureTransitionList
	for i := range enc.end {
		index, valid := trackerIndexAt(i)
		if valid && enc.metadata.IsOwned(index) {
			t.mergeTexture(&transitions, index, &enc.start[i], &enc.end[i])
		}
	}
	return transitions
}

// ensureSize grows the state vector if needed.
func (t *TextureTracker) ensureSize(size int) {
	for len(t.states) < size {
		t.states = append(t.states, textureState{})
	}
}

// mergeTexture applies a use of a texture that first uses each subresource
// in start and leaves it in end. A texture not tracked yet takes end with no
// barrier when start is whole, and starts Uninitialized otherwise.
func (t *TextureTracker) mergeTexture(transitions *textureTransitionList, index TrackerIndex, start, end *textureState) {
	if !t.IsTracked(index) {
		t.InsertSingle(index, TextureUsesUninitialized)
		if start.isWhole() {
			t.states[index].copyFrom(end)
			return
		}
	}
	t.states[index].advance(transitions, index, start, end)
}

// TextureUsageScope tracks texture usage within ONE WebGPU usage scope: a
// render pass, a compute dispatch, or a single copy command. Adding a usage
// that is incompatible with an earlier usage of the same subresource in the
// same scope is a validation error; usages of different mip levels or array
// layers never conflict. Usages across different scopes are not merged
// here; the command buffer's EncoderTextureTracker transitions between them.
//
// Reference: wgpu-core track/texture.rs TextureUsageScope
type TextureUsageScope struct {
	states   []textureState
	metadata ResourceMetadata
}

// NewTextureUsageScope creates a new usage scope.
func NewTextureUsageScope() *TextureUsageScope {
	return &TextureUsageScope{
		states:   make([]textureState, 0, 32),
		metadata: NewResourceMetadata(),
	}
}

// SetUsage adds usage of every subresource of a texture to the scope.
// Returns an error, and changes nothing, if one of them already has an
// incompatible usage.
func (s *TextureUsageScope) SetUsage(index TrackerIndex, usage TextureUses) error {
	return s.SetSubresourceUsage(index, TextureSelector{}, usage)
}

// SetSubresourceUsage adds usage of the subresources selector selects to the
// scope. Returns an error, and changes nothing, if one of them already has an
// incompatible usage; usages of different subresources never conflict.
//
// Reference: wgpu-core track/texture.rs TextureUsageScope::merge_single
func (s *TextureUsageScope) SetSubresourceUsage(index TrackerIndex, selector TextureSelector, usage TextureUses) error {
	s.ensureSize(int(index) + 1)
	if !s.metadata.IsOwned(index) {
		s.states[index].reset(TextureUsesNone)
		s.metadata.SetOwned(index, true)
	}
	return s.states[index].merge(index, selector, usage)
}

// ReplaceUsage unconditionally gives every subresource of a texture usage. It
// is intended only after preflight validation, or after an explicit
// transition where the scope must describe the state after the encoded
// barrier.
func (s *TextureUsageScope) ReplaceUsage(index TrackerIndex, usage TextureUses) {
	s.ensureSize(int(index) + 1)
	s.states[index].reset(usage)
	s.metadata.SetOwned(index, true)
}

// GetUsage returns the usage every subresource of a texture shares in this
// scope, or TextureUsesNone if their usages differ or the texture is not
// used.
func (s *TextureUsageScope) GetUsage(index TrackerIndex) TextureUses {
	if s.IsUsed(index) {
		return s.states[index].wholeUsage()
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

// ensureSize grows the state vector if needed. States a Clear dropped are
// taken back, with the storage of their subresources; SetUsage resets each
// before its first use.
func (s *TextureUsageScope) ensureSize(size int) {
	for len(s.states) < size {
		if len(s.states) < cap(s.states) {
			s.states = s.states[:len(s.states)+1]
			continue
		}
		s.states = append(s.states, textureState{})
	}
}

// TexturePendingTransition represents a texture state transition that needs
// a barrier: the subresources Selector selects move from one usage to
// another. Produced by TextureTracker.Merge() when a command buffer's usage
// scope requires a different state than the device tracker's current state.
//
// Reference: wgpu-core track/mod.rs PendingTransition<TextureUses>
type TexturePendingTransition struct {
	Index    TrackerIndex
	Selector TextureSelector
	Usage    TextureStateTransition
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

// IntoHAL converts a pending transition to a HAL texture barrier over the
// subresources it selects; one selecting every subresource leaves the range
// at its zero value, meaning all mip levels and array layers. The caller
// provides the hal.Texture handle.
func (p TexturePendingTransition) IntoHAL(texture hal.Texture) hal.TextureBarrier {
	var subresources hal.TextureRange
	if !p.Selector.SelectsAll() {
		mipStart, mipEnd, layerStart, layerEnd := p.Selector.bounds(p.Selector.TextureMipLevelCount, p.Selector.TextureArrayLayerCount)
		subresources = hal.TextureRange{
			BaseMipLevel:    mipStart,
			MipLevelCount:   mipEnd - mipStart,
			BaseArrayLayer:  layerStart,
			ArrayLayerCount: layerEnd - layerStart,
		}
	}
	return hal.TextureBarrier{
		Texture: texture,
		Range:   subresources,
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
