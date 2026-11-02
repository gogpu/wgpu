//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal/noop"
)

// newDownlevelDevice builds a device whose adapter reports the given downlevel
// flags (a device without an adapter reports none).
func newDownlevelDevice(t *testing.T, flags gputypes.DownlevelFlags) *Device {
	t.Helper()
	raw := &noop.Device{}
	adapter := &core.Adapter{DownlevelCapabilities: gputypes.DownlevelCapabilities{Flags: flags}}
	coreDevice := core.NewDevice(raw, adapter, 0, gputypes.DefaultLimits(), "downlevel-test")
	queue := &Queue{hal: &noop.Queue{}, halDevice: raw}
	device := &Device{core: coreDevice, queue: queue}
	queue.device = device
	t.Cleanup(device.Release)
	return device
}

func viewFormatsDescriptor(format gputypes.TextureFormat, viewFormats ...gputypes.TextureFormat) *TextureDescriptor {
	return &TextureDescriptor{
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     TextureDimension2D,
		Format:        format,
		Usage:         TextureUsageTextureBinding | TextureUsageCopyDst,
		ViewFormats:   viewFormats,
	}
}

// TestCreateTextureViewFormats: TextureDescriptor.ViewFormats must match the
// texture format modulo the sRGB suffix, and a list that reinterprets the
// format needs the VIEW_FORMATS downlevel flag (Rust wgpu-core
// validate_texture_descriptor_inner).
func TestCreateTextureViewFormats(t *testing.T) {
	const (
		rgba     = gputypes.TextureFormatRGBA8Unorm
		rgbaSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	)
	withFlag := newDownlevelDevice(t, gputypes.DownlevelFlagsViewFormats)
	withoutFlag := newDownlevelDevice(t, gputypes.DownlevelFlagsComputeShaders)

	tests := []struct {
		name     string
		device   *Device
		desc     *TextureDescriptor
		wantKind core.CreateTextureErrorKind
		wantErr  bool
	}{
		{name: "sRGB variant with the flag", device: withFlag, desc: viewFormatsDescriptor(rgba, rgbaSrgb)},
		{name: "own format without the flag", device: withoutFlag, desc: viewFormatsDescriptor(rgba, rgba)},
		{name: "no view formats without the flag", device: withoutFlag, desc: viewFormatsDescriptor(rgba)},
		{name: "unrelated view format", device: withFlag, desc: viewFormatsDescriptor(rgba, gputypes.TextureFormatR32Float),
			wantKind: core.CreateTextureErrorInvalidViewFormat, wantErr: true},
		{name: "sRGB variant without the flag", device: withoutFlag, desc: viewFormatsDescriptor(rgba, rgbaSrgb),
			wantKind: core.CreateTextureErrorMissingDownlevelFlags, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tex, err := tt.device.CreateTexture(tt.desc)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CreateTexture: %v", err)
				}
				tex.Release()
				return
			}
			var cte *core.CreateTextureError
			if !errors.As(err, &cte) || cte.Kind != tt.wantKind {
				t.Fatalf("CreateTexture = %v, want *core.CreateTextureError kind %d", err, tt.wantKind)
			}
		})
	}
}

// TestCreateTextureViewFormat: a view format is the texture format or one of
// its view formats (Rust wgpu-core create_view_inner); a surface texture has
// none.
func TestCreateTextureViewFormat(t *testing.T) {
	const (
		rgba     = gputypes.TextureFormatRGBA8Unorm
		rgbaSrgb = gputypes.TextureFormatRGBA8UnormSrgb
	)
	device := newDownlevelDevice(t, gputypes.DownlevelFlagsViewFormats)
	listed, err := device.CreateTexture(viewFormatsDescriptor(rgba, rgbaSrgb))
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer listed.Release()
	plain, err := device.CreateTexture(viewFormatsDescriptor(rgba))
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer plain.Release()

	tests := []struct {
		name    string
		texture *Texture
		desc    *TextureViewDescriptor
		wantErr bool
	}{
		{name: "nil descriptor", texture: plain},
		{name: "default format", texture: plain, desc: &TextureViewDescriptor{}},
		{name: "texture format", texture: plain, desc: &TextureViewDescriptor{Format: rgba}},
		{name: "listed view format", texture: listed, desc: &TextureViewDescriptor{Format: rgbaSrgb}},
		{name: "unlisted sRGB variant", texture: plain, desc: &TextureViewDescriptor{Format: rgbaSrgb}, wantErr: true},
		{name: "unrelated format", texture: listed, desc: &TextureViewDescriptor{Format: gputypes.TextureFormatBGRA8Unorm}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view, err := device.CreateTextureView(tt.texture, tt.desc)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("CreateTextureView: %v", err)
				}
				view.Release()
				return
			}
			var cve *core.CreateTextureViewError
			if !errors.As(err, &cve) || cve.Kind != core.CreateTextureViewErrorFormatReinterpretation {
				t.Fatalf("CreateTextureView = %v, want a format reinterpretation error", err)
			}
		})
	}
}

// TestDepthStencilBufferCopiesNeedDownlevelFlag: buffer-texture copies and
// WriteTexture of a depth/stencil texture need DEPTH_TEXTURE_AND_BUFFER_COPIES
// (Rust copy_buffer_to_texture, copy_texture_to_buffer, write_texture).
func TestDepthStencilBufferCopiesNeedDownlevelFlag(t *testing.T) {
	copyBuffer := func(device *Device) *Buffer {
		buf, err := device.CreateBuffer(&BufferDescriptor{Size: 4096, Usage: BufferUsageCopySrc | BufferUsageCopyDst})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(buf.Release)
		return buf
	}
	stencilTexture := func(device *Device) *Texture {
		tex, err := device.CreateTexture(&TextureDescriptor{
			Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     TextureDimension2D,
			Format:        gputypes.TextureFormatStencil8,
			Usage:         TextureUsageCopySrc | TextureUsageCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	layout := ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4}
	size := Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}

	for _, tt := range []struct {
		name string
		flag gputypes.DownlevelFlags
		want bool
	}{
		{name: "without the flag", flag: gputypes.DownlevelFlagsComputeShaders, want: true},
		{name: "with the flag", flag: gputypes.DownlevelFlagsDepthTextureAndBufferCopies},
	} {
		t.Run("buffer to texture, "+tt.name, func(t *testing.T) {
			device := newDownlevelDevice(t, tt.flag)
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			defer enc.DiscardEncoding()
			enc.CopyBufferToTexture(copyBuffer(device), stencilTexture(device),
				[]BufferTextureCopy{{BufferLayout: layout, Size: size}})
			_, err = enc.Finish()
			requireMissingDownlevelFlags(t, err, tt.want)
		})
		t.Run("write texture, "+tt.name, func(t *testing.T) {
			device := newDownlevelDevice(t, tt.flag)
			tex := stencilTexture(device)
			err := device.Queue().WriteTexture(&ImageCopyTexture{Texture: tex}, make([]byte, 1024), &layout, &size)
			requireMissingDownlevelFlags(t, err, tt.want)
		})
	}
}

func requireMissingDownlevelFlags(t *testing.T, err error, want bool) {
	t.Helper()
	var te *core.TransferError
	got := errors.As(err, &te) && te.Kind == core.TransferErrorMissingDownlevelFlags
	if got != want {
		t.Fatalf("error = %v, missing-downlevel-flags = %v, want %v", err, got, want)
	}
	if !want && err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
