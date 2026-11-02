// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// Render bundles: core records a bundle as a command list, resolves its bind
// groups through the binder when the bundle is finished and replays the list
// into the pass's backend encoder when the bundle is executed (Rust wgpu-core
// command/bundle.rs RenderBundleEncoder::finish and RenderBundle::execute).
// No backend bundle object is involved, so bundles work on every backend.
// Each draw here adds a uniform color to a target cleared to black (blend
// One + One), so a draw executed twice shows twice the color.

const (
	bundleTargetWidth  = 64 // 256 bytes per row, the copy alignment
	bundleTargetHeight = 4
)

// bundleColorShader draws a full-height quad over [xLo, xHi] in NDC x in the
// color of the uniform at group 0, binding 0.
func bundleColorShader(xLo, xHi float32) string {
	return fmt.Sprintf(`
struct U { color: vec4<f32> }
@group(0) @binding(0) var<uniform> u: U;

@vertex
fn vs(@builtin(vertex_index) v: u32) -> @builtin(position) vec4<f32> {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    let c = quad[v %% 6u];
    return vec4<f32>(select(%[1]f, %[2]f, c.x > 0.0), c.y, 0.0, 1.0);
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return u.color;
}
`, xLo, xHi)
}

// bundleRig holds additive pipelines that draw the left or the right half of
// the target, and the layout of their uniform color group.
type bundleRig struct {
	device      *wgpu.Device
	bgl         *wgpu.BindGroupLayout
	left, right *wgpu.RenderPipeline
}

func newBundleRig(t *testing.T, device *wgpu.Device) *bundleRig {
	t.Helper()
	bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
		Binding: 0, Visibility: wgpu.ShaderStageFragment,
		Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
	})
	return &bundleRig{
		device: device,
		bgl:    bgl,
		left:   newAdditivePipeline(t, device, bundleColorShader(-1, 0), bgl),
		right:  newAdditivePipeline(t, device, bundleColorShader(0, 1), bgl),
	}
}

func newAdditivePipeline(t *testing.T, device *wgpu.Device, wgsl string, bgl *wgpu.BindGroupLayout) *wgpu.RenderPipeline {
	t.Helper()
	shader := newDynamicShader(t, device, wgsl)
	add := gputypes.BlendComponent{
		SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne, Operation: gputypes.BlendOperationAdd,
	}
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "bundle",
		Layout: newRebindPipelineLayout(t, device, bgl),
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				Blend:     &gputypes.BlendState{Color: add, Alpha: add},
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// group returns a bind group whose uniform holds the color rgba/255.
func (r *bundleRig) group(t *testing.T, rgba [4]uint32) *wgpu.BindGroup {
	t.Helper()
	words := make([]uint32, 4)
	for i, c := range rgba {
		words[i] = math.Float32bits(float32(c) / 255)
	}
	data := make([]byte, 16)
	putWords(data, words)
	buf := newPublicBuffer(t, r.device, 16, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	if err := r.device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return newBindGroup(t, r.device, r.bgl, wgpu.BindGroupEntry{Binding: 0, Buffer: buf, Size: 16})
}

// bundle records a bundle that sets group before pipeline and draws one quad.
func (r *bundleRig) bundle(t *testing.T, pipeline *wgpu.RenderPipeline, group *wgpu.BindGroup) *wgpu.RenderBundle {
	t.Helper()
	return recordBundle(t, r.device, func(enc *wgpu.RenderBundleEncoder) {
		enc.SetBindGroup(0, group, nil)
		enc.SetPipeline(pipeline)
		enc.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	})
}

func recordBundle(t *testing.T, device *wgpu.Device, record func(enc *wgpu.RenderBundleEncoder)) *wgpu.RenderBundle {
	t.Helper()
	enc, err := device.CreateRenderBundleEncoder(&wgpu.RenderBundleEncoderDescriptor{
		Label:        "bundle",
		ColorFormats: []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm},
		SampleCount:  1,
	})
	if err != nil {
		t.Fatalf("CreateRenderBundleEncoder: %v", err)
	}
	record(enc)
	bundle, err := enc.Finish(&wgpu.RenderBundleDescriptor{Label: "bundle"})
	if err != nil {
		t.Fatalf("RenderBundleEncoder.Finish: %v", err)
	}
	t.Cleanup(bundle.Release)
	return bundle
}

// bundleTarget is an RGBA8 render target that tests can also bind.
type bundleTarget struct {
	texture *wgpu.Texture
	view    *wgpu.TextureView
}

func newBundleTarget(t *testing.T, device *wgpu.Device, usage gputypes.TextureUsage) *bundleTarget {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "bundle-target",
		Size:          wgpu.Extent3D{Width: bundleTargetWidth, Height: bundleTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc | usage,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return &bundleTarget{texture: tex, view: view}
}

// encodePasses records one render pass over target per element of passes;
// the first clears the target to black, the others load it. A validation
// error in a pass is left to enc.Finish, so the caller always finishes the
// encoder.
func (bt *bundleTarget) encodePasses(t *testing.T, enc *wgpu.CommandEncoder, passes ...func(pass *wgpu.RenderPassEncoder)) {
	t.Helper()
	for i, record := range passes {
		load := gputypes.LoadOpLoad
		if i == 0 {
			load = gputypes.LoadOpClear
		}
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: bt.view, LoadOp: load, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{A: 0},
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		record(pass)
		_ = pass.End() // an encoder error is reported by Finish
	}
}

// render runs passes over the target in one command buffer and returns the
// target's pixels, row-major.
func (bt *bundleTarget) render(t *testing.T, device *wgpu.Device, passes ...func(pass *wgpu.RenderPassEncoder)) []uint32 {
	t.Helper()
	enc := newEncoder(t, device)
	bt.encodePasses(t, enc, passes...)
	const words = bundleTargetWidth * bundleTargetHeight
	staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(bt.texture, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: bundleTargetWidth * 4, RowsPerImage: bundleTargetHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: bt.texture},
		Size:         wgpu.Extent3D{Width: bundleTargetWidth, Height: bundleTargetHeight, DepthOrArrayLayers: 1},
	}})
	submit(t, device, enc)
	return readPublicBuffer(t, staging, words)
}

// checkHalves checks the pixel in the middle row of the left and right half.
func checkHalves(t *testing.T, px []uint32, left, right [4]uint32) {
	t.Helper()
	row := bundleTargetHeight / 2 * bundleTargetWidth
	checkBundlePixel(t, "left half", px[row+2], left)
	checkBundlePixel(t, "right half", px[row+bundleTargetWidth-3], right)
}

func checkBundlePixel(t *testing.T, what string, px uint32, want [4]uint32) {
	t.Helper()
	got := [4]uint32{px & 0xff, px >> 8 & 0xff, px >> 16 & 0xff, px >> 24}
	if got != want {
		t.Errorf("%s: got %v, want %v (rgba)", what, got, want)
	}
}

// TestRenderBundleExecutedTwice executes one bundle twice in a pass: the
// bundle sets its own pipeline and group each time, so the second execution
// draws again after the pass state was reset by the first.
func TestRenderBundleExecutedTwice(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		rig := newBundleRig(t, device)
		bundle := rig.bundle(t, rig.left, rig.group(t, [4]uint32{10, 0, 0, 20}))
		px := newBundleTarget(t, device, 0).render(t, device, func(pass *wgpu.RenderPassEncoder) {
			pass.ExecuteBundles(bundle)
			pass.ExecuteBundles(bundle)
		})
		checkHalves(t, px, [4]uint32{20, 0, 0, 40}, [4]uint32{})
	})
}

// TestRenderBundleTwoInOneCall executes two bundles with different pipelines
// and groups in one ExecuteBundles call, then the first one again.
func TestRenderBundleTwoInOneCall(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		rig := newBundleRig(t, device)
		a := rig.bundle(t, rig.left, rig.group(t, [4]uint32{10, 0, 0, 20}))
		b := rig.bundle(t, rig.right, rig.group(t, [4]uint32{0, 30, 0, 20}))
		px := newBundleTarget(t, device, 0).render(t, device, func(pass *wgpu.RenderPassEncoder) {
			pass.ExecuteBundles(a, b, a)
		})
		checkHalves(t, px, [4]uint32{20, 0, 0, 40}, [4]uint32{0, 30, 0, 20})
	})
}

// TestRenderBundleAcrossPasses executes one bundle in two passes of one
// command buffer, and again in a later submission.
func TestRenderBundleAcrossPasses(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		rig := newBundleRig(t, device)
		bundle := rig.bundle(t, rig.right, rig.group(t, [4]uint32{0, 0, 10, 10}))
		target := newBundleTarget(t, device, 0)
		exec := func(pass *wgpu.RenderPassEncoder) { pass.ExecuteBundles(bundle) }
		twoPasses := []func(pass *wgpu.RenderPassEncoder){exec, exec}
		px := target.render(t, device, twoPasses...)
		checkHalves(t, px, [4]uint32{}, [4]uint32{0, 0, 20, 20})
		px = target.render(t, device, exec)
		checkHalves(t, px, [4]uint32{}, [4]uint32{0, 0, 10, 10})
	})
}

// TestRenderBundleResetsPassState checks that ExecuteBundles leaves the pass
// without a pipeline and bind groups (WebGPU executeBundles, Rust wgpu-core
// State::reset_bundle): a draw after it must set them again.
func TestRenderBundleResetsPassState(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
		if !b.software && b.backend == gputypes.BackendDX12 {
			t.Skip("known defect, not the bundle reset: DX12: an errored encoder releases resources before its " +
				"command list is closed (debug layer: resource deleted prior to closing the command list); " +
				"the reset itself is covered by TestExecuteBundlesResetsPassState in the root package")
		}
		rig := newBundleRig(t, device)
		group := rig.group(t, [4]uint32{10, 0, 0, 10})
		bundle := rig.bundle(t, rig.left, group)
		draw := gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1}
		tests := []struct {
			name  string
			after func(pass *wgpu.RenderPassEncoder)
			want  error
		}{
			{"no pipeline", func(pass *wgpu.RenderPassEncoder) { pass.Draw(draw) }, wgpu.ErrDrawMissingPipeline},
			{"no bind group", func(pass *wgpu.RenderPassEncoder) {
				pass.SetPipeline(rig.right)
				pass.Draw(draw)
			}, wgpu.ErrDrawMissingBindGroup},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				enc := newEncoder(t, device)
				newBundleTarget(t, device, 0).encodePasses(t, enc, func(pass *wgpu.RenderPassEncoder) {
					pass.SetPipeline(rig.right)
					pass.SetBindGroup(0, group, nil)
					pass.ExecuteBundles(bundle)
					tt.after(pass)
				})
				if _, err := enc.Finish(); !errors.Is(err, tt.want) {
					t.Fatalf("Finish() error = %v, want %v", err, tt.want)
				}
			})
		}
	})
}

// TestRenderBundleUsageConflict executes a bundle that samples the pass's
// own color attachment. The bundle's resources belong to the pass's usage
// scope (Rust wgpu-core render.rs execute_bundle, merge_render_bundle), so
// the sampled and attachment usages of one texture conflict.
func TestRenderBundleUsageConflict(t *testing.T) {
	forEachDynamicOffsetDevice(t, 0, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		target := newBundleTarget(t, device, wgpu.TextureUsageTextureBinding)
		bgl := newBindGroupLayout(t, device, gputypes.BindGroupLayoutEntry{
			Binding: 0, Visibility: wgpu.ShaderStageFragment,
			Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			},
		})
		group := newBindGroup(t, device, bgl, wgpu.BindGroupEntry{Binding: 0, TextureView: target.view})
		shader := strings.Replace(bundleColorShader(-1, 1),
			"struct U { color: vec4<f32> }\n@group(0) @binding(0) var<uniform> u: U;",
			"@group(0) @binding(0) var t: texture_2d<f32>;", 1)
		shader = strings.Replace(shader, "return u.color;", "return textureLoad(t, vec2<i32>(0, 0), 0);", 1)
		pipeline := newAdditivePipeline(t, device, shader, bgl)
		bundle := recordBundle(t, device, func(enc *wgpu.RenderBundleEncoder) {
			enc.SetPipeline(pipeline)
			enc.SetBindGroup(0, group, nil)
			enc.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		})

		enc := newEncoder(t, device)
		target.encodePasses(t, enc, func(pass *wgpu.RenderPassEncoder) {
			pass.ExecuteBundles(bundle)
		})
		_, err := enc.Finish()
		if err == nil {
			t.Fatal("Finish() succeeded, want a usage conflict between the attachment and the bundle's sampled texture")
		}
		if !strings.Contains(err.Error(), "ExecuteBundles") {
			t.Fatalf("Finish() error = %v, want it to come from ExecuteBundles", err)
		}
	})
}
