// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// TestShaderModuleLoweredOnce checks on every backend that a WGSL module is
// parsed and validated once, in core, and that the backend compiles the IR
// core hands it in hal.ShaderModuleDescriptor.IR instead of lowering the
// source again (P1-CORE-013), with and without overrides.
func TestShaderModuleLoweredOnce(t *testing.T) {
	sources := []string{
		`@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1) fn main() { out[0] = 1u; }`,
		`override k: u32 = 3u;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1) fn main() { out[0] = k; }`,
	}
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for i, source := range sources {
			before := shaderir.Lowerings()
			module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: source})
			if err != nil {
				t.Fatalf("source %d: CreateShaderModule: %v", i, err)
			}
			lowerings := shaderir.Lowerings() - before
			module.Release()
			if lowerings != 1 {
				t.Errorf("source %d: lowered %d times, want 1", i, lowerings)
			}
		}
	})
}
