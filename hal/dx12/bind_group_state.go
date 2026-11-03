// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import "math/bits"

// maxBindGroups is the number of bind group slots the DX12 backend exposes
// (Limits.MaxBindGroups).
const maxBindGroups = 4

// boundBindGroup is a bind group set on a pass, with its dynamic offsets.
type boundBindGroup struct {
	group   *BindGroup
	offsets []uint32
}

// passBindGroups holds the bind groups set on a pass until a draw or dispatch
// binds them to the root parameters of the current pipeline, as Rust
// wgpu-hal's PassState root elements with their dirty bits (dx12/command.rs
// set_bind_group, update_root_elements). The root parameters of a group come
// from the pipeline layout, which SetBindGroup does not know, and a root
// signature change discards every root argument, so a group set before the
// pipeline, or kept across a pipeline change, is bound again at the next draw.
type passBindGroups struct {
	groups [maxBindGroups]boundBindGroup
	dirty  uint32 // bit i: group i is set and not bound to the current root signature
}

// set records group at index with a copy of its dynamic offsets.
func (s *passBindGroups) set(index uint32, group *BindGroup, offsets []uint32) {
	if index >= maxBindGroups {
		return
	}
	b := &s.groups[index]
	b.group = group
	b.offsets = append(b.offsets[:0], offsets...)
	s.dirty |= 1 << index
}

// invalidate marks every set group for binding: a new root signature
// discards the root arguments.
func (s *passBindGroups) invalidate() {
	for i := range s.groups {
		if s.groups[i].group != nil {
			s.dirty |= 1 << i
		}
	}
}

// flush binds the groups marked for binding to the root parameters of
// mappings, the current pipeline's. A group the pipeline layout does not have
// stays set and is bound again after the next pipeline change.
func (s *passBindGroups) flush(e *CommandEncoder, isCompute bool, mappings []rootParamMapping) {
	for s.dirty != 0 {
		i := bits.TrailingZeros32(s.dirty)
		s.dirty &^= 1 << i
		if i < len(mappings) {
			b := &s.groups[i]
			e.bindGroupToRootParams(mappings[i], b.group, b.offsets, isCompute)
		}
	}
}

// bindGroupToRootParams binds a bind group to the root parameters of mapping:
// its CBV/SRV/UAV descriptor table (samplers come from the global sampler heap
// bound in SetPipeline) and its dynamic buffers, as Rust wgpu-hal's
// set_bind_group. A dynamic uniform buffer's root CBV points at its binding
// plus the offset; a dynamic storage buffer's offset, and after the offsets
// its binding size, go into the group's root constants. offsets are in the
// order of bg.dynamicBuffers, binding order.
func (e *CommandEncoder) bindGroupToRootParams(mapping rootParamMapping, bg *BindGroup, offsets []uint32, isCompute bool) {
	if bg.gpuDescHandle.Ptr != 0 && mapping.cbvSrvUavIndex >= 0 {
		if isCompute {
			e.cmdList.SetComputeRootDescriptorTable(uint32(mapping.cbvSrvUavIndex), bg.gpuDescHandle)
		} else {
			e.cmdList.SetGraphicsRootDescriptorTable(uint32(mapping.cbvSrvUavIndex), bg.gpuDescHandle)
		}
	}

	uniformRoot := mapping.dynamicUniformIndex
	var storage uint32
	for i, db := range bg.dynamicBuffers {
		var offset uint32
		if i < len(offsets) {
			offset = offsets[i]
		}
		if db.uniform {
			if uniformRoot >= 0 {
				e.setRootConstantBufferView(uint32(uniformRoot), db.gpuVA+uint64(offset), isCompute)
				uniformRoot++
			}
			continue
		}
		if mapping.dynamicStorageIndex >= 0 {
			root := uint32(mapping.dynamicStorageIndex)
			e.setRoot32BitConstant(root, offset, storage, isCompute)
			e.setRoot32BitConstant(root, db.size, uint32(mapping.dynamicStorageCount)+storage, isCompute)
		}
		storage++
	}
}

func (e *CommandEncoder) setRootConstantBufferView(root uint32, address uint64, isCompute bool) {
	if isCompute {
		e.cmdList.SetComputeRootConstantBufferView(root, address)
	} else {
		e.cmdList.SetGraphicsRootConstantBufferView(root, address)
	}
}

func (e *CommandEncoder) setRoot32BitConstant(root, value, offset uint32, isCompute bool) {
	if isCompute {
		e.cmdList.SetComputeRoot32BitConstant(root, value, offset)
	} else {
		e.cmdList.SetGraphicsRoot32BitConstant(root, value, offset)
	}
}
