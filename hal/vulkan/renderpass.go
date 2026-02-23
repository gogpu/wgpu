// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// RenderPassKey uniquely identifies a render pass configuration.
// Used for caching VkRenderPass objects.
type RenderPassKey struct {
	ColorFormat      vk.Format
	ColorLoadOp      vk.AttachmentLoadOp
	ColorStoreOp     vk.AttachmentStoreOp
	DepthFormat      vk.Format
	DepthLoadOp      vk.AttachmentLoadOp
	DepthStoreOp     vk.AttachmentStoreOp
	StencilLoadOp    vk.AttachmentLoadOp
	StencilStoreOp   vk.AttachmentStoreOp
	SampleCount      vk.SampleCountFlagBits
	ColorFinalLayout vk.ImageLayout
	HasResolve       bool // true when MSAA resolve target is present
}

// FramebufferKey uniquely identifies a framebuffer configuration.
// Supports multiple attachments for MSAA (color + resolve + depth/stencil).
type FramebufferKey struct {
	RenderPass  vk.RenderPass
	ColorView   vk.ImageView // MSAA color view or single-sample color view
	ResolveView vk.ImageView // Resolve target (0 if no MSAA)
	DepthView   vk.ImageView // Depth/stencil view (0 if none)
	Width       uint32
	Height      uint32
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
// Attachment order (indices must match framebuffer view order):
//   - 0: color (always, MSAA or single-sample)
//   - 1: resolve (only if HasResolve && SampleCount > 1)
//   - next: depth/stencil (if DepthFormat != Undefined)
func (c *RenderPassCache) createRenderPass(key RenderPassKey) (vk.RenderPass, error) {
	attachments := make([]vk.AttachmentDescription, 0, 3)
	colorRef := vk.AttachmentReference{
		Attachment: vk.AttachmentUnused,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	var resolveRef *vk.AttachmentReference
	var depthRef *vk.AttachmentReference

	hasMSAAResolve := key.HasResolve && key.SampleCount > vk.SampleCountFlagBits(1)

	// Color attachment (attachment 0)
	if key.ColorFormat != vk.FormatUndefined {
		colorFinalLayout := key.ColorFinalLayout
		colorStoreOp := key.ColorStoreOp

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
		if key.ColorLoadOp == vk.AttachmentLoadOpLoad {
			colorInitialLayout = colorFinalLayout
		}

		attachments = append(attachments, vk.AttachmentDescription{
			Format:         key.ColorFormat,
			Samples:        key.SampleCount,
			LoadOp:         key.ColorLoadOp,
			StoreOp:        colorStoreOp,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  colorInitialLayout,
			FinalLayout:    colorFinalLayout,
		})
		colorRef.Attachment = 0
	}

	// Resolve attachment (attachment 1, only for MSAA)
	if hasMSAAResolve && key.ColorFormat != vk.FormatUndefined {
		attachments = append(attachments, vk.AttachmentDescription{
			Format:         key.ColorFormat,
			Samples:        vk.SampleCountFlagBits(1), // Resolve target is always single-sample
			LoadOp:         vk.AttachmentLoadOpDontCare,
			StoreOp:        vk.AttachmentStoreOpStore,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    key.ColorFinalLayout, // The resolve target gets the "real" final layout
		})
		resolveRef = &vk.AttachmentReference{
			Attachment: uint32(len(attachments) - 1),
			Layout:     vk.ImageLayoutColorAttachmentOptimal,
		}
	}

	// Depth/stencil attachment (last attachment)
	if key.DepthFormat != vk.FormatUndefined {
		depthInitialLayout := vk.ImageLayoutUndefined
		if key.DepthLoadOp == vk.AttachmentLoadOpLoad {
			depthInitialLayout = vk.ImageLayoutDepthStencilAttachmentOptimal
		}
		attachments = append(attachments, vk.AttachmentDescription{
			Format:         key.DepthFormat,
			Samples:        key.SampleCount,
			LoadOp:         key.DepthLoadOp,
			StoreOp:        key.DepthStoreOp,
			StencilLoadOp:  key.StencilLoadOp,
			StencilStoreOp: key.StencilStoreOp,
			InitialLayout:  depthInitialLayout,
			FinalLayout:    vk.ImageLayoutDepthStencilAttachmentOptimal,
		})
		depthRef = &vk.AttachmentReference{
			Attachment: uint32(len(attachments) - 1),
			Layout:     vk.ImageLayoutDepthStencilAttachmentOptimal,
		}
	}

	// Subpass
	subpass := vk.SubpassDescription{
		PipelineBindPoint:       vk.PipelineBindPointGraphics,
		ColorAttachmentCount:    0,
		PDepthStencilAttachment: depthRef,
		PResolveAttachments:     resolveRef,
	}
	if colorRef.Attachment != vk.AttachmentUnused {
		subpass.ColorAttachmentCount = 1
		subpass.PColorAttachments = &colorRef
	}

	// No explicit subpass dependencies - Vulkan handles implicit ones.
	// This matches Rust wgpu which doesn't add explicit dependencies.
	createInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: uint32(len(attachments)),
		SubpassCount:    1,
		PSubpasses:      &subpass,
		DependencyCount: 0, // No explicit dependencies (matches Rust wgpu)
		PDependencies:   nil,
	}
	if len(attachments) > 0 {
		createInfo.PAttachments = &attachments[0]
	}

	var renderPass vk.RenderPass
	result := c.cmds.CreateRenderPass(c.device, &createInfo, nil, &renderPass)
	runtime.KeepAlive(attachments)
	runtime.KeepAlive(colorRef)
	runtime.KeepAlive(resolveRef)
	runtime.KeepAlive(depthRef)
	runtime.KeepAlive(createInfo)

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
// The view order MUST match the attachment order in the render pass:
//   - ColorView (always)
//   - ResolveView (only if non-zero, for MSAA resolve)
//   - DepthView (only if non-zero, for depth/stencil)
func (c *RenderPassCache) createFramebuffer(key FramebufferKey) (vk.Framebuffer, error) {
	views := make([]vk.ImageView, 0, 3)
	if key.ColorView != 0 {
		views = append(views, key.ColorView)
	}
	if key.ResolveView != 0 {
		views = append(views, key.ResolveView)
	}
	if key.DepthView != 0 {
		views = append(views, key.DepthView)
	}

	createInfo := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      key.RenderPass,
		AttachmentCount: uint32(len(views)),
		Width:           key.Width,
		Height:          key.Height,
		Layers:          1,
	}
	if len(views) > 0 {
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
		if key.ColorView == imageView || key.ResolveView == imageView || key.DepthView == imageView {
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
