//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// depthSliceTexture is a 16x16x8 3D render target with 3 mips: depth 8 at
// mip 0, 4 at mip 1, 2 at mip 2.
func depthSliceTexture3D() *Texture {
	return NewTexture(&mockTexture{}, nil,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension3D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 8},
		3, 1, "volume")
}

func depthSlicePtr(v uint32) *uint32 { return &v }

func depthSlicePass(view *TextureView, slice *uint32) *RenderPassDescriptor {
	return &RenderPassDescriptor{
		Label: "depth-slice",
		ColorAttachments: []RenderPassColorAttachment{{
			View: view, DepthSlice: slice,
			LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
	}
}

// TestValidateRenderPassDescriptor_DepthSlice covers WebGPU
// "GPURenderPassColorAttachment Valid Usage" for depthSlice, as Rust
// wgpu-core command/render.rs checks it.
func TestValidateRenderPassDescriptor_DepthSlice(t *testing.T) {
	volume := depthSliceTexture3D()
	flat := testTexture(gputypes.TextureFormatRGBA8Unorm, 16, 16, 1)
	view3D := func(mip uint32) *TextureView {
		return &TextureView{HAL: &mockTextureView{}, Parent: volume,
			Dimension: gputypes.TextureViewDimension3D, BaseMipLevel: mip}
	}
	view2D := &TextureView{HAL: &mockTextureView{}, Parent: flat, Dimension: gputypes.TextureViewDimension2D}
	unknown := &TextureView{HAL: &mockTextureView{}, Parent: flat}

	tests := []struct {
		name    string
		desc    *RenderPassDescriptor
		kind    RenderPassValidationErrorKind
		wantErr bool
		msg     string
	}{
		{name: "3D slice 0", desc: depthSlicePass(view3D(0), depthSlicePtr(0))},
		{name: "3D last slice at mip 0", desc: depthSlicePass(view3D(0), depthSlicePtr(7))},
		{name: "3D last slice at mip 1", desc: depthSlicePass(view3D(1), depthSlicePtr(3))},
		{name: "3D missing slice", desc: depthSlicePass(view3D(0), nil), wantErr: true,
			kind: RenderPassErrorMissingDepthSlice,
			msg:  "color attachment's view is 3D and requires depth slice to be provided"},
		{name: "3D slice == depth at mip 0", desc: depthSlicePass(view3D(0), depthSlicePtr(8)), wantErr: true,
			kind: RenderPassErrorDepthSliceLimit, msg: "depth slice must be less than 8 but is 8"},
		{name: "3D slice in mip 0 but past mip 2", desc: depthSlicePass(view3D(2), depthSlicePtr(2)), wantErr: true,
			kind: RenderPassErrorDepthSliceLimit, msg: "depth slice must be less than 2 but is 2"},
		// webgpu.h WGPU_DEPTH_SLICE_UNDEFINED given as a value is an error,
		// never "undefined": only nil means undefined.
		{name: "3D slice 0xFFFFFFFF", desc: depthSlicePass(view3D(0), depthSlicePtr(0xFFFFFFFF)), wantErr: true,
			kind: RenderPassErrorDepthSliceLimit, msg: "depth slice must be less than 8 but is 4294967295"},
		{name: "2D without slice", desc: depthSlicePass(view2D, nil)},
		{name: "2D with slice 0", desc: depthSlicePass(view2D, depthSlicePtr(0)), wantErr: true,
			kind: RenderPassErrorUnneededDepthSlice,
			msg:  "depth slice was provided but the color attachment's view is not 3D"},
		{name: "2D with slice 0xFFFFFFFF", desc: depthSlicePass(view2D, depthSlicePtr(0xFFFFFFFF)), wantErr: true,
			kind: RenderPassErrorUnneededDepthSlice,
			msg:  "depth slice was provided but the color attachment's view is not 3D"},
		{name: "unknown dimension is not checked", desc: depthSlicePass(unknown, depthSlicePtr(3))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateRenderPassDescriptor(tt.desc, gputypes.DefaultLimits())
			checkDepthSliceErr(t, err, tt.wantErr, tt.kind, tt.msg)
		})
	}
}

func checkDepthSliceErr(t *testing.T, err error, wantErr bool, kind RenderPassValidationErrorKind, msg string) {
	t.Helper()
	if !wantErr {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var rpve *RenderPassValidationError
	if !errors.As(err, &rpve) {
		t.Fatalf("error = %v, want *RenderPassValidationError", err)
	}
	if rpve.Kind != kind || rpve.AttachmentIndex != 0 {
		t.Fatalf("kind = %v, attachment = %d; want kind %v, attachment 0", rpve.Kind, rpve.AttachmentIndex, kind)
	}
	if !strings.Contains(err.Error(), msg) || !strings.Contains(err.Error(), `render pass "depth-slice": color attachment [0]`) {
		t.Fatalf("error text %q does not contain %q", err.Error(), msg)
	}
}

// captureEncoder records the HAL render pass descriptor it receives.
type captureEncoder struct {
	*recordingEncoder
	got *hal.RenderPassDescriptor
}

func (c *captureEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	c.got = desc
	return mockRenderPassEncoder{}
}

// TestBeginRenderPass_DepthSliceReachesHAL checks that BeginRenderPass
// rejects an invalid depth slice and hands a valid one to HAL as a copy.
func TestBeginRenderPass_DepthSliceReachesHAL(t *testing.T) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	volume := depthSliceTexture3D()
	view := &TextureView{HAL: &mockTextureView{}, Parent: volume, Dimension: gputypes.TextureViewDimension3D}

	t.Run("invalid", func(t *testing.T) {
		enc, err := device.CreateCommandEncoderWithHAL(&captureEncoder{recordingEncoder: newRecordingEncoder()}, "bad")
		if err != nil {
			t.Fatalf("CreateCommandEncoderWithHAL: %v", err)
		}
		if _, err := enc.BeginRenderPass(depthSlicePass(view, nil)); !IsRenderPassValidationError(err) {
			t.Fatalf("BeginRenderPass error = %v, want a render pass validation error", err)
		}
	})

	t.Run("valid", func(t *testing.T) {
		capture := &captureEncoder{recordingEncoder: newRecordingEncoder()}
		enc, err := device.CreateCommandEncoderWithHAL(capture, "good")
		if err != nil {
			t.Fatalf("CreateCommandEncoderWithHAL: %v", err)
		}
		slice := uint32(5)
		if _, err := enc.BeginRenderPass(depthSlicePass(view, &slice)); err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		slice = 1 // the HAL descriptor must not alias the caller's value
		if capture.got == nil || len(capture.got.ColorAttachments) != 1 {
			t.Fatalf("HAL descriptor = %+v, want one color attachment", capture.got)
		}
		got := capture.got.ColorAttachments[0].DepthSlice
		if got == nil || *got != 5 {
			t.Fatalf("HAL DepthSlice = %v, want 5", got)
		}
	})
}
