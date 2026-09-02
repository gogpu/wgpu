//go:build !(js && wasm)

package vulkan

import (
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/internal/indirect"
)

type pfnCmdDrawIndirectCount func(
	commandBuffer vk.CommandBuffer,
	buffer vk.Buffer,
	offset vk.DeviceSize,
	countBuffer vk.Buffer,
	countBufferOffset vk.DeviceSize,
	maxDrawCount uint32,
)

type pfnCmdDrawIndexedIndirectCount func(
	commandBuffer vk.CommandBuffer,
	buffer vk.Buffer,
	offset vk.DeviceSize,
	countBuffer vk.Buffer,
	countBufferOffset vk.DeviceSize,
	maxDrawCount uint32,
)

type vulkanIndirectCountBuffers struct {
	indirect *Buffer
	count    *Buffer
}

type vulkanIndirectCountNativeCmd func(
	cmd vk.CommandBuffer,
	indirect vk.Buffer,
	offset vk.DeviceSize,
	count vk.Buffer,
	countOffset vk.DeviceSize,
	maxDrawCount uint32,
)

func loadDrawIndirectCountProcs(device *Device) {
	if device == nil || !device.supportsDrawIndirectCount {
		return
	}
	if ptr := vk.GetDeviceProcAddr(device.handle, "vkCmdDrawIndirectCount"); ptr != nil {
		device.cmdDrawIndirectCount = *(*pfnCmdDrawIndirectCount)(ptr)
	}
	if ptr := vk.GetDeviceProcAddr(device.handle, "vkCmdDrawIndexedIndirectCount"); ptr != nil {
		device.cmdDrawIndexedIndirectCount = *(*pfnCmdDrawIndexedIndirectCount)(ptr)
	}
}

func supportsDrawIndirectCountFeature(apiVersion uint32) bool {
	return apiVersion >= vkMakeVersion(1, 2, 0)
}

func (e *RenderPassEncoder) vulkanIndirectCountBuffers(
	buffer, countBuffer hal.Buffer,
	maxDrawCount uint32,
) (vulkanIndirectCountBuffers, bool) {
	buf, ok := buffer.(*Buffer)
	countBuf, countOK := countBuffer.(*Buffer)
	if !ok || !countOK || e.encoder.active == 0 || maxDrawCount == 0 {
		return vulkanIndirectCountBuffers{}, false
	}
	return vulkanIndirectCountBuffers{indirect: buf, count: countBuf}, true
}

func (e *RenderPassEncoder) drawIndirectCount(
	buffer hal.Buffer, offset uint64,
	countBuffer hal.Buffer, countOffset uint64,
	maxDrawCount uint32,
	recordStride uint32,
	nativeCmd vulkanIndirectCountNativeCmd,
	fallback func(*vk.Commands, vk.CommandBuffer, vk.Buffer, vk.DeviceSize, uint32, uint32),
) {
	bufs, ok := e.vulkanIndirectCountBuffers(buffer, countBuffer, maxDrawCount)
	if !ok {
		return
	}
	if !indirect.RangeFits(bufs.indirect.size, offset, uint64(recordStride), maxDrawCount) {
		return
	}
	if countOffset+4 > bufs.count.size {
		return
	}
	if nativeCmd != nil {
		nativeCmd(e.encoder.active, bufs.indirect.handle, vk.DeviceSize(offset), bufs.count.handle, vk.DeviceSize(countOffset), maxDrawCount)
		return
	}
	fallback(e.encoder.device.cmds, e.encoder.active, bufs.indirect.handle, vk.DeviceSize(offset), maxDrawCount, recordStride)
}

var _ unsafe.Pointer
