//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// lateBufferBinding tracks a single buffer binding that requires late validation.
// Populated from two independent sources (order-independent, matching Rust):
//   - ShaderExpectSize is set when a pipeline is bound (from LateSizedBufferGroup)
//   - BindingIndex and BoundSize are set when a bind group is bound (from LateBufferBindingInfo)
//
// Matches Rust wgpu-core's LateBufferBinding (command/bind.rs:293-297).
type lateBufferBinding struct {
	BindingIndex     uint32
	ShaderExpectSize uint64
	BoundSize        uint64
}

// binderPayload holds per-slot state for the binder: the bind group set at the
// slot, its dynamic offsets and its late buffer bindings.
// Matches Rust wgpu-core's EntryPayload (command/bind.rs).
type binderPayload struct {
	group *BindGroup
	// dynamicOffsets is a copy of the offsets given to SetBindGroup. Every
	// assignment allocates a new slice: a backend may keep the slice it was
	// bound with until the command buffer is played back (GLES records it).
	dynamicOffsets             []uint32
	lateBufferBindings         []lateBufferBinding
	lateBindingsEffectiveCount int
}

func (p *binderPayload) reset() {
	p.group = nil
	p.dynamicOffsets = nil
	p.lateBufferBindings = p.lateBufferBindings[:0]
	p.lateBindingsEffectiveCount = 0
}

// bindGroupSetter is the method of hal.RenderPassEncoder and
// hal.ComputePassEncoder that the binder binds groups through.
type bindGroupSetter interface {
	SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32)
}

// binder tracks bind group assignments, validates compatibility at draw/dispatch
// time and binds the groups to the backend, matching Rust wgpu-core's Binder
// (command/bind.rs) and flush_bindings_helper (command/pass.rs).
//
// When SetPipeline is called, the expected layouts are set from the pipeline layout.
// When SetBindGroup is called, the group and its dynamic offsets are recorded at
// that slot; nothing reaches the backend yet. Before Draw/DrawIndexed/Dispatch,
// checkCompatibility verifies that every slot expected by the pipeline has a
// compatible bind group assigned, and flush binds the groups whose backend
// binding is stale. So the backend never gets a bind group without the
// pipeline layout it is bound to: WebGPU allows SetBindGroup before
// SetPipeline, and for a group index the current pipeline does not use.
type binder struct {
	// assigned holds the layout of the bind group set at each slot via SetBindGroup.
	// nil means no bind group has been assigned to that slot.
	assigned [MaxBindGroups]*BindGroupLayout

	// expected holds the layout expected at each slot by the current pipeline.
	// nil means the pipeline does not use that slot.
	expected [MaxBindGroups]*BindGroupLayout

	// maxSlots is the number of bind group slots expected by the current pipeline.
	// This equals len(pipelineLayout.BindGroupLayouts).
	maxSlots uint32

	// payloads holds per-slot late buffer binding tracking.
	// Matches Rust wgpu-core's Binder.payloads (command/bind.rs:322).
	payloads [MaxBindGroups]binderPayload

	// rebindStart is the first slot whose backend binding may be stale: a
	// group was set there, or the layout the pipeline expects there changed
	// (which, by Vulkan's pipeline layout compatibility, disturbs every
	// higher slot too). flush binds the valid slots from here up.
	// Matches Rust wgpu-core's BoundBindGroupLayouts.rebind_start.
	rebindStart uint32
}

// reset clears all binder state: the pass has no pipeline and no bind groups.
// Called after ExecuteBundles (Rust wgpu-core State::reset_bundle).
func (b *binder) reset() {
	b.assigned = [MaxBindGroups]*BindGroupLayout{}
	b.expected = [MaxBindGroups]*BindGroupLayout{}
	b.maxSlots = 0
	for i := range b.payloads {
		b.payloads[i].reset()
	}
	b.rebindStart = 0
}

// updateExpectations sets the expected layouts from a pipeline's bind group layouts.
// Called from SetPipeline. Previously assigned bind groups are preserved so that
// bind groups set before the pipeline remain valid (matching WebGPU spec behavior).
//
// A slot that gains an expected layout, or whose expected layout changes to an
// incompatible one, lowers rebindStart to it; losing an expectation does not.
// Matches Rust wgpu-core's BoundBindGroupLayouts::update_expectations.
func (b *binder) updateExpectations(layouts []*BindGroupLayout) {
	if len(layouts) > MaxBindGroups {
		layouts = layouts[:MaxBindGroups]
	}
	b.maxSlots = uint32(len(layouts)) //nolint:gosec // at most MaxBindGroups

	var next [MaxBindGroups]*BindGroupLayout
	copy(next[:], layouts)
	for i, layout := range next {
		old := b.expected[i]
		b.expected[i] = layout
		if layout != nil && (old == nil || !old.isCompatibleWith(layout)) {
			b.updateRebindStart(uint32(i))
		}
	}
}

// updateRebindStart lowers rebindStart to index.
func (b *binder) updateRebindStart(index uint32) {
	if index < b.rebindStart {
		b.rebindStart = index
	}
}

// setGroup records the bind group set at index with a copy of its dynamic
// offsets. The backend gets it at the next flush. Matches Rust wgpu-core's
// Binder::assign_group.
func (b *binder) setGroup(index uint32, group *BindGroup, offsets []uint32) {
	if index >= MaxBindGroups {
		return
	}
	b.assign(index, group.layout)
	b.assignBindGroup(index, group)
	payload := &b.payloads[index]
	payload.group = group
	payload.dynamicOffsets = nil
	if len(offsets) > 0 {
		payload.dynamicOffsets = append([]uint32(nil), offsets...)
	}
}

// isValid reports whether slot i holds a group compatible with the layout
// the current pipeline expects there (Rust compat Entry::is_valid).
func (b *binder) isValid(i uint32) bool {
	exp, asg := b.expected[i], b.assigned[i]
	return exp != nil && asg != nil && b.payloads[i].group != nil && asg.isCompatibleWith(exp)
}

// flush binds to the backend every valid slot from rebindStart up, with its
// dynamic offsets, and marks all bindings current. Called right before a draw
// or dispatch, after the draw-time validation passed.
// Matches Rust wgpu-core's flush_bindings_helper (command/pass.rs).
func (b *binder) flush(raw bindGroupSetter) {
	b.bindValid(raw, b.takeRebindStart())
}

// bindValid binds every valid slot from start up. With start 0 it restores
// all bindings on a backend pass that has none.
func (b *binder) bindValid(raw bindGroupSetter, start uint32) {
	if raw == nil {
		return
	}
	b.listValid(start, func(i uint32, group *BindGroup, offsets []uint32) {
		if group.hal != nil {
			raw.SetBindGroup(i, group.hal, offsets)
		}
	})
}

// takeRebindStart returns the first slot whose backend binding may be stale
// and marks all bindings current (Rust Binder::take_rebind_start_index).
func (b *binder) takeRebindStart() uint32 {
	start := b.rebindStart
	b.rebindStart = MaxBindGroups
	return start
}

// listValid calls fn with every valid slot from start up, its group and its
// dynamic offsets (Rust Binder::list_valid_with_start). A render bundle
// records these as its SetBindGroup commands; a pass binds them.
func (b *binder) listValid(start uint32, fn func(i uint32, group *BindGroup, offsets []uint32)) {
	for i := start; i < MaxBindGroups; i++ {
		if b.isValid(i) {
			fn(i, b.payloads[i].group, b.payloads[i].dynamicOffsets)
		}
	}
}

// activeGroup returns the group set at slot i if the current pipeline uses
// the slot, or nil (Rust compat Entry::is_active).
func (b *binder) activeGroup(i uint32) *BindGroup {
	if i >= MaxBindGroups || b.expected[i] == nil {
		return nil
	}
	return b.payloads[i].group
}

// updateLateBufferBindingsFromPipeline fills ShaderExpectSize on each payload
// from the pipeline's LateSizedBufferGroups. Called from SetPipeline.
// Order-independent with assignBindGroup — if the bind group was set first,
// entries already have BoundSize filled, and we just add ShaderExpectSize.
//
// Matches Rust wgpu-core's Binder::change_pipeline_layout (command/bind.rs:341-375).
func (b *binder) updateLateBufferBindingsFromPipeline(lateGroups [MaxBindGroups]LateSizedBufferGroup) {
	for i, lateGroup := range lateGroups {
		payload := &b.payloads[i]
		payload.lateBindingsEffectiveCount = len(lateGroup.ShaderSizes)

		// Update existing entries (bind group was set before pipeline).
		for j := 0; j < len(payload.lateBufferBindings) && j < len(lateGroup.ShaderSizes); j++ {
			payload.lateBufferBindings[j].ShaderExpectSize = lateGroup.ShaderSizes[j]
		}

		// Add new entries for bindings not yet tracked (pipeline bound before bind group).
		if len(lateGroup.ShaderSizes) > len(payload.lateBufferBindings) {
			for _, shaderSize := range lateGroup.ShaderSizes[len(payload.lateBufferBindings):] {
				payload.lateBufferBindings = append(payload.lateBufferBindings, lateBufferBinding{
					ShaderExpectSize: shaderSize,
				})
			}
		}
	}
}

// assignBindGroup fills BoundSize and BindingIndex on a payload from the bind
// group's LateBufferBindingInfos. Called from SetBindGroup.
// Order-independent with updateLateBufferBindingsFromPipeline — if the pipeline
// was set first, entries already have ShaderExpectSize filled.
//
// Matches the late binding part of Rust wgpu-core's Binder::assign_group.
func (b *binder) assignBindGroup(index uint32, bindGroup *BindGroup) {
	if index >= MaxBindGroups {
		return
	}
	payload := &b.payloads[index]

	infos := bindGroup.lateBufferBindingInfos

	// Update existing entries (pipeline was set before bind group).
	for j := 0; j < len(payload.lateBufferBindings) && j < len(infos); j++ {
		payload.lateBufferBindings[j].BindingIndex = infos[j].BindingIndex
		payload.lateBufferBindings[j].BoundSize = infos[j].Size
	}

	// Add new entries for bindings not yet tracked (bind group bound before pipeline).
	if len(infos) > len(payload.lateBufferBindings) {
		for _, info := range infos[len(payload.lateBufferBindings):] {
			payload.lateBufferBindings = append(payload.lateBufferBindings, lateBufferBinding{
				BindingIndex: info.BindingIndex,
				BoundSize:    info.Size,
			})
		}
	}
}

// assign records a bind group assignment at the given slot.
// The layout pointer is stored for later compatibility checks, and the slot's
// backend binding becomes stale (Rust BoundBindGroupLayouts::assign).
func (b *binder) assign(index uint32, layout *BindGroupLayout) {
	if index < MaxBindGroups {
		b.assigned[index] = layout
		b.updateRebindStart(index)
	}
}

// validateSetBindGroup performs common validation for SetBindGroup on both
// render and compute passes. Returns a non-nil error message if validation fails.
//
// The index is checked against the device's maxBindGroups limit, not against
// the current pipeline: WebGPU allows setting a group the current pipeline
// does not use, for a later pipeline (Rust wgpu-core pass.rs set_bind_group).
func validateSetBindGroup(passName string, index uint32, group *BindGroup, offsets []uint32, limits *gputypes.Limits) error {
	if group == nil {
		return fmt.Errorf("wgpu: %s.SetBindGroup: bind group is nil", passName)
	}
	if index >= MaxBindGroups {
		return fmt.Errorf("wgpu: %s.SetBindGroup: index %d >= MaxBindGroups (%d)", passName, index, MaxBindGroups)
	}
	if limits.MaxBindGroups > 0 && index >= limits.MaxBindGroups {
		return fmt.Errorf("wgpu: %s.SetBindGroup: index %d >= device limit maxBindGroups (%d)",
			passName, index, limits.MaxBindGroups)
	}
	if err := validateDynamicOffsets(index, group, offsets, limits); err != nil {
		return fmt.Errorf("wgpu: %s.SetBindGroup: %w", passName, err)
	}
	return nil
}

// checkLateBufferBindings validates that all late-bound buffer bindings have
// sufficient size. Returns an error if any bound buffer is smaller than the
// shader-required minimum. This is the "late" validation for entries with
// MinBindingSize == 0.
//
// The returned error wraps errLateBufferTooSmall so callers can re-wrap with
// the appropriate public sentinel (ErrDrawLateBufferTooSmall or ErrDispatchLateBufferTooSmall).
//
// Matches Rust wgpu-core's Binder::check_late_buffer_bindings (command/bind.rs:480-499).
func (b *binder) checkLateBufferBindings() error {
	for groupIndex := uint32(0); groupIndex < b.maxSlots; groupIndex++ {
		payload := &b.payloads[groupIndex]
		effectiveCount := payload.lateBindingsEffectiveCount
		if effectiveCount > len(payload.lateBufferBindings) {
			effectiveCount = len(payload.lateBufferBindings)
		}
		for j := 0; j < effectiveCount; j++ {
			lb := &payload.lateBufferBindings[j]
			if lb.BoundSize < lb.ShaderExpectSize {
				return fmt.Errorf(
					"wgpu: buffer binding at group %d, binding %d has size %d, "+
						"but the shader requires a minimum of %d (set MinBindingSize in layout to avoid late validation): %w",
					groupIndex, lb.BindingIndex, lb.BoundSize, lb.ShaderExpectSize,
					errLateBufferTooSmall,
				)
			}
		}
	}
	return nil
}

// Unexported sentinel errors for binder validation. Callers re-wrap with
// the appropriate public sentinel (ErrDrawMissing*/ErrDispatchMissing*).
var (
	errBindGroupMissing      = errors.New("bind group missing")
	errBindGroupIncompatible = errors.New("bind group incompatible")
	errLateBufferTooSmall    = errors.New("late buffer too small")
)

// checkCompatibility validates that all slots expected by the current pipeline
// have compatible bind groups assigned. Returns an error describing the first
// incompatible or missing slot, or nil if all slots are satisfied.
//
// Compatibility is checked entry-by-entry, matching Rust wgpu-core's
// binder.check_compatibility() behavior. Two layouts are compatible if they
// have the same bindings with matching types, visibility, and counts.
// This allows equivalent layouts created via separate CreateBindGroupLayout
// calls to be considered compatible.
//
// The returned error wraps errBindGroupMissing or errBindGroupIncompatible
// so callers can re-wrap with the appropriate public sentinel.
func (b *binder) checkCompatibility() error {
	for i := uint32(0); i < b.maxSlots; i++ {
		exp := b.expected[i]
		if exp == nil {
			// Pipeline does not use this slot.
			continue
		}
		asg := b.assigned[i]
		if asg == nil {
			return fmt.Errorf(
				"wgpu: bind group at index %d is required by the pipeline but not set (call SetBindGroup): %w",
				i, errBindGroupMissing,
			)
		}
		if !asg.isCompatibleWith(exp) {
			return fmt.Errorf(
				"wgpu: bind group at index %d has incompatible layout (assigned layout %p != expected layout %p): %w",
				i, asg, exp, errBindGroupIncompatible,
			)
		}
	}
	return nil
}
