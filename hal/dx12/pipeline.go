// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// -----------------------------------------------------------------------------
// ShaderModule Implementation
// -----------------------------------------------------------------------------

// ShaderModule implements hal.ShaderModule for DirectX 12.
// Stores per-entry-point DXBC bytecode compiled from WGSL via HLSL.
type ShaderModule struct {
	entryPoints map[string][]byte // entryName → DXBC bytecode
	device      *Device
}

// Destroy releases the shader module resources.
func (m *ShaderModule) Destroy() {
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
type rootParamMapping struct {
	cbvSrvUavIndex int // root param index for CBV/SRV/UAV table, or -1
	samplerIndex   int // root param index for sampler table, or -1
}

// PipelineLayout implements hal.PipelineLayout for DirectX 12.
// It wraps an ID3D12RootSignature.
type PipelineLayout struct {
	rootSignature    *d3d12.ID3D12RootSignature
	bindGroupLayouts []*BindGroupLayout
	groupMappings    []rootParamMapping // actual root param indices per bind group
	device           *Device
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
type BindGroup struct {
	layout           *BindGroupLayout
	gpuDescHandle    d3d12.D3D12_GPU_DESCRIPTOR_HANDLE // GPU handle for CBV/SRV/UAV table
	samplerGPUHandle d3d12.D3D12_GPU_DESCRIPTOR_HANDLE // GPU handle for sampler table
	device           *Device

	// Tracked allocation indices for descriptor recycling
	viewHeapIndex    uint32
	viewCount        uint32
	samplerHeapIndex uint32
	samplerCount     uint32
}

// Destroy releases the bind group resources and recycles descriptor heap slots.
func (g *BindGroup) Destroy() {
	if g.device != nil {
		if g.viewCount > 0 {
			g.device.viewHeap.Free(g.viewHeapIndex, g.viewCount)
		}
		if g.samplerCount > 0 {
			g.device.samplerHeap.Free(g.samplerHeapIndex, g.samplerCount)
		}
	}
	g.layout = nil
	g.device = nil
}

// GPUDescriptorHandle returns the GPU descriptor handle for CBV/SRV/UAV.
func (g *BindGroup) GPUDescriptorHandle() d3d12.D3D12_GPU_DESCRIPTOR_HANDLE {
	return g.gpuDescHandle
}

// SamplerGPUHandle returns the GPU descriptor handle for samplers.
func (g *BindGroup) SamplerGPUHandle() d3d12.D3D12_GPU_DESCRIPTOR_HANDLE {
	return g.samplerGPUHandle
}

// -----------------------------------------------------------------------------
// Pipeline Creation Helpers
// -----------------------------------------------------------------------------

// createRootSignatureFromLayouts creates a D3D12 root signature from bind group layouts.
// Returns the root signature and the mapping of bind group indices to root parameter indices.
func (d *Device) createRootSignatureFromLayouts(layouts []hal.BindGroupLayout) (*d3d12.ID3D12RootSignature, []rootParamMapping, error) {
	// Count total descriptor ranges needed
	var rootParams []d3d12.D3D12_ROOT_PARAMETER
	var descriptorRanges [][]d3d12.D3D12_DESCRIPTOR_RANGE // Keep ranges alive
	var groupMappings []rootParamMapping

	for groupIdx, layout := range layouts {
		bgLayout, ok := layout.(*BindGroupLayout)
		if !ok {
			return nil, nil, fmt.Errorf("dx12: invalid bind group layout type at index %d", groupIdx)
		}

		mapping := rootParamMapping{cbvSrvUavIndex: -1, samplerIndex: -1}

		// Skip empty layouts
		if len(bgLayout.entries) == 0 {
			groupMappings = append(groupMappings, mapping)
			continue
		}

		// Separate entries by descriptor type
		var cbvSrvUavRanges []d3d12.D3D12_DESCRIPTOR_RANGE
		var samplerRanges []d3d12.D3D12_DESCRIPTOR_RANGE

		for _, entry := range bgLayout.entries {
			rangeType, isSampler := bindingTypeToD3D12DescriptorRangeType(entry.Type)

			descRange := d3d12.D3D12_DESCRIPTOR_RANGE{
				RangeType:                         rangeType,
				NumDescriptors:                    1,
				BaseShaderRegister:                entry.Binding,
				RegisterSpace:                     uint32(groupIdx), // Use group index as register space
				OffsetInDescriptorsFromTableStart: 0xFFFFFFFF,       // D3D12_DESCRIPTOR_RANGE_OFFSET_APPEND
			}

			if isSampler {
				samplerRanges = append(samplerRanges, descRange)
			} else {
				cbvSrvUavRanges = append(cbvSrvUavRanges, descRange)
			}
		}

		// Create root parameter for CBV/SRV/UAV table
		if len(cbvSrvUavRanges) > 0 {
			descriptorRanges = append(descriptorRanges, cbvSrvUavRanges)
			rangeIdx := len(descriptorRanges) - 1

			mapping.cbvSrvUavIndex = len(rootParams)

			param := d3d12.D3D12_ROOT_PARAMETER{
				ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_DESCRIPTOR_TABLE,
				ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
			}

			// Set descriptor table in union
			table := (*d3d12.D3D12_ROOT_DESCRIPTOR_TABLE)(unsafe.Pointer(&param.Union[0]))
			table.NumDescriptorRanges = uint32(len(descriptorRanges[rangeIdx]))
			table.DescriptorRanges = &descriptorRanges[rangeIdx][0]

			rootParams = append(rootParams, param)
		}

		// Create root parameter for sampler table
		if len(samplerRanges) > 0 {
			descriptorRanges = append(descriptorRanges, samplerRanges)
			rangeIdx := len(descriptorRanges) - 1

			mapping.samplerIndex = len(rootParams)

			param := d3d12.D3D12_ROOT_PARAMETER{
				ParameterType:    d3d12.D3D12_ROOT_PARAMETER_TYPE_DESCRIPTOR_TABLE,
				ShaderVisibility: d3d12.D3D12_SHADER_VISIBILITY_ALL,
			}

			// Set descriptor table in union
			table := (*d3d12.D3D12_ROOT_DESCRIPTOR_TABLE)(unsafe.Pointer(&param.Union[0]))
			table.NumDescriptorRanges = uint32(len(descriptorRanges[rangeIdx]))
			table.DescriptorRanges = &descriptorRanges[rangeIdx][0]

			rootParams = append(rootParams, param)
		}

		groupMappings = append(groupMappings, mapping)
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
		return nil, nil, fmt.Errorf("dx12: failed to serialize root signature: %w", err)
	}
	defer blob.Release()

	// Check if device is already lost before attempting to create root signature.
	// Device removal can happen asynchronously (e.g. TDR from a prior GPU hang).
	if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
		return nil, nil, fmt.Errorf("dx12: device already removed before CreateRootSignature: %w", reason)
	}

	// Create root signature
	rootSig, err := d.raw.CreateRootSignature(0, blob.GetBufferPointer(), blob.GetBufferSize())
	if err != nil {
		// Include device removed reason for better diagnostics
		if reason := d.raw.GetDeviceRemovedReason(); reason != nil {
			return nil, nil, fmt.Errorf("dx12: failed to create root signature (device removed: %s): %w", reason.Error(), err)
		}
		return nil, nil, fmt.Errorf("dx12: failed to create root signature: %w", err)
	}

	return rootSig, groupMappings, nil
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
func (d *Device) buildGraphicsPipelineStateDesc(
	desc *hal.RenderPipelineDescriptor,
	inputElements []d3d12.D3D12_INPUT_ELEMENT_DESC,
	_ [][]byte, // Keep semantic name strings alive (prevents GC during PSO creation)
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
		shaderModule, ok := desc.Vertex.Module.(*ShaderModule)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid vertex shader module type")
		}
		bytecode := shaderModule.EntryPointBytecode(desc.Vertex.EntryPoint)
		if len(bytecode) > 0 {
			psoDesc.VS = d3d12.D3D12_SHADER_BYTECODE{
				ShaderBytecode: unsafe.Pointer(&bytecode[0]),
				BytecodeLength: uintptr(len(bytecode)),
			}
		} else {
			return nil, fmt.Errorf("dx12: vertex shader entry point %q not found in module", desc.Vertex.EntryPoint)
		}
	}

	// Set pixel (fragment) shader
	if desc.Fragment != nil && desc.Fragment.Module != nil {
		shaderModule, ok := desc.Fragment.Module.(*ShaderModule)
		if !ok {
			return nil, fmt.Errorf("dx12: invalid fragment shader module type")
		}
		bytecode := shaderModule.EntryPointBytecode(desc.Fragment.EntryPoint)
		if len(bytecode) > 0 {
			psoDesc.PS = d3d12.D3D12_SHADER_BYTECODE{
				ShaderBytecode: unsafe.Pointer(&bytecode[0]),
				BytecodeLength: uintptr(len(bytecode)),
			}
		} else {
			return nil, fmt.Errorf("dx12: fragment shader entry point %q not found in module", desc.Fragment.EntryPoint)
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

	// Blend state
	psoDesc.BlendState = d3d12.D3D12_BLEND_DESC{
		AlphaToCoverageEnable:  boolToInt32(desc.Multisample.AlphaToCoverageEnabled),
		IndependentBlendEnable: 0, // Will set to 1 if we have different blend states per target
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
	//nolint:prealloc // Size depends on nested attributes
	var elements []d3d12.D3D12_INPUT_ELEMENT_DESC
	//nolint:prealloc // Size depends on nested attributes
	var semanticNames [][]byte // Keep strings alive

	// Create semantic names like "TEXCOORD0", "TEXCOORD1", etc.
	// D3D12 uses semantic name + index for vertex attributes

	for slotIdx, buffer := range buffers {
		for _, attr := range buffer.Attributes {
			// Create semantic name "TEXCOORD" with semantic index = shader location
			semanticName := []byte("TEXCOORD\x00")
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
	pso           *d3d12.ID3D12PipelineState
	rootSignature *d3d12.ID3D12RootSignature // Reference, not owned
	groupMappings []rootParamMapping         // bind group → root param index mapping
	topology      d3d12.D3D_PRIMITIVE_TOPOLOGY
	vertexStrides []uint32 // Strides per vertex buffer slot
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
	pso           *d3d12.ID3D12PipelineState
	rootSignature *d3d12.ID3D12RootSignature // Reference, not owned
	groupMappings []rootParamMapping         // bind group → root param index mapping
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
