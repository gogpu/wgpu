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

// TestCopyBufferToBufferValidation: CopyBufferToBuffer enforces the WebGPU
// copyBufferToBuffer valid usage list (Rust wgpu-core transfer.rs
// copy_buffer_to_buffer); Finish reports a *core.TransferError of the kind of
// the first failed check (P1-VAL-014).
func TestCopyBufferToBufferValidation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newBuffer := func(size uint64, usage gputypes.BufferUsage) *wgpu.Buffer {
		t.Helper()
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: size, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(buf.Release)
		return buf
	}
	const copyBoth = wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst
	src := newBuffer(64, copyBoth)
	dst := newBuffer(64, copyBoth)
	storageOnly := newBuffer(64, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
	srcOnly := newBuffer(64, wgpu.BufferUsageCopySrc)

	const maxU64 = ^uint64(0)
	tests := []struct {
		name       string
		src, dst   *wgpu.Buffer
		srcOffset  uint64
		dstOffset  uint64
		size       uint64
		wantKind   core.TransferErrorKind
		wantSide   core.CopySide
		wantFailed bool
	}{
		{name: "valid", src: src, dst: dst, size: 64},
		{name: "valid with offsets", src: src, dst: dst, srcOffset: 8, dstOffset: 16, size: 32},
		{name: "same buffer", src: src, dst: src, size: 16,
			wantKind: core.TransferErrorSameSourceDestinationBuffer, wantFailed: true},
		{name: "source without COPY_SRC", src: storageOnly, dst: dst, size: 16,
			wantKind: core.TransferErrorMissingBufferUsage, wantSide: core.CopySideSource, wantFailed: true},
		{name: "destination without COPY_DST", src: src, dst: srcOnly, size: 16,
			wantKind: core.TransferErrorMissingBufferUsage, wantSide: core.CopySideDestination, wantFailed: true},
		{name: "size not a multiple of 4", src: src, dst: dst, size: 6,
			wantKind: core.TransferErrorUnalignedCopySize, wantFailed: true},
		{name: "source offset not a multiple of 4", src: src, dst: dst, srcOffset: 2, size: 8,
			wantKind: core.TransferErrorUnalignedBufferOffset, wantFailed: true},
		{name: "destination offset not a multiple of 4", src: src, dst: dst, dstOffset: 6, size: 8,
			wantKind: core.TransferErrorUnalignedBufferOffset, wantFailed: true},
		{name: "source range past the end", src: src, dst: dst, srcOffset: 48, size: 20,
			wantKind: core.TransferErrorBufferEndOffsetOverrun, wantSide: core.CopySideSource, wantFailed: true},
		{name: "destination range past the end", src: src, dst: dst, dstOffset: 48, size: 20,
			wantKind: core.TransferErrorBufferEndOffsetOverrun, wantSide: core.CopySideDestination, wantFailed: true},
		{name: "source offset past the end", src: src, dst: dst, srcOffset: 68, size: 0,
			wantKind: core.TransferErrorBufferStartOffsetOverrun, wantSide: core.CopySideSource, wantFailed: true},
		{name: "size overflows the offset", src: src, dst: dst, srcOffset: 8, size: maxU64 - 3,
			wantKind: core.TransferErrorBufferEndOffsetOverrun, wantSide: core.CopySideSource, wantFailed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			enc.CopyBufferToBuffer(tc.src, tc.srcOffset, tc.dst, tc.dstOffset, tc.size)
			cmd, err := enc.Finish()
			if !tc.wantFailed {
				if err != nil {
					t.Fatalf("Finish: %v", err)
				}
				cmd.Release()
				return
			}
			var te *core.TransferError
			if !errors.As(err, &te) {
				t.Fatalf("Finish error = %v (%T), want *core.TransferError", err, err)
			}
			if te.Kind != tc.wantKind || te.Side != tc.wantSide {
				t.Fatalf("TransferError kind %d side %v, want kind %d side %v (%v)",
					te.Kind, te.Side, tc.wantKind, tc.wantSide, err)
			}
		})
	}
}

// TestClearBufferValidation: CommandEncoder.ClearBuffer enforces the WebGPU
// clearBuffer valid usage list (Rust wgpu-core clear.rs clear_buffer); Finish
// reports a *core.ClearError of the kind of the first failed check
// (P1-VAL-014). A released or nil buffer is an error, never a nil HAL buffer
// handed to the backend.
func TestClearBufferValidation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	newBuffer := func(usage gputypes.BufferUsage) *wgpu.Buffer {
		t.Helper()
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 64, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(buf.Release)
		return buf
	}
	dst := newBuffer(wgpu.BufferUsageCopyDst)
	srcOnly := newBuffer(wgpu.BufferUsageCopySrc)

	const maxU64 = ^uint64(0)
	tests := []struct {
		name       string
		buffer     *wgpu.Buffer
		offset     uint64
		size       uint64
		wantKind   core.ClearErrorKind
		wantFailed bool
	}{
		{name: "valid whole buffer", buffer: dst, size: 64},
		{name: "valid range", buffer: dst, offset: 8, size: 16},
		{name: "valid zero size", buffer: dst, offset: 64},
		{name: "without COPY_DST", buffer: srcOnly, size: 16,
			wantKind: core.ClearErrorMissingBufferUsage, wantFailed: true},
		{name: "offset not a multiple of 4", buffer: dst, offset: 2, size: 8,
			wantKind: core.ClearErrorUnalignedBufferOffset, wantFailed: true},
		{name: "size not a multiple of 4", buffer: dst, size: 6,
			wantKind: core.ClearErrorUnalignedFillSize, wantFailed: true},
		{name: "offset plus size wraps", buffer: dst, offset: 8, size: maxU64 - 3,
			wantKind: core.ClearErrorOffsetPlusSizeExceeds64BitBounds, wantFailed: true},
		{name: "range past the end", buffer: dst, offset: 48, size: 20,
			wantKind: core.ClearErrorBufferOverrun, wantFailed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			enc.ClearBuffer(tc.buffer, tc.offset, tc.size)
			cmd, err := enc.Finish()
			if !tc.wantFailed {
				if err != nil {
					t.Fatalf("Finish: %v", err)
				}
				cmd.Release()
				return
			}
			var ce *core.ClearError
			if !errors.As(err, &ce) || ce.Kind != tc.wantKind {
				t.Fatalf("Finish error = %v (%T), want *core.ClearError kind %d", err, err, tc.wantKind)
			}
		})
	}

	t.Run("nil buffer", func(t *testing.T) {
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.ClearBuffer(nil, 0, 4)
		if _, err := enc.Finish(); err == nil {
			t.Fatal("Finish should fail for a nil buffer")
		}
	})

	t.Run("released buffer", func(t *testing.T) {
		released, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 64, Usage: wgpu.BufferUsageCopyDst})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		released.Release()
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.ClearBuffer(released, 0, 4)
		if _, err := enc.Finish(); !errors.Is(err, wgpu.ErrReleased) {
			t.Fatalf("Finish error = %v, want ErrReleased", err)
		}
	})
}
