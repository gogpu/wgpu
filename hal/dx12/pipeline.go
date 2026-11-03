// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"crypto/sha256"
	"fmt"
	"os"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/hlsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// -----------------------------------------------------------------------------
// ShaderModule Implementation
// -----------------------------------------------------------------------------

// ShaderModule implements hal.ShaderModule for DirectX 12.
// Stores the WGSL lowered to IR. Each pipeline stage is compiled when its
// pipeline is created, with that pipeline's layout (Device.stageBytecode), so
// a WGSL module never holds bytecode itself.
// For pre-compiled SPIR-V, entryPoints is populated directly.
type ShaderModule struct {
	wgslSource  string            // Raw WGSL source (shader cache key of the DXIL path)
	irModule    *shaderir.Module  // WGSL lowered to IR; nil for pre-compiled input
	entryPoints map[string][]byte // entryName → bytecode of pre-compiled input
	device      *Device
}

// Destroy releases the shader module resources.
func (m *ShaderModule) Destroy() {
	m.wgslSource = ""
	m.irModule = nil
	m.entryPoints = nil
	m.device = nil
}

// EntryPointBytecode returns the compiled DXBC bytecode for the given entry point.
func (m *ShaderModule) EntryPointBytecode(name string) []byte {
	if m.entryPoints == nil {
		return nil
	}
	return m.entryPoints[name]
}

// -----------------------------------------------------------------------------
// BindGroupLayout Implementation
// -----------------------------------------------------------------------------

// BindGroupLayoutEntry stores binding information for root signature creation.
type BindGroupLayoutEntry struct {
	Binding    uint32
	Type       BindingType
	Visibility gputypes.ShaderStages
	Count      uint32 // For arrays
	// HasDynamicOffset marks a buffer binding whose offset SetBindGroup
	// adds to: a uniform buffer becomes a root CBV, a storage buffer reads
	// its offset from root constants (dynamicBuffer).
	HasDynamicOffset bool
}

// isDynamicUniform reports whether the entry is a uniform buffer with a
// dynamic offset, which is bound as a root CBV instead of a descriptor in
// the group's table.
func (e *BindGroupLayoutEntry) isDynamicUniform() bool {
	return e.HasDynamicOffset && e.Type == BindingTypeUniformBuffer
}

// isDynamicStorage reports whether the entry is a storage buffer with a
// dynamic offset, whose offset the shader reads from root constants.
func (e *BindGroupLayoutEntry) isDynamicStorage() bool {
	return e.HasDynamicOffset && (e.Type == BindingTypeStorageBuffer || e.Type == BindingTypeReadOnlyStorageBuffer)
}

// BindingType describes the type of resource binding.
type BindingType uint8

const (
	BindingTypeUniformBuffer BindingType = iota
	BindingTypeStorageBuffer
	BindingTypeReadOnlyStorageBuffer
	BindingTypeSampler
	BindingTypeComparisonSampler
	BindingTypeSampledTexture
	BindingTypeStorageTexture
)

// BindGroupLayout implements hal.BindGroupLayout for DirectX 12.
type BindGroupLayout struct {
	entries []BindGroupLayoutEntry
	device  *Device
}

// Destroy releases the bind group layout resources.
func (l *BindGroupLayout) Destroy() {
	l.entries = nil
	l.device = nil
}

// Entries returns the bind group layout entries.
func (l *BindGroupLayout) Entries() []BindGroupLayoutEntry {
	return l.entries
}

// -----------------------------------------------------------------------------
// PipelineLayout Implementation
// -----------------------------------------------------------------------------

// rootParamMapping stores actual root parameter indices for a single bind group.
// Values are -1 when the group has no descriptors of that type.
//
// A group's dynamic buffers follow its table, as in Rust wgpu-hal
// (dx12/device.rs create_pipeline_layout): one root CBV per dynamic uniform
// buffer, in binding order, then one root constants parameter holding the
// offsets of the dynamic storage buffers and, after them, their binding sizes
// (naga's DynamicStorageBufferSizeIndex, which arrayLength() reads).
type rootParamMapping struct {
	cbvSrvUavIndex int // root param index for CBV/SRV/UAV table, or -1

	dynamicUniformIndex int // root param index of the first dynamic uniform root CBV, or -1
	dynamicStorageIndex int // root param index of the dynamic storage root constants, or -1
	dynamicStorageCount int // number of dynamic storage buffers in the group
}

func newRootParamMapping() rootParamMapping {
	return rootParamMapping{cbvSrvUavIndex: -1, dynamicUniformIndex: -1, dynamicStorageIndex: -1}
}

// PipelineLayout implements hal.PipelineLayout for DirectX 12.
// It wraps an ID3D12RootSignature and stores naga HLSL options for deferred
// shader compilation, matching Rust wgpu-hal architecture.
type PipelineLayout struct {
	rootSignature             *d3d12.ID3D12RootSignature
	rootSignatureHash         [32]byte // SHA-256 of serialized root signature blob (PSO cache key)
	bindGroupLayouts          []*BindGroupLayout
	groupMappings             []rootParamMapping // actual root param indices per bind group
	samplerRootIndex          int                // root param index for global sampler heap table, or -1
	specialConstantsRootIndex int                // root param index of the special constants (first vertex/instance, workgroup count)
	nagaOptions               *hlsl.Options      // HLSL compile options with proper BindingMap
	device                    *Device
}

// Destroy releases the pipeline layout resources.
func (l *PipelineLayout) Destroy() {
	if l.rootSignature != nil {
		l.rootSignature.Release()
		l.rootSignature = nil
	}
	l.bindGroupLayouts = nil
	l.device = nil
}

// RootSignature returns the underlying D3D12 root signature.
func (l *PipelineLayout) RootSignature() *d3d12.ID3D12RootSignature {
	return l.rootSignature
}

// -----------------------------------------------------------------------------
// BindGroup Implementation (Updated)
// -----------------------------------------------------------------------------

// BindGroup implements hal.BindGroup for DirectX 12.
// Uses the Rust wgpu-hal sampler heap pattern: samplers are not copied into
// a per-group sampler table. Instead, a sampler index buffer (StructuredBuffer<uint>)
// is created containing the global sampler pool indices. The shader reads
// nagaSamplerHeap[indexBuffer[binding]] to access the sampler.
type BindGroup struct {
	layout        *BindGroupLayout
	gpuDescHandle d3d12.D3D12_GPU_DESCRIPTOR_HANDLE // GPU handle for CBV/SRV/UAV table
	device        *Device

	// Tracked allocation indices for descriptor recycling
	viewHeapIndex uint32
	viewCount     uint32

	// Sampler index buffer (StructuredBuffer<uint> containing sampler pool indices).
	// This GPU buffer is allocated per bind group if the group has any samplers.
	samplerIndexBuffer *d3d12.ID3D12Resource

	// storageBuffers holds references to storage buffers bound in this group.
	// Populated at CreateBindGroup time by matching layout entries with type
	// BindingTypeStorageBuffer against the corresponding buffer bindings.
	// Used by ComputePassEncoder to track which buffers transition to
	// UNORDERED_ACCESS after Dispatch() (BUG-DX12-012 fix).
	storageBuffers         []*Buffer
	readOnlyStorageBuffers []*Buffer
	uniformBuffers         []*Buffer
	sampledTextures        []*TextureView
	storageTextures        []*TextureView

	// dynamicBuffers are the buffers with a dynamic offset, in binding
	// order: the order of SetBindGroup's offsets.
	dynamicBuffers []dynamicBuffer
}

// dynamicBuffer is a buffer binding with a dynamic offset, as Rust wgpu-hal's
// DynamicBuffer (dx12/mod.rs). A uniform buffer is bound as a root CBV at
// gpuVA plus the offset. A storage buffer keeps its descriptor in the table,
// viewed to the end of the buffer, and the shader adds the offset from root
// constants; size is its binding size, which arrayLength() counts.
type dynamicBuffer struct {
	uniform bool
	gpuVA   uint64 // uniform: GPU address of the binding at offset 0
	size    uint32 // storage: binding size in bytes
}

// Destroy releases the bind group resources and recycles descriptor heap slots.
func (g *BindGroup) Destroy() {
	if g.device != nil {
		if g.viewCount > 0 {
			g.device.viewHeap.Free(g.viewHeapIndex, g.viewCount)
		}
	}
	if g.samplerIndexBuffer != nil {
		g.samplerIndexBuffer.Release()
		g.samplerIndexBuffer = nil
	}
	g.layout = nil
	g.device = nil
}

// GPUDescriptorHandle returns the GPU descriptor handle for CBV/SRV/UAV.
func (g *BindGroup) GPUDescriptorHandle() d3d12.D3D12_GPU_DESCRIPTOR_HANDLE {
	return g.gpuDescHandle
}

// -----------------------------------------------------------------------------
// Pipeline Creation Helpers
// -----------------------------------------------------------------------------

// pipelineLayoutResult holds the output of root signature creation.
type pipelineLayoutResult struct {
	rootSignature     *d3d12.ID3D12RootSignature
	rootSignatureHash [32]byte
	groupMappings     []rootParamMapping
	samplerRootIndex  int
	// specialConstantsRootIndex is the root parameter of the special constants.
	specialConstantsRootIndex int
	nagaOptions               *hlsl.Options
}

// createRootSignatureFromLayouts creates a D3D12 root signature from bind group layouts.
// Implements the Rust wgpu-hal architecture:
//   - Monotonic per-type register counters (bind_cbv, bind_srv, bind_uav)
//   - Sampler bindings mapped to space=255 with index_within_group as register
//   - Per-group sampler index buffer SRV in the CBV/SRV/UAV table
//   - Global sampler heap root parameter (2x2048 sampler ranges)
//   - Full naga HLSL options with BindingMap and SamplerBufferBindingMap
func (d *Device) createRootSignatureFromLayouts(layouts []hal.BindGroupLayout) (*pipelineLayoutResult, error) {
	var rootParams []d3d12.D3D12_ROOT_PARAMETER
	var allRanges []d3d12.D3D12_DESCRIPTOR_RANGE // flat slice to prevent reallocation
	var groupMappings []rootParamMapping

	// Monotonic per-type register counters (matches Rust wgpu-hal).
	var bindCBV, bindSRV, bindUAV uint32

	// naga HLSL binding maps.
	bindingMap := make(map[ir.ResourceBinding]hlsl.BindTarget)
	samplerBufferBindingMap := make(map[uint32]hlsl.BindTarget)
	var dynamicStorageTargets map[uint32]hlsl.OffsetsBindTarget

	// Track whether any bind group has samplers.
	samplerInAnyGroup := false

	// Pre-count total descriptor ranges to avoid reallocation.
	totalRanges := 0
	for _, layout := range layouts {
		bgLayout, ok := layout.(*BindGroupLayout)
		if !ok {
			continue
		}
		samplerInGroup := false
		for i := range bgLayout.entries {
			entry := &bgLayout.entries[i]
			switch {
			case entry.Type == BindingTypeSampler, entry.Type == BindingTypeComparisonSampler:
				samplerInGroup = true
			case entry.isDynamicUniform():
				// A root CBV, not a descriptor range.
			default:
				totalRanges++
			}
		}
		if samplerInGroup {
			totalRanges++ // sampler index buffer SRV
			samplerInAnyGroup = true
		}
	}
	if samplerInAnyGroup {
		totalRanges += 2 // global sampler heap ranges (standard + comparison)
	}
	allRanges = make([]d3d12.D3D12_DESCRIPTOR_RANGE, 0, totalRanges)

	for groupIdx, layout := range layouts {
		bgLayout, ok := layout.(*BindGroupLayout)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid bind group layout type at index %d", groupIdx)
		}

		mapping := newRootParamMapping()

		// Skip empty layouts
		if len(bgLayout.entries) == 0 {
			groupMappings = append(groupMappings, mapping)
			continue
		}

		// The dynamic storage buffers' root constants hold their offsets
		// first and their binding sizes after them.
		for i := range bgLayout.entries {
			if bgLayout.entries[i].isDynamicStorage() {
				mapping.dynamicStorageCount++
			}
		}
		var dynamicStorageIdx uint32

		// Build CBV/SRV/UAV descriptor ranges for this group.
		rangeBase := len(allRanges)

		for _, entry := range bgLayout.entries {
			rangeType, isSampler := bindingTypeToD3D12DescriptorRangeType(entry.Type)
			if isSampler || entry.isDynamicUniform() {
				continue // Samplers handled separately below, dynamic uniform buffers after the table
			}

			// Assign register using per-type monotonic counter.
			var reg *uint32
			switch rangeType {
			case d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_CBV:
				reg = &bindCBV
			case d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV:
				reg = &bindSRV
			case d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV:
				reg = &bindUAV
			default:
				reg = &bindSRV
			}

			target := hlsl.BindTarget{
				Space:    0,
				Register: *reg,
			}
			if entry.isDynamicStorage() {
				offsetIdx := dynamicStorageIdx
				sizeIdx := uint32(mapping.dynamicStorageCount) + dynamicStorageIdx
				target.DynamicStorageBufferOffsetsIndex = &offsetIdx
				target.DynamicStorageBufferSizeIndex = &sizeIdx
				dynamicStorageIdx++
			}
			bindingMap[ir.ResourceBinding{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}] = target

			allRanges = append(allRanges, d3d12.D3D12_DESCRIPTOR_RANGE{
				RangeType:                         rangeType,
				NumDescriptors:                    1,
				BaseShaderRegister:                *reg,
				RegisterSpace:                     0,
				OffsetInDescriptorsFromTableStart: 0xFFFFFFFF, // D3D12_DESCRIPTOR_RANGE_OFFSET_APPEND
			})
			*reg++
		}

		// Handle samplers: assign to BindingMap with space=255 and
		// index_within_group as register. Add sampler index buffer SRV.
		var samplerIdxInGroup uint32
		for _, entry := range bgLayout.entries {
			if entry.Type != BindingTypeSampler && entry.Type != BindingTypeComparisonSampler {
				continue
			}
			bindingMap[ir.ResourceBinding{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}] = hlsl.BindTarget{
				Space:    255,
				Register: samplerIdxInGroup,
			}
			samplerIdxInGroup++
		}

		if samplerIdxInGroup > 0 {
			// Add sampler index buffer SRV to the CBV/SRV/UAV table.
			// This is the StructuredBuffer<uint> that maps sampler binding index
			// to the global sampler heap slot.
			samplerBufferBindingMap[uint32(groupIdx)] = hlsl.BindTarget{
				Space:    0,
				Register: bindSRV,
			}
			allRanges = append(allRanges, d3d12.D3D12_DESCRIPTOR_RANGE{
				RangeType:                         d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV,
				NumDescriptors:                    1,
				BaseShaderRegister:                bindSRV,
				RegisterSpace:                     0,
				OffsetInDescriptorsFromTableStart: 0xFFFFFFFF,
			})
			bindSRV++
		}

		// Create root parameter for CBV/SRV/UAV table (includes sampler index buffer SRV).
		if len(allRanges) > rangeBase {
			mapping.cbvSrvUavIndex = len(rootParams)

			param := d3d12.D3D12_ROOT_PARAMETER{
				ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_DESCRIPTOR_TABLE,
				ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
			}

			rangeSlice := allRanges[rangeBase:]
			table := (*d3d12.D3D12_ROOT_DESCRIPTOR_TABLE)(unsafe.Pointer(&param.Union[0]))
			table.NumDescriptorRanges = uint32(len(rangeSlice))
			table.DescriptorRanges = &rangeSlice[0]

			rootParams = append(rootParams, param)
		}

		// Root CBVs for the dynamic uniform buffers, so SetBindGroup can move
		// their GPU address by the offset (Rust: "easier than trying to
		// patch up the offset on the shader side"). D3D12 does not bounds
		// check root descriptors, so naga clamps the indices into them
		// (RestrictIndexing).
		for _, entry := range bgLayout.entries {
			if !entry.isDynamicUniform() {
				continue
			}
			if mapping.dynamicUniformIndex < 0 {
				mapping.dynamicUniformIndex = len(rootParams)
			}
			bindingMap[ir.ResourceBinding{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}] = hlsl.BindTarget{Space: 0, Register: bindCBV, RestrictIndexing: true}
			param := d3d12.D3D12_ROOT_PARAMETER{
				ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_CBV,
				ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
			}
			descriptor := (*d3d12.D3D12_ROOT_DESCRIPTOR)(unsafe.Pointer(&param.Union[0]))
			descriptor.ShaderRegister = bindCBV
			descriptor.RegisterSpace = 0
			rootParams = append(rootParams, param)
			bindCBV++
		}

		// Root constants for the dynamic storage buffers: their offsets,
		// which the shader adds to every access, then their binding sizes.
		if mapping.dynamicStorageCount > 0 {
			mapping.dynamicStorageIndex = len(rootParams)
			values := uint32(2 * mapping.dynamicStorageCount)
			if dynamicStorageTargets == nil {
				dynamicStorageTargets = make(map[uint32]hlsl.OffsetsBindTarget)
			}
			dynamicStorageTargets[uint32(groupIdx)] = hlsl.OffsetsBindTarget{Space: 0, Register: bindCBV, Size: values}
			param := d3d12.D3D12_ROOT_PARAMETER{
				ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_32BIT_CONSTANTS,
				ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
			}
			constants := (*d3d12.D3D12_ROOT_CONSTANTS)(unsafe.Pointer(&param.Union[0]))
			constants.ShaderRegister = bindCBV
			constants.RegisterSpace = 0
			constants.Num32BitValues = values
			rootParams = append(rootParams, param)
			bindCBV++
		}

		groupMappings = append(groupMappings, mapping)
	}

	// Global sampler heap root parameter (matches Rust wgpu-hal).
	// Two ranges in the same descriptor table: standard samplers at s0-s2047
	// and comparison samplers at s2048-s4095, both in space 0.
	samplerRootIndex := -1
	if samplerInAnyGroup {
		samplerRootIndex = len(rootParams)

		rangeBase := len(allRanges)
		// Standard samplers (registers 0-2047) and comparison samplers (registers 2048-4095).
		allRanges = append(allRanges,
			d3d12.D3D12_DESCRIPTOR_RANGE{
				RangeType:                         d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SAMPLER,
				NumDescriptors:                    2048,
				BaseShaderRegister:                0,
				RegisterSpace:                     0,
				OffsetInDescriptorsFromTableStart: 0,
			},
			d3d12.D3D12_DESCRIPTOR_RANGE{
				RangeType:                         d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SAMPLER,
				NumDescriptors:                    2048,
				BaseShaderRegister:                2048,
				RegisterSpace:                     0,
				OffsetInDescriptorsFromTableStart: 0,
			},
		)

		samplerRanges := allRanges[rangeBase:]
		param := d3d12.D3D12_ROOT_PARAMETER{
			ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_DESCRIPTOR_TABLE,
			ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
		}
		table := (*d3d12.D3D12_ROOT_DESCRIPTOR_TABLE)(unsafe.Pointer(&param.Union[0]))
		table.NumDescriptorRanges = uint32(len(samplerRanges))
		table.DescriptorRanges = &samplerRanges[0]

		rootParams = append(rootParams, param)
	}

	// Special constants: three 32-bit root constants at the next CBV
	// register, (first_vertex, first_instance, unused) for draws and
	// (x, y, z) for dispatches. D3D12's SV_VertexID and SV_InstanceID do not
	// include the first vertex and instance, and there is no workgroup count
	// built-in, so naga adds and reads these (SpecialConstantsBinding). Rust
	// wgpu-hal adds them to every layout, since wgpu-core always sets
	// FIRST_VERTEX_INSTANCE | NUM_WORK_GROUPS (dx12/device.rs
	// create_pipeline_layout).
	specialConstantsRootIndex := len(rootParams)
	specialConstantsBinding := hlsl.BindTarget{Space: 0, Register: bindCBV}
	{
		param := d3d12.D3D12_ROOT_PARAMETER{
			ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_32BIT_CONSTANTS,
			ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
		}
		constants := (*d3d12.D3D12_ROOT_CONSTANTS)(unsafe.Pointer(&param.Union[0]))
		constants.ShaderRegister = specialConstantsBinding.Register
		constants.RegisterSpace = uint32(specialConstantsBinding.Space)
		constants.Num32BitValues = specialConstantsCount
		rootParams = append(rootParams, param)
	}

	// Build root signature description
	desc := d3d12.D3D12_ROOT_SIGNATURE_DESC{
		Flags: d3d12.D3D12_ROOT_SIGNATURE_FLAG_ALLOW_INPUT_ASSEMBLER_INPUT_LAYOUT,
	}

	if len(rootParams) > 0 {
		desc.NumParameters = uint32(len(rootParams))
		desc.Parameters = &rootParams[0]
	}

	// Serialize root signature
	blob, errorBlob, err := d.instance.d3d12Lib.SerializeRootSignature(&desc, d3d12.D3D_ROOT_SIGNATURE_VERSION_1_0)
	if err != nil {
		if errorBlob != nil {
			errorBlob.Release()
		}
		return nil, fmt.Errorf("dx12: failed to serialize root signature: %w", err)
	}
	defer blob.Release()

	rootSigHash := sha256.Sum256(unsafe.Slice((*byte)(blob.GetBufferPointer()), blob.GetBufferSize()))

	// Check if device is already lost before attempting to create root signature.
	if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
		d.logDREDBreadcrumbs()
		return nil, fmt.Errorf("dx12: device already removed before CreateRootSignature: %w", reason)
	}

	// Create root signature
	rootSig, err := d.raw.CreateRootSignature(0, blob.GetBufferPointer(), blob.GetBufferSize())
	if err != nil {
		if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
			d.logDREDBreadcrumbs()
			return nil, fmt.Errorf("dx12: failed to create root signature (device removed: %s): %w", reason.Error(), err)
		}
		return nil, fmt.Errorf("dx12: failed to create root signature: %w", err)
	}

	// Build naga HLSL options for deferred shader compilation.
	// FXC compiles shader model 5.1 at most (shaderStageToTarget). Bounds
	// checks follow Rust's restrict_indexing (true, what wgpu-hal passes):
	// Index and Buffer Restrict; D3D12 checks image loads itself; binding
	// arrays are Unchecked, as Rust's HLSL writer never checks them. The
	// DXIL path takes the bindings from these options (compileStageDXIL).
	nagaOpts := &hlsl.Options{
		ShaderModel:                        backend.ShaderModel{Major: 5, Minor: 1},
		BindingMap:                         bindingMap,
		SamplerBufferBindingMap:            samplerBufferBindingMap,
		DynamicStorageBufferOffsetsTargets: dynamicStorageTargets,
		SpecialConstantsBinding:            &specialConstantsBinding,
		SamplerHeapTargets: &hlsl.SamplerHeapBindTargets{
			StandardSamplers:   hlsl.BindTarget{Space: 0, Register: 0},
			ComparisonSamplers: hlsl.BindTarget{Space: 0, Register: 2048},
		},
		BoundsCheckPolicies: backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckRestrict,
			Buffer:       backend.BoundsCheckRestrict,
			ImageLoad:    backend.BoundsCheckRestrict,
			BindingArray: backend.BoundsCheckUnchecked,
		},
	}

	return &pipelineLayoutResult{
		rootSignature:             rootSig,
		rootSignatureHash:         rootSigHash,
		groupMappings:             groupMappings,
		samplerRootIndex:          samplerRootIndex,
		specialConstantsRootIndex: specialConstantsRootIndex,
		nagaOptions:               nagaOpts,
	}, nil
}

// bindingTypeToD3D12DescriptorRangeType converts binding type to D3D12 descriptor range type.
// Returns the range type and whether it's a sampler.
func bindingTypeToD3D12DescriptorRangeType(t BindingType) (d3d12.D3D12_DESCRIPTOR_RANGE_TYPE, bool) {
	switch t {
	case BindingTypeUniformBuffer:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_CBV, false
	case BindingTypeStorageBuffer:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV, false
	case BindingTypeReadOnlyStorageBuffer:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV, false
	case BindingTypeSampler, BindingTypeComparisonSampler:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SAMPLER, true
	case BindingTypeSampledTexture:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV, false
	case BindingTypeStorageTexture:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_UAV, false
	default:
		return d3d12.D3D12_DESCRIPTOR_RANGE_TYPE_SRV, false
	}
}

// -----------------------------------------------------------------------------
// RenderPipeline Creation
// -----------------------------------------------------------------------------

// buildGraphicsPipelineStateDesc builds a D3D12_GRAPHICS_PIPELINE_STATE_DESC from a render pipeline descriptor.
// Note: semanticNames is passed to keep the byte slices alive during PSO creation (D3D12 reads the pointers).
// vsBytecode and psBytecode are the compiled stages (Device.stageBytecode).
func (d *Device) buildGraphicsPipelineStateDesc(
	desc *hal.RenderPipelineDescriptor,
	inputElements []d3d12.D3D12_INPUT_ELEMENT_DESC,
	_ [][]byte, // Keep semantic name strings alive (prevents GC during PSO creation)
	vsBytecode, psBytecode []byte,
) (*d3d12.D3D12_GRAPHICS_PIPELINE_STATE_DESC, error) {
	psoDesc := &d3d12.D3D12_GRAPHICS_PIPELINE_STATE_DESC{}

	// Set root signature from pipeline layout.
	// DX12 requires a valid root signature for every PSO. If no layout is
	// provided (shader has no resource bindings), use the device's shared
	// empty root signature. This prevents GPU hangs on drivers that don't
	// gracefully handle nil root signatures (observed as DPC_WATCHDOG_VIOLATION).
	if desc.Layout != nil {
		pipelineLayout, ok := desc.Layout.(*PipelineLayout)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid pipeline layout type")
		}
		psoDesc.RootSignature = pipelineLayout.rootSignature
	} else {
		emptyRS, err := d.getOrCreateEmptyRootSignature()
		if err != nil {
			return nil, fmt.Errorf("dx12: failed to get empty root signature: %w", err)
		}
		psoDesc.RootSignature = emptyRS
	}

	// Set vertex shader
	if desc.Vertex.Module != nil {
		bc, err := resolveShaderBytecode(desc.Vertex.Module, vsBytecode, desc.Vertex.EntryPoint, "GOGPU_DX12_DXIL_OVERRIDE_VS")
		if err != nil {
			return nil, fmt.Errorf("dx12: vertex shader: %w", err)
		}
		psoDesc.VS = d3d12.D3D12_SHADER_BYTECODE{
			ShaderBytecode: unsafe.Pointer(&bc[0]),
			BytecodeLength: uintptr(len(bc)),
		}
	}

	// Set pixel (fragment) shader
	if desc.Fragment != nil && desc.Fragment.Module != nil {
		bc, err := resolveShaderBytecode(desc.Fragment.Module, psBytecode, desc.Fragment.EntryPoint, "GOGPU_DX12_DXIL_OVERRIDE_PS")
		if err != nil {
			return nil, fmt.Errorf("dx12: fragment shader: %w", err)
		}
		psoDesc.PS = d3d12.D3D12_SHADER_BYTECODE{
			ShaderBytecode: unsafe.Pointer(&bc[0]),
			BytecodeLength: uintptr(len(bc)),
		}
	}

	// Input layout
	if len(inputElements) > 0 {
		psoDesc.InputLayout = d3d12.D3D12_INPUT_LAYOUT_DESC{
			InputElementDescs: &inputElements[0],
			NumElements:       uint32(len(inputElements)),
		}
	}

	// Primitive topology type
	psoDesc.PrimitiveTopologyType = primitiveTopologyTypeToD3D12(desc.Primitive.Topology)

	// Index buffer strip cut value for strip topologies
	if desc.Primitive.StripIndexFormat != nil {
		switch *desc.Primitive.StripIndexFormat {
		case gputypes.IndexFormatUint16:
			psoDesc.IBStripCutValue = d3d12.D3D12_INDEX_BUFFER_STRIP_CUT_VALUE_0xFFFF
		case gputypes.IndexFormatUint32:
			psoDesc.IBStripCutValue = d3d12.D3D12_INDEX_BUFFER_STRIP_CUT_VALUE_0xFFFFFFFF
		}
	}

	// Rasterizer state
	psoDesc.RasterizerState = d3d12.D3D12_RASTERIZER_DESC{
		FillMode:              d3d12.D3D12_FILL_MODE_SOLID,
		CullMode:              cullModeToD3D12(desc.Primitive.CullMode),
		FrontCounterClockwise: frontFaceToD3D12(desc.Primitive.FrontFace),
		DepthBias:             0,
		DepthBiasClamp:        0,
		SlopeScaledDepthBias:  0,
		DepthClipEnable:       1, // TRUE - enable depth clipping by default
		MultisampleEnable:     0,
		AntialiasedLineEnable: 0,
		ForcedSampleCount:     0,
		ConservativeRaster:    d3d12.D3D12_CONSERVATIVE_RASTERIZATION_MODE_OFF,
	}

	// Handle unclipped depth
	if desc.Primitive.UnclippedDepth {
		psoDesc.RasterizerState.DepthClipEnable = 0 // FALSE
	}

	// Depth/stencil state
	if desc.DepthStencil != nil {
		psoDesc.DepthStencilState = buildDepthStencilDesc(desc.DepthStencil)
		psoDesc.DSVFormat = textureFormatToD3D12(desc.DepthStencil.Format)

		// Apply depth bias from depth/stencil state
		psoDesc.RasterizerState.DepthBias = desc.DepthStencil.DepthBias
		psoDesc.RasterizerState.DepthBiasClamp = desc.DepthStencil.DepthBiasClamp
		psoDesc.RasterizerState.SlopeScaledDepthBias = desc.DepthStencil.DepthBiasSlopeScale
	}

	// Blend state — IndependentBlendEnable must be TRUE so that D3D12 uses
	// the per-target blend descriptors in RenderTarget[0..N] instead of
	// replicating RenderTarget[0] across all targets. This is required for
	// correct MRT (multiple render targets) behavior.
	// Matches Rust wgpu-hal: IndependentBlendEnable: true.into() (dx12/device.rs:1980).
	psoDesc.BlendState = d3d12.D3D12_BLEND_DESC{
		AlphaToCoverageEnable:  boolToInt32(desc.Multisample.AlphaToCoverageEnabled),
		IndependentBlendEnable: 1, // TRUE: each render target has its own blend state
	}

	// Color targets
	if desc.Fragment != nil {
		psoDesc.NumRenderTargets = uint32(len(desc.Fragment.Targets))
		for i, target := range desc.Fragment.Targets {
			if i >= 8 {
				break // D3D12 supports max 8 render targets
			}
			psoDesc.RTVFormats[i] = textureFormatToD3D12(target.Format)
			psoDesc.BlendState.RenderTarget[i] = buildRenderTargetBlendDesc(&target)
		}
	}

	// Multisample state
	psoDesc.SampleDesc = d3d12.DXGI_SAMPLE_DESC{
		Count:   desc.Multisample.Count,
		Quality: 0,
	}
	if psoDesc.SampleDesc.Count == 0 {
		psoDesc.SampleDesc.Count = 1
	}
	psoDesc.SampleMask = uint32(desc.Multisample.Mask & 0xFFFFFFFF)
	if psoDesc.SampleMask == 0 {
		psoDesc.SampleMask = 0xFFFFFFFF
	}

	return psoDesc, nil
}

// buildDepthStencilDesc builds a D3D12_DEPTH_STENCIL_DESC from a depth/stencil state.
func resolveShaderBytecode(module hal.ShaderModule, bytecode []byte, entryPoint, overrideEnv string) ([]byte, error) {
	if _, ok := module.(*ShaderModule); !ok {
		return nil, fmt.Errorf("invalid shader module type")
	}
	if path := os.Getenv(overrideEnv); path != "" {
		override, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s %q: %w", overrideEnv, path, err)
		}
		bytecode = override
		fmt.Fprintf(os.Stderr, "[dx12] shader overridden from %s (%d bytes)\n", path, len(override))
	}
	if len(bytecode) == 0 {
		return nil, fmt.Errorf("entry point %q not found in module", entryPoint)
	}
	return bytecode, nil
}

func buildDepthStencilDesc(ds *hal.DepthStencilState) d3d12.D3D12_DEPTH_STENCIL_DESC {
	desc := d3d12.D3D12_DEPTH_STENCIL_DESC{
		DepthEnable:      1, // TRUE - enable depth testing
		DepthWriteMask:   d3d12.D3D12_DEPTH_WRITE_MASK_ZERO,
		DepthFunc:        compareFunctionToD3D12(ds.DepthCompare),
		StencilEnable:    0,
		StencilReadMask:  uint8(ds.StencilReadMask),
		StencilWriteMask: uint8(ds.StencilWriteMask),
	}

	// Depth write
	if ds.DepthWriteEnabled {
		desc.DepthWriteMask = d3d12.D3D12_DEPTH_WRITE_MASK_ALL
	}

	// Disable depth test if compare is Always and no write
	if ds.DepthCompare == gputypes.CompareFunctionAlways && !ds.DepthWriteEnabled {
		desc.DepthEnable = 0
	}

	// Stencil operations
	hasStencil := ds.StencilFront.Compare != gputypes.CompareFunctionAlways ||
		ds.StencilFront.FailOp != hal.StencilOperationKeep ||
		ds.StencilFront.DepthFailOp != hal.StencilOperationKeep ||
		ds.StencilFront.PassOp != hal.StencilOperationKeep ||
		ds.StencilBack.Compare != gputypes.CompareFunctionAlways ||
		ds.StencilBack.FailOp != hal.StencilOperationKeep ||
		ds.StencilBack.DepthFailOp != hal.StencilOperationKeep ||
		ds.StencilBack.PassOp != hal.StencilOperationKeep

	if hasStencil {
		desc.StencilEnable = 1

		desc.FrontFace = d3d12.D3D12_DEPTH_STENCILOP_DESC{
			StencilFailOp:      stencilOpToD3D12(ds.StencilFront.FailOp),
			StencilDepthFailOp: stencilOpToD3D12(ds.StencilFront.DepthFailOp),
			StencilPassOp:      stencilOpToD3D12(ds.StencilFront.PassOp),
			StencilFunc:        compareFunctionToD3D12(ds.StencilFront.Compare),
		}

		desc.BackFace = d3d12.D3D12_DEPTH_STENCILOP_DESC{
			StencilFailOp:      stencilOpToD3D12(ds.StencilBack.FailOp),
			StencilDepthFailOp: stencilOpToD3D12(ds.StencilBack.DepthFailOp),
			StencilPassOp:      stencilOpToD3D12(ds.StencilBack.PassOp),
			StencilFunc:        compareFunctionToD3D12(ds.StencilBack.Compare),
		}
	}

	return desc
}

// buildRenderTargetBlendDesc builds a D3D12_RENDER_TARGET_BLEND_DESC from a color target state.
func buildRenderTargetBlendDesc(target *gputypes.ColorTargetState) d3d12.D3D12_RENDER_TARGET_BLEND_DESC {
	desc := d3d12.D3D12_RENDER_TARGET_BLEND_DESC{
		BlendEnable:           0,
		LogicOpEnable:         0,
		SrcBlend:              d3d12.D3D12_BLEND_ONE,
		DestBlend:             d3d12.D3D12_BLEND_ZERO,
		BlendOp:               d3d12.D3D12_BLEND_OP_ADD,
		SrcBlendAlpha:         d3d12.D3D12_BLEND_ONE,
		DestBlendAlpha:        d3d12.D3D12_BLEND_ZERO,
		BlendOpAlpha:          d3d12.D3D12_BLEND_OP_ADD,
		LogicOp:               d3d12.D3D12_LOGIC_OP_NOOP,
		RenderTargetWriteMask: colorWriteMaskToD3D12(target.WriteMask),
	}

	if target.Blend != nil {
		desc.BlendEnable = 1
		desc.SrcBlend = blendFactorToD3D12(target.Blend.Color.SrcFactor)
		desc.DestBlend = blendFactorToD3D12(target.Blend.Color.DstFactor)
		desc.BlendOp = blendOperationToD3D12(target.Blend.Color.Operation)
		desc.SrcBlendAlpha = blendFactorToD3D12(target.Blend.Alpha.SrcFactor)
		desc.DestBlendAlpha = blendFactorToD3D12(target.Blend.Alpha.DstFactor)
		desc.BlendOpAlpha = blendOperationToD3D12(target.Blend.Alpha.Operation)
	}

	return desc
}

// buildInputLayout builds input element descriptors from vertex buffer layouts.
func buildInputLayout(buffers []gputypes.VertexBufferLayout) ([]d3d12.D3D12_INPUT_ELEMENT_DESC, [][]byte) {
	var elements []d3d12.D3D12_INPUT_ELEMENT_DESC
	var semanticNames [][]byte // Keep strings alive

	// Create semantic names like "TEXCOORD0", "TEXCOORD1", etc.
	// D3D12 uses semantic name + index for vertex attributes

	for slotIdx, buffer := range buffers {
		for _, attr := range buffer.Attributes {
			// Create semantic name "LOC" with semantic index = shader location.
			// Must match naga HLSL output which uses LOC{N} for @location(N).
			semanticName := []byte("LOC\x00")
			semanticNames = append(semanticNames, semanticName)

			element := d3d12.D3D12_INPUT_ELEMENT_DESC{
				SemanticName:         &semanticNames[len(semanticNames)-1][0],
				SemanticIndex:        attr.ShaderLocation,
				Format:               vertexFormatToD3D12(attr.Format),
				InputSlot:            uint32(slotIdx),
				AlignedByteOffset:    uint32(attr.Offset),
				InputSlotClass:       inputStepModeToD3D12(buffer.StepMode),
				InstanceDataStepRate: 0,
			}

			if buffer.StepMode == gputypes.VertexStepModeInstance {
				element.InstanceDataStepRate = 1
			}

			elements = append(elements, element)
		}
	}

	return elements, semanticNames
}

// boolToInt32 converts a bool to int32 (for D3D12 BOOL).
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// -----------------------------------------------------------------------------
// RenderPipeline Implementation
// -----------------------------------------------------------------------------

// RenderPipeline implements hal.RenderPipeline for DirectX 12.
type RenderPipeline struct {
	pso                       *d3d12.ID3D12PipelineState
	rootSignature             *d3d12.ID3D12RootSignature // Reference, not owned
	groupMappings             []rootParamMapping         // bind group → root param index mapping
	samplerRootIndex          int                        // root param index for global sampler heap, or -1
	specialConstantsRootIndex int                        // root param index of the special constants, or -1 (no layout)
	topology                  d3d12.D3D_PRIMITIVE_TOPOLOGY
	vertexStrides             []uint32 // Strides per vertex buffer slot
}

// Destroy releases the render pipeline resources.
func (p *RenderPipeline) Destroy() {
	if p.pso != nil {
		p.pso.Release()
		p.pso = nil
	}
	// Don't release rootSignature - it's owned by PipelineLayout
	p.rootSignature = nil
	p.vertexStrides = nil
}

// PSO returns the underlying D3D12 pipeline state object.
func (p *RenderPipeline) PSO() *d3d12.ID3D12PipelineState {
	return p.pso
}

// RootSignature returns the root signature.
func (p *RenderPipeline) RootSignature() *d3d12.ID3D12RootSignature {
	return p.rootSignature
}

// Topology returns the primitive topology.
func (p *RenderPipeline) Topology() d3d12.D3D_PRIMITIVE_TOPOLOGY {
	return p.topology
}

// VertexStrides returns the vertex buffer strides.
func (p *RenderPipeline) VertexStrides() []uint32 {
	return p.vertexStrides
}

// -----------------------------------------------------------------------------
// ComputePipeline Implementation
// -----------------------------------------------------------------------------

// ComputePipeline implements hal.ComputePipeline for DirectX 12.
type ComputePipeline struct {
	pso                       *d3d12.ID3D12PipelineState
	rootSignature             *d3d12.ID3D12RootSignature // Reference, not owned
	groupMappings             []rootParamMapping         // bind group → root param index mapping
	samplerRootIndex          int                        // root param index for global sampler heap, or -1
	specialConstantsRootIndex int                        // root param index of the special constants, or -1 (no layout)
}

// Destroy releases the compute pipeline resources.
func (p *ComputePipeline) Destroy() {
	if p.pso != nil {
		p.pso.Release()
		p.pso = nil
	}
	// Don't release rootSignature - it's owned by PipelineLayout
	p.rootSignature = nil
}

// PSO returns the underlying D3D12 pipeline state object.
func (p *ComputePipeline) PSO() *d3d12.ID3D12PipelineState {
	return p.pso
}

// RootSignature returns the root signature.
func (p *ComputePipeline) RootSignature() *d3d12.ID3D12RootSignature {
	return p.rootSignature
}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var (
	_ hal.ShaderModule    = (*ShaderModule)(nil)
	_ hal.BindGroupLayout = (*BindGroupLayout)(nil)
	_ hal.PipelineLayout  = (*PipelineLayout)(nil)
	_ hal.BindGroup       = (*BindGroup)(nil)
	_ hal.RenderPipeline  = (*RenderPipeline)(nil)
	_ hal.ComputePipeline = (*ComputePipeline)(nil)
)
