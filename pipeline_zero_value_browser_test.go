//go:build js && wasm

package wgpu

import "testing"

// TestBrowserPipelineZeroValueGetBindGroupLayout pins the browser zero-value
// contract of the one context with GetBindGroupLayout (S3, ADR-070 §15.5): a
// zero-value pipeline has no browser object and returns nil, as the pre-move
// body's nil check did.
func TestBrowserPipelineZeroValueGetBindGroupLayout(t *testing.T) {
	var rp RenderPipeline
	if got := rp.GetBindGroupLayout(0); got != nil {
		t.Errorf("zero-value RenderPipeline.GetBindGroupLayout = %v, want nil", got)
	}
	var cp ComputePipeline
	if got := cp.GetBindGroupLayout(0); got != nil {
		t.Errorf("zero-value ComputePipeline.GetBindGroupLayout = %v, want nil", got)
	}
}
