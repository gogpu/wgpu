// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import "github.com/gogpu/gputypes"

// shaderCapabilityFeatures returns the shader capability features (the naga
// capabilities a device grants) that DX12 offers. dxil is true when devices
// compile shaders with naga's DXIL backend instead of HLSL and FXC.
//
// Only the HLSL/FXC path offers them for now: on the DXIL path the readback
// tests (hal/allbackends TestFeature*) fail, so the rows Rust wgpu-hal ties
// to shader model 6.x (SHADER_INT64, the 64-bit atomics, barycentrics,
// per-vertex inputs) are not offered either (P1-API-008). naga cards:
//   - i64 shifts and negation refused by the DXIL validator (invalid
//     record): naga P1-DXIL-080;
//   - a component of a run-time-indexed local array (the clip distance
//     test's vertex positions): naga P1-DXIL-081;
//   - clip_distances with N >= 2 stores only element 0: naga P1-DXIL-082;
//   - primitive_index interpolation mode (reads 0 for the second triangle
//     on Intel Iris Xe): naga P1-DXIL-083.
func shaderCapabilityFeatures(dxil bool) gputypes.Features {
	var result gputypes.Features
	if dxil {
		return result
	}
	// SV_PrimitiveID exists from shader model 4; Rust offers it always
	// (dx12/adapter.rs).
	result.Insert(gputypes.FeaturePrimitiveIndex)
	// Deviation from Rust, which does not offer CLIP_DISTANCES on DX12:
	// SV_ClipDistance exists from shader model 4, and Dawn enables the
	// feature on every D3D12 device (dawn/native/d3d12/PhysicalDeviceD3D12.cpp).
	result.Insert(gputypes.FeatureClipDistances)
	return result
}
