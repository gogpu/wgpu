//go:build !rust && !(js && wasm)

package wgpu

import (
	"log/slog"
	"runtime"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// bindGroupCleanupRef holds the data needed to destroy a bind group's HAL
// resources from a runtime.AddCleanup callback. This struct must NOT reference
// the BindGroup itself — runtime.AddCleanup requires the callback argument to
// be independent of the cleaned-up object.
type bindGroupCleanupRef struct {
	label    string
	released *atomic.Bool
	ref      *core.ResourceRef // for ref.Drop() in GC path (ADR-056)
}

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	hal      hal.BindGroupLayout
	device   *Device
	released bool
	// entries stores the layout entries for entry-by-entry compatibility checks.
	// This matches Rust wgpu-core's pattern where binder.check_compatibility()
	// compares layouts by their entries, not by pointer identity.
	entries []gputypes.BindGroupLayoutEntry
}

// isCompatibleWith returns true if two layouts have identical entries.
// This matches Rust wgpu-core's entry-by-entry compatibility check in
// binder.check_compatibility(), allowing equivalent layouts created via
// separate CreateBindGroupLayout calls to be considered compatible.
func (l *BindGroupLayout) isCompatibleWith(other *BindGroupLayout) bool {
	if l == other {
		return true // pointer equality fast path
	}
	if len(l.entries) != len(other.entries) {
		return false
	}
	for i := range l.entries {
		if !bindGroupLayoutEntriesEqual(&l.entries[i], &other.entries[i]) {
			return false
		}
	}
	return true
}

// bindGroupLayoutEntriesEqual compares two BindGroupLayoutEntry values,
// dereferencing pointer fields (Buffer, Sampler, Texture, StorageTexture)
// to compare by value rather than by pointer identity.
func bindGroupLayoutEntriesEqual(a, b *gputypes.BindGroupLayoutEntry) bool {
	if a.Binding != b.Binding || a.Visibility != b.Visibility {
		return false
	}

	// Compare Buffer pointer fields by value.
	if !optionalEqual(a.Buffer, b.Buffer) {
		return false
	}
	// Compare Sampler pointer fields by value.
	if !optionalEqual(a.Sampler, b.Sampler) {
		return false
	}
	// Compare Texture pointer fields by value.
	if !optionalEqual(a.Texture, b.Texture) {
		return false
	}
	// Compare StorageTexture pointer fields by value.
	if !optionalEqual(a.StorageTexture, b.StorageTexture) {
		return false
	}
	return true
}

// optionalEqual compares two optional (pointer) values by dereferenced content.
// Both nil → equal; one nil → not equal; both non-nil → compare dereferenced values.
func optionalEqual[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Release destroys the bind group layout. Destruction is deferred until the
// GPU completes any submission that may reference this layout.
func (l *BindGroupLayout) Release() {
	if l.released {
		return
	}
	l.released = true

	halDevice := l.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := l.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyBindGroupLayout(l.hal)
		return
	}

	subIdx := l.device.lastSubmissionIndex()
	halLayout := l.hal
	dq.Defer(subIdx, "BindGroupLayout", func() {
		halDevice.DestroyBindGroupLayout(halLayout)
	})
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	hal      hal.PipelineLayout
	device   *Device
	released bool
	// bindGroupCount is the number of bind group layouts in this layout.
	// Used for validation in SetBindGroup.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts used to create this pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
}

// Release destroys the pipeline layout. Destruction is deferred until the
// GPU completes any submission that may reference this layout.
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true

	halDevice := l.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := l.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyPipelineLayout(l.hal)
		return
	}

	subIdx := l.device.lastSubmissionIndex()
	halLayout := l.hal
	dq.Defer(subIdx, "PipelineLayout", func() {
		halDevice.DestroyPipelineLayout(halLayout)
	})
}

// LateBufferBindingInfo records the actual buffer binding size for a layout entry
// with MinBindingSize == 0. At draw/dispatch time, these sizes are compared against
// the shader-required minimums stored on the pipeline.
//
// Matches Rust wgpu-core's BindGroupLateBufferBindingInfo (binding_model.rs:1167-1173).
type LateBufferBindingInfo struct {
	// BindingIndex is the binding number from the layout entry.
	BindingIndex uint32
	// Size is the actual buffer binding size at bind group creation time.
	Size uint64
}

// BindGroup represents bound GPU resources for shader access.
type BindGroup struct {
	hal     hal.BindGroup
	device  *Device
	cleanup runtime.Cleanup
	// released is heap-allocated separately so that the cleanup ref can share
	// it without holding an interior pointer into the BindGroup struct. An interior
	// pointer would make the BindGroup reachable from the cleanup arg, preventing
	// GC collection and causing the cleanup to never fire.
	released *atomic.Bool
	// layout is the bind group layout used to create this bind group.
	// Stored for draw-time compatibility validation via the binder.
	layout *BindGroupLayout
	// lateBufferBindingInfos records actual buffer sizes for layout entries
	// with MinBindingSize == 0. Listed in iteration order of the layout entries,
	// matching Rust wgpu-core's BindGroup.late_buffer_binding_infos.
	lateBufferBindingInfos []LateBufferBindingInfo
	// ref is the GPU-aware reference counter for this bind group (Phase 2).
	// Clone'd when used in a render/compute pass, Drop'd when GPU completes submission.
	ref *core.ResourceRef
	// boundBuffers holds references to buffers bound in this bind group (VAL-A6).
	// Used at Submit time to verify that referenced buffers are still alive
	// and not mapped. Matches Rust wgpu-core's pattern where
	// validate_command_buffer iterates cmd_buf_data.trackers.buffers.
	boundBuffers []*Buffer
	// boundTextures holds references to textures bound in this bind group (VAL-A6).
	// Used at Submit time to verify that referenced textures are still alive.
	boundTextures []*Texture
}

// Release marks the bind group for destruction. Drops the application's
// ownership reference via ResourceRef.Drop(). If the bind group is still
// referenced by in-flight GPU submissions (Clone'd via SetBindGroup), the
// HAL bind group stays alive until the GPU completes and Triage drops all
// tracked refs. The onZero callback (set at CreateBindGroup) fires only
// when the last reference drops, deferring HAL destruction via DestroyQueue.
//
// This matches Rust wgpu's Arc<BindGroup> Drop behavior — deterministic,
// refcount-driven destruction. ADR-056: unified resource lifecycle.
func (g *BindGroup) Release() {
	if g.released == nil || !g.released.CompareAndSwap(false, true) {
		return
	}

	// Cancel the GC cleanup — we are destroying explicitly.
	g.cleanup.Stop()

	if g.ref != nil {
		g.ref.Drop()
	}
}

// registerBindGroupCleanup registers a runtime.AddCleanup handler on the bind group.
// When GC collects the bind group without an explicit Release(), the cleanup
// drops the ResourceRef — the same refcount-driven path as Release(). ADR-056.
func registerBindGroupCleanup(bg *BindGroup, _ *Device, label string) runtime.Cleanup {
	return runtime.AddCleanup(bg, func(ref bindGroupCleanupRef) {
		if !ref.released.CompareAndSwap(false, true) {
			return
		}
		slog.Warn("wgpu: BindGroup released by GC (missing explicit Release)", "label", ref.label)
		if ref.ref != nil {
			ref.ref.Drop()
		}
	}, bindGroupCleanupRef{
		label:    label,
		released: bg.released,
		ref:      bg.ref,
	})
}
