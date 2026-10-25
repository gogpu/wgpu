//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"

	"github.com/gogpu/naga"
)

const pushConstantShaderWGSL = `
struct Push { value: u32 }
var<push_constant> push: Push;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() { out[0] = push.value; }
`

func TestCmdPushConstants(t *testing.T) {
	c, instance := newTestInstance(t, "VK_KHR_surface")
	pd, device, family := newTestDevice(t, c, instance)

	spirv, err := naga.Compile(pushConstantShaderWGSL)
	if err != nil {
		t.Fatalf("compile shader: %v", err)
	}
	moduleInfo := ShaderModuleCreateInfo{
		SType:    StructureTypeShaderModuleCreateInfo,
		CodeSize: uintptr(len(spirv)),
		PCode:    (*uint32)(unsafe.Pointer(&spirv[0])),
	}
	var module ShaderModule
	if r := c.CreateShaderModule(device, &moduleInfo, nil, &module); r != Success {
		t.Fatalf("vkCreateShaderModule: %d", r)
	}
	defer c.DestroyShaderModule(device, module, nil)

	binding := DescriptorSetLayoutBinding{
		DescriptorType:  DescriptorTypeStorageBuffer,
		DescriptorCount: 1,
		StageFlags:      ShaderStageFlags(ShaderStageComputeBit),
	}
	setLayoutInfo := DescriptorSetLayoutCreateInfo{SType: StructureTypeDescriptorSetLayoutCreateInfo, BindingCount: 1, PBindings: &binding}
	var setLayout DescriptorSetLayout
	if r := c.CreateDescriptorSetLayout(device, &setLayoutInfo, nil, &setLayout); r != Success {
		t.Fatalf("vkCreateDescriptorSetLayout: %d", r)
	}
	defer c.DestroyDescriptorSetLayout(device, setLayout, nil)

	pushRange := PushConstantRange{StageFlags: ShaderStageFlags(ShaderStageComputeBit), Size: 4}
	layoutInfo := PipelineLayoutCreateInfo{
		SType:                  StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount:         1,
		PSetLayouts:            &setLayout,
		PushConstantRangeCount: 1,
		PPushConstantRanges:    &pushRange,
	}
	var layout PipelineLayout
	if r := c.CreatePipelineLayout(device, &layoutInfo, nil, &layout); r != Success {
		t.Fatalf("vkCreatePipelineLayout: %d", r)
	}
	defer c.DestroyPipelineLayout(device, layout, nil)

	entry := []byte("main\x00")
	pipelineInfo := ComputePipelineCreateInfo{
		SType: StructureTypeComputePipelineCreateInfo,
		Stage: PipelineShaderStageCreateInfo{
			SType:  StructureTypePipelineShaderStageCreateInfo,
			Stage:  ShaderStageComputeBit,
			Module: module,
			PName:  uintptr(unsafe.Pointer(&entry[0])),
		},
		Layout: layout,
	}
	var pipeline Pipeline
	if r := c.CreateComputePipelines(device, 0, 1, &pipelineInfo, nil, &pipeline); r != Success {
		t.Fatalf("vkCreateComputePipelines: %d", r)
	}
	defer c.DestroyPipeline(device, pipeline, nil)

	bufferInfo := BufferCreateInfo{SType: StructureTypeBufferCreateInfo, Size: 4, Usage: BufferUsageFlags(BufferUsageStorageBufferBit)}
	var buffer Buffer
	if r := c.CreateBuffer(device, &bufferInfo, nil, &buffer); r != Success {
		t.Fatalf("vkCreateBuffer: %d", r)
	}
	defer c.DestroyBuffer(device, buffer, nil)
	var req MemoryRequirements
	c.GetBufferMemoryRequirements(device, buffer, &req)
	var memProps PhysicalDeviceMemoryProperties
	c.GetPhysicalDeviceMemoryProperties(pd, &memProps)
	hostVisible := MemoryPropertyFlags(MemoryPropertyHostVisibleBit | MemoryPropertyHostCoherentBit)
	memoryType := memProps.MemoryTypeCount
	for i := range memProps.MemoryTypeCount {
		if req.MemoryTypeBits&(1<<i) != 0 && memProps.MemoryTypes[i].PropertyFlags&hostVisible == hostVisible {
			memoryType = i
			break
		}
	}
	if memoryType == memProps.MemoryTypeCount {
		t.Skip("no host-visible, host-coherent memory type")
	}
	allocInfo := MemoryAllocateInfo{SType: StructureTypeMemoryAllocateInfo, AllocationSize: req.Size, MemoryTypeIndex: memoryType}
	var memory DeviceMemory
	if r := c.AllocateMemory(device, &allocInfo, nil, &memory); r != Success {
		t.Fatalf("vkAllocateMemory: %d", r)
	}
	defer c.FreeMemory(device, memory, nil)
	if r := c.BindBufferMemory(device, buffer, memory, 0); r != Success {
		t.Fatalf("vkBindBufferMemory: %d", r)
	}

	poolSize := DescriptorPoolSize{Type: DescriptorTypeStorageBuffer, DescriptorCount: 1}
	descPoolInfo := DescriptorPoolCreateInfo{SType: StructureTypeDescriptorPoolCreateInfo, MaxSets: 1, PoolSizeCount: 1, PPoolSizes: &poolSize}
	var descPool DescriptorPool
	if r := c.CreateDescriptorPool(device, &descPoolInfo, nil, &descPool); r != Success {
		t.Fatalf("vkCreateDescriptorPool: %d", r)
	}
	defer c.DestroyDescriptorPool(device, descPool, nil)
	setInfo := DescriptorSetAllocateInfo{SType: StructureTypeDescriptorSetAllocateInfo, DescriptorPool: descPool, DescriptorSetCount: 1, PSetLayouts: &setLayout}
	var set DescriptorSet
	if r := c.AllocateDescriptorSets(device, &setInfo, &set); r != Success {
		t.Fatalf("vkAllocateDescriptorSets: %d", r)
	}
	bufferDesc := DescriptorBufferInfo{Buffer: buffer, Range: 4}
	write := WriteDescriptorSet{
		SType:           StructureTypeWriteDescriptorSet,
		DstSet:          set,
		DescriptorCount: 1,
		DescriptorType:  DescriptorTypeStorageBuffer,
		PBufferInfo:     &bufferDesc,
	}
	c.UpdateDescriptorSets(device, 1, &write, 0, nil)

	cmdPoolInfo := CommandPoolCreateInfo{SType: StructureTypeCommandPoolCreateInfo, QueueFamilyIndex: family}
	var cmdPool CommandPool
	if r := c.CreateCommandPool(device, &cmdPoolInfo, nil, &cmdPool); r != Success {
		t.Fatalf("vkCreateCommandPool: %d", r)
	}
	defer c.DestroyCommandPool(device, cmdPool, nil)
	cmdInfo := CommandBufferAllocateInfo{SType: StructureTypeCommandBufferAllocateInfo, CommandPool: cmdPool, Level: CommandBufferLevelPrimary, CommandBufferCount: 1}
	var cmd CommandBuffer
	if r := c.AllocateCommandBuffers(device, &cmdInfo, &cmd); r != Success {
		t.Fatalf("vkAllocateCommandBuffers: %d", r)
	}

	beginInfo := CommandBufferBeginInfo{SType: StructureTypeCommandBufferBeginInfo}
	if r := c.BeginCommandBuffer(cmd, &beginInfo); r != Success {
		t.Fatalf("vkBeginCommandBuffer: %d", r)
	}
	c.CmdBindPipeline(cmd, PipelineBindPointCompute, pipeline)
	c.CmdBindDescriptorSets(cmd, PipelineBindPointCompute, layout, 0, 1, &set, 0, nil)
	const want = uint32(0xC0FFEE42)
	value := want
	c.CmdPushConstants(cmd, layout, ShaderStageFlags(ShaderStageComputeBit), 0, 4, (*uintptr)(unsafe.Pointer(&value)))
	c.CmdDispatch(cmd, 1, 1, 1)
	toHost := MemoryBarrier{
		SType:         StructureTypeMemoryBarrier,
		SrcAccessMask: AccessFlags(AccessShaderWriteBit),
		DstAccessMask: AccessFlags(AccessHostReadBit),
	}
	c.CmdPipelineBarrier(cmd, PipelineStageFlags(PipelineStageComputeShaderBit), PipelineStageFlags(PipelineStageHostBit), 0, 1, &toHost, 0, nil, 0, nil)
	if r := c.EndCommandBuffer(cmd); r != Success {
		t.Fatalf("vkEndCommandBuffer: %d", r)
	}

	var queue Queue
	c.GetDeviceQueue(device, family, 0, &queue)
	submit := SubmitInfo{SType: StructureTypeSubmitInfo, CommandBufferCount: 1, PCommandBuffers: &cmd}
	if r := c.QueueSubmit(queue, 1, &submit, 0); r != Success {
		t.Fatalf("vkQueueSubmit: %d", r)
	}
	if r := c.QueueWaitIdle(queue); r != Success {
		t.Fatalf("vkQueueWaitIdle: %d", r)
	}

	var mapped unsafe.Pointer
	if r := c.MapMemory(device, memory, 0, 4, 0, uintptr(unsafe.Pointer(&mapped))); r != Success {
		t.Fatalf("vkMapMemory: %d", r)
	}
	defer c.UnmapMemory(device, memory)
	if got := *(*uint32)(mapped); got != want {
		t.Fatalf("shader read push constant %#x, want %#x", got, want)
	}
}
