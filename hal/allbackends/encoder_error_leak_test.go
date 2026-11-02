// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// An encoder that ends in an error state, is discarded or abandoned, or
// whose command buffer is never submitted, never reaches the queue. The
// device must still free its command buffers at shutdown: on Vulkan a
// command buffer alive at vkDestroyDevice is
// VUID-vkDestroyDevice-device-05137, which the validation layer reports when
// the test's device is released.

func TestErroredEncoderReleasesCommandBuffers(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, device *wgpu.Device)
	}{
		{"finish fails", func(t *testing.T, device *wgpu.Device) {
			buf := newPublicBuffer(t, device, 64, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
			enc := newEncoder(t, device)
			enc.CopyBufferToBuffer(buf, 0, buf, 32, 3) // size not a multiple of 4
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish accepted an invalid copy")
			}
		}},
		{"pass error", func(t *testing.T, device *wgpu.Device) {
			enc := newEncoder(t, device)
			pass, err := enc.BeginComputePass(nil)
			if err != nil {
				t.Fatalf("BeginComputePass: %v", err)
			}
			pass.Dispatch(1, 1, 1) // no pipeline
			_ = pass.End()
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish accepted a dispatch without a pipeline")
			}
		}},
		{"discard after pass", func(t *testing.T, device *wgpu.Device) {
			buf := newPublicBuffer(t, device, 64, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
			enc := newEncoder(t, device)
			pass, err := enc.BeginComputePass(nil)
			if err != nil {
				t.Fatalf("BeginComputePass: %v", err)
			}
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
			enc.CopyBufferToBuffer(buf, 0, buf, 32, 16)
			enc.DiscardEncoding()
		}},
		{"abandoned in error", func(t *testing.T, device *wgpu.Device) {
			// The encoder is left in its error state: never finished, never
			// discarded, as a caller that gives up on the first error does.
			buf := newPublicBuffer(t, device, 64, wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
			enc := newEncoder(t, device)
			enc.CopyBufferToBuffer(buf, 0, buf, 32, 3)
		}},
		{"finished, never submitted", func(t *testing.T, device *wgpu.Device) {
			enc := newEncoder(t, device)
			if _, err := enc.Finish(); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		}},
	}
	for _, b := range overrideBackends {
		if b.backend != gputypes.BackendVulkan || b.software {
			continue
		}
		for _, c := range cases {
			t.Run(b.name+"/"+c.name, func(t *testing.T) {
				device, _ := openBackendDevice(t, b, 0, gputypes.PowerPreferenceLowPower)
				c.run(t, device)
			})
		}
	}
}
