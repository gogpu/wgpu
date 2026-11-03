//go:build !(js && wasm)

package core

import (
	"math/bits"

	"github.com/gogpu/gputypes"
)

// TexelCopyBufferLayout is the layout of the texel data in the buffer of a
// buffer-texture copy (WebGPU GPUTexelCopyBufferLayout). A zero BytesPerRow or
// RowsPerImage means "not specified", like an undefined member in WebGPU.
type TexelCopyBufferLayout struct {
	// Offset is the byte offset of the first texel block.
	Offset uint64
	// BytesPerRow is the stride between rows, in bytes.
	BytesPerRow uint32
	// RowsPerImage is the number of rows between images.
	RowsPerImage uint32
}

// ValidateLinearTextureData checks the buffer layout of a buffer-texture copy
// against the copy size and the buffer, as Rust wgpu-core command/transfer.rs
// validate_linear_texture_data with wgpu-types get_buffer_texture_copy_info
// (W3C "validating linear texture data"): bytesPerRow holds a complete row and
// rowsPerImage a complete image, both are given when more than one row or
// image is copied, the byte size of the copy does not overflow, and the copy
// lies inside the buffer. bufferSide is the buffer's side of the copy (it
// only labels the error). The format must be copyable with the aspect; a
// combined depth/stencil format needs a single aspect.
//
// Rust also checks that the copy width and height are multiples of the block
// size here; ValidateTextureCopyRange reports that first for every copy, so
// it is not repeated.
func ValidateLinearTextureData(layout TexelCopyBufferLayout, format gputypes.TextureFormat, aspect gputypes.TextureAspect, bufferSize uint64, bufferSide CopySide, copySize gputypes.Extent3D) error {
	blockSize, ok := format.BlockCopySizeAspect(aspect)
	if !ok {
		return &TransferError{Kind: TransferErrorCopyAspectNotOne}
	}
	bw, bh := format.BlockDimensions()
	widthBlocks := (uint64(copySize.Width) + uint64(bw) - 1) / uint64(bw)
	heightBlocks := (uint64(copySize.Height) + uint64(bh) - 1) / uint64(bh)
	layers := uint64(copySize.DepthOrArrayLayers)

	// The spec calls this bytesInLastRow. Cannot overflow: 2^32 * 16 < 2^64.
	rowBytesDense := widthBlocks * uint64(blockSize)
	rowStride := rowBytesDense
	if layout.BytesPerRow != 0 {
		if uint64(layout.BytesPerRow) < rowBytesDense {
			return &TransferError{Kind: TransferErrorInvalidBytesPerRow, BytesPerRow: layout.BytesPerRow}
		}
		rowStride = uint64(layout.BytesPerRow)
	}
	imageStrideRows := heightBlocks
	if layout.RowsPerImage != 0 {
		if uint64(layout.RowsPerImage) < heightBlocks {
			return &TransferError{Kind: TransferErrorInvalidRowsPerImage}
		}
		imageStrideRows = uint64(layout.RowsPerImage)
	}

	bytesInCopy, ok := bytesInLinearCopy(rowBytesDense, rowStride, heightBlocks, imageStrideRows, layers)
	if !ok {
		return &TransferError{Kind: TransferErrorSizeOverflow}
	}

	// get_buffer_texture_copy_info proceeds with defaults for an unspecified
	// bytesPerRow / rowsPerImage; WebGPU requires them whenever they apply.
	if layout.BytesPerRow == 0 && (layers > 1 || heightBlocks > 1) {
		return &TransferError{Kind: TransferErrorUnspecifiedBytesPerRow}
	}
	if layout.RowsPerImage == 0 && layers > 1 {
		return &TransferError{Kind: TransferErrorUnspecifiedRowsPerImage}
	}

	if layout.Offset > bufferSize {
		return &TransferError{
			Kind: TransferErrorBufferStartOffsetOverrun, Side: bufferSide,
			Offset: layout.Offset, BufferSize: bufferSize,
		}
	}
	// Cannot underflow: the offset is at most the buffer size.
	if bytesInCopy > bufferSize-layout.Offset {
		return &TransferError{
			Kind: TransferErrorBufferEndOffsetOverrun, Side: bufferSide,
			Offset: layout.Offset, Size: bytesInCopy, BufferSize: bufferSize,
		}
	}
	return nil
}

// bytesInLinearCopy returns the number of buffer bytes a copy covers: every
// padding byte except the padding after the last row (Rust
// BufferTextureCopyInfo::bytes_in_copy). ok is false when a product or sum
// does not fit in 64 bits.
func bytesInLinearCopy(rowBytesDense, rowStride, heightBlocks, imageStrideRows, layers uint64) (bytes uint64, ok bool) {
	var imageBytesDense uint64
	if heightBlocks > 0 {
		lastRows, ok := mulU64(heightBlocks-1, rowStride)
		if !ok {
			return 0, false
		}
		if imageBytesDense, ok = addU64(lastRows, rowBytesDense); !ok {
			return 0, false
		}
	}
	// The image stride can overflow while the copy itself does not (one layer
	// never uses it); Rust reports that as an overflow too.
	imageStride, ok := mulU64(rowStride, imageStrideRows)
	if !ok {
		return 0, false
	}
	if layers <= 1 {
		return layers * imageBytesDense, true
	}
	allButLast, ok := mulU64(layers-1, imageStride)
	if !ok {
		return 0, false
	}
	return addU64(allButLast, imageBytesDense)
}

func mulU64(a, b uint64) (uint64, bool) {
	hi, lo := bits.Mul64(a, b)
	return lo, hi == 0
}

func addU64(a, b uint64) (uint64, bool) {
	sum, carry := bits.Add64(a, b, 0)
	return sum, carry == 0
}
