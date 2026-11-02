//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestValidateTextureBufferCopy pins Rust wgpu-core
// validate_texture_buffer_copy for command encoder copies
// (wgpu-core/src/command/transfer.rs): one sample, a buffer offset aligned
// to the texel block copy size (4 for depth/stencil), and a bytesPerRow that
// is a multiple of 256 whenever it is given.
func TestValidateTextureBufferCopy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		format      gputypes.TextureFormat
		samples     uint32
		offset      uint64
		bytesPerRow uint32
		want        TransferErrorKind
		wantErr     bool
	}{
		{name: "aligned", format: gputypes.TextureFormatRGBA8Unorm, samples: 1, offset: 512, bytesPerRow: 256},
		{name: "bytesPerRow omitted", format: gputypes.TextureFormatRGBA8Unorm, samples: 1},
		{name: "bytesPerRow 32", format: gputypes.TextureFormatRGBA8Unorm, samples: 1, bytesPerRow: 32,
			want: TransferErrorUnalignedBytesPerRow, wantErr: true},
		{name: "bytesPerRow 260", format: gputypes.TextureFormatR8Unorm, samples: 1, bytesPerRow: 260,
			want: TransferErrorUnalignedBytesPerRow, wantErr: true},
		{name: "offset 2 for 4-byte texels", format: gputypes.TextureFormatRGBA8Unorm, samples: 1, offset: 2, bytesPerRow: 256,
			want: TransferErrorUnalignedBufferOffset, wantErr: true},
		{name: "offset 1 for 1-byte texels", format: gputypes.TextureFormatR8Unorm, samples: 1, offset: 1, bytesPerRow: 256},
		{name: "offset 2 for depth", format: gputypes.TextureFormatDepth16Unorm, samples: 1, offset: 2, bytesPerRow: 256,
			want: TransferErrorUnalignedBufferOffset, wantErr: true},
		{name: "multisampled", format: gputypes.TextureFormatRGBA8Unorm, samples: 4, bytesPerRow: 256,
			want: TransferErrorInvalidSampleCount, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTextureBufferCopy(tt.format, tt.samples, tt.offset, tt.bytesPerRow)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var te *TransferError
			if !errors.As(err, &te) || te.Kind != tt.want {
				t.Fatalf("error %v (%T), want *TransferError kind %d", err, err, tt.want)
			}
			if te.Error() == "" {
				t.Fatal("empty error text")
			}
		})
	}
}

// TestValidateBufferToBufferCopy pins Rust wgpu-core copy_buffer_to_buffer
// (wgpu-core/src/command/transfer.rs) and the W3C copyBufferToBuffer valid
// usage list: every check fails with its own kind, and the checks run in
// Rust's order.
func TestValidateBufferToBufferCopy(t *testing.T) {
	t.Parallel()
	const (
		srcUsage = gputypes.BufferUsageCopySrc
		dstUsage = gputypes.BufferUsageCopyDst
		maxU64   = ^uint64(0)
	)
	src := func(offset, size uint64) BufferCopyEndpoint {
		return BufferCopyEndpoint{Usage: srcUsage, Size: size, Offset: offset}
	}
	dst := func(offset, size uint64) BufferCopyEndpoint {
		return BufferCopyEndpoint{Usage: dstUsage, Size: size, Offset: offset}
	}
	tests := []struct {
		name     string
		same     bool
		src, dst BufferCopyEndpoint
		size     uint64
		want     TransferErrorKind
		side     CopySide
		wantErr  bool
	}{
		{name: "valid", src: src(0, 64), dst: dst(0, 64), size: 64},
		{name: "valid with offsets", src: src(8, 64), dst: dst(16, 64), size: 32},
		{name: "valid zero size at the end", src: src(64, 64), dst: dst(64, 64), size: 0},
		{name: "same buffer", same: true, src: src(0, 64), dst: dst(32, 64), size: 16,
			want: TransferErrorSameSourceDestinationBuffer, wantErr: true},
		{name: "source without COPY_SRC", src: BufferCopyEndpoint{Usage: gputypes.BufferUsageStorage, Size: 64},
			dst: dst(0, 64), size: 16, want: TransferErrorMissingBufferUsage, side: CopySideSource, wantErr: true},
		{name: "destination without COPY_DST", src: src(0, 64), dst: BufferCopyEndpoint{Usage: srcUsage, Size: 64},
			size: 16, want: TransferErrorMissingBufferUsage, side: CopySideDestination, wantErr: true},
		{name: "size not a multiple of 4", src: src(0, 64), dst: dst(0, 64), size: 6,
			want: TransferErrorUnalignedCopySize, wantErr: true},
		{name: "source offset not a multiple of 4", src: src(2, 64), dst: dst(0, 64), size: 8,
			want: TransferErrorUnalignedBufferOffset, wantErr: true},
		{name: "destination offset not a multiple of 4", src: src(0, 64), dst: dst(6, 64), size: 8,
			want: TransferErrorUnalignedBufferOffset, wantErr: true},
		{name: "source offset past the end", src: src(68, 64), dst: dst(0, 64), size: 0,
			want: TransferErrorBufferStartOffsetOverrun, side: CopySideSource, wantErr: true},
		{name: "source range past the end", src: src(48, 64), dst: dst(0, 64), size: 20,
			want: TransferErrorBufferEndOffsetOverrun, side: CopySideSource, wantErr: true},
		{name: "destination offset past the end", src: src(0, 64), dst: dst(68, 64), size: 0,
			want: TransferErrorBufferStartOffsetOverrun, side: CopySideDestination, wantErr: true},
		{name: "destination range past the end", src: src(0, 64), dst: dst(48, 64), size: 20,
			want: TransferErrorBufferEndOffsetOverrun, side: CopySideDestination, wantErr: true},
		{name: "size overflows the source range", src: src(8, 64), dst: dst(0, 64), size: maxU64 - 3,
			want: TransferErrorBufferEndOffsetOverrun, side: CopySideSource, wantErr: true},
		{name: "size overflows the destination range", src: src(0, maxU64&^3), dst: dst(8, 64), size: maxU64 - 3,
			want: TransferErrorBufferEndOffsetOverrun, side: CopySideDestination, wantErr: true},
		// Rust order: the source start offset is checked before alignment.
		{name: "source start overrun before alignment", src: src(68, 64), dst: dst(0, 64), size: 6,
			want: TransferErrorBufferStartOffsetOverrun, side: CopySideSource, wantErr: true},
		// Rust order: usage is checked before alignment and ranges.
		{name: "usage before alignment", src: BufferCopyEndpoint{Size: 64}, dst: dst(2, 64), size: 6,
			want: TransferErrorMissingBufferUsage, side: CopySideSource, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateBufferToBufferCopy(tt.same, tt.src, tt.dst, tt.size)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var te *TransferError
			if !errors.As(err, &te) || te.Kind != tt.want {
				t.Fatalf("error %v (%T), want *TransferError kind %d", err, err, tt.want)
			}
			if te.Side != tt.side {
				t.Errorf("side = %v, want %v", te.Side, tt.side)
			}
			if te.Error() == "" {
				t.Fatal("empty error text")
			}
		})
	}
}

// TestTransferErrorBufferCopyText: every buffer-copy kind has its own message.
func TestTransferErrorBufferCopyText(t *testing.T) {
	t.Parallel()
	seen := map[string]TransferErrorKind{}
	for _, kind := range []TransferErrorKind{
		TransferErrorSameSourceDestinationBuffer,
		TransferErrorMissingBufferUsage,
		TransferErrorUnalignedCopySize,
		TransferErrorBufferStartOffsetOverrun,
		TransferErrorBufferEndOffsetOverrun,
	} {
		text := (&TransferError{Kind: kind}).Error()
		if text == "copy: unknown error" {
			t.Errorf("kind %d has no message", kind)
		}
		if other, dup := seen[text]; dup {
			t.Errorf("kinds %d and %d share the message %q", other, kind, text)
		}
		seen[text] = kind
	}
}
