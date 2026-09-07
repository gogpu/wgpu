// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && !rust && !(js && wasm)

package wgpu_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register the Vulkan HAL backend. Without a real backend registered only
	// the noop/software backends are available, and neither encodes a Vulkan
	// render pass.
	//
	// This import is why the file sits behind the integration tag: registering
	// a real backend changes adapter selection for every test in the package.
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

const (
	depthOnlyWidth  = 256
	depthOnlyHeight = 256
	// depthOnlyBytesPerTexel is the size of one Depth32Float texel.
	depthOnlyBytesPerTexel = 4
	// depthOnlyClearValue is deliberately neither 0 nor 1: an untouched depth
	// texture would plausibly read back as either, and then a pass that never
	// ran would look like a pass that did.
	depthOnlyClearValue = 0.25
	// depthOnlyEpsilon allows for nothing but float32 representation. The
	// value is cleared, not computed, so it comes back exactly.
	depthOnlyEpsilon = 1e-6
)

// TestDepthOnlyRenderPassExecutes renders a pass that has a depth attachment
// and no color attachment, then reads the depth back to confirm the pass ran.
//
// A depth-only pass is legal in WebGPU and is how a shadow map is drawn. The
// Vulkan HAL used to return a render pass encoder without calling
// vkCmdBeginRenderPass whenever the descriptor named no color attachments,
// because the render area and sample count were derived from the color
// attachments alone. End then called vkCmdEndRenderPass regardless, which is
// undefined behavior: the driver faults rather than reporting a validation
// error, killing the process several frames of stack away from anything that
// names a pass.
//
// Reaching the end of this test at all covers the fault. The depth readback
// covers the other half -- that the pass was actually begun and executed,
// rather than merely not crashing.
func TestDepthOnlyRenderPassExecutes(t *testing.T) {
	device := newVulkanDevice(t)

	// bytesPerRow must be a multiple of 256 per the copy alignment rules.
	bytesPerRow := alignUp(depthOnlyWidth*depthOnlyBytesPerTexel, 256)
	bufferSize := uint64(bytesPerRow) * uint64(depthOnlyHeight)

	depth, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "depth-only-target",
		Size: wgpu.Extent3D{
			Width:              depthOnlyWidth,
			Height:             depthOnlyHeight,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatDepth32Float,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(depth.Release)

	depthView, err := device.CreateTextureView(depth, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(depthView.Release)

	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "depth-readback",
		Size:  bufferSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(staging.Release)

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "depth-only"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label:            "depth-only-pass",
		ColorAttachments: nil,
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View:            depthView,
			DepthLoadOp:     gputypes.LoadOpClear,
			DepthStoreOp:    gputypes.StoreOpStore,
			DepthClearValue: depthOnlyClearValue,
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass with no color attachment: %v", err)
	}

	// The fault was here.
	pass.End()

	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	// The copy needs its own encoder: the depth texture is a render attachment
	// for the pass and a copy source afterwards, and the resource tracker will
	// not carry both usages through one submission.
	readback, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "depth-readback"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder for readback: %v", err)
	}
	readback.CopyTextureToBuffer(depth, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{
			BytesPerRow:  bytesPerRow,
			RowsPerImage: depthOnlyHeight,
		},
		TextureBase: wgpu.ImageCopyTexture{
			Texture: depth,
			Aspect:  gputypes.TextureAspectDepthOnly,
		},
		Size: wgpu.Extent3D{
			Width:              depthOnlyWidth,
			Height:             depthOnlyHeight,
			DepthOrArrayLayers: 1,
		},
	}})
	readbackCmd, err := readback.Finish()
	if err != nil {
		t.Fatalf("Finish readback: %v", err)
	}

	if _, err := device.Queue().Submit(cmd, readbackCmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	assertDepthCleared(t, staging, bufferSize, bytesPerRow)
}

// assertDepthCleared checks every texel holds the value the pass cleared depth
// to. A pass that was never begun leaves the texture untouched, so this is
// what separates "did not crash" from "actually ran".
func assertDepthCleared(t *testing.T, staging *wgpu.Buffer, bufferSize uint64, bytesPerRow uint32) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := staging.Map(ctx, wgpu.MapModeRead, 0, bufferSize); err != nil {
		t.Fatalf("Map staging buffer: %v", err)
	}
	rng, err := staging.MappedRange(0, bufferSize)
	if err != nil {
		_ = staging.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	pixels := make([]byte, bufferSize)
	copy(pixels, rng.Bytes())
	if err := staging.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}

	wrong := 0
	var firstWrong float32
	for y := range depthOnlyHeight {
		for x := range depthOnlyWidth {
			off := uint32(y)*bytesPerRow + uint32(x)*depthOnlyBytesPerTexel
			d := math.Float32frombits(
				uint32(pixels[off]) |
					uint32(pixels[off+1])<<8 |
					uint32(pixels[off+2])<<16 |
					uint32(pixels[off+3])<<24,
			)
			if math.Abs(float64(d-depthOnlyClearValue)) > depthOnlyEpsilon {
				if wrong == 0 {
					firstWrong = d
				}
				wrong++
			}
		}
	}

	if wrong > 0 {
		t.Fatalf("depth = %v at %d of %d texels, want %v: the depth-only pass did not execute",
			firstWrong, wrong, depthOnlyWidth*depthOnlyHeight, float32(depthOnlyClearValue))
	}
}

// newVulkanDevice returns a device on the Vulkan backend, skipping when this
// machine cannot provide one. The bug under test is in the Vulkan HAL, so
// another backend's adapter would make the test pass without proving anything.
func newVulkanDevice(t *testing.T) *wgpu.Device {
	t.Helper()

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsVulkan})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	t.Cleanup(adapter.Release)

	if got := adapter.Info().Backend; got != gputypes.BackendVulkan {
		t.Skipf("adapter backend is %v, want Vulkan", got)
	}
	t.Logf("adapter: %s (%v)", adapter.Info().Name, adapter.Info().Backend)

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "depth-only-pass-test"})
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)

	return device
}

func alignUp(n, a uint32) uint32 {
	return (n + a - 1) / a * a
}
