//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestBufferTextureCopiesKeepTheAspect: the aspect of a buffer-texture copy
// region reaches the backend in both directions. CopyBufferToTexture dropped
// it, so a stencil-only upload was recorded for the color aspect.
func TestBufferTextureCopiesKeepTheAspect(t *testing.T) {
	f := newZeroSizeFixture(t)
	tex, err := f.device.CreateTexture(&TextureDescriptor{
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        gputypes.TextureFormatDepth24PlusStencil8,
		Usage:         TextureUsageCopySrc | TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	region := BufferTextureCopy{
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		TextureBase:  ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectStencilOnly},
		Size:         Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	}
	enc := f.encoder(t)
	enc.CopyBufferToTexture(f.srcBuf, tex, []BufferTextureCopy{region})
	enc.CopyTextureToBuffer(tex, f.dstBuf, []BufferTextureCopy{region})
	if _, err := enc.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	recorded := f.raw.encoder
	if got := recorded.lastBufferToTexture[0].TextureBase.Aspect; got != gputypes.TextureAspectStencilOnly {
		t.Errorf("CopyBufferToTexture aspect at the backend = %v, want stencil-only", got)
	}
	if got := recorded.lastTextureToBuffer[0].TextureBase.Aspect; got != gputypes.TextureAspectStencilOnly {
		t.Errorf("CopyTextureToBuffer aspect at the backend = %v, want stencil-only", got)
	}
}
