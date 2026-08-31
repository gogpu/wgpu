//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"

	_ "github.com/gogpu/wgpu/hal/noop"
)

// =============================================================================
// ADR-056: Unified Resource Lifecycle tests
//
// These tests verify that Release() goes through ResourceRef.Drop() for all
// tracked resources (BindGroup, RenderPipeline, ComputePipeline), so that
// in-flight GPU references prevent premature HAL destruction.
// =============================================================================

// TestBindGroup_ReleaseUsesRefDrop verifies that BindGroup.Release() decrements
// the refcount via Drop() instead of directly calling dq.Defer(). When a
// SetBindGroup Clone'd the ref, Release() should NOT destroy the HAL resource
// immediately — the refcount stays at 1 (from the Clone) and the onZero callback
// fires only when Triage drops the tracked ref after GPU completion.
func TestBindGroup_ReleaseUsesRefDrop(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "lifecycle-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "lifecycle-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	ref := bg.TestRef()
	if ref == nil {
		t.Fatal("BindGroup should have a non-nil ResourceRef")
	}

	// Initial refcount should be 1 (application owner).
	if got := ref.RefCount(); got != 1 {
		t.Fatalf("initial refcount: want 1, got %d", got)
	}

	// Simulate SetBindGroup → trackRef → Clone.
	ref.Clone()
	if got := ref.RefCount(); got != 2 {
		t.Fatalf("after Clone: want 2, got %d", got)
	}

	// Record DestroyQueue state before Release.
	pendingBefore := dq.Len()

	// Release drops the application reference: refcount 2 → 1.
	bg.Release()
	if got := ref.RefCount(); got != 1 {
		t.Fatalf("after Release (with in-flight clone): want 1, got %d", got)
	}

	// The HAL resource should NOT be destroyed yet — the Clone still holds a ref.
	// With the old Phase 1 code, Release() would call dq.Defer() directly,
	// ignoring the Clone. With ADR-056, onZero doesn't fire until refcount = 0.
	pendingAfterRelease := dq.Len()
	if pendingAfterRelease != pendingBefore {
		t.Errorf("dq.Defer should NOT be called while refs remain: pending before=%d, after=%d",
			pendingBefore, pendingAfterRelease)
	}

	// Simulate GPU completion → Triage → Drop the cloned ref.
	ref.Drop()
	if got := ref.RefCount(); got != 0 {
		t.Fatalf("after final Drop: want 0, got %d", got)
	}

	// NOW the onZero callback should have fired, scheduling deferred destruction.
	pendingAfterDrop := dq.Len()
	if pendingAfterDrop <= pendingBefore {
		t.Errorf("onZero should schedule dq.Defer: pending before=%d, after final drop=%d",
			pendingBefore, pendingAfterDrop)
	}
}

// TestBindGroup_ReleaseWithoutClone_DestroysImmediately verifies that when
// a BindGroup is released without any in-flight Clone (never used in a pass),
// the refcount goes 1→0 and onZero fires immediately.
func TestBindGroup_ReleaseWithoutClone_DestroysImmediately(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "immediate-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "immediate-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	ref := bg.TestRef()
	pendingBefore := dq.Len()

	// Release with no Clone → refcount 1→0 → onZero fires → dq.Defer.
	bg.Release()

	if got := ref.RefCount(); got != 0 {
		t.Fatalf("after Release without Clone: want refcount=0, got %d", got)
	}

	pendingAfter := dq.Len()
	if pendingAfter <= pendingBefore {
		t.Errorf("onZero should schedule dq.Defer immediately: pending before=%d, after=%d",
			pendingBefore, pendingAfter)
	}
}

// TestRenderPipeline_ReleaseUsesRefDrop verifies that RenderPipeline.Release()
// goes through ResourceRef.Drop() rather than dq.Defer() directly.
func TestRenderPipeline_ReleaseUsesRefDrop(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "lifecycle-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "lifecycle-rp",
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}

	ref := pipeline.TestRef()
	if ref == nil {
		t.Fatal("RenderPipeline should have a non-nil ResourceRef")
	}

	// Simulate SetPipeline → Clone.
	ref.Clone()

	pendingBefore := dq.Len()

	// Release: refcount 2 → 1 (Clone still holds).
	pipeline.Release()
	if got := ref.RefCount(); got != 1 {
		t.Fatalf("after Release with in-flight clone: want 1, got %d", got)
	}
	if dq.Len() != pendingBefore {
		t.Error("dq.Defer should NOT be called while refs remain")
	}

	// Simulate GPU completion → Drop cloned ref.
	ref.Drop()
	if got := ref.RefCount(); got != 0 {
		t.Fatalf("after final Drop: want 0, got %d", got)
	}
	if dq.Len() <= pendingBefore {
		t.Error("onZero should schedule dq.Defer after final Drop")
	}
}

// TestComputePipeline_ReleaseUsesRefDrop verifies that ComputePipeline.Release()
// goes through ResourceRef.Drop() rather than dq.Defer() directly.
func TestComputePipeline_ReleaseUsesRefDrop(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "lifecycle-cs",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "lifecycle-cp",
		Module:     mod,
		EntryPoint: "main",
	})
	if err != nil {
		t.Skipf("CreateComputePipeline not supported: %v", err)
	}

	ref := pipeline.TestRef()
	if ref == nil {
		t.Fatal("ComputePipeline should have a non-nil ResourceRef")
	}

	// Simulate SetPipeline → Clone.
	ref.Clone()

	pendingBefore := dq.Len()

	// Release: refcount 2 → 1 (Clone still holds).
	pipeline.Release()
	if got := ref.RefCount(); got != 1 {
		t.Fatalf("after Release with in-flight clone: want 1, got %d", got)
	}
	if dq.Len() != pendingBefore {
		t.Error("dq.Defer should NOT be called while refs remain")
	}

	// Simulate GPU completion → Drop cloned ref.
	ref.Drop()
	if got := ref.RefCount(); got != 0 {
		t.Fatalf("after final Drop: want 0, got %d", got)
	}
	if dq.Len() <= pendingBefore {
		t.Error("onZero should schedule dq.Defer after final Drop")
	}
}

// TestBindGroup_WithBuffer_ReleaseUsesRefDrop verifies the full scenario from
// Issue #287: a BindGroup containing a buffer is used via SetBindGroup (which
// Clone's the ref), then Released before the GPU completes. The HAL bind group
// must NOT be destroyed until the GPU submission finishes.
func TestBindGroup_WithBuffer_ReleaseUsesRefDrop(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lifecycle-buf",
		Size:  64,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "lifecycle-buf-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageVertex | wgpu.ShaderStageFragment,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 64,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "lifecycle-buf-bg",
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 64},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	ref := bg.TestRef()
	if ref == nil {
		t.Fatal("BindGroup should have ResourceRef")
	}

	// Simulate the encoder path: SetBindGroup → trackRef → Clone.
	ref.Clone()
	if got := ref.RefCount(); got != 2 {
		t.Fatalf("after SetBindGroup Clone: want 2, got %d", got)
	}

	pendingBefore := dq.Len()

	// User calls Release() while GPU is still processing.
	bg.Release()

	// Refcount 2→1, NOT 0. HAL bind group is still alive.
	if got := ref.RefCount(); got != 1 {
		t.Fatalf("after Release with in-flight: want 1, got %d", got)
	}
	if dq.Len() != pendingBefore {
		t.Error("HAL destruction should NOT be scheduled while refs remain")
	}

	// Simulate GPU completion: Triage calls Drop on tracked ref.
	ref.Drop()
	if got := ref.RefCount(); got != 0 {
		t.Fatalf("after GPU completion: want 0, got %d", got)
	}

	// NOW onZero fires → dq.Defer schedules HAL destruction.
	if dq.Len() <= pendingBefore {
		t.Error("onZero should schedule HAL destruction after GPU completion")
	}
}

// TestMixedResourceLifecycle_TrackedSubmission verifies that when multiple
// resource types (BindGroup, RenderPipeline) are tracked in a single submission,
// Releasing them before GPU completion does not destroy HAL resources prematurely.
// After Triage, all onZero callbacks fire and schedule deferred destruction.
func TestMixedResourceLifecycle_TrackedSubmission(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	// Create resources.
	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "mixed-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "mixed-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "mixed-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "mixed-rp",
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}

	bgRef := bg.TestRef()
	rpRef := pipeline.TestRef()

	// Simulate encoding: Clone refs for the submission.
	bgRef.Clone()
	rpRef.Clone()

	// Simulate Submit: TrackSubmission with cloned refs.
	dq.TrackSubmission(42, []*core.ResourceRef{bgRef, rpRef})

	// User releases both resources before GPU completes.
	bg.Release()
	pipeline.Release()

	// Refcounts: both should be 1 (Clone from tracked submission).
	if got := bgRef.RefCount(); got != 1 {
		t.Fatalf("bg ref after Release: want 1, got %d", got)
	}
	if got := rpRef.RefCount(); got != 1 {
		t.Fatalf("rp ref after Release: want 1, got %d", got)
	}

	pendingBefore := dq.Len()

	// Simulate GPU completion: Triage drops the tracked refs.
	dq.Triage(42)

	// Both refcounts should be 0, and onZero should have scheduled dq.Defer.
	if got := bgRef.RefCount(); got != 0 {
		t.Fatalf("bg ref after Triage: want 0, got %d", got)
	}
	if got := rpRef.RefCount(); got != 0 {
		t.Fatalf("rp ref after Triage: want 0, got %d", got)
	}

	pendingAfter := dq.Len()
	if pendingAfter <= pendingBefore {
		t.Errorf("onZero callbacks should schedule deferred destruction: pending before=%d, after=%d",
			pendingBefore, pendingAfter)
	}
}

// TestLastSubmissionIndex_NoDeadlock_OnZeroDuringTriage verifies that onZero
// callbacks can safely call LastSubmissionIndex() during Triage without deadlock.
//
// Regression test for v0.30.28 deadlock:
//
//	Submit() → Queue.mu.Lock()
//	  → postSubmit() → Triage()
//	    → onZero callback → LastSubmissionIndex() → Queue.mu.Lock() ← DEADLOCK
//
// Fixed in v0.30.29: lastSubmissionIndex changed to atomic.Uint64.
func TestLastSubmissionIndex_NoDeadlock_OnZeroDuringTriage(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	// Create a ResourceRef whose onZero calls lastSubmissionIndex —
	// this is the exact pattern that caused the v0.30.28 deadlock.
	var capturedIdx uint64
	ref := core.NewResourceRef("deadlock-test", func() {
		capturedIdx = device.Queue().LastSubmissionIndex()
		dq.Defer(capturedIdx, "deadlock-test", func() {})
	})

	// Simulate recording: Clone for the submission.
	ref.Clone()

	// Track in submission at index 10.
	dq.TrackSubmission(10, []*core.ResourceRef{ref})

	// Drop the "user" ref (simulates Release).
	ref.Drop() // refCount 2→1

	// Triage with completedIndex=10: drops tracked ref → refCount 1→0 → onZero.
	// onZero calls LastSubmissionIndex() + dq.Defer().
	// With mutex-based LastSubmissionIndex, this would deadlock if called
	// under Queue.mu. With atomic, it completes safely.
	//
	// We use a timeout channel to detect deadlock.
	done := make(chan struct{})
	go func() {
		dq.Triage(10)
		close(done)
	}()

	select {
	case <-done:
		// Success — no deadlock.
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: Triage blocked for 5s — onZero → LastSubmissionIndex likely re-locked")
	}

	if capturedIdx == 0 && ref.RefCount() != 0 {
		t.Error("onZero should have fired and captured lastSubmissionIndex")
	}
}
