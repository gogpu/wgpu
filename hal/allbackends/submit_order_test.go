// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// submitOrderSize is the byte size of the buffers the submit-order tests
// copy: a multiple of the 4-byte copy alignment.
const submitOrderSize = 256

// TestUnsubmittedCommandBufferHasNoEffect records a copy into a buffer,
// finishes the command buffer and releases it without submitting it: the
// buffer must keep the contents written before (WebGPU: commands run when
// their command buffer is submitted, never when they are recorded).
func TestUnsubmittedCommandBufferHasNoEffect(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		before := bytes.Repeat([]byte{0x11}, submitOrderSize)
		src := newSubmitOrderBuffer(t, device, bytes.Repeat([]byte{0xAB}, submitOrderSize))
		dst := newSubmitOrderBuffer(t, device, before)

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.CopyBufferToBuffer(src, 0, dst, 0, submitOrderSize)
		cb, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		cb.Release()

		if got := readSubmitOrderBuffer(t, device, dst); !bytes.Equal(got, before) {
			t.Fatalf("dst = %x..., want the unchanged %x...: an unsubmitted copy ran", got[:8], before[:8])
		}
	})
}

// TestWriteBufferBeforeSubmitIsOverwritten records a copy into a buffer,
// then writes the buffer through the queue, then submits the copy: the
// write runs first on the queue timeline, so the copy overwrites it.
func TestWriteBufferBeforeSubmitIsOverwritten(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		copied := bytes.Repeat([]byte{0xAB}, submitOrderSize)
		src := newSubmitOrderBuffer(t, device, copied)
		dst := newSubmitOrderBuffer(t, device, make([]byte, submitOrderSize))

		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		enc.CopyBufferToBuffer(src, 0, dst, 0, submitOrderSize)
		if err := device.Queue().WriteBuffer(dst, 0, bytes.Repeat([]byte{0x22}, submitOrderSize)); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		cb, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if _, err := device.Queue().Submit(cb); err != nil {
			t.Fatalf("Submit: %v", err)
		}

		if got := readSubmitOrderBuffer(t, device, dst); !bytes.Equal(got, copied) {
			t.Fatalf("dst = %x..., want the copied %x...: the copy ran before the write", got[:8], copied[:8])
		}
	})
}

// newSubmitOrderBuffer returns a copy source and destination buffer of
// len(data) bytes, written with data through the queue.
func newSubmitOrderBuffer(t *testing.T, device *wgpu.Device, data []byte) *wgpu.Buffer {
	t.Helper()
	b := newPublicBuffer(t, device, uint64(len(data)), gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(b, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return b
}

// readSubmitOrderBuffer copies buf to a staging buffer in a submitted
// command buffer and returns its bytes.
func readSubmitOrderBuffer(t *testing.T, device *wgpu.Device, buf *wgpu.Buffer) []byte {
	t.Helper()
	staging := newPublicBuffer(t, device, submitOrderSize, gputypes.BufferUsageMapRead|gputypes.BufferUsageCopyDst)
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToBuffer(buf, 0, staging, 0, submitOrderSize)
	})
	return mapStaging(t, staging, submitOrderSize)
}
