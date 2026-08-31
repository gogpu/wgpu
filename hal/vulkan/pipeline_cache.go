//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"runtime"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/internal/pipelinecache"
)

const vulkanPipelineCacheFile = "pipeline.cache"

// initPipelineCache creates or restores the device-wide VkPipelineCache from disk.
// Pipeline cache is a performance optimization — failures are logged and the
// device continues with pipelineCache = 0 (VK_NULL_HANDLE).
// Reference: wgpu-hal/src/vulkan/device.rs (pipeline cache create/restore).
func (d *Device) initPipelineCache(props *vk.PhysicalDeviceProperties) {
	adapterKey := pipelinecache.VulkanAdapterKey(
		props.VendorID,
		props.DeviceID,
		props.DriverVersion,
		props.PipelineCacheUUID,
	)
	cachePath, err := pipelinecache.UserCachePath("vulkan", adapterKey, vulkanPipelineCacheFile)
	if err != nil {
		hal.Logger().Warn("vulkan: pipeline cache disabled, cache directory unavailable",
			"error", err,
		)
		return
	}
	d.pipelineCachePath = cachePath

	initialData, err := pipelinecache.LoadBlob(cachePath)
	if err != nil {
		hal.Logger().Warn("vulkan: failed to load pipeline cache, starting empty",
			"path", cachePath,
			"error", err,
		)
		initialData = nil
	}

	createInfo := vk.PipelineCacheCreateInfo{
		SType: vk.StructureTypePipelineCacheCreateInfo,
	}
	if len(initialData) > 0 {
		createInfo.InitialDataSize = uintptr(len(initialData))
		createInfo.PInitialData = (*uintptr)(unsafe.Pointer(&initialData[0]))
	}

	var cache vk.PipelineCache
	result := d.cmds.CreatePipelineCache(d.handle, &createInfo, nil, &cache)
	runtime.KeepAlive(initialData)
	if result == vk.ErrorInitializationFailed && len(initialData) > 0 {
		hal.Logger().Info("vulkan: stale pipeline cache rejected by driver, recreating empty",
			"path", cachePath,
		)
		_ = pipelinecache.DeleteBlob(cachePath)
		createInfo.InitialDataSize = 0
		createInfo.PInitialData = nil
		result = d.cmds.CreatePipelineCache(d.handle, &createInfo, nil, &cache)
	}
	if result != vk.Success {
		hal.Logger().Warn("vulkan: vkCreatePipelineCache failed, pipeline cache disabled",
			"result", result,
		)
		return
	}

	d.pipelineCache = cache
	if len(initialData) > 0 {
		hal.Logger().Info("vulkan: restored pipeline cache from disk",
			"path", cachePath,
			"bytes", len(initialData),
		)
	}
}

// savePipelineCache persists the VkPipelineCache blob to disk.
func (d *Device) savePipelineCache() {
	if d.pipelineCache == 0 || d.pipelineCachePath == "" {
		return
	}

	var size uintptr
	result := d.cmds.GetPipelineCacheData(d.handle, d.pipelineCache, &size, nil)
	if result != vk.Success || size == 0 {
		return
	}

	data := make([]byte, size)
	bufPtr := (*uintptr)(unsafe.Pointer(&data[0]))
	result = d.cmds.GetPipelineCacheData(d.handle, d.pipelineCache, &size, bufPtr)
	runtime.KeepAlive(data)
	if result != vk.Success {
		hal.Logger().Warn("vulkan: failed to read pipeline cache data",
			"result", result,
		)
		return
	}
	data = data[:size]

	if err := pipelinecache.SaveBlob(d.pipelineCachePath, data); err != nil {
		hal.Logger().Warn("vulkan: failed to save pipeline cache",
			"path", d.pipelineCachePath,
			"error", err,
		)
		return
	}
	hal.Logger().Info("vulkan: saved pipeline cache to disk",
		"path", d.pipelineCachePath,
		"bytes", len(data),
	)
}

// destroyPipelineCache saves and destroys the VkPipelineCache handle.
func (d *Device) destroyPipelineCache() {
	if d.pipelineCache == 0 {
		return
	}
	d.savePipelineCache()
	d.cmds.DestroyPipelineCache(d.handle, d.pipelineCache, nil)
	d.pipelineCache = 0
	d.pipelineCachePath = ""
}
