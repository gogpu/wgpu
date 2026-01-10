// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command vulkan-renderpass-test validates the hypothesis that Intel Iris Xe
// driver bug is specific to VK_KHR_dynamic_rendering, not vkCreateGraphicsPipelines.
//
// This test creates a pipeline using traditional VkRenderPass instead of
// dynamic rendering to verify if the Intel driver works correctly with the
// traditional approach.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"

	"github.com/gogpu/naga"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n=== SUCCESS: VkRenderPass pipeline created! ===")
	fmt.Println("This confirms Intel bug is in VK_KHR_dynamic_rendering, not vkCreateGraphicsPipelines.")
}

// makeVersion creates Vulkan version number (same as VK_MAKE_API_VERSION).
func makeVersion(major, minor, patch uint32) uint32 {
	return (major << 22) | (minor << 12) | patch
}

func run() error {
	fmt.Println("=== Intel Dynamic Rendering Bug Verification ===")
	fmt.Println("Testing: VkRenderPass (traditional) vs VK_KHR_dynamic_rendering")
	fmt.Println()

	// Initialize Vulkan
	fmt.Print("1. Initializing Vulkan... ")
	if err := vk.Init(); err != nil {
		return fmt.Errorf("vk.Init: %w", err)
	}
	fmt.Println("OK")

	// Load global commands
	cmds := &vk.Commands{}
	fmt.Print("2. Loading global commands... ")
	if err := cmds.LoadGlobal(); err != nil {
		return fmt.Errorf("LoadGlobal: %w", err)
	}
	fmt.Println("OK")

	// Create instance (no validation layers)
	fmt.Print("3. Creating instance... ")
	appInfo := vk.ApplicationInfo{
		SType:            vk.StructureTypeApplicationInfo,
		PApplicationName: uintptr(unsafe.Pointer(&[]byte("RenderPassTest\x00")[0])),
		ApiVersion:       makeVersion(1, 0, 0), // Vulkan 1.0 for compatibility
	}

	extensions := []string{"VK_KHR_surface\x00", "VK_KHR_win32_surface\x00"}
	extensionPtrs := make([]uintptr, len(extensions))
	for i, ext := range extensions {
		extensionPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(ext)))
	}

	createInfo := vk.InstanceCreateInfo{
		SType:                 vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo:      &appInfo,
		EnabledExtensionCount: uint32(len(extensions)),
	}
	if len(extensionPtrs) > 0 {
		createInfo.PpEnabledExtensionNames = uintptr(unsafe.Pointer(&extensionPtrs[0]))
	}

	var instance vk.Instance
	if result := cmds.CreateInstance(&createInfo, nil, &instance); result != vk.Success {
		return fmt.Errorf("vkCreateInstance failed: %d", result)
	}
	defer cmds.DestroyInstance(instance, nil)
	fmt.Println("OK")

	// Load instance commands
	fmt.Print("4. Loading instance commands... ")
	if err := cmds.LoadInstance(instance); err != nil {
		return fmt.Errorf("LoadInstance: %w", err)
	}
	vk.SetDeviceProcAddr(instance)
	fmt.Println("OK")

	// Enumerate physical devices
	fmt.Print("5. Enumerating physical devices... ")
	var deviceCount uint32
	cmds.EnumeratePhysicalDevices(instance, &deviceCount, nil)
	if deviceCount == 0 {
		return fmt.Errorf("no physical devices")
	}
	devices := make([]vk.PhysicalDevice, deviceCount)
	cmds.EnumeratePhysicalDevices(instance, &deviceCount, &devices[0])

	physicalDevice := devices[0]
	var props vk.PhysicalDeviceProperties
	cmds.GetPhysicalDeviceProperties(physicalDevice, &props)
	deviceName := cStringToGo(props.DeviceName[:])
	fmt.Printf("OK (%s)\n", deviceName)

	// Get queue family
	fmt.Print("6. Finding graphics queue family... ")
	var queueFamilyCount uint32
	cmds.GetPhysicalDeviceQueueFamilyProperties(physicalDevice, &queueFamilyCount, nil)
	queueFamilies := make([]vk.QueueFamilyProperties, queueFamilyCount)
	cmds.GetPhysicalDeviceQueueFamilyProperties(physicalDevice, &queueFamilyCount, &queueFamilies[0])

	graphicsFamily := uint32(0xFFFFFFFF)
	for i, qf := range queueFamilies {
		if qf.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
			graphicsFamily = uint32(i)
			break
		}
	}
	if graphicsFamily == 0xFFFFFFFF {
		return fmt.Errorf("no graphics queue family")
	}
	fmt.Printf("OK (family %d)\n", graphicsFamily)

	// Create logical device
	fmt.Print("7. Creating logical device... ")
	queuePriority := float32(1.0)
	queueCreateInfo := vk.DeviceQueueCreateInfo{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: graphicsFamily,
		QueueCount:       1,
		PQueuePriorities: &queuePriority,
	}

	swapchainExt := "VK_KHR_swapchain\x00"
	deviceExtensions := []uintptr{uintptr(unsafe.Pointer(unsafe.StringData(swapchainExt)))}

	deviceCreateInfo := vk.DeviceCreateInfo{
		SType:                   vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:    1,
		PQueueCreateInfos:       &queueCreateInfo,
		EnabledExtensionCount:   uint32(len(deviceExtensions)),
		PpEnabledExtensionNames: uintptr(unsafe.Pointer(&deviceExtensions[0])),
	}

	var device vk.Device
	if result := cmds.CreateDevice(physicalDevice, &deviceCreateInfo, nil, &device); result != vk.Success {
		return fmt.Errorf("vkCreateDevice failed: %d", result)
	}
	defer cmds.DestroyDevice(device, nil)
	fmt.Println("OK")

	// Load device commands
	fmt.Print("8. Loading device commands... ")
	if err := cmds.LoadDevice(device); err != nil {
		return fmt.Errorf("LoadDevice: %w", err)
	}
	fmt.Println("OK")

	// Debug: check function pointers
	createPipeFn := cmds.DebugFunctionPointer("vkCreateGraphicsPipelines")
	createRPFn := cmds.DebugFunctionPointer("vkCreateRenderPass")
	fmt.Printf("    vkCreateGraphicsPipelines: %p\n", createPipeFn)
	fmt.Printf("    vkCreateRenderPass: %p\n", createRPFn)
	if createPipeFn == nil {
		return fmt.Errorf("vkCreateGraphicsPipelines function pointer is NULL!")
	}

	// Create VkRenderPass (traditional approach)
	fmt.Print("9. Creating VkRenderPass (traditional)... ")
	colorAttachment := vk.AttachmentDescription{
		Format:         vk.FormatB8g8r8a8Unorm,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutPresentSrcKhr,
	}

	colorAttachmentRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}

	subpass := vk.SubpassDescription{
		PipelineBindPoint:    vk.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    &colorAttachmentRef,
	}

	renderPassCreateInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments:    &colorAttachment,
		SubpassCount:    1,
		PSubpasses:      &subpass,
	}

	var renderPass vk.RenderPass
	if result := cmds.CreateRenderPass(device, &renderPassCreateInfo, nil, &renderPass); result != vk.Success {
		return fmt.Errorf("vkCreateRenderPass failed: %d", result)
	}
	defer cmds.DestroyRenderPass(device, renderPass, nil)
	fmt.Println("OK")

	// Create shader modules using naga-compiled SPIR-V
	fmt.Println("10. Compiling shaders with naga...")
	nagaVertSPIRV, err := compileWGSLToSPIRV(vertexWGSL)
	if err != nil {
		fmt.Printf("    naga vertex shader failed: %v, falling back to hardcoded\n", err)
		nagaVertSPIRV = vertexShaderSPIRV
	} else {
		fmt.Printf("    naga vertex: %d words\n", len(nagaVertSPIRV))
	}

	nagaFragSPIRV, err := compileWGSLToSPIRV(fragmentWGSL)
	if err != nil {
		fmt.Printf("    naga fragment shader failed: %v, falling back to hardcoded\n", err)
		nagaFragSPIRV = fragmentShaderSPIRV
	} else {
		fmt.Printf("    naga fragment: %d words\n", len(nagaFragSPIRV))
	}

	fmt.Print("    Creating shader modules... ")
	vertShader, err := createShaderModule(cmds, device, nagaVertSPIRV)
	if err != nil {
		return fmt.Errorf("vertex shader: %w", err)
	}
	defer cmds.DestroyShaderModule(device, vertShader, nil)

	fragShader, err := createShaderModule(cmds, device, nagaFragSPIRV)
	if err != nil {
		return fmt.Errorf("fragment shader: %w", err)
	}
	defer cmds.DestroyShaderModule(device, fragShader, nil)
	fmt.Printf("OK\n    vertShader: 0x%X, fragShader: 0x%X\n", vertShader, fragShader)

	// Create pipeline layout
	fmt.Print("11. Creating pipeline layout... ")
	layoutCreateInfo := vk.PipelineLayoutCreateInfo{
		SType: vk.StructureTypePipelineLayoutCreateInfo,
	}
	var pipelineLayout vk.PipelineLayout
	if result := cmds.CreatePipelineLayout(device, &layoutCreateInfo, nil, &pipelineLayout); result != vk.Success {
		return fmt.Errorf("vkCreatePipelineLayout failed: %d", result)
	}
	defer cmds.DestroyPipelineLayout(device, pipelineLayout, nil)
	fmt.Println("OK")

	// Create graphics pipeline with VkRenderPass (NOT dynamic rendering)
	fmt.Print("12. Creating graphics pipeline (VkRenderPass)... \n")

	// Debug: print struct sizes
	fmt.Printf("    sizeof(GraphicsPipelineCreateInfo) = %d\n", unsafe.Sizeof(vk.GraphicsPipelineCreateInfo{}))
	fmt.Printf("    sizeof(PipelineShaderStageCreateInfo) = %d\n", unsafe.Sizeof(vk.PipelineShaderStageCreateInfo{}))
	fmt.Printf("    sizeof(PipelineVertexInputStateCreateInfo) = %d\n", unsafe.Sizeof(vk.PipelineVertexInputStateCreateInfo{}))

	mainEntry := []byte("main\x00")

	stages := []vk.PipelineShaderStageCreateInfo{
		{
			SType:  vk.StructureTypePipelineShaderStageCreateInfo,
			Stage:  vk.ShaderStageVertexBit,
			Module: vertShader,
			PName:  uintptr(unsafe.Pointer(&mainEntry[0])),
		},
		{
			SType:  vk.StructureTypePipelineShaderStageCreateInfo,
			Stage:  vk.ShaderStageFragmentBit,
			Module: fragShader,
			PName:  uintptr(unsafe.Pointer(&mainEntry[0])),
		},
	}

	vertexInputState := vk.PipelineVertexInputStateCreateInfo{
		SType: vk.StructureTypePipelineVertexInputStateCreateInfo,
	}

	inputAssemblyState := vk.PipelineInputAssemblyStateCreateInfo{
		SType:    vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology: vk.PrimitiveTopologyTriangleList,
	}

	// Use dynamic viewport/scissor (matching HAL behavior)
	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		ScissorCount:  1,
	}

	// Dynamic state for viewport and scissor
	dynamicStates := []vk.DynamicState{
		vk.DynamicStateViewport,
		vk.DynamicStateScissor,
	}
	dynamicState := vk.PipelineDynamicStateCreateInfo{
		SType:             vk.StructureTypePipelineDynamicStateCreateInfo,
		DynamicStateCount: uint32(len(dynamicStates)),
		PDynamicStates:    &dynamicStates[0],
	}

	rasterizationState := vk.PipelineRasterizationStateCreateInfo{
		SType:       vk.StructureTypePipelineRasterizationStateCreateInfo,
		PolygonMode: vk.PolygonModeFill,
		CullMode:    vk.CullModeFlags(vk.CullModeNone),
		FrontFace:   vk.FrontFaceCounterClockwise,
		LineWidth:   1.0,
	}

	multisampleState := vk.PipelineMultisampleStateCreateInfo{
		SType:                vk.StructureTypePipelineMultisampleStateCreateInfo,
		RasterizationSamples: vk.SampleCount1Bit,
	}

	colorBlendAttachment := vk.PipelineColorBlendAttachmentState{
		ColorWriteMask: vk.ColorComponentFlags(vk.ColorComponentRBit | vk.ColorComponentGBit | vk.ColorComponentBBit | vk.ColorComponentABit),
		BlendEnable:    vk.Bool32(vk.False),
	}

	colorBlendState := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		AttachmentCount: 1,
		PAttachments:    &colorBlendAttachment,
	}

	pipelineCreateInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          uint32(len(stages)),
		PStages:             &stages[0],
		PVertexInputState:   &vertexInputState,
		PInputAssemblyState: &inputAssemblyState,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizationState,
		PMultisampleState:   &multisampleState,
		PColorBlendState:    &colorBlendState,
		PDynamicState:       &dynamicState,
		Layout:              pipelineLayout,
		RenderPass:          renderPass, // <-- TRADITIONAL: Using VkRenderPass!
		Subpass:             0,
	}

	var pipeline vk.Pipeline
	fmt.Printf("    createInfo address: %p\n", &pipelineCreateInfo)
	fmt.Printf("    createInfo.SType: %d (expected %d)\n", pipelineCreateInfo.SType, vk.StructureTypeGraphicsPipelineCreateInfo)
	fmt.Printf("    createInfo.RenderPass: 0x%X\n", pipelineCreateInfo.RenderPass)
	fmt.Printf("    createInfo.Layout: 0x%X\n", pipelineCreateInfo.Layout)
	fmt.Printf("    createInfo.StageCount: %d\n", pipelineCreateInfo.StageCount)

	// Try syscall directly (bypassing goffi entirely)
	fmt.Println("    Trying syscall directly...")
	syscallPipeline, syscallResult, syscallErr := SyscallCreatePipeline(cmds, device, &pipelineCreateInfo)
	if syscallErr != nil {
		return fmt.Errorf("syscall error: %w", syscallErr)
	}
	fmt.Printf("    Syscall result: %d, pipeline: 0x%X\n", syscallResult, syscallPipeline)

	// Try direct FFI call
	fmt.Println("    Trying direct FFI call...")
	directPipeline, directResult, directErr := DirectCreatePipeline(cmds, device, &pipelineCreateInfo)
	if directErr != nil {
		return fmt.Errorf("direct FFI call error: %w", directErr)
	}
	fmt.Printf("    Direct FFI result: %d, pipeline: 0x%X\n", directResult, directPipeline)

	// Now try wrapper call
	fmt.Println("    Trying wrapper call...")
	fmt.Printf("    pipeline address before call: %p, value: 0x%X\n", &pipeline, pipeline)
	result := cmds.CreateGraphicsPipelines(device, 0, 1, &pipelineCreateInfo, nil, &pipeline)
	fmt.Printf("    result: %d, pipeline value after call: 0x%X\n", result, pipeline)
	if result != vk.Success {
		return fmt.Errorf("vkCreateGraphicsPipelines failed: %d", result)
	}

	// Check for Intel null pipeline bug
	if pipeline == 0 {
		return fmt.Errorf("Intel bug: VK_SUCCESS but pipeline == VK_NULL_HANDLE")
	}

	cmds.DestroyPipeline(device, pipeline, nil)
	fmt.Println("OK")

	fmt.Printf("\nPipeline handle: 0x%X (valid!)\n", pipeline)

	return nil
}

func createShaderModule(cmds *vk.Commands, device vk.Device, code []uint32) (vk.ShaderModule, error) {
	createInfo := vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(code) * 4),
		PCode:    &code[0],
	}

	var module vk.ShaderModule
	if result := cmds.CreateShaderModule(device, &createInfo, nil, &module); result != vk.Success {
		return 0, fmt.Errorf("vkCreateShaderModule failed: %d", result)
	}
	return module, nil
}

func cStringToGo(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// compileWGSLToSPIRV compiles WGSL shader to SPIR-V using naga.
func compileWGSLToSPIRV(wgslSource string) ([]uint32, error) {
	spirvBytes, err := naga.Compile(wgslSource)
	if err != nil {
		return nil, fmt.Errorf("naga compile failed: %w", err)
	}

	// Convert bytes to uint32 slice
	if len(spirvBytes)%4 != 0 {
		return nil, fmt.Errorf("SPIR-V byte count not multiple of 4")
	}
	spirvU32 := make([]uint32, len(spirvBytes)/4)
	for i := range spirvU32 {
		spirvU32[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
	}
	return spirvU32, nil
}

// WGSL shader sources
const vertexWGSL = `
@vertex
fn main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    var positions = array<vec2<f32>, 3>(
        vec2<f32>(0.0, -0.5),
        vec2<f32>(0.5, 0.5),
        vec2<f32>(-0.5, 0.5)
    );
    return vec4<f32>(positions[idx], 0.0, 1.0);
}
`

const fragmentWGSL = `
@fragment
fn main() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`

// Same SPIR-V shaders as vulkan-triangle test (fallback)
var vertexShaderSPIRV = []uint32{
	0x07230203, 0x00010000, 0x0008000a, 0x00000030, 0x00000000, 0x00020011, 0x00000001, 0x0006000b,
	0x00000001, 0x4c534c47, 0x6474732e, 0x3035342e, 0x00000000, 0x0003000e, 0x00000000, 0x00000001,
	0x0007000f, 0x00000000, 0x00000004, 0x6e69616d, 0x00000000, 0x0000000a, 0x0000000e, 0x00030003,
	0x00000002, 0x000001c2, 0x00040005, 0x00000004, 0x6e69616d, 0x00000000, 0x00060005, 0x00000008,
	0x73696f50, 0x6f697469, 0x0000736e, 0x00050005, 0x0000000a, 0x495f6c67, 0x7865646e, 0x00000000,
	0x00060005, 0x0000000c, 0x505f6c67, 0x65567265, 0x78657472, 0x00000000, 0x00060006, 0x0000000c,
	0x00000000, 0x505f6c67, 0x7469736f, 0x006e6f69, 0x00070006, 0x0000000c, 0x00000001, 0x505f6c67,
	0x746e696f, 0x657a6953, 0x00000000, 0x00070006, 0x0000000c, 0x00000002, 0x435f6c67, 0x4470696c,
	0x61747369, 0x0065636e, 0x00070006, 0x0000000c, 0x00000003, 0x435f6c67, 0x446c6c75, 0x61747369,
	0x0065636e, 0x00030005, 0x0000000e, 0x00000000, 0x00040047, 0x0000000a, 0x0000000b, 0x0000002a,
	0x00050048, 0x0000000c, 0x00000000, 0x0000000b, 0x00000000, 0x00050048, 0x0000000c, 0x00000001,
	0x0000000b, 0x00000001, 0x00050048, 0x0000000c, 0x00000002, 0x0000000b, 0x00000003, 0x00050048,
	0x0000000c, 0x00000003, 0x0000000b, 0x00000004, 0x00030047, 0x0000000c, 0x00000002, 0x00020013,
	0x00000002, 0x00030021, 0x00000003, 0x00000002, 0x00030016, 0x00000006, 0x00000020, 0x00040017,
	0x00000007, 0x00000006, 0x00000002, 0x00040020, 0x00000008, 0x00000007, 0x00000007, 0x00040015,
	0x00000009, 0x00000020, 0x00000001, 0x00040020, 0x0000000a, 0x00000001, 0x00000009, 0x00040017,
	0x0000000b, 0x00000006, 0x00000004, 0x00040015, 0x0000000d, 0x00000020, 0x00000000, 0x0004001c,
	0x0000000e, 0x00000006, 0x0000000d, 0x0006001e, 0x0000000c, 0x0000000b, 0x00000006, 0x0000000e,
	0x0000000e, 0x00040020, 0x0000000f, 0x00000003, 0x0000000c, 0x0004003b, 0x0000000f, 0x0000000e,
	0x00000003, 0x00040043, 0x00000009, 0x00000010, 0x00000000, 0x0004002b, 0x00000006, 0x00000011,
	0x00000000, 0x0004002b, 0x00000006, 0x00000012, 0xbf000000, 0x0005002c, 0x00000007, 0x00000013,
	0x00000011, 0x00000012, 0x0004002b, 0x00000006, 0x00000014, 0x3f000000, 0x0005002c, 0x00000007,
	0x00000015, 0x00000014, 0x00000014, 0x0005002c, 0x00000007, 0x00000016, 0xbf000000, 0x00000014,
	0x0006002c, 0x00000017, 0x00000018, 0x00000013, 0x00000015, 0x00000016, 0x00040020, 0x00000019,
	0x0000000b, 0x00000017, 0x00040020, 0x0000001a, 0x00000003, 0x0000000b, 0x0004002b, 0x00000006,
	0x0000001b, 0x3f800000, 0x00050036, 0x00000002, 0x00000004, 0x00000000, 0x00000003, 0x000200f8,
	0x00000005, 0x0004003d, 0x00000009, 0x0000001c, 0x0000000a, 0x00050041, 0x00000019, 0x0000001d,
	0x00000018, 0x0000001c, 0x0004003d, 0x00000007, 0x0000001e, 0x0000001d, 0x00050051, 0x00000006,
	0x0000001f, 0x0000001e, 0x00000000, 0x00050051, 0x00000006, 0x00000020, 0x0000001e, 0x00000001,
	0x00070050, 0x0000000b, 0x00000021, 0x0000001f, 0x00000020, 0x00000011, 0x0000001b, 0x00050041,
	0x0000001a, 0x00000022, 0x0000000e, 0x00000010, 0x0003003e, 0x00000022, 0x00000021, 0x000100fd,
	0x00010038,
}

var fragmentShaderSPIRV = []uint32{
	0x07230203, 0x00010000, 0x0008000a, 0x0000000d, 0x00000000, 0x00020011, 0x00000001, 0x0006000b,
	0x00000001, 0x4c534c47, 0x6474732e, 0x3035342e, 0x00000000, 0x0003000e, 0x00000000, 0x00000001,
	0x0006000f, 0x00000004, 0x00000004, 0x6e69616d, 0x00000000, 0x00000009, 0x00030010, 0x00000004,
	0x00000007, 0x00030003, 0x00000002, 0x000001c2, 0x00040005, 0x00000004, 0x6e69616d, 0x00000000,
	0x00050005, 0x00000009, 0x4374756f, 0x726f6c6f, 0x00000000, 0x00040047, 0x00000009, 0x0000001e,
	0x00000000, 0x00020013, 0x00000002, 0x00030021, 0x00000003, 0x00000002, 0x00030016, 0x00000006,
	0x00000020, 0x00040017, 0x00000007, 0x00000006, 0x00000004, 0x00040020, 0x00000008, 0x00000003,
	0x00000007, 0x0004003b, 0x00000008, 0x00000009, 0x00000003, 0x0004002b, 0x00000006, 0x0000000a,
	0x3f800000, 0x0004002b, 0x00000006, 0x0000000b, 0x00000000, 0x0007002c, 0x00000007, 0x0000000c,
	0x0000000a, 0x0000000b, 0x0000000b, 0x0000000a, 0x00050036, 0x00000002, 0x00000004, 0x00000000,
	0x00000003, 0x000200f8, 0x00000005, 0x0003003e, 0x00000009, 0x0000000c, 0x000100fd, 0x00010038,
}
