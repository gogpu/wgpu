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
	handle      vk.SwapchainKHR
	surface     *Surface
	device      *Device
	images      []vk.Image
	imageViews  []vk.ImageView
	format      vk.Format
	extent      vk.Extent2D
	presentMode vk.PresentModeKHR
	// Acquire semaphores - rotated through for each acquire (like wgpu).
	// We don't know which image we'll get, so we can't index by image.
	acquireSemaphores []vk.Semaphore
	// Fences to track when each acquire semaphore can be reused.
	// Must wait on fence[i] before reusing acquireSemaphores[i].
	acquireFences  []vk.Fence
	nextAcquireIdx int

	// Present semaphores - one per swapchain image (known after acquire).
	presentSemaphores []vk.Semaphore

	acquireFence      vk.Fence     // Fence for post-acquire sync (Windows/Intel fix)
	currentImage      uint32       // Current swapchain image index
	currentAcquireIdx int          // Index of acquire semaphore used for current frame
	currentAcquireSem vk.Semaphore // The acquire semaphore used for current frame
	imageAcquired     bool
	surfaceTextures   []*SwapchainTexture
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
//
//nolint:maintidx // Vulkan swapchain setup requires many sequential steps
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

	// Create synchronization primitives (wgpu-style).
	// Acquire semaphores: rotated through for each acquire (we don't know which image we'll get).
	// Present semaphores: one per swapchain image (known after acquire).
	semaphoreInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
	}

	// Create arrays for rotating semaphores (same count as images).
	acquireSemaphores := make([]vk.Semaphore, imageCount)
	presentSemaphores := make([]vk.Semaphore, imageCount)

	// Create acquire semaphores
	for i := range acquireSemaphores {
		result = vkCreateSemaphore(device, &semaphoreInfo, nil, &acquireSemaphores[i])
		if result != vk.Success {
			for j := 0; j < i; j++ {
				vkDestroySemaphore(device, acquireSemaphores[j], nil)
			}
			for _, view := range imageViews {
				vkDestroyImageViewSwapchain(device, view, nil)
			}
			vkDestroySwapchainKHR(device, swapchainHandle, nil)
			return fmt.Errorf("vulkan: vkCreateSemaphore (acquireSemaphore[%d]) failed: %d", i, result)
		}
	}

	// Create present semaphores
	for i := range presentSemaphores {
		result = vkCreateSemaphore(device, &semaphoreInfo, nil, &presentSemaphores[i])
		if result != vk.Success {
			for j := 0; j < i; j++ {
				vkDestroySemaphore(device, presentSemaphores[j], nil)
			}
			for _, sem := range acquireSemaphores {
				vkDestroySemaphore(device, sem, nil)
			}
			for _, view := range imageViews {
				vkDestroyImageViewSwapchain(device, view, nil)
			}
			vkDestroySwapchainKHR(device, swapchainHandle, nil)
			return fmt.Errorf("vulkan: vkCreateSemaphore (presentSemaphore[%d]) failed: %d", i, result)
		}
	}

	// Create per-acquire fences to track when each acquire semaphore can be reused.
	// Each fence is waited on before reusing its corresponding acquire semaphore,
	// and signaled when the submission that waited on that semaphore completes.
	// Start SIGNALED so first use doesn't block.
	fenceInfoSignaled := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: vk.FenceCreateFlags(vk.FenceCreateSignaledBit),
	}
	acquireFences := make([]vk.Fence, imageCount)
	for i := range acquireFences {
		result = vkCreateFenceSwapchain(device, &fenceInfoSignaled, nil, &acquireFences[i])
		if result != vk.Success {
			for j := 0; j < i; j++ {
				vkDestroyFenceSwapchain(device, acquireFences[j], nil)
			}
			for _, sem := range presentSemaphores {
				vkDestroySemaphore(device, sem, nil)
			}
			for _, sem := range acquireSemaphores {
				vkDestroySemaphore(device, sem, nil)
			}
			for _, view := range imageViews {
				vkDestroyImageViewSwapchain(device, view, nil)
			}
			vkDestroySwapchainKHR(device, swapchainHandle, nil)
			return fmt.Errorf("vulkan: vkCreateFence (acquireFence[%d]) failed: %d", i, result)
		}
	}

	// Create post-acquire fence for Windows/Intel synchronization.
	// This ensures the acquired image is fully ready before rendering.
	// See wgpu-rs issues #8310 and #8354 for details.
	fenceInfoUnsignaled := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: 0, // Start unsignaled - acquire will signal it
	}
	var acquireFence vk.Fence
	result = vkCreateFenceSwapchain(device, &fenceInfoUnsignaled, nil, &acquireFence)
	if result != vk.Success {
		for _, fence := range acquireFences {
			vkDestroyFenceSwapchain(device, fence, nil)
		}
		for _, sem := range presentSemaphores {
			vkDestroySemaphore(device, sem, nil)
		}
		for _, sem := range acquireSemaphores {
			vkDestroySemaphore(device, sem, nil)
		}
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
		handle:            swapchainHandle,
		surface:           s,
		device:            device,
		images:            images,
		imageViews:        imageViews,
		format:            vkFormat,
		extent:            extent,
		presentMode:       presentMode,
		acquireSemaphores: acquireSemaphores,
		acquireFences:     acquireFences,
		nextAcquireIdx:    0,
		presentSemaphores: presentSemaphores,
		acquireFence:      acquireFence,
		surfaceTextures:   surfaceTextures,
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

	// Destroy post-acquire fence
	if sc.acquireFence != 0 {
		vkDestroyFenceSwapchain(sc.device, sc.acquireFence, nil)
		sc.acquireFence = 0
	}

	// Destroy per-acquire fences
	for i, fence := range sc.acquireFences {
		if fence != 0 {
			vkDestroyFenceSwapchain(sc.device, fence, nil)
			sc.acquireFences[i] = 0
		}
	}
	sc.acquireFences = nil

	// Destroy acquire semaphores
	for i, sem := range sc.acquireSemaphores {
		if sem != 0 {
			vkDestroySemaphore(sc.device, sem, nil)
			sc.acquireSemaphores[i] = 0
		}
	}
	sc.acquireSemaphores = nil

	// Destroy present semaphores
	for i, sem := range sc.presentSemaphores {
		if sem != 0 {
			vkDestroySemaphore(sc.device, sem, nil)
			sc.presentSemaphores[i] = 0
		}
	}
	sc.presentSemaphores = nil

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
// Uses rotating acquire semaphores like wgpu to avoid reuse conflicts.
// Returns (nil, false, nil) if the frame should be skipped (timeout).
//
// Adapted from wgpu-hal vulkan/swapchain/native.rs acquire() function.
// Key differences from original blocking implementation:
// - Uses configurable timeout instead of infinite wait
// - Returns nil on timeout instead of blocking forever
// - Caller should skip frame rendering on nil return
func (sc *Swapchain) acquireNextImage() (*SwapchainTexture, bool, error) {
	if sc.imageAcquired {
		return nil, false, fmt.Errorf("vulkan: image already acquired")
	}

	// Timeout for all wait operations (matches wgpu's approach).
	// 16ms = one VSync period at 60Hz. This allows:
	// - VSync to signal the next available frame
	// - Reasonable responsiveness if GPU falls behind
	const timeout = uint64(16_000_000) // 16ms in nanoseconds

	// Wait for the previous submission that used this acquire semaphore to complete.
	// This ensures it's safe to reuse the semaphore.
	// (wgpu: wait_for_fence with previously_used_submission_index)
	acquireIdx := sc.nextAcquireIdx
	acquireFenceForSem := sc.acquireFences[acquireIdx]
	fenceStatus := vkGetFenceStatusSwapchain(sc.device, acquireFenceForSem)
	if fenceStatus == vk.NotReady {
		waitResult := vkWaitForFencesSwapchain(sc.device, 1, &acquireFenceForSem, vk.True, timeout)
		if waitResult == vk.Timeout {
			// Timeout - return nil to skip frame. DON'T advance semaphore rotation.
			// (wgpu: returns Ok(None) without advancing)
			return nil, false, nil
		} else if waitResult != vk.Success {
			return nil, false, fmt.Errorf("vulkan: vkWaitForFences (per-acquire) failed: %d", waitResult)
		}
	} else if fenceStatus != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkGetFenceStatus (per-acquire) failed: %d", fenceStatus)
	}
	// Reset fence for reuse
	resetResult := vkResetFencesSwapchain(sc.device, 1, &acquireFenceForSem)
	if resetResult != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkResetFences (per-acquire) failed: %d", resetResult)
	}

	// Get the acquire semaphore from the rotating pool.
	acquireSem := sc.acquireSemaphores[acquireIdx]

	// Acquire next image with semaphore AND post-acquire fence.
	// (wgpu: vkAcquireNextImageKHR with same timeout_ns)
	var imageIndex uint32
	result := vkAcquireNextImageKHR(sc.device, sc.handle, timeout, acquireSem, sc.acquireFence, &imageIndex)

	switch result {
	case vk.Success, vk.SuboptimalKhr:
		// OK - continue
	case vk.Timeout:
		// Timeout - return nil to skip frame. DON'T advance.
		// (wgpu: returns Ok(None))
		return nil, false, nil
	case vk.NotReady, vk.ErrorOutOfDateKhr:
		// Surface needs reconfiguration
		// (wgpu: returns Err(Outdated))
		return nil, false, hal.ErrSurfaceOutdated
	default:
		return nil, false, fmt.Errorf("vulkan: vkAcquireNextImageKHR failed: %d", result)
	}

	// Wait for post-acquire fence (Windows/Intel synchronization).
	// This is critical for proper frame pacing with DXGI swapchains.
	// See wgpu-rs issues #8310 and #8354.
	// (wgpu: wait_for_fences(&[self.fence], false, timeout_ns))
	postAcquireStatus := vkGetFenceStatusSwapchain(sc.device, sc.acquireFence)
	if postAcquireStatus == vk.NotReady {
		waitResult := vkWaitForFencesSwapchain(sc.device, 1, &sc.acquireFence, vk.True, timeout)
		if waitResult == vk.Timeout {
			// Timeout - skip frame
			return nil, false, nil
		} else if waitResult != vk.Success {
			return nil, false, fmt.Errorf("vulkan: vkWaitForFences (post-acquire) failed: %d", waitResult)
		}
	} else if postAcquireStatus != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkGetFenceStatus (post-acquire) failed: %d", postAcquireStatus)
	}

	// Reset post-acquire fence for next acquire
	postAcquireReset := vkResetFencesSwapchain(sc.device, 1, &sc.acquireFence)
	if postAcquireReset != vk.Success {
		return nil, false, fmt.Errorf("vulkan: vkResetFences (post-acquire) failed: %d", postAcquireReset)
	}

	// Store the current acquire index and semaphore for use in Submit.
	// Submit will signal acquireFences[currentAcquireIdx] when done.
	sc.currentAcquireIdx = acquireIdx
	sc.currentAcquireSem = acquireSem

	// Advance the semaphore rotation index for next frame
	sc.nextAcquireIdx = (sc.nextAcquireIdx + 1) % len(sc.acquireSemaphores)

	sc.currentImage = imageIndex
	sc.imageAcquired = true
	return sc.surfaceTextures[imageIndex], result == vk.SuboptimalKhr, nil
}

// present presents the current image to the screen.
func (sc *Swapchain) present(queue *Queue) error {
	if !sc.imageAcquired {
		return fmt.Errorf("vulkan: no image acquired to present")
	}

	// Use the present semaphore for the current image.
	// Submit signals this, and present waits on it.
	presentSem := sc.presentSemaphores[sc.currentImage]

	presentInfo := vk.PresentInfoKHR{
		SType:              vk.StructureTypePresentInfoKhr,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    &presentSem,
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

func vkGetFenceStatusSwapchain(d *Device, fence vk.Fence) vk.Result {
	return d.cmds.GetFenceStatus(d.handle, fence)
}

func vkDeviceWaitIdle(d *Device) vk.Result {
	return d.cmds.DeviceWaitIdle(d.handle)
}
