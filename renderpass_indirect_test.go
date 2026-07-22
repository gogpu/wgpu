//go:build !rust && !(js && wasm)

package wgpu

import "testing"

func TestIndexedIndirectRangeFits(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
		want       bool
	}{
		{name: "one record", bufferSize: 20, offset: 0, drawCount: 1, want: true},
		{name: "three records", bufferSize: 60, offset: 0, drawCount: 3, want: true},
		{name: "tail records", bufferSize: 80, offset: 20, drawCount: 3, want: true},
		{name: "one byte short", bufferSize: 59, offset: 0, drawCount: 3, want: false},
		{name: "offset past end", bufferSize: 20, offset: 21, drawCount: 1, want: false},
		{name: "offset overflow", bufferSize: ^uint64(0), offset: ^uint64(0) - 3, drawCount: 1, want: false},
		{name: "count overflow", bufferSize: ^uint64(0), offset: 0, drawCount: ^uint32(0), want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := indexedIndirectRangeFits(test.bufferSize, test.offset, test.drawCount); got != test.want {
				t.Fatalf("indexedIndirectRangeFits(%d, %d, %d) = %t, want %t", test.bufferSize, test.offset, test.drawCount, got, test.want)
			}
		})
	}
}

func BenchmarkIndexedIndirectRangeFitsCount1(b *testing.B) {
	benchmarkIndexedIndirectRangeFits(b, 1)
}

func BenchmarkIndexedIndirectRangeFitsCountN(b *testing.B) {
	benchmarkIndexedIndirectRangeFits(b, 1024)
}

func benchmarkIndexedIndirectRangeFits(b *testing.B, drawCount uint32) {
	b.ReportAllocs()
	var fits bool
	bufferSize := uint64(drawCount)*indexedIndirectRecordSize + 4
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fits = indexedIndirectRangeFits(bufferSize, 4, drawCount)
	}
	b.StopTimer()
	if !fits {
		b.Fatal("indexedIndirectRangeFits rejected a valid range")
	}
}
