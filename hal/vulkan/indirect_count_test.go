//go:build !(js && wasm)

package vulkan

import "testing"

func TestIndexedIndirectCallPlanCapturesCountAndStride(t *testing.T) {
	tests := []struct {
		name       string
		supports   bool
		max        uint32
		drawCount  uint32
		wantOffset uint64
		wantCount  uint32
		wantBatch  bool
	}{
		{name: "native multi draw", supports: true, max: 8, drawCount: 3, wantOffset: 4, wantCount: 3, wantBatch: true},
		{name: "single draw fallback", supports: false, max: 8, drawCount: 3, wantOffset: 4, wantCount: 1, wantBatch: false},
		{name: "count one", supports: false, max: 0, drawCount: 1, wantOffset: 4, wantCount: 1, wantBatch: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call, batched, ok := indexedIndirectCallPlan(test.supports, test.max, test.wantOffset, test.drawCount)
			if !ok {
				t.Fatal("indexedIndirectCallPlan rejected a valid range")
			}
			if batched != test.wantBatch {
				t.Fatalf("batched = %t, want %t", batched, test.wantBatch)
			}
			if call.offset != test.wantOffset || call.count != test.wantCount || call.stride != indexedIndirectStride {
				t.Fatalf("captured call = offset %d, count %d, stride %d; want offset %d, count %d, stride %d", call.offset, call.count, call.stride, test.wantOffset, test.wantCount, indexedIndirectStride)
			}
		})
	}
}

func TestIndirectCallPlanUsesFixedRecordStrideForBothDrawKinds(t *testing.T) {
	for _, test := range []struct {
		name   string
		stride uint32
	}{
		{name: "draw", stride: drawIndirectStride},
		{name: "indexed", stride: drawIndexedIndirectStride},
	} {
		t.Run(test.name, func(t *testing.T) {
			call, batched, ok := indirectCallPlan(true, 8, 4, 3, test.stride)
			if !ok || !batched {
				t.Fatalf("indirectCallPlan = %#v, batched=%t, ok=%t", call, batched, ok)
			}
			if call.offset != 4 || call.count != 3 || call.stride != test.stride {
				t.Fatalf("call = %#v, want offset=4 count=3 stride=%d", call, test.stride)
			}
		})
	}
}

func TestIndexedIndirectRecordOffset(t *testing.T) {
	if got, ok := indexedIndirectRecordOffset(4, 3); !ok || got != 64 {
		t.Fatalf("indexedIndirectRecordOffset(4, 3) = %d, %t; want 64, true", got, ok)
	}
	if _, ok := indexedIndirectRecordOffset(^uint64(0)-3, 1); ok {
		t.Fatal("indexedIndirectRecordOffset should reject uint64 overflow")
	}
}

func TestIndexedIndirectHelpersAllocateZero(t *testing.T) {
	var (
		offset uint64
		ok     bool
	)
	if allocs := testing.AllocsPerRun(1000, func() {
		offset, ok = indexedIndirectRecordOffset(4, 3)
	}); allocs != 0 {
		t.Fatalf("indexedIndirectRecordOffset allocations = %v, want 0", allocs)
	}
	if offset != 64 || !ok {
		t.Fatalf("indexedIndirectRecordOffset result = %d, %t; want 64, true", offset, ok)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _, ok = indexedIndirectCallPlan(true, 8, 4, 3)
	}); allocs != 0 {
		t.Fatalf("indexedIndirectCallPlan allocations = %v, want 0", allocs)
	}
}

func BenchmarkIndexedIndirectCallPlanCount1(b *testing.B) {
	benchmarkIndexedIndirectCallPlan(b, 1)
}

func BenchmarkIndexedIndirectCallPlanCountN(b *testing.B) {
	benchmarkIndexedIndirectCallPlan(b, 1024)
}

func benchmarkIndexedIndirectCallPlan(b *testing.B, drawCount uint32) {
	b.ReportAllocs()
	var call indexedIndirectCall
	var ok bool
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		call, _, ok = indexedIndirectCallPlan(true, ^uint32(0), 4, drawCount)
	}
	b.StopTimer()
	if !ok || call.count != drawCount || call.stride != indexedIndirectStride {
		b.Fatalf("indexedIndirectCallPlan result = %#v, %t", call, ok)
	}
}
