//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// The bundle tests moved here with the types (S6's Move): they read the
// encoder's recording, the bundle's label and the pass's read-only flags,
// which no root declaration layer can name. The fixtures are local — a core
// device over the test HAL doubles and resources built from it. No
// internal/hal/noop import: a test import is a graph edge like any other
// (layoutcheck golden-subset).

var rgba8Formats = []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}

// testHALRenderResource is the HAL pipeline and bind group double: the
// fixtures never call anything through it.
type testHALRenderResource struct{ hal.RenderPipeline }

func (*testHALRenderResource) Destroy() {}

func (*testHALRenderResource) NativeHandle() uintptr { return 0 }

// bundleTestRenderPass records the commands core sends to the backend pass.
type bundleTestRenderPass struct {
	hal.RenderPassEncoder
	calls []string
}

func (p *bundleTestRenderPass) End() {}

func (p *bundleTestRenderPass) SetPipeline(hal.RenderPipeline) {
	p.calls = append(p.calls, "SetPipeline")
}

func (p *bundleTestRenderPass) SetBindGroup(index uint32, _ hal.BindGroup, offsets []uint32) {
	p.calls = append(p.calls, fmt.Sprintf("SetBindGroup(%d, %v)", index, offsets))
}

func (p *bundleTestRenderPass) SetVertexBuffer(slot uint32, _ hal.Buffer, offset uint64) {
	p.calls = append(p.calls, fmt.Sprintf("SetVertexBuffer(%d, %d)", slot, offset))
}

func (p *bundleTestRenderPass) SetIndexBuffer(_ hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	p.calls = append(p.calls, fmt.Sprintf("SetIndexBuffer(%v, %d)", format, offset))
}

func (p *bundleTestRenderPass) Draw(args gputypes.DrawArgs) {
	p.calls = append(p.calls, fmt.Sprintf("Draw(%d)", args.VertexCount))
}

func (p *bundleTestRenderPass) DrawIndexed(args gputypes.DrawIndexedArgs) {
	p.calls = append(p.calls, fmt.Sprintf("DrawIndexed(%d)", args.IndexCount))
}

// bundleTestCommandEncoder hands out the recording pass on BeginRenderPass and
// carries the test HAL encoder's lifecycle doubles.
type bundleTestCommandEncoder struct {
	testHALCommandEncoder
	pass *bundleTestRenderPass
}

func (e *bundleTestCommandEncoder) BeginRenderPass(*hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return e.pass
}

// bundleTestHALDevice is the HAL device whose command encoder records.
type bundleTestHALDevice struct {
	testHALDevice
	commandEncoder *bundleTestCommandEncoder
}

func (d bundleTestHALDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return d.commandEncoder, nil
}

// bundleTestRig is a core device with the default limits (the bundle
// validation reads maxColorAttachments and maxVertexBuffers) and a render pass
// whose backend pass records the commands it gets.
type bundleTestRig struct {
	device *core.Device
	view   testDeviceView
	pass   *bundleTestRenderPass
}

func newBundleTestRig(t *testing.T) *bundleTestRig {
	t.Helper()
	command := &bundleTestCommandEncoder{pass: &bundleTestRenderPass{}}
	raw := bundleTestHALDevice{commandEncoder: command}
	limits := gputypes.DefaultLimits()
	// Zero workgroup limits keep the indirect-validation pipeline, which would
	// need a real HAL device, out of the fixture.
	limits.MaxComputeWorkgroupsPerDimension = 0
	device := core.NewDevice(raw, nil, 0, limits, "bundle-test")
	t.Cleanup(device.Destroy)
	return &bundleTestRig{device: device, view: testDeviceView{device: device, raw: raw}, pass: command.pass}
}

// newEmptyBundle returns a finished bundle with no commands for a render
// pass with no attachments.
func newEmptyBundle(view testDeviceView) *RenderBundle {
	return &RenderBundle{
		device: view,
		bundleContents: &bundleContents{
			target: renderBundleTarget{context: core.RenderPassContext{SampleCount: 1}, depthReadOnly: true, stencilReadOnly: true},
			scope:  core.NewRenderBundleScope(),
		},
	}
}

// beginPass begins a render pass with no attachments, or with one RGBA8
// color attachment.
func (r *bundleTestRig) beginPass(t *testing.T, color bool) (*CommandEncoder, *RenderPassEncoder) {
	t.Helper()
	coreEncoder, err := r.device.CreateCommandEncoder("bundle-test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	encoder := &CommandEncoder{core: coreEncoder}
	desc := &RenderPassDescriptor{}
	if color {
		tex := newTrackedTexture(t, r.device, r.view, "attachment", gputypes.TextureUsageRenderAttachment)
		view := newTestTextureView(t, r.device, r.view, tex)
		desc.ColorAttachments = []RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}
	}
	start, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	return encoder, NewRenderPassEncoder(start, encoder)
}

// pipeline returns a pipeline rendering to formats with the given layouts.
func (r *bundleTestRig) pipeline(formats []gputypes.TextureFormat, layouts ...*resource.BindGroupLayout) *pipeline.RenderPipeline {
	return r.pipelineInit(&pipeline.RenderPipelineInit{
		BindGroupLayouts: layouts,
		PassContext:      core.RenderPassContext{ColorFormats: formats, SampleCount: 1},
	})
}

// pipelineInit returns a pipeline over init, with the rig's device, a HAL
// double and a fresh ref filled in where init leaves them unset.
func (r *bundleTestRig) pipelineInit(init *pipeline.RenderPipelineInit) *pipeline.RenderPipeline {
	init.Device = r.view
	if init.HAL == nil {
		init.HAL = &testHALRenderResource{}
	}
	if init.Ref == nil {
		init.Ref = core.NewResourceRef("RenderPipeline:test", nil)
	}
	return pipeline.NewRenderPipeline(init)
}

func (r *bundleTestRig) group(layout *resource.BindGroupLayout) *resource.BindGroup {
	return resource.NewBindGroup(&resource.BindGroupInit{
		HAL:    &testHALRenderResource{},
		Device: r.view,
		Layout: layout,
		Ref:    core.NewResourceRef("BindGroup:test", nil),
	})
}

func (r *bundleTestRig) buffer(t *testing.T, usage gputypes.BufferUsage) *resource.Buffer {
	t.Helper()
	return newTestResourceBuffer(t, r.device, r.view, 256, usage, "bundle")
}

func (r *bundleTestRig) encoder(t *testing.T, desc *RenderBundleEncoderDescriptor) *RenderBundleEncoder {
	t.Helper()
	if desc == nil {
		desc = &RenderBundleEncoderDescriptor{ColorFormats: rgba8Formats}
	}
	enc, err := NewRenderBundleEncoder(r.view, desc)
	if err != nil {
		t.Fatalf("NewRenderBundleEncoder() error = %v", err)
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
	rig := newBundleTestRig(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewRenderBundleEncoder(rig.view, &tt.desc)
			if tt.wantErr {
				if !errors.Is(err, ErrBundleDescriptor) {
					t.Fatalf("NewRenderBundleEncoder() error = %v, want ErrBundleDescriptor", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRenderBundleEncoder() error = %v", err)
			}
			got := enc.target
			if got.depthReadOnly != tt.depthRO || got.stencilReadOnly != tt.stencilRO ||
				got.context.SampleCount != tt.wantSampleCount || got.context.DepthStencilFormat != tt.wantDepthStencilFormat {
				t.Fatalf("target = %+v", got)
			}
		})
	}
}

func TestRenderBundleFinishValidation(t *testing.T) {
	rig := newBundleTestRig(t)
	layout := rebindLayout(false)
	other := newBundleTestRig(t)
	draw := gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1}
	released := rig.pipeline(rgba8Formats)
	released.Release()
	depthWriter := rig.pipelineInit(&pipeline.RenderPipelineInit{
		PassContext: core.RenderPassContext{DepthStencilFormat: gputypes.TextureFormatDepth32Float, SampleCount: 1},
		WritesDepth: true,
	})
	needsVertex := rig.pipeline(rgba8Formats)
	needsVertex.SetRequiredVertexBuffers(1)
	uniform := rig.buffer(t, gputypes.BufferUsageUniform)
	vertex := rig.buffer(t, gputypes.BufferUsageVertex)
	index := rig.buffer(t, gputypes.BufferUsageIndex)

	tests := []struct {
		name   string
		desc   *RenderBundleEncoderDescriptor
		record func(e *RenderBundleEncoder)
		want   error
	}{
		{"draw without pipeline", nil, func(e *RenderBundleEncoder) { e.Draw(draw) }, core.ErrDrawMissingPipeline},
		{"nil pipeline", nil, func(e *RenderBundleEncoder) { e.SetPipeline(nil) }, ErrBundleCommand},
		{"released pipeline", nil, func(e *RenderBundleEncoder) { e.SetPipeline(released) }, core.ErrReleased},
		{"pipeline of another device", nil, func(e *RenderBundleEncoder) { e.SetPipeline(other.pipeline(rgba8Formats)) }, ErrBundleDevice},
		{"pipeline color format", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline([]gputypes.TextureFormat{gputypes.TextureFormatBGRA8Unorm}))
		}, &core.RenderPassCompatibilityError{}},
		{"pipeline writes read-only depth", &RenderBundleEncoderDescriptor{
			DepthStencilFormat: gputypes.TextureFormatDepth32Float, DepthReadOnly: true,
		}, func(e *RenderBundleEncoder) { e.SetPipeline(depthWriter) }, ErrBundleDepthWrite},
		{"missing bind group", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline(rgba8Formats, layout))
			e.Draw(draw)
		}, core.ErrDrawMissingBindGroup},
		{"bind group of another device", nil, func(e *RenderBundleEncoder) { e.SetBindGroup(0, other.group(layout), nil) }, ErrBundleDevice},
		{"bind group index out of range", nil, func(e *RenderBundleEncoder) { e.SetBindGroup(core.MaxBindGroups, rig.group(layout), nil) }, nil},
		{"missing vertex buffer", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(needsVertex)
			e.Draw(draw)
		}, core.ErrDrawMissingVertexBuffer},
		{"vertex buffer without vertex usage", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, uniform, 0) }, ErrBundleCommand},
		{"vertex offset unaligned", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, vertex, 2) }, ErrBundleCommand},
		{"vertex offset past the end", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, vertex, 260) }, ErrBundleCommand},
		{"nil vertex buffer", nil, func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, nil, 0) }, ErrBundleCommand},
		{"index offset unaligned", nil, func(e *RenderBundleEncoder) { e.SetIndexBuffer(index, gputypes.IndexFormatUint32, 2) }, ErrBundleCommand},
		{"draw indexed without index buffer", nil, func(e *RenderBundleEncoder) {
			e.SetPipeline(rig.pipeline(rgba8Formats))
			e.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 3, InstanceCount: 1})
		}, core.ErrDrawMissingIndexBuffer},
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
	rig := newBundleTestRig(t)
	layout := rebindLayout(false)
	group := rig.group(layout)
	pipe := rig.pipeline(rgba8Formats, layout)
	vertex := rig.buffer(t, gputypes.BufferUsageVertex)
	index := rig.buffer(t, gputypes.BufferUsageIndex)

	enc := rig.encoder(t, nil)
	enc.SetBindGroup(0, group, nil)
	enc.SetPipeline(pipe)
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
	rig := newBundleTestRig(t)
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
	rig := newBundleTestRig(t)
	enc := rig.encoder(t, nil)
	if _, err := enc.Finish(nil); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	enc.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if len(enc.commands) != 0 {
		t.Fatalf("commands after Finish = %d, want 0", len(enc.commands))
	}
	if bundle, err := enc.Finish(nil); bundle != nil || !errors.Is(err, core.ErrReleased) {
		t.Fatalf("second Finish() = (%v, %v), want ErrReleased", bundle, err)
	}
	var nilEncoder *RenderBundleEncoder
	if _, err := nilEncoder.Finish(nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("nil Finish() error = %v, want ErrReleased", err)
	}
}

// TestRenderBundleKeepsResources checks that a bundle holds a reference to
// each resource it uses until Release, and that executing it gives the
// command encoder references of its own.
func TestRenderBundleKeepsResources(t *testing.T) {
	rig := newBundleTestRig(t)
	layout := rebindLayout(false)
	pipe := rig.pipeline(rgba8Formats, layout)
	group := rig.group(layout)
	enc := rig.encoder(t, nil)
	enc.SetPipeline(pipe)
	enc.SetBindGroup(0, group, nil)
	enc.SetPipeline(pipe)
	enc.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	bundle, err := enc.Finish(nil)
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if got := pipe.Ref().RefCount(); got != 2 {
		t.Fatalf("pipeline refs after Finish = %d, want 2 (one per resource, however often it is used)", got)
	}

	encoder, pass := rig.beginPass(t, true)
	pass.ExecuteBundles(bundle)
	if got := group.Ref().RefCount(); got != 3 {
		t.Fatalf("bind group refs after ExecuteBundles = %d, want 3", got)
	}
	bundle.Release()
	bundle.Release()
	if got := group.Ref().RefCount(); got != 2 {
		t.Fatalf("bind group refs after Release = %d, want 2 (the encoder keeps its own)", got)
	}
	pass.ExecuteBundles(bundle)
	_ = pass.End()
	if _, err := encoder.Finish(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("Finish() after executing a released bundle: error = %v, want ErrReleased", err)
	}
	var nilBundle *RenderBundle
	nilBundle.Release()
}

func TestExecuteBundlesChecks(t *testing.T) {
	rig := newBundleTestRig(t)
	other := newBundleTestRig(t)
	colorBundle := newEmptyBundle(rig.view)
	colorBundle.target.context.ColorFormats = rgba8Formats
	writingBundle := newEmptyBundle(rig.view)
	writingBundle.target.depthReadOnly = false
	tests := []struct {
		name     string
		bundle   *RenderBundle
		readOnly bool
		want     error
	}{
		{name: "nil", want: ErrBundleCommand},
		{name: "another device", bundle: newEmptyBundle(other.view), want: ErrBundleDevice},
		{name: "color formats", bundle: colorBundle, want: &core.RenderPassCompatibilityError{}},
		{name: "depth written in a read-only pass", bundle: writingBundle, readOnly: true, want: ErrBundleReadOnly},
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

// TestExecuteBundlesResetsPassStateAfterError pins the pre-move contract that
// the pass state is cleared even when a bundle fails its checks: the reset
// runs once after the loop, error break included.
func TestExecuteBundlesResetsPassStateAfterError(t *testing.T) {
	rig := newBundleTestRig(t)
	_, pass := rig.beginPass(t, false)
	layout := rebindLayout(false)
	pass.SetPassStateForTest(layout, rig.group(layout))

	pass.ExecuteBundles(nil) // a nil bundle fails the first check
	if set, group, assigned := pass.PassStateForTest(); set || group != nil || assigned != nil {
		t.Fatal("ExecuteBundles kept the pipeline or the bind groups after an error")
	}
}

// TestRenderBundleUsageScope checks that a bundle's buffer usages conflict
// with each other when the bundle is finished, and with the pass's usages
// when it is executed (Rust RenderBundleScope, merge_render_bundle).
func TestRenderBundleUsageScope(t *testing.T) {
	rig := newBundleTestRig(t)
	layout := rebindLayout(false)
	buffer := rig.buffer(t, gputypes.BufferUsageVertex|gputypes.BufferUsageStorage)
	if td := buffer.CoreBuffer().TrackingData(); td == nil || !td.Index().IsValid() {
		t.Skip("the test buffers have no tracker index")
	}
	writer := resource.NewBindGroup(&resource.BindGroupInit{
		HAL:    &testHALRenderResource{},
		Device: rig.view,
		Layout: layout,
		Ref:    core.NewResourceRef("BindGroup:test", nil),
		Usages: &core.BindGroupUsages{Buffers: []core.BindGroupBufferUse{{Buffer: buffer.CoreBuffer(), Usage: track.BufferUsesStorageWrite}}},
	})

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
