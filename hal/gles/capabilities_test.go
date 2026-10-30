// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestParseGLVersion follows Rust wgpu-hal gles/adapter.rs parse_full_version
// and parse_version: there is no fallback version; a string that parses
// neither way reports ok=false (no adapter). Before v0.34.6 "" and garbage
// were treated as GL 3.3, so a broken context passed as a usable one.
func TestParseGLVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantES    bool
		wantOK    bool
	}{
		// Desktop GL
		{"4.6.0 NVIDIA 536.23", 4, 6, false, true},
		{"3.3 Mesa 23.0.0", 3, 3, false, true},
		{"4.5", 4, 5, false, true},
		{"3.3.0 - Build 31.0.101.2125", 3, 3, false, true},
		{"1.1.0", 1, 1, false, true},
		{"2.1 Mesa 20", 2, 1, false, true},
		{"3.0 Mesa 23.0.4", 3, 0, false, true},
		{"3.2 (Core Profile) Mesa 23", 3, 2, false, true},
		{"4.1 ATI-4.8.54", 4, 1, false, true},
		{"4.10", 4, 1, false, true}, // Rust trims trailing zeros of the minor

		// OpenGL ES
		{"OpenGL ES 3.2 V@490.0", 3, 2, true, true},
		{"OpenGL ES 3.0 Mesa 23.0.4", 3, 0, true, true},
		{"OpenGL ES 3.1 Adreno (TM) 730", 3, 1, true, true},
		{"OpenGL ES 3.0", 3, 0, true, true},
		{"OpenGL ES 2.0", 2, 0, true, true},

		// Unparseable: no adapter
		{"OpenGL ES-CM 1.1", 0, 0, false, false},
		{"", 0, 0, false, false},
		{"garbage", 0, 0, false, false},
		{"3", 0, 0, false, false},
	}

	for _, tt := range tests {
		major, minor, isES, ok := parseGLVersion(tt.input)
		if major != tt.wantMajor || minor != tt.wantMinor || isES != tt.wantES || ok != tt.wantOK {
			t.Errorf("parseGLVersion(%q) = (%d, %d, %v, %v), want (%d, %d, %v, %v)",
				tt.input, major, minor, isES, ok, tt.wantMajor, tt.wantMinor, tt.wantES, tt.wantOK)
		}
	}
}

// TestCheckGLVersion: the adapter gate is OpenGL ES 3.0 or desktop OpenGL 3.3
// (Rust wgpu-hal gles/adapter.rs expose). The windows-latest CI runner exposed a
// context below it and crashed in glGenVertexArrays when a test opened it.
func TestCheckGLVersion(t *testing.T) {
	tests := []struct {
		version string
		wantErr bool
	}{
		{"1.1.0", true},
		{"2.1 Mesa 20", true},
		{"3.0 Mesa 23.0.4", true},
		{"3.2 (Core Profile) Mesa 23", true},
		{"3.3.0 - Build 31.0.101.2125", false},
		{"4.6.0 NVIDIA 536.23", false},
		{"OpenGL ES 2.0", true},
		{"OpenGL ES 3.0", false},
		{"OpenGL ES 3.2 V@490.0", false},
		{"OpenGL ES-CM 1.1", true},
		{"", true},
	}
	for _, tt := range tests {
		_, _, _, err := checkGLVersion(tt.version)
		if (err != nil) != tt.wantErr {
			t.Errorf("checkGLVersion(%q) error = %v, want error %v", tt.version, err, tt.wantErr)
		}
	}
}

func TestParseVersionNumbers(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
	}{
		{"4.6.0 NVIDIA", 4, 6},
		{"3.30", 3, 30},
		{"3.2 V@490", 3, 2},
		{"4.50", 4, 50},
		{"1.0", 1, 0},
		{"", 0, 0},
		{"abc", 0, 0},
		{".5", 0, 0}, // no major
		{"3", 0, 0},  // no dot
	}

	for _, tt := range tests {
		major, minor := parseVersionNumbers(tt.input)
		if major != tt.wantMajor || minor != tt.wantMinor {
			t.Errorf("parseVersionNumbers(%q) = (%d, %d), want (%d, %d)",
				tt.input, major, minor, tt.wantMajor, tt.wantMinor)
		}
	}
}

func TestParseGLSLVersion(t *testing.T) {
	tests := []struct {
		input string
		isES  bool
		want  int
	}{
		// Desktop GLSL
		{"4.60 NVIDIA via Cg compiler", false, 450}, // capped at 450
		{"4.50", false, 450},
		{"3.30", false, 330},
		{"4.30 - Build 31.0.101.2125", false, 430},

		// ES GLSL
		{"OpenGL ES GLSL ES 3.00", true, 300},
		{"OpenGL ES GLSL ES 3.20", true, 320},

		// Fallbacks
		{"", false, 330},
		{"", true, 300},
		{"garbage", false, 330},
		{"garbage", true, 300},
	}

	for _, tt := range tests {
		got := parseGLSLVersion(tt.input, tt.isES)
		if got != tt.want {
			t.Errorf("parseGLSLVersion(%q, %v) = %d, want %d",
				tt.input, tt.isES, got, tt.want)
		}
	}
}

func TestInferDeviceType(t *testing.T) {
	tests := []struct {
		vendor   string
		renderer string
		want     gputypes.DeviceType
	}{
		// CPU renderers
		{"Mesa", "llvmpipe (LLVM 15.0.7)", gputypes.DeviceTypeCPU},
		{"Google Inc.", "SwiftShader Device", gputypes.DeviceTypeCPU},
		{"Mesa", "Mesa Offscreen", gputypes.DeviceTypeCPU},

		// Integrated via vendor
		{"Intel", "Intel(R) Iris(R) Xe Graphics", gputypes.DeviceTypeIntegratedGPU},
		{"Qualcomm", "Adreno (TM) 730", gputypes.DeviceTypeIntegratedGPU},

		// Integrated via renderer string
		{"ARM", "Mali-G710", gputypes.DeviceTypeIntegratedGPU},
		{"NVIDIA Corporation", "NVIDIA Tegra X1", gputypes.DeviceTypeIntegratedGPU},
		{"Apple", "Apple M1 Pro", gputypes.DeviceTypeIntegratedGPU},

		// Default (likely discrete but we report Other)
		{"NVIDIA Corporation", "NVIDIA GeForce RTX 4090", gputypes.DeviceTypeOther},
		{"ATI Technologies Inc.", "AMD Radeon RX 7900 XT", gputypes.DeviceTypeOther},
	}

	for _, tt := range tests {
		got := inferDeviceType(tt.vendor, tt.renderer)
		if got != tt.want {
			t.Errorf("inferDeviceType(%q, %q) = %v, want %v",
				tt.vendor, tt.renderer, got, tt.want)
		}
	}
}

func TestInferVendorID(t *testing.T) {
	tests := []struct {
		vendor string
		want   uint32
	}{
		{"NVIDIA Corporation", vendorIDNVIDIA},
		{"Intel Open Source Technology Center", vendorIDIntel},
		{"ATI Technologies Inc.", vendorIDAMD},
		{"AMD", vendorIDAMD},
		{"Qualcomm", vendorIDQualcomm},
		{"ARM", vendorIDARM},
		{"Broadcom", vendorIDBroadcom},
		{"Apple", vendorIDApple},
		{"Mesa", vendorIDMesa},
		{"ImgTec", vendorIDImgTec},
		{"Unknown Vendor", 0},
	}

	for _, tt := range tests {
		got := inferVendorID(tt.vendor)
		if got != tt.want {
			t.Errorf("inferVendorID(%q) = 0x%04X, want 0x%04X",
				tt.vendor, got, tt.want)
		}
	}
}

func TestHasExtension(t *testing.T) {
	exts := map[string]bool{
		"GL_EXT_texture_compression_s3tc": true,
		"GL_ARB_depth_clamp":              true,
		"GL_EXT_color_buffer_float":       true,
	}

	if !hasExtension(exts, "GL_EXT_texture_compression_s3tc") {
		t.Error("expected GL_EXT_texture_compression_s3tc to be present")
	}
	if !hasExtension(exts, "GL_MISSING", "GL_ARB_depth_clamp") {
		t.Error("expected GL_ARB_depth_clamp to match via multi-name check")
	}
	if hasExtension(exts, "GL_MISSING", "GL_ALSO_MISSING") {
		t.Error("expected no match for missing extensions")
	}
	if hasExtension(exts) {
		t.Error("expected no match for empty name list")
	}
}

func TestGlVersionAtLeast(t *testing.T) {
	tests := []struct {
		major, minor int
		isES         bool
		reqES        [2]int
		reqFull      [2]int
		want         bool
	}{
		// ES 3.1 >= ES 3.1
		{3, 1, true, [2]int{3, 1}, [2]int{4, 3}, true},
		// ES 3.0 < ES 3.1
		{3, 0, true, [2]int{3, 1}, [2]int{4, 3}, false},
		// GL 4.3 >= Full 4.3
		{4, 3, false, [2]int{3, 1}, [2]int{4, 3}, true},
		// GL 4.6 >= Full 4.3
		{4, 6, false, [2]int{3, 1}, [2]int{4, 3}, true},
		// GL 3.3 < Full 4.3
		{3, 3, false, [2]int{3, 1}, [2]int{4, 3}, false},
	}

	for _, tt := range tests {
		got := glVersionAtLeast(tt.major, tt.minor, tt.isES, tt.reqES, tt.reqFull)
		if got != tt.want {
			t.Errorf("glVersionAtLeast(%d.%d, es=%v, reqES=%v, reqFull=%v) = %v, want %v",
				tt.major, tt.minor, tt.isES, tt.reqES, tt.reqFull, got, tt.want)
		}
	}
}
