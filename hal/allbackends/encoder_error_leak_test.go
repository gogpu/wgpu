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
		{"pass error with resources, finished", func(t *testing.T, device *wgpu.Device) {
			enc := recordErroredPassWithResources(t, device)
			if _, err := enc.Finish(); err == nil {
				t.Fatal("Finish accepted a dispatch over the workgroup limit")
			}
		}},
		{"pass error with resources, abandoned", func(t *testing.T, device *wgpu.Device) {
			// The pass took references to the pipeline, the bind group and
			// its buffers; the encoder is then left unfinished. The device
			// must still destroy them when it is released.
			recordErroredPassWithResources(t, device)
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

// recordErroredPassWithResources records a compute pass that sets a pipeline
// and a bind group over two storage buffers, then fails a dispatch, and
// returns the encoder in its error state. The pipeline, layouts, group and
// buffers are released by the test's cleanups before the device.
func recordErroredPassWithResources(t *testing.T, device *wgpu.Device) *wgpu.CommandEncoder {
	t.Helper()
	storage := wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst
	input := newPublicBuffer(t, device, 256, storage)
	output := newPublicBuffer(t, device, 256, storage)
	bgl := newBindGroupLayout(t, device,
		gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
		gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: wgpu.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
	)
	pipeline := newComputePipeline(t, device, doubleWGSL, bgl)
	group := newBindGroup(t, device, bgl,
		wgpu.BindGroupEntry{Binding: 0, Buffer: input, Size: 256},
		wgpu.BindGroupEntry{Binding: 1, Buffer: output, Size: 256},
	)
	enc := newEncoder(t, device)
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Dispatch(1<<30, 1, 1) // over maxComputeWorkgroupsPerDimension
	if err := pass.End(); err == nil {
		t.Fatal("End accepted a dispatch over the workgroup limit")
	}
	return enc
}
