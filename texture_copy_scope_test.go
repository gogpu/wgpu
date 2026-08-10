//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
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

			scope := cb.core.TextureScope()
			for tex, want := range tt.want {
				idx := tex.coreTexture.TrackingData().Index()
				if got := scope.GetUsage(idx); got != want {
					t.Errorf("texture %q usage = %v, want %v", tex.coreTexture.Label(), got, want)
				}
			}
		})
	}
}

func TestCopyCommandTextureScopeDrivesBarriers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		before    track.TextureUses
		copyUsage track.TextureUses
		record    func(*CommandEncoder, *Texture, *Buffer)
	}{
		{
			name:      "sampled to copy source",
			before:    track.TextureUsesResource,
			copyUsage: track.TextureUsesCopySrc,
			record: func(enc *CommandEncoder, tex *Texture, buffer *Buffer) {
				enc.CopyTextureToBuffer(tex, buffer, []BufferTextureCopy{{
					TextureBase:  ImageCopyTexture{Texture: tex},
					BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
					Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
				}})
			},
		},
		{
			name:      "render target to copy destination",
			before:    track.TextureUsesColorTarget,
			copyUsage: track.TextureUsesCopyDst,
			record: func(enc *CommandEncoder, tex *Texture, buffer *Buffer) {
				enc.CopyBufferToTexture(buffer, tex, []BufferTextureCopy{{
					TextureBase:  ImageCopyTexture{Texture: tex},
					BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
					Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
				}})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, device := newTestDeviceWithTracker(t)
			defer device.Release()
			tex := createCopyScopeTexture(t, device, tt.name, TextureUsageCopySrc|TextureUsageCopyDst)
			defer tex.Release()
			buffer, err := device.CreateBuffer(&BufferDescriptor{
				Size:  256,
				Usage: BufferUsageCopySrc | BufferUsageCopyDst,
			})
			if err != nil {
				t.Fatalf("CreateBuffer: %v", err)
			}
			defer buffer.Release()

			idx := tex.coreTexture.TrackingData().Index()
			device.core.Tracker().InsertTexture(idx, tt.before)
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.record(enc, tex, buffer)
			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			defer cb.Release()

			probeTracker := core.NewDeviceTracker()
			probeTracker.InsertTexture(idx, tt.before)
			transitions := mergeTextureScopes(probeTracker, []*CommandBuffer{cb})
			if len(transitions) != 1 {
				t.Fatalf("copy scope produced %d transitions, want 1", len(transitions))
			}
			transition := transitions[0]
			if transition.Index != idx || transition.Usage.From != tt.before || transition.Usage.To != tt.copyUsage {
				t.Fatalf("transition = %+v, want index %d %v -> %v", transition, idx, tt.before, tt.copyUsage)
			}

			barrierCB, err := device.Queue().injectBarriers([]*CommandBuffer{cb})
			if err != nil {
				t.Fatalf("injectBarriers: %v", err)
			}
			if barrierCB == nil {
				t.Fatal("expected copy usage transition to inject a barrier command buffer")
			}
			if got := device.core.Tracker().Textures().GetUsage(idx); got != tt.copyUsage {
				t.Errorf("tracked usage after injection = %v, want %v", got, tt.copyUsage)
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

		idx := tex.coreTexture.TrackingData().Index()
		if scope := enc.core.Mutable().TextureScope(); scope.IsUsed(idx) {
			t.Fatalf("failed copy recorded texture usage %v", scope.GetUsage(idx))
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

		idx := tex.coreTexture.TrackingData().Index()
		if scope := enc.core.Mutable().TextureScope(); scope.IsUsed(idx) {
			t.Fatalf("failed copy recorded texture usage %v", scope.GetUsage(idx))
		}
		if _, err := enc.Finish(); err == nil {
			t.Fatal("Finish succeeded after copy from released buffer")
		}
	})
}

func createCopyScopeTexture(t *testing.T, device *Device, label string, copyUsage TextureUsage) *Texture {
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

func TestCopyUsageConflictsAreAtomic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seed       func(*testing.T, *CommandEncoder, *Texture, *Texture, *Buffer)
		record     func(*CommandEncoder, *Texture, *Texture, *Buffer)
		wantSrc    track.TextureUses
		wantDst    track.TextureUses
		wantBuffer track.BufferUses
	}{
		{
			name: "texture to buffer texture conflict leaves buffer untouched",
			seed: func(t *testing.T, enc *CommandEncoder, src, _ *Texture, _ *Buffer) {
				mustSetTextureScopeUsage(t, enc, src, track.TextureUsesCopyDst)
			},
			record: func(enc *CommandEncoder, src, _ *Texture, buffer *Buffer) {
				enc.CopyTextureToBuffer(src, buffer, nil)
			},
			wantSrc: track.TextureUsesCopyDst,
		},
		{
			name: "texture to buffer buffer conflict leaves texture untouched",
			seed: func(t *testing.T, enc *CommandEncoder, _, _ *Texture, buffer *Buffer) {
				mustSetBufferScopeUsage(t, enc, buffer, track.BufferUsesCopySrc)
			},
			record: func(enc *CommandEncoder, src, _ *Texture, buffer *Buffer) {
				enc.CopyTextureToBuffer(src, buffer, nil)
			},
			wantBuffer: track.BufferUsesCopySrc,
		},
		{
			name: "buffer to texture texture conflict leaves buffer untouched",
			seed: func(t *testing.T, enc *CommandEncoder, _, dst *Texture, _ *Buffer) {
				mustSetTextureScopeUsage(t, enc, dst, track.TextureUsesCopySrc)
			},
			record: func(enc *CommandEncoder, _, dst *Texture, buffer *Buffer) {
				enc.CopyBufferToTexture(buffer, dst, nil)
			},
			wantDst: track.TextureUsesCopySrc,
		},
		{
			name: "buffer to texture buffer conflict leaves texture untouched",
			seed: func(t *testing.T, enc *CommandEncoder, _, _ *Texture, buffer *Buffer) {
				mustSetBufferScopeUsage(t, enc, buffer, track.BufferUsesCopyDst)
			},
			record: func(enc *CommandEncoder, _, dst *Texture, buffer *Buffer) {
				enc.CopyBufferToTexture(buffer, dst, nil)
			},
			wantBuffer: track.BufferUsesCopyDst,
		},
		{
			name: "texture to texture source conflict leaves destination untouched",
			seed: func(t *testing.T, enc *CommandEncoder, src, _ *Texture, _ *Buffer) {
				mustSetTextureScopeUsage(t, enc, src, track.TextureUsesCopyDst)
			},
			record: func(enc *CommandEncoder, src, dst *Texture, _ *Buffer) {
				enc.CopyTextureToTexture(src, dst, nil)
			},
			wantSrc: track.TextureUsesCopyDst,
		},
		{
			name: "texture to texture destination conflict leaves source untouched",
			seed: func(t *testing.T, enc *CommandEncoder, _, dst *Texture, _ *Buffer) {
				mustSetTextureScopeUsage(t, enc, dst, track.TextureUsesCopySrc)
			},
			record: func(enc *CommandEncoder, src, dst *Texture, _ *Buffer) {
				enc.CopyTextureToTexture(src, dst, nil)
			},
			wantDst: track.TextureUsesCopySrc,
		},
		{
			name: "texture to texture same endpoint conflict leaves scope untouched",
			seed: func(_ *testing.T, _ *CommandEncoder, _, _ *Texture, _ *Buffer) {
			},
			record: func(enc *CommandEncoder, src, _ *Texture, _ *Buffer) {
				enc.CopyTextureToTexture(src, src, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, device := newTestDeviceWithTracker(t)
			defer device.Release()
			src := createCopyScopeTexture(t, device, "atomic-src", TextureUsageCopySrc|TextureUsageCopyDst)
			defer src.Release()
			dst := createCopyScopeTexture(t, device, "atomic-dst", TextureUsageCopySrc|TextureUsageCopyDst)
			defer dst.Release()
			buffer, err := device.CreateBuffer(&BufferDescriptor{
				Size: 256, Usage: BufferUsageCopySrc | BufferUsageCopyDst,
			})
			if err != nil {
				t.Fatalf("CreateBuffer: %v", err)
			}
			defer buffer.Release()

			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.seed(t, enc, src, dst, buffer)
			tt.record(enc, src, dst, buffer)

			textureScope := enc.core.Mutable().TextureScope()
			if got := textureScope.GetUsage(src.coreTexture.TrackingData().Index()); got != tt.wantSrc {
				t.Errorf("source texture usage = %v, want %v", got, tt.wantSrc)
			}
			if got := textureScope.GetUsage(dst.coreTexture.TrackingData().Index()); got != tt.wantDst {
				t.Errorf("destination texture usage = %v, want %v", got, tt.wantDst)
			}
			if got := enc.core.Mutable().BufferScope().GetUsage(buffer.core.TrackingData().Index()); got != tt.wantBuffer {
				t.Errorf("buffer usage = %v, want %v", got, tt.wantBuffer)
			}
			if got := len(enc.trackedRefs); got != 0 {
				t.Errorf("failed copy retained %d refs, want 0", got)
			}
			if len(enc.usedTextures) != 0 || len(enc.usedBuffers) != 0 {
				t.Errorf("failed copy changed submit validation sets: textures=%d buffers=%d", len(enc.usedTextures), len(enc.usedBuffers))
			}
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish succeeded after copy usage conflict")
			}
		})
	}
}

func mustSetTextureScopeUsage(t *testing.T, enc *CommandEncoder, texture *Texture, usage track.TextureUses) {
	t.Helper()
	if err := enc.core.Mutable().TextureScope().SetUsage(texture.coreTexture.TrackingData().Index(), usage); err != nil {
		t.Fatalf("seed texture scope: %v", err)
	}
}

func mustSetBufferScopeUsage(t *testing.T, enc *CommandEncoder, buffer *Buffer, usage track.BufferUses) {
	t.Helper()
	if err := enc.core.Mutable().BufferScope().SetUsage(buffer.core.TrackingData().Index(), usage); err != nil {
		t.Fatalf("seed buffer scope: %v", err)
	}
}
