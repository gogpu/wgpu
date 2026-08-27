// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// AccelerationStructure implements hal.AccelerationStructure for Metal.
//
// Wraps an id<MTLAccelerationStructure> created via
// [MTLDevice newAccelerationStructureWithSize:].
//
// Reference: Rust wgpu-hal metal/mod.rs:1397 (AccelerationStructure { raw }).
type AccelerationStructure struct {
	raw    ID // id<MTLAccelerationStructure>
	device *Device
}

var _ hal.AccelerationStructure = (*AccelerationStructure)(nil)

// Destroy releases the acceleration structure.
func (as *AccelerationStructure) Destroy() {
	if as != nil && as.raw != 0 {
		Release(as.raw)
		as.raw = 0
	}
}

// NativeHandle returns the raw MTLAccelerationStructure handle.
func (as *AccelerationStructure) NativeHandle() uintptr {
	if as == nil {
		return 0
	}
	return uintptr(as.raw)
}

// --------------------------------------------------------------------------
// MTLAccelerationStructureUsage constants
// --------------------------------------------------------------------------

// MTLAccelerationStructureUsage describes how the AS will be used.
type MTLAccelerationStructureUsage NSUInteger

const (
	MTLAccelerationStructureUsageNone            MTLAccelerationStructureUsage = 0
	MTLAccelerationStructureUsageRefit           MTLAccelerationStructureUsage = 1 << 0
	MTLAccelerationStructureUsagePreferFastBuild MTLAccelerationStructureUsage = 1 << 1
	MTLAccelerationStructureUsageExtendedLimits  MTLAccelerationStructureUsage = 1 << 2
)

// MTLAccelerationStructureInstanceDescriptorType identifies instance layout.
type MTLAccelerationStructureInstanceDescriptorType NSUInteger

const (
	MTLAccelerationStructureInstanceDescriptorTypeDefault  MTLAccelerationStructureInstanceDescriptorType = 0
	MTLAccelerationStructureInstanceDescriptorTypeUserID   MTLAccelerationStructureInstanceDescriptorType = 1
	MTLAccelerationStructureInstanceDescriptorTypeMotion   MTLAccelerationStructureInstanceDescriptorType = 2
	MTLAccelerationStructureInstanceDescriptorTypeIndirect MTLAccelerationStructureInstanceDescriptorType = 3
)

// --------------------------------------------------------------------------
// MTLAccelerationStructureInstanceDescriptor (64 bytes)
// --------------------------------------------------------------------------
//
// Metal defines this struct for indirect TLAS instances:
//
//	struct MTLAccelerationStructureInstanceDescriptor {
//	    MTLPackedFloat4x3 transformationMatrix;  // 48 bytes
//	    uint32_t          options;               // 4 bytes (MTLAccelerationStructureInstanceOptions)
//	    uint32_t          mask;                  // 4 bytes
//	    uint32_t          intersectionFunctionTableOffset; // 4 bytes
//	    uint32_t          accelerationStructureIndex;      // 4 bytes
//	};
//
// Total = 64 bytes.
//
// Rust wgpu uses MTLIndirectAccelerationStructureInstanceDescriptor with
// gpuResourceID and userID fields. The indirect variant is 112 bytes. We use
// the standard 64-byte layout matching the non-indirect descriptor. The core
// layer selects the appropriate layout via RawTlasInstanceSize.

const mtlASInstanceDescriptorSize = 64

// --------------------------------------------------------------------------
// Build sizes query
// --------------------------------------------------------------------------

// getAccelerationStructureBuildSizes queries Metal for the acceleration structure
// sizes needed to build from the given descriptor.
//
// Creates a transient MTLAccelerationStructureDescriptor, calls
// [MTLDevice accelerationStructureSizesWithDescriptor:], and returns the
// three sizes. The descriptor is released after the query.
//
// Reference: Rust wgpu-hal metal/device.rs:2076-2091.
func (d *Device) getAccelerationStructureBuildSizes(desc *hal.GetAccelerationStructureBuildSizesDescriptor) hal.AccelerationStructureBuildSizes {
	if desc == nil || desc.Entries == nil {
		return hal.AccelerationStructureBuildSizes{}
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	mtlDesc := mapAccelerationStructureDescriptor(desc.Entries, desc.Flags)
	if mtlDesc == 0 {
		return hal.AccelerationStructureBuildSizes{}
	}
	defer Release(mtlDesc)

	// MTLAccelerationStructureSizes accelerationStructureSizesWithDescriptor:
	// Returns a struct { NSUInteger accelerationStructureSize;
	//                     NSUInteger buildScratchBufferSize;
	//                     NSUInteger refitScratchBufferSize; }
	// which is 3 x uint (24 bytes on 64-bit).
	var sizes [3]uint64
	_ = msgSend(d.raw, Sel("accelerationStructureSizesWithDescriptor:"),
		mtlASSizesType, unsafe.Pointer(&sizes),
		argPointer(uintptr(mtlDesc)),
	)

	return hal.AccelerationStructureBuildSizes{
		AccelerationStructureSize: sizes[0],
		BuildScratchSize:          sizes[1],
		UpdateScratchSize:         sizes[2],
	}
}

// mtlASSizesType describes MTLAccelerationStructureSizes =
// { NSUInteger, NSUInteger, NSUInteger }.
var mtlASSizesType = mtlSizeType // same layout: 3 x uint64

// --------------------------------------------------------------------------
// Acceleration structure descriptor mapping
// --------------------------------------------------------------------------

// mapAccelerationStructureDescriptor converts HAL entries + flags into a
// transient Metal descriptor suitable for size queries and builds.
//
// The caller owns the returned ID and must Release it.
//
// Reference: Rust wgpu-hal metal/conv.rs:379-494.
func mapAccelerationStructureDescriptor(entries *hal.AccelerationStructureEntries, flags gputypes.AccelerationStructureFlags) ID {
	var mtlDesc ID

	switch {
	case entries.Instances != nil:
		mtlDesc = mapInstanceDescriptor(entries.Instances)
	case len(entries.Triangles) > 0:
		mtlDesc = mapTrianglesDescriptor(entries.Triangles)
	case len(entries.AABBs) > 0:
		mtlDesc = mapAABBsDescriptor(entries.AABBs)
	default:
		return 0
	}

	if mtlDesc == 0 {
		return 0
	}

	// Map build flags → MTLAccelerationStructureUsage.
	var usage MTLAccelerationStructureUsage
	if flags&gputypes.ASFlagAllowUpdate != 0 {
		usage |= MTLAccelerationStructureUsageRefit
	}
	if flags&gputypes.ASFlagPreferFastBuild != 0 {
		usage |= MTLAccelerationStructureUsagePreferFastBuild
	}
	_ = MsgSend(mtlDesc, Sel("setUsage:"), uintptr(usage))

	return mtlDesc
}

// mapInstanceDescriptor creates an MTLInstanceAccelerationStructureDescriptor.
func mapInstanceDescriptor(instances *hal.AccelerationStructureInstances) ID {
	cls := GetClass("MTLInstanceAccelerationStructureDescriptor")
	if cls == 0 {
		return 0
	}
	desc := MsgSend(ID(cls), Sel("descriptor"))
	if desc == 0 {
		return 0
	}
	Retain(desc)

	// Use indirect instance layout (matches Rust wgpu).
	_ = MsgSend(desc, Sel("setInstanceDescriptorType:"),
		uintptr(MTLAccelerationStructureInstanceDescriptorTypeDefault))
	_ = MsgSend(desc, Sel("setInstanceCount:"), uintptr(instances.Count))

	if instances.Buffer != nil {
		buf, ok := instances.Buffer.(*Buffer)
		if ok && buf != nil {
			_ = MsgSend(desc, Sel("setInstanceDescriptorBuffer:"), uintptr(buf.raw))
			_ = MsgSend(desc, Sel("setInstanceDescriptorBufferOffset:"), uintptr(instances.Offset))
		}
	}

	return desc
}

// mapTrianglesDescriptor creates an MTLPrimitiveAccelerationStructureDescriptor
// for triangle geometry.
func mapTrianglesDescriptor(triangles []hal.AccelerationStructureTriangles) ID {
	// Build NSMutableArray of geometry descriptors.
	arrCls := GetClass("NSMutableArray")
	if arrCls == 0 {
		return 0
	}
	arr := MsgSend(MsgSend(ID(arrCls), Sel("alloc")), Sel("initWithCapacity:"), uintptr(len(triangles)))
	if arr == 0 {
		return 0
	}
	defer Release(arr)

	for _, tri := range triangles {
		geomCls := GetClass("MTLAccelerationStructureTriangleGeometryDescriptor")
		if geomCls == 0 {
			return 0
		}
		geomDesc := MsgSend(ID(geomCls), Sel("descriptor"))
		if geomDesc == 0 {
			return 0
		}

		// Vertex buffer
		if tri.VertexBuffer != nil {
			if buf, ok := tri.VertexBuffer.(*Buffer); ok && buf != nil {
				_ = MsgSend(geomDesc, Sel("setVertexBuffer:"), uintptr(buf.raw))
				vertexOffset := uint64(tri.FirstVertex) * tri.VertexStride
				_ = MsgSend(geomDesc, Sel("setVertexBufferOffset:"), uintptr(vertexOffset))
			}
		}
		_ = MsgSend(geomDesc, Sel("setVertexStride:"), uintptr(tri.VertexStride))

		// Index buffer
		if tri.Indices != nil {
			if buf, ok := tri.Indices.Buffer.(*Buffer); ok && buf != nil {
				_ = MsgSend(geomDesc, Sel("setIndexBuffer:"), uintptr(buf.raw))
				_ = MsgSend(geomDesc, Sel("setIndexBufferOffset:"), uintptr(tri.Indices.Offset))
				_ = MsgSend(geomDesc, Sel("setIndexType:"), uintptr(indexFormatToMTL(tri.Indices.Format)))
				_ = MsgSend(geomDesc, Sel("setTriangleCount:"), uintptr(tri.Indices.Count/3))
			}
		} else {
			_ = MsgSend(geomDesc, Sel("setTriangleCount:"), uintptr(tri.VertexCount/3))
		}

		// Transform buffer
		if tri.Transform != nil {
			if buf, ok := tri.Transform.Buffer.(*Buffer); ok && buf != nil {
				_ = MsgSend(geomDesc, Sel("setTransformationMatrixBuffer:"), uintptr(buf.raw))
				_ = MsgSend(geomDesc, Sel("setTransformationMatrixBufferOffset:"), uintptr(tri.Transform.Offset))
			}
		}

		// Geometry flags
		if tri.Flags.Contains(gputypes.ASGeometryFlagOpaque) {
			msgSendVoid(geomDesc, Sel("setOpaque:"), argBool(true))
		}
		if !tri.Flags.Contains(gputypes.ASGeometryFlagNoDuplicateAnyHitInvocation) {
			_ = MsgSend(geomDesc, Sel("setAllowDuplicateIntersectionFunctionInvocation:"), uintptr(1))
		}

		_ = MsgSend(arr, Sel("addObject:"), uintptr(geomDesc))
	}

	// Create primitive descriptor with geometry array.
	primCls := GetClass("MTLPrimitiveAccelerationStructureDescriptor")
	if primCls == 0 {
		return 0
	}
	primDesc := MsgSend(ID(primCls), Sel("descriptor"))
	if primDesc == 0 {
		return 0
	}
	Retain(primDesc)
	_ = MsgSend(primDesc, Sel("setGeometryDescriptors:"), uintptr(arr))

	return primDesc
}

// mapAABBsDescriptor creates an MTLPrimitiveAccelerationStructureDescriptor
// for AABB geometry.
func mapAABBsDescriptor(aabbs []hal.AccelerationStructureAABBs) ID {
	arrCls := GetClass("NSMutableArray")
	if arrCls == 0 {
		return 0
	}
	arr := MsgSend(MsgSend(ID(arrCls), Sel("alloc")), Sel("initWithCapacity:"), uintptr(len(aabbs)))
	if arr == 0 {
		return 0
	}
	defer Release(arr)

	for _, aabb := range aabbs {
		geomCls := GetClass("MTLAccelerationStructureBoundingBoxGeometryDescriptor")
		if geomCls == 0 {
			return 0
		}
		geomDesc := MsgSend(ID(geomCls), Sel("descriptor"))
		if geomDesc == 0 {
			return 0
		}

		if aabb.Buffer != nil {
			if buf, ok := aabb.Buffer.(*Buffer); ok && buf != nil {
				_ = MsgSend(geomDesc, Sel("setBoundingBoxBuffer:"), uintptr(buf.raw))
				_ = MsgSend(geomDesc, Sel("setBoundingBoxBufferOffset:"), uintptr(aabb.Offset))
			}
		}
		_ = MsgSend(geomDesc, Sel("setBoundingBoxCount:"), uintptr(aabb.Count))
		_ = MsgSend(geomDesc, Sel("setBoundingBoxStride:"), uintptr(aabb.Stride))

		if aabb.Flags.Contains(gputypes.ASGeometryFlagOpaque) {
			msgSendVoid(geomDesc, Sel("setOpaque:"), argBool(true))
		}
		if !aabb.Flags.Contains(gputypes.ASGeometryFlagNoDuplicateAnyHitInvocation) {
			_ = MsgSend(geomDesc, Sel("setAllowDuplicateIntersectionFunctionInvocation:"), uintptr(1))
		}

		_ = MsgSend(arr, Sel("addObject:"), uintptr(geomDesc))
	}

	primCls := GetClass("MTLPrimitiveAccelerationStructureDescriptor")
	if primCls == 0 {
		return 0
	}
	primDesc := MsgSend(ID(primCls), Sel("descriptor"))
	if primDesc == 0 {
		return 0
	}
	Retain(primDesc)
	_ = MsgSend(primDesc, Sel("setGeometryDescriptors:"), uintptr(arr))

	return primDesc
}

// --------------------------------------------------------------------------
// TlasInstance → backend-packed bytes (64 bytes)
// --------------------------------------------------------------------------

// tlasInstanceToBytes converts a HAL TlasInstance to Metal's 64-byte
// MTLAccelerationStructureInstanceDescriptor layout.
//
// Layout (little-endian, tightly packed):
//
//	[0..48)   MTLPackedFloat4x3 transformationMatrix (column-major)
//	[48..52)  uint32 options  (MTLAccelerationStructureInstanceOptions, always 0 for now)
//	[52..56)  uint32 mask
//	[56..60)  uint32 intersectionFunctionTableOffset
//	[60..64)  uint32 accelerationStructureIndex
//
// The transform is a 3x4 matrix stored column-major in 4 packed float3 columns.
// HAL TlasInstance.Transform is row-major [m00,m01,m02,m03, m10,m11,m12,m13, m20,m21,m22,m23].
// Metal expects columns [m00,m10,m20], [m01,m11,m21], [m02,m12,m22], [m03,m13,m23].
//
// Reference: Rust wgpu-hal metal/device.rs:2123-2159.
func tlasInstanceToBytes(instance hal.TlasInstance) []byte {
	buf := make([]byte, mtlASInstanceDescriptorSize)

	// Column 0: [m00, m10, m20]
	binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(instance.Transform[0]))
	binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(instance.Transform[4]))
	binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(instance.Transform[8]))
	// Column 1: [m01, m11, m21]
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(instance.Transform[1]))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(instance.Transform[5]))
	binary.LittleEndian.PutUint32(buf[20:24], math.Float32bits(instance.Transform[9]))
	// Column 2: [m02, m12, m22]
	binary.LittleEndian.PutUint32(buf[24:28], math.Float32bits(instance.Transform[2]))
	binary.LittleEndian.PutUint32(buf[28:32], math.Float32bits(instance.Transform[6]))
	binary.LittleEndian.PutUint32(buf[32:36], math.Float32bits(instance.Transform[10]))
	// Column 3: [m03, m13, m23]
	binary.LittleEndian.PutUint32(buf[36:40], math.Float32bits(instance.Transform[3]))
	binary.LittleEndian.PutUint32(buf[40:44], math.Float32bits(instance.Transform[7]))
	binary.LittleEndian.PutUint32(buf[44:48], math.Float32bits(instance.Transform[11]))

	// options = 0 (MTLAccelerationStructureInstanceOptionNone)
	binary.LittleEndian.PutUint32(buf[48:52], 0)

	// mask
	binary.LittleEndian.PutUint32(buf[52:56], uint32(instance.Mask))

	// intersectionFunctionTableOffset = ShaderBindingTableRecordOffset
	binary.LittleEndian.PutUint32(buf[56:60], instance.ShaderBindingTableRecordOffset)

	// accelerationStructureIndex — derived from BlasAddress. Metal uses an index
	// into the instance descriptor's acceleration structure list, not a raw GPU
	// address. The core layer encodes the BLAS index in BlasAddress.
	binary.LittleEndian.PutUint32(buf[60:64], uint32(instance.BlasAddress))

	return buf
}

// --------------------------------------------------------------------------
// Command encoder: build, copy, barrier, compact-size
// --------------------------------------------------------------------------

// enterAccelerationStructureEncoder creates or returns the active
// MTLAccelerationStructureCommandEncoder for the current command buffer.
//
// Metal command buffers support one active encoder at a time. This method
// ends any existing blit encoder before creating the AS encoder, matching
// the Rust pattern in enter_acceleration_structure_builder.
//
// Reference: Rust wgpu-hal metal/command.rs:264-281.
func (e *CommandEncoder) enterAccelerationStructureEncoder() ID {
	if e.cmdBuffer == 0 {
		return 0
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	encoder := MsgSend(e.cmdBuffer, Sel("accelerationStructureCommandEncoder"))
	if encoder == 0 {
		return 0
	}
	Retain(encoder)
	return encoder
}

// buildAccelerationStructures builds one or more acceleration structures.
//
// Creates a transient MTLAccelerationStructureCommandEncoder and issues either
// buildAccelerationStructure:descriptor:scratchBuffer:scratchBufferOffset: (full build)
// or refitAccelerationStructure:descriptor:destination:scratchBuffer:scratchBufferOffset:
// (incremental update) for each descriptor.
//
// The encoder is ended and released after all builds complete.
//
// Reference: Rust wgpu-hal metal/command.rs:1840-1879.
func (e *CommandEncoder) buildAccelerationStructures(descriptors []hal.BuildAccelerationStructureDescriptor) {
	if len(descriptors) == 0 {
		return
	}

	encoder := e.enterAccelerationStructureEncoder()
	if encoder == 0 {
		return
	}
	defer func() {
		_ = MsgSend(encoder, Sel("endEncoding"))
		Release(encoder)
	}()

	pool := NewAutoreleasePool()
	defer pool.Drain()

	for i := range descriptors {
		desc := &descriptors[i]
		if desc.Entries == nil || desc.DestinationAccelerationStructure == nil {
			continue
		}

		dstAS, ok := desc.DestinationAccelerationStructure.(*AccelerationStructure)
		if !ok || dstAS == nil || dstAS.raw == 0 {
			continue
		}

		mtlDesc := mapAccelerationStructureDescriptor(desc.Entries, desc.Flags)
		if mtlDesc == 0 {
			continue
		}

		var scratchBufRaw uintptr
		var scratchOffset uintptr
		if desc.ScratchBuffer != nil {
			if sb, ok := desc.ScratchBuffer.(*Buffer); ok && sb != nil {
				scratchBufRaw = uintptr(sb.raw)
				scratchOffset = uintptr(desc.ScratchBufferOffset)
			}
		}

		switch desc.Mode {
		case hal.AccelerationStructureBuildModeBuild:
			// buildAccelerationStructure:descriptor:scratchBuffer:scratchBufferOffset:
			_ = MsgSend(encoder,
				Sel("buildAccelerationStructure:descriptor:scratchBuffer:scratchBufferOffset:"),
				uintptr(dstAS.raw), uintptr(mtlDesc), scratchBufRaw, scratchOffset,
			)

		case hal.AccelerationStructureBuildModeUpdate:
			// refitAccelerationStructure:descriptor:destination:scratchBuffer:scratchBufferOffset:
			var srcRaw uintptr
			if desc.SourceAccelerationStructure != nil {
				if srcAS, ok := desc.SourceAccelerationStructure.(*AccelerationStructure); ok && srcAS != nil {
					srcRaw = uintptr(srcAS.raw)
				}
			}
			_ = MsgSend(encoder,
				Sel("refitAccelerationStructure:descriptor:destination:scratchBuffer:scratchBufferOffset:"),
				srcRaw, uintptr(mtlDesc), uintptr(dstAS.raw), scratchBufRaw, scratchOffset,
			)
		}

		Release(mtlDesc)
	}
}

// copyAccelerationStructure copies or compacts an acceleration structure.
//
// Creates a transient MTLAccelerationStructureCommandEncoder and calls either
// copyAccelerationStructure:toAccelerationStructure: (clone) or
// copyAndCompactAccelerationStructure:toAccelerationStructure: (compact).
//
// Reference: Rust wgpu-hal metal/command.rs:746-764.
func (e *CommandEncoder) copyAccelerationStructure(src, dst hal.AccelerationStructure, copyMode gputypes.AccelerationStructureCopyMode) {
	srcAS, ok := src.(*AccelerationStructure)
	if !ok || srcAS == nil || srcAS.raw == 0 {
		return
	}
	dstAS, ok := dst.(*AccelerationStructure)
	if !ok || dstAS == nil || dstAS.raw == 0 {
		return
	}

	encoder := e.enterAccelerationStructureEncoder()
	if encoder == 0 {
		return
	}
	defer func() {
		_ = MsgSend(encoder, Sel("endEncoding"))
		Release(encoder)
	}()

	switch copyMode {
	case gputypes.AccelerationStructureCopyModeClone:
		_ = MsgSend(encoder,
			Sel("copyAccelerationStructure:toAccelerationStructure:"),
			uintptr(srcAS.raw), uintptr(dstAS.raw),
		)
	case gputypes.AccelerationStructureCopyModeCompact:
		_ = MsgSend(encoder,
			Sel("copyAndCompactAccelerationStructure:toAccelerationStructure:"),
			uintptr(srcAS.raw), uintptr(dstAS.raw),
		)
	}
}

// readAccelerationStructureCompactSize writes the post-compaction size
// of an acceleration structure into a buffer.
//
// Creates a transient MTLAccelerationStructureCommandEncoder and calls
// writeCompactedAccelerationStructureSize:toBuffer:offset:.
//
// Reference: Rust wgpu-hal metal/command.rs:1887-1898.
func (e *CommandEncoder) readAccelerationStructureCompactSize(accelStruct hal.AccelerationStructure, buffer hal.Buffer, offset uint64) {
	asObj, ok := accelStruct.(*AccelerationStructure)
	if !ok || asObj == nil || asObj.raw == 0 {
		return
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.raw == 0 {
		return
	}

	encoder := e.enterAccelerationStructureEncoder()
	if encoder == 0 {
		return
	}
	defer func() {
		_ = MsgSend(encoder, Sel("endEncoding"))
		Release(encoder)
	}()

	_ = MsgSend(encoder,
		Sel("writeCompactedAccelerationStructureSize:toBuffer:offset:"),
		uintptr(asObj.raw), uintptr(buf.raw), uintptr(offset),
	)
}
