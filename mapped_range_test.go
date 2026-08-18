//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

func TestMappedRangeBytesMutAndFlush(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mapped-write",
		Size:             16,
		Usage:            wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	rangeValue, err := buf.MappedRange(0, 16)
	if err != nil {
		t.Fatalf("MappedRange: %v", err)
	}
	data := rangeValue.BytesMut()
	if len(data) != 16 {
		t.Fatalf("BytesMut length = %d, want 16", len(data))
	}
	data[0] = 0x42
	if err := rangeValue.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := buf.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
}
