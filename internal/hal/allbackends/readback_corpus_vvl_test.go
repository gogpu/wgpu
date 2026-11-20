// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// vvlNegativeWGSL is valid WGSL whose compute pipeline cannot run as the
// case describes it: binding 0 is declared as a uniform buffer, while the
// case's binding 0 is a storage buffer, so the pipeline layout contradicts
// the shader. The Khronos validation layer reports that as an error when the
// pipeline is created (VUID-VkComputePipelineCreateInfo-layout-07988/07990).
// Nothing is accessed out of bounds — the read takes the first 4 bytes of a
// 16-byte binding — so a driver without the layer runs the case harmlessly and
// it passes: exactly the vacuous pass this test and WGPU_REQUIRE=vvl exist to
// catch.
const vvlNegativeWGSL = `
@group(0) @binding(0) var<uniform> src: vec4<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

@compute @workgroup_size(1)
fn main() {
    dst[0] = src.x;
}
`

// vvlNegativeCase is vvlNegativeWGSL as a corpus case whose expectations are
// unchecked, so the only thing that can fail it is a validation message.
func vvlNegativeCase() *corpusCase {
	return &corpusCase{
		name:       "negative/vvl-descriptor-type-mismatch",
		wgsl:       vvlNegativeWGSL,
		desc:       "a storage buffer bound where the shader declares a uniform buffer",
		entry:      "main",
		workgroups: [3]uint32{1, 1, 1},
		bindings: []corpusBinding{
			{binding: 0, kind: gputypes.BufferBindingTypeStorage, words: []uint32{0xDEADBEEF, 0, 0, 0}},
			{binding: 1, kind: gputypes.BufferBindingTypeStorage, words: []uint32{0, 0}},
		},
	}
}

// TestReadbackCorpusVVLNegative proves the validation capture is not vacuous:
// a case a Vulkan validation layer must reject fails through the same
// executeCorpusCase path TestReadbackCorpus uses, with the layer's own message
// (a VUID) in the failure detail. The same case passes without the layer, so a
// runner that claims Vulkan validation coverage has to make a missing layer a
// failure — WGPU_REQUIRE=vulkan,vvl does that here and in forEachHALAdapter.
//
// The test skips where there is no Vulkan adapter or no layer to prove
// anything with.
func TestReadbackCorpusVVLNegative(t *testing.T) {
	cfg := halBackendConfigFor(backendVulkan)
	backend, ok := hal.GetBackend(cfg.variant)
	if !ok {
		requireOrSkip(t, cfg, string(cfg.name)+" backend not registered on this platform")
	}
	flags, vvl := startVulkanValidation(t)
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Backends: cfg.mask, Flags: flags})
	if err != nil {
		requireOrSkip(t, cfg, "CreateInstance: "+err.Error())
	}
	defer instance.Destroy()
	if !vvl.layerActive() {
		reason := "the Vulkan validation layer is not loaded on this instance"
		if requireSwitch("vvl") {
			t.Fatalf("%s, but WGPU_REQUIRE=%q requires vvl", reason, os.Getenv("WGPU_REQUIRE"))
		}
		t.Skip(reason)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		requireOrSkip(t, cfg, "no adapters")
	}
	var a *hal.ExposedAdapter
	for i := range adapters {
		if adapters[i].Capabilities.DownlevelCapabilities.Flags&gputypes.DownlevelFlagsComputeShaders != 0 {
			a = &adapters[i]
			break
		}
	}
	if a == nil {
		requireOrSkip(t, cfg, "no adapter with compute shaders")
	}
	open, err := a.Adapter.Open(0, a.Capabilities.Limits)
	if err != nil {
		t.Fatalf("Open %q: %v", a.Info.Name, err)
	}
	defer func() {
		_ = open.Device.WaitIdle()
		open.Device.Destroy()
	}()
	for _, m := range vvl.drain() {
		t.Errorf("Vulkan validation message while opening the device: %s", m)
	}

	c := vvlNegativeCase()
	tag, detail, failed := executeCorpusCase(&halRunner{device: open.Device, queue: open.Queue}, vvl, c, nil)
	if !failed {
		t.Fatalf("%q passed (%s) on %q: a validation error did not fail its case, the capture is vacuous",
			c.name, tag, a.Info.Name)
	}
	if !strings.Contains(detail, "Vulkan validation: VUID-") {
		t.Fatalf("%q failed for the wrong reason on %q: %s; want the validation layer's VUID",
			c.name, a.Info.Name, detail)
	}
	t.Logf("RESULT %-40s %-9s %-36s %s %s", c.name, backendVulkan, a.Info.Name, tag, detail)
}
