//go:build !(js && wasm)

package core

import (
	"log/slog"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/raytracing"
)

// tlasTransformSize is the size of a 3x4 float32 transform in a transform buffer.
const tlasTransformSize = 48

// tlasMaxCustomData is the exclusive upper bound of TlasInstance.CustomData
// (instanceCustomIndex is 24 bits).
const tlasMaxCustomData = 1 << 24

// BlasTriangleGeometry is one triangle geometry of a BLAS build.
//
// Matches Rust wgpu-core ray_tracing.rs BlasTriangleGeometry.
type BlasTriangleGeometry struct {
	// Size describes the geometry. It must be compatible with the size
	// descriptor at the same index given to CreateBlas.
	Size                  *gputypes.BlasTriangleGeometrySizeDescriptor
	VertexBuffer          *Buffer
	FirstVertex           uint32
	VertexStride          uint64
	IndexBuffer           *Buffer // optional
	FirstIndex            *uint32 // required iff IndexBuffer != nil
	TransformBuffer       *Buffer // optional, requires ASFlagUseTransform
	TransformBufferOffset *uint64 // required iff TransformBuffer != nil
}

// BlasAabbGeometry is one AABB geometry of a BLAS build.
//
// Matches Rust wgpu-core ray_tracing.rs BlasAabbGeometry.
type BlasAabbGeometry struct {
	Size            *gputypes.BlasAABBGeometrySizeDescriptor
	Stride          uint64
	AabbBuffer      *Buffer
	PrimitiveOffset uint32
}

// BlasBuildEntry pairs a BLAS with the geometry to build it from. Exactly
// one of Triangles and AABBs may be set, matching the BLAS's creation sizes.
type BlasBuildEntry struct {
	Blas      *Blas
	Triangles []BlasTriangleGeometry
	AABBs     []BlasAabbGeometry
}

// TlasInstance is one instance of a TLAS build.
type TlasInstance struct {
	Blas       *Blas
	Transform  [12]float32
	CustomData uint32
	Mask       uint8
}

// TlasPackage pairs a TLAS with its instances. Nil instances are empty slots
// and are skipped; the TLAS is built from the remaining ones in order.
//
// Matches Rust wgpu-core ray_tracing.rs TlasPackage.
type TlasPackage struct {
	Tlas      *Tlas
	Instances []*TlasInstance
}

// asAction is an acceleration structure action recorded during encoding and
// validated at submit, in recording order.
//
// Matches Rust wgpu-core ray_tracing.rs AsAction.
type asAction struct {
	build   *asBuild // non-nil for AsAction::Build
	useTlas *Tlas    // non-nil for AsAction::UseTlas
}

// asBuild records which acceleration structures one build command built.
type asBuild struct {
	blases []*Blas
	tlases []tlasBuild
}

// tlasBuild records a built TLAS and the BLASes its instances reference.
type tlasBuild struct {
	tlas         *Tlas
	dependencies []*Blas
}

type blasStore struct {
	blas                *Blas
	entries             hal.AccelerationStructureEntries
	scratchBufferOffset uint64
}

type tlasStore struct {
	tlas                *Tlas
	entries             hal.AccelerationStructureEntries
	scratchBufferOffset uint64
	// rangeStart/rangeEnd are this TLAS's bytes in the instance staging data.
	rangeStart, rangeEnd int
}

// buildState carries per-call state for BuildAccelerationStructures.
type buildState struct {
	device        *Device
	guard         SnatchGuard
	tracker       *track.EncoderBufferTracker
	inputBarriers []hal.BufferBarrier
	alignment     uint64
}

// BuildAccelerationStructures records builds of the given BLASes and then the
// given TLASes into the encoder.
//
// All BLASes are built first, then each TLAS's instances are packed into a
// staging buffer, copied into the TLAS's instance buffer, and the TLASes are
// built. One scratch buffer, sized for the larger of the two phases, is
// shared by both; it and the staging buffer are temporary resources of the
// command buffer (see CoreCommandBuffer.TakeTempBufferDestroyer).
//
// Whether the BLASes a TLAS references are built is only known at submit
// time, where the recorded build is validated (see
// Device.ValidateAccelerationStructureActions).
//
// Matches Rust wgpu-core command/ray_tracing.rs build_acceleration_structures.
func (e *CoreCommandEncoder) BuildAccelerationStructures(blasEntries []BlasBuildEntry, tlasPackages []TlasPackage) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusRecording {
		return e.statusError("build acceleration structures")
	}
	if !e.device.Features.Contains(gputypes.FeatureRayQuery) {
		return newBuildASError(BuildASErrorMissingFeature, "requires feature FeatureRayQuery")
	}

	guard := e.device.snatchLock.Read()
	defer guard.Release()
	halDevice := e.device.Raw(guard)
	if halDevice == nil {
		return ErrResourceDestroyed
	}
	raw, err := e.recordingLocked(guard)
	if err != nil {
		return err
	}
	e.cbDirty = true

	scratchAlignment := uint64(e.device.alignments().RayTracingScratchBufferAlignment)
	state := &buildState{
		device:    e.device,
		guard:     guard,
		tracker:   e.mutable.bufferTracker,
		alignment: max(scratchAlignment, 1),
	}

	build := &asBuild{}
	blasStorage, blasScratchSize, err := state.iterBlas(blasEntries, build)
	if err != nil {
		return err
	}
	tlasStorage, tlasScratchSize, staging, err := state.iterTlas(halDevice, tlasPackages, build)
	if err != nil {
		return err
	}

	scratchSize := max(blasScratchSize, tlasScratchSize)
	if scratchSize == 0 {
		// Nothing to build.
		return nil
	}
	if scratchSize == math.MaxUint64 {
		return &BuildAccelerationStructureError{Kind: BuildASErrorHAL, Message: "scratch buffer size overflows"}
	}

	scratch, err := halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label: "(wgpu internal) acceleration structure scratch buffer",
		Size:  scratchSize,
		Usage: gputypes.BufferUsageAccelerationStructureScratch,
	})
	if err != nil {
		return &BuildAccelerationStructureError{Kind: BuildASErrorHAL, Message: "create scratch buffer: " + err.Error(), HALError: err}
	}
	e.mutable.tempBuffers = append(e.mutable.tempBuffers, scratch)

	var stagingBuffer hal.Buffer
	if len(staging) > 0 {
		stagingBuffer, err = createInstanceStagingBuffer(halDevice, staging)
		if err != nil {
			return &BuildAccelerationStructureError{Kind: BuildASErrorHAL, Message: "create instance staging buffer: " + err.Error(), HALError: err}
		}
		e.mutable.tempBuffers = append(e.mutable.tempBuffers, stagingBuffer)
	}

	blasDescriptors, err := blasBuildDescriptors(blasStorage, scratch, guard)
	if err != nil {
		return err
	}
	tlasDescriptors := make([]hal.BuildAccelerationStructureDescriptor, 0, len(tlasStorage))
	for i := range tlasStorage {
		store := &tlasStorage[i]
		if store.tlas.state.UpdateMode == gputypes.AccelerationStructureUpdateModePreferUpdate {
			slog.Warn("wgpu: BuildAccelerationStructures called with PreferUpdate, but only rebuild is implemented")
		}
		dst := store.tlas.Raw(guard)
		if dst == nil {
			return newBuildASError(BuildASErrorDestroyedResource, "TLAS %q is destroyed", store.tlas.Label())
		}
		tlasDescriptors = append(tlasDescriptors, hal.BuildAccelerationStructureDescriptor{
			Entries:                          &store.entries,
			Mode:                             hal.AccelerationStructureBuildModeBuild,
			Flags:                            store.tlas.state.Flags,
			DestinationAccelerationStructure: dst,
			ScratchBuffer:                    scratch,
			ScratchBufferOffset:              store.scratchBufferOffset,
		})
	}

	recordBlasBuilds(raw, state.inputBarriers, blasDescriptors, scratch, len(tlasStorage) > 0)
	if len(tlasStorage) > 0 {
		recordTlasBuilds(raw, tlasStorage, tlasDescriptors, stagingBuffer)
	}

	e.mutable.asActions = append(e.mutable.asActions, asAction{build: build})
	return nil
}

// iterBlas validates the BLAS entries, records their input buffer usages, and
// converts them to HAL entries with scratch offsets.
//
// Matches Rust wgpu-core command/ray_tracing.rs iter_blas.
func (s *buildState) iterBlas(entries []BlasBuildEntry, build *asBuild) ([]blasStore, uint64, error) {
	storage := make([]blasStore, 0, len(entries))
	var scratchSize uint64
	for i := range entries {
		entry := &entries[i]
		blas := entry.Blas
		if blas == nil {
			return nil, 0, newBuildASError(BuildASErrorDestroyedResource, "BLAS entry %d has no BLAS", i)
		}
		if blas.Raw(s.guard) == nil {
			return nil, 0, newBuildASError(BuildASErrorDestroyedResource, "BLAS %q is destroyed", blas.Label())
		}
		build.blases = append(build.blases, blas)

		var halEntries hal.AccelerationStructureEntries
		switch {
		case entry.Triangles != nil && entry.AABBs != nil:
			return nil, 0, newBuildASError(BuildASErrorGeometryKind,
				"BLAS %q entry has both triangle and AABB geometries", blas.Label())
		case entry.AABBs != nil:
			aabbs, err := s.aabbEntries(blas, entry.AABBs)
			if err != nil {
				return nil, 0, err
			}
			halEntries.AABBs = aabbs
		default:
			triangles, err := s.triangleEntries(blas, entry.Triangles)
			if err != nil {
				return nil, 0, err
			}
			// Non-nil even when empty, so the entries are not mistaken for AABBs.
			halEntries.Triangles = triangles
		}

		storage = append(storage, blasStore{
			blas:                blas,
			entries:             halEntries,
			scratchBufferOffset: scratchSize,
		})
		scratchSize = saturatingAdd(scratchSize, alignUp(blas.state.SizeInfo.BuildScratchSize, s.alignment))
	}
	return storage, scratchSize, nil
}

func (s *buildState) triangleEntries(blas *Blas, geometries []BlasTriangleGeometry) ([]hal.AccelerationStructureTriangles, error) {
	label := blas.Label()
	if blas.sizes.AABBs != nil && len(geometries) > 0 {
		return nil, newBuildASError(BuildASErrorBlasGeometryKindMismatch,
			"BLAS %q was created with AABB geometry but built with triangles", label)
	}
	sizeDescs := blas.sizes.Triangles
	out := make([]hal.AccelerationStructureTriangles, 0, len(geometries))
	for i := range geometries {
		mesh := &geometries[i]
		if i >= len(sizeDescs) {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasBuildSizes,
				"BLAS %q build has more geometries than at creation (%d)", label, len(sizeDescs))
		}
		if mesh.Size == nil {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasBuildSizes,
				"BLAS %q geometry %d has no size descriptor", label, i)
		}
		if err := validateTriangleSize(label, &sizeDescs[i], mesh); err != nil {
			return nil, err
		}

		vertexRaw, err := s.useInputBuffer(mesh.VertexBuffer, "vertex", label)
		if err != nil {
			return nil, err
		}
		vertexEnd, ok := checkedMul(uint64(mesh.Size.VertexCount)+uint64(mesh.FirstVertex), mesh.VertexStride)
		if !ok || mesh.VertexBuffer.Size() < vertexEnd {
			return nil, newBuildASError(BuildASErrorInsufficientBufferSize,
				"vertex buffer %q (size %d) is too small for %d vertices at first vertex %d with stride %d",
				mesh.VertexBuffer.Label(), mesh.VertexBuffer.Size(), mesh.Size.VertexCount, mesh.FirstVertex, mesh.VertexStride)
		}

		tri := hal.AccelerationStructureTriangles{
			VertexBuffer: vertexRaw,
			VertexFormat: mesh.Size.VertexFormat,
			FirstVertex:  mesh.FirstVertex,
			VertexCount:  mesh.Size.VertexCount,
			VertexStride: mesh.VertexStride,
			Flags:        mesh.Size.Flags,
		}
		if mesh.IndexBuffer != nil {
			indices, err := s.triangleIndices(mesh)
			if err != nil {
				return nil, err
			}
			tri.Indices = indices
		}
		transform, err := s.triangleTransform(blas, mesh)
		if err != nil {
			return nil, err
		}
		tri.Transform = transform
		out = append(out, tri)
	}
	return out, nil
}

// validateTriangleSize checks a build geometry against its creation-time
// size descriptor.
func validateTriangleSize(label string, created *gputypes.BlasTriangleGeometrySizeDescriptor, mesh *BlasTriangleGeometry) error {
	size := mesh.Size
	if created.Flags != size.Flags {
		return newBuildASError(BuildASErrorIncompatibleBlasFlags,
			"BLAS %q geometry flags differ: creation %v, provided %v", label, created.Flags, size.Flags)
	}
	if created.VertexCount < size.VertexCount {
		return newBuildASError(BuildASErrorIncompatibleBlasVertexCount,
			"BLAS %q build vertex count %d exceeds creation count %d", label, size.VertexCount, created.VertexCount)
	}
	if created.VertexFormat != size.VertexFormat {
		return newBuildASError(BuildASErrorDifferentBlasVertexFormats,
			"BLAS %q vertex formats differ: creation %v, provided %v", label, created.VertexFormat, size.VertexFormat)
	}
	minStride, strideAlignment := blasVertexStrideRequirements(created.VertexFormat)
	if mesh.VertexStride < minStride {
		return newBuildASError(BuildASErrorVertexStrideTooSmall,
			"BLAS %q vertex stride must be at least %d, got %d", label, minStride, mesh.VertexStride)
	}
	if mesh.VertexStride%strideAlignment != 0 {
		return newBuildASError(BuildASErrorVertexStrideUnaligned,
			"BLAS %q vertex stride must be a multiple of %d, got %d", label, strideAlignment, mesh.VertexStride)
	}
	switch {
	case (created.IndexCount == nil) != (size.IndexCount == nil):
		return newBuildASError(BuildASErrorBlasIndexCountProvidedMismatch,
			"BLAS %q index count was provided at creation or build, but not both", label)
	case created.IndexCount != nil && *created.IndexCount < *size.IndexCount:
		return newBuildASError(BuildASErrorIncompatibleBlasIndexCount,
			"BLAS %q build index count %d exceeds creation count %d", label, *size.IndexCount, *created.IndexCount)
	}
	if !equalOptional(created.IndexFormat, size.IndexFormat) {
		return newBuildASError(BuildASErrorDifferentBlasIndexFormats,
			"BLAS %q index formats differ between creation and build", label)
	}
	if created.IndexCount != nil && mesh.IndexBuffer == nil {
		return newBuildASError(BuildASErrorMissingIndexBuffer,
			"BLAS %q size descriptor requires an index buffer, but none was provided", label)
	}
	return nil
}

func (s *buildState) triangleIndices(mesh *BlasTriangleGeometry) (*hal.AccelerationStructureTriangleIndices, error) {
	indexBuffer := mesh.IndexBuffer
	if mesh.FirstIndex == nil || mesh.Size.IndexCount == nil || mesh.Size.IndexFormat == nil {
		return nil, newBuildASError(BuildASErrorMissingAssociatedData,
			"index buffer %q is missing FirstIndex or the size descriptor's index data", indexBuffer.Label())
	}
	raw, err := s.useInputBuffer(indexBuffer, "index", "")
	if err != nil {
		return nil, err
	}
	stride := uint64(indexFormatSize(*mesh.Size.IndexFormat))
	count := *mesh.Size.IndexCount
	// hal.AccelerationStructureTriangleIndices takes a 32-bit offset.
	offset := uint64(*mesh.FirstIndex) * stride
	size := uint64(count) * stride
	if offset > math.MaxUint32 || size > math.MaxUint32 {
		return nil, newBuildASError(BuildASErrorOffsetLimitedTo4GB,
			"index buffer %q region (offset %d, size %d) exceeds 4 GiB", indexBuffer.Label(), offset, size)
	}
	if count%3 != 0 {
		return nil, newBuildASError(BuildASErrorInvalidIndexCount,
			"index buffer %q index count %d is not divisible by 3", indexBuffer.Label(), count)
	}
	if indexBuffer.Size() < offset || indexBuffer.Size()-offset < size {
		return nil, newBuildASError(BuildASErrorInsufficientBufferSize,
			"index buffer %q (size %d) is too small for %d bytes at offset %d",
			indexBuffer.Label(), indexBuffer.Size(), size, offset)
	}
	return &hal.AccelerationStructureTriangleIndices{
		Format: *mesh.Size.IndexFormat,
		Buffer: raw,
		Offset: uint32(offset),
		Count:  count,
	}, nil
}

func (s *buildState) triangleTransform(blas *Blas, mesh *BlasTriangleGeometry) (*hal.AccelerationStructureTriangleTransform, error) {
	useTransform := blas.state.Flags.Contains(gputypes.ASFlagUseTransform)
	transformBuffer := mesh.TransformBuffer
	if transformBuffer == nil {
		if useTransform {
			return nil, newBuildASError(BuildASErrorTransformMissing,
				"BLAS %q has flag UseTransform but the transform buffer is missing", blas.Label())
		}
		return nil, nil //nolint:nilnil // no transform is a valid result
	}
	if !useTransform {
		return nil, newBuildASError(BuildASErrorUseTransformMissing,
			"BLAS %q is missing flag UseTransform but a transform buffer is set", blas.Label())
	}
	if mesh.TransformBufferOffset == nil {
		return nil, newBuildASError(BuildASErrorMissingAssociatedData,
			"transform buffer %q is missing TransformBufferOffset", transformBuffer.Label())
	}
	raw, err := s.useInputBuffer(transformBuffer, "transform", blas.Label())
	if err != nil {
		return nil, err
	}
	offset := *mesh.TransformBufferOffset
	if offset > math.MaxUint32 {
		return nil, newBuildASError(BuildASErrorOffsetLimitedTo4GB,
			"transform buffer %q offset %d exceeds 4 GiB", transformBuffer.Label(), offset)
	}
	if offset%gputypes.TransformBufferAlignment != 0 {
		return nil, newBuildASError(BuildASErrorUnalignedTransformBufferOffset,
			"transform buffer %q offset %d is not a multiple of %d",
			transformBuffer.Label(), offset, gputypes.TransformBufferAlignment)
	}
	if transformBuffer.Size() < tlasTransformSize || transformBuffer.Size()-tlasTransformSize < offset {
		return nil, newBuildASError(BuildASErrorInsufficientBufferSize,
			"transform buffer %q (size %d) is too small for %d bytes at offset %d",
			transformBuffer.Label(), transformBuffer.Size(), tlasTransformSize, offset)
	}
	return &hal.AccelerationStructureTriangleTransform{Buffer: raw, Offset: uint32(offset)}, nil
}

func (s *buildState) aabbEntries(blas *Blas, geometries []BlasAabbGeometry) ([]hal.AccelerationStructureAABBs, error) {
	label := blas.Label()
	if blas.sizes.AABBs == nil && len(geometries) > 0 {
		return nil, newBuildASError(BuildASErrorBlasGeometryKindMismatch,
			"BLAS %q was created with triangle geometry but built with AABBs", label)
	}
	sizeDescs := blas.sizes.AABBs
	out := make([]hal.AccelerationStructureAABBs, 0, len(geometries))
	for i := range geometries {
		aabb := &geometries[i]
		if i >= len(sizeDescs) {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasBuildSizes,
				"BLAS %q build has more geometries than at creation (%d)", label, len(sizeDescs))
		}
		if aabb.Size == nil {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasBuildSizes,
				"BLAS %q geometry %d has no size descriptor", label, i)
		}
		created := &sizeDescs[i]
		if created.Flags != aabb.Size.Flags {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasFlags,
				"BLAS %q geometry flags differ: creation %v, provided %v", label, created.Flags, aabb.Size.Flags)
		}
		if created.PrimitiveCount < aabb.Size.PrimitiveCount {
			return nil, newBuildASError(BuildASErrorIncompatibleBlasAabbPrimitiveCount,
				"BLAS %q build AABB count %d exceeds creation count %d", label, aabb.Size.PrimitiveCount, created.PrimitiveCount)
		}
		if aabb.PrimitiveOffset%8 != 0 {
			return nil, newBuildASError(BuildASErrorUnalignedAabbPrimitiveOffset,
				"BLAS %q AABB primitive offset must be a multiple of 8", label)
		}
		if aabb.Stride < gputypes.AABBGeometryMinStride || aabb.Stride%8 != 0 {
			return nil, newBuildASError(BuildASErrorInvalidAabbStride,
				"BLAS %q AABB stride %d is invalid (must be >= %d and a multiple of 8)",
				label, aabb.Stride, gputypes.AABBGeometryMinStride)
		}

		raw, err := s.useInputBuffer(aabb.AabbBuffer, "AABB", label)
		if err != nil {
			return nil, err
		}
		size, ok := checkedMul(uint64(aabb.Size.PrimitiveCount), aabb.Stride)
		if !ok || size > math.MaxUint32 {
			return nil, newBuildASError(BuildASErrorOffsetLimitedTo4GB,
				"AABB buffer %q region exceeds 4 GiB", aabb.AabbBuffer.Label())
		}
		offset := uint64(aabb.PrimitiveOffset)
		if aabb.AabbBuffer.Size() < offset || aabb.AabbBuffer.Size()-offset < size {
			return nil, newBuildASError(BuildASErrorInsufficientBufferSize,
				"AABB buffer %q (size %d) is too small for %d bytes at offset %d",
				aabb.AabbBuffer.Label(), aabb.AabbBuffer.Size(), size, offset)
		}
		out = append(out, hal.AccelerationStructureAABBs{
			Buffer: raw,
			Offset: aabb.PrimitiveOffset,
			Count:  aabb.Size.PrimitiveCount,
			Stride: aabb.Stride,
			Flags:  aabb.Size.Flags,
		})
	}
	return out, nil
}

// useInputBuffer checks that buffer is alive and has BLAS_INPUT usage,
// records the BLAS input usage in the encoder's tracker (queueing a barrier
// if the state changes), and returns the HAL buffer.
func (s *buildState) useInputBuffer(buffer *Buffer, role, blasLabel string) (hal.Buffer, error) {
	if buffer == nil {
		return nil, newBuildASError(BuildASErrorMissingAssociatedData, "BLAS %q geometry has no %s buffer", blasLabel, role)
	}
	raw := buffer.Raw(s.guard)
	if raw == nil {
		return nil, newBuildASError(BuildASErrorDestroyedResource, "%s buffer %q is destroyed", role, buffer.Label())
	}
	if !buffer.Usage().Contains(gputypes.BufferUsageBlasInput) {
		return nil, newBuildASError(BuildASErrorMissingBufferUsage,
			"%s buffer %q is missing usage BufferUsageBlasInput", role, buffer.Label())
	}
	if s.tracker != nil {
		if td := buffer.TrackingData(); td != nil && td.Index().IsValid() {
			if transition, needed := s.tracker.SetSingle(td.Index(), track.BufferUsesBlasInput); needed {
				s.inputBarriers = append(s.inputBarriers, transition.IntoHAL(raw))
			}
		}
	}
	return raw, nil
}

// iterTlas validates the TLAS packages, packs their instances into one
// staging blob, and records the BLAS dependencies of each TLAS.
func (s *buildState) iterTlas(
	halDevice hal.Device,
	packages []TlasPackage,
	build *asBuild,
) ([]tlasStore, uint64, []byte, error) {
	storage := make([]tlasStore, 0, len(packages))
	var staging []byte
	var scratchSize uint64
	for i := range packages {
		pkg := &packages[i]
		tlas := pkg.Tlas
		if tlas == nil {
			return nil, 0, nil, newBuildASError(BuildASErrorDestroyedResource, "TLAS package %d has no TLAS", i)
		}
		if tlas.Raw(s.guard) == nil {
			return nil, 0, nil, newBuildASError(BuildASErrorDestroyedResource, "TLAS %q is destroyed", tlas.Label())
		}

		scratchOffset := scratchSize
		scratchSize = saturatingAdd(scratchSize, alignUp(tlas.state.SizeInfo.BuildScratchSize, s.alignment))

		firstByte := len(staging)
		var dependencies []*Blas
		seen := make(map[*Blas]struct{})
		var instanceCount uint32
		for _, instance := range pkg.Instances {
			if instance == nil {
				continue
			}
			if instance.CustomData >= tlasMaxCustomData {
				return nil, 0, nil, newBuildASError(BuildASErrorTlasInvalidCustomIndex,
					"TLAS %q instance custom data %d does not fit in 24 bits", tlas.Label(), instance.CustomData)
			}
			blas := instance.Blas
			if blas == nil || blas.Raw(s.guard) == nil {
				return nil, 0, nil, newBuildASError(BuildASErrorDestroyedResource,
					"TLAS %q instance references a destroyed BLAS", tlas.Label())
			}
			if tlas.state.Flags.Contains(gputypes.ASFlagAllowRayHitVertexReturn) &&
				!blas.state.Flags.Contains(gputypes.ASFlagAllowRayHitVertexReturn) {
				return nil, 0, nil, newBuildASError(BuildASErrorTlasDependentMissingVertexReturn,
					"TLAS %q has flag AllowRayHitVertexReturn but its BLAS %q does not", tlas.Label(), blas.Label())
			}
			staging = append(staging, halDevice.TlasInstanceToBytes(hal.TlasInstance{
				Transform:   instance.Transform,
				CustomData:  instance.CustomData,
				Mask:        instance.Mask,
				BlasAddress: blas.state.Handle,
			})...)
			instanceCount++
			if _, ok := seen[blas]; !ok {
				seen[blas] = struct{}{}
				dependencies = append(dependencies, blas)
			}
		}

		build.tlases = append(build.tlases, tlasBuild{tlas: tlas, dependencies: dependencies})

		if instanceCount > tlas.state.MaxInstances {
			return nil, 0, nil, newBuildASError(BuildASErrorTlasInstanceCountExceeded,
				"TLAS %q has %d instances, but its maximum is %d", tlas.Label(), instanceCount, tlas.state.MaxInstances)
		}

		storage = append(storage, tlasStore{
			tlas: tlas,
			entries: hal.AccelerationStructureEntries{
				Instances: &hal.AccelerationStructureInstances{Buffer: tlas.instanceBuffer, Count: instanceCount},
			},
			scratchBufferOffset: scratchOffset,
			rangeStart:          firstByte,
			rangeEnd:            len(staging),
		})
	}
	return storage, scratchSize, staging, nil
}

// blasBuildDescriptors converts the stored BLAS entries to HAL build
// descriptors.
//
// Matches Rust wgpu-core command/ray_tracing.rs map_blas.
func blasBuildDescriptors(storage []blasStore, scratch hal.Buffer, guard SnatchGuard) ([]hal.BuildAccelerationStructureDescriptor, error) {
	descriptors := make([]hal.BuildAccelerationStructureDescriptor, 0, len(storage))
	for i := range storage {
		store := &storage[i]
		blas := store.blas
		if blas.state.UpdateMode == gputypes.AccelerationStructureUpdateModePreferUpdate {
			slog.Debug("wgpu: BuildAccelerationStructures: only rebuild is implemented")
		}
		dst := blas.Raw(guard)
		if dst == nil {
			return nil, newBuildASError(BuildASErrorDestroyedResource, "BLAS %q is destroyed", blas.Label())
		}
		if blas.CompactionState() == raytracing.CompactionCompacted {
			return nil, newBuildASError(BuildASErrorCompactedBlas, "BLAS %q is compacted and so cannot be built", blas.Label())
		}
		descriptors = append(descriptors, hal.BuildAccelerationStructureDescriptor{
			Entries:                          &store.entries,
			Mode:                             hal.AccelerationStructureBuildModeBuild,
			Flags:                            blas.state.Flags,
			DestinationAccelerationStructure: dst,
			ScratchBuffer:                    scratch,
			ScratchBufferOffset:              store.scratchBufferOffset,
		})
	}
	return descriptors, nil
}

// recordBlasBuilds records the BLAS input barriers and builds, and the
// barrier that orders them before the TLAS builds (or before later use).
//
// Matches Rust wgpu-core command/ray_tracing.rs build_blas (without the
// compaction size queries, which are not implemented yet).
func recordBlasBuilds(
	raw hal.CommandEncoder,
	inputBarriers []hal.BufferBarrier,
	descriptors []hal.BuildAccelerationStructureDescriptor,
	scratch hal.Buffer,
	tlasPresent bool,
) {
	if len(inputBarriers) > 0 {
		raw.TransitionBuffers(inputBarriers)
	}
	blasPresent := len(descriptors) > 0
	if blasPresent {
		raw.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{
			Usage: hal.AccelerationStructureUsageTransition{
				OldUsage: hal.AccelerationStructureUsesBuildInput,
				NewUsage: hal.AccelerationStructureUsesBuildOutput,
			},
		})
		raw.BuildAccelerationStructures(descriptors)
	}
	if blasPresent && tlasPresent {
		// The TLAS builds reuse the scratch buffer.
		scratchUsage := gputypes.BufferUsageAccelerationStructureScratch
		raw.TransitionBuffers([]hal.BufferBarrier{{
			Buffer: scratch,
			Usage:  hal.BufferUsageTransition{OldUsage: scratchUsage, NewUsage: scratchUsage},
		}})
	}

	var source, destination hal.AccelerationStructureUses
	if blasPresent {
		source |= hal.AccelerationStructureUsesBuildOutput
		destination |= hal.AccelerationStructureUsesBuildInput
	}
	if tlasPresent {
		source |= hal.AccelerationStructureUsesShaderInput
		destination |= hal.AccelerationStructureUsesBuildOutput
	}
	raw.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{
		Usage: hal.AccelerationStructureUsageTransition{OldUsage: source, NewUsage: destination},
	})
}

// recordTlasBuilds copies the packed instances into each TLAS's instance
// buffer, builds the TLASes, and makes them visible to shaders.
func recordTlasBuilds(
	raw hal.CommandEncoder,
	storage []tlasStore,
	descriptors []hal.BuildAccelerationStructureDescriptor,
	staging hal.Buffer,
) {
	if staging != nil {
		raw.TransitionBuffers([]hal.BufferBarrier{{
			Buffer: staging,
			Usage: hal.BufferUsageTransition{
				OldUsage: gputypes.BufferUsageMapWrite,
				NewUsage: gputypes.BufferUsageCopySrc,
			},
		}})
	}

	var instanceBarriers []hal.BufferBarrier
	for i := range storage {
		store := &storage[i]
		size := store.rangeEnd - store.rangeStart
		if size == 0 {
			continue
		}
		instanceBuffer := store.tlas.instanceBuffer
		instanceBarriers = append(instanceBarriers, hal.BufferBarrier{
			Buffer: instanceBuffer,
			Usage: hal.BufferUsageTransition{
				OldUsage: gputypes.BufferUsageCopyDst,
				NewUsage: gputypes.BufferUsageTlasInput,
			},
		})
		raw.TransitionBuffers([]hal.BufferBarrier{{
			Buffer: instanceBuffer,
			Usage: hal.BufferUsageTransition{
				OldUsage: gputypes.BufferUsageTlasInput,
				NewUsage: gputypes.BufferUsageCopyDst,
			},
		}})
		raw.CopyBufferToBuffer(staging, instanceBuffer, []hal.BufferCopy{{
			SrcOffset: uint64(store.rangeStart), //nolint:gosec // non-negative slice offset
			Size:      uint64(size),             //nolint:gosec // non-negative slice length
		}})
	}
	if len(instanceBarriers) > 0 {
		raw.TransitionBuffers(instanceBarriers)
	}

	raw.BuildAccelerationStructures(descriptors)
	raw.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{
		Usage: hal.AccelerationStructureUsageTransition{
			OldUsage: hal.AccelerationStructureUsesBuildOutput,
			NewUsage: hal.AccelerationStructureUsesShaderInput,
		},
	})
}

// createInstanceStagingBuffer creates a host-visible buffer holding data.
func createInstanceStagingBuffer(halDevice hal.Device, data []byte) (hal.Buffer, error) {
	size := alignUp(uint64(len(data)), 4)
	buffer, err := halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label:            "(wgpu internal) TLAS instance staging buffer",
		Size:             size,
		Usage:            gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		return nil, err
	}
	mapping, err := halDevice.MapBuffer(buffer, 0, size)
	if err != nil {
		halDevice.DestroyBuffer(buffer)
		return nil, err
	}
	copy(unsafe.Slice((*byte)(mapping.Ptr), len(data)), data) //nolint:gosec // mapping covers size >= len(data) bytes
	if err := halDevice.UnmapBuffer(buffer); err != nil {
		halDevice.DestroyBuffer(buffer)
		return nil, err
	}
	return buffer, nil
}

// blasVertexStrideRequirements returns the minimum vertex stride and the
// stride alignment for a BLAS vertex format.
//
// Matches Rust wgpu-types VertexFormat::min_acceleration_structure_vertex_stride
// and acceleration_structure_stride_alignment.
func blasVertexStrideRequirements(format gputypes.VertexFormat) (minStride, alignment uint64) {
	switch format {
	case gputypes.VertexFormatFloat16x2, gputypes.VertexFormatSnorm16x2:
		return 4, 2
	case gputypes.VertexFormatFloat16x4, gputypes.VertexFormatSnorm16x4:
		return 6, 2 // Only the first 3 components are read.
	case gputypes.VertexFormatFloat32x2:
		return 8, 4
	default: // Float32x3
		return 12, 4
	}
}

func indexFormatSize(format gputypes.IndexFormat) uint32 {
	if format == gputypes.IndexFormatUint16 {
		return 2
	}
	return 4
}

func equalOptional[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func alignUp(n, alignment uint64) uint64 {
	if alignment <= 1 {
		return n
	}
	if rem := n % alignment; rem != 0 {
		return saturatingAdd(n, alignment-rem)
	}
	return n
}

func saturatingAdd(a, b uint64) uint64 {
	if a > math.MaxUint64-b {
		return math.MaxUint64
	}
	return a + b
}

func checkedMul(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, false
	}
	return a * b, true
}
