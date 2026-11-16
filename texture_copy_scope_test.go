//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core/track"
)

func TestCopyCommandsPopulateTextureScope(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	src := createCopyScopeTexture(t, device, "copy-scope-src", TextureUsageCopySrc)
	defer src.Release()
	dst := createCopyScopeTexture(t, device, "copy-scope-dst", TextureUsageCopyDst)
	defer dst.Release()
	buffer, err := device.CreateBuffer(&BufferDescriptor{
		Label: "copy-scope-buffer",
		Size:  256,
		Usage: BufferUsageCopySrc | BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()

	bufferTextureRegion := []BufferTextureCopy{{
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
		Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}}
	textureRegion := []TextureCopy{{
		Source:      ImageCopyTexture{Texture: src},
		Destination: ImageCopyTexture{Texture: dst},
		Size:        Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
	}}

	tests := []struct {
		name   string
		record func(*CommandEncoder)
		want   map[*Texture]track.TextureUses
	}{
		{
			name: "texture to buffer",
			record: func(enc *CommandEncoder) {
				regions := append([]BufferTextureCopy(nil), bufferTextureRegion...)
				regions[0].TextureBase.Texture = src
				enc.CopyTextureToBuffer(src, buffer, regions)
			},
			want: map[*Texture]track.TextureUses{src: track.TextureUsesCopySrc},
		},
		{
			name: "buffer to texture",
			record: func(enc *CommandEncoder) {
				regions := append([]BufferTextureCopy(nil), bufferTextureRegion...)
				regions[0].TextureBase.Texture = dst
				enc.CopyBufferToTexture(buffer, dst, regions)
			},
			want: map[*Texture]track.TextureUses{dst: track.TextureUsesCopyDst},
		},
		{
			name:   "texture to texture",
			record: func(enc *CommandEncoder) { enc.CopyTextureToTexture(src, dst, textureRegion) },
			want: map[*Texture]track.TextureUses{
				src: track.TextureUsesCopySrc,
				dst: track.TextureUsesCopyDst,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.record(enc)
			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			defer cb.Release()

			scope := cb.impl.Core().TextureScope()
			for tex, want := range tt.want {
				idx := tex.impl.CoreTexture().TrackingData().Index()
				if got := scope.EndUsage(idx); got != want {
					t.Errorf("texture %q usage = %v, want %v", tex.impl.CoreTexture().Label(), got, want)
				}
			}
		})
	}
}

func TestFailedCopyDoesNotPopulateTextureScope(t *testing.T) {
	t.Parallel()

	t.Run("released destination buffer", func(t *testing.T) {
		_, _, device := newTestDeviceWithTracker(t)
		defer device.Release()
		tex := createCopyScopeTexture(t, device, "failed-copy-src", TextureUsageCopySrc)
		defer tex.Release()
		buffer, err := device.CreateBuffer(&BufferDescriptor{Size: 256, Usage: BufferUsageCopyDst})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		buffer.Release()

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.CopyTextureToBuffer(tex, buffer, nil)

		idx := tex.impl.CoreTexture().TrackingData().Index()
		if scope := enc.impl.Core().Mutable().TextureScope(); scope.IsUsed(idx) {
			t.Fatalf("failed copy recorded texture usage %v", scope.EndUsage(idx))
		}
		if _, err := enc.Finish(); err == nil {
			t.Fatal("Finish succeeded after copy to released buffer")
		}
	})

	t.Run("released source buffer", func(t *testing.T) {
		_, _, device := newTestDeviceWithTracker(t)
		defer device.Release()
		tex := createCopyScopeTexture(t, device, "failed-copy-dst", TextureUsageCopyDst)
		defer tex.Release()
		buffer, err := device.CreateBuffer(&BufferDescriptor{Size: 256, Usage: BufferUsageCopySrc})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		buffer.Release()

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.CopyBufferToTexture(buffer, tex, nil)

		idx := tex.impl.CoreTexture().TrackingData().Index()
		if scope := enc.impl.Core().Mutable().TextureScope(); scope.IsUsed(idx) {
			t.Fatalf("failed copy recorded texture usage %v", scope.EndUsage(idx))
		}
		if _, err := enc.Finish(); err == nil {
			t.Fatal("Finish succeeded after copy from released buffer")
		}
	})
}

func createCopyScopeTexture(t *testing.T, device *Device, label string, copyUsage gputypes.TextureUsage) *Texture {
	t.Helper()
	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         label,
		Size:          Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         copyUsage | TextureUsageTextureBinding | TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	return tex
}

// TestCopyAfterEarlierCommandUsageIsNotAConflict: a copy is its own usage
// scope. A resource left in an "incompatible" usage by an earlier command of
// the same encoder is transitioned, not rejected. (Before the fix, #312's
// encoder-wide scope rejected all of these cases.)
func TestCopyAfterEarlierCommandUsageIsNotAConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seed       func(*CommandEncoder, *Texture, *Texture, *Buffer)
		record     func(*CommandEncoder, *Texture, *Texture, *Buffer)
		wantSrc    track.TextureUses
		wantDst    track.TextureUses
		wantBuffer track.BufferUses
	}{
		{
			name: "texture written earlier is copied to a buffer",
			seed: func(enc *CommandEncoder, src, _ *Texture, _ *Buffer) {
				seedTextureUsage(enc, src, track.TextureUsesCopyDst)
			},
			record: func(enc *CommandEncoder, src, _ *Texture, buffer *Buffer) {
				enc.CopyTextureToBuffer(src, buffer, nil)
			},
			wantSrc: track.TextureUsesCopySrc, wantBuffer: track.BufferUsesCopyDst,
		},
		{
			name: "buffer read earlier receives a texture copy",
			seed: func(enc *CommandEncoder, _, _ *Texture, buffer *Buffer) {
				seedBufferUsage(enc, buffer, track.BufferUsesCopySrc)
			},
			record: func(enc *CommandEncoder, src, _ *Texture, buffer *Buffer) {
				enc.CopyTextureToBuffer(src, buffer, nil)
			},
			wantSrc: track.TextureUsesCopySrc, wantBuffer: track.BufferUsesCopyDst,
		},
		{
			name: "texture read earlier receives a buffer upload",
			seed: func(enc *CommandEncoder, _, dst *Texture, _ *Buffer) {
				seedTextureUsage(enc, dst, track.TextureUsesCopySrc)
			},
			record: func(enc *CommandEncoder, _, dst *Texture, buffer *Buffer) {
				enc.CopyBufferToTexture(buffer, dst, nil)
			},
			wantDst: track.TextureUsesCopyDst, wantBuffer: track.BufferUsesCopySrc,
		},
		{
			name: "buffer written earlier is uploaded to a texture",
			seed: func(enc *CommandEncoder, _, _ *Texture, buffer *Buffer) {
				seedBufferUsage(enc, buffer, track.BufferUsesCopyDst)
			},
			record: func(enc *CommandEncoder, _, dst *Texture, buffer *Buffer) {
				enc.CopyBufferToTexture(buffer, dst, nil)
			},
			wantDst: track.TextureUsesCopyDst, wantBuffer: track.BufferUsesCopySrc,
		},
		{
			name: "render target is copied to another texture",
			seed: func(enc *CommandEncoder, src, _ *Texture, _ *Buffer) {
				seedTextureUsage(enc, src, track.TextureUsesColorTarget)
			},
			record: func(enc *CommandEncoder, src, dst *Texture, _ *Buffer) {
				enc.CopyTextureToTexture(src, dst, nil)
			},
			wantSrc: track.TextureUsesCopySrc, wantDst: track.TextureUsesCopyDst,
		},
		{
			name: "sampled texture receives a texture copy",
			seed: func(enc *CommandEncoder, _, dst *Texture, _ *Buffer) {
				seedTextureUsage(enc, dst, track.TextureUsesResource)
			},
			record: func(enc *CommandEncoder, src, dst *Texture, _ *Buffer) {
				enc.CopyTextureToTexture(src, dst, nil)
			},
			wantSrc: track.TextureUsesCopySrc, wantDst: track.TextureUsesCopyDst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, device := newTestDeviceWithTracker(t)
			defer device.Release()
			src := createCopyScopeTexture(t, device, "earlier-src", TextureUsageCopySrc|TextureUsageCopyDst)
			defer src.Release()
			dst := createCopyScopeTexture(t, device, "earlier-dst", TextureUsageCopySrc|TextureUsageCopyDst)
			defer dst.Release()
			buffer := createCopyScopeBuffer(t, device, "earlier-buffer")
			defer buffer.Release()

			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.seed(enc, src, dst, buffer)
			tt.record(enc, src, dst, buffer)

			textures := enc.impl.Core().Mutable().TextureScope()
			if want := tt.wantSrc; want != 0 {
				if got := textures.EndUsage(src.impl.CoreTexture().TrackingData().Index()); got != want {
					t.Errorf("source texture usage = %v, want %v", got, want)
				}
			}
			if want := tt.wantDst; want != 0 {
				if got := textures.EndUsage(dst.impl.CoreTexture().TrackingData().Index()); got != want {
					t.Errorf("destination texture usage = %v, want %v", got, want)
				}
			}
			if got := enc.impl.Core().Mutable().BufferScope().EndUsage(buffer.impl.CoreBuffer().TrackingData().Index()); got != tt.wantBuffer {
				t.Errorf("buffer usage = %v, want %v", got, tt.wantBuffer)
			}
			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			cb.Release()
		})
	}
}

// TestCopyOntoItselfIsRejectedAtomically: the copy's own usage scope makes a
// resource that is both source and destination a conflict. The rejected copy
// must leave the tracker, the retained refs and the submit validation sets
// untouched.
func TestCopyOntoItselfIsRejectedAtomically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record func(*CommandEncoder, *Texture, *Buffer)
	}{
		{
			name:   "texture to same texture",
			record: func(enc *CommandEncoder, tex *Texture, _ *Buffer) { enc.CopyTextureToTexture(tex, tex, nil) },
		},
		{
			name: "buffer to same buffer",
			record: func(enc *CommandEncoder, _ *Texture, buf *Buffer) {
				enc.CopyBufferToBuffer(buf, 0, buf, 128, 64)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, device := newTestDeviceWithTracker(t)
			defer device.Release()
			tex := createCopyScopeTexture(t, device, "self-copy", TextureUsageCopySrc|TextureUsageCopyDst)
			defer tex.Release()
			buf := createCopyScopeBuffer(t, device, "self-copy")
			defer buf.Release()

			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.record(enc, tex, buf)

			if enc.impl.Core().Mutable().TextureScope().IsUsed(tex.impl.CoreTexture().TrackingData().Index()) {
				t.Error("rejected copy recorded the texture in the command buffer tracker")
			}
			if enc.impl.Core().Mutable().BufferScope().IsUsed(buf.impl.CoreBuffer().TrackingData().Index()) {
				t.Error("rejected copy recorded the buffer in the command buffer tracker")
			}
			if got := len(enc.impl.TrackedRefs()); got != 0 {
				t.Errorf("rejected copy retained %d refs, want 0", got)
			}
			if len(enc.impl.UsedTextures()) != 0 || len(enc.impl.UsedBuffers()) != 0 {
				t.Errorf("rejected copy changed submit validation sets: textures=%d buffers=%d",
					len(enc.impl.UsedTextures()), len(enc.impl.UsedBuffers()))
			}
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish succeeded after a copy onto itself")
			}
		})
	}
}

func TestCopyBufferToBufferRecordsBothUsages(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	src := createCopyScopeBuffer(t, device, "buffer-copy-src")
	defer src.Release()
	dst := createCopyScopeBuffer(t, device, "buffer-copy-dst")
	defer dst.Release()

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToBuffer(src, 0, dst, 0, 64)
	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	defer cb.Release()

	if got := cb.impl.Core().BufferScope().EndUsage(src.impl.CoreBuffer().TrackingData().Index()); got != track.BufferUsesCopySrc {
		t.Errorf("source usage = %v, want %v", got, track.BufferUsesCopySrc)
	}
	if got := cb.impl.Core().BufferScope().EndUsage(dst.impl.CoreBuffer().TrackingData().Index()); got != track.BufferUsesCopyDst {
		t.Errorf("destination usage = %v, want %v", got, track.BufferUsesCopyDst)
	}
}

func createCopyScopeBuffer(t *testing.T, device *Device, label string) *Buffer {
	t.Helper()
	buffer, err := device.CreateBuffer(&BufferDescriptor{
		Label: label,
		Size:  256,
		Usage: BufferUsageCopySrc | BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	return buffer
}

// seedTextureUsage simulates an earlier command of the encoder that left the
// texture in usage.
func seedTextureUsage(enc *CommandEncoder, texture *Texture, usage track.TextureUses) {
	enc.impl.Core().Mutable().TextureScope().SetSingle(texture.impl.CoreTexture().TrackingData().Index(), usage)
}

// seedBufferUsage simulates an earlier command of the encoder that left the
// buffer in usage.
func seedBufferUsage(enc *CommandEncoder, buffer *Buffer, usage track.BufferUses) {
	enc.impl.Core().Mutable().BufferScope().SetSingle(buffer.impl.CoreBuffer().TrackingData().Index(), usage)
}
