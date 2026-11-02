//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal/noop"
)

var rgba8Formats = []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}

// newEmptyBundle returns a finished bundle with no commands for a render
// pass with no attachments.
func newEmptyBundle(device *Device) *RenderBundle {
	return &RenderBundle{
		device: device,
		target: renderBundleTarget{context: core.RenderPassContext{SampleCount: 1}, depthReadOnly: true, stencilReadOnly: true},
		scope:  core.NewRenderBundleScope(),
	}
}

// bundleTestRig is a noop device with a render pass whose backend pass
// records the commands it gets.
type bundleTestRig struct {
	device *Device
	pass   *captureRenderPass
}

func newBundleTestRig() *bundleTestRig {
	pass := &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}, renderPass: pass}
	return &bundleTestRig{device: newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command}), pass: pass}
}

// beginPass begins a render pass with no attachments, or with one RGBA8
// color attachment.
func (r *bundleTestRig) beginPass(t *testing.T, color bool) (*CommandEncoder, *RenderPassEncoder) {
	t.Helper()
	encoder, err := r.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	desc := &RenderPassDescriptor{}
	if color {
		tex, err := r.device.CreateTexture(&TextureDescriptor{
			Size: Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1,
			Dimension: TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm, Usage: TextureUsageRenderAttachment,
		})
		if err != nil {
			t.Fatalf("CreateTexture() error = %v", err)
		}
		view, err := r.device.CreateTextureView(tex, nil)
		if err != nil {
			t.Fatalf("CreateTextureView() error = %v", err)
		}
		desc.ColorAttachments = []RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}
	}
	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	return encoder, pass
}

// pipeline returns a pipeline rendering to formats with the given layouts.
func (r *bundleTestRig) pipeline(formats []gputypes.TextureFormat, layouts ...*BindGroupLayout) *RenderPipeline {
	return &RenderPipeline{
		hal:              &testHALResource{},
		device:           r.device,
		bindGroupLayouts: layouts,
		passContext:      core.RenderPassContext{ColorFormats: formats, SampleCount: 1},
		ref:              core.NewResourceRef("RenderPipeline:test", nil),
	}
}

func (r *bundleTestRig) group(layout *BindGroupLayout) *BindGroup {
	return &BindGroup{hal: &testHALResource{}, device: r.device, layout: layout, ref: core.NewResourceRef("BindGroup:test", nil)}
}

func (r *bundleTestRig) buffer(t *testing.T, usage gputypes.BufferUsage) *Buffer {
	t.Helper()
	buf, err := r.device.CreateBuffer(&BufferDescriptor{Label: "bundle", Size: 256, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer() error = %v", err)
	}
	return buf
}

func (r *bundleTestRig) encoder(t *testing.T, desc *RenderBundleEncoderDescriptor) *RenderBundleEncoder {
	t.Helper()
	if desc == nil {
		desc = &RenderBundleEncoderDescriptor{ColorFormats: rgba8Formats}
	}
	enc, err := r.device.CreateRenderBundleEncoder(desc)
	if err != nil {
		t.Fatalf("CreateRenderBundleEncoder() error = %v", err)
	}
	return enc
}

func TestRenderBundleEncoderDescriptor(t *testing.T) {
	tests := []struct {
		name                   string
		desc                   RenderBundleEncoderDescriptor
		wantErr                bool
		depthRO, stencilRO     bool
		wantSampleCount        uint32
		wantDepthStencilFormat gputypes.TextureFormat
		wantColorFormatsCopied bool
	}{
		{name: "no attachment", wantErr: true},
		{name: "only undefined color", desc: RenderBundleEncoderDescriptor{ColorFormats: []gputypes.TextureFormat{gputypes.TextureFormatUndefined}}, wantErr: true},
		{name: "depth format as color", desc: RenderBundleEncoderDescriptor{ColorFormats: []gputypes.TextureFormat{gputypes.TextureFormatDepth32Float}}, wantErr: true},
		{name: "compressed color", desc: RenderBundleEncoderDescriptor{ColorFormats: []gputypes.TextureFormat{gputypes.TextureFormatBC1RGBAUnorm}}, wantErr: true},
		{name: "color format as depth", desc: RenderBundleEncoderDescriptor{DepthStencilFormat: gputypes.TextureFormatRGBA8Unorm}, wantErr: true},
		{name: "too many colors", desc: RenderBundleEncoderDescriptor{ColorFormats: make([]gputypes.TextureFormat, 9)}, wantErr: true},
		{name: "color", desc: RenderBundleEncoderDescriptor{ColorFormats: rgba8Formats}, depthRO: true, stencilRO: true, wantSampleCount: 1},
		{name: "depth written, no stencil aspect", desc: RenderBundleEncoderDescriptor{DepthStencilFormat: gputypes.TextureFormatDepth32Float, SampleCount: 4},
			depthRO: false, stencilRO: true, wantSampleCount: 4, wantDepthStencilFormat: gputypes.TextureFormatDepth32Float},
		{name: "stencil read-only", desc: RenderBundleEncoderDescriptor{DepthStencilFormat: gputypes.TextureFormatDepth24PlusStencil8, StencilReadOnly: true},
			depthRO: false, stencilRO: true, wantSampleCount: 1, wantDepthStencilFormat: gputypes.TextureFormatDepth24PlusStencil8},
	}
	rig := newBundleTestRig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := rig.device.CreateRenderBundleEncoder(&tt.desc)
			if tt.wantErr {
				if !errors.Is(err, errBundleDescriptor) {
					t.Fatalf("CreateRenderBundleEncoder() error = %v, want errBundleDescriptor", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateRenderBundleEncoder() error = %v", err)
			}
			got := enc.target
			if got.depthReadOnly != tt.depthRO || got.stencilReadOnly != tt.stencilRO ||
				got.context.SampleCount != tt.wantSampleCount || got.context.DepthStencilFormat != tt.wantDepthStencilFormat {
				t.Fatalf("target = %+v", got)
			}
		})
	}
	if _, err := rig.device.CreateRenderBundleEncoder(nil); err == nil {
		t.Fatal("CreateRenderBundleEncoder(nil) succeeded")
	}
}

func TestRenderBundleFinishValidation(t *testing.T) {
	rig := newBundleTestRig()
	layout := rebindLayout(false)
	other := newBundleTestRig()
	draw := gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1}
	released := rig.pipeline(rgba8Formats)
	released.released = true
	depthWriter := rig.pipeline(nil)
	depthWriter.passContext.DepthStencilFormat = gputypes.TextureFormatDepth32Float
	depthWriter.writesDepth = true
	needsVertex := rig.pipeline(rgba8Formats)
	needsVertex.requiredVertexBuffers = 1
	uniform := rig.buffer(t, BufferUsageUniform)
	vertex := rig.buffer(t, BufferUsageVertex)
	index := rig.buffer(t, BufferUsageIndex)

	tests := []struct {
		name   string
		desc   *RenderBundleEncoderDescriptor
		record func(e *RenderBundleEncoder)
		want   error
	}{
		{"draw without pipeline", nil, func(e *RenderBundleEncoder) { e.Draw(draw) }, ErrDrawMissingPipeline},
		{"nil pipeline", nil, func(e *RenderBundleEncoder) { e.SetPipeline(nil) }, errBundleCommand},
		{"released pipeline", nil, func(e *RenderBundleEncoder) { e.SetPipeline(released) }, ErrReleased},
		{"pipeline of another device", nil, func(e *RenderBundleEncoder) { e.SetPipeline(other.pipeline(rgba8Formats)) }, errBundleDevice},
		{"pipeline color format", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline([]gputypes.TextureFormat{gputypes.TextureFormatBGRA8Unorm}))
		}, &core.RenderPassCompatibilityError{}},
		{"pipeline writes read-only depth", &RenderBundleEncoderDescriptor{
			DepthStencilFormat: gputypes.TextureFormatDepth32Float, DepthReadOnly: true,
		}, func(e *RenderBundleEncoder) { e.SetPipeline(depthWriter) }, errBundleDepthWrite},
		{"missing bind group", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline(rgba8Formats, layout))
			e.Draw(draw)
		}, ErrDrawMissingBindGroup},
		{"bind group of another device", nil, func(e *RenderBundleEncoder) { e.SetBindGroup(0, other.group(layout), nil) }, errBundleDevice},
		{"bind group index out of range", nil, func(e *RenderBundleEncoder) { e.SetBindGroup(MaxBindGroups, rig.group(layout), nil) }, nil},
		{"missing vertex buffer", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(needsVertex)
			e.Draw(draw)
		}, ErrDrawMissingVertexBuffer},
		{"vertex buffer without vertex usage", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, uniform, 0) }, errBundleCommand},
		{"vertex offset unaligned", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, vertex, 2) }, errBundleCommand},
		{"vertex offset past the end", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, vertex, 260) }, errBundleCommand},
		{"nil vertex buffer", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, nil, 0) }, errBundleCommand},
		{"index offset unaligned", nil, func(e *RenderBundleEncoder) { e.SetIndexBuffer(index, gputypes.IndexFormatUint32, 2) }, errBundleCommand},
		{"draw indexed without index buffer", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline(rgba8Formats))
			e.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 3, InstanceCount: 1})
		}, ErrDrawMissingIndexBuffer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := rig.encoder(t, tt.desc)
			tt.record(enc)
			bundle, err := enc.Finish(nil)
			if bundle != nil || err == nil {
				t.Fatalf("Finish() = (%v, %v), want an error", bundle, err)
			}
			if tt.want == nil {
				return
			}
			var compat *core.RenderPassCompatibilityError
			if errors.As(tt.want, &compat) {
				if !errors.As(err, &compat) {
					t.Fatalf("Finish() error = %v, want a RenderPassCompatibilityError", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRenderBundleReplay checks the resolved command list: a group set
// before the pipeline is bound after it, right before the first draw that
// uses it; vertex and index buffers keep their place; a draw with no
// vertices is dropped; the list replays the same way each time.
func TestRenderBundleReplay(t *testing.T) {
	rig := newBundleTestRig()
	layout := rebindLayout(false)
	group := rig.group(layout)
	pipeline := rig.pipeline(rgba8Formats, layout)
	vertex := rig.buffer(t, BufferUsageVertex)
	index := rig.buffer(t, BufferUsageIndex)

	enc := rig.encoder(t, nil)
	enc.SetBindGroup(0, group, nil)
	enc.SetPipeline(pipeline)
	enc.SetVertexBuffer(1, vertex, 4)
	enc.SetIndexBuffer(index, gputypes.IndexFormatUint16, 2)
	enc.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 3, InstanceCount: 1})
	enc.Draw(gputypes.DrawArgs{VertexCount: 0, InstanceCount: 1})
	enc.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	bundle, err := enc.Finish(&RenderBundleDescriptor{Label: "replay"})
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if bundle.label != "replay" {
		t.Errorf("label = %q, want replay", bundle.label)
	}

	encoder, pass := rig.beginPass(t, true)
	pass.ExecuteBundles(bundle, bundle)
	_ = pass.End()
	if _, err := encoder.Finish(); err != nil {
		t.Fatalf("CommandEncoder.Finish() error = %v", err)
	}
	once := []string{
		"SetPipeline", "SetVertexBuffer(1, 4)", "SetIndexBuffer(" + gputypes.IndexFormatUint16.String() + ", 2)",
		"SetBindGroup(0, [])", "DrawIndexed(3)", "Draw(6)",
	}
	if want := slices.Concat(once, once); !slices.Equal(rig.pass.calls, want) {
		t.Fatalf("backend calls =\n%q\nwant\n%q", rig.pass.calls, want)
	}
}

// TestRenderBundleCopiesOffsets checks that the encoder keeps its own copy
// of the dynamic offsets given to SetBindGroup.
func TestRenderBundleCopiesOffsets(t *testing.T) {
	rig := newBundleTestRig()
	enc := rig.encoder(t, nil)
	offsets := []uint32{256}
	enc.SetBindGroup(0, rig.group(rebindLayout(true)), offsets)
	offsets[0] = 512
	if got := enc.commands[0].offsets; !slices.Equal(got, []uint32{256}) {
		t.Fatalf("recorded offsets = %v, want [256]", got)
	}
}

// TestRenderBundleEncoderAfterFinish checks that commands after Finish are
// ignored and that a second Finish fails.
func TestRenderBundleEncoderAfterFinish(t *testing.T) {
	rig := newBundleTestRig()
	enc := rig.encoder(t, nil)
	if _, err := enc.Finish(nil); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	enc.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if len(enc.commands) != 0 {
		t.Fatalf("commands after Finish = %d, want 0", len(enc.commands))
	}
	if bundle, err := enc.Finish(nil); bundle != nil || !errors.Is(err, ErrReleased) {
		t.Fatalf("second Finish() = (%v, %v), want ErrReleased", bundle, err)
	}
	var nilEncoder *RenderBundleEncoder
	if _, err := nilEncoder.Finish(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("nil Finish() error = %v, want ErrReleased", err)
	}
}

// TestRenderBundleKeepsResources checks that a bundle holds a reference to
// each resource it uses until Release, and that executing it gives the
// command encoder references of its own.
func TestRenderBundleKeepsResources(t *testing.T) {
	rig := newBundleTestRig()
	layout := rebindLayout(false)
	pipeline := rig.pipeline(rgba8Formats, layout)
	group := rig.group(layout)
	enc := rig.encoder(t, nil)
	enc.SetPipeline(pipeline)
	enc.SetBindGroup(0, group, nil)
	enc.SetPipeline(pipeline)
	enc.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	bundle, err := enc.Finish(nil)
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if got := pipeline.ref.RefCount(); got != 2 {
		t.Fatalf("pipeline refs after Finish = %d, want 2 (one per resource, however often it is used)", got)
	}

	encoder, pass := rig.beginPass(t, true)
	pass.ExecuteBundles(bundle)
	if got := group.ref.RefCount(); got != 3 {
		t.Fatalf("bind group refs after ExecuteBundles = %d, want 3", got)
	}
	bundle.Release()
	bundle.Release()
	if got := group.ref.RefCount(); got != 2 {
		t.Fatalf("bind group refs after Release = %d, want 2 (the encoder keeps its own)", got)
	}
	pass.ExecuteBundles(bundle)
	_ = pass.End()
	if _, err := encoder.Finish(); !errors.Is(err, ErrReleased) {
		t.Fatalf("Finish() after executing a released bundle: error = %v, want ErrReleased", err)
	}
	var nilBundle *RenderBundle
	nilBundle.Release()
}

func TestExecuteBundlesChecks(t *testing.T) {
	rig := newBundleTestRig()
	other := newBundleTestRig()
	colorBundle := newEmptyBundle(rig.device)
	colorBundle.target.context.ColorFormats = rgba8Formats
	writingBundle := newEmptyBundle(rig.device)
	writingBundle.target.depthReadOnly = false
	tests := []struct {
		name     string
		bundle   *RenderBundle
		readOnly bool
		want     error
	}{
		{name: "nil", want: errBundleCommand},
		{name: "another device", bundle: newEmptyBundle(other.device), want: errBundleDevice},
		{name: "color formats", bundle: colorBundle, want: &core.RenderPassCompatibilityError{}},
		{name: "depth written in a read-only pass", bundle: writingBundle, readOnly: true, want: errBundleReadOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, pass := rig.beginPass(t, false)
			pass.depthReadOnly = tt.readOnly
			pass.ExecuteBundles(tt.bundle)
			_ = pass.End()
			_, err := encoder.Finish()
			var compat *core.RenderPassCompatibilityError
			if errors.As(tt.want, &compat) {
				if !errors.As(err, &compat) {
					t.Fatalf("Finish() error = %v, want a RenderPassCompatibilityError", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestRenderBundleUsageScope checks that a bundle's buffer usages conflict
// with each other when the bundle is finished, and with the pass's usages
// when it is executed (Rust RenderBundleScope, merge_render_bundle).
func TestRenderBundleUsageScope(t *testing.T) {
	rig := newBundleTestRig()
	layout := rebindLayout(false)
	buffer := rig.buffer(t, BufferUsageVertex|BufferUsageStorage)
	if td := buffer.core.TrackingData(); td == nil || !td.Index().IsValid() {
		t.Skip("noop buffers have no tracker index")
	}
	writer := rig.group(layout)
	writer.usages = &core.BindGroupUsages{Buffers: []core.BindGroupBufferUse{{Buffer: buffer.core, Usage: track.BufferUsesStorageWrite}}}

	t.Run("inside the bundle", func(t *testing.T) {
		enc := rig.encoder(t, nil)
		enc.SetBindGroup(0, writer, nil)
		enc.SetVertexBuffer(0, buffer, 0)
		if _, err := enc.Finish(nil); err == nil {
			t.Fatal("Finish() succeeded, want a usage conflict")
		}
	})
	t.Run("with the pass", func(t *testing.T) {
		enc := rig.encoder(t, nil)
		enc.SetBindGroup(0, writer, nil)
		bundle, err := enc.Finish(nil)
		if err != nil {
			t.Fatalf("Finish() error = %v", err)
		}
		encoder, pass := rig.beginPass(t, true)
		pass.SetVertexBuffer(0, buffer, 0)
		pass.ExecuteBundles(bundle)
		_ = pass.End()
		if _, err := encoder.Finish(); err == nil {
			t.Fatal("CommandEncoder.Finish() succeeded, want a usage conflict")
		}
	})
}
