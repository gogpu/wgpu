// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
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

	// Dynamic state — all pipelines declare the same 4 dynamic states.
	// This matches Rust wgpu behavior and avoids Vulkan validation warnings
	// when switching between pipelines in a unified render pass (VK-PIPE-001).
	dynamicStates := []vk.DynamicState{
		vk.DynamicStateViewport,
		vk.DynamicStateScissor,
		vk.DynamicStateBlendConstants,
		vk.DynamicStateStencilReference,
	}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    &dynamicStates[0],
	}

	// Create compatible render pass for pipeline (not dynamic rendering).
	// This is required for Intel drivers that don't properly support VK_KHR_dynamic_rendering.
	var depthFormat vk.Format
	if desc.DepthStencil != nil {
		depthFormat = textureFormatToVk(desc.DepthStencil.Format)
	}

	// Build render pass key for pipeline-compatible render pass
	var colorFormat vk.Format
	if len(colorFormats) > 0 {
		colorFormat = colorFormats[0]
	}

	rpKey := RenderPassKey{
		ColorFormat:      colorFormat,
		ColorLoadOp:      vk.AttachmentLoadOpClear,
		ColorStoreOp:     vk.AttachmentStoreOpStore,
		SampleCount:      vk.SampleCountFlagBits(sampleCount),
		ColorFinalLayout: vk.ImageLayoutPresentSrcKhr,
		HasResolve:       sampleCount > 1, // MSAA pipelines need resolve attachment
	}
	if depthFormat != vk.FormatUndefined {
		rpKey.DepthFormat = depthFormat
		rpKey.DepthLoadOp = vk.AttachmentLoadOpClear
		rpKey.DepthStoreOp = vk.AttachmentStoreOpDontCare
		rpKey.StencilLoadOp = vk.AttachmentLoadOpDontCare
		rpKey.StencilStoreOp = vk.AttachmentStoreOpDontCare
	}

	// Get or create compatible render pass
	cache := d.GetRenderPassCache()
	compatibleRenderPass, err := cache.GetOrCreateRenderPass(rpKey)
	if err != nil {
		return nil, fmt.Errorf("vulkan: failed to create compatible render pass: %w", err)
	}

	// Create graphics pipeline with VkRenderPass (not dynamic rendering)
	createInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
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
		RenderPass:          compatibleRenderPass,
		Subpass:             0,
	}

	var pipeline vk.Pipeline
	result := vkCreateGraphicsPipelines(d.cmds, d.handle, 0, 1, &createInfo, nil, &pipeline)

	// Keep all data structures alive until after the Vulkan call completes.
	// This is critical because unsafe.Pointer→uintptr conversions break GC tracking.
	runtime.KeepAlive(stages)
	runtime.KeepAlive(shaderModules)
	runtime.KeepAlive(vertexEntryBytes)
	runtime.KeepAlive(fragmentEntryBytes)
	runtime.KeepAlive(vertexBindings)
	runtime.KeepAlive(vertexAttribs)
	runtime.KeepAlive(colorBlendAttachments)
	runtime.KeepAlive(dynamicStates)

	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateGraphicsPipelines failed: %d", result)
	}

	// Defensive check: Intel Vulkan drivers may return VK_SUCCESS but write VK_NULL_HANDLE.
	// This is a Vulkan spec violation, but we must handle it to prevent undefined behavior.
	// See: https://github.com/gogpu/wgpu/issues/24
	if pipeline == 0 {
		return nil, hal.ErrDriverBug
	}

	rp := &RenderPipeline{
		handle: pipeline,
		layout: pipelineLayout,
		device: d,
	}
	if desc.Label != "" {
		d.setObjectName(vk.ObjectTypePipeline, uint64(pipeline), desc.Label)
	} else {
		d.setObjectName(vk.ObjectTypePipeline, uint64(pipeline), "RenderPipeline")
	}
	return rp, nil
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

	// Defensive check: Intel Vulkan drivers may return VK_SUCCESS but write VK_NULL_HANDLE.
	// This is a Vulkan spec violation, but we must handle it to prevent undefined behavior.
	// See: https://github.com/gogpu/wgpu/issues/24
	if pipeline == 0 {
		return nil, hal.ErrDriverBug
	}

	cp := &ComputePipeline{
		handle: pipeline,
		layout: pipelineLayout,
		device: d,
	}
	if desc.Label != "" {
		d.setObjectName(vk.ObjectTypePipeline, uint64(pipeline), desc.Label)
	} else {
		d.setObjectName(vk.ObjectTypePipeline, uint64(pipeline), "ComputePipeline")
	}
	return cp, nil
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

//nolint:unused // Reserved for depth-stencil pipeline support
func hasStencilComponent(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8:
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

func vkCreateGraphicsPipelines(cmds *vk.Commands, device vk.Device, cache vk.PipelineCache, count uint32, createInfo *vk.GraphicsPipelineCreateInfo, _ unsafe.Pointer, pipeline *vk.Pipeline) vk.Result {
	return cmds.CreateGraphicsPipelines(device, cache, count, createInfo, nil, pipeline)
}

func vkCreateComputePipelines(cmds *vk.Commands, device vk.Device, cache vk.PipelineCache, count uint32, createInfo *vk.ComputePipelineCreateInfo, _ unsafe.Pointer, pipeline *vk.Pipeline) vk.Result {
	return cmds.CreateComputePipelines(device, cache, count, createInfo, nil, pipeline)
}

func vkDestroyPipeline(cmds *vk.Commands, device vk.Device, pipeline vk.Pipeline, _ unsafe.Pointer) {
	cmds.DestroyPipeline(device, pipeline, nil)
}
