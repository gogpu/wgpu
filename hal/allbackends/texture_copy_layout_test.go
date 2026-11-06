// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// openVulkanCopyDevice opens a Vulkan device with the validation layer and the
// given features, or skips the test. The Vulkan HAL converts the byte layout
// of a buffer-texture copy to texels (bufferRowLength, bufferImageHeight); the
// other backends have their own paths.
func openVulkanCopyDevice(t *testing.T, features gputypes.Features) *wgpu.Device {
	t.Helper()
	flags := enableVulkanValidation(t)
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsVulkan, Flags: flags})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: gputypes.PowerPreferenceLowPower})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	if info := adapter.Info(); info.Backend != gputypes.BackendVulkan {
		t.Skipf("no Vulkan adapter (got %s, backend %v)", info.Name, info.Backend)
	}
	if missing := features &^ adapter.Features(); missing != 0 {
		t.Skipf("%s lacks features %#x", adapter.Info().Name, uint64(missing))
	}
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{RequiredFeatures: features})
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	return device
}

// copyLayoutRoundTrip uploads rows of rowBytes bytes to the texture, 256 bytes
// apart, and reads them back 512 bytes apart. A backend that places the rows
// by the wrong stride (a wrong bufferRowLength) reads them back shifted, which
// a read with the same stride as the upload cannot see. blockRows is the
// number of block rows of the copy.
func copyLayoutRoundTrip(t *testing.T, device *wgpu.Device, desc *wgpu.TextureDescriptor, aspect gputypes.TextureAspect, rowBytes, blockRows int) {
	t.Helper()
	const uploadStride, readStride = 256, 512
	size := desc.Size
	tex, err := device.CreateTexture(desc)
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)

	rows := make([][]byte, blockRows)
	upload := make([]byte, uploadStride*blockRows)
	for y := range rows {
		rows[y] = make([]byte, rowBytes)
		for x := range rows[y] {
			rows[y][x] = byte(1 + y*rowBytes + x)
		}
		copy(upload[y*uploadStride:], rows[y])
	}
	src := newPublicBuffer(t, device, uint64(len(upload)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(src, 0, upload); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: uploadStride, RowsPerImage: uint32(blockRows)},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Aspect: aspect},
			Size:         size,
		}})
	})

	staging := newPublicBuffer(t, device, uint64(readStride*blockRows), wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyTextureToBuffer(tex, staging, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: readStride, RowsPerImage: uint32(blockRows)},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex, Aspect: aspect},
			Size:         size,
		}})
	})
	out := make([]byte, 0, readStride*blockRows)
	for _, w := range readPublicBuffer(t, staging, readStride*blockRows/4) {
		out = binary.LittleEndian.AppendUint32(out, w)
	}
	for y, want := range rows {
		if got := out[y*readStride : y*readStride+rowBytes]; !bytes.Equal(got, want) {
			t.Errorf("block row %d = %v, want %v", y, got, want)
		}
	}
}

// TestVulkanCompressedTextureCopyLayout: the rows of a BC1 copy (blocks of 4x4
// texels, 8 bytes) land where bytesPerRow says. bufferRowLength must be
// blockWidth * (bytesPerRow / blockSize) texels, not bytesPerRow / blockSize.
func TestVulkanCompressedTextureCopyLayout(t *testing.T) {
	device := openVulkanCopyDevice(t, gputypes.Features(gputypes.FeatureTextureCompressionBC))
	// 16x8 texels: 4 blocks of 8 bytes in a row, 2 block rows.
	copyLayoutRoundTrip(t, device, &wgpu.TextureDescriptor{
		Label:         "bc1-layout",
		Size:          wgpu.Extent3D{Width: 16, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatBC1RGBAUnorm,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc | wgpu.TextureUsageTextureBinding,
	}, gputypes.TextureAspectAll, 4*8, 2)
}

// TestVulkanStencilAspectCopyLayout: a stencil-only copy of a combined
// depth/stencil texture has one byte per texel, so bufferRowLength is
// bytesPerRow, not bytesPerRow / 4.
func TestVulkanStencilAspectCopyLayout(t *testing.T) {
	device := openVulkanCopyDevice(t, 0)
	copyLayoutRoundTrip(t, device, &wgpu.TextureDescriptor{
		Label:         "stencil-layout",
		Size:          wgpu.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth24PlusStencil8,
		Usage:         wgpu.TextureUsageCopyDst | wgpu.TextureUsageCopySrc,
	}, gputypes.TextureAspectStencilOnly, 8, 4)
}
