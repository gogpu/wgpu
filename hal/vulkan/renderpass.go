// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"sync"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/types"
)

// RenderPassKey uniquely identifies a render pass configuration.
// Used for caching VkRenderPass objects.
type RenderPassKey struct {
	ColorFormat     vk.Format
	ColorLoadOp     vk.AttachmentLoadOp
	ColorStoreOp    vk.AttachmentStoreOp
	DepthFormat     vk.Format
	DepthLoadOp     vk.AttachmentLoadOp
	DepthStoreOp    vk.AttachmentStoreOp
	StencilLoadOp   vk.AttachmentLoadOp
	StencilStoreOp  vk.AttachmentStoreOp
	SampleCount     vk.SampleCountFlagBits
	ColorFinalLayout vk.ImageLayout
}

// FramebufferKey uniquely identifies a framebuffer configuration.
type FramebufferKey struct {
	RenderPass vk.RenderPass
	ImageView  vk.ImageView
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
func (c *RenderPassCache) createRenderPass(key RenderPassKey) (vk.RenderPass, error) {
	attachments := make([]vk.AttachmentDescription, 0, 2)
	colorRef := vk.AttachmentReference{
		Attachment: vk.AttachmentUnused,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}
	var depthRef *vk.AttachmentReference

	// Color attachment
	if key.ColorFormat != vk.FormatUndefined {
		attachments = append(attachments, vk.AttachmentDescription{
			Format:         key.ColorFormat,
			Samples:        key.SampleCount,
			LoadOp:         key.ColorLoadOp,
			StoreOp:        key.ColorStoreOp,
			StencilLoadOp:  vk.AttachmentLoadOpDontCare,
			StencilStoreOp: vk.AttachmentStoreOpDontCare,
			InitialLayout:  vk.ImageLayoutUndefined,
			FinalLayout:    key.ColorFinalLayout,
		})
		colorRef.Attachment = 0
	}

	// Depth/stencil attachment
	if key.DepthFormat != vk.FormatUndefined {
		attachments = append(attachments, vk.AttachmentDescription{
			Format:         key.DepthFormat,
			Samples:        key.SampleCount,
			LoadOp:         key.DepthLoadOp,
			StoreOp:        key.DepthStoreOp,
			StencilLoadOp:  key.StencilLoadOp,
			StencilStoreOp: key.StencilStoreOp,
			InitialLayout:  vk.ImageLayoutUndefined,
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
	if result != vk.Success {
		return 0, &vkError{code: result, op: "vkCreateRenderPass"}
	}

	return renderPass, nil
}

// createFramebuffer creates a new VkFramebuffer.
func (c *RenderPassCache) createFramebuffer(key FramebufferKey) (vk.Framebuffer, error) {
	createInfo := vk.FramebufferCreateInfo{
		SType:           vk.StructureTypeFramebufferCreateInfo,
		RenderPass:      key.RenderPass,
		AttachmentCount: 1,
		PAttachments:    &key.ImageView,
		Width:           key.Width,
		Height:          key.Height,
		Layers:          1,
	}

	var framebuffer vk.Framebuffer
	result := c.cmds.CreateFramebuffer(c.device, &createInfo, nil, &framebuffer)
	if result != vk.Success {
		return 0, &vkError{code: result, op: "vkCreateFramebuffer"}
	}

	return framebuffer, nil
}

// InvalidateFramebuffer removes a framebuffer from cache.
// Called when swapchain is recreated.
func (c *RenderPassCache) InvalidateFramebuffer(imageView vk.ImageView) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, fb := range c.framebuffers {
		if key.ImageView == imageView {
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

// Helper function to convert texture format to VkFormat for render pass key
func formatToVkForRenderPass(format types.TextureFormat) vk.Format {
	return textureFormatToVk(format)
}
