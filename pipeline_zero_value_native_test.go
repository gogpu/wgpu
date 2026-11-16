//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"
)

// These tests pin the ADR-070 §15.5 zero-value contract of the pipeline
// wrappers (S3: RenderPipeline, ComputePipeline) to the pre-move truth: both
// zero values released as a no-op, a nil pointer panicked through a bare field
// dereference, and the render bundle rejected a zero-value pipeline as another
// device's (its nil device view failed the identity check) rather than
// reporting a nil pipeline or panicking. The file is white-box because the
// bundle's sentinel (errBundleDevice) is unexported; the per-type Release and
// panic checks are the pre-move behavior of the public types.

// requirePipelineZeroValuePanics asserts that f panics: both Release methods
// dereference the receiver through the wrapper, as the pre-move methods did.
func requirePipelineZeroValuePanics(t *testing.T, name string, f func()) {
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
		t.Errorf("nil %s did not panic; the pre-move method dereferenced the receiver", name)
	}
}

// TestRenderPipelineZeroValueReleaseIsNoop: a zero-value RenderPipeline has no
// implementation and releases cleanly, twice (pre-move: the nil released flag
// returned early and the nil ref dropped nothing).
func TestRenderPipelineZeroValueReleaseIsNoop(t *testing.T) {
	var p RenderPipeline
	p.Release()
	p.Release()
}

// TestComputePipelineZeroValueReleaseIsNoop: the compute pipeline zero value
// behaves the same way.
func TestComputePipelineZeroValueReleaseIsNoop(t *testing.T) {
	var p ComputePipeline
	p.Release()
	p.Release()
}

// TestRenderPipelineNilReleasePanics: a nil *RenderPipeline still panics.
func TestRenderPipelineNilReleasePanics(t *testing.T) {
	var p *RenderPipeline
	requirePipelineZeroValuePanics(t, "*RenderPipeline.Release", func() { p.Release() })
}

// TestComputePipelineNilReleasePanics: a nil *ComputePipeline still panics.
func TestComputePipelineNilReleasePanics(t *testing.T) {
	var p *ComputePipeline
	requirePipelineZeroValuePanics(t, "*ComputePipeline.Release", func() { p.Release() })
}

// TestRenderBundleRejectsZeroValuePipeline: a zero-value pipeline recorded in
// a render bundle has no device view, so Finish rejects it as another device's
// pipeline — the pre-move zero value's nil device failed the same identity
// check — and it is not a "pipeline is nil" command error.
func TestRenderBundleRejectsZeroValuePipeline(t *testing.T) {
	rig := newBundleTestRig()
	enc := rig.encoder(t, nil)
	enc.SetPipeline(&RenderPipeline{})
	bundle, err := enc.Finish(nil)
	if bundle != nil {
		t.Fatalf("Finish() = %v, want an error", bundle)
	}
	if !errors.Is(err, errBundleDevice) {
		t.Fatalf("Finish() error = %v, want %v", err, errBundleDevice)
	}
	if errors.Is(err, errBundleCommand) {
		t.Fatalf("Finish() error = %v, reported as a command error, want a device error", err)
	}
}
