// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package vulkan

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

const defaultEntryPoint = "main"

// CreateRenderPipeline creates a render pipeline.
//
//nolint:maintidx // Pipeline creation is inherently complex due to all the state it configures.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: render pipeline descriptor is nil")
	}

	// Get pipeline layout
	var pipelineLayout vk.PipelineLayout
	if desc.Layout != nil {
		vkLayout, ok := desc.Layout.(*PipelineLayout)
		if !ok || vkLayout == nil {
			return nil, fmt.Errorf("vulkan: invalid pipeline layout")
		}
		pipelineLayout = vkLayout.handle
	}

	// Build shader stages
	var stages []vk.PipelineShaderStageCreateInfo
	var shaderModules []vk.ShaderModule // Keep references to prevent GC

	// Vertex shader (required)
	if desc.Vertex.Module == nil {
		return nil, fmt.Errorf("vulkan: vertex shader module is required")
	}
	vertexModule, ok := desc.Vertex.Module.(*ShaderModule)
	if !ok || vertexModule == nil {
		return nil, fmt.Errorf("vulkan: invalid vertex shader module")
	}
	shaderModules = append(shaderModules, vertexModule.handle)

	entryPointVertex := desc.Vertex.EntryPoint
	if entryPointVertex == "" {
		entryPointVertex = defaultEntryPoint
	}
	vertexEntryBytes := append([]byte(entryPointVertex), 0)

	stages = append(stages, vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageVertexBit,
		Module: vertexModule.handle,
		PName:  uintptr(unsafe.Pointer(&vertexEntryBytes[0])),
	})

	// Fragment shader (optional)
	var fragmentEntryBytes []byte
	if desc.Fragment != nil && desc.Fragment.Module != nil {
		fragmentModule, ok := desc.Fragment.Module.(*ShaderModule)
		if !ok || fragmentModule == nil {
			return nil, fmt.Errorf("vulkan: invalid fragment shader module")
		}
		shaderModules = append(shaderModules, fragmentModule.handle)

		entryPointFragment := desc.Fragment.EntryPoint
		if entryPointFragment == "" {
			entryPointFragment = defaultEntryPoint
		}
		fragmentEntryBytes = append([]byte(entryPointFragment), 0)

		stages = append(stages, vk.PipelineShaderStageCreateInfo{
			SType:  vk.StructureTypePipelineShaderStageCreateInfo,
			Stage:  vk.ShaderStageFragmentBit,
			Module: fragmentModule.handle,
			PName:  uintptr(unsafe.Pointer(&fragmentEntryBytes[0])),
		})
	}

	// Vertex input state - pre-allocate for performance
	totalAttribs := 0
	for _, buf := range desc.Vertex.Buffers {
		totalAttribs += len(buf.Attributes)
	}
	vertexBindings := make([]vk.VertexInputBindingDescription, 0, len(desc.Vertex.Buffers))
	vertexAttribs := make([]vk.VertexInputAttributeDescription, 0, totalAttribs)

	for i, buf := range desc.Vertex.Buffers {
		vertexBindings = append(vertexBindings, vk.VertexInputBindingDescription{
			Binding:   uint32(i),
			Stride:    uint32(buf.ArrayStride),
			InputRate: vertexStepModeToVk(buf.StepMode),
		})

		for _, attr := range buf.Attributes {
			vertexAttribs = append(vertexAttribs, vk.VertexInputAttributeDescription{
				Location: attr.ShaderLocation,
				Binding:  uint32(i),
				Format:   vertexFormatToVk(attr.Format),
				Offset:   uint32(attr.Offset),
			})
		}
	}

	vertexInputState := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   uint32(len(vertexBindings)),
		VertexAttributeDescriptionCount: uint32(len(vertexAttribs)),
	}
	if len(vertexBindings) > 0 {
		vertexInputState.PVertexBindingDescriptions = &vertexBindings[0]
	}
	if len(vertexAttribs) > 0 {
		vertexInputState.PVertexAttributeDescriptions = &vertexAttribs[0]
	}

	// Input assembly state
	inputAssemblyState := vk.PipelineInputAssemblyStateCreateInfo{
		SType:                  vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               primitiveTopologyToVk(desc.Primitive.Topology),
		PrimitiveRestartEnable: vk.Bool32(vk.False),
	}
	// Primitive restart is enabled for strip topologies with explicit index format
	if desc.Primitive.StripIndexFormat != nil {
		inputAssemblyState.PrimitiveRestartEnable = vk.Bool32(vk.True)
	}

	// Viewport state (dynamic)
	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		ScissorCount:  1,
	}

	// Rasterization state
	rasterizationState := vk.PipelineRasterizationStateCreateInfo{
		SType:                   vk.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        boolToVk(desc.Primitive.UnclippedDepth),
		RasterizerDiscardEnable: vk.Bool32(vk.False),
		PolygonMode:             vk.PolygonModeFill,
		CullMode:                cullModeToVk(desc.Primitive.CullMode),
		FrontFace:               frontFaceToVk(desc.Primitive.FrontFace),
		DepthBiasEnable:         vk.Bool32(vk.False),
		LineWidth:               1.0,
	}

	// Multisample state
	sampleCount := uint32(1)
	if desc.Multisample.Count > 0 {
		sampleCount = desc.Multisample.Count
	}
	multisampleState := vk.PipelineMultisampleStateCreateInfo{
		SType:                vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples: vk.SampleCountFlagBits(sampleCount),
		SampleShadingEnable:  vk.Bool32(vk.False),
		MinSampleShading:     1.0,
	}
	if desc.Multisample.Mask != 0 {
		mask := vk.SampleMask(desc.Multisample.Mask)
		multisampleState.PSampleMask = &mask
	}
	if desc.Multisample.AlphaToCoverageEnabled {
		multisampleState.AlphaToCoverageEnable = vk.Bool32(vk.True)
	}

	// Depth stencil state
	var depthStencilState *vk.PipelineDepthStencilStateCreateInfo
	if desc.DepthStencil != nil {
		depthStencilState = &vk.PipelineDepthStencilStateCreateInfo{
			SType:                 vk.StructureTypePipelineDepthStencilStateCreateInfo,
			DepthTestEnable:       vk.Bool32(vk.True),
			DepthWriteEnable:      boolToVk(desc.DepthStencil.DepthWriteEnabled),
			DepthCompareOp:        compareFunctionToVk(desc.DepthStencil.DepthCompare),
			DepthBoundsTestEnable: vk.Bool32(vk.False),
			StencilTestEnable:     vk.Bool32(vk.False),
		}

		// Enable depth bias if any values are non-zero
		if desc.DepthStencil.DepthBias != 0 || desc.DepthStencil.DepthBiasSlopeScale != 0 {
			rasterizationState.DepthBiasEnable = vk.Bool32(vk.True)
			rasterizationState.DepthBiasConstantFactor = float32(desc.DepthStencil.DepthBias)
			rasterizationState.DepthBiasSlopeFactor = desc.DepthStencil.DepthBiasSlopeScale
			rasterizationState.DepthBiasClamp = desc.DepthStencil.DepthBiasClamp
		}

		// Stencil operations
		if desc.DepthStencil.StencilReadMask != 0 || desc.DepthStencil.StencilWriteMask != 0 {
			depthStencilState.StencilTestEnable = vk.Bool32(vk.True)
			depthStencilState.Front = stencilFaceStateToVk(desc.DepthStencil.StencilFront)
			depthStencilState.Front.CompareMask = desc.DepthStencil.StencilReadMask
			depthStencilState.Front.WriteMask = desc.DepthStencil.StencilWriteMask
			depthStencilState.Back = stencilFaceStateToVk(desc.DepthStencil.StencilBack)
			depthStencilState.Back.CompareMask = desc.DepthStencil.StencilReadMask
			depthStencilState.Back.WriteMask = desc.DepthStencil.StencilWriteMask
		}
	}

	// Color blend state
	var colorBlendAttachments []vk.PipelineColorBlendAttachmentState
	var colorFormats []vk.Format

	if desc.Fragment != nil {
		for _, target := range desc.Fragment.Targets {
			colorFormats = append(colorFormats, textureFormatToVk(target.Format))

			attachment := vk.PipelineColorBlendAttachmentState{
				ColorWriteMask: colorWriteMaskToVk(target.WriteMask),
				BlendEnable:    vk.Bool32(vk.False),
			}

			if target.Blend != nil {
				attachment.BlendEnable = vk.Bool32(vk.True)
				attachment.SrcColorBlendFactor = blendFactorToVk(target.Blend.Color.SrcFactor)
				attachment.DstColorBlendFactor = blendFactorToVk(target.Blend.Color.DstFactor)
				attachment.ColorBlendOp = blendOperationToVk(target.Blend.Color.Operation)
				attachment.SrcAlphaBlendFactor = blendFactorToVk(target.Blend.Alpha.SrcFactor)
				attachment.DstAlphaBlendFactor = blendFactorToVk(target.Blend.Alpha.DstFactor)
				attachment.AlphaBlendOp = blendOperationToVk(target.Blend.Alpha.Operation)
			}

			colorBlendAttachments = append(colorBlendAttachments, attachment)
		}
	}

	colorBlendState := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vk.Bool32(vk.False),
		AttachmentCount: uint32(len(colorBlendAttachments)),
	}
	if len(colorBlendAttachments) > 0 {
		colorBlendState.PAttachments = &colorBlendAttachments[0]
	}

	// Dynamic state (viewport and scissor are always dynamic)
	dynamicStates := []vk.DynamicState{
		vk.DynamicStateViewport,
		vk.DynamicStateScissor,
	}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    &dynamicStates[0],
	}

	// Build pipeline rendering info for dynamic rendering (Vulkan 1.3+)
	var depthFormat vk.Format
	var stencilFormat vk.Format
	if desc.DepthStencil != nil {
		depthFormat = textureFormatToVk(desc.DepthStencil.Format)
		// Check if format has stencil
		if hasStencilComponent(desc.DepthStencil.Format) {
			stencilFormat = depthFormat
		}
	}

	renderingInfo := vk.PipelineRenderingCreateInfo{
		SType:                   vk.StructureTypePipelineRenderingCreateInfo,
		ColorAttachmentCount:    uint32(len(colorFormats)),
		DepthAttachmentFormat:   depthFormat,
		StencilAttachmentFormat: stencilFormat,
	}
	if len(colorFormats) > 0 {
		renderingInfo.PColorAttachmentFormats = &colorFormats[0]
	}

	// Create graphics pipeline
	createInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		PNext:               (*uintptr)(unsafe.Pointer(&renderingInfo)),
		StageCount:          uint32(len(stages)),
		PStages:             &stages[0],
		PVertexInputState:   &vertexInputState,
		PInputAssemblyState: &inputAssemblyState,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizationState,
		PMultisampleState:   &multisampleState,
		PDepthStencilState:  depthStencilState,
		PColorBlendState:    &colorBlendState,
		PDynamicState:       &dynamicState,
		Layout:              pipelineLayout,
		RenderPass:          0, // Dynamic rendering, no render pass
		Subpass:             0,
	}

	var pipeline vk.Pipeline
	result := vkCreateGraphicsPipelines(d.cmds, d.handle, 0, 1, &createInfo, nil, &pipeline)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateGraphicsPipelines failed: %d", result)
	}

	// Keep shader entry point bytes alive for debug (optional, modules are kept)
	_ = shaderModules
	_ = vertexEntryBytes
	_ = fragmentEntryBytes

	return &RenderPipeline{
		handle: pipeline,
		layout: pipelineLayout,
		device: d,
	}, nil
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	vkPipeline, ok := pipeline.(*RenderPipeline)
	if !ok || vkPipeline == nil {
		return
	}

	if vkPipeline.handle != 0 {
		vkDestroyPipeline(d.cmds, d.handle, vkPipeline.handle, nil)
		vkPipeline.handle = 0
	}

	vkPipeline.device = nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: compute pipeline descriptor is nil")
	}

	// Get pipeline layout
	var pipelineLayout vk.PipelineLayout
	if desc.Layout != nil {
		vkLayout, ok := desc.Layout.(*PipelineLayout)
		if !ok || vkLayout == nil {
			return nil, fmt.Errorf("vulkan: invalid pipeline layout")
		}
		pipelineLayout = vkLayout.handle
	}

	// Get compute shader module
	if desc.Compute.Module == nil {
		return nil, fmt.Errorf("vulkan: compute shader module is required")
	}
	computeModule, ok := desc.Compute.Module.(*ShaderModule)
	if !ok || computeModule == nil {
		return nil, fmt.Errorf("vulkan: invalid compute shader module")
	}

	entryPoint := desc.Compute.EntryPoint
	if entryPoint == "" {
		entryPoint = defaultEntryPoint
	}
	entryPointBytes := append([]byte(entryPoint), 0)

	stage := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageComputeBit,
		Module: computeModule.handle,
		PName:  uintptr(unsafe.Pointer(&entryPointBytes[0])),
	}

	createInfo := vk.ComputePipelineCreateInfo{
		SType:  vk.StructureTypeComputePipelineCreateInfo,
		Stage:  stage,
		Layout: pipelineLayout,
	}

	var pipeline vk.Pipeline
	result := vkCreateComputePipelines(d.cmds, d.handle, 0, 1, &createInfo, nil, &pipeline)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateComputePipelines failed: %d", result)
	}

	return &ComputePipeline{
		handle: pipeline,
		layout: pipelineLayout,
		device: d,
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	vkPipeline, ok := pipeline.(*ComputePipeline)
	if !ok || vkPipeline == nil {
		return
	}

	if vkPipeline.handle != 0 {
		vkDestroyPipeline(d.cmds, d.handle, vkPipeline.handle, nil)
		vkPipeline.handle = 0
	}

	vkPipeline.device = nil
}

// Helper functions

func hasStencilComponent(format types.TextureFormat) bool {
	switch format {
	case types.TextureFormatDepth24PlusStencil8,
		types.TextureFormatDepth32FloatStencil8,
		types.TextureFormatStencil8:
		return true
	default:
		return false
	}
}

func boolToVk(b bool) vk.Bool32 {
	if b {
		return vk.Bool32(vk.True)
	}
	return vk.Bool32(vk.False)
}

// Vulkan function wrappers

func vkCreateGraphicsPipelines(cmds *vk.Commands, device vk.Device, cache vk.PipelineCache, count uint32, createInfo *vk.GraphicsPipelineCreateInfo, allocator unsafe.Pointer, pipeline *vk.Pipeline) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.CreateGraphicsPipelines(),
		uintptr(device),
		uintptr(cache),
		uintptr(count),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(pipeline)))
	return vk.Result(ret)
}

func vkCreateComputePipelines(cmds *vk.Commands, device vk.Device, cache vk.PipelineCache, count uint32, createInfo *vk.ComputePipelineCreateInfo, allocator unsafe.Pointer, pipeline *vk.Pipeline) vk.Result {
	ret, _, _ := syscall.SyscallN(cmds.CreateComputePipelines(),
		uintptr(device),
		uintptr(cache),
		uintptr(count),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(pipeline)))
	return vk.Result(ret)
}

func vkDestroyPipeline(cmds *vk.Commands, device vk.Device, pipeline vk.Pipeline, allocator unsafe.Pointer) {
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(cmds.DestroyPipeline(),
		uintptr(device),
		uintptr(pipeline),
		uintptr(allocator))
}
