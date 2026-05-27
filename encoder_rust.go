//go:build rust

package wgpu

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
)

// CommandEncoder records GPU commands for later submission.
// On Rust backend, this wraps go-webgpu/webgpu CommandEncoder.
type CommandEncoder struct {
	r        *rwgpu.CommandEncoder
	device   *Device
	released bool
}

// BeginRenderPass begins a render pass.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}

	rDesc := convertRenderPassDescriptorRust(desc)
	rp, err := e.r.BeginRenderPass(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to begin render pass: %w", err)
	}

	return &RenderPassEncoder{r: rp}, nil
}

// BeginComputePass begins a compute pass.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.released {
		return nil, ErrReleased
	}

	var rDesc *rwgpu.ComputePassDescriptor
	if desc != nil {
		rDesc = &rwgpu.ComputePassDescriptor{
			Label: desc.Label,
		}
	}

	rp, err := e.r.BeginComputePass(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to begin compute pass: %w", err)
	}

	return &ComputePassEncoder{r: rp}, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *Buffer, srcOffset uint64, dst *Buffer, dstOffset uint64, size uint64) {
	if e.released || src == nil || dst == nil {
		return
	}
	e.r.CopyBufferToBuffer(src.r, srcOffset, dst.r, dstOffset, size)
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src *Buffer, dst *Texture, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		rSrc := &rwgpu.TexelCopyBufferInfo{
			Buffer: src.r.Handle(),
			Layout: rwgpu.TexelCopyBufferLayout{
				Offset:       r.BufferLayout.Offset,
				BytesPerRow:  r.BufferLayout.BytesPerRow,
				RowsPerImage: r.BufferLayout.RowsPerImage,
			},
		}
		rDst := &rwgpu.TexelCopyTextureInfo{
			Texture:  dst.r.Handle(),
			MipLevel: r.TextureBase.MipLevel,
			Origin:   rwgpu.Origin3D{X: r.TextureBase.Origin.X, Y: r.TextureBase.Origin.Y, Z: r.TextureBase.Origin.Z},
		}
		rSize := &rwgpu.Extent3D{
			Width:              r.Size.Width,
			Height:             r.Size.Height,
			DepthOrArrayLayers: r.Size.DepthOrArrayLayers,
		}
		e.r.CopyBufferToTexture(rSrc, rDst, rSize)
	}
}

// ClearBuffer clears a buffer region to zero.
func (e *CommandEncoder) ClearBuffer(buffer *Buffer, offset, size uint64) {
	if e.released || buffer == nil {
		return
	}
	e.r.ClearBuffer(buffer.r, offset, size)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *Texture, dst *Buffer, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	rRegions := make([]rwgpu.BufferTextureCopy, len(regions))
	for i, r := range regions {
		rRegions[i] = rwgpu.BufferTextureCopy{
			BufferLayout: rwgpu.ImageDataLayout(r.BufferLayout),
			TextureBase: rwgpu.ImageCopyTexture{
				Texture:  r.TextureBase.Texture.r,
				MipLevel: r.TextureBase.MipLevel,
				Origin:   rwgpu.Origin3D(r.TextureBase.Origin),
				Aspect:   rwgpu.TextureAspect(r.TextureBase.Aspect),
			},
			Size: rwgpu.Extent3D{Width: r.Size.Width, Height: r.Size.Height, DepthOrArrayLayers: r.Size.DepthOrArrayLayers},
		}
	}
	e.r.CopyTextureToBuffer(src.r, dst.r, rRegions)
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *Texture, regions []TextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	rRegions := make([]rwgpu.TextureCopy, len(regions))
	for i, r := range regions {
		rRegions[i] = rwgpu.TextureCopy{
			Source: rwgpu.ImageCopyTexture{
				Texture:  r.Source.Texture.r,
				MipLevel: r.Source.MipLevel,
				Origin:   rwgpu.Origin3D(r.Source.Origin),
				Aspect:   rwgpu.TextureAspect(r.Source.Aspect),
			},
			Destination: rwgpu.ImageCopyTexture{
				Texture:  r.Destination.Texture.r,
				MipLevel: r.Destination.MipLevel,
				Origin:   rwgpu.Origin3D(r.Destination.Origin),
				Aspect:   rwgpu.TextureAspect(r.Destination.Aspect),
			},
			Size: rwgpu.Extent3D{Width: r.Size.Width, Height: r.Size.Height, DepthOrArrayLayers: r.Size.DepthOrArrayLayers},
		}
	}
	e.r.CopyTextureToTexture(src.r, dst.r, rRegions)
}

// TransitionTextures transitions texture states for synchronization.
// On Rust backend, this is a no-op. wgpu-native handles barriers internally.
func (e *CommandEncoder) TransitionTextures(_ []TextureBarrier) {
	// No-op: wgpu-native manages resource state transitions automatically.
}

// DiscardEncoding discards the encoder without producing a command buffer.
func (e *CommandEncoder) DiscardEncoding() {
	if e.released {
		return
	}
	e.released = true
	if e.r != nil {
		e.r.Release()
	}
}

// Finish completes command recording and returns a CommandBuffer.
func (e *CommandEncoder) Finish() (*CommandBuffer, error) {
	if e.released {
		return nil, ErrReleased
	}
	e.released = true

	rcb, err := e.r.Finish()
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to finish command encoder: %w", err)
	}

	return &CommandBuffer{r: rcb}, nil
}

// CommandBuffer holds recorded GPU commands ready for submission.
// On Rust backend, this wraps go-webgpu/webgpu CommandBuffer.
type CommandBuffer struct {
	r         *rwgpu.CommandBuffer
	submitted bool
}

// Release releases a CommandBuffer that will NOT be submitted to the GPU.
func (cb *CommandBuffer) Release() {
	if cb == nil || cb.r == nil {
		return
	}
	cb.r.Release()
	cb.r = nil
}

// --- Render pass descriptor conversion ---

func convertRenderPassDescriptorRust(desc *RenderPassDescriptor) *rwgpu.RenderPassDescriptor {
	if desc == nil {
		return &rwgpu.RenderPassDescriptor{}
	}

	rDesc := &rwgpu.RenderPassDescriptor{
		Label: desc.Label,
	}

	rDesc.ColorAttachments = make([]rwgpu.RenderPassColorAttachment, len(desc.ColorAttachments))
	for i, ca := range desc.ColorAttachments {
		att := rwgpu.RenderPassColorAttachment{
			LoadOp:  ca.LoadOp,
			StoreOp: ca.StoreOp,
			ClearValue: rwgpu.Color{
				R: ca.ClearValue.R,
				G: ca.ClearValue.G,
				B: ca.ClearValue.B,
				A: ca.ClearValue.A,
			},
		}
		if ca.View != nil {
			att.View = ca.View.r
		}
		if ca.ResolveTarget != nil {
			att.ResolveTarget = ca.ResolveTarget.r
		}
		rDesc.ColorAttachments[i] = att
	}

	if desc.DepthStencilAttachment != nil {
		dsa := desc.DepthStencilAttachment
		rDSA := &rwgpu.RenderPassDepthStencilAttachment{
			DepthLoadOp:       dsa.DepthLoadOp,
			DepthStoreOp:      dsa.DepthStoreOp,
			DepthClearValue:   dsa.DepthClearValue,
			DepthReadOnly:     dsa.DepthReadOnly,
			StencilLoadOp:     dsa.StencilLoadOp,
			StencilStoreOp:    dsa.StencilStoreOp,
			StencilClearValue: dsa.StencilClearValue,
			StencilReadOnly:   dsa.StencilReadOnly,
		}
		if dsa.View != nil {
			rDSA.View = dsa.View.r
		}
		rDesc.DepthStencilAttachment = rDSA
	}

	return rDesc
}
