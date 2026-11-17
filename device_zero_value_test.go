//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The public zero-value and invalid-input contract of Device (ADR-070 S11 fix
// round, S11 review F1-F3). The internal table
// (internal/device/zero_value_native_test.go) pins the implementation's zero
// value; this file pins the receiver callers actually hold — a zero-value
// wgpu.Device — and the answers the declaration layer owes it. Each expected
// string is the pre-move answer at 1d1778e (root device_native.go:89, :136,
// :439, :480, :405, :1466, :1602), captured by probing the pre-move tree.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/core"
)

// mustNotPanic runs fn and reports a panic as a test failure, so one regressed
// entry point does not crash the whole binary before the others are checked.
func mustNotPanic(t *testing.T, name string, fn func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v (pre-move it returned an error)", name, r)
		}
	}()
	return fn()
}

// TestPublicZeroValueDeviceAnswers pins the answers a zero-value public Device
// gives on its create and wait entry points: the pre-move descriptor errors and
// the released sentinel, never a nil-receiver panic. Pre-move the same methods
// read a zero atomic.Bool and then the zero core device, exactly as the
// implementation's zero value still does (S11 review F2).
func TestPublicZeroValueDeviceAnswers(t *testing.T) {
	var d wgpu.Device

	tests := []struct {
		name     string
		run      func() error
		want     string
		released bool
	}{
		{"CreateBuffer(nil)", func() error { _, err := d.CreateBuffer(nil); return err }, "wgpu: buffer descriptor is nil", false},
		{"CreateTexture(nil)", func() error { _, err := d.CreateTexture(nil); return err }, "wgpu: texture descriptor is nil", false},
		{"CreateShaderModule(nil)", func() error { _, err := d.CreateShaderModule(nil); return err }, "wgpu: shader module descriptor is nil", false},
		{"CreateBindGroupLayout(nil)", func() error { _, err := d.CreateBindGroupLayout(nil); return err }, "wgpu: bind group layout descriptor is nil", false},
		{"CreateSampler(nil)", func() error { _, err := d.CreateSampler(nil); return err }, "wgpu: resource already released", true},
		{"CreateQuerySet(nil)", func() error { _, err := d.CreateQuerySet(nil); return err }, "wgpu: query set descriptor is nil", false},
		{"WaitIdle()", d.WaitIdle, "wgpu: resource already released", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mustNotPanic(t, tt.name, tt.run)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("%s error = %v, want %q", tt.name, err, tt.want)
			}
			if tt.released && !errors.Is(err, wgpu.ErrReleased) {
				t.Fatalf("%s error = %v, want wgpu.ErrReleased through errors.Is", tt.name, err)
			}
		})
	}
}

// TestRenderPipelineFragmentWithoutModule checks that a render pipeline whose
// descriptor has a fragment state with no module is rejected with the typed
// MissingFragmentModule creation error. The HAL descriptor carries no fragment
// stage in this shape (toHAL drops it), so the pre-check in the implementation
// is the only place the invalid descriptor is still visible; before the S11 fix
// round the call silently built a pipeline without a fragment stage.
func TestRenderPipelineFragmentWithoutModule(t *testing.T) {
	device := newSoftwareDevice(t)

	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "frag-nil-vertex",
		WGSL:  `@vertex fn vs() -> @builtin(position) vec4f { return vec4f(0.0); }`,
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer module.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "frag-nil-module",
		Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			// Module deliberately nil: WebGPU requires GPUFragmentState.module.
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm}},
		},
	})
	if pipeline != nil {
		pipeline.Release()
	}
	if err == nil {
		t.Fatal("CreateRenderPipeline succeeded with a fragment state without a module")
	}
	var rpe *core.CreateRenderPipelineError
	if !errors.As(err, &rpe) || rpe.Kind != core.CreateRenderPipelineErrorMissingFragmentModule {
		t.Fatalf("error %v (%T), want *core.CreateRenderPipelineError of kind MissingFragmentModule", err, err)
	}
	if rpe.Label != "frag-nil-module" {
		t.Errorf("error label = %q, want the descriptor's label", rpe.Label)
	}
}

// TestComputePipelineWithoutModule checks the compute path of the same class:
// a compute pipeline with no module is rejected by the existing compute
// validation, because its HAL descriptor keeps the (nil) module instead of
// dropping the stage. The case is pinned here so a later change cannot lose it.
func TestComputePipelineWithoutModule(t *testing.T) {
	device := newSoftwareDevice(t)

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "compute-no-module",
		EntryPoint: "cs",
	})
	if pipeline != nil {
		pipeline.Release()
	}
	if err == nil {
		t.Fatal("CreateComputePipeline succeeded without a module")
	}
	var cpe *core.CreateComputePipelineError
	if !errors.As(err, &cpe) || cpe.Kind != core.CreateComputePipelineErrorMissingModule {
		t.Fatalf("error %v (%T), want *core.CreateComputePipelineError of kind MissingModule", err, err)
	}
}

// TestCreateTextureViewZeroValueTexture checks that a non-nil public Texture
// with no implementation answers ErrReleased, as it did pre-move
// (texture.impl.ResolveHAL() is nil-safe there), rather than collapsing into
// the "texture is nil" answer for an absent handle (S11 review F3).
func TestCreateTextureViewZeroValueTexture(t *testing.T) {
	device := newSoftwareDevice(t)

	if _, err := device.CreateTextureView(&wgpu.Texture{}, nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("CreateTextureView(zero Texture) error = %v, want ErrReleased", err)
	}
	// An absent texture keeps its pre-move answer.
	if _, err := device.CreateTextureView(nil, nil); err == nil || err.Error() != "wgpu: texture is nil" {
		t.Fatalf("CreateTextureView(nil) error = %v, want %q", err, "wgpu: texture is nil")
	}
}

// TestCreateBindGroupZeroValueEntries pins the per-handle identity of the bind
// group entries at the boundary: a zero-value TextureView answered ErrReleased
// before the move, a zero-value Buffer panicked in the pre-move entry
// inspection, and a zero-value Sampler was accepted and bound nothing. This
// moves-only window keeps all three answers exactly; the normalisation to
// defined errors is carded as P2-CORE-034 (post-window), and the pinned panic
// below is window-temporary behavior identity, not a contract.
func TestCreateBindGroupZeroValueEntries(t *testing.T) {
	device := newSoftwareDevice(t)

	layoutEntry := func(kind string) gputypes.BindGroupLayoutEntry {
		switch kind {
		case "texture":
			return gputypes.BindGroupLayoutEntry{
				Binding: 0, Visibility: gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeFloat,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			}
		case "sampler":
			return gputypes.BindGroupLayoutEntry{
				Binding: 0, Visibility: gputypes.ShaderStageFragment,
				Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
			}
		default:
			return gputypes.BindGroupLayoutEntry{
				Binding: 0, Visibility: gputypes.ShaderStageVertex,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			}
		}
	}
	bindEntry := func(kind string) wgpu.BindGroupEntry {
		switch kind {
		case "texture":
			return wgpu.BindGroupEntry{Binding: 0, TextureView: &wgpu.TextureView{}}
		case "sampler":
			return wgpu.BindGroupEntry{Binding: 0, Sampler: &wgpu.Sampler{}}
		default:
			return wgpu.BindGroupEntry{Binding: 0, Buffer: &wgpu.Buffer{}, Size: 16}
		}
	}
	create := func(t *testing.T, kind string) (*wgpu.BindGroup, error) {
		t.Helper()
		layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
			Label:   kind,
			Entries: []gputypes.BindGroupLayoutEntry{layoutEntry(kind)},
		})
		if err != nil {
			t.Fatalf("CreateBindGroupLayout: %v", err)
		}
		t.Cleanup(layout.Release)
		return device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:   kind,
			Layout:  layout,
			Entries: []wgpu.BindGroupEntry{bindEntry(kind)},
		})
	}

	t.Run("zero TextureView answers ErrReleased", func(t *testing.T) {
		group, err := create(t, "texture")
		if group != nil {
			group.Release()
		}
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Fatalf("CreateBindGroup error = %v, want ErrReleased", err)
		}
	})

	t.Run("zero Buffer panics as before the move (P2-CORE-034 window-temporary)", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("a zero-value Buffer entry must panic exactly as it did before the move")
			}
		}()
		group, _ := create(t, "buffer")
		if group != nil {
			group.Release()
		}
	})

	t.Run("zero Sampler is accepted and binds nothing (P2-CORE-034 window-temporary)", func(t *testing.T) {
		group, err := create(t, "sampler")
		if err != nil {
			t.Fatalf("CreateBindGroup = %v, want the pre-move silent acceptance", err)
		}
		if group == nil {
			t.Fatal("CreateBindGroup returned no group")
		}
		group.Release()
	})
}
