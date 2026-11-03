// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap_test

import (
	"testing"

	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

// TestLanguageVersion checks the OS to MSL version table of Rust wgpu-hal
// (metal/adapter.rs, msl_version) at every boundary on macOS and iOS.
func TestLanguageVersion(t *testing.T) {
	v := func(major, minor uint8) msl.Version { return msl.Version{Major: major, Minor: minor} }
	tests := []struct {
		platform mslmap.Platform
		os       mslmap.OSVersion
		want     msl.Version
	}{
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 26, Minor: 1}, v(4, 0)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 26}, v(4, 0)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 15, Minor: 7, Patch: 3}, v(3, 2)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 15}, v(3, 2)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 14, Minor: 6}, v(3, 1)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 13}, v(3, 0)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 12, Minor: 7}, v(2, 4)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 11}, v(2, 3)},
		// macOS 11+ reports 10.16 to binaries in compatibility mode.
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 16}, v(2, 2)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 15}, v(2, 2)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 14, Patch: 6}, v(2, 1)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 13}, v(2, 0)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 12}, v(1, 2)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 11}, v(1, 1)},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 10}, v(1, 0)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 26}, v(4, 0)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 18, Minor: 2}, v(3, 2)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 17}, v(3, 1)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 16}, v(3, 0)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 15}, v(2, 4)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 14}, v(2, 3)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 13}, v(2, 2)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 12}, v(2, 1)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 11}, v(2, 0)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 10}, v(1, 2)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 9}, v(1, 1)},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 8}, v(1, 0)},
	}
	for _, tt := range tests {
		if got := mslmap.LanguageVersion(tt.platform, tt.os); got != tt.want {
			t.Errorf("LanguageVersion(%d, %+v) = %v, want %v", tt.platform, tt.os, got, tt.want)
		}
	}
}

// TestPreserveInvariance checks the availability gate of Rust wgpu-hal
// (metal/device.rs: macOS 11, iOS 13).
func TestPreserveInvariance(t *testing.T) {
	tests := []struct {
		platform mslmap.Platform
		os       mslmap.OSVersion
		want     bool
	}{
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 11}, true},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 15, Minor: 2}, true},
		{mslmap.PlatformMacOS, mslmap.OSVersion{Major: 10, Minor: 15}, false},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 13}, true},
		{mslmap.PlatformIOS, mslmap.OSVersion{Major: 12, Minor: 4}, false},
	}
	for _, tt := range tests {
		if got := mslmap.PreserveInvariance(tt.platform, tt.os); got != tt.want {
			t.Errorf("PreserveInvariance(%d, %+v) = %v, want %v", tt.platform, tt.os, got, tt.want)
		}
	}
}

// TestCompileLanguageVersion checks MTLLanguageVersion values from Apple's
// MTLLibrary.h.
func TestCompileLanguageVersion(t *testing.T) {
	tests := []struct {
		v    msl.Version
		want uint64
	}{
		{msl.Version{Major: 1, Minor: 1}, 0x10001},
		{msl.Version{Major: 2, Minor: 0}, 0x20000},
		{msl.Version{Major: 2, Minor: 4}, 0x20004},
		{msl.Version{Major: 3, Minor: 2}, 0x30002},
		{msl.Version{Major: 4, Minor: 0}, 0x40000},
	}
	for _, tt := range tests {
		if got := mslmap.CompileLanguageVersion(tt.v); got != tt.want {
			t.Errorf("CompileLanguageVersion(%v) = %#x, want %#x", tt.v, got, tt.want)
		}
	}
}
