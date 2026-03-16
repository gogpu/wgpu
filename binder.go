package wgpu

import "fmt"

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
}

// reset clears all binder state. Called when a new pipeline is set.
func (b *binder) reset() {
	b.assigned = [MaxBindGroups]*BindGroupLayout{}
	b.expected = [MaxBindGroups]*BindGroupLayout{}
	b.maxSlots = 0
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

// checkCompatibility validates that all slots expected by the current pipeline
// have compatible bind groups assigned. Returns an error describing the first
// incompatible or missing slot, or nil if all slots are satisfied.
//
// Compatibility is checked entry-by-entry, matching Rust wgpu-core's
// binder.check_compatibility() behavior. Two layouts are compatible if they
// have the same bindings with matching types, visibility, and counts.
// This allows equivalent layouts created via separate CreateBindGroupLayout
// calls to be considered compatible.
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
				"wgpu: bind group at index %d is required by the pipeline but not set (call SetBindGroup)",
				i,
			)
		}
		if !asg.isCompatibleWith(exp) {
			return fmt.Errorf(
				"wgpu: bind group at index %d has incompatible layout (assigned layout %p != expected layout %p)",
				i, asg, exp,
			)
		}
	}
	return nil
}
