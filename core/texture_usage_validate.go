//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
)

// ValidateTextureUsageFlags validates texture usage flag combinations at creation time.
// Reference: wgpu-core device/resource.rs texture usage validation (VAL-C14..C18).
//
// Note: VAL-C17 (storage + render attachment) is intentionally absent — the W3C
// WebGPU spec and Rust wgpu allow that combination at creation; conflicts are
// detected at bind-time and draw-time.
func ValidateTextureUsageFlags(
	usage gputypes.TextureUsage,
	sampleCount uint32,
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

	// VAL-C16: Depth/stencil formats cannot be used as storage textures.
	// RENDER_ATTACHMENT is valid — W3C uses it for both color and depth/stencil attachments.
	if format.IsDepthStencil() {
		if usage.Contains(gputypes.TextureUsageStorageBinding) {
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
