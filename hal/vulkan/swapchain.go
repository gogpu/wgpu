//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"fmt"
	"image"
	"math"
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

	// broken is set after a synchronization, layout, or presentation failure.
	// A broken swapchain cannot safely reuse its binary semaphores; callers must
	// reconfigure or destroy it before attempting another frame.
	broken     bool
	failureErr error
	destroyed  bool
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

func (sc *Swapchain) markBroken(err error) {
	if err == nil {
		err = fmt.Errorf("vulkan: swapchain synchronization failed")
	}
	sc.broken = true
	sc.failureErr = err
	sc.imageAcquired = false
}

func (sc *Swapchain) stateError(operation string) error {
	if sc == nil || sc.destroyed {
		return fmt.Errorf("vulkan: cannot %s a destroyed swapchain", operation)
	}
	if sc.broken {
		if sc.failureErr != nil {
			return fmt.Errorf("vulkan: cannot %s a broken swapchain: %w", operation, sc.failureErr)
		}
		return fmt.Errorf("vulkan: cannot %s a broken swapchain", operation)
	}
	return nil
}

type swapchainSurfaceSnapshot struct {
	capabilities vk.SurfaceCapabilitiesKHR
	formats      []vk.SurfaceFormatKHR
	presentModes []vk.PresentModeKHR
}

// textureFormatForSurfacePair projects the format/color-space pairs that this
// backend can configure. Rust wgpu v29 exposes RGBA16Float only for linear
// scRGB; accepting the same VkFormat with another color space would advertise
// a capability that createSwapchain cannot honor.
func textureFormatForSurfacePair(surfaceFormat vk.SurfaceFormatKHR) gputypes.TextureFormat {
	format := vkFormatToTextureFormat(surfaceFormat.Format)
	if format == gputypes.TextureFormatRGBA16Float && surfaceFormat.ColorSpace != vk.ColorSpaceExtendedSrgbLinearExt {
		return gputypes.TextureFormatUndefined
	}
	return format
}

func (snapshot swapchainSurfaceSnapshot) formatFor(requested gputypes.TextureFormat) (vk.SurfaceFormatKHR, error) {
	requestedVk := textureFormatToVk(requested)
	if requestedVk == vk.FormatUndefined {
		return vk.SurfaceFormatKHR{}, fmt.Errorf("vulkan: unsupported surface format %v", requested)
	}
	preferredColorSpace := vk.ColorSpaceSrgbNonlinearKhr
	if requested == gputypes.TextureFormatRGBA16Float {
		// Match Rust wgpu v29's scRGB swapchain pairing.
		preferredColorSpace = vk.ColorSpaceExtendedSrgbLinearExt
	}

	var fallback *vk.SurfaceFormatKHR
	for i := range snapshot.formats {
		format := snapshot.formats[i]
		if format.Format != requestedVk || textureFormatForSurfacePair(format) != requested {
			continue
		}
		if format.ColorSpace == preferredColorSpace {
			return format, nil
		}
		if fallback == nil {
			fallback = &format
		}
	}
	if fallback == nil {
		return vk.SurfaceFormatKHR{}, fmt.Errorf("vulkan: surface does not support format %v", requested)
	}
	return *fallback, nil
}

func (snapshot swapchainSurfaceSnapshot) presentModeFor(requested hal.PresentMode) (vk.PresentModeKHR, error) {
	requestedVk, ok := presentModeToVkChecked(requested)
	if !ok {
		return 0, fmt.Errorf("vulkan: unsupported present mode %v", requested)
	}
	for _, mode := range snapshot.presentModes {
		if mode == requestedVk {
			return mode, nil
		}
	}
	return 0, fmt.Errorf("vulkan: surface does not support present mode %v", requested)
}

func compositeAlphaFor(flags vk.CompositeAlphaFlagsKHR, requested hal.CompositeAlphaMode) (vk.CompositeAlphaFlagBitsKHR, error) {
	available := func(flag vk.CompositeAlphaFlagBitsKHR) bool {
		return vk.Flags(flags)&vk.Flags(flag) != 0
	}

	if requested == hal.CompositeAlphaModeAuto {
		// Prefer opaque only when the surface reports it; otherwise use the
		// first Vulkan-supported mode in a stable order.
		for _, mode := range []vk.CompositeAlphaFlagBitsKHR{
			vk.CompositeAlphaOpaqueBitKhr,
			vk.CompositeAlphaPreMultipliedBitKhr,
			vk.CompositeAlphaPostMultipliedBitKhr,
			vk.CompositeAlphaInheritBitKhr,
		} {
			if available(mode) {
				return mode, nil
			}
		}
		return 0, fmt.Errorf("vulkan: surface reports no composite alpha mode")
	}

	requestedFlag, ok := mapCompositeAlphaToVk(requested)
	if !ok || !available(requestedFlag) {
		return 0, fmt.Errorf("vulkan: surface does not support composite alpha mode %v", requested)
	}
	return requestedFlag, nil
}

func mapCompositeAlphaToVk(mode hal.CompositeAlphaMode) (vk.CompositeAlphaFlagBitsKHR, bool) {
	switch mode {
	case hal.CompositeAlphaModeOpaque:
		return vk.CompositeAlphaOpaqueBitKhr, true
	case hal.CompositeAlphaModePremultiplied:
		return vk.CompositeAlphaPreMultipliedBitKhr, true
	case hal.CompositeAlphaModeUnpremultiplied:
		return vk.CompositeAlphaPostMultipliedBitKhr, true
	case hal.CompositeAlphaModeInherit:
		return vk.CompositeAlphaInheritBitKhr, true
	default:
		return 0, false
	}
}

func presentModeToVkChecked(mode hal.PresentMode) (vk.PresentModeKHR, bool) {
	switch mode {
	case hal.PresentModeImmediate:
		return vk.PresentModeImmediateKhr, true
	case hal.PresentModeMailbox:
		return vk.PresentModeMailboxKhr, true
	case hal.PresentModeFifo:
		return vk.PresentModeFifoKhr, true
	case hal.PresentModeFifoRelaxed:
		return vk.PresentModeFifoRelaxedKhr, true
	default:
		return 0, false
	}
}

func swapchainImageUsage(usage gputypes.TextureUsage, supported vk.ImageUsageFlags) (vk.ImageUsageFlags, error) {
	if usage.ContainsUnknownBits() {
		return 0, fmt.Errorf("vulkan: surface configuration has unknown texture usage bits")
	}
	// Surface textures are renderable by contract, even when callers omit the
	// bit in a legacy descriptor. Every requested usage is still checked against
	// the driver-reported flags before creating the swapchain.
	effective := usage | gputypes.TextureUsageRenderAttachment
	requested := textureUsageToVk(effective)
	if vk.Flags(supported)&vk.Flags(requested) != vk.Flags(requested) {
		return 0, fmt.Errorf("vulkan: surface does not support requested texture usage %v", effective)
	}
	return requested, nil
}

func validateSurfaceSnapshot(snapshot swapchainSurfaceSnapshot, config *hal.SurfaceConfiguration) (vk.SurfaceFormatKHR, vk.PresentModeKHR, vk.CompositeAlphaFlagBitsKHR, vk.ImageUsageFlags, error) {
	if config == nil {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, fmt.Errorf("vulkan: surface configuration is nil")
	}
	if len(snapshot.formats) == 0 {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, fmt.Errorf("vulkan: surface returned no formats")
	}
	if len(snapshot.presentModes) == 0 {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, fmt.Errorf("vulkan: surface returned no present modes")
	}
	format, err := snapshot.formatFor(config.Format)
	if err != nil {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, err
	}
	presentMode, err := snapshot.presentModeFor(config.PresentMode)
	if err != nil {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, err
	}
	alphaMode, err := compositeAlphaFor(snapshot.capabilities.SupportedCompositeAlpha, config.AlphaMode)
	if err != nil {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, err
	}
	usage, err := swapchainImageUsage(config.Usage, snapshot.capabilities.SupportedUsageFlags)
	if err != nil {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, err
	}
	if snapshot.capabilities.CurrentTransform == 0 {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, fmt.Errorf("vulkan: surface returned no current transform")
	}
	if vk.Flags(snapshot.capabilities.SupportedTransforms)&vk.Flags(snapshot.capabilities.CurrentTransform) == 0 {
		return vk.SurfaceFormatKHR{}, 0, 0, 0, fmt.Errorf("vulkan: surface current transform is not supported")
	}
	return format, presentMode, alphaMode, usage, nil
}

func querySwapchainSurfaceSnapshot(instance *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR) (swapchainSurfaceSnapshot, error) {
	var capabilities vk.SurfaceCapabilitiesKHR
	result := vkGetPhysicalDeviceSurfaceCapabilitiesKHR(instance, device, surface, &capabilities)
	if result != vk.Success {
		return swapchainSurfaceSnapshot{}, surfaceQueryError("vkGetPhysicalDeviceSurfaceCapabilitiesKHR", result)
	}

	formats, err := querySwapchainFormats(instance, device, surface)
	if err != nil {
		return swapchainSurfaceSnapshot{}, err
	}
	presentModes, err := querySwapchainPresentModes(instance, device, surface)
	if err != nil {
		return swapchainSurfaceSnapshot{}, err
	}
	return swapchainSurfaceSnapshot{
		capabilities: capabilities,
		formats:      formats,
		presentModes: presentModes,
	}, nil
}

func surfaceQueryError(operation string, result vk.Result) error {
	return mapVulkanResult(operation, result)
}

const undefinedSurfaceExtent = ^uint32(0)

// selectSwapchainExtent follows Vulkan's two surface extent modes. A defined
// current extent is compositor-owned; UINT32_MAX lets the application choose
// an extent within the advertised range.
func selectSwapchainExtent(capabilities vk.SurfaceCapabilitiesKHR, requestedWidth, requestedHeight uint32) (vk.Extent2D, error) {
	if capabilities.MinImageExtent.Width > capabilities.MaxImageExtent.Width ||
		capabilities.MinImageExtent.Height > capabilities.MaxImageExtent.Height {
		return vk.Extent2D{}, fmt.Errorf("vulkan: surface returned invalid image extent range")
	}
	widthDefined := capabilities.CurrentExtent.Width != undefinedSurfaceExtent
	heightDefined := capabilities.CurrentExtent.Height != undefinedSurfaceExtent
	if widthDefined != heightDefined {
		return vk.Extent2D{}, fmt.Errorf("vulkan: surface returned partially defined current extent")
	}

	var extent vk.Extent2D
	if widthDefined {
		extent = capabilities.CurrentExtent
		if extent.Width < capabilities.MinImageExtent.Width || extent.Width > capabilities.MaxImageExtent.Width ||
			extent.Height < capabilities.MinImageExtent.Height || extent.Height > capabilities.MaxImageExtent.Height {
			return vk.Extent2D{}, fmt.Errorf("vulkan: surface current extent is outside its advertised range")
		}
	} else {
		extent = vk.Extent2D{
			Width:  clampUint32(requestedWidth, capabilities.MinImageExtent.Width, capabilities.MaxImageExtent.Width),
			Height: clampUint32(requestedHeight, capabilities.MinImageExtent.Height, capabilities.MaxImageExtent.Height),
		}
	}
	if extent.Width == 0 || extent.Height == 0 {
		return vk.Extent2D{}, hal.ErrZeroArea
	}
	return extent, nil
}

func querySwapchainFormats(instance *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR) ([]vk.SurfaceFormatKHR, error) {
	return querySwapchainFormatsWith(func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result {
		return instance.cmds.GetPhysicalDeviceSurfaceFormatsKHR(device, surface, count, formats)
	})
}

func querySwapchainFormatsWith(query func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result) ([]vk.SurfaceFormatKHR, error) {
	return queryRequiredSwapchainValues(
		"vkGetPhysicalDeviceSurfaceFormatsKHR",
		"vulkan: surface returned no formats",
		query,
	)
}

func queryRequiredSwapchainValues[T any](operation, emptyMessage string, query func(count *uint32, values *T) vk.Result) ([]T, error) {
	for attempt := 0; attempt < 2; attempt++ {
		var count uint32
		result := query(&count, nil)
		if result != vk.Success && result != vk.Incomplete {
			return nil, surfaceQueryError(operation+" (count)", result)
		}
		if count == 0 {
			return nil, errors.New(emptyMessage)
		}
		values := make([]T, count)
		returned := count
		result = query(&returned, &values[0])
		if result != vk.Success && result != vk.Incomplete {
			return nil, surfaceQueryError(operation, result)
		}
		if result == vk.Incomplete || returned > uint32(len(values)) {
			continue
		}
		if returned == 0 {
			return nil, errors.New(emptyMessage)
		}
		return values[:returned], nil
	}
	return nil, fmt.Errorf("vulkan: %s returned an unstable count", operation)
}

func querySwapchainPresentModes(instance *Instance, device vk.PhysicalDevice, surface vk.SurfaceKHR) ([]vk.PresentModeKHR, error) {
	return querySwapchainPresentModesWith(func(count *uint32, modes *vk.PresentModeKHR) vk.Result {
		return instance.cmds.GetPhysicalDeviceSurfacePresentModesKHR(device, surface, count, modes)
	})
}

func querySwapchainPresentModesWith(query func(count *uint32, modes *vk.PresentModeKHR) vk.Result) ([]vk.PresentModeKHR, error) {
	return queryRequiredSwapchainValues(
		"vkGetPhysicalDeviceSurfacePresentModesKHR",
		"vulkan: surface returned no present modes",
		query,
	)
}

func querySwapchainImages(device *Device, swapchain vk.SwapchainKHR) ([]vk.Image, error) {
	return querySwapchainImagesWith(func(count *uint32, images *vk.Image) vk.Result {
		return vkGetSwapchainImagesKHR(device, swapchain, count, images)
	})
}

func querySwapchainImagesWith(query func(count *uint32, images *vk.Image) vk.Result) ([]vk.Image, error) {
	return queryRequiredSwapchainValues(
		"vkGetSwapchainImagesKHR",
		"vulkan: swapchain returned no images",
		query,
	)
}

// createSwapchain creates a new swapchain for the surface.
//
//nolint:maintidx // Vulkan swapchain setup requires many sequential steps
func (s *Surface) createSwapchain(device *Device, config *hal.SurfaceConfiguration) error {
	if s == nil || s.handle == 0 {
		return fmt.Errorf("vulkan: cannot create swapchain for null surface")
	}
	if config == nil {
		return fmt.Errorf("vulkan: cannot create swapchain with nil configuration")
	}

	if s.instance == nil || device == nil || device.instance == nil || device.instance != s.instance {
		return fmt.Errorf("vulkan: device does not belong to surface instance")
	}

	// Query every capability used by VkSwapchainCreateInfoKHR and validate the
	// requested configuration before changing an existing swapchain.
	snapshot, err := querySwapchainSurfaceSnapshot(s.instance, device.physicalDevice, s.handle)
	if err != nil {
		return err
	}
	selectedFormat, presentMode, compositeAlpha, imageUsage, err := validateSurfaceSnapshot(snapshot, config)
	if err != nil {
		return err
	}
	capabilities := snapshot.capabilities
	policy := swapchainPolicyForSurface(s)
	preTransform, err := policy.preTransform(capabilities)
	if err != nil {
		return err
	}

	// Determine image count
	if capabilities.MinImageCount == 0 {
		return fmt.Errorf("vulkan: surface returned invalid minimum image count")
	}
	if capabilities.MaxImageArrayLayers == 0 {
		return fmt.Errorf("vulkan: surface does not support one image array layer")
	}
	imageCount := capabilities.MinImageCount
	if imageCount < math.MaxUint32 {
		imageCount++
	}
	if capabilities.MaxImageCount > 0 && imageCount > capabilities.MaxImageCount {
		imageCount = capabilities.MaxImageCount
	}
	if imageCount < capabilities.MinImageCount || imageCount == 0 {
		return fmt.Errorf("vulkan: surface returned invalid image count range")
	}

	extent, err := selectSwapchainExtent(capabilities, config.Width, config.Height)
	if err != nil {
		return err
	}

	// Log surface capabilities for HiDPI diagnostics (BUG-VK-HIDPI-001).
	hal.Logger().Debug("vulkan: surface capabilities",
		"requestedWidth", config.Width,
		"requestedHeight", config.Height,
		"currentExtent", [2]uint32{capabilities.CurrentExtent.Width, capabilities.CurrentExtent.Height},
		"minExtent", [2]uint32{capabilities.MinImageExtent.Width, capabilities.MinImageExtent.Height},
		"maxExtent", [2]uint32{capabilities.MaxImageExtent.Width, capabilities.MaxImageExtent.Height},
	)

	// Warn if the driver clamped the extent to different dimensions than
	// requested. This commonly happens on X11 HiDPI where the compositor
	// reports physical pixels that differ from the application's logical
	// pixels. Downstream code (e.g., MSAA textures) should use
	// Surface.ActualExtent() to match the real swapchain size.
	if extent.Width != config.Width || extent.Height != config.Height {
		hal.Logger().Warn("vulkan: swapchain extent clamped by driver",
			"requestedWidth", config.Width,
			"requestedHeight", config.Height,
			"actualWidth", extent.Width,
			"actualHeight", extent.Height,
		)
	}

	vkFormat := selectedFormat.Format

	// Wait for an existing swapchain before passing it as OldSwapchain. This
	// keeps its semaphores and views intact if creation or image enumeration
	// fails; reconfiguration is transactional until the replacement is ready.
	var oldSwapchain *Swapchain
	var oldSwapchainHandle vk.SwapchainKHR
	if s.swapchain != nil {
		if s.swapchain.device != device {
			return fmt.Errorf("vulkan: cannot reconfigure a surface with a different device")
		}
		oldSwapchain = s.swapchain
		oldSwapchainHandle = oldSwapchain.handle
		if result := vkDeviceWaitIdle(device); result != vk.Success {
			return fmt.Errorf("vulkan: vkDeviceWaitIdle before reconfigure failed: %d", result)
		}
	}

	// Create swapchain (passing old handle for seamless transition)
	createInfo := vk.SwapchainCreateInfoKHR{
		SType:            vk.StructureTypeSwapchainCreateInfoKhr,
		Surface:          s.handle,
		MinImageCount:    imageCount,
		ImageFormat:      vkFormat,
		ImageColorSpace:  selectedFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       imageUsage,
		ImageSharingMode: vk.SharingModeExclusive,
		PreTransform:     preTransform,
		CompositeAlpha:   compositeAlpha,
		PresentMode:      presentMode,
		Clipped:          vk.True,
		OldSwapchain:     oldSwapchainHandle,
	}

	var swapchainHandle vk.SwapchainKHR
	result := vkCreateSwapchainKHR(device, &createInfo, nil, &swapchainHandle)
	if result != vk.Success {
		return swapchainCreateError(result)
	}

	// Get swapchain images
	images, err := querySwapchainImages(device, swapchainHandle)
	if err != nil {
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return err
	}
	swapchainImageCount := uint32(len(images))

	// Log actual swapchain creation result (wgpu#185: HiDPI diagnostic).
	hal.Logger().Info("vulkan: swapchain created",
		"extent", fmt.Sprintf("%dx%d", extent.Width, extent.Height),
		"images", swapchainImageCount,
		"format", vkFormat,
		"presentMode", presentMode,
	)

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
	acquireSemaphores := make([]vk.Semaphore, len(images))
	presentSemaphores := make([]vk.Semaphore, len(images))

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
	if fenceResult != vk.Success {
		_ = swapchain.destroyResources()
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return fmt.Errorf("vulkan: vkCreateFence (acquire) failed: %d", fenceResult)
	}
	swapchain.acquireFence = acquireFence

	// Link swapchain to surface textures
	for _, tex := range surfaceTextures {
		tex.swapchain = swapchain
	}

	oldDevice := s.device
	if err := device.registerConfiguredSurface(s); err != nil {
		if destroyErr := swapchain.destroyWithError(); destroyErr != nil {
			hal.Logger().Error("vulkan: failed to clean up unregistered swapchain", "error", destroyErr)
		}
		return fmt.Errorf("vulkan: register configured surface: %w", err)
	}

	// Retire the old swapchain only after the replacement has fully succeeded.
	// Its handle remains valid until its views and synchronization primitives are
	// released, as required by Vulkan's OldSwapchain transition rules.
	if err := retireOldSwapchain(device, oldSwapchain, oldSwapchainHandle); err != nil {
		_ = swapchain.destroyResources()
		vkDestroySwapchainKHR(device, swapchainHandle, nil)
		return err
	}

	s.swapchain = swapchain
	s.device = device
	if oldDevice != nil && oldDevice != device {
		oldDevice.unregisterConfiguredSurface(s)
	}

	return nil
}

func retireOldSwapchain(device *Device, oldSwapchain *Swapchain, oldHandle vk.SwapchainKHR) error {
	if oldSwapchain == nil {
		return nil
	}
	if err := oldSwapchain.destroyResourcesAfterIdle(); err != nil {
		return fmt.Errorf("vulkan: retire old swapchain: %w", err)
	}
	if oldHandle != 0 {
		vkDestroySwapchainKHR(device, oldHandle, nil)
	}
	oldSwapchain.handle = 0
	oldSwapchain.destroyed = true
	if device.queue == nil {
		return nil
	}
	device.queue.mu.Lock()
	if device.queue.activeSwapchain == oldSwapchain {
		device.queue.activeSwapchain = nil
		device.queue.acquireUsed = false
	}
	device.queue.mu.Unlock()
	return nil
}

// releaseSyncResources releases synchronization primitives after the device is
// idle. It keeps the idle wait explicit so callers can propagate failures and
// reconfiguration can remain transactional.
func (sc *Swapchain) releaseSyncResources() error {
	if sc == nil || sc.device == nil {
		return nil
	}
	if result := vkDeviceWaitIdle(sc.device); result != vk.Success {
		return fmt.Errorf("vulkan: vkDeviceWaitIdle before releasing swapchain synchronization failed: %d", result)
	}
	sc.releaseSyncResourcesAfterIdle()
	return nil
}

func (sc *Swapchain) releaseSyncResourcesAfterIdle() {
	if sc == nil || sc.device == nil {
		return
	}
	for i, sem := range sc.acquireSemaphores {
		if sem != 0 {
			vkDestroySemaphore(sc.device, sem, nil)
			sc.acquireSemaphores[i] = 0
		}
	}
	sc.acquireSemaphores = nil

	for i, sem := range sc.presentSemaphores {
		if sem != 0 {
			vkDestroySemaphore(sc.device, sem, nil)
			sc.presentSemaphores[i] = 0
		}
	}
	sc.presentSemaphores = nil
	sc.acquireFenceValues = nil
	sc.imageAcquired = false
}

// destroyResources destroys swapchain resources after waiting for the device.
func (sc *Swapchain) destroyResources() error {
	if sc == nil || sc.device == nil {
		return nil
	}
	if err := sc.releaseSyncResources(); err != nil {
		return err
	}
	return sc.destroyResourcesAfterIdle()
}

// destroyResourcesAfterIdle is used by transactional reconfiguration after a
// successful vkDeviceWaitIdle. It performs no unchecked synchronization wait.
func (sc *Swapchain) destroyResourcesAfterIdle() error {
	if sc == nil || sc.device == nil {
		return nil
	}
	sc.releaseSyncResourcesAfterIdle()

	for _, view := range sc.imageViews {
		if view != 0 {
			vkDestroyImageViewSwapchain(sc.device, view, nil)
		}
	}
	sc.imageViews = nil
	sc.images = nil
	sc.surfaceTextures = nil

	if sc.acquireFence != 0 {
		sc.device.cmds.DestroyFence(sc.device.handle, sc.acquireFence, nil)
		sc.acquireFence = 0
	}

	if sc.barrierFence != 0 {
		sc.device.cmds.DestroyFence(sc.device.handle, sc.barrierFence, nil)
		sc.barrierFence = 0
	}
	if sc.barrierPool != 0 {
		// vkDestroyCommandPool is a void Vulkan command; there is no result to
		// propagate. DeviceWaitIdle above establishes the required lifetime
		// guarantee before this teardown.
		sc.device.cmds.DestroyCommandPool(sc.device.handle, sc.barrierPool, nil)
		sc.barrierPool = 0
	}
	sc.imageLayouts = nil
	sc.currentAcquireSem = 0
	sc.currentAcquireIdx = 0
	sc.nextAcquireIdx = 0
	return nil
}

// Destroy destroys the swapchain completely. It is idempotent. The public HAL
// method keeps its historical no-result signature; internal owners use
// destroyWithError when they need to preserve a failed synchronization result.
func (sc *Swapchain) Destroy() {
	if err := sc.destroyWithError(); err != nil {
		hal.Logger().Error("vulkan: failed to destroy swapchain", "error", err)
	}
}

func (sc *Swapchain) destroyWithError() error {
	if sc == nil || sc.destroyed {
		return nil
	}
	if sc.device == nil {
		sc.destroyed = true
		sc.handle = 0
		return nil
	}
	if err := sc.destroyResources(); err != nil {
		return err
	}
	if sc.handle != 0 {
		vkDestroySwapchainKHR(sc.device, sc.handle, nil)
		sc.handle = 0
	}
	sc.destroyed = true
	sc.failureErr = nil
	return nil
}

func swapchainCreateError(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorInitializationFailed:
		// Rust wgpu-hal treats initialization failure as a lost surface: on
		// common WSI implementations it means the native surface can no longer
		// produce a swapchain and retrying the same handle cannot recover.
		return hal.ErrSurfaceLost
	case vk.ErrorNativeWindowInUseKhr:
		return fmt.Errorf("vulkan: vkCreateSwapchainKHR failed: native window is already in use")
	default:
		return mapVulkanResult("vkCreateSwapchainKHR", result)
	}
}

func (sc *Swapchain) destroyAfterIdle() {
	if sc == nil {
		return
	}
	_ = sc.destroyResourcesAfterIdle()
	if sc.handle != 0 && sc.device != nil {
		vkDestroySwapchainKHR(sc.device, sc.handle, nil)
	}
	sc.handle = 0
	sc.destroyed = true
	sc.failureErr = nil
	sc.device = nil
	sc.surface = nil
}

func (sc *Swapchain) abandonDeviceResources() {
	if sc == nil {
		return
	}
	for _, texture := range sc.surfaceTextures {
		if texture != nil {
			texture.handle = 0
			texture.view = 0
			texture.swapchain = nil
		}
	}
	sc.handle = 0
	sc.images = nil
	sc.imageViews = nil
	sc.acquireSemaphores = nil
	sc.acquireFenceValues = nil
	sc.presentSemaphores = nil
	sc.surfaceTextures = nil
	sc.imageLayouts = nil
	sc.acquireFence = 0
	sc.barrierFence = 0
	sc.barrierPool = 0
	sc.currentAcquireSem = 0
	sc.imageAcquired = false
	sc.device = nil
	sc.surface = nil
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
	if err := sc.stateError("acquire an image from"); err != nil {
		return nil, false, err
	}
	if sc.imageAcquired {
		return nil, false, fmt.Errorf("vulkan: image already acquired")
	}
	if sc.device == nil || sc.handle == 0 {
		return nil, false, hal.ErrSurfaceLost
	}
	if len(sc.acquireSemaphores) == 0 || len(sc.acquireFenceValues) != len(sc.acquireSemaphores) {
		err := fmt.Errorf("vulkan: swapchain acquire synchronization is unavailable")
		sc.markBroken(err)
		return nil, false, err
	}
	if len(sc.surfaceTextures) == 0 || len(sc.images) != len(sc.surfaceTextures) || len(sc.presentSemaphores) != len(sc.surfaceTextures) || len(sc.imageLayouts) != len(sc.surfaceTextures) {
		err := fmt.Errorf("vulkan: swapchain image state is inconsistent")
		sc.markBroken(err)
		return nil, false, err
	}

	// Timeout for acquire - match wgpu-core's FRAME_TIMEOUT_MS = 1000
	// This is the proven timeout that works across drivers.
	// On timeout, caller should retry once (wgpu pattern).
	const requestedTimeout = uint64(1_000_000_000) // 1000ms = 1 second
	policy := swapchainPolicyForSurface(sc.surface)
	timeout := policy.acquireTimeout(requestedTimeout)

	// Get the acquire semaphore from the rotating pool.
	acquireIdx := sc.nextAcquireIdx
	if acquireIdx < 0 || acquireIdx >= len(sc.acquireSemaphores) {
		err := fmt.Errorf("vulkan: acquire semaphore index %d is out of range", acquireIdx)
		sc.markBroken(err)
		return nil, false, err
	}
	acquireSem := sc.acquireSemaphores[acquireIdx]
	if acquireSem == 0 {
		err := fmt.Errorf("vulkan: acquire semaphore %d has been destroyed", acquireIdx)
		sc.markBroken(err)
		return nil, false, err
	}

	// Pre-acquire wait: ensure the GPU has consumed this semaphore from
	// a previous frame's Submit before we pass it to vkAcquireNextImageKHR again.
	// Without this, the semaphore may still have pending operations,
	// violating VUID-vkAcquireNextImageKHR-semaphore-01779.
	// See: wgpu-hal/src/vulkan/swapchain/native.rs — previously_used_submission_index
	if prevValue := sc.acquireFenceValues[acquireIdx]; prevValue > 0 {
		if err := sc.device.timelineFence.waitForValue(
			sc.device.cmds, sc.device.handle, prevValue, timeout,
		); err != nil {
			sc.markBroken(fmt.Errorf("vulkan: wait for acquire semaphore %d: %w", acquireIdx, err))
			return nil, false, sc.failureErr
		}
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
		if result == vk.ErrorOutOfDateKhr {
			sc.markBroken(hal.ErrSurfaceOutdated)
		}
		return nil, false, hal.ErrSurfaceOutdated
	case vk.ErrorSurfaceLostKhr:
		// Surface destroyed (e.g., Wayland compositor killed it).
		// (wgpu: returns Err(Lost))
		sc.markBroken(hal.ErrSurfaceLost)
		return nil, false, hal.ErrSurfaceLost
	case vk.ErrorDeviceLost:
		sc.markBroken(hal.ErrDeviceLost)
		return nil, false, hal.ErrDeviceLost
	default:
		err := mapVulkanResult("vkAcquireNextImageKHR", result)
		sc.markBroken(err)
		return nil, false, err
	}

	// Post-acquire fence wait: sync with presentation engine for proper frame pacing.
	// Rust wgpu: "very important on Windows to avoid bad frame pacing where the
	// Vulkan driver is using a DXGI swapchain" (issues #8310, #8354).
	// Previously removed due to Intel driver timeouts — re-enabled for testing
	// with updated drivers (2026-03).
	if fence != 0 {
		waitResult := sc.device.cmds.WaitForFences(sc.device.handle, 1, &fence, vk.True, timeout)
		if waitResult != vk.Success {
			sc.markBroken(mapVulkanResult("vkWaitForFences after acquire", waitResult))
			return nil, false, sc.failureErr
		}
		resetResult := sc.device.cmds.ResetFences(sc.device.handle, 1, &fence)
		if resetResult != vk.Success {
			sc.markBroken(mapVulkanResult("vkResetFences after acquire", resetResult))
			return nil, false, sc.failureErr
		}
	}
	if imageIndex >= uint32(len(sc.surfaceTextures)) {
		err := fmt.Errorf("vulkan: acquired image index %d is out of range", imageIndex)
		sc.markBroken(err)
		return nil, false, err
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
	sc.imageLayouts[imageIndex] = vk.ImageLayoutUndefined

	return sc.surfaceTextures[imageIndex], policy.reportSuboptimal(result == vk.SuboptimalKhr), nil
}

// present presents the current image to the screen.
//
// damageRects is an optional list of rectangles (physical pixels, top-left
// origin) indicating which surface regions changed this frame. When non-empty
// and the device supports VK_KHR_incremental_present, a VkPresentRegionsKHR
// structure is chained into VkPresentInfoKHR.PNext as a compositor hint.
// When empty or unsupported, the present path is identical to a full present.
func (sc *Swapchain) present(queue *Queue, damageRects []image.Rectangle) error {
	if err := sc.stateError("present from"); err != nil {
		return err
	}
	if !sc.imageAcquired {
		return fmt.Errorf("vulkan: no image acquired to present")
	}
	if queue == nil || queue.device != sc.device {
		return fmt.Errorf("vulkan: present queue does not belong to swapchain device")
	}
	if queue.activeSwapchain != sc || !queue.acquireUsed {
		err := fmt.Errorf("vulkan: no successful swapchain submission is ready for presentation")
		sc.markBroken(err)
		return err
	}
	if sc.handle == 0 || sc.currentImage >= uint32(len(sc.presentSemaphores)) || sc.currentImage >= uint32(len(sc.images)) || sc.images[sc.currentImage] == 0 {
		err := hal.ErrSurfaceLost
		sc.markBroken(err)
		return hal.ErrSurfaceLost
	}
	presentSem := sc.presentSemaphores[sc.currentImage]
	if presentSem == 0 {
		err := fmt.Errorf("vulkan: present semaphore for image %d has been destroyed", sc.currentImage)
		sc.markBroken(err)
		return err
	}

	// BUG-WGPU-VK-006: Ensure the swapchain image is in PRESENT_SRC_KHR layout
	// before vkQueuePresentKHR. When a render pass directly targets the swapchain
	// image with finalLayout=PRESENT_SRC_KHR, the layout is already correct and
	// this is a no-op (zero overhead in the common case). When the image was used
	// differently (blit-only, offscreen-only, resolve target without PRESENT_SRC),
	// this inserts an explicit pipeline barrier to transition the layout.
	if err := sc.ensurePresentLayout(queue); err != nil {
		sc.markBroken(fmt.Errorf("vulkan: present layout transition failed: %w", err))
		hal.Logger().Error("vulkan: present layout transition failed",
			"err", err, "imageIndex", sc.currentImage)
		// The image's semaphore/layout state is no longer safe to reuse. A
		// reconfigure or destroy must drain the device before cleanup.
		return sc.failureErr
	}

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
		if swapchainPolicyForSurface(sc.surface).reportSuboptimal(true) {
			hal.Logger().Debug("vulkan: suboptimal swapchain present", "imageIndex", sc.currentImage)
		}
		return nil
	case vk.ErrorOutOfDateKhr:
		sc.markBroken(hal.ErrSurfaceOutdated)
		return hal.ErrSurfaceOutdated
	case vk.ErrorSurfaceLostKhr:
		sc.markBroken(hal.ErrSurfaceLost)
		return hal.ErrSurfaceLost
	case vk.ErrorDeviceLost:
		sc.markBroken(hal.ErrDeviceLost)
		return hal.ErrDeviceLost
	default:
		err := mapVulkanResult("vkQueuePresentKHR", result)
		sc.markBroken(err)
		return err
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
		return fmt.Errorf("vulkan: swapchain image layout index %d is out of range", idx)
	}
	if sc.device == nil || queue == nil || queue.device != sc.device {
		return fmt.Errorf("vulkan: invalid queue for present layout transition")
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
			sc.device.cmds.DestroyCommandPool(sc.device.handle, pool, nil)
			sc.barrierPool = 0
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

	// Wait for the barrier submission to complete on the GPU before resetting
	// the command pool. Without this wait, the command buffer is still pending
	// and vkResetCommandPool violates VUID-vkResetCommandPool-commandPool-00040.
	//
	// This wait is synchronous but only occurs when the barrier fires (uncommon
	// case: blit-only or offscreen paths where no render pass transitions the
	// swapchain image to PRESENT_SRC_KHR). In the common case (render pass with
	// finalLayout = PRESENT_SRC_KHR), ensurePresentLayout returns early above.
	const barrierTimeout = uint64(1_000_000_000) // 1 second
	waitResult := sc.device.cmds.WaitForFences(sc.device.handle, 1, &sc.barrierFence, vk.True, barrierTimeout)
	if waitResult != vk.Success {
		return fmt.Errorf("vulkan: vkWaitForFences (barrier) failed: %d", waitResult)
	}
	resetResult := sc.device.cmds.ResetFences(sc.device.handle, 1, &sc.barrierFence)
	if resetResult != vk.Success {
		return fmt.Errorf("vulkan: vkResetFences (barrier) failed: %d", resetResult)
	}

	// Reset the command pool so the buffer can be reused next frame.
	// Safe now because WaitForFences guarantees the command buffer is complete.
	resetPoolResult := sc.device.cmds.ResetCommandPool(sc.device.handle, sc.barrierPool, 0)
	if resetPoolResult != vk.Success {
		return fmt.Errorf("vulkan: vkResetCommandPool (barrier) failed: %d", resetPoolResult)
	}

	// Update tracked layout only after the barrier completed and its command
	// buffer was safely recycled.
	sc.imageLayouts[idx] = vk.ImageLayoutPresentSrcKhr

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
