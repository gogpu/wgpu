//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// TestBufferTextureCopyBytesPerRowAlignment: a buffer-texture copy recorded on
// a command encoder needs bytesPerRow to be a multiple of 256 (WebGPU
// GPUTexelCopyBufferInfo validation, Rust validate_texture_buffer_copy);
// Finish reports a *core.TransferError (P1-VAL-016). Both copy directions.
func TestBufferTextureCopyBytesPerRowAlignment(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 1024, Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	for _, tc := range []struct {
		name        string
		bytesPerRow uint32
		wantErr     bool
	}{
		{"256", 256, false},
		{"32", 32, true},
	} {
		for _, direction := range []string{"texture-to-buffer", "buffer-to-texture"} {
			t.Run(direction+"/"+tc.name, func(t *testing.T) {
				enc, err := device.CreateCommandEncoder(nil)
				if err != nil {
					t.Fatalf("CreateCommandEncoder: %v", err)
				}
				regions := []wgpu.BufferTextureCopy{{
					TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
					BufferLayout: wgpu.ImageDataLayout{BytesPerRow: tc.bytesPerRow, RowsPerImage: 4},
					Size:         wgpu.Extent3D{Width: 8, Height: 1, DepthOrArrayLayers: 1},
				}}
				if direction == "texture-to-buffer" {
					enc.CopyTextureToBuffer(tex, buf, regions)
				} else {
					enc.CopyBufferToTexture(buf, tex, regions)
				}
				cmd, err := enc.Finish()
				if !tc.wantErr {
					if err != nil {
						t.Fatalf("Finish: %v", err)
					}
					cmd.Release()
					return
				}
				var te *core.TransferError
				if !errors.As(err, &te) || te.Kind != core.TransferErrorUnalignedBytesPerRow {
					t.Fatalf("Finish error = %v, want *core.TransferError UnalignedBytesPerRow", err)
				}
			})
		}
	}
}
