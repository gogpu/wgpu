//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
)

// TestRenderPassAttachmentLifetimeValidationCoversEverySlot is the root
// package's surface-lifetime test of the attachment checks, moved with the
// declarations it exercises (S4's Move). The views are the encoder domain's
// handle types; a released view answers ResolveHAL with nil, which is the
// state the surface test produced by retiring an acquisition.
func TestRenderPassAttachmentLifetimeValidationCoversEverySlot(t *testing.T) {
	device, view := newTestCoreDevice(t)
	colorParent := newTrackedTexture(t, device, view, "color", gputypes.TextureUsageRenderAttachment)
	t.Cleanup(colorParent.Release)
	resolveParent := newTrackedTexture(t, device, view, "resolve", gputypes.TextureUsageRenderAttachment)
	t.Cleanup(resolveParent.Release)
	depthParent := newTrackedTexture(t, device, view, "depth", gputypes.TextureUsageRenderAttachment)
	t.Cleanup(depthParent.Release)

	color := newTestTextureView(t, device, view, colorParent)
	t.Cleanup(color.Release)
	resolve := newTestTextureView(t, device, view, resolveParent)
	t.Cleanup(resolve.Release)
	depth := newTestTextureView(t, device, view, depthParent)
	t.Cleanup(depth.Release)

	desc := &RenderPassDescriptor{
		ColorAttachments:       []RenderPassColorAttachment{{View: color, ResolveTarget: resolve}},
		DepthStencilAttachment: &RenderPassDepthStencilAttachment{View: depth},
	}
	trackRenderPassTextureViews(nil, desc)
	trackRenderPassTextureViews(&CommandEncoder{}, nil)
	encoder := &CommandEncoder{}
	trackRenderPassTextureViews(encoder, desc)
	if len(encoder.usedTextures) != 3 {
		t.Fatalf("tracked texture owners = %d, want 3", len(encoder.usedTextures))
	}
	if err := validateRenderPassTextureViews(nil); err != nil {
		t.Fatalf("nil descriptor validation: %v", err)
	}
	if err := validateRenderPassTextureViews(desc); err != nil {
		t.Fatalf("active attachment validation: %v", err)
	}

	color.Release()
	resolve.Release()
	depth.Release()
	tests := []struct {
		name string
		desc *RenderPassDescriptor
	}{
		{
			name: "color",
			desc: &RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{View: color}}},
		},
		{
			name: "resolve",
			desc: &RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{ResolveTarget: resolve}}},
		},
		{
			name: "depth stencil",
			desc: &RenderPassDescriptor{DepthStencilAttachment: &RenderPassDepthStencilAttachment{View: depth}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRenderPassTextureViews(test.desc); !errors.Is(err, core.ErrReleased) {
				t.Fatalf("validation = %v, want ErrReleased", err)
			}
		})
	}
}
