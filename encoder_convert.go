// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// Conversions from the public encoder argument types to the encoder domain's
// handle types. The public wrappers are thin handles (ADR-070), so a value
// converts to its implementation; the plain struct fields convert directly
// (Origin3D and Extent3D have the same shape as their gputypes twins).

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// encoderBuffer maps a public buffer wrapper to its resource handle. A
// non-nil wrapper with a nil implementation stays a non-nil zero buffer, so
// the encoder reports it released exactly as it did before the move; an
// absent buffer stays nil.
func encoderBuffer(b *Buffer) *resource.Buffer {
	if b == nil {
		return nil
	}
	if b.impl == nil {
		return &resource.Buffer{}
	}
	return b.impl
}

// encoderTexture maps a public texture wrapper to its resource handle, with
// the same nil-implementation rule as encoderBuffer.
func encoderTexture(t *Texture) *resource.Texture {
	if t == nil {
		return nil
	}
	if t.impl == nil {
		return &resource.Texture{}
	}
	return t.impl
}

// encoderTextureView maps a public texture-view wrapper to its resource
// handle: a present attachment with a nil implementation stays a non-nil zero
// view, so BeginRenderPass refuses it as released, as before the move.
func encoderTextureView(v *TextureView) *resource.TextureView {
	if v == nil {
		return nil
	}
	if v.impl == nil {
		return &resource.TextureView{}
	}
	return v.impl
}

// encoderImageCopyTexture converts a public image copy target.
func encoderImageCopyTexture(t *ImageCopyTexture) encoder.ImageCopyTexture {
	return encoder.ImageCopyTexture{
		Texture:  encoderTexture(t.Texture),
		MipLevel: t.MipLevel,
		Origin:   gputypes.Origin3D(t.Origin),
		Aspect:   t.Aspect,
	}
}

// encoderBufferTextureCopies converts buffer-texture copy regions.
func encoderBufferTextureCopies(regions []BufferTextureCopy) []encoder.BufferTextureCopy {
	if regions == nil {
		return nil
	}
	out := make([]encoder.BufferTextureCopy, len(regions))
	for i, r := range regions {
		out[i] = encoder.BufferTextureCopy{
			BufferLayout: encoder.ImageDataLayout{
				Offset:       r.BufferLayout.Offset,
				BytesPerRow:  r.BufferLayout.BytesPerRow,
				RowsPerImage: r.BufferLayout.RowsPerImage,
			},
			TextureBase: encoderImageCopyTexture(&r.TextureBase),
			Size:        gputypes.Extent3D(r.Size),
		}
	}
	return out
}

// encoderTextureCopies converts texture-to-texture copy regions.
func encoderTextureCopies(regions []TextureCopy) []encoder.TextureCopy {
	if regions == nil {
		return nil
	}
	out := make([]encoder.TextureCopy, len(regions))
	for i, r := range regions {
		out[i] = encoder.TextureCopy{
			Source:      encoderImageCopyTexture(&r.Source),
			Destination: encoderImageCopyTexture(&r.Destination),
			Size:        gputypes.Extent3D(r.Size),
		}
	}
	return out
}

// encoderTextureBarriers converts explicit texture barriers.
func encoderTextureBarriers(barriers []TextureBarrier) []encoder.TextureBarrier {
	if barriers == nil {
		return nil
	}
	out := make([]encoder.TextureBarrier, len(barriers))
	for i, b := range barriers {
		out[i] = encoder.TextureBarrier{
			Texture: encoderTexture(b.Texture),
			Range: encoder.TextureRange{
				Aspect:          b.Range.Aspect,
				BaseMipLevel:    b.Range.BaseMipLevel,
				MipLevelCount:   b.Range.MipLevelCount,
				BaseArrayLayer:  b.Range.BaseArrayLayer,
				ArrayLayerCount: b.Range.ArrayLayerCount,
			},
			Usage: encoder.TextureUsageTransition{
				OldUsage: b.Usage.OldUsage,
				NewUsage: b.Usage.NewUsage,
			},
		}
	}
	return out
}
