//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// computeCall is one state or dispatch call on a HAL compute pass.
type computeCall struct {
	op       string // "pipeline", "group", "dispatchIndirect"
	pipeline hal.ComputePipeline
	index    uint32
}

// recordingComputePass records the calls made on one HAL compute pass.
type recordingComputePass struct {
	*noop.ComputePassEncoder
	label string
	calls []computeCall
}

func (p *recordingComputePass) SetPipeline(pipeline hal.ComputePipeline) {
	p.calls = append(p.calls, computeCall{op: "pipeline", pipeline: pipeline})
}

func (p *recordingComputePass) SetBindGroup(index uint32, _ hal.BindGroup, _ []uint32) {
	p.calls = append(p.calls, computeCall{op: "group", index: index})
}

func (p *recordingComputePass) DispatchIndirect(hal.Buffer, uint64) {
	p.calls = append(p.calls, computeCall{op: "dispatchIndirect"})
}

// passRecordingEncoder hands out a recordingComputePass per BeginComputePass.
type passRecordingEncoder struct {
	*noop.CommandEncoder
	passes []*recordingComputePass
}

func (e *passRecordingEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	p := &recordingComputePass{ComputePassEncoder: &noop.ComputePassEncoder{}}
	if desc != nil {
		p.label = desc.Label
	}
	e.passes = append(e.passes, p)
	return p
}

// TestDispatchIndirectValidatedKeepsUserState checks the passes a validated
// DispatchIndirect records (Rust wgpu-core compute.rs dispatch_indirect with
// indirect validation): the validation pipeline and its groups go only to a
// pass of their own, never to the user's pass, and the pass the user's
// dispatch runs in gets the user's pipeline and bind groups first.
func TestDispatchIndirectValidatedKeepsUserState(t *testing.T) {
	command := &passRecordingEncoder{CommandEncoder: &noop.CommandEncoder{}}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
	iv := device.core.IndirectValidation()
	if iv == nil {
		t.Fatal("the noop device has no indirect validation; the test needs it")
	}
	indirectCore := core.NewBuffer(&noop.Buffer{}, device.core, BufferUsageIndirect|BufferUsageStorage, 64, "indirect")
	indirectCore.Ref = core.NewResourceRef("Buffer:indirect", nil)
	indirect := &Buffer{core: indirectCore}
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass() error = %v", err)
	}

	l := rebindLayout(false)
	userPipeline := &ComputePipeline{hal: &testHALResource{}, bindGroupLayouts: []*BindGroupLayout{l}}
	pass.SetPipeline(userPipeline)
	pass.SetBindGroup(0, rebindGroup(l), nil)
	pass.DispatchIndirect(indirect, 0)
	if err := pass.End(); err != nil {
		t.Fatalf("End() error = %v", err)
	}

	if len(command.passes) != 3 {
		t.Fatalf("HAL compute passes = %d, want 3 (user, validation, user dispatch)", len(command.passes))
	}
	for _, c := range command.passes[0].calls {
		if c.op == "pipeline" && c.pipeline == iv.Pipeline() {
			t.Error("the validation pipeline was bound on the user's pass")
		}
	}
	if got := command.passes[1].calls; len(got) == 0 || got[0].pipeline != iv.Pipeline() {
		t.Errorf("validation pass calls = %+v, want the validation pipeline first", got)
	}
	want := []computeCall{
		{op: "pipeline", pipeline: userPipeline.hal},
		{op: "group", index: 0},
		{op: "dispatchIndirect"},
	}
	if got := command.passes[2].calls; len(got) != len(want) {
		t.Fatalf("user dispatch pass calls = %+v, want %+v", got, want)
	}
	for i, c := range command.passes[2].calls {
		if c != want[i] {
			t.Errorf("user dispatch pass call %d = %+v, want %+v", i, c, want[i])
		}
	}
}
