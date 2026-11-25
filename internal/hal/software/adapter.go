//go:build !(js && wasm)

package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/software/shader"
)

// Adapter implements hal.Adapter for the software backend.
type Adapter struct{}

// Open creates a software device with the requested features and limits.
// Always succeeds and returns a device/queue pair.
func (a *Adapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	return hal.OpenDevice{
		Device: &Device{},
		Queue:  &Queue{},
	}, nil
}

// TextureFormatCapabilities returns default capabilities for all formats. A
// format the backend draws color into (one with a color codec) multisamples
// too, and one that is not an integer format also resolves; depth, stencil
// and compressed formats do neither.
func (a *Adapter) TextureFormatCapabilities(format gputypes.TextureFormat) hal.TextureFormatCapabilities {
	flags := hal.TextureFormatCapabilitySampled |
		hal.TextureFormatCapabilityStorage |
		hal.TextureFormatCapabilityStorageReadWrite |
		hal.TextureFormatCapabilityRenderAttachment |
		hal.TextureFormatCapabilityBlendable
	if codec, ok := shader.NewColorCodec(format); ok {
		flags |= hal.TextureFormatCapabilityMultisample
		if !codec.Integer() {
			flags |= hal.TextureFormatCapabilityMultisampleResolve
		}
	}
	return hal.TextureFormatCapabilities{Flags: flags}
}

// SurfaceCapabilities returns default surface capabilities.
func (a *Adapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities {
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
		},
		PresentModes: []gputypes.PresentMode{
			hal.PresentModeImmediate,
			hal.PresentModeMailbox,
			hal.PresentModeFifo,
			hal.PresentModeFifoRelaxed,
		},
		AlphaModes: []gputypes.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
			hal.CompositeAlphaModeUnpremultiplied,
			hal.CompositeAlphaModeInherit,
		},
	}
}

// Destroy is a no-op for the software adapter.
func (a *Adapter) Destroy() {}
