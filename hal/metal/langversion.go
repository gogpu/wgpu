// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"runtime"
	"unsafe"

	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu/hal/metal/mslmap"
)

// nsOperatingSystemVersionType describes NSOperatingSystemVersion =
// { NSInteger, NSInteger, NSInteger }: the same layout as MTLSize.
var nsOperatingSystemVersionType = mtlSizeType

// shaderCompileOptions is how this system's Metal compiler is driven: the
// MSL version shaders are written and compiled for, and whether
// preserveInvariance exists.
type shaderCompileOptions struct {
	mslVersion         msl.Version
	preserveInvariance bool
}

// systemShaderCompileOptions reads the OS version and picks the compile
// options from it, as Rust wgpu-hal does (metal/adapter.rs msl_version,
// metal/device.rs load_shader).
func systemShaderCompileOptions() shaderCompileOptions {
	platform := mslmap.PlatformMacOS
	if runtime.GOOS == "ios" {
		platform = mslmap.PlatformIOS
	}
	os := operatingSystemVersion()
	return shaderCompileOptions{
		mslVersion:         mslmap.LanguageVersion(platform, os),
		preserveInvariance: mslmap.PreserveInvariance(platform, os),
	}
}

// operatingSystemVersion returns NSProcessInfo.processInfo
// .operatingSystemVersion. The 24-byte struct comes back through
// objc_msgSend_stret on amd64 and through the x8 result pointer on arm64
// (objcMsgSendSymbol).
func operatingSystemVersion() mslmap.OSVersion {
	info := MsgSend(ID(GetClass("NSProcessInfo")), Sel("processInfo"))
	var v [3]int64
	if err := msgSend(info, Sel("operatingSystemVersion"), nsOperatingSystemVersionType, unsafe.Pointer(&v)); err != nil {
		return mslmap.OSVersion{}
	}
	return mslmap.OSVersion{Major: int(v[0]), Minor: int(v[1]), Patch: int(v[2])}
}

// newLibraryWithSource compiles MSL source into an MTLLibrary with the
// adapter's compile options: MTLCompileOptions.languageVersion at the MSL
// version the source was written for, and preserveInvariance where it
// exists. It returns the library, or 0 and the NSError (autoreleased).
func (d *Device) newLibraryWithSource(source string) (library, nsError ID) {
	str := NSString(source)
	defer Release(str)

	compile := d.adapter.compileOptions
	options := MsgSend(MsgSend(ID(GetClass("MTLCompileOptions")), Sel("alloc")), Sel("init"))
	if options != 0 {
		defer Release(options)
		msgSendVoid(options, Sel("setLanguageVersion:"), argUint64(mslmap.CompileLanguageVersion(compile.mslVersion)))
		if compile.preserveInvariance {
			msgSendVoid(options, Sel("setPreserveInvariance:"), argBool(true))
		}
	}

	// errorPtr travels as a uintptr, which keeps nothing in place: pin it so
	// Metal writes the NSError into errorPtr, not into a moved stack
	// (P1-VK-012).
	var errorPtr ID
	var pinner runtime.Pinner
	pinner.Pin(&errorPtr)
	library = MsgSend(d.raw, Sel("newLibraryWithSource:options:error:"),
		uintptr(str), uintptr(options), uintptr(unsafe.Pointer(&errorPtr)))
	pinner.Unpin()
	return library, errorPtr
}
