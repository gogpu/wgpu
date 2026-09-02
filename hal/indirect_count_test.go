//go:build !(js && wasm)

package hal

import "testing"

func TestRecordIndirectCountMax_ForwardsMaxDrawCount(t *testing.T) {
	t.Parallel()
	var (
		gotBuffer Buffer
		gotOffset uint64
		gotCount  uint32
		calls     int
	)
	record := func(buffer Buffer, offset uint64, drawCount uint32) {
		calls++
		gotBuffer = buffer
		gotOffset = offset
		gotCount = drawCount
	}

	stubBuffer := bufferHandle(1)
	stubCount := bufferHandle(2)
	RecordIndirectCountMax(record, stubBuffer, 16, stubCount, 8, 3)

	if calls != 1 {
		t.Fatalf("record calls = %d, want 1", calls)
	}
	if gotBuffer != stubBuffer || gotOffset != 16 || gotCount != 3 {
		t.Fatalf("record args = buffer %v offset %d count %d; want buffer %v offset 16 count 3",
			gotBuffer, gotOffset, gotCount, stubBuffer)
	}
}

func TestRecordIndirectCountMax_ZeroCountIsNoOp(t *testing.T) {
	t.Parallel()
	calls := 0
	record := func(Buffer, uint64, uint32) { calls++ }
	RecordIndirectCountMax(record, bufferHandle(1), 0, bufferHandle(2), 0, 0)
	if calls != 0 {
		t.Fatalf("record calls = %d, want 0", calls)
	}
}

func TestRecordIndirectCountMax_IgnoresCountBuffer(t *testing.T) {
	t.Parallel()
	calls := 0
	record := func(_ Buffer, _ uint64, drawCount uint32) {
		calls++
		if drawCount != 5 {
			t.Fatalf("drawCount = %d, want 5", drawCount)
		}
	}
	RecordIndirectCountMax(record, bufferHandle(1), 0, bufferHandle(99), 123, 5)
	if calls != 1 {
		t.Fatalf("record calls = %d, want 1", calls)
	}
}

type bufferHandle uintptr

func (bufferHandle) Destroy()           {}
func (h bufferHandle) NativeHandle() uintptr { return uintptr(h) }
