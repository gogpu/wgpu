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
