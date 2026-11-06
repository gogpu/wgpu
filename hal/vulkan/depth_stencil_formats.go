//go:build !(js && wasm)

package vulkan

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// depthStencilFormats records which of the depth/stencil formats Vulkan does
// not guarantee a physical device supports as optimal-tiling, sampled
// depth-stencil attachments (Rust wgpu-hal PrivateCapabilities texture_d24,
// texture_d24_s8 and texture_s8, vulkan/adapter.rs). Vulkan guarantees
// D16_UNORM, at least one of X8_D24_UNORM_PACK32 and D32_SFLOAT, and at least
// one of D24_UNORM_S8_UINT and D32_SFLOAT_S8_UINT (Vulkan spec, "Required
// Format Support"); S8_UINT is optional (Intel Iris Xe lacks it, AMD lacks
// both 24-bit depth formats).
type depthStencilFormats struct {
	d24   bool // X8_D24_UNORM_PACK32
	d24s8 bool // D24_UNORM_S8_UINT
	s8    bool // S8_UINT
}

// depthStencilRequiredFeatures is what a depth/stencil format must support to
// back a WebGPU texture (Rust depth_stencil_required_flags).
const depthStencilRequiredFeatures = vk.FormatFeatureFlags(vk.FormatFeatureSampledImageBit | vk.FormatFeatureDepthStencilAttachmentBit)

// queryDepthStencilFormats asks the physical device which optional
// depth/stencil formats it supports.
func queryDepthStencilFormats(cmds *vk.Commands, physicalDevice vk.PhysicalDevice) depthStencilFormats {
	supports := func(format vk.Format) bool {
		var props vk.FormatProperties
		cmds.GetPhysicalDeviceFormatProperties(physicalDevice, format, &props)
		return props.OptimalTilingFeatures&depthStencilRequiredFeatures == depthStencilRequiredFeatures
	}
	return depthStencilFormats{
		d24:   supports(vk.FormatX8D24UnormPack32),
		d24s8: supports(vk.FormatD24UnormS8Uint),
		s8:    supports(vk.FormatS8Uint),
	}
}

// textureFormatToVk maps a WebGPU format to the Vulkan format of its image,
// choosing among the depth/stencil formats the device supports, as Rust
// PrivateCapabilities::map_texture_format does (vulkan/conv.rs):
//   - Depth24Plus: X8_D24_UNORM_PACK32, else D32_SFLOAT (WebGPU asks for at
//     least 24 bits of depth);
//   - Depth24PlusStencil8: D24_UNORM_S8_UINT, else D32_SFLOAT_S8_UINT;
//   - Stencil8: S8_UINT, else D24_UNORM_S8_UINT, else D32_SFLOAT_S8_UINT.
//
// A Stencil8 texture on a combined image keeps WebGPU's view of it: views and
// buffer-texture copies name STENCIL only (textureAspectToVk), and only
// barriers add the hidden DEPTH aspect (barrierAspects). Every other format
// maps as the package-level textureFormatToVk does.
func (s depthStencilFormats) textureFormatToVk(format gputypes.TextureFormat) vk.Format {
	switch format {
	case gputypes.TextureFormatDepth24Plus:
		if s.d24 {
			return vk.FormatX8D24UnormPack32
		}
		return vk.FormatD32Sfloat
	case gputypes.TextureFormatDepth24PlusStencil8:
		if s.d24s8 {
			return vk.FormatD24UnormS8Uint
		}
		return vk.FormatD32SfloatS8Uint
	case gputypes.TextureFormatStencil8:
		switch {
		case s.s8:
			return vk.FormatS8Uint
		case s.d24s8:
			return vk.FormatD24UnormS8Uint
		default:
			return vk.FormatD32SfloatS8Uint
		}
	default:
		return textureFormatToVk(format)
	}
}

// barrierAspects returns the aspects an image barrier of a texture of format
// covers. A Stencil8 texture emulated on a combined depth-stencil image also
// transitions its hidden depth aspect: without separateDepthStencilLayouts a
// barrier of such an image must name both (VUID-VkImageMemoryBarrier-image-03320;
// Rust conv.rs map_subresource_range_combined_aspect).
func (s depthStencilFormats) barrierAspects(aspect gputypes.TextureAspect, format gputypes.TextureFormat) vk.ImageAspectFlags {
	flags := textureAspectToVk(aspect, format)
	if format == gputypes.TextureFormatStencil8 && !s.s8 {
		flags |= vk.ImageAspectFlags(vk.ImageAspectDepthBit)
	}
	return flags
}
