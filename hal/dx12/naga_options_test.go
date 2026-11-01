// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestDXILShaderModel checks that the DXIL path compiles for the adapter's
// highest shader model, capped at naga's: naga refuses a shader that needs a
// newer model, and never raises the one it is given.
func TestDXILShaderModel(t *testing.T) {
	for _, c := range []struct {
		sm   d3d12.D3D_SHADER_MODEL
		want backend.ShaderModel
	}{
		{d3d12.D3D_SHADER_MODEL_5_1, backend.ShaderModel{Major: 6, Minor: 0}},
		{d3d12.D3D_SHADER_MODEL_6_0, backend.ShaderModel{Major: 6, Minor: 0}},
		{d3d12.D3D_SHADER_MODEL_6_5, backend.ShaderModel{Major: 6, Minor: 5}},
		{d3d12.D3D_SHADER_MODEL_6_7, backend.ShaderModel{Major: 6, Minor: 7}},
		{d3d12.D3D_SHADER_MODEL(0x6A), dxilShaderModelMax},
		{d3d12.D3D_SHADER_MODEL(0x70), dxilShaderModelMax},
	} {
		if got := dxilShaderModel(c.sm); got != c.want {
			t.Errorf("dxilShaderModel(%#x) = %v, want %v", uint32(c.sm), got, c.want)
		}
	}

	opts := (&Device{shaderModel: d3d12.D3D_SHADER_MODEL_6_6}).dxilOptions()
	want := backend.BoundsCheckPolicies{
		Index:        backend.BoundsCheckRestrict,
		Buffer:       backend.BoundsCheckRestrict,
		ImageLoad:    backend.BoundsCheckRestrict,
		BindingArray: backend.BoundsCheckUnchecked,
	}
	if opts.ShaderModel != (backend.ShaderModel{Major: 6, Minor: 6}) || opts.BoundsCheckPolicies != want {
		t.Errorf("dxilOptions() = SM %v, bounds %+v", opts.ShaderModel, opts.BoundsCheckPolicies)
	}
}
