//go:build !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
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

// binderPayload holds per-slot state for the binder, including late buffer bindings.
// Matches Rust wgpu-core's EntryPayload (command/bind.rs:299-307).
type binderPayload struct {
	lateBufferBindings         []lateBufferBinding
	lateBindingsEffectiveCount int
}

func (p *binderPayload) reset() {
	p.lateBufferBindings = p.lateBufferBindings[:0]
	p.lateBindingsEffectiveCount = 0
}

// binder tracks bind group assignments and validates compatibility at draw/dispatch
// time, matching Rust wgpu-core's Binder pattern.
//
// When SetPipeline is called, the expected layouts are set from the pipeline layout.
// When SetBindGroup is called, the assigned layout is recorded at that slot.
// Before Draw/DrawIndexed/Dispatch, checkCompatibility verifies that every slot
// expected by the pipeline has a compatible bind group assigned.
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
}

// reset clears all binder state. Called when a new pipeline is set.
func (b *binder) reset() {
	b.assigned = [MaxBindGroups]*BindGroupLayout{}
	b.expected = [MaxBindGroups]*BindGroupLayout{}
	b.maxSlots = 0
	for i := range b.payloads {
		b.payloads[i].reset()
	}
}

// updateExpectations sets the expected layouts from a pipeline's bind group layouts.
// Called from SetPipeline. Previously assigned bind groups are preserved so that
// bind groups set before the pipeline remain valid (matching WebGPU spec behavior).
func (b *binder) updateExpectations(layouts []*BindGroupLayout) {
	// Clear old expectations.
	b.expected = [MaxBindGroups]*BindGroupLayout{}

	n := uint32(len(layouts)) //nolint:gosec // layout count fits uint32
	if n > MaxBindGroups {
		n = MaxBindGroups
	}
	b.maxSlots = n

	for i := uint32(0); i < n; i++ {
		b.expected[i] = layouts[i]
	}
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
// Matches Rust wgpu-core's Binder::set_group (command/bind.rs:386-421).
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
// Called from SetBindGroup. The layout pointer is stored for later compatibility checks.
func (b *binder) assign(index uint32, layout *BindGroupLayout) {
	if index < MaxBindGroups {
		b.assigned[index] = layout
	}
}

// validateSetBindGroup performs common validation for SetBindGroup on both
// render and compute passes. Returns a non-nil error message if validation fails.
func validateSetBindGroup(passName string, index uint32, group *BindGroup, offsets []uint32, pipelineBGCount uint32) error {
	if group == nil {
		return fmt.Errorf("wgpu: %s.SetBindGroup: bind group is nil", passName)
	}
	if index >= MaxBindGroups {
		return fmt.Errorf("wgpu: %s.SetBindGroup: index %d >= MaxBindGroups (%d)", passName, index, MaxBindGroups)
	}
	if pipelineBGCount > 0 && index >= pipelineBGCount {
		return fmt.Errorf("wgpu: %s.SetBindGroup: group index %d exceeds pipeline layout bind group count %d",
			passName, index, pipelineBGCount)
	}
	for i, offset := range offsets {
		if offset%256 != 0 {
			return fmt.Errorf("wgpu: %s.SetBindGroup: dynamic offset[%d]=%d not aligned to 256", passName, i, offset)
		}
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
