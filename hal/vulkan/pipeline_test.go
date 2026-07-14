//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"strings"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestCreateRenderPipelineRejectsImplicitLayoutBeforeFFI(t *testing.T) {
	// A nil Layout would become VK_NULL_HANDLE in VkGraphicsPipelineCreateInfo.
	// The guard must run before touching the device, shader, or Vulkan command
	// table so this malformed request cannot reach a driver and crash it.
	_, err := (&Device{}).CreateRenderPipeline(&hal.RenderPipelineDescriptor{})
	if err == nil {
		t.Fatal("CreateRenderPipeline with nil layout returned nil error")
	}
	if want := "render pipeline layout is required"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err, want)
	}
}

func TestCreateComputePipelineRejectsImplicitLayoutBeforeFFI(t *testing.T) {
	_, err := (&Device{}).CreateComputePipeline(&hal.ComputePipelineDescriptor{})
	if err == nil {
		t.Fatal("CreateComputePipeline with nil layout returned nil error")
	}
	if want := "compute pipeline layout is required"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err, want)
	}
}
