//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// CopyBytesPerRowAlignment is the alignment of bytesPerRow in a buffer-texture
// copy recorded on a command encoder (WebGPU GPUTexelCopyBufferInfo; Rust
// wgt::COPY_BYTES_PER_ROW_ALIGNMENT). Queue.WriteTexture has no such rule.
const CopyBytesPerRowAlignment = 256

// depthStencilCopyOffsetAlignment is the buffer offset alignment of a copy
// to or from a depth/stencil aspect.
const depthStencilCopyOffsetAlignment = 4

// ValidateTextureBufferCopy checks the buffer side of a buffer-texture copy
// recorded on a command encoder, as Rust wgpu-core
// validate_texture_buffer_copy with aligned = true
// (wgpu-core/src/command/transfer.rs): the texture has one sample, the buffer
// offset is a multiple of the format's texel block copy size (4 for
// depth/stencil formats), and bytesPerRow, when given (non-zero), is a
// multiple of CopyBytesPerRowAlignment.
func ValidateTextureBufferCopy(format gputypes.TextureFormat, sampleCount uint32, offset uint64, bytesPerRow uint32) error {
	if sampleCount != 1 {
		return &TransferError{Kind: TransferErrorInvalidSampleCount, SampleCount: sampleCount}
	}
	alignment := format.BlockCopySize()
	if format.IsDepthStencil() {
		alignment = depthStencilCopyOffsetAlignment
	}
	if alignment != 0 && offset%uint64(alignment) != 0 {
		return &TransferError{Kind: TransferErrorUnalignedBufferOffset, Offset: offset, Alignment: alignment}
	}
	if bytesPerRow%CopyBytesPerRowAlignment != 0 {
		return &TransferError{Kind: TransferErrorUnalignedBytesPerRow, BytesPerRow: bytesPerRow}
	}
	return nil
}

// CopyBufferAlignment is the alignment of the size and both offsets of a
// buffer-to-buffer copy (WebGPU copyBufferToBuffer; Rust
// wgt::COPY_BUFFER_ALIGNMENT).
const CopyBufferAlignment = 4

// BufferCopyEndpoint describes one side of a buffer-to-buffer copy.
type BufferCopyEndpoint struct {
	// Usage is the usage the buffer was created with.
	Usage gputypes.BufferUsage
	// Size is the buffer size in bytes.
	Size uint64
	// Offset is the byte offset the copy starts at.
	Offset uint64
}

// ValidateBufferToBufferCopy checks a copyBufferToBuffer command recorded on a
// command encoder, in the order of Rust wgpu-core
// command/transfer.rs copy_buffer_to_buffer, which covers the W3C spec's valid
// usage list: the buffers differ, the source has COPY_SRC and the destination
// COPY_DST, the source offset is inside its buffer, size and both offsets are
// multiples of CopyBufferAlignment, and both ranges fit their buffers. The
// range checks compare against the remaining bytes rather than adding offset
// and size, so a huge size cannot wrap around. sameBuffer reports that src and
// dst are one buffer.
func ValidateBufferToBufferCopy(sameBuffer bool, src, dst BufferCopyEndpoint, size uint64) error {
	if sameBuffer {
		return &TransferError{Kind: TransferErrorSameSourceDestinationBuffer}
	}
	if !src.Usage.Contains(gputypes.BufferUsageCopySrc) {
		return &TransferError{Kind: TransferErrorMissingBufferUsage, Side: CopySideSource, Usage: gputypes.BufferUsageCopySrc}
	}
	if !dst.Usage.Contains(gputypes.BufferUsageCopyDst) {
		return &TransferError{Kind: TransferErrorMissingBufferUsage, Side: CopySideDestination, Usage: gputypes.BufferUsageCopyDst}
	}
	if err := validateBufferCopyStart(CopySideSource, src); err != nil {
		return err
	}
	if err := validateBufferCopyAlignment(src.Offset, dst.Offset, size); err != nil {
		return err
	}
	if err := validateBufferCopyEnd(CopySideSource, src, size); err != nil {
		return err
	}
	if err := validateBufferCopyStart(CopySideDestination, dst); err != nil {
		return err
	}
	return validateBufferCopyEnd(CopySideDestination, dst, size)
}

// validateBufferCopyAlignment checks size, source offset and destination
// offset, in that order (Rust: UnalignedCopySize, then UnalignedBufferOffset).
func validateBufferCopyAlignment(srcOffset, dstOffset, size uint64) error {
	if size%CopyBufferAlignment != 0 {
		return &TransferError{Kind: TransferErrorUnalignedCopySize, Size: size}
	}
	if srcOffset%CopyBufferAlignment != 0 {
		return &TransferError{Kind: TransferErrorUnalignedBufferOffset, Offset: srcOffset, Alignment: CopyBufferAlignment}
	}
	if dstOffset%CopyBufferAlignment != 0 {
		return &TransferError{Kind: TransferErrorUnalignedBufferOffset, Offset: dstOffset, Alignment: CopyBufferAlignment}
	}
	return nil
}

// validateBufferCopyStart checks that the copy offset is inside the buffer.
func validateBufferCopyStart(side CopySide, e BufferCopyEndpoint) error {
	if e.Offset > e.Size {
		return &TransferError{Kind: TransferErrorBufferStartOffsetOverrun, Side: side, Offset: e.Offset, BufferSize: e.Size}
	}
	return nil
}

// validateBufferCopyEnd checks that size bytes fit after the copy offset. The
// offset is already known to be inside the buffer, so the subtraction cannot
// underflow.
func validateBufferCopyEnd(side CopySide, e BufferCopyEndpoint, size uint64) error {
	if size > e.Size-e.Offset {
		return &TransferError{Kind: TransferErrorBufferEndOffsetOverrun, Side: side, Offset: e.Offset, Size: size, BufferSize: e.Size}
	}
	return nil
}
