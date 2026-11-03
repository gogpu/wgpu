//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestRequireDepthTextureAndBufferCopies: copies between buffers and
// depth/stencil textures need the DEPTH_TEXTURE_AND_BUFFER_COPIES downlevel
// flag, copies of other formats never do (Rust wgpu-core copy_buffer_to_texture,
// copy_texture_to_buffer).
func TestRequireDepthTextureAndBufferCopies(t *testing.T) {
	t.Parallel()
	withFlags := func(flags gputypes.DownlevelFlags) *Device {
		return &Device{downlevel: gputypes.DownlevelCapabilities{Flags: flags}}
	}
	all := gputypes.DownlevelFlagsAll()
	without := all &^ gputypes.DownlevelFlagsDepthTextureAndBufferCopies

	depthFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatStencil8,
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
	}
	for _, format := range depthFormats {
		if err := withFlags(all).RequireDepthTextureAndBufferCopies(format); err != nil {
			t.Errorf("%v with the flag: unexpected error %v", format, err)
		}
		err := withFlags(without).RequireDepthTextureAndBufferCopies(format)
		var te *TransferError
		if !errors.As(err, &te) || te.Kind != TransferErrorMissingDownlevelFlags || te.Format != format {
			t.Errorf("%v without the flag: error %v, want TransferErrorMissingDownlevelFlags", format, err)
			continue
		}
		if te.Unwrap() == nil || te.Error() == "" {
			t.Errorf("%v: the error has no cause or message", format)
		}
	}
	for _, format := range []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatR32Float} {
		if err := withFlags(0).RequireDepthTextureAndBufferCopies(format); err != nil {
			t.Errorf("%v on a device without flags: unexpected error %v", format, err)
		}
	}
	var none *Device
	if err := none.RequireDepthTextureAndBufferCopies(gputypes.TextureFormatDepth32Float); err != nil {
		t.Errorf("nil device: unexpected error %v", err)
	}
}
