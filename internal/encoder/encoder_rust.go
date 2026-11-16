//go:build rust

package encoder

import (
	"fmt"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// CommandEncoder records GPU commands for later submission.
// On Rust backend, this wraps go-webgpu/webgpu CommandEncoder.
//
// The former device field is gone: it was write-only (ADR-070 §13.14, B13;
// every rust error path reads the wrapped encoder, not the device), and the
// destination of the encoder move may not hold a root device pointer.
type CommandEncoder struct {
	r        *rwgpu.CommandEncoder
	released bool
}

// NewCommandEncoder wraps a go-webgpu command encoder. The root declaration
// layer wraps the result in the public CommandEncoder.
func NewCommandEncoder(r *rwgpu.CommandEncoder) *CommandEncoder {
	return &CommandEncoder{r: r}
}

// BeginRenderPass begins a render pass and returns the go-webgpu pass object
// the public RenderPassEncoder records through.
func (e *CommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*rwgpu.RenderPassEncoder, error) {
	if e.released {
		return nil, core.ErrReleased
	}

	rDesc, err := convertRenderPassDescriptorRust(desc)
	if err != nil {
		return nil, err
	}
	rp, err := e.r.BeginRenderPass(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to begin render pass: %w", err)
	}

	return rp, nil
}

// BeginComputePass begins a compute pass and returns the go-webgpu pass object
// the public ComputePassEncoder records through.
func (e *CommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*rwgpu.ComputePassEncoder, error) {
	if e.released {
		return nil, core.ErrReleased
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

	return rp, nil
}

// CopyBufferToBuffer copies data between buffers.
func (e *CommandEncoder) CopyBufferToBuffer(src *resource.Buffer, srcOffset uint64, dst *resource.Buffer, dstOffset uint64, size uint64) {
	if e.released || src == nil || dst == nil {
		return
	}
	e.r.CopyBufferToBuffer(src.R(), srcOffset, dst.R(), dstOffset, size)
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (e *CommandEncoder) CopyBufferToTexture(src *resource.Buffer, dst *resource.Texture, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	for _, r := range regions {
		rSrc := &rwgpu.TexelCopyBufferInfo{
			Buffer: src.R().Handle(),
			Layout: rwgpu.TexelCopyBufferLayout{
				Offset:       r.BufferLayout.Offset,
				BytesPerRow:  r.BufferLayout.BytesPerRow,
				RowsPerImage: r.BufferLayout.RowsPerImage,
			},
		}
		rDst := &rwgpu.TexelCopyTextureInfo{
			Texture:  dst.Rust().Handle(),
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
func (e *CommandEncoder) ClearBuffer(buffer *resource.Buffer, offset, size uint64) {
	if e.released || buffer == nil {
		return
	}
	e.r.ClearBuffer(buffer.R(), offset, size)
}

// CopyTextureToBuffer copies data from a texture to a buffer.
func (e *CommandEncoder) CopyTextureToBuffer(src *resource.Texture, dst *resource.Buffer, regions []BufferTextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	rRegions := make([]rwgpu.BufferTextureCopy, len(regions))
	for i, r := range regions {
		rRegions[i] = rwgpu.BufferTextureCopy{
			BufferLayout: rwgpu.ImageDataLayout(r.BufferLayout),
			TextureBase: rwgpu.ImageCopyTexture{
				Texture:  r.TextureBase.Texture.Rust(),
				MipLevel: r.TextureBase.MipLevel,
				Origin:   rwgpu.Origin3D(r.TextureBase.Origin),
				Aspect:   rwgpu.TextureAspect(r.TextureBase.Aspect),
			},
			Size: rwgpu.Extent3D{Width: r.Size.Width, Height: r.Size.Height, DepthOrArrayLayers: r.Size.DepthOrArrayLayers},
		}
	}
	e.r.CopyTextureToBuffer(src.Rust(), dst.R(), rRegions)
}

// CopyTextureToTexture copies data between textures.
func (e *CommandEncoder) CopyTextureToTexture(src, dst *resource.Texture, regions []TextureCopy) {
	if e.released || src == nil || dst == nil {
		return
	}
	rRegions := make([]rwgpu.TextureCopy, len(regions))
	for i, r := range regions {
		rRegions[i] = rwgpu.TextureCopy{
			Source: rwgpu.ImageCopyTexture{
				Texture:  r.Source.Texture.Rust(),
				MipLevel: r.Source.MipLevel,
				Origin:   rwgpu.Origin3D(r.Source.Origin),
				Aspect:   rwgpu.TextureAspect(r.Source.Aspect),
			},
			Destination: rwgpu.ImageCopyTexture{
				Texture:  r.Destination.Texture.Rust(),
				MipLevel: r.Destination.MipLevel,
				Origin:   rwgpu.Origin3D(r.Destination.Origin),
				Aspect:   rwgpu.TextureAspect(r.Destination.Aspect),
			},
			Size: rwgpu.Extent3D{Width: r.Size.Width, Height: r.Size.Height, DepthOrArrayLayers: r.Size.DepthOrArrayLayers},
		}
	}
	e.r.CopyTextureToTexture(src.Rust(), dst.Rust(), rRegions)
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
		return nil, core.ErrReleased
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

// R returns the go-webgpu command buffer, or nil once released.
func (cb *CommandBuffer) R() *rwgpu.CommandBuffer { return cb.r }

// MarkSubmitted marks the command buffer submitted; a command buffer is not
// submitted twice.
func (cb *CommandBuffer) MarkSubmitted() { cb.submitted = true }

// IsSubmitted reports whether the command buffer has been submitted.
func (cb *CommandBuffer) IsSubmitted() bool { return cb.submitted }
