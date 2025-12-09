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

	var imageIndex uint32
	result := vkAcquireNextImageKHR(sc.device, sc.handle, ^uint64(0), sc.imageAvailable, 0, &imageIndex)

	switch result {
	case vk.Success:
		// OK
	case vk.SuboptimalKhr:
		// Suboptimal but usable
		sc.currentImage = imageIndex
		sc.imageAcquired = true
		return sc.surfaceTextures[imageIndex], true, nil
	case vk.ErrorOutOfDateKhr:
		return nil, false, hal.ErrSurfaceOutdated
	default:
		return nil, false, fmt.Errorf("vulkan: vkAcquireNextImageKHR failed: %d", result)
	}

	sc.currentImage = imageIndex
	sc.imageAcquired = true
	return sc.surfaceTextures[imageIndex], false, nil
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

// Vulkan function wrappers

func vkGetPhysicalDeviceSurfaceCapabilitiesKHR(i *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR, capabilities *vk.SurfaceCapabilitiesKHR) vk.Result {
	proc := vk.GetInstanceProcAddr(i.handle, "vkGetPhysicalDeviceSurfaceCapabilitiesKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(device),
		uintptr(surface),
		uintptr(unsafe.Pointer(capabilities)))
	return vk.Result(r)
}

func vkCreateSwapchainKHR(d *Device, createInfo *vk.SwapchainCreateInfoKHR, allocator unsafe.Pointer, swapchain *vk.SwapchainKHR) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkCreateSwapchainKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(swapchain)))
	return vk.Result(r)
}

//nolint:unparam // allocator kept for Vulkan API consistency
func vkDestroySwapchainKHR(d *Device, swapchain vk.SwapchainKHR, allocator unsafe.Pointer) {
	proc := vk.GetDeviceProcAddr(d.handle, "vkDestroySwapchainKHR")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(swapchain),
		uintptr(allocator))
}

func vkGetSwapchainImagesKHR(d *Device, swapchain vk.SwapchainKHR, count *uint32, images *vk.Image) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkGetSwapchainImagesKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(swapchain),
		uintptr(unsafe.Pointer(count)),
		uintptr(unsafe.Pointer(images)))
	return vk.Result(r)
}

func vkAcquireNextImageKHR(d *Device, swapchain vk.SwapchainKHR, timeout uint64, semaphore vk.Semaphore, fence vk.Fence, imageIndex *uint32) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkAcquireNextImageKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(swapchain),
		uintptr(timeout),
		uintptr(semaphore),
		uintptr(fence),
		uintptr(unsafe.Pointer(imageIndex)))
	return vk.Result(r)
}

func vkQueuePresentKHR(q *Queue, presentInfo *vk.PresentInfoKHR) vk.Result {
	proc := vk.GetDeviceProcAddr(q.device.handle, "vkQueuePresentKHR")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(q.handle),
		uintptr(unsafe.Pointer(presentInfo)))
	return vk.Result(r)
}

func vkCreateImageViewSwapchain(d *Device, createInfo *vk.ImageViewCreateInfo, allocator unsafe.Pointer, view *vk.ImageView) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkCreateImageView")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(view)))
	return vk.Result(r)
}

//nolint:unparam // allocator kept for Vulkan API consistency
func vkDestroyImageViewSwapchain(d *Device, view vk.ImageView, allocator unsafe.Pointer) {
	proc := vk.GetDeviceProcAddr(d.handle, "vkDestroyImageView")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(view),
		uintptr(allocator))
}

func vkCreateSemaphore(d *Device, createInfo *vk.SemaphoreCreateInfo, allocator unsafe.Pointer, semaphore *vk.Semaphore) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkCreateSemaphore")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(unsafe.Pointer(createInfo)),
		uintptr(allocator),
		uintptr(unsafe.Pointer(semaphore)))
	return vk.Result(r)
}

func vkDestroySemaphore(d *Device, semaphore vk.Semaphore, allocator unsafe.Pointer) {
	proc := vk.GetDeviceProcAddr(d.handle, "vkDestroySemaphore")
	if proc == 0 {
		return
	}
	//nolint:errcheck // Vulkan void function
	syscall.SyscallN(proc,
		uintptr(d.handle),
		uintptr(semaphore),
		uintptr(allocator))
}

func vkDeviceWaitIdle(d *Device) vk.Result {
	proc := vk.GetDeviceProcAddr(d.handle, "vkDeviceWaitIdle")
	if proc == 0 {
		return vk.ErrorExtensionNotPresent
	}
	r, _, _ := syscall.SyscallN(proc,
		uintptr(d.handle))
	return vk.Result(r)
}
