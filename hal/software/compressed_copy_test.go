//go:build !(js && wasm)

package software

import (
	"bytes"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestCompressedTextureLayersRoundTrip writes both layers of a BC1 8x8
// 2D-array texture in one WriteTexture and reads them back with one
// CopyTextureToBuffer, both with RowsPerImage 2. RowsPerImage counts texel
// block rows (WebGPU GPUTexelCopyBufferLayout, hal.ImageDataLayout), so
// layer 1 starts two block rows, 32 bytes, into the data. The old backend
// took it for texel rows and divided it by the block height: layer 1 was
// read from 16 bytes in, the second block row of layer 0.
func TestCompressedTextureLayersRoundTrip(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	const w, h, layers = 8, 8, 2
	const bytesPerRow, rowsPerImage = 2 * 8, 2 // two BC1 blocks of 8 bytes; two block rows
	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: layers},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBC1RGBAUnorm,
		Usage:         gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	data := make([]byte, bytesPerRow*rowsPerImage*layers)
	for i := range data {
		data[i] = byte(i)
	}
	layout := hal.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: rowsPerImage}
	size := hal.Extent3D{Width: w, Height: h, DepthOrArrayLayers: layers}
	if err := q.WriteTexture(&hal.ImageCopyTexture{Texture: tex}, data, &layout, &size); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}

	buf, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(data)), Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyTextureToBuffer(tex, buf, []hal.BufferTextureCopy{{
		BufferLayout: layout,
		TextureBase:  hal.ImageCopyTexture{Texture: tex},
		Size:         size,
	}})
	submit(t, enc)
	if got := buf.(*Buffer).GetData(); !bytes.Equal(got, data) {
		t.Fatalf("read back\n%v\nwant\n%v", got, data)
	}
}
