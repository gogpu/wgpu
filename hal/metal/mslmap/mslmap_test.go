// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

// wgslRuntimeArray calls arrayLength() on a runtime-sized storage array and
// also reads a uniform buffer. The generated MSL therefore has both the sizes
// argument and a normal buffer binding.
const wgslRuntimeArray = `
struct Params {
	scale: u32,
}

@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@group(0) @binding(1) var<uniform> params: Params;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
	if (gid.x < arrayLength(&out)) {
		out[gid.x] = arrayLength(&out) * params.scale;
	}
}
`

// bufferParamRe finds each MSL parameter name and its buffer slot, for example
// "out [[buffer(0)]]".
var bufferParamRe = regexp.MustCompile(`(\w+)\s*\[\[buffer\((\d+)\)\]\]`)

func compile(t *testing.T, source string, opts msl.Options) (string, msl.TranslationInfo) {
	t.Helper()

	ast, err := naga.Parse(source)
	if err != nil {
		t.Fatalf("naga.Parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, source)
	if err != nil {
		t.Fatalf("naga.LowerWithSource: %v", err)
	}
	out, info, err := msl.Compile(module, opts)
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}
	return out, info
}

func bufferSlots(mslSource string) map[string]string {
	slots := make(map[string]string)
	for _, m := range bufferParamRe.FindAllStringSubmatch(mslSource, -1) {
		slots[m[1]] = m[2]
	}
	return slots
}

// TestDefaultOptionsLeaveSizesBufferUnbound records the bug that this package
// fixes. With msl.DefaultOptions, naga reports that the sizes buffer is
// required, but it writes the argument without a [[buffer(N)]] attribute.
func TestDefaultOptionsLeaveSizesBufferUnbound(t *testing.T) {
	t.Parallel()

	source, info := compile(t, wgslRuntimeArray, msl.DefaultOptions())

	if !info.RequiresSizesBuffer {
		t.Fatal("RequiresSizesBuffer = false; want true for a runtime-sized array")
	}
	if !strings.Contains(source, "_buffer_sizes") {
		t.Fatal("generated MSL has no _buffer_sizes argument")
	}
	if slot, ok := bufferSlots(source)["_buffer_sizes"]; ok {
		t.Fatalf("_buffer_sizes has [[buffer(%s)]] under DefaultOptions; want no attribute", slot)
	}
}

// TestOptionsBindSizesBuffer checks that the map from this package puts the
// sizes argument at the slot that the caller asked for.
func TestOptionsBindSizesBuffer(t *testing.T) {
	t.Parallel()

	ast, err := naga.Parse(wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.Parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.LowerWithSource: %v", err)
	}

	const sizesSlot = 2

	opts, err := mslmap.Options(module, sizesSlot)
	if err != nil {
		t.Fatalf("mslmap.Options: %v", err)
	}
	source, _, err := msl.Compile(module, opts)
	if err != nil {
		t.Fatalf("msl.Compile: %v", err)
	}

	got, ok := bufferSlots(source)["_buffer_sizes"]
	if !ok {
		t.Fatalf("_buffer_sizes has no [[buffer(N)]] attribute\nMSL:\n%s", source)
	}
	if got != "2" {
		t.Errorf("_buffer_sizes bound to buffer(%s); want buffer(%d)", got, sizesSlot)
	}

	bindings, err := mslmap.RuntimeArrayBindings(module)
	if err != nil {
		t.Fatalf("mslmap.RuntimeArrayBindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("RuntimeArrayBindings = %v; want exactly the storage array binding", bindings)
	}
	if bindings[0].Group != 0 || bindings[0].Binding != 0 {
		t.Errorf("RuntimeArrayBindings[0] = (%d,%d); want (0,0)", bindings[0].Group, bindings[0].Binding)
	}
}

// TestOptionsPreserveOrdinaryBindings is the most important check. naga uses
// PerEntryPointMap[ep].Resources as is and does not number the resources
// itself. An incomplete map would move every normal resource to a wrong slot,
// with no error. The normal bindings must stay the same as with
// msl.DefaultOptions.
func TestOptionsPreserveOrdinaryBindings(t *testing.T) {
	t.Parallel()

	ast, err := naga.Parse(wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.Parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.LowerWithSource: %v", err)
	}

	auto, _, err := msl.Compile(module, msl.DefaultOptions())
	if err != nil {
		t.Fatalf("msl.Compile (auto): %v", err)
	}
	opts, err := mslmap.Options(module, 2)
	if err != nil {
		t.Fatalf("mslmap.Options: %v", err)
	}
	mapped, _, err := msl.Compile(module, opts)
	if err != nil {
		t.Fatalf("msl.Compile (mapped): %v", err)
	}

	autoSlots := bufferSlots(auto)
	mappedSlots := bufferSlots(mapped)
	delete(mappedSlots, "_buffer_sizes")

	if len(autoSlots) == 0 {
		t.Fatalf("no [[buffer(N)]] parameters found in auto-numbered MSL:\n%s", auto)
	}
	for name, want := range autoSlots {
		got, ok := mappedSlots[name]
		if !ok {
			t.Errorf("parameter %q lost its [[buffer(N)]] attribute under the explicit map", name)
			continue
		}
		if got != want {
			t.Errorf("parameter %q bound to buffer(%s); want buffer(%s) as with auto-numbering", name, got, want)
		}
	}
	for name := range mappedSlots {
		if _, ok := autoSlots[name]; !ok {
			t.Errorf("parameter %q gained a [[buffer(N)]] attribute under the explicit map", name)
		}
	}
}

// TestOptionsRejectsOutOfRangeSlot checks the guard for a slot that does not
// fit in a uint8.
func TestOptionsRejectsOutOfRangeSlot(t *testing.T) {
	t.Parallel()

	ast, err := naga.Parse(wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.Parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, wgslRuntimeArray)
	if err != nil {
		t.Fatalf("naga.LowerWithSource: %v", err)
	}

	if _, err := mslmap.Options(module, 256); err == nil {
		t.Error("mslmap.Options(module, 256) = nil error; want an out-of-range error")
	}
}
