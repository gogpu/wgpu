//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// foreignView is a hal.TextureView belonging to no backend in this package,
// standing in for a view handed over from another HAL.
type foreignView struct{}

func (foreignView) Destroy()              {}
func (foreignView) NativeHandle() uintptr { return 0 }

// attachmentView builds a TextureView standing in for a render target. Only
// the fields the render-area helpers read are populated.
func attachmentView(width, height, samples uint32, format gputypes.TextureFormat) *TextureView {
	return &TextureView{
		size: Extent3D{Width: width, Height: height, Depth: 1},
		texture: &Texture{
			size:    Extent3D{Width: width, Height: height, Depth: 1},
			format:  format,
			samples: samples,
		},
	}
}

// TestRenderAreaView covers which attachment defines a pass's render area.
// A nil want means the pass names nothing usable, so the caller should decline
// to begin it rather than encode one with a zero extent.
func TestRenderAreaView(t *testing.T) {
	color := attachmentView(800, 600, 1, gputypes.TextureFormatRGBA8Unorm)
	second := attachmentView(640, 480, 1, gputypes.TextureFormatRGBA8Unorm)
	depth := attachmentView(2048, 2048, 1, gputypes.TextureFormatDepth32Float)
	var typedNil *TextureView

	cases := []struct {
		name string
		desc *hal.RenderPassDescriptor
		want *TextureView
	}{
		{
			name: "color attachment wins over depth",
			desc: &hal.RenderPassDescriptor{
				ColorAttachments:       []hal.RenderPassColorAttachment{{View: color}},
				DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{View: depth},
			},
			want: color,
		},
		{
			// A sparse color array is legal in WebGPU and is how MRT declares
			// an unused slot, so a gap is skipped rather than stopped at.
			name: "sparse color array skips the absent slot",
			desc: &hal.RenderPassDescriptor{
				ColorAttachments: []hal.RenderPassColorAttachment{{View: nil}, {View: second}},
			},
			want: second,
		},
		{
			// The depth-only pass -- how a shadow map is drawn, and the reason
			// this fallback exists at all.
			name: "no color attachments falls back to depth",
			desc: &hal.RenderPassDescriptor{
				DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{View: depth},
			},
			want: depth,
		},
		{
			name: "no usable color slot falls back to depth",
			desc: &hal.RenderPassDescriptor{
				ColorAttachments:       []hal.RenderPassColorAttachment{{View: nil}},
				DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{View: depth},
			},
			want: depth,
		},
		{
			name: "nil descriptor",
			desc: nil,
			want: nil,
		},
		{
			name: "empty descriptor",
			desc: &hal.RenderPassDescriptor{},
			want: nil,
		},
		{
			name: "color slot with no view and no depth",
			desc: &hal.RenderPassDescriptor{
				ColorAttachments: []hal.RenderPassColorAttachment{{View: nil}},
			},
			want: nil,
		},
		{
			name: "depth attachment with no view",
			desc: &hal.RenderPassDescriptor{
				DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{View: nil},
			},
			want: nil,
		},
		{
			// A typed-nil pointer satisfies the type assertion; a foreign view
			// fails it. Neither may be dereferenced, and neither may be
			// mistaken for a render area.
			name: "typed-nil view",
			desc: &hal.RenderPassDescriptor{
				DepthStencilAttachment: &hal.RenderPassDepthStencilAttachment{View: typedNil},
			},
			want: nil,
		},
		{
			name: "view from another backend",
			desc: &hal.RenderPassDescriptor{
				ColorAttachments: []hal.RenderPassColorAttachment{{View: foreignView{}}},
			},
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderAreaView(tc.desc); got != tc.want {
				t.Fatalf("render area view = %p, want %p", got, tc.want)
			}
		})
	}
}

// TestAttachmentSampleCountFollowsTheView verifies that the pass sample count
// comes from the view that defines the render area -- including a depth-only
// pass, whose only attachment is the depth target.
func TestAttachmentSampleCountFollowsTheView(t *testing.T) {
	cases := []struct {
		name string
		view *TextureView
		want vk.SampleCountFlagBits
	}{
		{
			name: "single sampled",
			view: attachmentView(64, 64, 1, gputypes.TextureFormatRGBA8Unorm),
			want: vk.SampleCountFlagBits(1),
		},
		{
			name: "msaa depth target",
			view: attachmentView(64, 64, 4, gputypes.TextureFormatDepth32Float),
			want: vk.SampleCountFlagBits(4),
		},
		{
			name: "swapchain view with no texture",
			view: &TextureView{size: Extent3D{Width: 64, Height: 64, Depth: 1}, isSwapchain: true},
			want: vk.SampleCountFlagBits(1),
		},
		{
			name: "no view",
			view: nil,
			want: vk.SampleCountFlagBits(1),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := attachmentSampleCount(tc.view); got != tc.want {
				t.Fatalf("sample count = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestEndDoesNotEndAPassThatWasNeverBegun is the fault this whole change
// exists for. BeginRenderPass declines to begin a pass whose attachments do
// not resolve, or whose render pass or framebuffer could not be created, and
// returns an encoder anyway. Ending that encoder used to call
// vkCmdEndRenderPass regardless, which is undefined behavior -- an access
// violation inside the driver rather than a validation error.
//
// The command buffer is non-zero and the device carries no dispatch table, so
// any call that reaches the driver panics and any call that does not is
// silent. TestEndEndsAPassThatWasBegun is the other half: without it this test
// would still pass if End stopped ending passes altogether.
func TestEndDoesNotEndAPassThatWasNeverBegun(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{}, // cmds is nil: reaching the driver panics
		active: 0x1234,    // recording, so the active guard does not hide the bug
	}
	rpe := &RenderPassEncoder{encoder: enc} // begun is false

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("End reached the driver for a pass that was never begun: %v", r)
		}
	}()

	rpe.End()
}

// TestEndEndsAPassThatWasBegun pins the other direction: a pass that
// vkCmdBeginRenderPass did run on must still be ended. Reaching the nil
// dispatch table is the proof that End tried.
func TestEndEndsAPassThatWasBegun(t *testing.T) {
	enc := &CommandEncoder{
		device: &Device{}, // cmds is nil: reaching the driver panics
		active: 0x1234,
	}
	rpe := &RenderPassEncoder{
		encoder:     enc,
		renderPass:  0x5678,
		framebuffer: 0x9abc,
		begun:       true,
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("End did not end a pass that was begun")
		}
	}()

	rpe.End()
}

// TestEndReleasesAnUnbegunEncoderToThePool verifies that declining to end the
// pass still clears the encoder, so a pooled encoder cannot carry a stale
// descriptor, or a set begun flag, into the next BeginRenderPass.
func TestEndReleasesAnUnbegunEncoderToThePool(t *testing.T) {
	enc := &CommandEncoder{device: &Device{}, active: 0x1234}
	rpe := &RenderPassEncoder{
		encoder: enc,
		desc:    &hal.RenderPassDescriptor{Label: "stale"},
	}

	rpe.End()

	if rpe.encoder != nil || rpe.desc != nil || rpe.framebuffer != 0 || rpe.renderPass != 0 || rpe.begun {
		t.Fatalf("End left the encoder populated: encoder=%v desc=%v framebuffer=%d renderPass=%d begun=%v",
			rpe.encoder, rpe.desc, rpe.framebuffer, rpe.renderPass, rpe.begun)
	}
}
