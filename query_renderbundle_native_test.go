//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/resource"
)

type testHALResource struct{}

func (*testHALResource) Destroy() {}

func (*testHALResource) NativeHandle() uintptr { return 0 }

// captureRenderPass records the commands core sends to the backend pass.
type captureRenderPass struct {
	*noop.RenderPassEncoder
	calls []string
}

func (p *captureRenderPass) SetPipeline(hal.RenderPipeline) {
	p.calls = append(p.calls, "SetPipeline")
}

func (p *captureRenderPass) SetBindGroup(index uint32, _ hal.BindGroup, offsets []uint32) {
	p.calls = append(p.calls, fmt.Sprintf("SetBindGroup(%d, %v)", index, offsets))
}

func (p *captureRenderPass) SetVertexBuffer(slot uint32, _ hal.Buffer, offset uint64) {
	p.calls = append(p.calls, fmt.Sprintf("SetVertexBuffer(%d, %d)", slot, offset))
}

func (p *captureRenderPass) SetIndexBuffer(_ hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	p.calls = append(p.calls, fmt.Sprintf("SetIndexBuffer(%v, %d)", format, offset))
}

func (p *captureRenderPass) Draw(args gputypes.DrawArgs) {
	p.calls = append(p.calls, fmt.Sprintf("Draw(%d)", args.VertexCount))
}

func (p *captureRenderPass) DrawIndexed(args gputypes.DrawIndexedArgs) {
	p.calls = append(p.calls, fmt.Sprintf("DrawIndexed(%d)", args.IndexCount))
}

type captureCommandEncoder struct {
	*noop.CommandEncoder
	resolved        int
	querySet        hal.QuerySet
	buffer          hal.Buffer
	renderPass      *captureRenderPass
	renderPassDesc  *hal.RenderPassDescriptor
	computePassDesc *hal.ComputePassDescriptor
}

func (e *captureCommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	e.computePassDesc = desc
	return &noop.ComputePassEncoder{}
}

func (e *captureCommandEncoder) ResolveQuerySet(querySet hal.QuerySet, _, _ uint32, buffer hal.Buffer, _ uint64) {
	e.resolved++
	e.querySet = querySet
	e.buffer = buffer
}

func (e *captureCommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	e.renderPassDesc = desc
	if e.renderPass == nil {
		return nil
	}
	return e.renderPass
}

type captureDevice struct {
	hal.Device
	commandEncoder hal.CommandEncoder
}

func (d *captureDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return d.commandEncoder, nil
}

func newCaptureDevice(h *captureDevice) *Device {
	limits := DefaultLimits()
	features := gputypes.Features(gputypes.FeatureTimestampQuery)
	return &Device{core: core.NewDevice(h, nil, features, limits, "query-render-bundle-test")}
}

func newTestQuerySet(raw hal.QuerySet, device *Device) *QuerySet {
	return &QuerySet{impl: resource.NewQuerySet(
		core.NewQuerySet(raw, device.core, hal.QueryTypeTimestamp, 4, "test query set"),
		device.seamView(),
	)}
}

func TestBeginRenderPassTimestampWrites(t *testing.T) {
	begin, end := uint32(2), uint32(3)
	raw := &testHALResource{}
	h := &captureDevice{Device: &noop.Device{}}
	device := newCaptureDevice(h)
	active := newTestQuerySet(raw, device)
	released := newTestQuerySet(raw, device)
	released.Release()

	tests := []struct {
		name    string
		writes  *RenderPassTimestampWrites
		wantNil bool
		wantErr bool
	}{
		{name: "absent", writes: nil, wantNil: true},
		// A query set that is missing or released is a validation error, as
		// in Rust wgpu-core (validate_pass_timestamp_writes, check_is_valid).
		{name: "nil query set", writes: &RenderPassTimestampWrites{EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "released query set", writes: &RenderPassTimestampWrites{QuerySet: released, EndOfPassWriteIndex: &end}, wantErr: true},
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
			command := &captureCommandEncoder{
				CommandEncoder: &noop.CommandEncoder{},
				renderPass:     &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}},
			}
			h.commandEncoder = command
			encoder, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatal(err)
			}
			pass, err := encoder.BeginRenderPass(&RenderPassDescriptor{TimestampWrites: tt.writes})
			if tt.wantErr {
				if err == nil {
					pass.End()
					t.Fatal("BeginRenderPass succeeded, want a timestampWrites error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer pass.End()
			if command.renderPassDesc == nil {
				t.Fatal("HAL BeginRenderPass was not called")
			}
			got := command.renderPassDesc.TimestampWrites
			if tt.wantNil {
				if got != nil {
					t.Fatalf("TimestampWrites = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.QuerySet != raw || got.BeginningOfPassWriteIndex != &begin || got.EndOfPassWriteIndex != &end {
				t.Fatalf("TimestampWrites = %+v", got)
			}
		})
	}
}

// TestBeginComputePassTimestampWrites checks that compute pass timestampWrites
// reach the HAL pass unchanged and that invalid ones are rejected before the
// HAL pass begins, as Rust wgpu-core validate_pass_timestamp_writes does.
func TestBeginComputePassTimestampWrites(t *testing.T) {
	begin, end, outOfRange := uint32(0), uint32(3), uint32(4)
	raw := &testHALResource{}
	h := &captureDevice{Device: &noop.Device{}}
	device := newCaptureDevice(h)
	active := newTestQuerySet(raw, device)
	released := newTestQuerySet(raw, device)
	released.Release()
	occlusion := &QuerySet{impl: resource.NewQuerySet(
		core.NewQuerySet(raw, device.core, hal.QueryTypeOcclusion, 4, "occlusion"),
		device.seamView(),
	)}
	other := newCaptureDevice(&captureDevice{Device: &noop.Device{}})
	foreign := newTestQuerySet(raw, other)

	tests := []struct {
		name    string
		writes  *ComputePassTimestampWrites
		wantErr bool
	}{
		{name: "absent"},
		{name: "both indices", writes: &ComputePassTimestampWrites{QuerySet: active, BeginningOfPassWriteIndex: &begin, EndOfPassWriteIndex: &end}},
		{name: "end only", writes: &ComputePassTimestampWrites{QuerySet: active, EndOfPassWriteIndex: &end}},
		{name: "nil query set", writes: &ComputePassTimestampWrites{EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "released query set", writes: &ComputePassTimestampWrites{QuerySet: released, EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "occlusion query set", writes: &ComputePassTimestampWrites{QuerySet: occlusion, EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "query set of another device", writes: &ComputePassTimestampWrites{QuerySet: foreign, EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "index out of range", writes: &ComputePassTimestampWrites{QuerySet: active, EndOfPassWriteIndex: &outOfRange}, wantErr: true},
		{name: "equal indices", writes: &ComputePassTimestampWrites{QuerySet: active, BeginningOfPassWriteIndex: &end, EndOfPassWriteIndex: &end}, wantErr: true},
		{name: "no index", writes: &ComputePassTimestampWrites{QuerySet: active}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
			h.commandEncoder = command
			encoder, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatal(err)
			}
			pass, err := encoder.BeginComputePass(&ComputePassDescriptor{TimestampWrites: tt.writes})
			if tt.wantErr {
				if err == nil {
					pass.End()
					t.Fatal("BeginComputePass succeeded, want a timestampWrites error")
				}
				if command.computePassDesc != nil {
					t.Fatal("HAL BeginComputePass was called for invalid timestampWrites")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer pass.End()
			if command.computePassDesc == nil {
				t.Fatal("HAL BeginComputePass was not called")
			}
			got := command.computePassDesc.TimestampWrites
			if tt.writes == nil {
				if got != nil {
					t.Fatalf("TimestampWrites = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.QuerySet != raw ||
				got.BeginningOfPassWriteIndex != tt.writes.BeginningOfPassWriteIndex ||
				got.EndOfPassWriteIndex != tt.writes.EndOfPassWriteIndex {
				t.Fatalf("TimestampWrites = %+v", got)
			}
		})
	}
}

// TestBeginComputePassTimestampWritesNeedsFeature checks the feature gate:
// a device without FeatureTimestampQuery rejects timestampWrites.
func TestBeginComputePassTimestampWritesNeedsFeature(t *testing.T) {
	end := uint32(1)
	raw := &testHALResource{}
	h := &captureDevice{Device: &noop.Device{}, commandEncoder: &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}}
	device := &Device{core: core.NewDevice(h, nil, 0, DefaultLimits(), "no-timestamp-feature")}
	querySet := newTestQuerySet(raw, device)
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	pass, err := encoder.BeginComputePass(&ComputePassDescriptor{
		TimestampWrites: &ComputePassTimestampWrites{QuerySet: querySet, EndOfPassWriteIndex: &end},
	})
	if err == nil {
		pass.End()
		t.Fatal("BeginComputePass succeeded without FeatureTimestampQuery")
	}
	var featureErr *core.FeatureError
	if !errors.As(err, &featureErr) {
		t.Fatalf("error = %v, want a *core.FeatureError", err)
	}
}

// TestQueueGetTimestampPeriod checks that the queue reports the HAL period.
func TestQueueGetTimestampPeriod(t *testing.T) {
	if got := (&Queue{hal: &periodQueue{period: 83.333}}).GetTimestampPeriod(); got != 83.333 {
		t.Errorf("GetTimestampPeriod() = %v, want 83.333", got)
	}
	if got := (&Queue{}).GetTimestampPeriod(); got != 1 {
		t.Errorf("GetTimestampPeriod() without a HAL queue = %v, want 1", got)
	}
}

type periodQueue struct {
	noop.Queue
	period float32
}

func (q *periodQueue) GetTimestampPeriod() float32 { return q.period }

func TestResolveQuerySetDelegates(t *testing.T) {
	raw := &testHALResource{}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
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
	querySet := newTestQuerySet(raw, device)
	encoder.ResolveQuerySet(querySet, 0, 1, buffer, 0)
	if command.resolved != 1 || command.querySet != raw || command.buffer == nil {
		t.Fatalf("ResolveQuerySet delegation = (%d, %v, %v)", command.resolved, command.querySet, command.buffer)
	}
}

func TestResolveQuerySetGuards(t *testing.T) {
	raw := &testHALResource{}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}}
	h := &captureDevice{Device: &noop.Device{}, commandEncoder: command}
	device := newCaptureDevice(h)
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	querySet := newTestQuerySet(raw, device)
	releasedQuerySet := newTestQuerySet(raw, device)
	releasedQuerySet.Release()
	activeDestination := NewTestBufferFromCore(nil)
	releasedDestination := NewTestBufferFromCore(nil)
	releasedDestination.Release()

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
	raw := &testHALResource{}
	tests := []struct {
		name        string
		querySet    func(*Device) *QuerySet
		destination *Buffer
	}{
		{name: "nil query set", destination: NewTestBufferFromCore(nil)},
		{name: "nil destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(raw, d) }},
		{name: "released query set", querySet: func(d *Device) *QuerySet {
			q := newTestQuerySet(raw, d)
			q.Release()
			return q
		}, destination: NewTestBufferFromCore(nil)},
		{name: "released destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(raw, d) }, destination: func() *Buffer {
			b := NewTestBufferFromCore(nil)
			b.Release()
			return b
		}()},
		{name: "nil HAL query set", querySet: func(*Device) *QuerySet { return &QuerySet{} }, destination: NewTestBufferFromCore(nil)},
		{name: "nil HAL destination", querySet: func(d *Device) *QuerySet { return newTestQuerySet(raw, d) }, destination: NewTestBufferFromCore(nil)},
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

func TestRenderPassDescriptorTimestampWritesToHAL(t *testing.T) {
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}})
	raw := &testHALResource{}
	active := newTestQuerySet(raw, device)
	defer device.destroyQueue().FlushAll()
	defer active.Release()
	released := newTestQuerySet(raw, device)
	released.Release()
	begin, end := uint32(1), uint32(3)
	tests := []struct {
		name   string
		writes *RenderPassTimestampWrites
		want   bool
	}{
		{name: "absent"},
		{name: "nil query set", writes: &RenderPassTimestampWrites{}},
		{name: "released query set", writes: &RenderPassTimestampWrites{QuerySet: released}},
		{name: "both boundaries", writes: &RenderPassTimestampWrites{QuerySet: active, BeginningOfPassWriteIndex: &begin, EndOfPassWriteIndex: &end}, want: true},
		{name: "begin only", writes: &RenderPassTimestampWrites{QuerySet: active, BeginningOfPassWriteIndex: &begin}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&RenderPassDescriptor{Label: "pass", TimestampWrites: tt.writes}).toHAL()
			if got.Label != "pass" {
				t.Fatalf("Label = %q", got.Label)
			}
			if !tt.want {
				if got.TimestampWrites != nil {
					t.Fatalf("TimestampWrites = %+v, want nil", got.TimestampWrites)
				}
				return
			}
			writes := got.TimestampWrites
			if writes == nil || writes.QuerySet != raw || writes.BeginningOfPassWriteIndex != tt.writes.BeginningOfPassWriteIndex || writes.EndOfPassWriteIndex != tt.writes.EndOfPassWriteIndex {
				t.Fatalf("TimestampWrites = %+v, want %+v", writes, tt.writes)
			}
		})
	}
}
