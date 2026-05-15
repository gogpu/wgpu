//go:build js && wasm

package wgpu

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
)

// CommandEncoder records GPU commands for later submission.
// On browser, this wraps a GPUCommandEncoder via internal/browser.CommandEncoder.
type CommandEncoder struct {
	browser  *browser.CommandEncoder
	released bool
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}
	jsDesc := buildRenderPassDescriptorJS(desc)
	bp := e.browser.BeginRenderPass(jsDesc)
	return &RenderPassEncoder{
		browser:  bp,
		released: false,
	}, nil
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}
	label := ""
	if desc != nil {
		label = desc.Label
	}
	jsDesc := browser.BuildComputePassDescriptor(label)
	bp := e.browser.BeginComputePass(jsDesc)
	return &ComputePassEncoder{
		browser:  bp,
		released: false,
	}, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	if e.released || src == nil || dst == nil {
		return
	}
	e.browser.CopyBufferToBuffer(
		src.browser.Ref(), srcOffset,
		dst.browser.Ref(), dstOffset,
		size,
	)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		srcObj := browser.BuildImageCopyTexture(
			src.browser.Ref(),
			r.TextureBase.MipLevel,
			r.TextureBase.Origin.X, r.TextureBase.Origin.Y, r.TextureBase.Origin.Z,
		)
		dstObj := browser.BuildImageCopyBuffer(
			dst.browser.Ref(),
			r.BufferLayout.Offset,
			r.BufferLayout.BytesPerRow,
			r.BufferLayout.RowsPerImage,
		)
		sizeObj := browser.BuildExtent3D(r.Size.Width, r.Size.Height, r.Size.DepthOrArrayLayers)
		e.browser.CopyTextureToBuffer(srcObj, dstObj, sizeObj)
	}
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		srcObj := browser.BuildImageCopyTexture(
			src.browser.Ref(),
			r.Source.MipLevel,
			r.Source.Origin.X, r.Source.Origin.Y, r.Source.Origin.Z,
		)
		dstObj := browser.BuildImageCopyTexture(
			dst.browser.Ref(),
			r.Destination.MipLevel,
			r.Destination.Origin.X, r.Destination.Origin.Y, r.Destination.Origin.Z,
		)
		sizeObj := browser.BuildExtent3D(r.Size.Width, r.Size.Height, r.Size.DepthOrArrayLayers)
		e.browser.CopyTextureToTexture(srcObj, dstObj, sizeObj)
	}
}

// TransitionTextures transitions texture states for synchronization.
// On browser, this is a no-op — the browser WebGPU API handles barriers internally.
func (e *CommandEncoder) TransitionTextures(barriers []TextureBarrier) {
	// No-op: browser WebGPU manages resource state transitions automatically.
}

// DiscardEncoding discards the encoder without producing a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	if e.released {
		return
	}
	e.released = true
	// Browser GC handles cleanup of the underlying GPUCommandEncoder.
}

// Finish completes command recording and returns a CommandBuffer.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	if e.released {
		return nil, ErrReleased
	}
	e.released = true
	bcb := e.browser.Finish(js.Undefined())
	return &CommandBuffer{
		browser:  bcb,
		released: false,
	}, nil
}

// CommandBuffer holds recorded GPU commands ready for submission.
// On browser, this wraps a GPUCommandBuffer via internal/browser.CommandBuffer.
type CommandBuffer struct {
	browser  *browser.CommandBuffer
	released bool
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
func (cb *CommandBuffer) Release() {
	if cb.released {
		return
	}
	cb.released = true
	// Browser GC handles cleanup.
}

// --- Render pass descriptor conversion ---

// buildRenderPassDescriptorJS converts a Go RenderPassDescriptor to a JS object
// via the internal/browser conversion helpers.
func buildRenderPassDescriptorJS(desc *RenderPassDescriptor) js.Value {
	colorAttachments := make([]browser.RenderPassColorAttachmentJS, len(desc.ColorAttachments))
	for i, ca := range desc.ColorAttachments {
		att := browser.RenderPassColorAttachmentJS{
			LoadOp:  browser.LoadOpToJS(ca.LoadOp),
			StoreOp: browser.StoreOpToJS(ca.StoreOp),
			ClearR:  ca.ClearValue.R,
			ClearG:  ca.ClearValue.G,
			ClearB:  ca.ClearValue.B,
			ClearA:  ca.ClearValue.A,
		}
		if ca.View != nil && ca.View.browser != nil {
			att.View = ca.View.browser.Ref()
		} else {
			att.View = js.Undefined()
		}
		if ca.ResolveTarget != nil && ca.ResolveTarget.browser != nil {
			att.ResolveTarget = ca.ResolveTarget.browser.Ref()
		} else {
			att.ResolveTarget = js.Undefined()
		}
		colorAttachments[i] = att
	}

	var depthStencil *browser.RenderPassDepthStencilAttachmentJS
	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		ds := &browser.RenderPassDepthStencilAttachmentJS{
			DepthLoadOp:       browser.LoadOpToJS(dsa.DepthLoadOp),
			DepthStoreOp:      browser.StoreOpToJS(dsa.DepthStoreOp),
			DepthClearValue:   dsa.DepthClearValue,
			DepthReadOnly:     dsa.DepthReadOnly,
			StencilLoadOp:     browser.LoadOpToJS(dsa.StencilLoadOp),
			StencilStoreOp:    browser.StoreOpToJS(dsa.StencilStoreOp),
			StencilClearValue: dsa.StencilClearValue,
			StencilReadOnly:   dsa.StencilReadOnly,
		}
		if dsa.View != nil && dsa.View.browser != nil {
			ds.View = dsa.View.browser.Ref()
		} else {
			ds.View = js.Undefined()
		}
		depthStencil = ds
	}

	return browser.BuildRenderPassDescriptor(desc.Label, colorAttachments, depthStencil)
}
