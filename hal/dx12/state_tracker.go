// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"sort"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// subresourceState is the command-local state of one resource subresource.
// The initial state is captured on first use; current is changed only while
// recording the command list. Neither field represents state currently owned
// by the GPU until Queue.Submit reconciles and commits the summary.
type subresourceState struct {
	subresource uint32
	initial     d3d12.D3D12_RESOURCE_STATES
	current     d3d12.D3D12_RESOURCE_STATES
}

// commandStateSummary is the state hand-off carried by a CommandBuffer.
// Resource is deliberately opaque here so the pure tracker can be tested
// without creating D3D12 resources; the queue type-switches it back to
// *Buffer/*Texture when emitting native barriers and committing state.
type commandStateSummary struct {
	commandIndex int
	resource     any
	states       []subresourceState
}

// stateBarrierPlan describes a queue preamble transition without any native
// D3D12 handle. Queue.Submit resolves Resource to its raw resource when it
// builds a temporary preamble command list.
type stateBarrierPlan struct {
	resource    any
	subresource uint32
	before      d3d12.D3D12_RESOURCE_STATES
	after       d3d12.D3D12_RESOURCE_STATES
}

// commandStateTracker owns state summaries for one command encoder. The map
// key is a resource pointer in production and a comparable test value in the
// pure tracker tests.
type commandStateTracker struct {
	resources map[any]map[uint32]*subresourceState
}

func newCommandStateTracker() commandStateTracker {
	return commandStateTracker{resources: make(map[any]map[uint32]*subresourceState)}
}

func (t *commandStateTracker) reset() {
	t.resources = make(map[any]map[uint32]*subresourceState)
}

// transition records a local transition and reports whether an inline barrier
// is required. It always uses explicit barriers; buffer callers may opt into
// DX12's COMMON implicit-promotion rules through transitionBuffer below.
func (t *commandStateTracker) transition(resource any, subresource uint32, initial, target d3d12.D3D12_RESOURCE_STATES) (before d3d12.D3D12_RESOURCE_STATES, barrier bool) {
	return t.transitionWithPolicy(resource, subresource, initial, target, true)
}

func (t *commandStateTracker) transitionWithPolicy(resource any, subresource uint32, initial, target d3d12.D3D12_RESOURCE_STATES, explicit bool) (before d3d12.D3D12_RESOURCE_STATES, barrier bool) {
	if t.resources == nil {
		t.resources = make(map[any]map[uint32]*subresourceState)
	}
	bySubresource := t.resources[resource]
	if bySubresource == nil {
		bySubresource = make(map[uint32]*subresourceState)
		t.resources[resource] = bySubresource
	}
	state := bySubresource[subresource]
	if state == nil {
		state = &subresourceState{subresource: subresource, initial: initial, current: initial}
		bySubresource[subresource] = state
	}
	before = state.current
	if before == target {
		return before, false
	}
	state.current = target
	return before, explicit
}

func (t *commandStateTracker) summary() []commandStateSummary {
	if len(t.resources) == 0 {
		return nil
	}
	keys := make([]any, 0, len(t.resources))
	for key := range t.resources {
		keys = append(keys, key)
	}
	// Pointer keys are not ordered, but deterministic ordering is useful for
	// tests and for stable debug output. Keep the sort limited to comparable
	// stringified keys; queue semantics never depend on this order.
	sort.SliceStable(keys, func(i, j int) bool {
		return stateResourceSortKey(keys[i]) < stateResourceSortKey(keys[j])
	})

	result := make([]commandStateSummary, 0, len(keys))
	for _, key := range keys {
		bySubresource := t.resources[key]
		subresources := make([]uint32, 0, len(bySubresource))
		for subresource := range bySubresource {
			subresources = append(subresources, subresource)
		}
		sort.Slice(subresources, func(i, j int) bool { return subresources[i] < subresources[j] })
		states := make([]subresourceState, 0, len(subresources))
		for _, subresource := range subresources {
			states = append(states, *bySubresource[subresource])
		}
		result = append(result, commandStateSummary{resource: key, states: states})
	}
	return result
}

func stateResourceSortKey(resource any) string {
	if value, ok := resource.(string); ok {
		return value
	}
	return "resource"
}

// planSubmissionState computes queue preambles and the state that would be
// scheduled after all command buffers execute in order. Buffer target states
// remain visible while planning command lists in the same ExecuteCommandLists
// call, then decay to COMMON at that call boundary. The input map is never
// mutated, so a failed ExecuteCommandLists can leave ownership intact.
func planSubmissionState(scheduled map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES, commands []commandStateSummary) ([][]stateBarrierPlan, map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES) {
	current := cloneScheduledState(scheduled)
	commandCount := 0
	for _, command := range commands {
		if command.commandIndex >= commandCount {
			commandCount = command.commandIndex + 1
		}
	}
	preambles := make([][]stateBarrierPlan, commandCount)

	for _, command := range commands {
		resourceStates := current[command.resource]
		if resourceStates == nil {
			resourceStates = make(map[uint32]d3d12.D3D12_RESOURCE_STATES)
			current[command.resource] = resourceStates
		}
		for _, state := range command.states {
			actual, exists := resourceStates[state.subresource]
			if !exists {
				actual = state.initial
			}
			if actual != state.initial {
				preambles[command.commandIndex] = append(preambles[command.commandIndex], stateBarrierPlan{
					resource:    command.resource,
					subresource: state.subresource,
					before:      actual,
					after:       state.initial,
				})
			}
			resourceStates[state.subresource] = state.current
		}
	}

	// D3D12 buffers decay to COMMON only after the ExecuteCommandLists call.
	// Do this after all command-list preambles have been planned so later lists
	// in this same call still observe the preceding list's target state.
	for resource, states := range current {
		if _, ok := resource.(*Buffer); ok {
			states[0] = d3d12.D3D12_RESOURCE_STATE_COMMON
		}
	}

	return preambles, current
}

func cloneScheduledState(scheduled map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES) map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES {
	clone := make(map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES, len(scheduled))
	for resource, states := range scheduled {
		statesClone := make(map[uint32]d3d12.D3D12_RESOURCE_STATES, len(states))
		for subresource, state := range states {
			statesClone[subresource] = state
		}
		clone[resource] = statesClone
	}
	return clone
}

func (t *commandStateTracker) transitionBuffer(buffer *Buffer, target d3d12.D3D12_RESOURCE_STATES) (d3d12.D3D12_RESOURCE_STATES, bool) {
	initial := buffer.scheduledStateSnapshot()
	current := t.currentState(buffer, 0, initial)
	return t.transitionWithPolicy(buffer, 0, initial, target, needsExplicitBarrier(current, target))
}

func (t *commandStateTracker) transitionTexture(texture *Texture, subresource uint32, target d3d12.D3D12_RESOURCE_STATES) (d3d12.D3D12_RESOURCE_STATES, bool) {
	initial := texture.scheduledStateSnapshot(subresource)
	return t.transition(texture, subresource, initial, target)
}

func (t *commandStateTracker) transitionBufferRead(buffer *Buffer, requested d3d12.D3D12_RESOURCE_STATES) (before, target d3d12.D3D12_RESOURCE_STATES, barrier bool) {
	initial := buffer.scheduledStateSnapshot()
	current := t.currentState(buffer, 0, initial)
	target = mergeCompatibleReadStates(current, requested)
	before, barrier = t.transitionWithPolicy(buffer, 0, initial, target, needsExplicitBarrier(current, target))
	return before, target, barrier
}

func (t *commandStateTracker) transitionTextureRead(texture *Texture, subresource uint32, requested d3d12.D3D12_RESOURCE_STATES) (before, target d3d12.D3D12_RESOURCE_STATES, barrier bool) {
	initial := texture.scheduledStateSnapshot(subresource)
	current := t.currentState(texture, subresource, initial)
	target = mergeCompatibleReadStates(current, requested)
	before, barrier = t.transition(texture, subresource, initial, target)
	return before, target, barrier
}

func (t *commandStateTracker) currentState(resource any, subresource uint32, fallback d3d12.D3D12_RESOURCE_STATES) d3d12.D3D12_RESOURCE_STATES {
	if states := t.resources[resource]; states != nil {
		if state := states[subresource]; state != nil {
			return state.current
		}
	}
	return fallback
}

func mergeCompatibleReadStates(current, requested d3d12.D3D12_RESOURCE_STATES) d3d12.D3D12_RESOURCE_STATES {
	if current == d3d12.D3D12_RESOURCE_STATE_COMMON || requested == d3d12.D3D12_RESOURCE_STATE_COMMON {
		return requested
	}
	if current&d3d12WriteStates != 0 || requested&d3d12WriteStates != 0 {
		return requested
	}
	return current | requested
}

const d3d12WriteStates = d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET |
	d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS |
	d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE |
	d3d12.D3D12_RESOURCE_STATE_STREAM_OUT |
	d3d12.D3D12_RESOURCE_STATE_COPY_DEST |
	d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST |
	d3d12.D3D12_RESOURCE_STATE_VIDEO_DECODE_WRITE |
	d3d12.D3D12_RESOURCE_STATE_VIDEO_PROCESS_WRITE |
	d3d12.D3D12_RESOURCE_STATE_VIDEO_ENCODE_WRITE

func (t *commandStateTracker) transitionTextureRange(texture *Texture, subresources []uint32, target d3d12.D3D12_RESOURCE_STATES) []stateBarrierPlan {
	plans := make([]stateBarrierPlan, 0, len(subresources))
	for _, subresource := range subresources {
		before, barrier := t.transitionTexture(texture, subresource, target)
		if barrier {
			plans = append(plans, stateBarrierPlan{resource: texture, subresource: subresource, before: before, after: target})
		}
	}
	return plans
}

// textureSubresource identifies both the native subresource index and its
// logical plane. Keeping the plane alongside the index lets depth/stencil
// attachment policy choose independent states for depth and stencil while
// callers that only need native barriers can continue using uint32 indexes.
type textureSubresource struct {
	subresource uint32
	plane       uint32
}

func textureAspectPlanes(texture *Texture, aspect gputypes.TextureAspect) []uint32 {
	if texture == nil {
		return nil
	}
	if texture.planeCount() < 2 {
		return []uint32{0}
	}
	if texture.format == gputypes.TextureFormatStencil8 {
		if aspect == gputypes.TextureAspectDepthOnly {
			return nil
		}
		// Stencil8 is backed by D24S8 on DX12, but only physical plane 1
		// belongs to its WebGPU interface.
		return []uint32{1}
	}
	if texture.format == gputypes.TextureFormatDepth24Plus {
		if aspect == gputypes.TextureAspectStencilOnly {
			return nil
		}
		// Depth24Plus is also backed by D24S8 but exposes only depth.
		return []uint32{0}
	}
	switch aspect {
	case gputypes.TextureAspectDepthOnly:
		return []uint32{0}
	case gputypes.TextureAspectStencilOnly:
		return []uint32{1}
	default:
		// Undefined follows WebGPU's default/all-aspects behavior. For a
		// packed D24S8/D32S8 resource this selects both physical planes.
		return []uint32{0, 1}
	}
}

func textureRangeSubresourcePlanes(texture *Texture, r hal.TextureRange) []textureSubresource {
	return textureRangeSubresourcePlanesForPlanes(texture, r, textureAspectPlanes(texture, r.Aspect))
}

func textureRangeSubresourcePlanesForPlanes(texture *Texture, r hal.TextureRange, planes []uint32) []textureSubresource {
	if texture == nil {
		return nil
	}
	mipLevels := texture.mipLevels
	if mipLevels == 0 {
		mipLevels = 1
	}
	baseMip := r.BaseMipLevel
	mipCount := r.MipLevelCount
	if mipCount == 0 {
		if baseMip >= mipLevels {
			return nil
		}
		mipCount = mipLevels - baseMip
	}
	if baseMip >= mipLevels || mipCount > mipLevels-baseMip {
		return nil
	}

	layers := uint32(1)
	if texture.dimension != gputypes.TextureDimension3D {
		layers = texture.size.DepthOrArrayLayers
		if layers == 0 {
			layers = 1
		}
	}
	baseLayer := r.BaseArrayLayer
	layerCount := r.ArrayLayerCount
	if texture.dimension == gputypes.TextureDimension3D {
		// Origin.Z is a texel coordinate for 3D copies; it never selects an
		// array layer or alters the subresource identity.
		baseLayer = 0
		layerCount = 1
	}
	if layerCount == 0 {
		if baseLayer >= layers {
			return nil
		}
		layerCount = layers - baseLayer
	}
	if baseLayer >= layers || layerCount > layers-baseLayer {
		return nil
	}

	result := make([]textureSubresource, 0, mipCount*layerCount*uint32(len(planes)))
	for _, plane := range planes {
		for layer := uint32(0); layer < layerCount; layer++ {
			for mip := uint32(0); mip < mipCount; mip++ {
				arrayLayer := baseLayer + layer
				result = append(result, textureSubresource{
					subresource: texture.subresourceIndexForPlane(baseMip+mip, arrayLayer, plane),
					plane:       plane,
				})
			}
		}
	}
	return result
}

func textureRangeSubresources(texture *Texture, r hal.TextureRange) []uint32 {
	planes := textureRangeSubresourcePlanes(texture, r)
	result := make([]uint32, 0, len(planes))
	for _, plane := range planes {
		result = append(result, plane.subresource)
	}
	return result
}

type bufferTextureCopyPlan struct {
	subresource     uint32
	bufferOffset    uint64
	bufferOriginY   uint32
	footprintHeight uint32
	footprintDepth  uint32
	textureOriginZ  uint32
}

// planBufferTextureCopies converts WebGPU's depth-or-array-layer copy into
// the native operations D3D12 requires. A 3D texture keeps one subresource
// but emits a depth-1 footprint per Z slice; a 2D array emits one footprint
// and one texture subresource per layer.
func planBufferTextureCopies(texture *Texture, copy hal.ImageCopyTexture, layout hal.ImageDataLayout, size hal.Extent3D) []bufferTextureCopyPlan {
	if texture == nil {
		return nil
	}
	depthOrLayers := size.DepthOrArrayLayers
	if depthOrLayers == 0 {
		depthOrLayers = 1
	}
	planes := textureAspectPlanes(texture, copy.Aspect)
	blockHeight := textureFormatBlockHeight(texture.format)
	if texture.dimension == gputypes.TextureDimension3D {
		rowsPerImage := layout.RowsPerImage
		if rowsPerImage == 0 {
			rowsPerImage = size.Height
		}
		blockRowsPerImage := (rowsPerImage + blockHeight - 1) / blockHeight
		sliceStride := uint64(layout.BytesPerRow) * uint64(blockRowsPerImage)
		plans := make([]bufferTextureCopyPlan, 0, int(depthOrLayers)*len(planes))
		for _, plane := range planes {
			for z := uint32(0); z < depthOrLayers; z++ {
				logicalOffset := layout.Offset + uint64(z)*sliceStride
				placedOffset, bufferOriginY, footprintHeight := planPlacedBufferSlice(logicalOffset, layout.BytesPerRow, blockHeight, size.Height)
				plans = append(plans, bufferTextureCopyPlan{
					subresource:     texture.subresourceIndexForPlane(copy.MipLevel, 0, plane),
					bufferOffset:    placedOffset,
					bufferOriginY:   bufferOriginY,
					footprintHeight: footprintHeight,
					footprintDepth:  1,
					textureOriginZ:  copy.Origin.Z + z,
				})
			}
		}
		return plans
	}

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}
	blockRowsPerImage := (rowsPerImage + blockHeight - 1) / blockHeight
	layerStride := uint64(layout.BytesPerRow) * uint64(blockRowsPerImage)
	plans := make([]bufferTextureCopyPlan, 0, int(depthOrLayers)*len(planes))
	for _, plane := range planes {
		for layer := uint32(0); layer < depthOrLayers; layer++ {
			logicalOffset := layout.Offset + uint64(layer)*layerStride
			placedOffset, bufferOriginY, footprintHeight := planPlacedBufferSlice(logicalOffset, layout.BytesPerRow, blockHeight, size.Height)
			plans = append(plans, bufferTextureCopyPlan{
				subresource:     texture.subresourceIndexForPlane(copy.MipLevel, copy.Origin.Z+layer, plane),
				bufferOffset:    placedOffset,
				bufferOriginY:   bufferOriginY,
				footprintHeight: footprintHeight,
				footprintDepth:  1,
				textureOriginZ:  0,
			})
		}
	}
	return plans
}

// planPlacedBufferSlice maps one logical image/depth slice to a legacy D3D12
// placed footprint. Rewinding by whole block rows preserves the exact data
// address while meeting the 512-byte placement requirement.
func planPlacedBufferSlice(logicalOffset uint64, bytesPerRow, blockHeight, copyHeight uint32) (placedOffset uint64, bufferOriginY, footprintHeight uint32) {
	placedOffset = logicalOffset
	for bytesPerRow != 0 && placedOffset%d3d12TexturePlacementAlignment != 0 && placedOffset >= uint64(bytesPerRow) {
		placedOffset -= uint64(bytesPerRow)
		bufferOriginY += blockHeight
	}
	return placedOffset, bufferOriginY, copyHeight + bufferOriginY
}

func textureFormatBlockHeight(format gputypes.TextureFormat) uint32 {
	switch format {
	case gputypes.TextureFormatBC1RGBAUnorm,
		gputypes.TextureFormatBC1RGBAUnormSrgb,
		gputypes.TextureFormatBC2RGBAUnorm,
		gputypes.TextureFormatBC2RGBAUnormSrgb,
		gputypes.TextureFormatBC3RGBAUnorm,
		gputypes.TextureFormatBC3RGBAUnormSrgb,
		gputypes.TextureFormatBC4RUnorm,
		gputypes.TextureFormatBC4RSnorm,
		gputypes.TextureFormatBC5RGUnorm,
		gputypes.TextureFormatBC5RGSnorm,
		gputypes.TextureFormatBC6HRGBUfloat,
		gputypes.TextureFormatBC6HRGBFloat,
		gputypes.TextureFormatBC7RGBAUnorm,
		gputypes.TextureFormatBC7RGBAUnormSrgb:
		return 4
	default:
		return 1
	}
}

type textureTextureCopyPlan struct {
	srcSubresource uint32
	dstSubresource uint32
	srcFront       uint32
	srcBack        uint32
	dstZ           uint32
}

// planTextureTextureCopies applies D3D12's distinct 3D-volume and 2D-array
// copy models while pairing the selected source and destination planes.
func planTextureTextureCopies(src, dst *Texture, copy hal.TextureCopy) []textureTextureCopyPlan {
	if src == nil || dst == nil {
		return nil
	}
	srcPlanes := textureAspectPlanes(src, copy.SrcBase.Aspect)
	dstPlanes := textureAspectPlanes(dst, copy.DstBase.Aspect)
	planeCount := len(srcPlanes)
	if len(dstPlanes) < planeCount {
		planeCount = len(dstPlanes)
	}
	if planeCount == 0 {
		return nil
	}
	depthOrLayers := copy.Size.DepthOrArrayLayers
	if depthOrLayers == 0 {
		depthOrLayers = 1
	}

	if src.dimension == gputypes.TextureDimension3D || dst.dimension == gputypes.TextureDimension3D {
		// Core validation requires matching texture dimensions. Refuse to
		// invent layer/depth semantics for direct HAL callers that bypass it.
		if src.dimension != dst.dimension {
			return nil
		}
		plans := make([]textureTextureCopyPlan, 0, planeCount)
		for plane := 0; plane < planeCount; plane++ {
			plans = append(plans, textureTextureCopyPlan{
				srcSubresource: src.subresourceIndexForPlane(copy.SrcBase.MipLevel, 0, srcPlanes[plane]),
				dstSubresource: dst.subresourceIndexForPlane(copy.DstBase.MipLevel, 0, dstPlanes[plane]),
				srcFront:       copy.SrcBase.Origin.Z,
				srcBack:        copy.SrcBase.Origin.Z + depthOrLayers,
				dstZ:           copy.DstBase.Origin.Z,
			})
		}
		return plans
	}

	plans := make([]textureTextureCopyPlan, 0, planeCount*int(depthOrLayers))
	for plane := 0; plane < planeCount; plane++ {
		for layer := uint32(0); layer < depthOrLayers; layer++ {
			plans = append(plans, textureTextureCopyPlan{
				srcSubresource: src.subresourceIndexForPlane(copy.SrcBase.MipLevel, copy.SrcBase.Origin.Z+layer, srcPlanes[plane]),
				dstSubresource: dst.subresourceIndexForPlane(copy.DstBase.MipLevel, copy.DstBase.Origin.Z+layer, dstPlanes[plane]),
				srcFront:       0,
				srcBack:        1,
				dstZ:           0,
			})
		}
	}
	return plans
}

// depthStencilPlaneState returns the D3D12 state for one depth/stencil plane
// under a render-pass attachment's independent read-only flags.
func depthStencilPlaneState(plane uint32, depthReadOnly, stencilReadOnly, shaderReadable bool) d3d12.D3D12_RESOURCE_STATES {
	readOnly := depthReadOnly
	if plane != 0 {
		readOnly = stencilReadOnly
	}
	if !readOnly {
		return d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE
	}
	state := d3d12.D3D12_RESOURCE_STATE_DEPTH_READ
	if shaderReadable {
		state |= d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE | d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE
	}
	return state
}

// depthStencilReadOnlyFlags protects physical planes that a WebGPU view does
// not expose. DX12 still sees those planes in the packed DSV and otherwise
// requires them to be in a writable depth/stencil state.
func depthStencilReadOnlyFlags(format gputypes.TextureFormat, aspect gputypes.TextureAspect, depthReadOnly, stencilReadOnly bool) (bool, bool) {
	switch format {
	case gputypes.TextureFormatDepth16Unorm, gputypes.TextureFormatDepth32Float:
		// Single-plane DSV formats cannot encode READ_ONLY_STENCIL.
		stencilReadOnly = false
	case gputypes.TextureFormatStencil8:
		depthReadOnly = true
	case gputypes.TextureFormatDepth24Plus:
		stencilReadOnly = true
	case gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8:
		if aspect == gputypes.TextureAspectStencilOnly {
			depthReadOnly = true
		}
		if aspect == gputypes.TextureAspectDepthOnly {
			stencilReadOnly = true
		}
	}
	return depthReadOnly, stencilReadOnly
}

// resourceStateOwner is embedded by Buffer and Texture. It is intentionally
// tiny: queue state is the only mutable state owned by the resource, while
// command-local state lives in commandStateTracker.
type resourceStateOwner struct {
	mu             sync.Mutex
	bufferState    d3d12.D3D12_RESOURCE_STATES
	bufferStateSet bool
	textureStates  []d3d12.D3D12_RESOURCE_STATES
}

func (o *resourceStateOwner) setTextureStates(states []d3d12.D3D12_RESOURCE_STATES) {
	o.mu.Lock()
	o.textureStates = append(o.textureStates[:0], states...)
	o.mu.Unlock()
}
