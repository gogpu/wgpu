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

// TestStencilUploadNeedsBlit: on a backend with StencilCopyNeedsBlit only the
// stencil aspect of a combined depth-stencil format is lowered to the blit;
// every other format and aspect, and every backend without the capability,
// copies directly.
func TestStencilUploadNeedsBlit(t *testing.T) {
	t.Parallel()
	device := func(needsBlit bool) *Device {
		return &Device{adapter: &Adapter{StencilCopyNeedsBlit: needsBlit}}
	}
	stencil := gputypes.TextureAspectStencilOnly
	for _, format := range []gputypes.TextureFormat{
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8,
	} {
		if !device(true).StencilUploadNeedsBlit(format, stencil) {
			t.Errorf("%v stencil with the capability: no blit", format)
		}
		if device(false).StencilUploadNeedsBlit(format, stencil) {
			t.Errorf("%v stencil without the capability: blit", format)
		}
		for _, aspect := range []gputypes.TextureAspect{gputypes.TextureAspectDepthOnly, gputypes.TextureAspectAll, gputypes.TextureAspectUndefined} {
			if device(true).StencilUploadNeedsBlit(format, aspect) {
				t.Errorf("%v aspect %v with the capability: blit", format, aspect)
			}
		}
	}
	for _, format := range []gputypes.TextureFormat{
		gputypes.TextureFormatStencil8,
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatRGBA8Unorm,
	} {
		for _, aspect := range []gputypes.TextureAspect{gputypes.TextureAspectAll, stencil} {
			if device(true).StencilUploadNeedsBlit(format, aspect) {
				t.Errorf("%v aspect %v with the capability: blit", format, aspect)
			}
		}
	}
	var none *Device
	if none.StencilUploadNeedsBlit(gputypes.TextureFormatDepth24PlusStencil8, stencil) {
		t.Errorf("nil device: blit")
	}
	if (&Device{}).StencilUploadNeedsBlit(gputypes.TextureFormatDepth24PlusStencil8, stencil) {
		t.Errorf("device without adapter: blit")
	}
}

// TestStencilBlitResources: the resources are created once, disposed once by
// Destroy, and not created again after it.
func TestStencilBlitResources(t *testing.T) {
	t.Parallel()
	d := &Device{stencilBlit: &stencilBlitSlot{}}
	created := 0
	res := &countingDisposer{}
	create := func() StencilBlitResources { created++; return res }
	if got := d.StencilBlitResources(create); got != res {
		t.Fatalf("first call: %v", got)
	}
	if got := d.StencilBlitResources(create); got != res || created != 1 {
		t.Fatalf("second call: %v, created %d times", got, created)
	}
	d.Destroy()
	d.Destroy()
	if res.disposed != 1 {
		t.Errorf("disposed %d times, want 1", res.disposed)
	}
	if got := d.StencilBlitResources(create); got != nil || created != 1 {
		t.Errorf("after Destroy: %v, created %d times", got, created)
	}
	if got := (&Device{}).StencilBlitResources(create); got != nil {
		t.Errorf("device without HAL integration: %v", got)
	}
}

type countingDisposer struct{ disposed int }

func (c *countingDisposer) Dispose() { c.disposed++ }
