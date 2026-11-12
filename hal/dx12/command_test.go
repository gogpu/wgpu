// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestClearBufferZeroesItsRange clears a range of a filled buffer on every
// adapter, with and without Storage usage, and reads the whole buffer back:
// the range is zero, and the bytes around it keep their value. The range
// spans three chunks of the zero buffer it is copied from.
func TestClearBufferZeroesItsRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}
	forEachDX12Adapter(t, func(t *testing.T, testDevice dx12TestDevice) {
		for name, usage := range map[string]gputypes.BufferUsage{
			"copy":    gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
			"storage": gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst | gputypes.BufferUsageStorage,
		} {
			t.Run(name, func(t *testing.T) { runClearBuffer(t, testDevice.device, testDevice.queue, usage) })
		}
	})
}

func runClearBuffer(t *testing.T, device hal.Device, queue hal.Queue, usage gputypes.BufferUsage) {
	t.Helper()
	const size = 2*zeroBufferSize + 1024
	const clearOffset = 256
	const clearSize = size - 2*clearOffset

	newBuffer := func(label string, bufferUsage gputypes.BufferUsage) hal.Buffer {
		buffer, err := device.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: bufferUsage})
		if err != nil {
			t.Fatalf("CreateBuffer(%s): %v", label, err)
		}
		t.Cleanup(func() { device.DestroyBuffer(buffer) })
		return buffer
	}
	cleared := newBuffer("cleared", usage)
	staging := newBuffer("staging", gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)

	want := bytes.Repeat([]byte{0xFF}, size)
	if err := queue.WriteBuffer(cleared, 0, want); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	clear(want[clearOffset : clearOffset+clearSize])

	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "clear-buffer"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := encoder.BeginEncoding("clear-buffer"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	encoder.ClearBuffer(cleared, clearOffset, clearSize)
	encoder.CopyBufferToBuffer(cleared, staging, []hal.BufferCopy{{Size: size}})
	commandBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{commandBuffer}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}

	mapping, err := device.MapBuffer(staging, 0, size)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	got := bytes.Clone(unsafe.Slice((*byte)(mapping.Ptr), size))
	if err := device.UnmapBuffer(staging); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}

	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("byte %d = %#x, want %#x (the clear covers %d to %d)", index, got[index], want[index], clearOffset, clearOffset+clearSize)
		}
	}
}
