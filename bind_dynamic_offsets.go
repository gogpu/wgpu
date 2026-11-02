//go:build !rust && !(js && wasm)

package wgpu

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/gogpu/gputypes"
)

// dynamicBinding describes one buffer binding of a bind group whose layout
// entry has HasDynamicOffset. A bind group keeps them sorted by binding
// number, the order of the dynamic offsets given to SetBindGroup.
// Matches Rust wgpu-core's BindGroupDynamicBindingData.
type dynamicBinding struct {
	binding     uint32
	bindingType gputypes.BufferBindingType
	bufferSize  uint64
	// bindingEnd is the binding's offset plus its size in the buffer.
	bindingEnd uint64
	// maxDynamicOffset is the largest dynamic offset that keeps the binding
	// inside the buffer: bufferSize - bindingEnd.
	maxDynamicOffset uint64
}

// buildDynamicBindings collects the dynamic buffer bindings of a bind group,
// sorted by binding number (Rust wgpu-core Device::create_bind_group sorts
// dynamic_binding_info by binding_idx).
func buildDynamicBindings(layoutEntries []gputypes.BindGroupLayoutEntry, entries []BindGroupEntry) []dynamicBinding {
	entryMap := buildBindGroupEntryMap(entries)
	var bindings []dynamicBinding
	for _, layoutEntry := range layoutEntries {
		if layoutEntry.Buffer == nil || !layoutEntry.Buffer.HasDynamicOffset {
			continue
		}
		bgEntry, ok := entryMap[layoutEntry.Binding]
		if !ok || bgEntry.Buffer == nil {
			continue
		}
		bufSize := bgEntry.Buffer.Size()
		end := bufSize
		if bgEntry.Size != 0 {
			end = bgEntry.Offset + bgEntry.Size
		}
		var maxOffset uint64
		if end <= bufSize {
			maxOffset = bufSize - end
		}
		bindings = append(bindings, dynamicBinding{
			binding:          layoutEntry.Binding,
			bindingType:      layoutEntry.Buffer.Type,
			bufferSize:       bufSize,
			bindingEnd:       end,
			maxDynamicOffset: maxOffset,
		})
	}
	slices.SortFunc(bindings, func(a, b dynamicBinding) int { return cmp.Compare(a.binding, b.binding) })
	return bindings
}

// Unexported sentinel errors for dynamic offset validation, matching Rust
// wgpu-core's BindError variants.
var (
	errDynamicOffsetCount       = errors.New("dynamic offset count does not match the bind group's dynamic bindings")
	errDynamicOffsetUnaligned   = errors.New("dynamic offset is not aligned")
	errDynamicOffsetOutOfBounds = errors.New("dynamic offset puts the binding past the end of its buffer")
)

// dynamicOffsetAlignment returns the alignment a dynamic offset of a binding
// type needs and the name of its limit (Rust buffer_binding_type_alignment).
// An unset binding type is a uniform buffer, WebGPU's default.
func dynamicOffsetAlignment(bindingType gputypes.BufferBindingType, limits *gputypes.Limits) (uint32, string) {
	switch bindingType {
	case gputypes.BufferBindingTypeStorage, gputypes.BufferBindingTypeReadOnlyStorage:
		return limits.MinStorageBufferOffsetAlignment, "minStorageBufferOffsetAlignment"
	default:
		return limits.MinUniformBufferOffsetAlignment, "minUniformBufferOffsetAlignment"
	}
}

// validateDynamicOffsets checks the dynamic offsets given to SetBindGroup:
// one per dynamic binding, each aligned to the limit of its binding type and
// keeping the binding inside its buffer. Matches Rust wgpu-core's
// BindGroup::validate_dynamic_bindings (W3C WebGPU setBindGroup validation).
func validateDynamicOffsets(index uint32, group *BindGroup, offsets []uint32, limits *gputypes.Limits) error {
	if len(offsets) != len(group.dynamicBindings) {
		return fmt.Errorf("bind group at index %d has %d dynamic binding(s), got %d offset(s): %w",
			index, len(group.dynamicBindings), len(offsets), errDynamicOffsetCount)
	}
	for i, offset := range offsets {
		info := &group.dynamicBindings[i]
		alignment, limitName := dynamicOffsetAlignment(info.bindingType, limits)
		if alignment != 0 && offset%alignment != 0 {
			return fmt.Errorf("bind group at index %d, binding %d: dynamic offset[%d]=%d is not a multiple of %s (%d): %w",
				index, info.binding, i, offset, limitName, alignment, errDynamicOffsetUnaligned)
		}
		if uint64(offset) > info.maxDynamicOffset {
			return fmt.Errorf("bind group at index %d, binding %d: dynamic offset[%d]=%d puts the binding end (%d) past the buffer size %d: %w",
				index, info.binding, i, offset, info.bindingEnd+uint64(offset), info.bufferSize, errDynamicOffsetOutOfBounds)
		}
	}
	return nil
}
