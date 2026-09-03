//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

type testHALResource struct{}

func (*testHALResource) Destroy() {}

func (*testHALResource) NativeHandle() uintptr { return 0 }

type captureRenderPass struct {
	*noop.RenderPassEncoder
	executed []hal.RenderBundle
}

func (p *captureRenderPass) ExecuteBundle(bundle hal.RenderBundle) {
	p.executed = append(p.executed, bundle)
}

type captureCommandEncoder struct {
	*noop.CommandEncoder
	resolved   int
	querySet   hal.QuerySet
	buffer     hal.Buffer
	renderPass *captureRenderPass
}

func (e *captureCommandEncoder) ResolveQuerySet(querySet hal.QuerySet, _, _ uint32, buffer hal.Buffer, _ uint64) {
	e.resolved++
	e.querySet = querySet
	e.buffer = buffer
}

func (e *captureCommandEncoder) BeginRenderPass(*hal.RenderPassDescriptor) hal.RenderPassEncoder {
	if e.renderPass == nil {
		return nil
	}
	return e.renderPass
}

type captureDevice struct {
	hal.Device
	commandEncoder         hal.CommandEncoder
	renderBundleEncoder    hal.RenderBundleEncoder
	renderBundleEncoderErr error
	destroyedRenderBundles int
}

func (d *captureDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return d.commandEncoder, nil
}

func (d *captureDevice) CreateRenderBundleEncoder(*hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return d.renderBundleEncoder, d.renderBundleEncoderErr
}

func (d *captureDevice) DestroyRenderBundle(hal.RenderBundle) { d.destroyedRenderBundles++ }

func newCaptureDevice(h *captureDevice) *Device {
	limits := DefaultLimits()
	return &Device{core: core.NewDevice(h, nil, 0, limits, "query-render-bundle-test")}
}

func newTestQuerySet(raw hal.QuerySet, device *Device) *QuerySet {
	return &QuerySet{
		core:   core.NewQuerySet(raw, device.core, hal.QueryTypeTimestamp, 4, "test query set"),
		device: device,
	}
}

type testRenderBundleEncoder struct {
	setPipelineCalls     int
	setBindGroupCalls    int
	setVertexBufferCalls int
	setIndexBufferCalls  int
	drawCalls            int
	drawIndexedCalls     int
	finishCalls          int
	bundle               hal.RenderBundle
}

func (e *testRenderBundleEncoder) SetPipeline(hal.RenderPipeline) { e.setPipelineCalls++ }
func (e *testRenderBundleEncoder) SetBindGroup(uint32, hal.BindGroup, []uint32) {
	e.setBindGroupCalls++
}
func (e *testRenderBundleEncoder) SetVertexBuffer(uint32, hal.Buffer, uint64) {
	e.setVertexBufferCalls++
}
func (e *testRenderBundleEncoder) SetIndexBuffer(hal.Buffer, gputypes.IndexFormat, uint64) {
	e.setIndexBufferCalls++
}
func (e *testRenderBundleEncoder) Draw(gputypes.DrawArgs) { e.drawCalls++ }
func (e *testRenderBundleEncoder) DrawIndexed(gputypes.DrawIndexedArgs) {
	e.drawIndexedCalls++
}
func (e *testRenderBundleEncoder) Finish() hal.RenderBundle {
	e.finishCalls++
	return e.bundle
}

func (e *testRenderBundleEncoder) commandCalls() int {
	return e.setPipelineCalls + e.setBindGroupCalls + e.setVertexBufferCalls +
		e.setIndexBufferCalls + e.drawCalls + e.drawIndexedCalls
}

func TestRenderBundleEncoderDescriptorToHAL(t *testing.T) {
	tests := []struct {
		name string
		desc *RenderBundleEncoderDescriptor
	}{
		{name: "nil"},
		{name: "populated", desc: &RenderBundleEncoderDescriptor{
			Label: "encoder", ColorFormats: []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm},
			DepthStencilFormat: gputypes.TextureFormatDepth24Plus, SampleCount: 4,
			DepthReadOnly: true, StencilReadOnly: true,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.desc.toHAL()
			if tt.desc == nil {
				if got != nil {
					t.Fatalf("toHAL() = %+v, want nil", got)
				}
				return
			}
			if got.Label != tt.desc.Label || got.SampleCount != tt.desc.SampleCount ||
				got.DepthReadOnly != tt.desc.DepthReadOnly || got.StencilReadOnly != tt.desc.StencilReadOnly ||
				got.DepthStencilFormat != tt.desc.DepthStencilFormat || len(got.ColorFormats) != 1 {
				t.Fatalf("toHAL() = %+v", got)
			}
		})
	}
}

func TestRenderPassDescriptorTimestampWrites(t *testing.T) {
	begin, end := uint32(2), uint32(3)
	resource := &testHALResource{}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}})
	active := newTestQuerySet(resource, device)
	released := newTestQuerySet(resource, device)
	released.released = true

	tests := []struct {
		name    string
		writes  *RenderPassTimestampWrites
		wantNil bool
	}{
		{name: "absent", writes: nil, wantNil: true},
		{name: "nil query set", writes: &RenderPassTimestampWrites{}, wantNil: true},
		{name: "released query set", writes: &RenderPassTimestampWrites{QuerySet: released}, wantNil: true},
		{
			name: "active query set",
			writes: &RenderPassTimestampWrites{
				QuerySet:                  active,
				BeginningOfPassWriteIndex: &begin,
				EndOfPassWriteIndex:       &end,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&RenderPassDescriptor{TimestampWrites: tt.writes}).toHAL().TimestampWrites
			if tt.wantNil {
				if got != nil {
					t.Fatalf("TimestampWrites = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.QuerySet != resource || got.BeginningOfPassWriteIndex != &begin || got.EndOfPassWriteIndex != &end {
				t.Fatalf("TimestampWrites = %+v", got)
			}
		})
	}
}

func TestRenderBundleEncoderCommandsAfterFinish(t *testing.T) {
	resource := &testHALResource{}
	tests := []struct {
		name string
		call func(*RenderBundleEncoder)
	}{
		{name: "SetPipeline", call: func(e *RenderBundleEncoder) { e.SetPipeline(&RenderPipeline{hal: resource}) }},
		{name: "SetBindGroup", call: func(e *RenderBundleEncoder) { e.SetBindGroup(0, &BindGroup{hal: resource}, nil) }},
		{name: "SetVertexBuffer", call: func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, &Buffer{}, 0) }},
		{name: "SetIndexBuffer", call: func(e *RenderBundleEncoder) { e.SetIndexBuffer(&Buffer{}, gputypes.IndexFormatUint16, 0) }},
		{name: "Draw", call: func(e *RenderBundleEncoder) { e.Draw(3, 1, 0, 0) }},
		{name: "DrawIndexed", call: func(e *RenderBundleEncoder) { e.DrawIndexed(3, 1, 0, 0, 0) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &testRenderBundleEncoder{bundle: resource}
			encoder := &RenderBundleEncoder{hal: h}
			tt.call(encoder)
			if got := h.commandCalls(); got != 1 {
				t.Fatalf("HAL command calls before Finish = %d, want 1", got)
			}
			if _, err := encoder.Finish(nil); err != nil {
				t.Fatalf("Finish() error = %v", err)
			}
			tt.call(encoder)
			if got := h.commandCalls(); got != 1 {
				t.Fatalf("HAL command calls after Finish = %d, want 1", got)
			}
		})
	}
}

func TestRenderBundleEncoderFinish(t *testing.T) {
	resource := &testHALResource{}
	tests := []struct {
		name      string
		desc      *RenderBundleDescriptor
		wantLabel string
	}{
		{name: "nil descriptor", desc: nil},
		{name: "label", desc: &RenderBundleDescriptor{Label: "test bundle"}, wantLabel: "test bundle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &testRenderBundleEncoder{bundle: resource}
			encoder := &RenderBundleEncoder{hal: h}
			bundle, err := encoder.Finish(tt.desc)
			if err != nil {
				t.Fatalf("Finish() error = %v", err)
			}
			if bundle == nil || bundle.hal != resource || bundle.label != tt.wantLabel {
				t.Fatalf("Finish() bundle = %+v", bundle)
			}
			if h.finishCalls != 1 {
				t.Fatalf("HAL Finish calls = %d, want 1", h.finishCalls)
			}
			if second, secondErr := encoder.Finish(nil); second != nil || !errors.Is(secondErr, ErrReleased) {
				t.Fatalf("second Finish() = (%v, %v), want (nil, ErrReleased)", second, secondErr)
			}
		})
	}
}

func TestDeviceCreateRenderBundleEncoder(t *testing.T) {
	resource := &testHALResource{}
	tests := []struct {
		name       string
		desc       *RenderBundleEncoderDescriptor
		released   bool
		halErr     error
		wantErr    bool
		wantResult bool
	}{
		{name: "nil descriptor", wantErr: true},
		{name: "released device", desc: &RenderBundleEncoderDescriptor{}, released: true, wantErr: true},
		{name: "HAL error", desc: &RenderBundleEncoderDescriptor{}, halErr: errors.New("create failed"), wantErr: true},
		{name: "success", desc: &RenderBundleEncoderDescriptor{}, wantResult: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &testRenderBundleEncoder{bundle: resource}
			deviceHAL := &captureDevice{Device: &noop.Device{}, renderBundleEncoder: h, renderBundleEncoderErr: tt.halErr}
			device := newCaptureDevice(deviceHAL)
			if tt.released {
				device.released.Store(true)
			}
			got, err := device.CreateRenderBundleEncoder(tt.desc)
			if (err != nil) != tt.wantErr || (got != nil) != tt.wantResult {
				t.Fatalf("CreateRenderBundleEncoder() = (%v, %v)", got, err)
			}
		})
	}
}

func TestRenderBundleRelease(t *testing.T) {
	resource := &testHALResource{}
	h := &captureDevice{Device: &noop.Device{}}
	device := newCaptureDevice(h)
	bundle := &RenderBundle{hal: resource, device: device}
	bundle.Release()
	bundle.Release()
	if got := h.destroyedRenderBundles; got != 1 {
		t.Fatalf("HAL destroy calls = %d, want 1", got)
	}
	var nilRenderBundle *RenderBundle
	nilRenderBundle.Release()
}

func TestResolveQuerySetAndExecuteBundles(t *testing.T) {
	resource := &testHALResource{}
	pass := &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}, renderPass: pass}
	h := &captureDevice{Device: &noop.Device{}, commandEncoder: command}
	device := newCaptureDevice(h)

	buffer, err := device.CreateBuffer(&BufferDescriptor{Size: 256, Usage: BufferUsageQueryResolve | BufferUsageCopySrc})
	if err != nil {
		t.Fatalf("CreateBuffer() error = %v", err)
	}
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	querySet := newTestQuerySet(resource, device)
	encoder.ResolveQuerySet(querySet, 0, 1, buffer, 0)
	if command.resolved != 1 || command.querySet != resource || command.buffer == nil {
		t.Fatalf("ResolveQuerySet delegation = (%d, %v, %v)", command.resolved, command.querySet, command.buffer)
	}

	renderPass, err := encoder.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	released := &RenderBundle{hal: resource}
	released.released.Store(true)
	renderPass.ExecuteBundles(nil, released, &RenderBundle{}, &RenderBundle{hal: resource})
	if len(pass.executed) != 1 || pass.executed[0] != resource {
		t.Fatalf("executed bundles = %v, want one active bundle", pass.executed)
	}
}

func TestResolveQuerySetGuards(t *testing.T) {
	resource := &testHALResource{}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
	h := &captureDevice{Device: &noop.Device{}, commandEncoder: command}
	device := newCaptureDevice(h)
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	querySet := newTestQuerySet(resource, device)
	releasedQuerySet := newTestQuerySet(resource, device)
	releasedQuerySet.released = true
	activeDestination := &Buffer{released: &atomic.Bool{}}
	releasedDestination := &Buffer{released: &atomic.Bool{}}
	releasedDestination.released.Store(true)

	tests := []struct {
		name        string
		encoder     *CommandEncoder
		querySet    *QuerySet
		destination *Buffer
	}{
		{name: "released encoder", encoder: &CommandEncoder{released: true}, querySet: querySet, destination: activeDestination},
		{name: "nil query set", encoder: encoder, destination: activeDestination},
		{name: "released query set", encoder: encoder, querySet: releasedQuerySet, destination: activeDestination},
		{name: "nil destination", encoder: encoder, querySet: querySet},
		{name: "released destination", encoder: encoder, querySet: querySet, destination: releasedDestination},
		{name: "nil HAL query set", encoder: encoder, querySet: &QuerySet{}, destination: activeDestination},
		{name: "nil HAL destination", encoder: encoder, querySet: querySet, destination: activeDestination},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.encoder.ResolveQuerySet(tt.querySet, 0, 1, tt.destination, 0)
			if command.resolved != 0 {
				t.Fatalf("HAL ResolveQuerySet calls = %d, want 0", command.resolved)
			}
		})
	}

	nilRawCommand := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
	nilRawDevice := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: nilRawCommand})
	nilRawEncoder, err := nilRawDevice.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	if _, err := nilRawEncoder.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	nilRawEncoder.core.TakeHALEncoder()
	nilRawEncoder.released = false
	nilRawEncoder.ResolveQuerySet(querySet, 0, 1, activeDestination, 0)
}

func TestResolveQuerySetRecordsErrors(t *testing.T) {
	resource := &testHALResource{}
	tests := []struct {
		name        string
		querySet    func(*Device) *QuerySet
		destination *Buffer
	}{
		{name: "nil query set", destination: &Buffer{released: &atomic.Bool{}}},
		{name: "nil destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(resource, d) }},
		{name: "released query set", querySet: func(d *Device) *QuerySet {
			q := newTestQuerySet(resource, d)
			q.released = true
			return q
		}, destination: &Buffer{released: &atomic.Bool{}}},
		{name: "released destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(resource, d) }, destination: func() *Buffer {
			b := &Buffer{released: &atomic.Bool{}}
			b.released.Store(true)
			return b
		}()},
		{name: "nil HAL query set", querySet: func(*Device) *QuerySet { return &QuerySet{} }, destination: &Buffer{released: &atomic.Bool{}}},
		{name: "nil HAL destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(resource, d) }, destination: &Buffer{released: &atomic.Bool{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
			device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
			encoder, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder() error = %v", err)
			}
			var querySet *QuerySet
			if tt.querySet != nil {
				querySet = tt.querySet(device)
			}
			encoder.ResolveQuerySet(querySet, 0, 1, tt.destination, 0)
			if command.resolved != 0 {
				t.Fatalf("HAL ResolveQuerySet calls = %d, want 0", command.resolved)
			}
			if _, err := encoder.Finish(); err == nil {
				t.Fatal("Finish() error = nil, want deferred ResolveQuerySet error")
			}
		})
	}
}

func TestExecuteBundlesWithoutRawPass(t *testing.T) {
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	renderPass, err := encoder.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	renderPass.ExecuteBundles(&RenderBundle{hal: &testHALResource{}})
}

func TestCreationWithoutHALDevice(t *testing.T) {
	device := &Device{core: nil}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "render bundle encoder", call: func() error { _, err := device.CreateRenderBundleEncoder(&RenderBundleEncoderDescriptor{}); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrReleased) {
				t.Fatalf("error = %v, want ErrReleased", err)
			}
		})
	}
}
