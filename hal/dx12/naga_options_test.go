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
		{d3d12.D3D_SHADER_MODEL_6_6, backend.ShaderModel{Major: 6, Minor: 5}},
		{d3d12.D3D_SHADER_MODEL_6_7, backend.ShaderModel{Major: 6, Minor: 5}},
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
	if opts.ShaderModel != (backend.ShaderModel{Major: 6, Minor: 5}) || opts.BoundsCheckPolicies != want {
		t.Errorf("dxilOptions() = SM %v, bounds %+v", opts.ShaderModel, opts.BoundsCheckPolicies)
	}
}

// TestDXILShaderModelRefusal checks what a device whose highest shader model
// is below the shader's need gets from the DXIL path: naga's
// *backend.VersionError, which wraps backend.ErrUnsupported, and no raised
// model.
//
// A 64-bit storage buffer element is what needs a newer model: naga's DXIL
// writer reads and writes it natively with dx.op.rawBufferLoad/Store, whose
// i64/f64 overloads exist only from SM 6.3 (naga TestStorageBufferInt64RequiresSM63).
// A 32-bit element of more than four components, as in gg's tile and strip
// compute shaders, needed SM 6.2 until naga v0.21.8 (P1-DXIL-072); it now
// compiles at SM 6.0 with one dx.op.bufferLoad per four components, as DXC
// writes it.
func TestDXILShaderModelRefusal(t *testing.T) {
	lower := func(src string) *ir.Module {
		t.Helper()
		module, err := shaderir.Lower(src)
		if err != nil {
			t.Fatalf("Lower: %v", err)
		}
		return module.IR
	}
	compile := func(m *ir.Module, sm d3d12.D3D_SHADER_MODEL) error {
		opts := (&Device{shaderModel: sm}).dxilOptions()
		opts.EntryPoint = &backend.EntryPointSelector{Stage: ir.StageCompute, Name: "main"}
		_, _, err := dxil.Compile(m, opts)
		return err
	}
	every := []d3d12.D3D_SHADER_MODEL{d3d12.D3D_SHADER_MODEL_6_0, d3d12.D3D_SHADER_MODEL_6_1,
		d3d12.D3D_SHADER_MODEL_6_2, d3d12.D3D_SHADER_MODEL_6_3, d3d12.D3D_SHADER_MODEL_6_6}

	wide32 := lower(`struct S { a: vec4<f32>, b: f32 }
@group(0) @binding(0) var<storage, read_write> data: array<S>;
@compute @workgroup_size(1) fn main() { data[0].b = data[0].a.x; }`)
	for _, sm := range every {
		if err := compile(wide32, sm); err != nil {
			t.Errorf("32-bit element of five components, SM %#x: %v", uint32(sm), err)
		}
	}

	wide64 := lower(`@group(0) @binding(0) var<storage, read_write> s: array<u64>;
@compute @workgroup_size(1) fn main() { s[1] = s[0] + 1; }`)
	for _, sm := range every {
		err := compile(wide64, sm)
		if sm >= d3d12.D3D_SHADER_MODEL_6_3 {
			if err != nil {
				t.Errorf("64-bit element, SM %#x: %v", uint32(sm), err)
			}
			continue
		}
		var ve *backend.VersionError
		if !errors.As(err, &ve) || !errors.Is(err, backend.ErrUnsupported) {
			t.Fatalf("64-bit element, SM %#x: error %v (%T), want *backend.VersionError wrapping backend.ErrUnsupported", uint32(sm), err, err)
		}
		if want := dxilShaderModel(sm); ve.Requested.Major != want.Major || ve.Requested.Minor != want.Minor {
			t.Errorf("64-bit element, SM %#x: VersionError.Requested = %v, want the device's %v", uint32(sm), ve.Requested, want)
		}
	}
}

// TestDXILCacheSourceHoldsBindings checks that the DXIL shader cache key
// changes with every binding option and does not depend on map order: a
// module compiled for one pipeline layout must never hit an entry made for
// another (BUG-DX12-SHADER-LAYOUT-001).
func TestDXILCacheSourceHoldsBindings(t *testing.T) {
	// Fresh pointers on every call: the key must encode them by value.
	index := func(i uint32) *uint32 { return &i }
	base := func() dxil.Options {
		return dxil.Options{
			ShaderModel: backend.ShaderModel{Major: 6, Minor: 0},
			BindingMap: dxil.BindingMap{
				{Group: 0, Binding: 0}: {Register: 0},
				{Group: 1, Binding: 0}: {Register: 1},
				{Group: 1, Binding: 3}: {Register: 2},
				{Group: 2, Binding: 0}: {Register: 0, DynamicStorageBufferOffsetsIndex: index(0), DynamicStorageBufferSizeIndex: index(1)},
				{Group: 2, Binding: 1}: {Register: 6, RestrictIndexing: true},
			},
			SamplerBufferBindingMap:            map[uint32]dxil.BindTarget{0: {Register: 4}, 1: {Register: 5}},
			SamplerHeapTargets:                 &dxil.SamplerHeapBindTargets{ComparisonSamplers: dxil.BindTarget{Register: 2048}},
			SpecialConstantsBinding:            &dxil.BindTarget{Register: 3},
			DynamicStorageBufferOffsetsTargets: map[uint32]dxil.OffsetsBindTarget{2: {Register: 7, Size: 2}},
		}
	}
	want := base()
	key := dxilCacheSource("src", &want)
	for i := range 20 {
		again := base()
		if got := dxilCacheSource("src", &again); got != key {
			t.Fatalf("run %d: key changed for equal options:\n%q\n%q", i, got, key)
		}
	}

	changes := map[string]func(o *dxil.Options){
		"source": nil,
		"register": func(o *dxil.Options) {
			o.BindingMap[ir.ResourceBinding{Group: 1, Binding: 0}] = dxil.BindTarget{Register: 2}
		},
		"space": func(o *dxil.Options) {
			o.BindingMap[ir.ResourceBinding{Group: 1, Binding: 0}] = dxil.BindTarget{Space: 1, Register: 1}
		},
		"restrict": func(o *dxil.Options) {
			o.BindingMap[ir.ResourceBinding{Group: 2, Binding: 1}] = dxil.BindTarget{Register: 6}
		},
		"dynamic offset index": func(o *dxil.Options) {
			o.BindingMap[ir.ResourceBinding{Group: 2, Binding: 0}] = dxil.BindTarget{DynamicStorageBufferOffsetsIndex: index(1), DynamicStorageBufferSizeIndex: index(1)}
		},
		"dynamic size index": func(o *dxil.Options) {
			o.BindingMap[ir.ResourceBinding{Group: 2, Binding: 0}] = dxil.BindTarget{DynamicStorageBufferOffsetsIndex: index(0)}
		},
		"offsets target": func(o *dxil.Options) {
			o.DynamicStorageBufferOffsetsTargets[2] = dxil.OffsetsBindTarget{Register: 8, Size: 2}
		},
		"sampler buffer":   func(o *dxil.Options) { o.SamplerBufferBindingMap[1] = dxil.BindTarget{Register: 6} },
		"sampler heaps":    func(o *dxil.Options) { o.SamplerHeapTargets.ComparisonSamplers.Register = 1024 },
		"special constant": func(o *dxil.Options) { o.SpecialConstantsBinding.Register = 7 },
		"no special":       func(o *dxil.Options) { o.SpecialConstantsBinding = nil },
		"shader model":     func(o *dxil.Options) { o.ShaderModel.Minor = 6 },
		"bounds":           func(o *dxil.Options) { o.BoundsCheckPolicies.Buffer = backend.BoundsCheckUnchecked },
	}
	for name, change := range changes {
		o, source := base(), "src"
		if change == nil {
			source = "other"
		} else {
			change(&o)
		}
		if dxilCacheSource(source, &o) == key {
			t.Errorf("%s: cache key did not change", name)
		}
	}
}
