// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// samplerBindMap gives, per texture unit, the sampler slot plus one that the
// pipeline's shaders combine with the texture there; 0 = the unit takes no
// sampler, so the zero value is a pipeline that samples nothing.
// Matches Rust wgpu-hal gles SamplerBindMap ([Option<u8>; MAX_TEXTURE_SLOTS]).
type samplerBindMap [maxTextureSlots]uint8

// samplerState pairs sampler objects with texture units. naga's GLSL output
// samples through combined sampler2Ds named after the texture binding, so a
// GL sampler must be bound to the unit of every texture the shader combines
// it with, and which textures those are depends on the pipeline. Bind groups
// record their samplers by sampler slot; the current pipeline's sampler map
// says which slot each texture unit takes, and every change on either side
// rebinds the affected units.
//
// Matches Rust wgpu-hal gles State.samplers and texture_slots[].sampler_index,
// with set_pipeline_inner and rebind_sampler_states (gles/command.rs). It is
// encoder state, reset at BeginEncoding like Rust's begin_encoding.
type samplerState struct {
	// samplers holds the GL sampler bound to each sampler slot; 0 = none.
	samplers [maxTextureSlots]uint32
	// textureSampler is the current pipeline's sampler map.
	textureSampler samplerBindMap
}

// setPipelineMap installs a pipeline's sampler map and returns the texture
// units whose pairing changed.
func (s *samplerState) setPipelineMap(m *samplerBindMap) uint32 {
	var dirty uint32
	for unit, next := range m {
		if s.textureSampler[unit] != next {
			s.textureSampler[unit] = next
			dirty |= 1 << unit
		}
	}
	return dirty
}

// setSampler records the sampler bound at a sampler slot and returns its
// dirty bit, or 0 for a slot out of range.
func (s *samplerState) setSampler(slot, sampler uint32) uint32 {
	if slot >= maxTextureSlots {
		return 0
	}
	s.samplers[slot] = sampler
	return 1 << slot
}

// rebind returns a BindSamplerCommand for every texture unit that is dirty
// or takes a dirty sampler slot (Rust rebind_sampler_states).
func (s *samplerState) rebind(dirtyTextures, dirtySamplers uint32) []Command {
	var cmds []Command
	for unit, next := range s.textureSampler {
		hasSlot := next != 0
		slot := uint32(next) - 1
		if dirtyTextures&(1<<unit) == 0 && (!hasSlot || dirtySamplers&(1<<slot) == 0) {
			continue
		}
		var sampler uint32
		if hasSlot {
			sampler = s.samplers[slot]
		}
		cmds = append(cmds, &BindSamplerCommand{unit: uint32(unit), sampler: sampler})
	}
	return cmds
}

// bindGroupSamplers records the samplers of a bind group in the state and
// returns the dirty texture units and sampler slots it touches: textures by
// their unit, samplers by their slot. groupInfos is the binding-to-slot table
// of the current pipeline layout; without it nothing is recorded.
func (s *samplerState) bindGroupSamplers(index uint32, group *BindGroup, groupInfos []BindGroupLayoutInfo) (dirtyTextures, dirtySamplers uint32) {
	if int(index) >= len(groupInfos) {
		return 0, 0
	}
	info := groupInfos[index]
	for _, entry := range group.entries {
		if int(entry.Binding) >= len(info.BindingToSlot) {
			continue
		}
		slot := uint32(info.BindingToSlot[entry.Binding])
		switch res := entry.Resource.(type) {
		case gputypes.SamplerBinding:
			dirtySamplers |= s.setSampler(slot, uint32(res.Sampler))
		case gputypes.TextureViewBinding:
			if group.layout.storageTexture(entry.Binding) == nil && slot < maxTextureSlots {
				dirtyTextures |= 1 << slot
			}
		}
	}
	return dirtyTextures, dirtySamplers
}

// BindSamplerCommand binds a sampler object to a texture unit; sampler 0
// unbinds it (Rust gles Command::BindSampler).
type BindSamplerCommand struct {
	unit    uint32
	sampler uint32
}

func (c *BindSamplerCommand) Execute(ctx *gl.Context) {
	ctx.BindSampler(c.unit, c.sampler)
}

// buildSamplerBindMap builds a pipeline's sampler map from the combined
// samplers naga reported for each of its stages. Rust wgpu-hal gles builds
// the same map over every stage (device.rs create_pipeline).
func buildSamplerBindMap(bindingMap map[ir.ResourceBinding]uint8, infos ...glsl.TranslationInfo) samplerBindMap {
	var m samplerBindMap
	for _, info := range infos {
		for _, tm := range info.TextureMappings {
			if tm.SamplerBinding == nil {
				continue
			}
			texKey := ir.ResourceBinding{Group: tm.TextureBinding.Group, Binding: tm.TextureBinding.Binding}
			samplerKey := ir.ResourceBinding{Group: tm.SamplerBinding.Group, Binding: tm.SamplerBinding.Binding}
			texUnit, texOk := bindingMap[texKey]
			samplerSlot, samplerOk := bindingMap[samplerKey]
			if texOk && samplerOk && texUnit < maxTextureSlots && samplerSlot < maxTextureSlots {
				m[texUnit] = samplerSlot + 1
			}
		}
	}
	return m
}
