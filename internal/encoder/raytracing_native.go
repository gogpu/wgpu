//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// BlasTriangleGeometry is one triangle geometry of a BLAS build, over the
// internal buffer handles. See the public BlasTriangleGeometry.
type BlasTriangleGeometry struct {
	Size                  *gputypes.BlasTriangleGeometrySizeDescriptor
	VertexBuffer          *resource.Buffer
	FirstVertex           uint32
	VertexStride          uint64
	IndexBuffer           *resource.Buffer
	FirstIndex            *uint32
	TransformBuffer       *resource.Buffer
	TransformBufferOffset *uint64
}

// BlasAabbGeometry is one AABB geometry of a BLAS build, over the internal
// buffer handles. See the public BlasAabbGeometry.
type BlasAabbGeometry struct {
	Size            *gputypes.BlasAABBGeometrySizeDescriptor
	Stride          uint64
	AabbBuffer      *resource.Buffer
	PrimitiveOffset uint32
}

// BlasBuildEntry pairs a BLAS with the geometry to build it from. Only one of
// Triangles and AABBs may be set.
type BlasBuildEntry struct {
	Blas      *resource.Blas
	Triangles []BlasTriangleGeometry
	AABBs     []BlasAabbGeometry
}

// TlasInstance is one non-empty instance slot of a TLAS build.
type TlasInstance struct {
	Blas       *resource.Blas
	Transform  [12]float32
	CustomData uint32
	Mask       uint8
}

// TlasPackage is a TLAS and the instances to build it from. The caller has
// already reconciled the TLAS's slot references (resource.Tlas.RetainSlot).
type TlasPackage struct {
	Tlas      *resource.Tlas
	Instances []TlasInstance
}

// BuildAccelerationStructures builds the given BLASes and then the given
// TLASes. Either argument may be nil.
//
// Errors are deferred to Finish, like other encoder commands. Whether the
// BLASes the TLAS instances reference are built is checked at Submit.
//
// Matches Rust wgpu's CommandEncoder::build_acceleration_structures.
func (e *CommandEncoder) BuildAccelerationStructures(blas []BlasBuildEntry, tlas []TlasPackage) {
	if e.released || e.locked("BuildAccelerationStructures") {
		return
	}
	fail := func(err error) {
		e.SetError(fmt.Errorf("wgpu: CommandEncoder.BuildAccelerationStructures: %w", err))
	}

	var buffers []*resource.Buffer
	useBuffer := func(buf *resource.Buffer) (*core.Buffer, error) {
		if buf == nil {
			return nil, nil //nolint:nilnil // optional buffer absent
		}
		if buf.CoreBuffer() == nil {
			return nil, fmt.Errorf("buffer is released: %w", core.ErrReleased)
		}
		if buf.IsReleased() {
			return nil, fmt.Errorf("buffer %q is released: %w", buf.Label(), core.ErrReleased)
		}
		buffers = append(buffers, buf)
		return buf.CoreBuffer(), nil
	}

	blasEntries := make([]core.BlasBuildEntry, len(blas))
	for i := range blas {
		entry, err := coreBlasBuildEntry(&blas[i], useBuffer)
		if err != nil {
			fail(err)
			return
		}
		blasEntries[i] = entry
	}

	tlasPackages := make([]core.TlasPackage, len(tlas))
	for i := range tlas {
		pkg, err := coreTlasPackage(&tlas[i])
		if err != nil {
			fail(err)
			return
		}
		tlasPackages[i] = pkg
	}

	if err := e.core.BuildAccelerationStructures(blasEntries, tlasPackages); err != nil {
		fail(err)
		return
	}

	// Keep everything the build reads or writes alive until the GPU is done.
	for i := range blasEntries {
		e.TrackRef(blasEntries[i].Blas.Ref)
	}
	for i := range tlasPackages {
		e.TrackRef(tlasPackages[i].Tlas.Ref)
		for _, inst := range tlasPackages[i].Instances {
			e.TrackRef(inst.Blas.Ref)
		}
	}
	for _, buf := range buffers {
		e.TrackRef(buf.CoreBuffer().Ref)
		e.TrackBuffer(buf)
	}
}

func coreBlasBuildEntry(entry *BlasBuildEntry, useBuffer func(*resource.Buffer) (*core.Buffer, error)) (core.BlasBuildEntry, error) {
	b := entry.Blas
	if b == nil {
		return core.BlasBuildEntry{}, fmt.Errorf("BLAS entry has no BLAS")
	}
	if b.IsReleased() {
		return core.BlasBuildEntry{}, fmt.Errorf("BLAS %q is released: %w", b.Core().Label(), core.ErrReleased)
	}
	out := core.BlasBuildEntry{Blas: b.Core()}
	if entry.Triangles != nil {
		out.Triangles = make([]core.BlasTriangleGeometry, len(entry.Triangles))
		for i := range entry.Triangles {
			g := &entry.Triangles[i]
			vertex, err := useBuffer(g.VertexBuffer)
			if err != nil {
				return core.BlasBuildEntry{}, err
			}
			index, err := useBuffer(g.IndexBuffer)
			if err != nil {
				return core.BlasBuildEntry{}, err
			}
			transform, err := useBuffer(g.TransformBuffer)
			if err != nil {
				return core.BlasBuildEntry{}, err
			}
			out.Triangles[i] = core.BlasTriangleGeometry{
				Size:                  g.Size,
				VertexBuffer:          vertex,
				FirstVertex:           g.FirstVertex,
				VertexStride:          g.VertexStride,
				IndexBuffer:           index,
				FirstIndex:            g.FirstIndex,
				TransformBuffer:       transform,
				TransformBufferOffset: g.TransformBufferOffset,
			}
		}
	}
	if entry.AABBs != nil {
		out.AABBs = make([]core.BlasAabbGeometry, len(entry.AABBs))
		for i := range entry.AABBs {
			g := &entry.AABBs[i]
			aabb, err := useBuffer(g.AabbBuffer)
			if err != nil {
				return core.BlasBuildEntry{}, err
			}
			out.AABBs[i] = core.BlasAabbGeometry{
				Size:            g.Size,
				Stride:          g.Stride,
				AabbBuffer:      aabb,
				PrimitiveOffset: g.PrimitiveOffset,
			}
		}
	}
	return out, nil
}

func coreTlasPackage(pkg *TlasPackage) (core.TlasPackage, error) {
	t := pkg.Tlas
	if t == nil {
		return core.TlasPackage{}, fmt.Errorf("TLAS is nil")
	}
	if t.IsReleased() {
		return core.TlasPackage{}, fmt.Errorf("TLAS %q is released: %w", t.Core().Label(), core.ErrReleased)
	}
	out := core.TlasPackage{Tlas: t.Core(), Instances: make([]*core.TlasInstance, 0, len(pkg.Instances))}
	for i := range pkg.Instances {
		inst := &pkg.Instances[i]
		if inst.Blas == nil {
			return core.TlasPackage{}, fmt.Errorf("TLAS %q instance has no BLAS", t.Core().Label())
		}
		out.Instances = append(out.Instances, &core.TlasInstance{
			Blas:       inst.Blas.Core(),
			Transform:  inst.Transform,
			CustomData: inst.CustomData,
			Mask:       inst.Mask,
		})
	}
	return out, nil
}
