package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
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

// TextureFormatCapabilities returns default capabilities for all formats.
func (a *Adapter) TextureFormatCapabilities(_ gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return hal.TextureFormatCapabilities{
		Flags: hal.TextureFormatCapabilitySampled |
			hal.TextureFormatCapabilityStorage |
			hal.TextureFormatCapabilityStorageReadWrite |
			hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable |
			hal.TextureFormatCapabilityMultisample |
			hal.TextureFormatCapabilityMultisampleResolve,
	}
}

// SurfaceCapabilities returns default surface capabilities.
func (a *Adapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities {
	return &hal.SurfaceCapabilities{
		Formats: []gputypes.TextureFormat{
			gputypes.TextureFormatBGRA8Unorm,
			gputypes.TextureFormatRGBA8Unorm,
		},
		PresentModes: []hal.PresentMode{
			hal.PresentModeImmediate,
			hal.PresentModeMailbox,
			hal.PresentModeFifo,
			hal.PresentModeFifoRelaxed,
		},
		AlphaModes: []hal.CompositeAlphaMode{
			hal.CompositeAlphaModeOpaque,
			hal.CompositeAlphaModePremultiplied,
			hal.CompositeAlphaModeUnpremultiplied,
			hal.CompositeAlphaModeInherit,
		},
	}
}

// Destroy is a no-op for the software adapter.
func (a *Adapter) Destroy() {}
