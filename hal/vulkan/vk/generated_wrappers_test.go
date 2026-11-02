//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import (
	"testing"
	"unsafe"
)

func TestGetImageSubresourceLayoutAndCmdClearColorImage(t *testing.T) {
	c, instance := newTestInstance(t, "VK_KHR_surface")
	pd, device, family := newTestDevice(t, c, instance)

	const width, height = 4, 4
	imageInfo := ImageCreateInfo{
		SType:         StructureTypeImageCreateInfo,
		ImageType:     ImageType2d,
		Format:        FormatR8g8b8a8Unorm,
		Extent:        Extent3D{Width: width, Height: height, Depth: 1},
		MipLevels:     1,
		ArrayLayers:   1,
		Samples:       SampleCount1Bit,
		Tiling:        ImageTilingLinear,
		Usage:         ImageUsageFlags(ImageUsageTransferDstBit),
		InitialLayout: ImageLayoutUndefined,
	}
	var image Image
	if r := c.CreateImage(device, &imageInfo, nil, &image); r != Success {
		t.Fatalf("vkCreateImage: %d", r)
	}
	defer c.DestroyImage(device, image, nil)

	var req MemoryRequirements
	c.GetImageMemoryRequirements(device, image, &req)
	memory := allocateHostVisible(t, c, pd, device, req, nil)
	if r := c.BindImageMemory(device, image, memory, 0); r != Success {
		t.Fatalf("vkBindImageMemory: %d", r)
	}

	subresource := ImageSubresource{AspectMask: ImageAspectFlags(ImageAspectColorBit)}
	var layout SubresourceLayout
	c.GetImageSubresourceLayout(device, image, &subresource, &layout)
	if layout.RowPitch < width*4 || layout.Size < layout.RowPitch*(height-1)+width*4 {
		t.Fatalf("vkGetImageSubresourceLayout returned %+v for a %dx%d RGBA8 image", layout, width, height)
	}

	poolInfo := CommandPoolCreateInfo{SType: StructureTypeCommandPoolCreateInfo, QueueFamilyIndex: family}
	var pool CommandPool
	if r := c.CreateCommandPool(device, &poolInfo, nil, &pool); r != Success {
		t.Fatalf("vkCreateCommandPool: %d", r)
	}
	defer c.DestroyCommandPool(device, pool, nil)
	allocInfo := CommandBufferAllocateInfo{
		SType:              StructureTypeCommandBufferAllocateInfo,
		CommandPool:        pool,
		Level:              CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	var cmd CommandBuffer
	if r := c.AllocateCommandBuffers(device, &allocInfo, &cmd); r != Success {
		t.Fatalf("vkAllocateCommandBuffers: %d", r)
	}

	beginInfo := CommandBufferBeginInfo{SType: StructureTypeCommandBufferBeginInfo}
	if r := c.BeginCommandBuffer(cmd, &beginInfo); r != Success {
		t.Fatalf("vkBeginCommandBuffer: %d", r)
	}
	subresourceRange := ImageSubresourceRange{AspectMask: ImageAspectFlags(ImageAspectColorBit), LevelCount: 1, LayerCount: 1}
	toGeneral := ImageMemoryBarrier{
		SType:               StructureTypeImageMemoryBarrier,
		DstAccessMask:       AccessFlags(AccessTransferWriteBit),
		OldLayout:           ImageLayoutUndefined,
		NewLayout:           ImageLayoutGeneral,
		SrcQueueFamilyIndex: QueueFamilyIgnored,
		DstQueueFamilyIndex: QueueFamilyIgnored,
		Image:               image,
		SubresourceRange:    subresourceRange,
	}
	c.CmdPipelineBarrier(cmd, PipelineStageFlags(PipelineStageTopOfPipeBit), PipelineStageFlags(PipelineStageTransferBit), 0, 0, nil, 0, nil, 1, &toGeneral)
	var color ClearColorValue
	*(*[4]float32)(unsafe.Pointer(&color)) = [4]float32{1, 0, 1, 1}
	c.CmdClearColorImage(cmd, image, ImageLayoutGeneral, &color, 1, &subresourceRange)
	toHost := toGeneral
	toHost.SrcAccessMask = AccessFlags(AccessTransferWriteBit)
	toHost.DstAccessMask = AccessFlags(AccessHostReadBit)
	toHost.OldLayout = ImageLayoutGeneral
	c.CmdPipelineBarrier(cmd, PipelineStageFlags(PipelineStageTransferBit), PipelineStageFlags(PipelineStageHostBit), 0, 0, nil, 0, nil, 1, &toHost)
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
	if r := c.MapMemory(device, memory, 0, req.Size, 0, uintptr(unsafe.Pointer(&mapped))); r != Success {
		t.Fatalf("vkMapMemory: %d", r)
	}
	defer c.UnmapMemory(device, memory)
	pixels := unsafe.Slice((*byte)(mapped), req.Size)
	for y := uint64(0); y < height; y++ {
		row := uint64(layout.Offset) + y*uint64(layout.RowPitch)
		for x := uint64(0); x < width; x++ {
			px := pixels[row+x*4 : row+x*4+4]
			if px[0] != 0xff || px[1] != 0 || px[2] != 0xff || px[3] != 0xff {
				t.Fatalf("pixel (%d,%d) = %v after vkCmdClearColorImage, want [255 0 255 255]", x, y, px)
			}
		}
	}
}

func allocateHostVisible(t *testing.T, c *Commands, pd PhysicalDevice, device Device, req MemoryRequirements, pNext unsafe.Pointer) DeviceMemory {
	t.Helper()
	var props PhysicalDeviceMemoryProperties
	c.GetPhysicalDeviceMemoryProperties(pd, &props)
	want := MemoryPropertyFlags(MemoryPropertyHostVisibleBit | MemoryPropertyHostCoherentBit)
	for i := uint32(0); i < props.MemoryTypeCount; i++ {
		if req.MemoryTypeBits&(1<<i) == 0 || props.MemoryTypes[i].PropertyFlags&want != want {
			continue
		}
		info := MemoryAllocateInfo{
			SType:           StructureTypeMemoryAllocateInfo,
			PNext:           (*uintptr)(pNext),
			AllocationSize:  req.Size,
			MemoryTypeIndex: i,
		}
		var memory DeviceMemory
		if r := c.AllocateMemory(device, &info, nil, &memory); r != Success {
			t.Fatalf("vkAllocateMemory: %d", r)
		}
		t.Cleanup(func() { c.FreeMemory(device, memory, nil) })
		return memory
	}
	t.Skip("no host-visible, host-coherent memory type")
	return 0
}
