//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"image"
	"unsafe"

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
	acquireFence      vk.Fence // Post-acquire fence for frame pacing (Rust wgpu pattern)

	// BUG-WGPU-VK-006: Swapchain image layout tracking.
	// Tracks the current Vulkan image layout for each swapchain image. Used to
	// determine whether an explicit barrier to PRESENT_SRC_KHR is needed before
	// vkQueuePresentKHR. Set to UNDEFINED on acquire (per Vulkan spec), updated
	// to PRESENT_SRC_KHR when a render pass with that finalLayout targets the
	// swapchain image, and checked in present() to insert a defensive barrier
	// when the layout is not PRESENT_SRC_KHR (e.g., blit-only or offscreen paths).
	imageLayouts []vk.ImageLayout

	// barrierPool is a dedicated VkCommandPool for recording layout transition
	// barriers before present. Separate from user command pools to avoid
	// interference with encoder lifecycle. Created lazily on first barrier need.
	barrierPool vk.CommandPool

	// barrierFence synchronizes the barrier command buffer submission in
	// ensurePresentLayout. We must wait for the barrier to complete on the GPU
	// before resetting the command pool, otherwise the command buffer is still
	// pending (VUID-vkResetCommandPool-commandPool-00040). Created lazily
	// alongside barrierPool.
	barrierFence vk.Fence
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

// CurrentUsage returns 0 — Vulkan swapchain images are managed by the swapchain.
func (t *SwapchainTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *SwapchainTexture) AddPendingRef()                      {}
func (t *SwapchainTexture) DecPendingRef()                      {}

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
	if s.handle == 0 {
		return fmt.Errorf("vulkan: cannot create swapchain for null surface")
	}

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

	// BUG-WGPU-VK-006: Initialize per-image layout tracking.
	// All swapchain images start in UNDEFINED layout (Vulkan spec).
	imgLayouts := make([]vk.ImageLayout, len(images))
	for i := range imgLayouts {
		imgLayouts[i] = vk.ImageLayoutUndefined
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
		imageLayouts:       imgLayouts,
	}

	// Create post-acquire fence for frame pacing (Rust wgpu pattern).
	// vkAcquireNextImageKHR signals this fence when the image is ready.
	// We wait on it before rendering to sync with the presentation engine.
	// Critical for Windows where Vulkan uses DXGI swapchain internally.
	var acquireFence vk.Fence
	fenceInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
	}
	fenceResult := device.cmds.CreateFence(device.handle, &fenceInfo, nil, &acquireFence)
	if fenceResult == vk.Success {
		swapchain.acquireFence = acquireFence
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

	// Destroy post-acquire fence
	if sc.acquireFence != 0 {
		sc.device.cmds.DestroyFence(sc.device.handle, sc.acquireFence, nil)
		sc.acquireFence = 0
	}

	// BUG-WGPU-VK-006: Destroy barrier fence and command pool.
	if sc.barrierFence != 0 {
		sc.device.cmds.DestroyFence(sc.device.handle, sc.barrierFence, nil)
		sc.barrierFence = 0
	}
	if sc.barrierPool != 0 {
		sc.device.cmds.DestroyCommandPool(sc.device.handle, sc.barrierPool, nil)
		sc.barrierPool = 0
	}
	sc.imageLayouts = nil
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

	// Pass acquireFence to vkAcquireNextImageKHR for post-acquire frame pacing.
	// Rust wgpu: "This wait is very important on Windows to avoid bad frame pacing
	// where the Vulkan driver is using a DXGI swapchain" (issues #8310, #8354).
	fence := sc.acquireFence
	var imageIndex uint32
	result := vkAcquireNextImageKHR(sc.device, sc.handle, timeout, acquireSem, fence, &imageIndex)

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

	// Post-acquire fence wait: sync with presentation engine for proper frame pacing.
	// Rust wgpu: "very important on Windows to avoid bad frame pacing where the
	// Vulkan driver is using a DXGI swapchain" (issues #8310, #8354).
	// Previously removed due to Intel driver timeouts — re-enabled for testing
	// with updated drivers (2026-03).
	if fence != 0 {
		waitResult := sc.device.cmds.WaitForFences(sc.device.handle, 1, &fence, vk.True, timeout)
		if waitResult != vk.Success {
			// Non-fatal: if wait fails, continue without frame pacing.
			// This handles drivers that don't support post-acquire fence properly.
			_ = waitResult
		}
		_ = sc.device.cmds.ResetFences(sc.device.handle, 1, &fence)
	}

	// Store the current acquire index and semaphore for use in Submit.
	sc.currentAcquireIdx = acquireIdx
	sc.currentAcquireSem = acquireSem

	// Advance the semaphore rotation index for next frame
	sc.nextAcquireIdx = (sc.nextAcquireIdx + 1) % len(sc.acquireSemaphores)

	sc.currentImage = imageIndex
	sc.imageAcquired = true

	// BUG-WGPU-VK-006: Per Vulkan spec, acquired swapchain images are in
	// UNDEFINED layout regardless of previous usage. Track this so present()
	// can insert a barrier if no render pass transitions to PRESENT_SRC_KHR.
	if int(imageIndex) < len(sc.imageLayouts) {
		sc.imageLayouts[imageIndex] = vk.ImageLayoutUndefined
	}

	return sc.surfaceTextures[imageIndex], result == vk.SuboptimalKhr, nil
}

// present presents the current image to the screen.
//
// damageRects is an optional list of rectangles (physical pixels, top-left
// origin) indicating which surface regions changed this frame. When non-empty
// and the device supports VK_KHR_incremental_present, a VkPresentRegionsKHR
// structure is chained into VkPresentInfoKHR.PNext as a compositor hint.
// When empty or unsupported, the present path is identical to a full present.
func (sc *Swapchain) present(queue *Queue, damageRects []image.Rectangle) error {
	if !sc.imageAcquired {
		return fmt.Errorf("vulkan: no image acquired to present")
	}

	// BUG-WGPU-VK-006: Ensure the swapchain image is in PRESENT_SRC_KHR layout
	// before vkQueuePresentKHR. When a render pass directly targets the swapchain
	// image with finalLayout=PRESENT_SRC_KHR, the layout is already correct and
	// this is a no-op (zero overhead in the common case). When the image was used
	// differently (blit-only, offscreen-only, resolve target without PRESENT_SRC),
	// this inserts an explicit pipeline barrier to transition the layout.
	if err := sc.ensurePresentLayout(queue); err != nil {
		hal.Logger().Warn("vulkan: present layout transition failed",
			"err", err, "imageIndex", sc.currentImage)
		// Non-fatal: attempt present anyway. The validation layer will report
		// the layout mismatch, but many drivers tolerate it.
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

	// VK_KHR_incremental_present: chain damage rects as a compositor hint.
	// Stack-allocate up to 8 rects to avoid heap allocation in the common case.
	var (
		stackRects     [8]vk.RectLayerKHR
		presentRegion  vk.PresentRegionKHR
		presentRegions vk.PresentRegionsKHR
	)
	if len(damageRects) > 0 && sc.device.supportsIncrementalPresent {
		vkRects := stackRects[:0]
		for _, r := range damageRects {
			vkRects = append(vkRects, vk.RectLayerKHR{
				Offset: vk.Offset2D{X: int32(r.Min.X), Y: int32(r.Min.Y)},
				Extent: vk.Extent2D{Width: uint32(r.Dx()), Height: uint32(r.Dy())},
				Layer:  0,
			})
		}
		presentRegion = vk.PresentRegionKHR{
			RectangleCount: uint32(len(vkRects)),
			PRectangles:    &vkRects[0],
		}
		presentRegions = vk.PresentRegionsKHR{
			SType:          vk.StructureTypePresentRegionsKhr,
			SwapchainCount: 1,
			PRegions:       &presentRegion,
		}
		presentInfo.PNext = (*uintptr)(unsafe.Pointer(&presentRegions))
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

// SetImageLayout updates the tracked layout for a swapchain image.
// Called from BeginRenderPass when the render pass finalLayout is known for a
// swapchain color attachment. This allows present() to skip the barrier when
// the render pass already transitions to PRESENT_SRC_KHR.
//
// BUG-WGPU-VK-006: Without this tracking, present() would either always insert
// a barrier (unnecessary overhead) or never insert one (the bug).
func (sc *Swapchain) SetImageLayout(imageIndex uint32, layout vk.ImageLayout) {
	if int(imageIndex) < len(sc.imageLayouts) {
		sc.imageLayouts[imageIndex] = layout
	}
}

// ensurePresentLayout checks whether the current swapchain image needs an
// explicit layout transition to PRESENT_SRC_KHR before vkQueuePresentKHR.
//
// In the common case (render pass directly targets the swapchain image with
// finalLayout = PRESENT_SRC_KHR), the tracked layout already matches and this
// function returns immediately — zero overhead.
//
// When the tracked layout differs (blit-only path, offscreen-only, image never
// rendered to), a one-shot command buffer is recorded with a pipeline barrier
// and submitted to the queue. This matches Chrome/Dawn's approach for the same
// edge case. The extra vkQueueSubmit is the minimum cost to guarantee spec
// compliance.
//
// BUG-WGPU-VK-006: Fixes VUID-VkPresentInfoKHR-pImageIndices-01430.
func (sc *Swapchain) ensurePresentLayout(queue *Queue) error {
	idx := sc.currentImage
	if int(idx) >= len(sc.imageLayouts) {
		return nil // defensive: out of range
	}

	currentLayout := sc.imageLayouts[idx]
	if currentLayout == vk.ImageLayoutPresentSrcKhr {
		// Common case: render pass already transitioned. Nothing to do.
		return nil
	}

	// Need to transition. Create the barrier pool and fence lazily on first use.
	if sc.barrierPool == 0 {
		createInfo := vk.CommandPoolCreateInfo{
			SType:            vk.StructureTypeCommandPoolCreateInfo,
			Flags:            vk.CommandPoolCreateFlags(vk.CommandPoolCreateTransientBit | vk.CommandPoolCreateResetCommandBufferBit),
			QueueFamilyIndex: sc.device.graphicsFamily,
		}
		var pool vk.CommandPool
		result := sc.device.cmds.CreateCommandPool(sc.device.handle, &createInfo, nil, &pool)
		if result != vk.Success {
			return fmt.Errorf("vulkan: vkCreateCommandPool (barrier) failed: %d", result)
		}
		sc.device.setObjectName(vk.ObjectTypeCommandPool, uint64(pool), "PresentBarrierPool")
		sc.barrierPool = pool

		// Create the fence used to wait for barrier submission completion.
		// VUID-vkResetCommandPool-commandPool-00040 requires all command buffers
		// allocated from the pool to not be in pending state before reset.
		fenceInfo := vk.FenceCreateInfo{
			SType: vk.StructureTypeFenceCreateInfo,
		}
		var fence vk.Fence
		fenceResult := sc.device.cmds.CreateFence(sc.device.handle, &fenceInfo, nil, &fence)
		if fenceResult != vk.Success {
			return fmt.Errorf("vulkan: vkCreateFence (barrier) failed: %d", fenceResult)
		}
		sc.device.setObjectName(vk.ObjectTypeFence, uint64(fence), "PresentBarrierFence")
		sc.barrierFence = fence
	}

	// Allocate a one-shot command buffer from the barrier pool.
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        sc.barrierPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: 1,
	}
	var cmdBuf vk.CommandBuffer
	result := sc.device.cmds.AllocateCommandBuffers(sc.device.handle, &allocInfo, &cmdBuf)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkAllocateCommandBuffers (barrier) failed: %d", result)
	}

	// Begin recording.
	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	result = sc.device.cmds.BeginCommandBuffer(cmdBuf, &beginInfo)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkBeginCommandBuffer (barrier) failed: %d", result)
	}

	// Determine source access mask and pipeline stage based on the tracked layout.
	// The oldLayout must match what the GPU actually sees when this barrier executes.
	var srcAccess vk.AccessFlags
	var srcStage vk.PipelineStageFlags
	switch currentLayout {
	case vk.ImageLayoutColorAttachmentOptimal:
		srcAccess = vk.AccessFlags(vk.AccessColorAttachmentWriteBit)
		srcStage = vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)
	case vk.ImageLayoutTransferDstOptimal:
		srcAccess = vk.AccessFlags(vk.AccessTransferWriteBit)
		srcStage = vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	case vk.ImageLayoutTransferSrcOptimal:
		srcAccess = vk.AccessFlags(vk.AccessTransferReadBit)
		srcStage = vk.PipelineStageFlags(vk.PipelineStageTransferBit)
	default:
		// UNDEFINED or unknown: no prior access to synchronize.
		// OldLayout = UNDEFINED is safe here because either:
		// 1. The image was never written to this frame (content is undefined anyway)
		// 2. The image is in an untracked layout (conservative: may discard, but
		//    this path only fires when no render pass targeted the swapchain,
		//    meaning nothing meaningful was rendered to it)
		srcAccess = 0
		srcStage = vk.PipelineStageFlags(vk.PipelineStageTopOfPipeBit)
	}

	barrier := vk.ImageMemoryBarrier{
		SType:               vk.StructureTypeImageMemoryBarrier,
		SrcAccessMask:       srcAccess,
		DstAccessMask:       0, // Present engine does not need explicit access
		OldLayout:           currentLayout,
		NewLayout:           vk.ImageLayoutPresentSrcKhr,
		SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
		DstQueueFamilyIndex: vk.QueueFamilyIgnored,
		Image:               sc.images[idx],
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	sc.device.cmds.CmdPipelineBarrier(
		cmdBuf,
		srcStage,
		vk.PipelineStageFlags(vk.PipelineStageBottomOfPipeBit),
		0,      // dependencyFlags
		0, nil, // memory barriers
		0, nil, // buffer barriers
		1, &barrier,
	)

	// End recording.
	result = sc.device.cmds.EndCommandBuffer(cmdBuf)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkEndCommandBuffer (barrier) failed: %d", result)
	}

	// Submit the barrier command buffer with the barrier fence. No semaphores —
	// this runs after the user's submit (which already waited on acquire and
	// signaled present semaphores). The barrier just needs to complete before
	// vkQueuePresentKHR, which is guaranteed by Vulkan's implicit ordering of
	// vkQueueSubmit calls on the same queue.
	//
	// The fence is required so we can wait for GPU completion before resetting
	// the command pool (VUID-vkResetCommandPool-commandPool-00040).
	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    &cmdBuf,
	}
	result = sc.device.cmds.QueueSubmit(queue.handle, 1, &submitInfo, sc.barrierFence)
	if result != vk.Success {
		return fmt.Errorf("vulkan: vkQueueSubmit (barrier) failed: %d", result)
	}

	// Update tracked layout.
	sc.imageLayouts[idx] = vk.ImageLayoutPresentSrcKhr

	// Wait for the barrier submission to complete on the GPU before resetting
	// the command pool. Without this wait, the command buffer is still pending
	// and vkResetCommandPool violates VUID-vkResetCommandPool-commandPool-00040.
	//
	// This wait is synchronous but only occurs when the barrier fires (uncommon
	// case: blit-only or offscreen paths where no render pass transitions the
	// swapchain image to PRESENT_SRC_KHR). In the common case (render pass with
	// finalLayout = PRESENT_SRC_KHR), ensurePresentLayout returns early above.
	const barrierTimeout = uint64(1_000_000_000) // 1 second
	sc.device.cmds.WaitForFences(sc.device.handle, 1, &sc.barrierFence, vk.True, barrierTimeout)
	sc.device.cmds.ResetFences(sc.device.handle, 1, &sc.barrierFence)

	// Reset the command pool so the buffer can be reused next frame.
	// Safe now because WaitForFences guarantees the command buffer is complete.
	sc.device.cmds.ResetCommandPool(sc.device.handle, sc.barrierPool, 0)

	hal.Logger().Debug("vulkan: inserted PRESENT_SRC_KHR barrier",
		"imageIndex", idx, "oldLayout", currentLayout)

	return nil
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
