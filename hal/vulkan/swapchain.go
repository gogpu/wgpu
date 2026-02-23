// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
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
	acquireSemaphores  []vk.Semaphore
	acquireFenceValues []uint64 // fence value when each acquire semaphore was last consumed by Submit
	nextAcquireIdx     int

	// Present semaphores - one per swapchain image (known after acquire).
	presentSemaphores []vk.Semaphore
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
	format    gputypes.TextureFormat
	size      Extent3D
}

// Destroy implements hal.Texture.
func (t *SwapchainTexture) Destroy() {
	// Swapchain textures are owned by the swapchain, not destroyed individually
}

// NativeHandle returns the raw VkImage handle as uintptr.
func (t *SwapchainTexture) NativeHandle() uintptr {
	return uintptr(t.handle)
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

	// Use config dimensions as primary source (matching Rust wgpu-hal behavior).
	// CurrentExtent from the driver is used only for clamping to the valid range.
	// Ref: wgpu-hal/src/vulkan/swapchain/native.rs:189-197
	extent := vk.Extent2D{
		Width:  config.Width,
		Height: config.Height,
	}

	// Clamp to driver-reported range when CurrentExtent is defined.
	// CurrentExtent of 0xFFFFFFFF means the surface size is determined by the swapchain.
	if capabilities.CurrentExtent.Width != 0xFFFFFFFF {
		extent.Width = clampUint32(extent.Width, capabilities.MinImageExtent.Width, capabilities.MaxImageExtent.Width)
		extent.Height = clampUint32(extent.Height, capabilities.MinImageExtent.Height, capabilities.MaxImageExtent.Height)
	}

	// Zero extent means the window is minimized -- skip swapchain creation.
	if extent.Width == 0 || extent.Height == 0 {
		return hal.ErrZeroArea
	}

	// Convert format
	vkFormat := textureFormatToVk(config.Format)

	// Convert present mode
	presentMode := presentModeToVk(config.PresentMode)

	// Convert usage
	imageUsage := vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit)
	if config.Usage&gputypes.TextureUsageCopySrc != 0 {
		imageUsage |= vk.ImageUsageFlags(vk.ImageUsageTransferSrcBit)
	}
	if config.Usage&gputypes.TextureUsageCopyDst != 0 {
		imageUsage |= vk.ImageUsageFlags(vk.ImageUsageTransferDstBit)
	}

	// Handle old swapchain - destroy resources (semaphores + image views) BEFORE creating new.
	// Using destroyResources() instead of releaseSyncResources() ensures image views from
	// the old swapchain are properly cleaned up, preventing "VkImageView has not been
	// destroyed" validation errors on device destruction.
	var oldSwapchain vk.SwapchainKHR
	if s.swapchain != nil {
		oldSwapchain = s.swapchain.handle
		// Destroy semaphores AND image views BEFORE creating new swapchain.
		// This does vkDeviceWaitIdle + destroy semaphores + destroy image views,
		// but NOT the swapchain handle (destroyed after new one is created).
		s.swapchain.destroyResources()
	}

	// Create swapchain (passing old handle for seamless transition)
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

	// Destroy old swapchain AFTER creating new (Vulkan requirement)
	if oldSwapchain != 0 {
		vkDestroySwapchainKHR(device, oldSwapchain, nil)
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

	// Label swapchain images and views for debug/validation (VK-VAL-002).
	for i, img := range images {
		device.setObjectName(vk.ObjectTypeImage, uint64(img),
			fmt.Sprintf("SwapchainImage(%d)", i))
		device.setObjectName(vk.ObjectTypeImageView, uint64(imageViews[i]),
			fmt.Sprintf("SwapchainView(%d)", i))
	}
	device.setObjectName(vk.ObjectTypeSwapchainKhr, uint64(swapchainHandle), "Swapchain")

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

	// Label acquire semaphores for debug/validation.
	for i, sem := range acquireSemaphores {
		device.setObjectName(vk.ObjectTypeSemaphore, uint64(sem),
			fmt.Sprintf("AcquireSemaphore(%d)", i))
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

	// Label present semaphores for debug/validation.
	for i, sem := range presentSemaphores {
		device.setObjectName(vk.ObjectTypeSemaphore, uint64(sem),
			fmt.Sprintf("PresentSemaphore(%d)", i))
	}

	// VK-IMPL-004: acquireFenceValues tracks the submission fence value when each
	// acquire semaphore was last consumed by Submit/SubmitForPresent. The pre-acquire
	// wait in acquireNextImage() uses this to ensure the GPU has finished before
	// reusing the semaphore (required by VUID-vkAcquireNextImageKHR-semaphore-01779).

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
		handle:             swapchainHandle,
		surface:            s,
		device:             device,
		images:             images,
		imageViews:         imageViews,
		format:             vkFormat,
		extent:             extent,
		presentMode:        presentMode,
		acquireSemaphores:  acquireSemaphores,
		acquireFenceValues: make([]uint64, len(acquireSemaphores)),
		nextAcquireIdx:     0,
		presentSemaphores:  presentSemaphores,
		surfaceTextures:    surfaceTextures,
	}

	// Link swapchain to surface textures
	for _, tex := range surfaceTextures {
		tex.swapchain = swapchain
	}

	s.swapchain = swapchain
	s.device = device

	return nil
}

// releaseSyncResources releases synchronization primitives (semaphores) BEFORE
// creating a new swapchain. This must be called before vkCreateSwapchainKHR
// when reconfiguring, as semaphores may be in pending state.
// Does NOT destroy the swapchain handle - that's done after creating the new one.
func (sc *Swapchain) releaseSyncResources() {
	if sc.device == nil {
		return
	}

	// Wait for device idle before destroying semaphores.
	// This is required because semaphores may be in pending state.
	// TODO: For better responsiveness, implement render thread architecture
	// like Ebiten (separate threads for events, game logic, rendering).
	vkDeviceWaitIdle(sc.device)

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

	// Reset state
	sc.imageAcquired = false
}

// destroyResources destroys swapchain resources (image views) after the
// swapchain handle has been destroyed or replaced.
func (sc *Swapchain) destroyResources() {
	if sc.device == nil {
		return
	}

	// Release sync resources if not already done
	sc.releaseSyncResources()

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

	// Timeout for acquire - match wgpu-core's FRAME_TIMEOUT_MS = 1000
	// This is the proven timeout that works across drivers.
	// On timeout, caller should retry once (wgpu pattern).
	const timeout = uint64(1_000_000_000) // 1000ms = 1 second

	// Get the acquire semaphore from the rotating pool.
	acquireIdx := sc.nextAcquireIdx
	acquireSem := sc.acquireSemaphores[acquireIdx]

	// Pre-acquire wait: ensure the GPU has consumed this semaphore from
	// a previous frame's Submit before we pass it to vkAcquireNextImageKHR again.
	// Without this, the semaphore may still have pending operations,
	// violating VUID-vkAcquireNextImageKHR-semaphore-01779.
	// See: wgpu-hal/src/vulkan/swapchain/native.rs — previously_used_submission_index
	if prevValue := sc.acquireFenceValues[acquireIdx]; prevValue > 0 {
		_ = sc.device.timelineFence.waitForValue(
			sc.device.cmds, sc.device.handle, prevValue, timeout,
		)
	}

	// Post-acquire fence wait is not implemented — it's a Windows/Intel optimization
	// for DXGI frame pacing (see wgpu issues #8310, #8354), not required for correctness.
	var imageIndex uint32
	result := vkAcquireNextImageKHR(sc.device, sc.handle, timeout, acquireSem, vk.Fence(0), &imageIndex)

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

	// NOTE: Post-acquire fence wait removed.
	// wgpu uses this for Windows/Intel DXGI swapchain frame pacing,
	// but it causes timeouts on other drivers. We rely on semaphore
	// synchronization which is sufficient for GPU-side correctness.

	// Store the current acquire index and semaphore for use in Submit.
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

// clampUint32 returns v clamped to [lo, hi].
func clampUint32(v, lo, hi uint32) uint32 {
	return max(lo, min(v, hi))
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

func vkDeviceWaitIdle(d *Device) vk.Result {
	return d.cmds.DeviceWaitIdle(d.handle)
}
