//go:build !rust && !(js && wasm)

package encoder

// The HAL conversions of the copy and barrier descriptors. They live in their
// own native file: the HAL copy types are native-only, and the browser and
// rust encoders convert straight to their backend objects.

import "github.com/gogpu/wgpu/internal/hal"

func (i *ImageCopyTexture) toHAL() *hal.ImageCopyTexture {
	if i == nil || i.Texture == nil || i.Texture.ResolveHAL() == nil {
		return nil
	}
	return &hal.ImageCopyTexture{
		Texture:  i.Texture.ResolveHAL(),
		MipLevel: i.MipLevel,
		Origin:   hal.Origin3D(i.Origin),
		Aspect:   i.Aspect,
	}
}

func (t *TextureCopy) toHAL() hal.TextureCopy {
	return hal.TextureCopy{
		SrcBase: *t.Source.toHAL(),
		DstBase: *t.Destination.toHAL(),
		Size:    hal.Extent3D(t.Size),
	}
}

func (b TextureBarrier) toHAL() hal.TextureBarrier {
	var t hal.Texture
	if b.Texture != nil {
		t = b.Texture.ResolveHAL()
	}
	return hal.TextureBarrier{
		Texture: t,
		Range: hal.TextureRange{
			Aspect:          b.Range.Aspect,
			BaseMipLevel:    b.Range.BaseMipLevel,
			MipLevelCount:   b.Range.MipLevelCount,
			BaseArrayLayer:  b.Range.BaseArrayLayer,
			ArrayLayerCount: b.Range.ArrayLayerCount,
		},
		Usage: hal.TextureUsageTransition{
			OldUsage: b.Usage.OldUsage,
			NewUsage: b.Usage.NewUsage,
		},
	}
}

func (l ImageDataLayout) toHAL() hal.ImageDataLayout {
	return hal.ImageDataLayout{Offset: l.Offset, BytesPerRow: l.BytesPerRow, RowsPerImage: l.RowsPerImage}
}

func (c BufferTextureCopy) toHAL() hal.BufferTextureCopy {
	btc := hal.BufferTextureCopy{
		BufferLayout: c.BufferLayout.toHAL(),
		Size:         hal.Extent3D(c.Size),
	}
	halTex := c.TextureBase.toHAL()
	if halTex != nil {
		btc.TextureBase = *halTex
	}
	return btc
}
