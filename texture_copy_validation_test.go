//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

// TestTextureCopyValidation: the texture copy commands enforce the WebGPU
// valid usage that does not need the copy range: COPY_SRC / COPY_DST usage on
// both resources, existing mip level and aspect, copyable formats, a single
// aspect for buffer-texture copies, all aspects, equal sample counts and
// disjoint subresources for texture-to-texture copies (Rust wgpu-core
// transfer.rs; P1-VAL-014 step 2). Finish reports a *core.TransferError.
func TestTextureCopyValidation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newTexture := func(format gputypes.TextureFormat, usage gputypes.TextureUsage, mips uint32) *wgpu.Texture {
		t.Helper()
		tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 4},
			MipLevelCount: mips,
			SampleCount:   1,
			Dimension:     wgpu.TextureDimension2D,
			Format:        format,
			Usage:         usage,
		})
		if err != nil {
			t.Fatalf("CreateTexture(%v): %v", format, err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	newBuffer := func(usage gputypes.BufferUsage) *wgpu.Buffer {
		t.Helper()
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 4096, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(buf.Release)
		return buf
	}
	const copyBoth = wgpu.TextureUsageCopySrc | wgpu.TextureUsageCopyDst
	color := newTexture(gputypes.TextureFormatRGBA8Unorm, copyBoth, 2)
	srcOnly := newTexture(gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageCopySrc, 1)
	dstOnly := newTexture(gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageCopyDst, 1)
	dstOnly2 := newTexture(gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageCopyDst, 1)
	srcOnly2 := newTexture(gputypes.TextureFormatRGBA8Unorm, wgpu.TextureUsageCopySrc, 1)
	combined := newTexture(gputypes.TextureFormatDepth24PlusStencil8, copyBoth, 1)
	depth24 := newTexture(gputypes.TextureFormatDepth24Plus, copyBoth, 1)
	bufBoth := newBuffer(wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst)
	bufNone := newBuffer(wgpu.BufferUsageVertex)

	region := func(mip uint32, aspect gputypes.TextureAspect) []wgpu.BufferTextureCopy {
		return []wgpu.BufferTextureCopy{{
			TextureBase:  wgpu.ImageCopyTexture{MipLevel: mip, Aspect: aspect},
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
			Size:         wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		}}
	}
	texRegion := func(src, dst *wgpu.Texture, srcMip, dstMip, srcZ, dstZ uint32, aspect gputypes.TextureAspect) []wgpu.TextureCopy {
		return []wgpu.TextureCopy{{
			Source:      wgpu.ImageCopyTexture{Texture: src, MipLevel: srcMip, Origin: wgpu.Origin3D{Z: srcZ}, Aspect: aspect},
			Destination: wgpu.ImageCopyTexture{Texture: dst, MipLevel: dstMip, Origin: wgpu.Origin3D{Z: dstZ}, Aspect: aspect},
			Size:        wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 2},
		}}
	}
	all := gputypes.TextureAspectAll

	tests := []struct {
		name     string
		record   func(enc *wgpu.CommandEncoder)
		wantKind core.TransferErrorKind
		wantErr  bool
	}{
		{name: "texture to buffer", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(color, bufBoth, region(0, all))
		}},
		{name: "texture to buffer, texture without COPY_SRC", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(dstOnly, bufBoth, region(0, all))
		}, wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
		{name: "texture to buffer, buffer without COPY_DST", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(color, bufNone, region(0, all))
		}, wantKind: core.TransferErrorMissingBufferUsage, wantErr: true},
		{name: "texture to buffer, mip level out of range", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(color, bufBoth, region(2, all))
		}, wantKind: core.TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "texture to buffer, depth aspect of a color texture", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(color, bufBoth, region(0, gputypes.TextureAspectDepthOnly))
		}, wantKind: core.TransferErrorInvalidTextureAspect, wantErr: true},
		{name: "texture to buffer, depth24plus", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(depth24, bufBoth, region(0, all))
		}, wantKind: core.TransferErrorCopyFromForbiddenTextureFormat, wantErr: true},
		{name: "texture to buffer, all aspects of a combined format", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToBuffer(combined, bufBoth, region(0, all))
		}, wantKind: core.TransferErrorCopyAspectNotOne, wantErr: true},
		{name: "buffer to texture", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(bufBoth, color, region(0, all))
		}},
		{name: "buffer to texture, texture without COPY_DST", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(bufBoth, srcOnly, region(0, all))
		}, wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
		{name: "buffer to texture, buffer without COPY_SRC", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(bufNone, color, region(0, all))
		}, wantKind: core.TransferErrorMissingBufferUsage, wantErr: true},
		{name: "buffer to texture, depth24plus", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyBufferToTexture(bufBoth, depth24, region(0, all))
		}, wantKind: core.TransferErrorCopyToForbiddenTextureFormat, wantErr: true},
		{name: "texture to texture", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(srcOnly, dstOnly, texRegion(srcOnly, dstOnly, 0, 0, 0, 0, all))
		}},
		{name: "texture to texture, source without COPY_SRC", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(dstOnly, dstOnly2, texRegion(dstOnly, dstOnly2, 0, 0, 0, 0, all))
		}, wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
		{name: "texture to texture, destination without COPY_DST", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(srcOnly, srcOnly2, texRegion(srcOnly, srcOnly2, 0, 0, 0, 0, all))
		}, wantKind: core.TransferErrorMissingTextureUsage, wantErr: true},
		{name: "texture to texture, one aspect of a combined format", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(combined, combined, texRegion(combined, combined, 0, 0, 0, 2, gputypes.TextureAspectStencilOnly))
		}, wantKind: core.TransferErrorCopySrcMissingAspects, wantErr: true},
		{name: "texture to texture, mip level out of range", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(color, color, texRegion(color, color, 0, 5, 0, 0, all))
		}, wantKind: core.TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "texture to texture, same subresource", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(color, color, texRegion(color, color, 0, 0, 0, 0, all))
		}, wantKind: core.TransferErrorInvalidCopyWithinSameTexture, wantErr: true},
		{name: "texture to texture, overlapping layers", record: func(enc *wgpu.CommandEncoder) {
			enc.CopyTextureToTexture(color, color, texRegion(color, color, 0, 0, 0, 1, all))
		}, wantKind: core.TransferErrorInvalidCopyWithinSameTexture, wantErr: true},
		// A copy between disjoint subresources of one texture is valid in WebGPU
		// (core.ValidateTextureToTextureCopy accepts it), but the encoder's usage
		// tracking is per texture and reports COPY_SRC against COPY_DST as a
		// conflict, so it is not covered here.
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tc.record(enc)
			cmd, err := enc.Finish()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Finish: %v", err)
				}
				cmd.Release()
				return
			}
			var te *core.TransferError
			if !errors.As(err, &te) || te.Kind != tc.wantKind {
				t.Fatalf("Finish error = %v (%T), want *core.TransferError kind %d", err, err, tc.wantKind)
			}
		})
	}
}
