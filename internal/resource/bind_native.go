//go:build !rust && !(js && wasm)

package resource

import (
	"log/slog"
	"runtime"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// BindGroupLayout defines the structure of resource bindings for shaders.
type BindGroupLayout struct {
	hal      hal.BindGroupLayout
	device   core.DeviceView
	released bool
	// entries stores the layout entries for entry-by-entry compatibility checks.
	// This matches Rust wgpu-core's pattern where binder.check_compatibility()
	// compares layouts by their entries, not by pointer identity.
	entries []gputypes.BindGroupLayoutEntry
}

// NewBindGroupLayout builds a bind group layout over a HAL object and the
// shared device view.
func NewBindGroupLayout(halLayout hal.BindGroupLayout, device core.DeviceView, entries []gputypes.BindGroupLayoutEntry) *BindGroupLayout {
	return &BindGroupLayout{hal: halLayout, device: device, entries: entries}
}

// HAL returns the HAL bind group layout, or nil.
func (l *BindGroupLayout) HAL() hal.BindGroupLayout {
	if l == nil {
		return nil
	}
	return l.hal
}

// Entries returns the layout entries, the basis of entry-by-entry
// compatibility.
func (l *BindGroupLayout) Entries() []gputypes.BindGroupLayoutEntry {
	if l == nil {
		return nil
	}
	return l.entries
}

// IsReleased reports whether Release has run on the layout.
func (l *BindGroupLayout) IsReleased() bool {
	return l != nil && l.released
}

// IsCompatibleWith returns true if two layouts have identical entries.
// This matches Rust wgpu-core's entry-by-entry compatibility check in
// binder.check_compatibility(), allowing equivalent layouts created via
// separate CreateBindGroupLayout calls to be considered compatible.
func (l *BindGroupLayout) IsCompatibleWith(other *BindGroupLayout) bool {
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

	halDevice := l.device.HALDevice()
	if halDevice == nil {
		return
	}

	dq := l.device.DestroyQueue()
	if dq == nil {
		halDevice.DestroyBindGroupLayout(l.hal)
		return
	}

	subIdx := l.device.LastSubmissionIndex()
	halLayout := l.hal
	dq.Defer(subIdx, "BindGroupLayout", func() {
		halDevice.DestroyBindGroupLayout(halLayout)
	})
}

// PipelineLayout defines the bind group layout arrangement for a pipeline.
type PipelineLayout struct {
	hal      hal.PipelineLayout
	device   core.DeviceView
	released bool
	// bindGroupCount is the number of bind group layouts in this layout.
	// Used for validation in SetBindGroup.
	bindGroupCount uint32
	// bindGroupLayouts stores the layouts used to create this pipeline layout.
	// Used by the binder for draw-time compatibility validation.
	bindGroupLayouts []*BindGroupLayout
	// ref counts the layout's owners: the application until Release, and
	// every pipeline created with it until that pipeline is destroyed, as
	// Rust wgpu-core's RenderPipeline and ComputePipeline hold an
	// Arc<PipelineLayout> (pipeline.rs:496, :1155). The HAL layout is
	// destroyed when the last one drops (ADR-056, P2-CORE-022).
	ref *core.ResourceRef
}

// NewPipelineLayout builds a pipeline layout over a HAL object and the shared
// device view.
func NewPipelineLayout(halLayout hal.PipelineLayout, device core.DeviceView, bindGroupCount uint32, bindGroupLayouts []*BindGroupLayout, ref *core.ResourceRef) *PipelineLayout {
	return &PipelineLayout{hal: halLayout, device: device, bindGroupCount: bindGroupCount, bindGroupLayouts: bindGroupLayouts, ref: ref}
}

// HAL returns the HAL pipeline layout, or nil.
func (l *PipelineLayout) HAL() hal.PipelineLayout {
	if l == nil {
		return nil
	}
	return l.hal
}

// BindGroupCount returns the number of bind group layouts.
func (l *PipelineLayout) BindGroupCount() uint32 {
	if l == nil {
		return 0
	}
	return l.bindGroupCount
}

// BindGroupLayouts returns the layouts used to create this pipeline layout.
func (l *PipelineLayout) BindGroupLayouts() []*BindGroupLayout {
	if l == nil {
		return nil
	}
	return l.bindGroupLayouts
}

// Ref returns the layout's reference count, or nil for one without it.
func (l *PipelineLayout) Ref() *core.ResourceRef {
	if l == nil {
		return nil
	}
	return l.ref
}

// IsReleased reports whether Release has run on the layout.
func (l *PipelineLayout) IsReleased() bool {
	return l != nil && l.released
}

// Release drops the application's reference to the pipeline layout. The HAL
// layout is destroyed once no pipeline created with it exists any more and the
// GPU has completed every submission that may use it: a layout released after
// its pipeline was created stays valid for that pipeline's binds (P2-CORE-022).
func (l *PipelineLayout) Release() {
	if l.released {
		return
	}
	l.released = true
	if l.ref != nil {
		l.ref.Drop()
		return
	}

	// No reference count: destroy after the submissions in flight.
	halDevice := l.device.HALDevice()
	if halDevice == nil {
		return
	}

	dq := l.device.DestroyQueue()
	if dq == nil {
		halDevice.DestroyPipelineLayout(l.hal)
		return
	}

	subIdx := l.device.LastSubmissionIndex()
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
	device  core.DeviceView
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
	// dynamicBindings are the bindings with a dynamic offset, sorted by
	// binding number; SetBindGroup validates its offsets against them.
	dynamicBindings []core.DynamicBinding
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
	// usages are the buffer and texture usages of the bound resources.
	// Setting the bind group merges them into the render pass's usage scope,
	// or into the usage scope of each dispatch whose pipeline uses the group,
	// so the resources are transitioned into these usages (Rust
	// BindGroup.used).
	usages *core.BindGroupUsages
}

// bindGroupCleanupRef holds the data needed to destroy a bind group's HAL
// resources from a runtime.AddCleanup callback. This struct must NOT reference
// the BindGroup itself — runtime.AddCleanup requires the callback argument to
// be independent of the cleaned-up object.
type bindGroupCleanupRef struct {
	label    string
	released *atomic.Bool
	ref      *core.ResourceRef // for ref.Drop() in GC path (ADR-056)
}

// BindGroupInit is the construction value for NewBindGroup. It is decomposed
// into the group's fields on entry and never stored.
type BindGroupInit struct {
	Label                  string
	HAL                    hal.BindGroup
	Device                 core.DeviceView
	Layout                 *BindGroupLayout
	LateBufferBindingInfos []LateBufferBindingInfo
	DynamicBindings        []core.DynamicBinding
	Ref                    *core.ResourceRef
	BoundBuffers           []*Buffer
	BoundTextures          []*Texture
	Usages                 *core.BindGroupUsages
}

// NewBindGroup builds a bind group over the HAL object and the shared device
// view, and arms its GC cleanup. The device that creates a bind group is the
// only caller.
func NewBindGroup(init *BindGroupInit) *BindGroup {
	g := &BindGroup{
		hal:                    init.HAL,
		device:                 init.Device,
		released:               new(atomic.Bool),
		layout:                 init.Layout,
		lateBufferBindingInfos: init.LateBufferBindingInfos,
		dynamicBindings:        init.DynamicBindings,
		ref:                    init.Ref,
		boundBuffers:           init.BoundBuffers,
		boundTextures:          init.BoundTextures,
		usages:                 init.Usages,
	}
	g.armCleanup(init.Label)
	return g
}

// armCleanup registers a runtime.AddCleanup handler on the bind group. When GC
// collects the bind group without an explicit Release(), the cleanup drops the
// ResourceRef — the same refcount-driven path as Release() (ADR-056). It is a
// method, not a package function, so the device that builds the group can arm
// it without naming this file's unexported declarations across a package
// boundary (ADR-070 §11 grain note).
func (g *BindGroup) armCleanup(label string) {
	g.cleanup = runtime.AddCleanup(g, func(ref bindGroupCleanupRef) {
		if !ref.released.CompareAndSwap(false, true) {
			return
		}
		slog.Warn("wgpu: BindGroup released by GC (missing explicit Release)", "label", ref.label)
		if ref.ref != nil {
			ref.ref.Drop()
		}
	}, bindGroupCleanupRef{
		label:    label,
		released: g.released,
		ref:      g.ref,
	})
}

// HAL returns the HAL bind group, or nil.
func (g *BindGroup) HAL() hal.BindGroup {
	if g == nil {
		return nil
	}
	return g.hal
}

// Ref returns the bind group's reference count, or nil for one without it.
func (g *BindGroup) Ref() *core.ResourceRef {
	if g == nil {
		return nil
	}
	return g.ref
}

// IsReleased reports whether Release has run on the bind group.
func (g *BindGroup) IsReleased() bool {
	return g != nil && g.released != nil && g.released.Load()
}

// Layout returns the bind group layout the group was created with.
func (g *BindGroup) Layout() *BindGroupLayout {
	if g == nil {
		return nil
	}
	return g.layout
}

// Device returns the shared device view the group was created on.
func (g *BindGroup) Device() core.DeviceView {
	if g == nil {
		return nil
	}
	return g.device
}

// LateBufferBindingInfos returns the recorded late buffer binding sizes, in
// layout entry order.
func (g *BindGroup) LateBufferBindingInfos() []LateBufferBindingInfo {
	if g == nil {
		return nil
	}
	return g.lateBufferBindingInfos
}

// DynamicBindings returns the bindings with a dynamic offset, sorted by
// binding number.
func (g *BindGroup) DynamicBindings() []core.DynamicBinding {
	if g == nil {
		return nil
	}
	return g.dynamicBindings
}

// BoundBuffers returns the buffers bound in the group, for submit-time
// validation.
func (g *BindGroup) BoundBuffers() []*Buffer {
	if g == nil {
		return nil
	}
	return g.boundBuffers
}

// BoundTextures returns the textures bound in the group, for submit-time
// validation.
func (g *BindGroup) BoundTextures() []*Texture {
	if g == nil {
		return nil
	}
	return g.boundTextures
}

// Usages returns the buffer and texture usages of the bound resources.
func (g *BindGroup) Usages() *core.BindGroupUsages {
	if g == nil {
		return nil
	}
	return g.usages
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
