//go:build !(js && wasm)

package track

import "github.com/gogpu/gputypes"

// Usage scopes versus command-buffer trackers
//
// WebGPU only defines usage conflicts inside a single usage scope: one render
// pass, one compute dispatch, or one copy command. Different commands of the
// same command encoder may use a resource with different usages; the
// implementation transitions the resource between them.
//
// The track package therefore has two kinds of per-command-buffer structures,
// exactly like Rust wgpu-core (track/mod.rs, "Use Case"):
//
//   - TextureUsageScope / BufferUsageScope: one usage scope. Adding a usage
//     merges it with earlier usages of the same scope and fails on conflict.
//   - EncoderTextureTracker / EncoderBufferTracker: the command buffer's
//     tracker. It never reports conflicts. It records the first usage of each
//     resource in the command buffer (start) and the current usage (end), and
//     returns the transitions needed between consecutive usage scopes.
//
// At submit time the device tracker emits barriers from its current state to
// each command buffer's start state, then adopts the command buffer's end
// state (see TextureTracker.MergeEncoder / BufferTracker.MergeEncoder).
//
// Reference: wgpu-core track/mod.rs —
//
//	CommandBuffer <- UsageScope = insert(buffer.start, buffer.end, scope)
//	                              OR barrier(buffer.end, scope) + update(buffer.end, scope)
//	Device <- CommandBuffer = insert(device.start, device.end, buffer.start, buffer.end)
//	                          OR barrier(device.end, buffer.start) + update(device.end, buffer.end)

// EncoderTextureTracker tracks texture states across the commands of one
// command buffer, per mip level and array layer as TextureTracker does.
//
// Reference: wgpu-core track/texture.rs TextureTracker (start_set + end_set)
type EncoderTextureTracker struct {
	start    []textureState
	end      []textureState
	metadata ResourceMetadata
}

// NewEncoderTextureTracker creates an empty command-buffer texture tracker.
func NewEncoderTextureTracker() *EncoderTextureTracker {
	return &EncoderTextureTracker{
		start:    make([]textureState, 0, 32),
		end:      make([]textureState, 0, 32),
		metadata: NewResourceMetadata(),
	}
}

// AppendSingle records that the next command uses the subresources selector
// selects with usage, appending the transitions it needs to transitions.
//
// On a subresource's first use in this command buffer the usage becomes both
// its start and its end state, and no transition is needed: the barrier
// into the start state is generated at submit time from the device tracker.
// On later uses a transition from its current end state is needed unless
// the state is unchanged and hardware-ordered (SkipBarrier). The end state
// is updated either way. AppendSingle never fails; conflicts are a
// usage-scope concern.
//
// Reference: wgpu-core track/texture.rs TextureTracker::set_single
func (t *EncoderTextureTracker) AppendSingle(transitions []TexturePendingTransition, index TrackerIndex, selector TextureSelector, usage TextureUses) []TexturePendingTransition {
	pendingTransitions := textureTransitionList(transitions)
	start, end := t.own(index)
	useSubresources(&pendingTransitions, index, start, end, selector, usage, usage)
	return pendingTransitions
}

// SetFromScope applies every usage of a finished usage scope (for example a
// render pass) to the tracker and returns the transitions that must be
// recorded before that scope's commands execute. Only the subresources the
// scope uses move.
//
// Reference: wgpu-core track/texture.rs TextureTracker::set_from_usage_scope
func (t *EncoderTextureTracker) SetFromScope(scope *TextureUsageScope) []TexturePendingTransition {
	if scope == nil {
		return nil
	}
	var transitions textureTransitionList
	for i := range scope.states {
		index, valid := trackerIndexAt(i)
		if !valid || !scope.metadata.IsOwned(index) {
			continue
		}
		source := &scope.states[i]
		start, end := t.own(index)
		if source.isWhole() {
			if source.usage != TextureUsesNone {
				useSubresources(&transitions, index, start, end, TextureSelector{}, source.usage, source.usage)
			}
			continue
		}
		mipLevelCount, arrayLayerCount := source.counts()
		start.split(mipLevelCount, arrayLayerCount)
		end.split(mipLevelCount, arrayLayerCount)
		for mipLevel := range mipLevelCount {
			for arrayLayer := range arrayLayerCount {
				if usage := *source.at(mipLevel, arrayLayer); usage != TextureUsesNone {
					useSubresource(&transitions, index, start, end, mipLevel, arrayLayer, usage, usage)
				}
			}
		}
		start.collapse()
		end.collapse()
	}
	return transitions
}

// RecordTransition records a barrier the caller has already encoded
// explicitly (CommandEncoder.TransitionTextures) over every subresource of a
// texture; see RecordSubresourceTransition.
func (t *EncoderTextureTracker) RecordTransition(index TrackerIndex, from, to TextureUses) {
	t.RecordSubresourceTransition(index, TextureSelector{}, from, to)
}

// RecordSubresourceTransition records a barrier the caller has already
// encoded explicitly (CommandEncoder.TransitionTextures) over the
// subresources selector selects. Their end state becomes to. For each of
// them used for the first time in the command buffer, from becomes its start
// state, so the submit-time barrier brings it into the state the explicit
// barrier expects. A zero from (undefined contents) has no start state to
// request; to is used instead.
func (t *EncoderTextureTracker) RecordSubresourceTransition(index TrackerIndex, selector TextureSelector, from, to TextureUses) {
	firstUsage := from
	if firstUsage.IsEmpty() {
		firstUsage = to
	}
	start, end := t.own(index)
	useSubresources(nil, index, start, end, selector, firstUsage, to)
}

// StartUsage returns the usage every subresource of the texture is first
// used in by this command buffer, or TextureUsesNone if the texture is not
// used or those usages differ.
func (t *EncoderTextureTracker) StartUsage(index TrackerIndex) TextureUses {
	if !t.IsUsed(index) {
		return TextureUsesNone
	}
	return t.start[index].wholeUsage()
}

// EndUsage returns the usage the command buffer leaves every subresource of
// the texture in, or TextureUsesNone if the texture is not used or those
// usages differ.
func (t *EncoderTextureTracker) EndUsage(index TrackerIndex) TextureUses {
	if !t.IsUsed(index) {
		return TextureUsesNone
	}
	return t.end[index].wholeUsage()
}

// IsUsed returns true if the texture is used in this command buffer.
func (t *EncoderTextureTracker) IsUsed(index TrackerIndex) bool {
	return int(index) < len(t.end) && t.metadata.IsOwned(index)
}

// IsEmpty returns true if no textures are used in this command buffer.
func (t *EncoderTextureTracker) IsEmpty() bool {
	return t.metadata.Count() == 0
}

// own returns the start and end states of the texture, which a first use
// in this command buffer makes unused.
func (t *EncoderTextureTracker) own(index TrackerIndex) (start, end *textureState) {
	t.ensureSize(int(index) + 1)
	start = &t.start[index]
	end = &t.end[index]
	if !t.metadata.IsOwned(index) {
		start.reset(TextureUsesNone)
		end.reset(TextureUsesNone)
		t.metadata.SetOwned(index, true)
	}
	return start, end
}

func (t *EncoderTextureTracker) ensureSize(size int) {
	for len(t.end) < size {
		t.start = append(t.start, textureState{})
		t.end = append(t.end, textureState{})
	}
}

// EncoderBufferTracker tracks buffer states across the commands of one
// command buffer. See EncoderTextureTracker for the semantics.
//
// Reference: wgpu-core track/buffer.rs BufferTracker (start + end)
type EncoderBufferTracker struct {
	start    []BufferUses
	end      []BufferUses
	metadata ResourceMetadata
}

// NewEncoderBufferTracker creates an empty command-buffer buffer tracker.
func NewEncoderBufferTracker() *EncoderBufferTracker {
	return &EncoderBufferTracker{
		start:    make([]BufferUses, 0, 32),
		end:      make([]BufferUses, 0, 32),
		metadata: NewResourceMetadata(),
	}
}

// SkipBufferBarrier returns true if a buffer moving from old to next needs no
// barrier: the state is unchanged and contains only hardware-ordered
// (read-only) usages.
//
// Reference: wgpu-core track/mod.rs skip_barrier with the INCLUSIVE set
func SkipBufferBarrier(old, next BufferUses) bool {
	return old == next && old.IsReadOnly()
}

// SetSingle records that the next command uses the buffer with usage. See
// EncoderTextureTracker.AppendSingle.
//
// Reference: wgpu-core track/buffer.rs BufferTracker::set_single
func (t *EncoderBufferTracker) SetSingle(index TrackerIndex, usage BufferUses) (PendingTransition, bool) {
	t.ensureSize(int(index) + 1)
	if !t.metadata.IsOwned(index) {
		t.start[index] = usage
		t.end[index] = usage
		t.metadata.SetOwned(index, true)
		return PendingTransition{}, false
	}
	current := t.end[index]
	t.end[index] = usage
	if SkipBufferBarrier(current, usage) {
		return PendingTransition{}, false
	}
	return PendingTransition{
		Index: index,
		Usage: StateTransition{From: current, To: usage},
	}, true
}

// SetFromScope applies every usage of a finished usage scope to the tracker
// and returns the transitions needed before that scope's commands.
//
// Reference: wgpu-core track/buffer.rs BufferTracker::set_from_usage_scope
func (t *EncoderBufferTracker) SetFromScope(scope *BufferUsageScope) []PendingTransition {
	if scope == nil {
		return nil
	}
	var transitions []PendingTransition
	for i := range scope.states {
		index, valid := trackerIndexAt(i)
		if !valid || !scope.metadata.IsOwned(index) {
			continue
		}
		if transition, ok := t.SetSingle(index, scope.states[i].usage); ok {
			transitions = append(transitions, transition)
		}
	}
	return transitions
}

// StartUsage returns the first usage of the buffer in this command buffer.
func (t *EncoderBufferTracker) StartUsage(index TrackerIndex) BufferUses {
	if !t.IsUsed(index) {
		return BufferUsesNone
	}
	return t.start[index]
}

// EndUsage returns the usage the command buffer leaves the buffer in.
func (t *EncoderBufferTracker) EndUsage(index TrackerIndex) BufferUses {
	if !t.IsUsed(index) {
		return BufferUsesNone
	}
	return t.end[index]
}

// IsUsed returns true if the buffer is used in this command buffer.
func (t *EncoderBufferTracker) IsUsed(index TrackerIndex) bool {
	return int(index) < len(t.end) && t.metadata.IsOwned(index)
}

// IsEmpty returns true if no buffers are used in this command buffer.
func (t *EncoderBufferTracker) IsEmpty() bool {
	return t.metadata.Count() == 0
}

func (t *EncoderBufferTracker) ensureSize(size int) {
	for len(t.end) < size {
		t.start = append(t.start, BufferUsesNone)
		t.end = append(t.end, BufferUsesNone)
	}
}

// trackerIndexAt converts a dense state-slice position into a TrackerIndex.
// Positions outside the TrackerIndex range (never produced by the allocator)
// are reported invalid instead of being truncated.
func trackerIndexAt(i int) (TrackerIndex, bool) {
	if i < 0 || uint64(i) >= uint64(InvalidTrackerIndex) {
		return 0, false
	}
	return TrackerIndex(i), true
}

// TextureUsesFromUsage converts a public texture usage, as passed to an
// explicit barrier, into tracker states. RenderAttachment maps to the depth
// or color target state depending on the texture format; on a depth/stencil
// texture RenderAttachment together with TextureBinding is the read-only
// attachment state (see TextureUses.ToTextureUsage). StorageBinding maps to
// the exclusive StorageWrite state so later uses are always fenced.
//
// Reference: wgpu-core conv.rs map_texture_usage
func TextureUsesFromUsage(usage gputypes.TextureUsage, depthStencil bool) TextureUses {
	var uses TextureUses
	if usage&gputypes.TextureUsageCopySrc != 0 {
		uses |= TextureUsesCopySrc
	}
	if usage&gputypes.TextureUsageCopyDst != 0 {
		uses |= TextureUsesCopyDst
	}
	if usage&gputypes.TextureUsageTextureBinding != 0 {
		uses |= TextureUsesResource
	}
	if usage&gputypes.TextureUsageStorageBinding != 0 {
		uses |= TextureUsesStorageWrite
	}
	if usage&gputypes.TextureUsageRenderAttachment != 0 {
		switch {
		case depthStencil && usage&gputypes.TextureUsageTextureBinding != 0:
			uses |= TextureUsesDepthStencilRead
		case depthStencil:
			uses |= TextureUsesDepthStencilWrite
		default:
			uses |= TextureUsesColorTarget
		}
	}
	return uses
}
