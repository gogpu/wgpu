//go:build !(js && wasm)

package core

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

type countedRenderPassEncoder struct {
	mockRenderPassEncoder
	drawCalls  int
	drawOffset uint64
	drawCount  uint32
	calls      int
	offset     uint64
	count      uint32
}

func (p *countedRenderPassEncoder) DrawIndirect(_ hal.Buffer, offset uint64, count uint32) {
	p.drawCalls++
	p.drawOffset = offset
	p.drawCount = count
}

func TestCoreRenderPassEncoderDrawIndirectForwardsCountOnce(t *testing.T) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	buffer := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageIndirect, 64, "indirect")
	recorder := &countedRenderPassEncoder{}
	pass := &CoreRenderPassEncoder{raw: recorder, device: device}

	pass.DrawIndirect(buffer, 4)
	if recorder.drawCalls != 1 || recorder.drawOffset != 4 || recorder.drawCount != 1 {
		t.Fatalf("single forwarding = calls %d offset %d count %d; want 1, 4, 1", recorder.drawCalls, recorder.drawOffset, recorder.drawCount)
	}
	pass.MultiDrawIndirect(buffer, 4, 3)
	if recorder.drawCalls != 2 || recorder.drawCount != 3 {
		t.Fatalf("multi forwarding = calls %d count %d; want 2, 3", recorder.drawCalls, recorder.drawCount)
	}
	pass.MultiDrawIndirect(nil, ^uint64(0), 0)
	if recorder.drawCalls != 2 {
		t.Fatalf("zero-count forwarding = calls %d, want 2", recorder.drawCalls)
	}
}

func (p *countedRenderPassEncoder) DrawIndexedIndirect(_ hal.Buffer, offset uint64, count uint32) {
	p.calls++
	p.offset = offset
	p.count = count
}

func TestCoreRenderPassEncoderDrawIndexedIndirectForwardsCountOnce(t *testing.T) {
	for _, drawCount := range []uint32{1, 3} {
		t.Run(fmt.Sprintf("count-%d", drawCount), func(t *testing.T) {
			device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
			buffer := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageIndirect, 64, "indirect")
			recorder := &countedRenderPassEncoder{}
			pass := &CoreRenderPassEncoder{raw: recorder, device: device}

			if drawCount == 1 {
				pass.DrawIndexedIndirect(buffer, 4)
			} else {
				pass.MultiDrawIndexedIndirect(buffer, 4, drawCount)
			}
			if recorder.calls != 1 {
				t.Fatalf("HAL DrawIndexedIndirect calls = %d, want 1", recorder.calls)
			}
			if recorder.offset != 4 || recorder.count != drawCount {
				t.Fatalf("HAL arguments = offset %d, count %d; want 4, %d", recorder.offset, recorder.count, drawCount)
			}
		})
	}
}

func TestCoreRenderPassEncoderDrawIndexedIndirectZeroCountIsNoOp(t *testing.T) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	recorder := &countedRenderPassEncoder{}
	pass := &CoreRenderPassEncoder{raw: recorder, device: device}

	pass.MultiDrawIndexedIndirect(nil, ^uint64(0), 0)
	if recorder.calls != 0 {
		t.Fatalf("HAL DrawIndexedIndirect calls = %d, want 0", recorder.calls)
	}
}

func BenchmarkCoreRenderPassEncoderDrawIndexedIndirectCount1(b *testing.B) {
	benchmarkCoreRenderPassEncoderDrawIndexedIndirect(b, 1)
}

func BenchmarkCoreRenderPassEncoderDrawIndexedIndirectCountN(b *testing.B) {
	benchmarkCoreRenderPassEncoderDrawIndexedIndirect(b, 1024)
}

func benchmarkCoreRenderPassEncoderDrawIndexedIndirect(b *testing.B, drawCount uint32) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	bufferSize := uint64(drawCount)*20 + 4
	buffer := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageIndirect, bufferSize, "indirect")
	recorder := &countedRenderPassEncoder{}
	pass := &CoreRenderPassEncoder{raw: recorder, device: device}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pass.MultiDrawIndexedIndirect(buffer, 4, drawCount)
	}
	b.StopTimer()
	if recorder.calls != b.N {
		b.Fatalf("HAL DrawIndexedIndirect calls = %d, want %d", recorder.calls, b.N)
	}
}
