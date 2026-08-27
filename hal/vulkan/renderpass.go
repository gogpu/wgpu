//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// ColorAttachmentKeyEntry describes one color attachment slot in a render pass
// cache key. A zero-value entry (Format == FormatUndefined) represents an
// absent slot -- matching Rust wgpu's Option<ColorAttachmentKey>.
type ColorAttachmentKeyEntry struct {
	Format      vk.Format
	LoadOp      vk.AttachmentLoadOp
	StoreOp     vk.AttachmentStoreOp
	FinalLayout vk.ImageLayout
	HasResolve  bool // true when this slot has an MSAA resolve target
}

// RenderPassKey uniquely identifies a render pass configuration.
// Used for caching VkRenderPass objects.
//
// Supports up to MaxColorAttachments (8) color attachments, matching the
// WebGPU specification and Rust wgpu-hal RenderPassKey.
type RenderPassKey struct {
	Colors     [hal.MaxColorAttachments]ColorAttachmentKeyEntry
	ColorCount int // number of active color attachment slots

	DepthFormat    vk.Format
	DepthLoadOp    vk.AttachmentLoadOp
	DepthStoreOp   vk.AttachmentStoreOp
	StencilLoadOp  vk.AttachmentLoadOp
	StencilStoreOp vk.AttachmentStoreOp

	SampleCount vk.SampleCountFlagBits
}

// FramebufferKey uniquely identifies a framebuffer configuration.
// Supports up to MaxTotalAttachments (17) views: 8 colors + 8 resolves + 1 depth.
//
// View order matches the attachment order in the render pass:
//   - color[0], resolve[0] (if MSAA), color[1], resolve[1] (if MSAA), ...
//   - depth/stencil (last, if present)
type FramebufferKey struct {
	RenderPass vk.RenderPass
	Views      [hal.MaxTotalAttachments]vk.ImageView
	ViewCount  int
	Width      uint32
	Height     uint32
}

// RenderPassCache caches VkRenderPass and VkFramebuffer objects.
// This is critical for performance and compatibility with Intel drivers
// that don't properly support VK_KHR_dynamic_rendering.
type RenderPassCache struct {
	device       vk.Device
	cmds         *vk.Commands
	mu           sync.RWMutex
	renderPasses map[RenderPassKey]vk.RenderPass
	framebuffers map[FramebufferKey]vk.Framebuffer
}

// NewRenderPassCache creates a new render pass cache.
func NewRenderPassCache(device vk.Device, cmds *vk.Commands) *RenderPassCache {
	return &RenderPassCache{
		device:       device,
		cmds:         cmds,
		renderPasses: make(map[RenderPassKey]vk.RenderPass),
		framebuffers: make(map[FramebufferKey]vk.Framebuffer),
	}
}

// GetOrCreateRenderPass returns a cached render pass or creates a new one.
func (c *RenderPassCache) GetOrCreateRenderPass(key RenderPassKey) (vk.RenderPass, error) {
	// Try read lock first
	c.mu.RLock()
	if rp, ok := c.renderPasses[key]; ok {
		c.mu.RUnlock()
		return rp, nil
	}
	c.mu.RUnlock()

	// Need to create - use write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if rp, ok := c.renderPasses[key]; ok {
		return rp, nil
	}

	// Create render pass
	rp, err := c.createRenderPass(key)
	if err != nil {
		return 0, err
	}

	c.renderPasses[key] = rp
	return rp, nil
}

// GetOrCreateFramebuffer returns a cached framebuffer or creates a new one.
func (c *RenderPassCache) GetOrCreateFramebuffer(key FramebufferKey) (vk.Framebuffer, error) {
	// Try read lock first
	c.mu.RLock()
	if fb, ok := c.framebuffers[key]; ok {
		c.mu.RUnlock()
		return fb, nil
	}
	c.mu.RUnlock()

	// Need to create - use write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if fb, ok := c.framebuffers[key]; ok {
		return fb, nil
	}

	// Create framebuffer
	fb, err := c.createFramebuffer(key)
	if err != nil {
		return 0, err
	}

	c.framebuffers[key] = fb
	return fb, nil
}

// createRenderPass creates a new VkRenderPass.
//
// Attachment order (indices must match framebuffer view order):
//
//	for each color slot i in 0..ColorCount:
//	  - color[i] attachment (MSAA or single-sample)
//	  - resolve[i] attachment (only if HasResolve && SampleCount > 1)
//	- depth/stencil (if DepthFormat != Undefined)
//
// The resolve attachment references array is always the same length as
// the color references array. Slots without resolve use VK_ATTACHMENT_UNUSED.
// This matches Rust wgpu-hal make_render_pass (vulkan/device.rs:76-231).
func (c *RenderPassCache) createRenderPass(key RenderPassKey) (vk.RenderPass, error) {
	var attachments [hal.MaxTotalAttachments]vk.AttachmentDescription
	attachmentCount := 0

	var colorRefs [hal.MaxColorAttachments]vk.AttachmentReference
	var resolveRefs [hal.MaxColorAttachments]vk.AttachmentReference
	hasAnyResolve := false

	unused := vk.AttachmentReference{
		Attachment: vk.AttachmentUnused,
		Layout:     vk.ImageLayoutUndefined,
	}

	for i := 0; i < key.ColorCount; i++ {
		cat := key.Colors[i]
		if cat.Format == vk.FormatUndefined {
			// Absent slot — Vulkan allows gaps in color attachment arrays.
			colorRefs[i] = unused
			resolveRefs[i] = unused
			continue
		}

		hasMSAAResolve := cat.HasResolve && key.SampleCount > vk.SampleCountFlagBits(1)

		colorFinalLayout := cat.FinalLayout
		colorStoreOp := cat.StoreOp

		if hasMSAAResolve {
			// With MSAA resolve, the MSAA color attachment is intermediate:
			// - FinalLayout = ColorAttachmentOptimal (not presented directly)
			// - StoreOp = DontCare (resolved content goes to resolve target)
			colorFinalLayout = vk.ImageLayoutColorAttachmentOptimal
			colorStoreOp = vk.AttachmentStoreOpDontCare
		}

		// When LoadOp is Load, InitialLayout must match the actual image layout
		// so Vulkan preserves existing contents. With Undefined, the driver may
		// discard the image data even when LoadOpLoad is specified.
		colorInitialLayout := vk.ImageLayoutUndefined
		if cat.LoadOp == vk.AttachmentLoadOpLoad {
			colorInitialLayout = colorFinalLayout
		}

		colorRefs[i] = vk.AttachmentReference{
			Attachment: uint32(attachmentCount),
			Layout:     vk.ImageLayoutColorAttachmentOptimal,
		}
		attachments[attachmentCount] = vk.AttachmentDescription{
			Format:         cat.Format,
			Samples:        key.SampleCount,
			LoadOp:         cat.LoadOp,
			StoreOp:        colorStoreOp,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  colorInitialLayout,
			FinalLayout:    colorFinalLayout,
		}
		attachmentCount++

		// Resolve attachment (immediately after this color attachment).
		//
		// BUG-WGPU-MSAA-RESOLVE-001: The resolve target MUST use LoadOp=Clear so that
		// pixels without MSAA fragment coverage are filled with the clear color instead
		// of retaining stale content from the previous frame (trail artifacts).
		if hasMSAAResolve {
			resolveRefs[i] = vk.AttachmentReference{
				Attachment: uint32(attachmentCount),
				Layout:     vk.ImageLayoutColorAttachmentOptimal,
			}
			attachments[attachmentCount] = vk.AttachmentDescription{
				Format:         cat.Format,
				Samples:        vk.SampleCountFlagBits(1), // Resolve target is always single-sample
				LoadOp:         vk.AttachmentLoadOpClear,
				StoreOp:        vk.AttachmentStoreOpStore,
				StencilLoadOp:  vk.AttachmentLoadOpDontCare,
				StencilStoreOp: vk.AttachmentStoreOpDontCare,
				InitialLayout:  vk.ImageLayoutUndefined,
				FinalLayout:    cat.FinalLayout, // The resolve target gets the "real" final layout
			}
			attachmentCount++
			hasAnyResolve = true
		} else {
			resolveRefs[i] = unused
		}
	}

	// Depth/stencil attachment (last attachment)
	var depthRef *vk.AttachmentReference
	if key.DepthFormat != vk.FormatUndefined {
		depthInitialLayout := vk.ImageLayoutUndefined
		if key.DepthLoadOp == vk.AttachmentLoadOpLoad {
			depthInitialLayout = vk.ImageLayoutDepthStencilAttachmentOptimal
		}
		attachments[attachmentCount] = vk.AttachmentDescription{
			Format:         key.DepthFormat,
			Samples:        key.SampleCount,
			LoadOp:         key.DepthLoadOp,
			StoreOp:        key.DepthStoreOp,
			StencilLoadOp:  key.StencilLoadOp,
			StencilStoreOp: key.StencilStoreOp,
			InitialLayout:  depthInitialLayout,
			FinalLayout:    vk.ImageLayoutDepthStencilAttachmentOptimal,
		}
		depthRef = &vk.AttachmentReference{
			Attachment: uint32(attachmentCount),
			Layout:     vk.ImageLayoutDepthStencilAttachmentOptimal,
		}
		attachmentCount++
	}

	// Subpass
	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    uint32(key.ColorCount),
		PDepthStencilAttachment: depthRef,
	}
	if key.ColorCount > 0 {
		subpass.PColorAttachments = &colorRefs[0]
		// Vulkan spec requires pResolveAttachments to be NULL or an array of
		// the same length as pColorAttachments. When any slot has resolve, we
		// pass the full array; slots without resolve use VK_ATTACHMENT_UNUSED.
		if hasAnyResolve {
			subpass.PResolveAttachments = &resolveRefs[0]
		}
	}

	// No explicit subpass dependencies — Vulkan handles implicit ones.
	// This matches Rust wgpu which doesn't add explicit dependencies.
	attachmentSlice := attachments[:attachmentCount]
	createInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: uint32(attachmentCount),
		SubpassCount:    1,
		PSubpasses:      &subpass,
		DependencyCount: 0,
		PDependencies:   nil,
	}
	if attachmentCount > 0 {
		createInfo.PAttachments = &attachmentSlice[0]
	}

	var renderPass vk.RenderPass
	result := c.cmds.CreateRenderPass(c.device, &createInfo, nil, &renderPass)
	runtime.KeepAlive(attachmentSlice)
	runtime.KeepAlive(colorRefs)
	runtime.KeepAlive(resolveRefs)
	runtime.KeepAlive(depthRef)
	runtime.KeepAlive(createInfo)
	runtime.KeepAlive(subpass)

	if result != vk.Success {
		return 0, &vkError{code: result, op: "vkCreateRenderPass"}
	}
	if renderPass == 0 {
		return 0, &vkError{code: -1, op: "vkCreateRenderPass returned NULL handle"}
	}

	c.setObjectName(vk.ObjectTypeRenderPass, uint64(renderPass),
		fmt.Sprintf("RenderPass(%d)", len(c.renderPasses)))
	return renderPass, nil
}

// createFramebuffer creates a new VkFramebuffer.
// The view order in key.Views MUST match the attachment order in the render pass:
//
//	color[0], resolve[0]?, color[1], resolve[1]?, ..., depth/stencil?
func (c *RenderPassCache) createFramebuffer(key FramebufferKey) (vk.Framebuffer, error) {
	views := key.Views[:key.ViewCount]

	createInfo := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      key.RenderPass,
		AttachmentCount: uint32(key.ViewCount),
		Width:           key.Width,
		Height:          key.Height,
		Layers:          1,
	}
	if key.ViewCount > 0 {
		createInfo.PAttachments = &views[0]
	}

	var framebuffer vk.Framebuffer
	result := c.cmds.CreateFramebuffer(c.device, &createInfo, nil, &framebuffer)
	runtime.KeepAlive(views)
	runtime.KeepAlive(createInfo)

	if result != vk.Success {
		return 0, &vkError{code: result, op: "vkCreateFramebuffer"}
	}
	if framebuffer == 0 {
		return 0, &vkError{code: -1, op: "vkCreateFramebuffer returned NULL handle"}
	}

	c.setObjectName(vk.ObjectTypeFramebuffer, uint64(framebuffer),
		fmt.Sprintf("Framebuffer(%d)", len(c.framebuffers)))
	return framebuffer, nil
}

// setObjectName labels a Vulkan object for debug/validation.
// No-op when VK_EXT_debug_utils is not available.
func (c *RenderPassCache) setObjectName(objectType vk.ObjectType, handle uint64, name string) {
	if !c.cmds.HasDebugUtils() || handle == 0 {
		return
	}
	nameBytes := append([]byte(name), 0)
	nameInfo := vk.DebugUtilsObjectNameInfoEXT{
		SType:        vk.StructureTypeDebugUtilsObjectNameInfoExt,
		ObjectType:   objectType,
		ObjectHandle: handle,
		PObjectName:  uintptr(unsafe.Pointer(&nameBytes[0])),
	}
	_ = c.cmds.SetDebugUtilsObjectNameEXT(c.device, &nameInfo)
	runtime.KeepAlive(nameBytes)
}

// InvalidateFramebuffer removes framebuffers from cache that reference the given image view.
// Called when swapchain is recreated.
func (c *RenderPassCache) InvalidateFramebuffer(imageView vk.ImageView) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, fb := range c.framebuffers {
		found := false
		for i := 0; i < key.ViewCount; i++ {
			if key.Views[i] == imageView {
				found = true
				break
			}
		}
		if found {
			c.cmds.DestroyFramebuffer(c.device, fb, nil)
			delete(c.framebuffers, key)
		}
	}
}

// Destroy releases all cached resources.
func (c *RenderPassCache) Destroy() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, fb := range c.framebuffers {
		c.cmds.DestroyFramebuffer(c.device, fb, nil)
	}
	c.framebuffers = nil

	for _, rp := range c.renderPasses {
		c.cmds.DestroyRenderPass(c.device, rp, nil)
	}
	c.renderPasses = nil
}

// vkError represents a Vulkan error.
type vkError struct {
	code vk.Result
	op   string
}

func (e *vkError) Error() string {
	return e.op + " failed: " + vkResultToString(e.code)
}

func vkResultToString(r vk.Result) string {
	switch r {
	case vk.Success:
		return "VK_SUCCESS"
	case vk.ErrorOutOfHostMemory:
		return "VK_ERROR_OUT_OF_HOST_MEMORY"
	case vk.ErrorOutOfDeviceMemory:
		return "VK_ERROR_OUT_OF_DEVICE_MEMORY"
	case vk.ErrorInitializationFailed:
		return "VK_ERROR_INITIALIZATION_FAILED"
	default:
		return "VK_ERROR_UNKNOWN"
	}
}

//nolint:unused // Helper for render pass format conversion
func formatToVkForRenderPass(format gputypes.TextureFormat) vk.Format {
	return textureFormatToVk(format)
}
