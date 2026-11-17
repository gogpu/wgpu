//go:build js && wasm

package encoder

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/resource"
)

// CommandEncoder records GPU commands for later submission.
// On browser, this wraps a GPUCommandEncoder via internal/browser.CommandEncoder.
type CommandEncoder struct {
	browser  *browser.CommandEncoder
	released bool
}

// NewCommandEncoder wraps a browser command encoder. The root declaration
// layer wraps the result in the public CommandEncoder.
func NewCommandEncoder(b *browser.CommandEncoder) *CommandEncoder {
	return &CommandEncoder{browser: b}
}

// BeginRenderPass begins a render pass and returns the pass encoder the public
// declaration layer wraps.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	if e.released {
		return nil, resource.ErrReleased
	}
	jsDesc := buildRenderPassDescriptorJS(desc)
	return &RenderPassEncoder{browser: e.browser.BeginRenderPass(jsDesc)}, nil
}

// BeginComputePass begins a compute pass and returns the browser pass object
// the public ComputePassEncoder records through.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*browser.ComputePassEncoder, error) {
	if e.released {
		return nil, resource.ErrReleased
	}
	label := ""
	if desc != nil {
		label = desc.Label
	}
	jsDesc := browser.BuildComputePassDescriptor(label)
	return e.browser.BeginComputePass(jsDesc), nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *resource.Buffer, srcOffset uint64, dst *resource.Buffer, dstOffset uint64, size uint64) {
	if e.released || src == nil || dst == nil {
		return
	}
	e.browser.CopyBufferToBuffer(
		src.Browser().Ref(), srcOffset,
		dst.Browser().Ref(), dstOffset,
		size,
	)
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src *resource.Buffer, dst *resource.Texture, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		srcObj := browser.BuildImageCopyBuffer(
			src.Browser().Ref(),
			r.BufferLayout.Offset,
			r.BufferLayout.BytesPerRow,
			r.BufferLayout.RowsPerImage,
		)
		dstObj := browser.BuildImageCopyTexture(
			dst.Browser().Ref(),
			r.TextureBase.MipLevel,
			r.TextureBase.Origin.X, r.TextureBase.Origin.Y, r.TextureBase.Origin.Z,
		)
		sizeObj := browser.BuildExtent3D(r.Size.Width, r.Size.Height, r.Size.DepthOrArrayLayers)
		e.browser.CopyBufferToTexture(srcObj, dstObj, sizeObj)
	}
}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer *resource.Buffer, offset, size uint64) {
	if e.released || buffer == nil {
		return
	}
	e.browser.ClearBuffer(buffer.Browser().Ref(), offset, size)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *resource.Texture, dst *resource.Buffer, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		srcObj := browser.BuildImageCopyTexture(
			src.Browser().Ref(),
			r.TextureBase.MipLevel,
			r.TextureBase.Origin.X, r.TextureBase.Origin.Y, r.TextureBase.Origin.Z,
		)
		dstObj := browser.BuildImageCopyBuffer(
			dst.Browser().Ref(),
			r.BufferLayout.Offset,
			r.BufferLayout.BytesPerRow,
			r.BufferLayout.RowsPerImage,
		)
		sizeObj := browser.BuildExtent3D(r.Size.Width, r.Size.Height, r.Size.DepthOrArrayLayers)
		e.browser.CopyTextureToBuffer(srcObj, dstObj, sizeObj)
	}
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *resource.Texture, regions []TextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		srcObj := browser.BuildImageCopyTexture(
			src.Browser().Ref(),
			r.Source.MipLevel,
			r.Source.Origin.X, r.Source.Origin.Y, r.Source.Origin.Z,
		)
		dstObj := browser.BuildImageCopyTexture(
			dst.Browser().Ref(),
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
		return nil, resource.ErrReleased
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
	if cb == nil || cb.released {
		return
	}
	cb.released = true
	// Browser GC handles cleanup.
}

// Browser returns the browser command buffer object, or nil.
func (cb *CommandBuffer) Browser() *browser.CommandBuffer { return cb.browser }

// Released reports whether the command buffer has been submitted or released.
func (cb *CommandBuffer) Released() bool { return cb.released }

// MarkSubmitted marks the command buffer spent. The browser queue submits it
// once: a submitted buffer is not submitted again, and its Release is a no-op.
func (cb *CommandBuffer) MarkSubmitted() { cb.released = true }

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
		if ca.View != nil && ca.View.Browser() != nil {
			att.View = ca.View.Browser().Ref()
		} else {
			att.View = js.Undefined()
		}
		if ca.ResolveTarget != nil && ca.ResolveTarget.Browser() != nil {
			att.ResolveTarget = ca.ResolveTarget.Browser().Ref()
		} else {
			att.ResolveTarget = js.Undefined()
		}
		if ca.DepthSlice != nil {
			slice := *ca.DepthSlice
			att.DepthSlice = &slice
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
		if dsa.View != nil && dsa.View.Browser() != nil {
			ds.View = dsa.View.Browser().Ref()
		} else {
			ds.View = js.Undefined()
		}
		depthStencil = ds
	}

	return browser.BuildRenderPassDescriptor(desc.Label, colorAttachments, depthStencil)
}
