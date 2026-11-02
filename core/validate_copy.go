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
