// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"errors"
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/dxil"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/internal/shaderir"
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

// TestDXILShaderModelRefusal checks what a device whose highest shader model
// is below the shader's need gets from the DXIL path: naga's
// *backend.VersionError, which wraps backend.ErrUnsupported, and no raised
// model. A storage buffer element of more than four components needs SM 6.2
// in naga's DXIL writer (P1-DXIL-072); gg's tile and strip compute shaders
// have such buffers.
func TestDXILShaderModelRefusal(t *testing.T) {
	const src = `struct S { a: vec4<f32>, b: f32 }
@group(0) @binding(0) var<storage, read_write> data: array<S>;
@compute @workgroup_size(1) fn main() { data[0].b = data[0].a.x; }`
	module, err := shaderir.Lower(src)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	compile := func(sm d3d12.D3D_SHADER_MODEL) error {
		opts := (&Device{shaderModel: sm}).dxilOptions()
		opts.EntryPoint = &backend.EntryPointSelector{Stage: ir.StageCompute, Name: "main"}
		_, _, err := dxil.Compile(module.IR, opts)
		return err
	}

	for _, sm := range []d3d12.D3D_SHADER_MODEL{d3d12.D3D_SHADER_MODEL_6_0, d3d12.D3D_SHADER_MODEL_6_1} {
		err := compile(sm)
		var ve *backend.VersionError
		if !errors.As(err, &ve) || !errors.Is(err, backend.ErrUnsupported) {
			t.Fatalf("SM %#x: error %v (%T), want *backend.VersionError wrapping backend.ErrUnsupported", uint32(sm), err, err)
		}
		if want := dxilShaderModel(sm); ve.Requested.Major != want.Major || ve.Requested.Minor != want.Minor {
			t.Errorf("SM %#x: VersionError.Requested = %v, want the device's %v", uint32(sm), ve.Requested, want)
		}
	}
	for _, sm := range []d3d12.D3D_SHADER_MODEL{d3d12.D3D_SHADER_MODEL_6_2, d3d12.D3D_SHADER_MODEL_6_6} {
		if err := compile(sm); err != nil {
			t.Errorf("SM %#x: %v", uint32(sm), err)
		}
	}
}
