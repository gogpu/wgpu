//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// Blas is a bottom-level acceleration structure: geometry (triangles or
// AABBs) that TLAS instances place in a scene. Experimental; requires
// FeatureRayQuery.
//
// The implementation lives in internal/resource (ADR-070).
//
// Matches Rust wgpu's Blas.
type Blas struct {
	impl *resource.Blas
}

// Handle returns the BLAS's device address, as written into TLAS instance
// data. The boolean is false if the backend has no such handle.
//
// Matches Rust wgpu's Blas::handle.
func (b *Blas) Handle() (uint64, bool) {
	if b == nil || b.impl.Core() == nil {
		return 0, false
	}
	return b.impl.Core().Handle(), true
}

// Release drops the application's reference to the BLAS. The acceleration
// structure is destroyed once no in-flight command buffer and no TLAS
// references it anymore.
func (b *Blas) Release() {
	if b == nil {
		return
	}
	b.impl.Release()
}

// blasImpl returns the implementation handle of a BLAS wrapper, or nil.
func blasImpl(b *Blas) *resource.Blas {
	if b == nil {
		return nil
	}
	return b.impl
}

// Tlas is a top-level acceleration structure: a set of BLAS instances that
// shaders trace rays against. It holds MaxInstances instance slots; nil slots
// are empty and skipped when the TLAS is built. Experimental; requires
// FeatureRayQuery.
//
// Changes to the instances take effect at the next
// CommandEncoder.BuildAccelerationStructures that includes the TLAS.
//
// Tlas is not safe for concurrent use.
//
// Matches Rust wgpu's Tlas.
type Tlas struct {
	impl *resource.Tlas

	// instances are the instance slots. The implementation holds a reference
	// on the BLAS of each slot (resource.Tlas.RetainSlot); slots changed
	// through InstancesMut are reconciled at the next build.
	instances []*TlasInstance
}

// MaxInstances returns the number of instance slots.
func (t *Tlas) MaxInstances() uint32 {
	return t.impl.Core().MaxInstances()
}

// Instances returns the instance slots. The slice must not be modified; use
// SetInstance or InstancesMut.
//
// Matches Rust wgpu's Tlas::get.
func (t *Tlas) Instances() []*TlasInstance {
	return t.instances
}

// SetInstance sets instance slot i (nil empties it). It panics if i is out
// of range, like indexing a slice.
//
// Matches Rust wgpu's Tlas IndexMut / get_mut_single.
func (t *Tlas) SetInstance(i int, instance *TlasInstance) {
	t.instances[i] = instance
	t.retainSlot(i)
}

// InstancesMut returns slots [lo, hi) for modification, or nil if the range
// is out of bounds.
//
// Matches Rust wgpu's Tlas::get_mut_slice.
func (t *Tlas) InstancesMut(lo, hi int) []*TlasInstance {
	if lo < 0 || hi > len(t.instances) || lo > hi {
		return nil
	}
	return t.instances[lo:hi:hi]
}

// Release drops the application's reference to the TLAS and to the BLASes
// its instance slots hold. The acceleration structure is destroyed once no
// in-flight command buffer or bind group references it anymore.
func (t *Tlas) Release() {
	if t == nil {
		return
	}
	t.impl.Release()
}

// retainSlot moves slot i's held BLAS reference to the BLAS it currently
// points at. Reports whether the slot's BLAS is still alive.
func (t *Tlas) retainSlot(i int) bool {
	var want *resource.Blas
	if inst := t.instances[i]; inst != nil {
		want = blasImpl(inst.Blas)
	}
	return t.impl.RetainSlot(i, want)
}

// tlasImpl returns the implementation handle of a TLAS wrapper, or nil.
func tlasImpl(t *Tlas) *resource.Tlas {
	if t == nil {
		return nil
	}
	return t.impl
}

// CreateBlas creates a bottom-level acceleration structure sized for the
// given geometry. It must be built with CommandEncoder.BuildAccelerationStructures
// before a TLAS referencing it is used. Requires FeatureRayQuery.
//
// Matches Rust wgpu's Device::create_blas.
func (d *Device) CreateBlas(desc *gputypes.CreateBlasDescriptor, sizes gputypes.BlasGeometrySizeDescriptors) (*Blas, error) {
	impl, err := d.impl.CreateBlas(desc, sizes)
	if err != nil {
		return nil, err
	}
	return &Blas{impl: impl}, nil
}

// CreateTlas creates a top-level acceleration structure with
// desc.MaxInstances empty instance slots. Requires FeatureRayQuery.
//
// Matches Rust wgpu's Device::create_tlas.
func (d *Device) CreateTlas(desc *gputypes.CreateTlasDescriptor) (*Tlas, error) {
	impl, err := d.impl.CreateTlas(desc)
	if err != nil {
		return nil, err
	}
	return &Tlas{impl: impl, instances: make([]*TlasInstance, impl.Core().MaxInstances())}, nil
}

// BuildAccelerationStructures builds the given BLASes and then the given
// TLASes from their current instances. Either argument may be nil.
//
// Errors are deferred to Finish, like other encoder commands. Whether the
// BLASes the TLAS instances reference are built is checked at Submit.
//
// Matches Rust wgpu's CommandEncoder::build_acceleration_structures.
func (e *CommandEncoder) BuildAccelerationStructures(blas []BlasBuildEntry, tlas []*Tlas) {
	if e.impl.Released() {
		return
	}
	blasEntries := make([]encoder.BlasBuildEntry, len(blas))
	for i := range blas {
		blasEntries[i] = encoderBlasBuildEntry(&blas[i])
	}
	tlasPackages := make([]encoder.TlasPackage, len(tlas))
	for i, t := range tlas {
		pkg, err := t.buildPackage()
		if err != nil {
			e.setError(fmt.Errorf("wgpu: CommandEncoder.BuildAccelerationStructures: %w", err))
			return
		}
		tlasPackages[i] = pkg
	}
	e.impl.BuildAccelerationStructures(blasEntries, tlasPackages)
}

// encoderBlasBuildEntry converts a public BLAS build entry to the encoder's
// form over the implementation handles.
func encoderBlasBuildEntry(entry *BlasBuildEntry) encoder.BlasBuildEntry {
	out := encoder.BlasBuildEntry{Blas: blasImpl(entry.Blas)}
	if entry.Geometry.Triangles != nil {
		out.Triangles = make([]encoder.BlasTriangleGeometry, len(entry.Geometry.Triangles))
		for i := range entry.Geometry.Triangles {
			g := &entry.Geometry.Triangles[i]
			out.Triangles[i] = encoder.BlasTriangleGeometry{
				Size:                  g.Size,
				VertexBuffer:          bufferImpl(g.VertexBuffer),
				FirstVertex:           g.FirstVertex,
				VertexStride:          g.VertexStride,
				IndexBuffer:           bufferImpl(g.IndexBuffer),
				FirstIndex:            g.FirstIndex,
				TransformBuffer:       bufferImpl(g.TransformBuffer),
				TransformBufferOffset: g.TransformBufferOffset,
			}
		}
	}
	if entry.Geometry.AABBs != nil {
		out.AABBs = make([]encoder.BlasAabbGeometry, len(entry.Geometry.AABBs))
		for i := range entry.Geometry.AABBs {
			g := &entry.Geometry.AABBs[i]
			out.AABBs[i] = encoder.BlasAabbGeometry{
				Size:            g.Size,
				Stride:          g.Stride,
				AabbBuffer:      bufferImpl(g.AabbBuffer),
				PrimitiveOffset: g.PrimitiveOffset,
			}
		}
	}
	return out
}

// buildPackage reconciles the slot references and converts the non-empty
// instances for a build.
func (t *Tlas) buildPackage() (encoder.TlasPackage, error) {
	if t == nil {
		return encoder.TlasPackage{}, fmt.Errorf("TLAS is nil")
	}
	if t.impl.IsReleased() {
		return encoder.TlasPackage{Tlas: t.impl}, nil // the encoder reports it
	}
	pkg := encoder.TlasPackage{Tlas: t.impl, Instances: make([]encoder.TlasInstance, 0, len(t.instances))}
	for i, inst := range t.instances {
		if !t.retainSlot(i) {
			return encoder.TlasPackage{}, fmt.Errorf("TLAS %q instance %d references a released BLAS: %w",
				t.impl.Core().Label(), i, ErrReleased)
		}
		if inst == nil {
			continue
		}
		if inst.Blas == nil {
			return encoder.TlasPackage{}, fmt.Errorf("TLAS %q instance %d has no BLAS", t.impl.Core().Label(), i)
		}
		pkg.Instances = append(pkg.Instances, encoder.TlasInstance{
			Blas:       inst.Blas.impl,
			Transform:  inst.Transform,
			CustomData: inst.CustomData,
			Mask:       inst.Mask,
		})
	}
	return pkg, nil
}
