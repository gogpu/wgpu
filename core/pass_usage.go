//go:build !(js && wasm)

package core

import (
	"fmt"

	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

// BindGroupBufferUse is one buffer binding of a bind group and the usage the
// binding implies (uniform, read-only storage, or read-write storage).
type BindGroupBufferUse struct {
	Buffer *Buffer
	Usage  track.BufferUses
}

// BindGroupTextureUse is one texture binding of a bind group and the usage
// the binding implies (sampled, or storage with the layout's access mode).
type BindGroupTextureUse struct {
	Texture *Texture
	Usage   track.TextureUses

	// Resolve returns the texture's current HAL texture, or nil if it is
	// gone. When nil, Texture.Raw is used.
	Resolve func() hal.Texture
}

// BindGroupUsages is the set of buffer and texture usages of a bind group.
// Setting the bind group in a pass merges these usages into the pass's usage
// scope (render passes) or into the usage scope of each dispatch that uses
// the bind group (compute passes).
//
// Reference: wgpu-core binding_model.rs BindGroup.used (BindGroupStates)
type BindGroupUsages struct {
	Buffers  []BindGroupBufferUse
	Textures []BindGroupTextureUse
}

// passUsageScope is one WebGPU usage scope (a render pass, or one compute
// dispatch) together with the resources it references, so that tracker
// indices can be resolved to HAL resources when barriers are recorded.
//
// Reference: wgpu-core track/mod.rs UsageScope
type passUsageScope struct {
	buffers  *track.BufferUsageScope
	textures *track.TextureUsageScope

	bufferRefs  []*Buffer
	textureRefs []BindGroupTextureUse
}

func newPassUsageScope() *passUsageScope {
	return newPassUsageScopeWithTextures(track.NewTextureUsageScope())
}

func newPassUsageScopeWithTextures(textures *track.TextureUsageScope) *passUsageScope {
	return &passUsageScope{
		buffers:  track.NewBufferUsageScope(),
		textures: textures,
	}
}

// clear empties the scope for reuse by the next dispatch.
func (s *passUsageScope) clear() {
	s.buffers.Clear()
	s.textures.Clear()
	s.bufferRefs = s.bufferRefs[:0]
	s.textureRefs = s.textureRefs[:0]
}

// addBuffer merges one buffer usage into the scope. Buffers without valid
// tracking data are skipped. Returns an error if the buffer already has an
// incompatible usage in this scope.
func (s *passUsageScope) addBuffer(buffer *Buffer, usage track.BufferUses) error {
	if buffer == nil {
		return nil
	}
	td := buffer.TrackingData()
	if td == nil || !td.Index().IsValid() {
		return nil
	}
	if !s.buffers.IsUsed(td.Index()) {
		s.bufferRefs = append(s.bufferRefs, buffer)
	}
	return s.buffers.SetUsage(td.Index(), usage)
}

// addTexture merges one texture usage into the scope. Textures without valid
// tracking data are skipped. Returns an error if the texture already has an
// incompatible usage in this scope.
func (s *passUsageScope) addTexture(use BindGroupTextureUse) error {
	if use.Texture == nil {
		return nil
	}
	td := use.Texture.TrackingData()
	if td == nil || !td.Index().IsValid() {
		return nil
	}
	if !s.textures.IsUsed(td.Index()) {
		s.textureRefs = append(s.textureRefs, use)
	}
	return s.textures.SetUsage(td.Index(), use.Usage)
}

// mergeBindGroup merges every usage of a bind group into the scope.
//
// Reference: wgpu-core track/mod.rs UsageScope::merge_bind_group
func (s *passUsageScope) mergeBindGroup(usages *BindGroupUsages) error {
	if usages == nil {
		return nil
	}
	for _, use := range usages.Buffers {
		if err := s.addBuffer(use.Buffer, use.Usage); err != nil {
			return err
		}
	}
	for _, use := range usages.Textures {
		if err := s.addTexture(use); err != nil {
			return err
		}
	}
	return nil
}

// scopeBarriersLocked applies a finished usage scope to the command buffer
// trackers and returns the barriers that must execute before the scope's
// commands. Entries that cannot be resolved to a HAL resource (render pass
// attachments, which BeginRenderPass transitions itself, or destroyed
// resources) are skipped. Caller must hold e.mu.
//
// Reference: wgpu-core track/mod.rs Tracker::set_from_usage_scope +
// CommandEncoder::drain_barriers
func (e *CoreCommandEncoder) scopeBarriersLocked(
	scope *passUsageScope,
	guard SnatchGuard,
) ([]hal.BufferBarrier, []hal.TextureBarrier) {
	if e.mutable == nil {
		return nil, nil
	}
	var bufferBarriers []hal.BufferBarrier
	if tracker := e.mutable.bufferTracker; tracker != nil {
		for _, transition := range tracker.SetFromScope(scope.buffers) {
			if raw := scope.resolveBuffer(transition.Index, guard); raw != nil {
				bufferBarriers = append(bufferBarriers, transition.IntoHAL(raw))
			}
		}
	}
	var textureBarriers []hal.TextureBarrier
	if tracker := e.mutable.textureTracker; tracker != nil {
		for _, transition := range tracker.SetFromScope(scope.textures) {
			if raw := scope.resolveTexture(transition.Index, guard); raw != nil {
				textureBarriers = append(textureBarriers, transition.IntoHAL(raw))
			}
		}
	}
	return bufferBarriers, textureBarriers
}

func (s *passUsageScope) resolveBuffer(index track.TrackerIndex, guard SnatchGuard) hal.Buffer {
	for _, buffer := range s.bufferRefs {
		if td := buffer.TrackingData(); td != nil && td.Index() == index {
			return buffer.Raw(guard)
		}
	}
	return nil
}

func (s *passUsageScope) resolveTexture(index track.TrackerIndex, guard SnatchGuard) hal.Texture {
	for _, use := range s.textureRefs {
		if td := use.Texture.TrackingData(); td == nil || td.Index() != index {
			continue
		}
		if use.Resolve != nil {
			return use.Resolve()
		}
		return use.Texture.Raw(guard)
	}
	return nil
}

// MergeBindGroup merges the usages of a bind group set in this render pass
// into the pass's usage scope. A render pass is one usage scope, so every
// bind group set in it counts, whether or not a draw uses it. Returns an error
// if a usage conflicts with another usage in the pass (for example a texture
// that is both a color attachment and sampled).
//
// The barriers into these usages are recorded when the pass ends, into a
// transit command buffer placed before the pass.
//
// Reference: wgpu-core command/pass.rs set_bind_group (merge_bind_groups for
// render passes)
func (p *CoreRenderPassEncoder) MergeBindGroup(usages *BindGroupUsages) error {
	if p.ended || p.scope == nil {
		return nil
	}
	return p.scope.mergeBindGroup(usages)
}

// transitionRenderPassScopeLocked applies a finished render pass's usage
// scope to the command buffer trackers. Barriers cannot be recorded inside a
// render pass, and the pass is already recorded, so any barriers go into a
// separate transit command buffer inserted just before the pass's command
// buffer. BeginRenderPass gave the pass a recording of its own for this.
// Caller must hold e.mu.
//
// Reference: wgpu-core command/render.rs (transit = encoder.open_pass("Pre
// Pass"), insert_barriers_from_scope, encoder.close_and_swap)
func (e *CoreCommandEncoder) transitionRenderPassScopeLocked(pass *CoreRenderPassEncoder) error {
	if pass.scope == nil {
		return nil
	}
	guard := e.device.snatchLock.Read()
	defer guard.Release()

	bufferBarriers, textureBarriers := e.scopeBarriersLocked(pass.scope, guard)
	if len(bufferBarriers) == 0 && len(textureBarriers) == 0 {
		return nil
	}
	if !e.multiRecording {
		hal.Logger().Debug("core: render pass barriers dropped: HAL encoder cannot begin a transit recording",
			"label", e.label, "buffers", len(bufferBarriers), "textures", len(textureBarriers))
		return nil
	}
	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		return ErrResourceDestroyed
	}
	raw := *halEncoder

	// The pass's recording goes to the end of the list...
	if err := e.closeRecordingLocked(raw); err != nil {
		return fmt.Errorf("close render pass command buffer: %w", err)
	}
	// ...and the transit command buffer right before it.
	if err := raw.BeginEncoding("(wgpu internal) Pre Pass"); err != nil {
		return fmt.Errorf("begin transit command buffer: %w", err)
	}
	if len(bufferBarriers) > 0 {
		raw.TransitionBuffers(bufferBarriers)
	}
	if len(textureBarriers) > 0 {
		raw.TransitionTextures(textureBarriers)
	}
	transit, err := raw.EndEncoding()
	if err != nil {
		return fmt.Errorf("end transit command buffer: %w", err)
	}
	last := len(e.cbList) - 1
	e.cbList = append(e.cbList, nil)
	copy(e.cbList[last+1:], e.cbList[last:])
	e.cbList[last] = transit
	return nil
}

// FlushDispatch builds the usage scope of the next dispatch from the bind
// groups its pipeline uses and the indirect buffer (if any), and records the
// barriers into those usages. Compute passes may record barriers between
// dispatches, so they go straight into the pass's command buffer.
//
// indirectUsage is the usage under which the dispatch reads the indirect
// buffer: Indirect for a direct indirect dispatch, StorageRead when a
// validation shader reads the arguments first. It is ignored when indirect
// is nil.
//
// Returns an error if two usages in the dispatch conflict; the dispatch must
// then not be recorded.
//
// Reference: wgpu-core command/compute.rs State::flush_bindings
func (p *CoreComputePassEncoder) FlushDispatch(groups []*BindGroupUsages, indirect *Buffer, indirectUsage track.BufferUses) error {
	if p.ended || p.dispatchScope == nil {
		return nil
	}
	scope := p.dispatchScope
	scope.clear()
	for _, group := range groups {
		if err := scope.mergeBindGroup(group); err != nil {
			return err
		}
	}
	if err := scope.addBuffer(indirect, indirectUsage); err != nil {
		return err
	}

	e := p.encoder
	e.mu.Lock()
	defer e.mu.Unlock()
	guard := e.device.snatchLock.Read()
	defer guard.Release()

	bufferBarriers, textureBarriers := e.scopeBarriersLocked(scope, guard)
	if len(bufferBarriers) == 0 && len(textureBarriers) == 0 {
		return nil
	}
	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		return ErrResourceDestroyed
	}
	if len(bufferBarriers) > 0 {
		(*halEncoder).TransitionBuffers(bufferBarriers)
	}
	if len(textureBarriers) > 0 {
		(*halEncoder).TransitionTextures(textureBarriers)
	}
	return nil
}
