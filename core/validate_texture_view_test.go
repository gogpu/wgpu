//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestValidateTextureDescriptorViewFormats pins Rust wgpu-core
// validate_texture_descriptor_inner: a view format is the texture format or
// differs from it only by the sRGB suffix, and needs its format feature.
func TestValidateTextureDescriptorViewFormats(t *testing.T) {
	t.Parallel()
	const (
		rgba     = gputypes.TextureFormatRGBA8Unorm
		rgbaSrgb = gputypes.TextureFormatRGBA8UnormSrgb
		bgraSrgb = gputypes.TextureFormatBGRA8UnormSrgb
		bc1      = gputypes.TextureFormatBC1RGBAUnorm
		bc1Srgb  = gputypes.TextureFormatBC1RGBAUnormSrgb
	)
	tests := []struct {
		name        string
		format      gputypes.TextureFormat
		viewFormats []gputypes.TextureFormat
		features    gputypes.Features
		wantKind    CreateTextureErrorKind
		wantErr     bool
		wantFeature bool
	}{
		{name: "no view formats", format: rgba},
		{name: "the texture's own format", format: rgba, viewFormats: []gputypes.TextureFormat{rgba}},
		{name: "sRGB variant", format: rgba, viewFormats: []gputypes.TextureFormat{rgbaSrgb}},
		{name: "linear variant of an sRGB texture", format: rgbaSrgb, viewFormats: []gputypes.TextureFormat{rgba}},
		{name: "own format and sRGB variant", format: rgba, viewFormats: []gputypes.TextureFormat{rgba, rgbaSrgb}},
		{name: "unrelated format", format: rgba, viewFormats: []gputypes.TextureFormat{gputypes.TextureFormatR32Float},
			wantKind: CreateTextureErrorInvalidViewFormat, wantErr: true},
		{name: "same channels, different order", format: rgba, viewFormats: []gputypes.TextureFormat{bgraSrgb},
			wantKind: CreateTextureErrorInvalidViewFormat, wantErr: true},
		{name: "second entry invalid", format: rgba, viewFormats: []gputypes.TextureFormat{rgbaSrgb, gputypes.TextureFormatR32Float},
			wantKind: CreateTextureErrorInvalidViewFormat, wantErr: true},
		{name: "compressed sRGB variant with the feature", format: bc1, viewFormats: []gputypes.TextureFormat{bc1Srgb},
			features: gputypes.Features(gputypes.FeatureTextureCompressionBC)},
		{name: "compressed view format without its feature", format: rgba, viewFormats: []gputypes.TextureFormat{bc1},
			wantFeature: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desc := &hal.TextureDescriptor{
				Label:         "view-formats",
				Size:          hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     gputypes.TextureDimension2D,
				Format:        tt.format,
				Usage:         gputypes.TextureUsageTextureBinding,
				ViewFormats:   tt.viewFormats,
			}
			err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), tt.features)
			switch {
			case !tt.wantErr:
				if err != nil {
					t.Fatalf("ValidateTextureDescriptor = %v, want nil", err)
				}
			case tt.wantFeature:
				if err == nil || IsCreateTextureError(err) {
					t.Fatalf("ValidateTextureDescriptor = %v, want a missing-feature error", err)
				}
			default:
				var cte *CreateTextureError
				if !errors.As(err, &cte) || cte.Kind != tt.wantKind {
					t.Fatalf("ValidateTextureDescriptor = %v, want *CreateTextureError kind %d", err, tt.wantKind)
				}
				if cte.ViewFormat == gputypes.TextureFormatUndefined || cte.Format != tt.format || cte.Error() == "" {
					t.Errorf("error does not name the formats: %+v", cte)
				}
			}
		})
	}
}

// TestDeviceRequireViewFormats: a list that reinterprets the format needs the
// VIEW_FORMATS downlevel flag; a list of the texture's own format, an empty
// list and a nil device need nothing (Rust device/resource.rs).
func TestDeviceRequireViewFormats(t *testing.T) {
	t.Parallel()
	const (
		rgba     = gputypes.TextureFormatRGBA8Unorm
		rgbaSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	)
	withFlag := &Device{downlevel: gputypes.DownlevelCapabilities{Flags: gputypes.DownlevelFlagsViewFormats}}
	without := &Device{downlevel: gputypes.DownlevelCapabilities{Flags: gputypes.DownlevelFlagsComputeShaders}}

	tests := []struct {
		name        string
		device      *Device
		viewFormats []gputypes.TextureFormat
		wantErr     bool
	}{
		{name: "reinterpreting with the flag", device: withFlag, viewFormats: []gputypes.TextureFormat{rgbaSrgb}},
		{name: "reinterpreting without the flag", device: without, viewFormats: []gputypes.TextureFormat{rgbaSrgb}, wantErr: true},
		{name: "own format only, without the flag", device: without, viewFormats: []gputypes.TextureFormat{rgba}},
		{name: "own format then reinterpreting, without the flag", device: without,
			viewFormats: []gputypes.TextureFormat{rgba, rgbaSrgb}, wantErr: true},
		{name: "empty list, without the flag", device: without},
		{name: "nil device", device: nil, viewFormats: []gputypes.TextureFormat{rgbaSrgb}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.device.RequireViewFormats("tex", rgba, tt.viewFormats)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("RequireViewFormats = %v, want nil", err)
				}
				return
			}
			var cte *CreateTextureError
			if !errors.As(err, &cte) || cte.Kind != CreateTextureErrorMissingDownlevelFlags {
				t.Fatalf("RequireViewFormats = %v, want a missing-downlevel-flags *CreateTextureError", err)
			}
			if cte.Unwrap() == nil || cte.Error() == "" {
				t.Errorf("error does not carry the downlevel cause: %+v", cte)
			}
		})
	}
}

// TestValidateTextureViewFormat pins Rust wgpu-core create_view_inner for a
// view of all aspects: the view format is the texture format or listed.
func TestValidateTextureViewFormat(t *testing.T) {
	t.Parallel()
	const (
		rgba     = gputypes.TextureFormatRGBA8Unorm
		rgbaSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	)
	listed := []gputypes.TextureFormat{rgbaSrgb}
	all, depth := gputypes.TextureAspectAll, gputypes.TextureAspectDepthOnly
	tests := []struct {
		name        string
		viewFormats []gputypes.TextureFormat
		aspect      gputypes.TextureAspect
		viewFormat  gputypes.TextureFormat
		wantErr     bool
	}{
		{name: "undefined format selects the texture's", viewFormat: gputypes.TextureFormatUndefined},
		{name: "the texture's format", viewFormat: rgba, aspect: all},
		{name: "listed view format", viewFormats: listed, viewFormat: rgbaSrgb, aspect: all},
		{name: "listed view format, undefined aspect", viewFormats: listed, viewFormat: rgbaSrgb},
		{name: "unlisted sRGB variant", viewFormat: rgbaSrgb, aspect: all, wantErr: true},
		{name: "unlisted sRGB variant, undefined aspect", viewFormat: rgbaSrgb, wantErr: true},
		{name: "other format listed", viewFormats: listed, viewFormat: gputypes.TextureFormatBGRA8Unorm, aspect: all, wantErr: true},
		{name: "single aspect is not checked here", viewFormat: gputypes.TextureFormatDepth32Float, aspect: depth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTextureViewFormat(rgba, tt.viewFormats, tt.aspect, tt.viewFormat)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("ValidateTextureViewFormat = %v, want nil", err)
				}
				return
			}
			var cve *CreateTextureViewError
			if !errors.As(err, &cve) || cve.Kind != CreateTextureViewErrorFormatReinterpretation {
				t.Fatalf("ValidateTextureViewFormat = %v, want a format reinterpretation error", err)
			}
			if cve.TextureFormat != rgba || cve.ViewFormat != tt.viewFormat || cve.Error() == "" {
				t.Errorf("error does not name the formats: %+v", cve)
			}
		})
	}
}
