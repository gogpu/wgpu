//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// These tests pin the ADR-070 §15.5 zero-value contract per wrapped type
// (S2: texture, texture view, sampler, bind group, bind group layout, pipeline
// layout, shader module, fence, query set). The contract is per type and the
// pre-move truth wins: a type whose zero value was a no-op stays a no-op, and a
// type that panicked keeps panicking (a lenient wrapper would hide real
// misuse, a stricter one would be a behavior change in a refactor).
//
// Each type gets its own test so a regression names the type.

// requireZeroValuePanics asserts that f panics, the pre-move behavior of the
// types whose zero value dereferenced a nil device.
func requireZeroValuePanics(t *testing.T, name string, f func()) {
	t.Helper()
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		f()
	}()
	if !panicked {
		t.Errorf("zero-value %s did not panic; the pre-move type panicked", name)
	}
}

// TestTextureZeroValueIsSafe: a zero-value Texture released twice is a no-op
// and Format reports Undefined (pre-move: the nil device path returned early).
func TestTextureZeroValueIsSafe(t *testing.T) {
	var tex wgpu.Texture
	tex.Release()
	tex.Release()
	if got := tex.Format(); got != gputypes.TextureFormatUndefined {
		t.Fatalf("zero-value Format = %v, want Undefined", got)
	}
}

// TestTextureViewZeroValueIsSafe: a zero-value TextureView releases cleanly
// and has no parent texture.
func TestTextureViewZeroValueIsSafe(t *testing.T) {
	var view wgpu.TextureView
	view.Release()
	view.Release()
	if got := view.Texture(); got != nil {
		t.Fatalf("zero-value Texture() = %p, want nil", got)
	}
}

// TestBindGroupZeroValueIsSafe: a zero-value BindGroup releases cleanly, twice
// (pre-move: the nil released flag returned early).
func TestBindGroupZeroValueIsSafe(t *testing.T) {
	var group wgpu.BindGroup
	group.Release()
	group.Release()
}

// TestQuerySetZeroValueIsSafe: a zero-value QuerySet releases cleanly and
// reports the Occlusion default with a count of zero.
func TestQuerySetZeroValueIsSafe(t *testing.T) {
	var qs wgpu.QuerySet
	qs.Release()
	qs.Release()
	if got := qs.Type(); got != wgpu.QueryTypeOcclusion {
		t.Fatalf("zero-value Type = %v, want Occlusion", got)
	}
	if got := qs.Count(); got != 0 {
		t.Fatalf("zero-value Count = %d, want 0", got)
	}
}

// TestSamplerZeroValuePanics: releasing a zero-value Sampler panicked before
// the move (a nil device view) and must keep panicking.
func TestSamplerZeroValuePanics(t *testing.T) {
	var s wgpu.Sampler
	requireZeroValuePanics(t, "Sampler", s.Release)
}

// TestBindGroupLayoutZeroValuePanics: same contract for BindGroupLayout.
func TestBindGroupLayoutZeroValuePanics(t *testing.T) {
	var l wgpu.BindGroupLayout
	requireZeroValuePanics(t, "BindGroupLayout", l.Release)
}

// TestPipelineLayoutZeroValuePanics: same contract for PipelineLayout.
func TestPipelineLayoutZeroValuePanics(t *testing.T) {
	var l wgpu.PipelineLayout
	requireZeroValuePanics(t, "PipelineLayout", l.Release)
}

// TestShaderModuleZeroValuePanics: same contract for ShaderModule.
func TestShaderModuleZeroValuePanics(t *testing.T) {
	var m wgpu.ShaderModule
	requireZeroValuePanics(t, "ShaderModule", m.Release)
}

// TestFenceZeroValuePanics: same contract for Fence.
func TestFenceZeroValuePanics(t *testing.T) {
	var f wgpu.Fence
	requireZeroValuePanics(t, "Fence", f.Release)
}
