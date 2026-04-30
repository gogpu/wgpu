//go:build !(js && wasm)

package wgpu_test

import (
	"strings"
	"testing"

	"github.com/gogpu/wgpu"
)

// =============================================================================
// VAL-A1: Queue.WriteBuffer bounds validation
// WebGPU spec §21.1, Rust wgpu-core queue.rs:647-672
// =============================================================================

// TestWriteBufferValid verifies the happy path: a 4-byte-aligned write
// within bounds to a CopyDst buffer succeeds.
func TestWriteBufferValid(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-valid",
		Size:  64,
		Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 16)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer should succeed: %v", err)
	}
}

// TestWriteBufferValidWithOffset verifies a write at a non-zero aligned
// offset within bounds succeeds.
func TestWriteBufferValidWithOffset(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-offset",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 8)
	if err := device.Queue().WriteBuffer(buf, 32, data); err != nil {
		t.Fatalf("WriteBuffer at offset 32 should succeed: %v", err)
	}
}

// TestWriteBufferValidExactFit verifies a write that fills the buffer
// exactly succeeds.
func TestWriteBufferValidExactFit(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-exact",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 64)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer exact fit should succeed: %v", err)
	}
}

// TestWriteBufferMissingCopyDst verifies that writing to a buffer
// without CopyDst usage returns an error.
func TestWriteBufferMissingCopyDst(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-no-copydst",
		Size:  64,
		Usage: wgpu.BufferUsageVertex, // no CopyDst
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 4)
	err = device.Queue().WriteBuffer(buf, 0, data)
	if err == nil {
		t.Fatal("WriteBuffer should fail: buffer missing CopyDst usage")
	}
	if !strings.Contains(err.Error(), "CopyDst") {
		t.Errorf("error should mention CopyDst, got: %v", err)
	}
}

// TestWriteBufferOffsetNotAligned verifies that a non-4-byte-aligned
// offset is rejected.
func TestWriteBufferOffsetNotAligned(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-offset-align",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 4)
	err = device.Queue().WriteBuffer(buf, 3, data)
	if err == nil {
		t.Fatal("WriteBuffer should fail: offset 3 not 4-byte aligned")
	}
	if !strings.Contains(err.Error(), "not 4-byte aligned") {
		t.Errorf("error should mention alignment, got: %v", err)
	}
}

// TestWriteBufferDataSizeNotAligned verifies that a data slice whose
// length is not a multiple of 4 is rejected.
func TestWriteBufferDataSizeNotAligned(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-data-align",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 5) // 5 is not 4-byte aligned
	err = device.Queue().WriteBuffer(buf, 0, data)
	if err == nil {
		t.Fatal("WriteBuffer should fail: data size 5 not 4-byte aligned")
	}
	if !strings.Contains(err.Error(), "data size") && !strings.Contains(err.Error(), "not 4-byte aligned") {
		t.Errorf("error should mention data size alignment, got: %v", err)
	}
}

// TestWriteBufferExceedsSize verifies that a write that overflows the
// buffer bounds is rejected.
func TestWriteBufferExceedsSize(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-overflow",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 32)
	err = device.Queue().WriteBuffer(buf, 48, data)
	if err == nil {
		t.Fatal("WriteBuffer should fail: offset 48 + size 32 > buffer size 64")
	}
	if !strings.Contains(err.Error(), "exceeds buffer size") {
		t.Errorf("error should mention exceeds buffer size, got: %v", err)
	}
}

// TestWriteBufferExceedsSizeExactBoundary verifies that writing exactly
// past the last byte is rejected (offset == buffer.Size()).
func TestWriteBufferExceedsSizeExactBoundary(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-boundary",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	data := make([]byte, 4)
	err = device.Queue().WriteBuffer(buf, 64, data) // offset at end
	if err == nil {
		t.Fatal("WriteBuffer should fail: offset 64 + size 4 > buffer size 64")
	}
	if !strings.Contains(err.Error(), "exceeds buffer size") {
		t.Errorf("error should mention exceeds buffer size, got: %v", err)
	}
}

// TestWriteBufferMappedAtCreation verifies that writing to a buffer
// that was created with MappedAtCreation=true is rejected.
func TestWriteBufferMappedAtCreation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "val-a1-mapped",
		Size:             64,
		Usage:            wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// Buffer is mapped at creation -- WriteBuffer must fail.
	data := make([]byte, 4)
	err = device.Queue().WriteBuffer(buf, 0, data)
	if err == nil {
		t.Fatal("WriteBuffer should fail: buffer is currently mapped")
	}
	if !strings.Contains(err.Error(), "mapped") {
		t.Errorf("error should mention mapped, got: %v", err)
	}
}

// TestWriteBufferNilBuffer verifies that passing a nil buffer returns
// an error (pre-existing check, not VAL-A1, but guards the new code path).
func TestWriteBufferNilBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	q := device.Queue()
	if q == nil {
		t.Skip("no queue")
	}

	data := make([]byte, 4)
	err := q.WriteBuffer(nil, 0, data)
	if err == nil {
		t.Fatal("WriteBuffer(nil buffer) should fail")
	}
}

// TestWriteBufferEmptyData verifies that writing an empty slice (len 0)
// succeeds. Zero-size is 4-byte aligned (0 % 4 == 0) and 0 + 0 <= size.
func TestWriteBufferEmptyData(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-empty",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// Empty write should pass all validation checks.
	if err := device.Queue().WriteBuffer(buf, 0, nil); err != nil {
		t.Fatalf("WriteBuffer with empty data should succeed: %v", err)
	}
}

// TestWriteBufferTableDriven exercises multiple offset/size/alignment
// combinations in a single table-driven test.
func TestWriteBufferTableDriven(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "val-a1-table",
		Size:  64,
		Usage: wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	tests := []struct {
		name    string
		offset  uint64
		size    int
		wantErr string // substring expected in error, "" = no error
	}{
		{"offset=0 size=4", 0, 4, ""},
		{"offset=4 size=4", 4, 4, ""},
		{"offset=60 size=4", 60, 4, ""}, // exactly fills last 4 bytes
		{"offset=0 size=64", 0, 64, ""}, // full buffer
		{"offset=0 size=0", 0, 0, ""},   // empty write
		{"offset=1 size=4", 1, 4, "not 4-byte aligned"},
		{"offset=2 size=4", 2, 4, "not 4-byte aligned"},
		{"offset=0 size=3", 0, 3, "not 4-byte aligned"},
		{"offset=0 size=1", 0, 1, "not 4-byte aligned"},
		{"offset=0 size=68", 0, 68, "exceeds buffer size"},
		{"offset=64 size=4", 64, 4, "exceeds buffer size"},
		{"offset=48 size=20", 48, 20, "exceeds buffer size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			err := device.Queue().WriteBuffer(buf, tt.offset, data)
			if tt.wantErr == "" { //nolint:nestif // table-driven test validation
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}
