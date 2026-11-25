//go:build !(js && wasm)

package core

import (
	"fmt"
	"slices"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/raytracing"
)

// defaultRawTlasInstanceSize is the size of one packed TLAS instance when the
// backend does not report hal.Alignments.RawTlasInstanceSize. It is the size
// of VkAccelerationStructureInstanceKHR / D3D12_RAYTRACING_INSTANCE_DESC.
const defaultRawTlasInstanceSize = 64

// Blas is a bottom-level acceleration structure with HAL integration.
//
// The HAL acceleration structure is held in a Snatchable so it can be
// destroyed safely. Bookkeeping (build sizes, flags, built index, compaction
// state) lives in an internal/raytracing.Blas guarded by mu; its Raw field is
// unused because the handle is owned by the Snatchable.
//
// Matches Rust wgpu-core resource.rs Blas.
type Blas struct {
	raw    *Snatchable[hal.AccelerationStructure]
	device *Device

	// sizes is a copy of the creation-time geometry size descriptors, used to
	// validate build entries.
	sizes gputypes.BlasGeometrySizeDescriptors

	mu    sync.Mutex
	state raytracing.Blas

	// Ref is the GPU-aware reference counter. The owner (public wrapper),
	// command buffers that build the BLAS, and TLASes that reference it hold
	// references. The HAL acceleration structure is destroyed when the last
	// reference is dropped.
	Ref *ResourceRef
}

// Label returns the BLAS debug label.
func (b *Blas) Label() string { return b.state.Label }

// Flags returns the creation flags.
func (b *Blas) Flags() gputypes.AccelerationStructureFlags { return b.state.Flags }

// Handle returns the device address of the BLAS, as used by TLAS instances.
func (b *Blas) Handle() uint64 { return b.state.Handle }

// Sizes returns the creation-time geometry size descriptors.
func (b *Blas) Sizes() gputypes.BlasGeometrySizeDescriptors { return b.sizes }

// SizeInfo returns the build sizes reported by the backend at creation.
func (b *Blas) SizeInfo() hal.AccelerationStructureBuildSizes { return b.state.SizeInfo }

// BuiltIndex returns the index of the build command that last built this
// BLAS, or 0 if it has never been built (as of the last submission).
func (b *Blas) BuiltIndex() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.BuiltIndex
}

// CompactionState returns the compaction lifecycle state.
func (b *Blas) CompactionState() raytracing.CompactionState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state.Compaction
}

// Raw returns the HAL acceleration structure, or nil if destroyed.
func (b *Blas) Raw(guard SnatchGuard) hal.AccelerationStructure {
	if b.raw == nil {
		return nil
	}
	p := b.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// IsDestroyed returns true if the HAL acceleration structure has been destroyed.
func (b *Blas) IsDestroyed() bool {
	return b.raw == nil || b.raw.IsSnatched()
}

// Destroy releases the HAL acceleration structure. Idempotent.
//
// Normally called by Ref's onZero callback once no command buffer or TLAS
// references the BLAS anymore.
func (b *Blas) Destroy() {
	if b.device == nil || b.raw == nil {
		return
	}
	halDevice, raw := snatchAccelerationStructure(b.device, b.raw)
	if halDevice != nil && raw != nil {
		halDevice.DestroyAccelerationStructure(raw)
	}
}

// Tlas is a top-level acceleration structure with HAL integration.
//
// It owns the HAL acceleration structure and a persistent instance buffer
// (MaxInstances * RawTlasInstanceSize bytes) that builds copy the packed
// instances into. After a submission builds it, the TLAS holds references on
// the BLASes it was built from, so they outlive it.
//
// Matches Rust wgpu-core resource.rs Tlas.
type Tlas struct {
	raw            *Snatchable[hal.AccelerationStructure]
	instanceBuffer hal.Buffer
	device         *Device

	mu    sync.Mutex
	state raytracing.Tlas
	// dependencies are the BLASes the last submitted build referenced. Each
	// holds a Ref clone owned by this TLAS.
	// Matches Rust wgpu-core Tlas.dependencies.
	dependencies []*Blas

	// Ref is the GPU-aware reference counter (see Blas.Ref).
	Ref *ResourceRef
}

// Label returns the TLAS debug label.
func (t *Tlas) Label() string { return t.state.Label }

// Flags returns the creation flags.
func (t *Tlas) Flags() gputypes.AccelerationStructureFlags { return t.state.Flags }

// MaxInstances returns the maximum number of instances the TLAS can hold.
func (t *Tlas) MaxInstances() uint32 { return t.state.MaxInstances }

// SizeInfo returns the build sizes reported by the backend at creation.
func (t *Tlas) SizeInfo() hal.AccelerationStructureBuildSizes { return t.state.SizeInfo }

// BuiltIndex returns the index of the build command that last built this
// TLAS, or 0 if it has never been built (as of the last submission).
func (t *Tlas) BuiltIndex() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state.BuiltIndex
}

// Raw returns the HAL acceleration structure, or nil if destroyed.
func (t *Tlas) Raw(guard SnatchGuard) hal.AccelerationStructure {
	if t.raw == nil {
		return nil
	}
	p := t.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// HAL returns the HAL acceleration structure under the device's snatch lock,
// or nil if it is destroyed.
func (t *Tlas) HAL() hal.AccelerationStructure {
	if t == nil || t.device == nil {
		return nil
	}
	guard := t.device.SnatchLock().Read()
	defer guard.Release()
	return t.Raw(guard)
}

// IsDestroyed returns true if the HAL acceleration structure has been destroyed.
func (t *Tlas) IsDestroyed() bool {
	return t.raw == nil || t.raw.IsSnatched()
}

// Destroy releases the HAL acceleration structure and the instance buffer,
// and drops the TLAS's references on its BLAS dependencies. Idempotent.
func (t *Tlas) Destroy() {
	if t.device == nil || t.raw == nil {
		return
	}
	halDevice, raw := snatchAccelerationStructure(t.device, t.raw)
	if halDevice != nil && raw != nil {
		halDevice.DestroyAccelerationStructure(raw)
		if t.instanceBuffer != nil {
			halDevice.DestroyBuffer(t.instanceBuffer)
		}
	}
	t.instanceBuffer = nil

	// Dropped outside the snatch lock: a dependency's onZero takes it again.
	t.mu.Lock()
	deps := t.dependencies
	t.dependencies = nil
	t.mu.Unlock()
	for _, blas := range deps {
		blas.Ref.Drop()
	}
}

// snatchAccelerationStructure takes the HAL acceleration structure out of raw
// under the device's exclusive snatch lock. It returns nils if the device or
// the acceleration structure is already gone.
func snatchAccelerationStructure(
	device *Device,
	raw *Snatchable[hal.AccelerationStructure],
) (hal.Device, hal.AccelerationStructure) {
	lock := device.SnatchLock()
	if lock == nil {
		return nil, nil
	}
	readGuard := lock.Read()
	halDevice := device.Raw(readGuard)
	readGuard.Release()
	if halDevice == nil {
		return nil, nil
	}

	guard := lock.Write()
	defer guard.Release()
	p := raw.Snatch(guard)
	if p == nil {
		return nil, nil
	}
	return halDevice, *p
}

// allowedBlasVertexFormats returns the vertex formats a BLAS triangle
// geometry may use. Rust additionally allows Float32x2, Float16x2, Float16x4,
// Snorm16x2 and Snorm16x4 with EXTENDED_ACCELERATION_STRUCTURE_VERTEX_FORMATS,
// which gputypes does not define.
//
// Matches Rust wgpu-types Features::allowed_vertex_formats_for_blas.
func allowedBlasVertexFormats() []gputypes.VertexFormat {
	return []gputypes.VertexFormat{gputypes.VertexFormatFloat32x3}
}

func isAllowedBlasVertexFormat(format gputypes.VertexFormat) bool {
	return slices.Contains(allowedBlasVertexFormats(), format)
}

// alignments returns the device's HAL alignment requirements.
func (d *Device) alignments() hal.Alignments {
	if d.adapter != nil && d.adapter.halCapabilities != nil {
		return d.adapter.halCapabilities.AlignmentsMask
	}
	return hal.Alignments{}
}

// CreateBlas creates a bottom-level acceleration structure sized for the
// given geometry descriptors.
//
// The BLAS is not built; CommandEncoder.BuildAccelerationStructures must
// build it before any TLAS that references it is used.
//
// Matches Rust wgpu-core device/ray_tracing.rs create_blas.
func (d *Device) CreateBlas(desc *gputypes.CreateBlasDescriptor, sizes gputypes.BlasGeometrySizeDescriptors) (*Blas, error) {
	if err := d.checkValid(); err != nil {
		return nil, err
	}
	if desc == nil {
		desc = &gputypes.CreateBlasDescriptor{}
	}
	blasErr := func(kind CreateBlasErrorKind, format string, args ...any) error {
		return &CreateBlasError{Kind: kind, Label: desc.Label, Message: fmt.Sprintf(format, args...)}
	}

	if !d.Features.Contains(gputypes.FeatureRayQuery) {
		return nil, blasErr(CreateBlasErrorMissingFeature, "requires feature FeatureRayQuery")
	}
	if desc.Flags.Contains(gputypes.ASFlagAllowRayHitVertexReturn) &&
		!d.Features.Contains(gputypes.FeatureRayHitVertexReturn) {
		return nil, blasErr(CreateBlasErrorMissingFeature,
			"flag AllowRayHitVertexReturn requires feature FeatureRayHitVertexReturn")
	}
	if sizes.Triangles != nil && sizes.AABBs != nil {
		return nil, blasErr(CreateBlasErrorGeometryKind,
			"geometry size descriptors must contain triangles or AABBs, not both")
	}

	entries, err := d.blasSizeQueryEntries(desc, &sizes, blasErr)
	if err != nil {
		return nil, err
	}

	guard := d.snatchLock.Read()
	defer guard.Release()
	halDevice := d.Raw(guard)
	if halDevice == nil {
		return nil, ErrDeviceDestroyed
	}

	sizeInfo := halDevice.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: entries,
		Flags:   desc.Flags,
	})
	raw, err := halDevice.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:           desc.Label,
		Size:            sizeInfo.AccelerationStructureSize,
		Format:          hal.AccelerationStructureFormatBottomLevel,
		AllowCompaction: desc.Flags.Contains(gputypes.ASFlagAllowCompaction),
	})
	if err != nil {
		return nil, &CreateBlasError{Kind: CreateBlasErrorHAL, Label: desc.Label, Message: err.Error(), HALError: err}
	}

	blas := &Blas{
		raw:    NewSnatchable(raw),
		device: d,
		sizes:  cloneBlasSizes(sizes),
		state: raytracing.Blas{
			Label:         desc.Label,
			SizeInfo:      sizeInfo,
			Flags:         desc.Flags,
			UpdateMode:    desc.UpdateMode,
			GeometryCount: uint32(len(sizes.Triangles) + len(sizes.AABBs)), //nolint:gosec // bounded by MaxBlasGeometryCount above
			Handle:        halDevice.GetAccelerationStructureDeviceAddress(raw),
		},
	}
	blas.Ref = NewResourceRef("Blas:"+desc.Label, blas.Destroy)
	return blas, nil
}

// blasSizeQueryEntries validates the size descriptors and converts them to
// buffer-less HAL entries for the build size query.
func (d *Device) blasSizeQueryEntries(
	desc *gputypes.CreateBlasDescriptor,
	sizes *gputypes.BlasGeometrySizeDescriptors,
	blasErr func(CreateBlasErrorKind, string, ...any) error,
) (*hal.AccelerationStructureEntries, error) {
	maxGeometries := d.Limits.MaxBlasGeometryCount
	maxPrimitives := d.Limits.MaxBlasPrimitiveCount

	if sizes.AABBs != nil {
		if uint64(len(sizes.AABBs)) > uint64(maxGeometries) {
			return nil, blasErr(CreateBlasErrorTooManyGeometries,
				"limit MaxBlasGeometryCount is %d, but the BLAS had %d geometries", maxGeometries, len(sizes.AABBs))
		}
		aabbs := make([]hal.AccelerationStructureAABBs, len(sizes.AABBs))
		for i, s := range sizes.AABBs {
			if s.PrimitiveCount > maxPrimitives {
				return nil, blasErr(CreateBlasErrorTooManyPrimitives,
					"limit MaxBlasPrimitiveCount is %d, but geometry %d had %d primitives", maxPrimitives, i, s.PrimitiveCount)
			}
			aabbs[i] = hal.AccelerationStructureAABBs{
				Count:  s.PrimitiveCount,
				Stride: gputypes.AABBGeometryMinStride,
				Flags:  s.Flags,
			}
		}
		return &hal.AccelerationStructureEntries{AABBs: aabbs}, nil
	}

	if uint64(len(sizes.Triangles)) > uint64(maxGeometries) {
		return nil, blasErr(CreateBlasErrorTooManyGeometries,
			"limit MaxBlasGeometryCount is %d, but the BLAS had %d geometries", maxGeometries, len(sizes.Triangles))
	}
	triangles := make([]hal.AccelerationStructureTriangles, len(sizes.Triangles))
	for i, s := range sizes.Triangles {
		if (s.IndexCount != nil) != (s.IndexFormat != nil) {
			return nil, blasErr(CreateBlasErrorMissingIndexData,
				"geometry %d: IndexFormat and IndexCount must be provided together", i)
		}
		if !isAllowedBlasVertexFormat(s.VertexFormat) {
			return nil, blasErr(CreateBlasErrorInvalidVertexFormat,
				"geometry %d: vertex format %v is not allowed (allowed: %v)", i, s.VertexFormat, allowedBlasVertexFormats())
		}
		// Rust compares the vertex count against the primitive limit.
		if s.VertexCount > maxPrimitives {
			return nil, blasErr(CreateBlasErrorTooManyPrimitives,
				"limit MaxBlasPrimitiveCount is %d, but geometry %d had %d vertices", maxPrimitives, i, s.VertexCount)
		}
		tri := hal.AccelerationStructureTriangles{
			VertexFormat: s.VertexFormat,
			VertexCount:  s.VertexCount,
			Flags:        s.Flags,
		}
		if s.IndexCount != nil {
			tri.Indices = &hal.AccelerationStructureTriangleIndices{Format: *s.IndexFormat, Count: *s.IndexCount}
		}
		if desc.Flags.Contains(gputypes.ASFlagUseTransform) {
			tri.Transform = &hal.AccelerationStructureTriangleTransform{}
		}
		triangles[i] = tri
	}
	return &hal.AccelerationStructureEntries{Triangles: triangles}, nil
}

// cloneBlasSizes deep-copies size descriptors so later caller mutations
// cannot change the creation-time values used for build validation.
func cloneBlasSizes(sizes gputypes.BlasGeometrySizeDescriptors) gputypes.BlasGeometrySizeDescriptors {
	var out gputypes.BlasGeometrySizeDescriptors
	if sizes.AABBs != nil {
		out.AABBs = append([]gputypes.BlasAABBGeometrySizeDescriptor{}, sizes.AABBs...)
	}
	if sizes.Triangles != nil {
		out.Triangles = make([]gputypes.BlasTriangleGeometrySizeDescriptor, len(sizes.Triangles))
		for i, s := range sizes.Triangles {
			if s.IndexFormat != nil {
				f := *s.IndexFormat
				s.IndexFormat = &f
			}
			if s.IndexCount != nil {
				c := *s.IndexCount
				s.IndexCount = &c
			}
			out.Triangles[i] = s
		}
	}
	return out
}

// CreateTlas creates a top-level acceleration structure and its persistent
// instance buffer.
//
// Matches Rust wgpu-core device/ray_tracing.rs create_tlas.
func (d *Device) CreateTlas(desc *gputypes.CreateTlasDescriptor) (*Tlas, error) {
	if err := d.checkValid(); err != nil {
		return nil, err
	}
	if desc == nil {
		desc = &gputypes.CreateTlasDescriptor{}
	}
	tlasErr := func(kind CreateTlasErrorKind, format string, args ...any) error {
		return &CreateTlasError{Kind: kind, Label: desc.Label, Message: fmt.Sprintf(format, args...)}
	}

	if !d.Features.Contains(gputypes.FeatureRayQuery) {
		return nil, tlasErr(CreateTlasErrorMissingFeature, "requires feature FeatureRayQuery")
	}
	if desc.MaxInstances > d.Limits.MaxTlasInstanceCount {
		return nil, tlasErr(CreateTlasErrorTooManyInstances,
			"limit MaxTlasInstanceCount is %d, but the TLAS had a maximum of %d instances",
			d.Limits.MaxTlasInstanceCount, desc.MaxInstances)
	}
	if desc.Flags.Contains(gputypes.ASFlagUseTransform) {
		return nil, tlasErr(CreateTlasErrorDisallowedFlag, "flag UseTransform is not allowed on a TLAS")
	}
	if desc.Flags.Contains(gputypes.ASFlagAllowRayHitVertexReturn) &&
		!d.Features.Contains(gputypes.FeatureRayHitVertexReturn) {
		return nil, tlasErr(CreateTlasErrorMissingFeature,
			"flag AllowRayHitVertexReturn requires feature FeatureRayHitVertexReturn")
	}

	guard := d.snatchLock.Read()
	defer guard.Release()
	halDevice := d.Raw(guard)
	if halDevice == nil {
		return nil, ErrDeviceDestroyed
	}

	sizeInfo := halDevice.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: &hal.AccelerationStructureEntries{
			Instances: &hal.AccelerationStructureInstances{Count: desc.MaxInstances},
		},
		Flags: desc.Flags,
	})
	raw, err := halDevice.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  desc.Label,
		Size:   sizeInfo.AccelerationStructureSize,
		Format: hal.AccelerationStructureFormatTopLevel,
	})
	if err != nil {
		return nil, &CreateTlasError{Kind: CreateTlasErrorHAL, Label: desc.Label, Message: err.Error(), HALError: err}
	}

	instanceSize := uint64(d.alignments().RawTlasInstanceSize)
	if instanceSize == 0 {
		instanceSize = defaultRawTlasInstanceSize
	}
	instanceBuffer, err := halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label: "(wgpu-core) instances_buffer",
		Size:  instanceSize * uint64(max(desc.MaxInstances, 1)),
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageTlasInput,
	})
	if err != nil {
		halDevice.DestroyAccelerationStructure(raw)
		return nil, &CreateTlasError{Kind: CreateTlasErrorHAL, Label: desc.Label, Message: err.Error(), HALError: err}
	}

	tlas := &Tlas{
		raw:            NewSnatchable(raw),
		instanceBuffer: instanceBuffer,
		device:         d,
		state: raytracing.Tlas{
			Label:          desc.Label,
			SizeInfo:       sizeInfo,
			Flags:          desc.Flags,
			UpdateMode:     desc.UpdateMode,
			MaxInstances:   desc.MaxInstances,
			InstanceBuffer: instanceBuffer,
		},
	}
	tlas.Ref = NewResourceRef("Tlas:"+desc.Label, tlas.Destroy)
	return tlas, nil
}
