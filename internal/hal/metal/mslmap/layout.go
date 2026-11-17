// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
)

// Kind is the Metal argument table a binding takes a slot in.
type Kind uint8

const (
	// KindNone marks a layout entry that takes no slot.
	KindNone Kind = iota
	// KindBuffer is the buffer argument table: uniform and storage buffers
	// and acceleration structures.
	KindBuffer
	// KindTexture is the texture argument table: sampled and storage
	// textures.
	KindTexture
	// KindSampler is the sampler argument table.
	KindSampler
)

// EntryKind returns the argument table of a bind group layout entry, as Rust
// wgpu-hal's Metal create_pipeline_layout sorts the binding types.
func EntryKind(entry *gputypes.BindGroupLayoutEntry) Kind {
	switch {
	case entry.Buffer != nil, entry.AccelerationStructure != nil:
		return KindBuffer
	case entry.Texture != nil, entry.StorageTexture != nil:
		return KindTexture
	case entry.Sampler != nil:
		return KindSampler
	default:
		return KindNone
	}
}

// Counts holds a number of slots per argument table.
type Counts struct {
	Buffers  int
	Textures int
	Samplers int
}

// Of returns the count of the argument table k.
func (c Counts) Of(k Kind) int {
	switch k {
	case KindBuffer:
		return c.Buffers
	case KindTexture:
		return c.Textures
	case KindSampler:
		return c.Samplers
	default:
		return 0
	}
}

func (c *Counts) add(k Kind) {
	switch k {
	case KindBuffer:
		c.Buffers++
	case KindTexture:
		c.Textures++
	case KindSampler:
		c.Samplers++
	}
}

// SortEntries returns a copy of entries sorted by binding number. WebGPU
// gives the dynamic offsets in binding order, and the slots below are
// assigned in that order too, so they never depend on the order the caller
// listed the entries in. Rust wgpu-core hands wgpu-hal the entries sorted.
func SortEntries(entries []gputypes.BindGroupLayoutEntry) []gputypes.BindGroupLayoutEntry {
	out := slices.Clone(entries)
	slices.SortStableFunc(out, func(a, b gputypes.BindGroupLayoutEntry) int {
		return cmp.Compare(a.Binding, b.Binding)
	})
	return out
}

// GroupSlots returns, for each entry of one bind group layout (sorted by
// SortEntries), its slot in its argument table counted from the group's
// first slot in stage, or -1 if the entry is not visible to stage or takes
// no slot. counts is the number of slots the group takes in stage.
//
// The Metal device binds a bind group with these slots and compiles the
// pipeline's shaders with StageLayout, which counts the same way.
//
// Reference: Rust wgpu-hal metal/device.rs create_bind_group (the per-stage
// counters of BindGroup).
func GroupSlots(entries []gputypes.BindGroupLayoutEntry, stage gputypes.ShaderStages) (slots []int, counts Counts) {
	slots = make([]int, len(entries))
	for i := range entries {
		entry := &entries[i]
		kind := EntryKind(entry)
		if kind == KindNone || entry.Visibility&stage == 0 {
			slots[i] = -1
			continue
		}
		slots[i] = counts.Of(kind)
		counts.add(kind)
	}
	return slots, counts
}

// StageLayout is where one shader stage finds its resources under one
// pipeline layout.
type StageLayout struct {
	// Resources maps each binding visible to the stage to its slot.
	Resources map[ir.ResourceBinding]msl.BindTarget
	// GroupBase holds, per bind group of the pipeline layout, the first slot
	// of the group in each argument table.
	GroupBase []Counts
	// SizesSlot is the buffer slot of naga's `_buffer_sizes` argument: the
	// first buffer slot after the bindings, reserved when a storage buffer
	// is visible to the stage. It is -1 when none is.
	SizesSlot int
}

// NewStageLayout lays out the bindings of a pipeline layout for one stage:
// groups holds the entries of each bind group layout, sorted by SortEntries
// (nil for a group without entries). Each argument table is numbered from 0
// over the bindings visible to the stage, group after group, in binding
// order.
//
// Reference: Rust wgpu-hal metal/device.rs create_pipeline_layout
// (per_stage_map, base_resource_indices, sizes_buffer).
func NewStageLayout(groups [][]gputypes.BindGroupLayoutEntry, stage gputypes.ShaderStages) (StageLayout, error) {
	layout := StageLayout{
		Resources: make(map[ir.ResourceBinding]msl.BindTarget),
		GroupBase: make([]Counts, len(groups)),
		SizesSlot: -1,
	}
	var next Counts
	needSizes := false
	for group, entries := range groups {
		layout.GroupBase[group] = next
		slots, counts := GroupSlots(entries, stage)
		for i := range entries {
			if slots[i] < 0 {
				continue
			}
			entry := &entries[i]
			kind := EntryKind(entry)
			slot := next.Of(kind) + slots[i]
			if slot > maxBufferSlot {
				return StageLayout{}, fmt.Errorf("mslmap: binding %d of group %d needs slot %d, above %d", entry.Binding, group, slot, maxBufferSlot)
			}
			binding := ir.ResourceBinding{Group: uint32(group), Binding: entry.Binding}
			layout.Resources[binding] = bindTarget(entry, kind, uint8(slot))
			if IsStorageBuffer(entry) {
				needSizes = true
			}
		}
		next.Buffers += counts.Buffers
		next.Textures += counts.Textures
		next.Samplers += counts.Samplers
	}
	if needSizes {
		if next.Buffers > maxBufferSlot {
			return StageLayout{}, fmt.Errorf("mslmap: buffer sizes slot %d out of range 0..%d", next.Buffers, maxBufferSlot)
		}
		layout.SizesSlot = next.Buffers
	}
	return layout, nil
}

// Options returns the MSL options that write only entryPoint of the given
// stage, with this layout's slots. naga refuses a resource the entry point
// uses that the layout does not hold (backend.ErrMissingBinding).
func (l *StageLayout) Options(stage ir.ShaderStage, entryPoint string) msl.Options {
	opts := BaseOptions()
	resources := msl.EntryPointResources{Resources: l.Resources}
	if l.SizesSlot >= 0 {
		slot := uint8(l.SizesSlot)
		resources.SizesBuffer = &slot
	}
	// The key is the WGSL entry point name, which is the IR name. It is not
	// the MSL function name, which naga can rename.
	opts.PerEntryPointMap = map[string]msl.EntryPointResources{entryPoint: resources}
	opts.EntryPoint = &backend.EntryPointSelector{Stage: stage, Name: entryPoint}
	return opts
}

// bindTarget returns the naga bind target of a layout entry at slot.
func bindTarget(entry *gputypes.BindGroupLayoutEntry, kind Kind, slot uint8) msl.BindTarget {
	var target msl.BindTarget
	switch kind {
	case KindBuffer:
		target.Buffer = &slot
		target.Mutable = entry.Buffer != nil && entry.Buffer.Type == gputypes.BufferBindingTypeStorage
	case KindTexture:
		target.Texture = &slot
		target.Mutable = entry.StorageTexture != nil && entry.StorageTexture.Access != gputypes.StorageTextureAccessReadOnly
	case KindSampler:
		target.Sampler = &msl.BindSamplerTarget{Slot: slot}
	}
	return target
}

// IsStorageBuffer reports whether entry is a storage buffer: only those can
// hold a runtime-sized array, whose length naga reads from `_buffer_sizes`.
func IsStorageBuffer(entry *gputypes.BindGroupLayoutEntry) bool {
	if entry.Buffer == nil {
		return false
	}
	return entry.Buffer.Type == gputypes.BufferBindingTypeStorage ||
		entry.Buffer.Type == gputypes.BufferBindingTypeReadOnlyStorage
}
