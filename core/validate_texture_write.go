//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// ValidateTextureWrite checks a Queue.WriteTexture upload, in the order of Rust
// wgpu-core device/queue.rs write_texture_inner (W3C writeTexture): the
// texture has COPY_DST, the copy range is valid (ValidateTextureCopyRange),
// the aspect exists, the format is copyable as a destination, the texture has
// one sample and the copy names a single aspect, and the data layout fits the
// data (ValidateLinearTextureData, with dataSize standing for the buffer
// size). Unlike a buffer-texture copy on a command encoder, the layout offset
// and bytesPerRow need no alignment: the queue stages the data itself. v is the
// destination texture view and copySize the size of the upload.
//
// The DEPTH_TEXTURE_AND_BUFFER_COPIES downlevel check Rust makes last is a
// device query: Device.RequireDepthTextureAndBufferCopies.
func ValidateTextureWrite(v TextureCopyView, layout TexelCopyBufferLayout, dataSize uint64, copySize gputypes.Extent3D) error {
	if err := validateTextureCopyUsage(CopySideDestination, v); err != nil {
		return err
	}
	if err := ValidateTextureCopyRange(CopySideDestination, v, copySize); err != nil {
		return err
	}
	if err := validateTextureCopyView(v); err != nil {
		return err
	}
	if err := validateTextureCopyDstFormat(v.Format, v.Aspect); err != nil {
		return err
	}
	if v.SampleCount != 1 {
		return &TransferError{Kind: TransferErrorInvalidSampleCount, SampleCount: v.SampleCount}
	}
	if !copyAspects(v.Format, v.Aspect).isOne() {
		return &TransferError{Kind: TransferErrorCopyAspectNotOne}
	}
	return ValidateLinearTextureData(layout, v.Format, v.Aspect, dataSize, CopySideSource, copySize)
}
