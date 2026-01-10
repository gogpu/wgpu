// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// Swapchain manages Vulkan swapchain for a surface.
type Swapchain struct {
	handle          vk.SwapchainKHR
	surface         *Surface
	device          *Device
	images          []vk.Image
	imageViews      []vk.ImageView
	format          vk.Format
	extent          vk.Extent2D
	presentMode     vk.PresentModeKHR
	imageAvailable  vk.Semaphore // Signaled when image is acquired
	renderFinished  vk.Semaphore // Signaled when rendering is complete
	acquireFence    vk.Fence     // Fence for acquire sync (critical on Windows/Intel)
	currentImage    uint32
	imageAcquired   bool
	surfaceTextures []*SwapchainTexture
}

// SwapchainTexture wraps a swapchain image as a SurfaceTexture.
type SwapchainTexture struct {
	handle    vk.Image
	view      vk.ImageView
	index     uint32
	swapchain *Swapchain
	format    types.TextureFormat
	size      Extent3D
}

// Destroy implements hal.Texture.
func (t *SwapchainTexture) Destroy() {
	// Swapchain textures are owned by the swapchain, not destroyed individually
}

// createSwapchain creates a new swapchain for the surface.
func (s *Surface) createSwapchain(device *Device, config *hal.SurfaceConfiguration) error {
	// Get surface capabilities
	var capabilities vk.SurfaceCapabilitiesKHR
	result := vkGetPhysicalDeviceSurfaceCapabilitiesKHR(s.instance, device.physicalDevice, s.handle, &capabilities)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkGetPhysicalDeviceSurfaceCapabilitiesKHR failed: %d", result)
	}

	// Determine image count
	imageCount := capabilities.MinImageCount + 1
	if capabilities.MaxImageCount > 0 && imageCount > capabilities.MaxImageCount {
		imageCount = capabilities.MaxImageCount
	}

	// Determine extent
	extent := capabilities.CurrentExtent
	if extent.Width == 0xFFFFFFFF {
		extent.Width = config.Width
		extent.Height = config.Height
	}

	// Convert format
	vkFormat := textureFormatToVk(config.Format)

	// Convert present mode
	presentMode := presentModeToVk(config.PresentMode)

	// Convert usage
	imageUsage := vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit)
	if config.Usage&types.TextureUsageCopySrc != 0 {
		imageUsage |= vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit)
	}
	if config.Usage&types.TextureUsageCopyDst != 0 {
		imageUsage |= vk.ImageUsageFlags(vk.ImageUsageTransferDstBit)
	}

	// Get old swapchain handle for recreation
	var oldSwapchain vk.SwapchainKHR
	if s.swapchain != nil {
		oldSwapchain = s.swapchain.handle
	}

	// Create swapchain
	createInfo := vk.SwapchainCreateInfoKHR{
		SType:            vk.StructureTypeSwapchainCreateInfoKhr,
		Surface:          s.handle,
		MinImageCount:    imageCount,
		ImageFormat:      vkFormat,
		ImageColorSpace:  vk.ColorSpaceSrgbNonlinearKhr,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       imageUsage,
		ImageSharingMode: vk.SharingModeExclusive,
		PreTransform:     capabilities.CurrentTransform,
		CompositeAlpha:   vk.CompositeAlphaOpaqueBitKhr,
		PresentMode:      presentMode,
		Clipped:          vk.True,
		OldSwapchain:     oldSwapchain,
	}

	var swapchainHandle vk.SwapchainKHR
	result = vkCreateSwapchainKHR(device, &createInfo, nil, &swapchainHandle)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkCreateSwapchainKHR failed: %d", result)
	}

	// Destroy old swapchain if it exists
	if s.swapchain != nil {
		s.swapchain.destroyResources()
		s.swapchain = nil
	}

	// Get swapchain images
	var swapchainImageCount uint32
	result = vkGetSwapchainImagesKHR(device, swapchainHandle, &swapchainImageCount, nil)
	if result != vk.Success {
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkGetSwapchainImagesKHR (count) failed: %d", result)
	}

	images := make([]vk.Image, swapchainImageCount)
	result = vkGetSwapchainImagesKHR(device, swapchainHandle, &swapchainImageCount, &images[0])
	if result != vk.Success {
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkGetSwapchainImagesKHR (images) failed: %d", result)
	}

	// Create image views
	imageViews := make([]vk.ImageView, len(images))
	for i, img := range images {
		viewCreateInfo := vk.ImageViewCreateInfo{
			SType:    vk.StructureTypeImageViewCreateInfo,
			Image:    img,
			ViewType: vk.ImageViewType2d,
			Format:   vkFormat,
			Components: vk.ComponentMapping{
				R: vk.ComponentSwizzleIdentity,
				G: vk.ComponentSwizzleIdentity,
				B: vk.ComponentSwizzleIdentity,
				A: vk.ComponentSwizzleIdentity,
			},
			SubresourceRange: vk.ImageSubresourceRange{
				AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
		}

		result = vkCreateImageViewSwapchain(device, &viewCreateInfo, nil, &imageViews[i])
		if result != vk.Success {
			// Cleanup created views
			for j := 0; j < i; j++ {
				vkDestroyImageViewSwapchain(device, imageViews[j], nil)
			}
			vkDestroySwapchainKHR(device, swapchainHandle, nil)
			return fmt.Errorf("vulkan: vkCreateImageView failed: %d", result)
		}
	}

	// Create synchronization semaphores
	semaphoreInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
	}

	var imageAvailable, renderFinished vk.Semaphore
	result = vkCreateSemaphore(device, &semaphoreInfo, nil, &imageAvailable)
	if result != vk.Success {
		for _, view := range imageViews {
			vkDestroyImageViewSwapchain(device, view, nil)
		}
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkCreateSemaphore (imageAvailable) failed: %d", result)
	}

	result = vkCreateSemaphore(device, &semaphoreInfo, nil, &renderFinished)
	if result != vk.Success {
		vkDestroySemaphore(device, imageAvailable, nil)
		for _, view := range imageViews {
			vkDestroyImageViewSwapchain(device, view, nil)
		}
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkCreateSemaphore (renderFinished) failed: %d", result)
	}

	// Create acquire fence for proper synchronization on Windows/Intel.
	// This is critical to ensure the acquired image is fully ready before rendering.
	// See wgpu-rs issues #8310 and #8354 for details.
	// Note: Fence starts unsignaled - the first acquire will signal it.
	fenceInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: 0, // Start unsignaled - acquire will signal it
	}

	var acquireFence vk.Fence
	result = vkCreateFenceSwapchain(device, &fenceInfo, nil, &acquireFence)
	if result != vk.Success {
		vkDestroySemaphore(device, renderFinished, nil)
		vkDestroySemaphore(device, imageAvailable, nil)
		for _, view := range imageViews {
			vkDestroyImageViewSwapchain(device, view, nil)
		}
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkCreateFence (acquireFence) failed: %d", result)
	}

	// Create surface textures
	surfaceTextures := make([]*SwapchainTexture, len(images))
	for i, img := range images {
		surfaceTextures[i] = &SwapchainTexture{
			handle: img,
			view:   imageViews[i],
			index:  uint32(i),
			format: config.Format,
			size: Extent3D{
				Width:  extent.Width,
				Height: extent.Height,
				Depth:  1,
			},
		}
	}

	// Store swapchain
	swapchain := &Swapchain{
		handle:          swapchainHandle,
		surface:         s,
		device:          device,
		images:          images,
		imageViews:      imageViews,
		format:          vkFormat,
		extent:          extent,
		presentMode:     presentMode,
		imageAvailable:  imageAvailable,
		renderFinished:  renderFinished,
		acquireFence:    acquireFence,
		surfaceTextures: surfaceTextures,
	}

	// Link swapchain to surface textures
	for _, tex := range surfaceTextures {
		tex.swapchain = swapchain
	}

	s.swapchain = swapchain
	s.device = device

	return nil
}

// destroyResources destroys swapchain resources without the swapchain itself.
func (sc *Swapchain) destroyResources() {
	if sc.device == nil {
		return
	}

	// Wait for device idle before destroying
	vkDeviceWaitIdle(sc.device)

	// Destroy fence
	if sc.acquireFence != 0 {
		vkDestroyFenceSwapchain(sc.device, sc.acquireFence, nil)
		sc.acquireFence = 0
	}

	// Destroy semaphores
	if sc.imageAvailable != 0 {
		vkDestroySemaphore(sc.device, sc.imageAvailable, nil)
		sc.imageAvailable = 0
	}
	if sc.renderFinished != 0 {
		vkDestroySemaphore(sc.device, sc.renderFinished, nil)
		sc.renderFinished = 0
	}

	// Destroy image views
	for _, view := range sc.imageViews {
		if view != 0 {
			vkDestroyImageViewSwapchain(sc.device, view, nil)
		}
	}
	sc.imageViews = nil
	sc.images = nil
	sc.surfaceTextures = nil
}

// Destroy destroys the swapchain completely.
func (sc *Swapchain) Destroy() {
	sc.destroyResources()

	if sc.handle != 0 && sc.device != nil {
		vkDestroySwapchainKHR(sc.device, sc.handle, nil)
		sc.handle = 0
	}
}

// acquireNextImage acquires the next available swapchain image.
func (sc *Swapchain) acquireNextImage() (*SwapchainTexture, bool, error) {
	if sc.imageAcquired {
		return nil, false, fmt.Errorf("vulkan: image already acquired")
	}

	const timeout = ^uint64(0) // UINT64_MAX = wait forever

	// Acquire next image with BOTH semaphore AND fence.
	// The fence will be signaled when the image is fully ready.
	var imageIndex uint32
	result := vkAcquireNextImageKHR(sc.device, sc.handle, timeout, sc.imageAvailable, sc.acquireFence, &imageIndex)

	switch result {
	case vk.Success, vk.SuboptimalKhr:
		// OK - continue to wait for fence
	case vk.ErrorOutOfDateKhr:
		return nil, false, hal.ErrSurfaceOutdated
	default:
		return nil, false, fmt.Errorf("vulkan: vkAcquireNextImageKHR failed: %d", result)
	}

	// Wait for the image to be fully ready to render.
	// This is critical on Windows with Intel/DXGI swapchains to avoid
	// rendering to an image that isn't fully ready.
	// See wgpu-rs issues #8310 and #8354 for details.
	waitResult := vkWaitForFencesSwapchain(sc.device, 1, &sc.acquireFence, vk.True, timeout)
	if waitResult != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkWaitForFences failed: %d", waitResult)
	}

	// Reset fence for next acquire
	resetResult := vkResetFencesSwapchain(sc.device, 1, &sc.acquireFence)
	if resetResult != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkResetFences failed: %d", resetResult)
	}

	sc.currentImage = imageIndex
	sc.imageAcquired = true
	return sc.surfaceTextures[imageIndex], result == vk.SuboptimalKhr, nil
}

// present presents the current image to the screen.
func (sc *Swapchain) present(queue *Queue) error {
	if !sc.imageAcquired {
		return fmt.Errorf("vulkan: no image acquired to present")
	}

	presentInfo := vk.PresentInfoKHR{
		SType:              vk.StructureTypePresentInfoKhr,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    &sc.renderFinished,
		SwapchainCount:     1,
		PSwapchains:        &sc.handle,
		PImageIndices:      &sc.currentImage,
	}

	result := vkQueuePresentKHR(queue, &presentInfo)
	sc.imageAcquired = false

	switch result {
	case vk.Success:
		return nil
	case vk.SuboptimalKhr:
		// Suboptimal but presented successfully
		return nil
	case vk.ErrorOutOfDateKhr:
		return hal.ErrSurfaceOutdated
	default:
		return fmt.Errorf("vulkan: vkQueuePresentKHR failed: %d", result)
	}
}

// presentModeToVk converts HAL PresentMode to Vulkan PresentModeKHR.
func presentModeToVk(mode hal.PresentMode) vk.PresentModeKHR {
	switch mode {
	case hal.PresentModeImmediate:
		return vk.PresentModeImmediateKhr
	case hal.PresentModeMailbox:
		return vk.PresentModeMailboxKhr
	case hal.PresentModeFifo:
		return vk.PresentModeFifoKhr
	case hal.PresentModeFifoRelaxed:
		return vk.PresentModeFifoRelaxedKhr
	default:
		return vk.PresentModeFifoKhr
	}
}

// Vulkan function wrappers using Commands methods

func vkGetPhysicalDeviceSurfaceCapabilitiesKHR(i *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR, capabilities *vk.SurfaceCapabilitiesKHR) vk.Result {
	return i.cmds.GetPhysicalDeviceSurfaceCapabilitiesKHR(device, surface, capabilities)
}

func vkCreateSwapchainKHR(d *Device, createInfo *vk.SwapchainCreateInfoKHR, _ *vk.AllocationCallbacks, swapchain *vk.SwapchainKHR) vk.Result {
	return d.cmds.CreateSwapchainKHR(d.handle, createInfo, nil, swapchain)
}

func vkDestroySwapchainKHR(d *Device, swapchain vk.SwapchainKHR, _ *vk.AllocationCallbacks) {
	d.cmds.DestroySwapchainKHR(d.handle, swapchain, nil)
}

func vkGetSwapchainImagesKHR(d *Device, swapchain vk.SwapchainKHR, count *uint32, images *vk.Image) vk.Result {
	return d.cmds.GetSwapchainImagesKHR(d.handle, swapchain, count, images)
}

func vkAcquireNextImageKHR(d *Device, swapchain vk.SwapchainKHR, timeout uint64, semaphore vk.Semaphore, fence vk.Fence, imageIndex *uint32) vk.Result {
	return d.cmds.AcquireNextImageKHR(d.handle, swapchain, timeout, semaphore, fence, imageIndex)
}

func vkQueuePresentKHR(q *Queue, presentInfo *vk.PresentInfoKHR) vk.Result {
	return q.device.cmds.QueuePresentKHR(q.handle, presentInfo)
}

func vkCreateImageViewSwapchain(d *Device, createInfo *vk.ImageViewCreateInfo, _ *vk.AllocationCallbacks, view *vk.ImageView) vk.Result {
	return d.cmds.CreateImageView(d.handle, createInfo, nil, view)
}

func vkDestroyImageViewSwapchain(d *Device, view vk.ImageView, _ *vk.AllocationCallbacks) {
	d.cmds.DestroyImageView(d.handle, view, nil)
}

func vkCreateSemaphore(d *Device, createInfo *vk.SemaphoreCreateInfo, _ *vk.AllocationCallbacks, semaphore *vk.Semaphore) vk.Result {
	return d.cmds.CreateSemaphore(d.handle, createInfo, nil, semaphore)
}

func vkDestroySemaphore(d *Device, semaphore vk.Semaphore, _ *vk.AllocationCallbacks) {
	d.cmds.DestroySemaphore(d.handle, semaphore, nil)
}

func vkCreateFenceSwapchain(d *Device, createInfo *vk.FenceCreateInfo, _ *vk.AllocationCallbacks, fence *vk.Fence) vk.Result {
	return d.cmds.CreateFence(d.handle, createInfo, nil, fence)
}

func vkDestroyFenceSwapchain(d *Device, fence vk.Fence, _ *vk.AllocationCallbacks) {
	d.cmds.DestroyFence(d.handle, fence, nil)
}

func vkWaitForFencesSwapchain(d *Device, fenceCount uint32, pFences *vk.Fence, waitAll vk.Bool32, timeout uint64) vk.Result {
	return d.cmds.WaitForFences(d.handle, fenceCount, pFences, waitAll, timeout)
}

func vkResetFencesSwapchain(d *Device, fenceCount uint32, pFences *vk.Fence) vk.Result {
	return d.cmds.ResetFences(d.handle, fenceCount, pFences)
}

func vkDeviceWaitIdle(d *Device) vk.Result {
	return d.cmds.DeviceWaitIdle(d.handle)
}
