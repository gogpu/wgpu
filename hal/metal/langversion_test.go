// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/naga/msl"
)

// TestOperatingSystemVersion checks the 24-byte struct return of
// NSProcessInfo.operatingSystemVersion, which takes objc_msgSend_stret on
// amd64 and the x8 result register on arm64: a wrong ABI path returns zeros
// or garbage instead of a macOS 10.x or later version.
func TestOperatingSystemVersion(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	v := operatingSystemVersion()
	if v.Major < 10 || v.Major > 100 || v.Minor < 0 || v.Minor > 100 || v.Patch < 0 || v.Patch > 100 {
		t.Fatalf("operatingSystemVersion() = %+v, want a plausible OS version", v)
	}
	t.Logf("OS %d.%d.%d, MSL %v", v.Major, v.Minor, v.Patch, systemShaderCompileOptions().mslVersion)
}

// TestNewLibraryWithSourceAtDeviceVersion compiles MSL with the device's
// compile options: Metal must accept the language version it was given.
func TestNewLibraryWithSourceAtDeviceVersion(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	raw := CreateSystemDefaultDevice()
	if raw == 0 {
		t.Skip("no Metal device available")
	}
	defer Release(raw)

	opts := systemShaderCompileOptions()
	if opts.mslVersion.Less(msl.Version2_1) {
		t.Fatalf("MSL %v on a system that runs these tests (macOS 10.14+), want 2.1 or later", opts.mslVersion)
	}
	d := &Device{raw: raw, adapter: &Adapter{raw: raw, compileOptions: opts}}
	library, nsError := d.newLibraryWithSource(`#include <metal_stdlib>
using namespace metal;
kernel void fill(device uint* out [[buffer(0)]], uint id [[thread_position_in_grid]]) {
    out[id] = id;
}
`)
	if library == 0 {
		t.Fatalf("newLibraryWithSource at MSL %v: %s", opts.mslVersion, formatNSError(nsError))
	}
	Release(library)
}
