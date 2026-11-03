//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// validateTextureViewFormats checks TextureDescriptor.ViewFormats as Rust
// wgpu-core device/resource.rs validate_texture_descriptor_inner does: each
// view format needs its format feature, and one that differs from the texture
// format may differ only by the sRGB suffix (W3C "GPUTextureDescriptor
// validating": view formats must be compatible with the format). The
// VIEW_FORMATS downlevel flag a reinterpreting list needs is a device query:
// Device.RequireViewFormats.
func validateTextureViewFormats(label string, format gputypes.TextureFormat, viewFormats []gputypes.TextureFormat, features gputypes.Features) error {
	for _, viewFormat := range viewFormats {
		if err := validateTextureFormatFeatures(viewFormat, 0, features); err != nil {
			return err
		}
		if viewFormat != format && viewFormat.RemoveSrgbSuffix() != format.RemoveSrgbSuffix() {
			return &CreateTextureError{
				Kind:       CreateTextureErrorInvalidViewFormat,
				Label:      label,
				Format:     format,
				ViewFormat: viewFormat,
			}
		}
	}
	return nil
}

// RequireViewFormats checks that the device can create a texture with the
// given view formats: a list that holds a format other than the texture's
// needs the VIEW_FORMATS downlevel flag (Rust device/resource.rs
// validate_texture_descriptor_inner). A list with only the texture's own
// format needs nothing. The check is one device query, so a nil device
// accepts the descriptor. label names the texture in the error.
func (d *Device) RequireViewFormats(label string, format gputypes.TextureFormat, viewFormats []gputypes.TextureFormat) error {
	if d == nil {
		return nil
	}
	for _, viewFormat := range viewFormats {
		if viewFormat == format {
			continue
		}
		if err := d.RequireDownlevelFlags(gputypes.DownlevelFlagsViewFormats); err != nil {
			return &CreateTextureError{Kind: CreateTextureErrorMissingDownlevelFlags, Label: label, Cause: err}
		}
		return nil
	}
	return nil
}

// ValidateTextureViewFormat checks the format of a texture view descriptor as
// Rust wgpu-core resource.rs create_view_inner does for a view of all aspects:
// the view format is the texture format or one of the texture's view formats.
// An undefined view format selects the texture's own format. A view of a
// single aspect (not TextureAspectAll) is not checked here: Rust requires the
// aspect-specific format of the texture, which gputypes does not provide yet.
func ValidateTextureViewFormat(textureFormat gputypes.TextureFormat, viewFormats []gputypes.TextureFormat, aspect gputypes.TextureAspect, viewFormat gputypes.TextureFormat) error {
	if viewFormat == gputypes.TextureFormatUndefined || viewFormat == textureFormat {
		return nil
	}
	if aspect != gputypes.TextureAspectUndefined && aspect != gputypes.TextureAspectAll {
		return nil
	}
	for _, listed := range viewFormats {
		if listed == viewFormat {
			return nil
		}
	}
	return &CreateTextureViewError{
		Kind:          CreateTextureViewErrorFormatReinterpretation,
		TextureFormat: textureFormat,
		ViewFormat:    viewFormat,
	}
}
