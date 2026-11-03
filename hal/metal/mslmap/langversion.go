// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package mslmap

import "github.com/gogpu/naga/msl"

// Platform is the Apple operating system a Metal device runs on. Go targets
// only macOS and iOS among Apple's systems.
type Platform int

const (
	// PlatformMacOS is macOS (GOOS darwin).
	PlatformMacOS Platform = iota
	// PlatformIOS is iOS and iPadOS (GOOS ios).
	PlatformIOS
)

// OSVersion is an operating system version, as
// NSProcessInfo.operatingSystemVersion reports it.
type OSVersion struct {
	Major, Minor, Patch int
}

// atLeast reports whether v is major.minor or newer.
func (v OSVersion) atLeast(major, minor int) bool {
	return v.Major > major || (v.Major == major && v.Minor >= minor)
}

// languageVersions lists, newest first, the first macOS and iOS versions
// whose Metal compiler accepts each MSL version.
var languageVersions = []struct {
	macOS, iOS [2]int
	version    msl.Version
}{
	{[2]int{26, 0}, [2]int{26, 0}, msl.Version{Major: 4, Minor: 0}},
	{[2]int{15, 0}, [2]int{18, 0}, msl.Version{Major: 3, Minor: 2}},
	{[2]int{14, 0}, [2]int{17, 0}, msl.Version{Major: 3, Minor: 1}},
	{[2]int{13, 0}, [2]int{16, 0}, msl.Version{Major: 3, Minor: 0}},
	{[2]int{12, 0}, [2]int{15, 0}, msl.Version{Major: 2, Minor: 4}},
	{[2]int{11, 0}, [2]int{14, 0}, msl.Version{Major: 2, Minor: 3}},
	{[2]int{10, 15}, [2]int{13, 0}, msl.Version{Major: 2, Minor: 2}},
	{[2]int{10, 14}, [2]int{12, 0}, msl.Version{Major: 2, Minor: 1}},
	{[2]int{10, 13}, [2]int{11, 0}, msl.Version{Major: 2, Minor: 0}},
	{[2]int{10, 12}, [2]int{10, 0}, msl.Version{Major: 1, Minor: 2}},
	{[2]int{10, 11}, [2]int{9, 0}, msl.Version{Major: 1, Minor: 1}},
}

// LanguageVersion returns the newest MSL version the Metal compiler of the
// given system accepts, as Rust wgpu-hal picks it
// (wgpu-hal/src/metal/adapter.rs, PrivateCapabilities::new, msl_version).
// Shaders are written for it (msl.Options.LangVersion) and compiled at it
// (MTLCompileOptions.languageVersion): naga refuses a feature that needs a
// newer version instead of raising it.
func LanguageVersion(platform Platform, os OSVersion) msl.Version {
	for _, row := range languageVersions {
		first := row.macOS
		if platform == PlatformIOS {
			first = row.iOS
		}
		if os.atLeast(first[0], first[1]) {
			return row.version
		}
	}
	return msl.Version{Major: 1, Minor: 0}
}

// PreserveInvariance reports whether the system has
// MTLCompileOptions.preserveInvariance (macOS 11, iOS 13), which Rust
// wgpu-hal sets wherever it exists (metal/device.rs load_shader) so that
// @invariant outputs are computed the same way in every pipeline.
func PreserveInvariance(platform Platform, os OSVersion) bool {
	if platform == PlatformIOS {
		return os.atLeast(13, 0)
	}
	return os.atLeast(11, 0)
}

// CompileLanguageVersion returns the MTLLanguageVersion value of v for
// MTLCompileOptions.languageVersion: the major version in the high 16 bits
// and the minor in the low ones (MTLLanguageVersion2_4 = 0x20004).
func CompileLanguageVersion(v msl.Version) uint64 {
	return uint64(v.Major)<<16 | uint64(v.Minor)
}
