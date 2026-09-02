//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
)

// ValidateTextureUsageFlags validates texture usage flag combinations at creation time.
// Reference: wgpu-core device/resource.rs texture usage validation (VAL-C14..C18).
func ValidateTextureUsageFlags(
	usage gputypes.TextureUsage,
	sampleCount uint32,
	dimension gputypes.TextureDimension,
	format gputypes.TextureFormat,
) error {
	if usage.ContainsUnknownBits() {
		return &CreateTextureError{
			Kind: CreateTextureErrorInvalidUsage,
		}
	}

	// VAL-C15: Compressed formats only support copy + sampled usages.
	if isBCFormat(format) || isETC2Format(format) || isASTCFormat(format) {
		const allowed = gputypes.TextureUsageCopySrc |
			gputypes.TextureUsageCopyDst |
			gputypes.TextureUsageTextureBinding
		if usage&^allowed != 0 {
			return &CreateTextureError{
				Kind: CreateTextureErrorInvalidUsage,
			}
		}
	}

	// VAL-C16: Depth/stencil formats cannot be used as color render targets or storage.
	if format.IsDepthStencil() {
		const forbidden = gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageStorageBinding
		if usage&forbidden != 0 {
			return &CreateTextureError{
				Kind: CreateTextureErrorInvalidUsage,
			}
		}
	}

	// VAL-C17: 1D/2D array and 3D cannot combine storage with render attachment.
	if dimension != gputypes.TextureDimensionUndefined {
		if usage.Contains(gputypes.TextureUsageStorageBinding) &&
			usage.Contains(gputypes.TextureUsageRenderAttachment) {
			return &CreateTextureError{
				Kind: CreateTextureErrorInvalidUsage,
			}
		}
	}

	// VAL-C18: Multisampled storage binding is rejected in validateTextureMultisample;
	// reinforce here for callers invoking ValidateTextureUsageFlags directly.
	if sampleCount > 1 && usage.Contains(gputypes.TextureUsageStorageBinding) {
		return &CreateTextureError{
			Kind: CreateTextureErrorMultisampleStorageBinding,
		}
	}

	return nil
}
