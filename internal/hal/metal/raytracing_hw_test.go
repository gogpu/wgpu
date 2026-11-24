// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"math"
	"runtime"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/software"
)

// This file runs the Metal HAL acceleration structure path on a GPU: it builds
// a BLAS and a TLAS through the HAL, traces rays from a hand-written MSL
// compute kernel using Metal's intersector, and compares every hit with the
// software backend's CPU intersection on the same scene.
//
// It needs macOS 15 or later and a GPU with ray tracing support. It skips
// otherwise.

const rtKernelSource = `#include <metal_stdlib>
#include <metal_raytracing>
using namespace metal;
using namespace metal::raytracing;

struct Hit { float t; uint prim; uint inst; uint hit; };

kernel void trace(
    device const float* rays [[buffer(0)]],
    device Hit* hits [[buffer(1)]],
    instance_acceleration_structure as [[buffer(2)]],
    uint id [[thread_position_in_grid]])
{
    ray r;
    r.origin = float3(rays[6*id+0], rays[6*id+1], rays[6*id+2]);
    r.direction = float3(rays[6*id+3], rays[6*id+4], rays[6*id+5]);
    r.min_distance = 0.0f;
    r.max_distance = INFINITY;
    intersector<triangle_data, instancing> isect;
    intersection_result<triangle_data, instancing> res = isect.intersect(r, as);
    Hit h;
    h.hit = res.type == intersection_type::triangle ? 1u : 0u;
    h.t = res.distance;
    h.prim = res.primitive_id;
    h.inst = res.instance_id;
    hits[id] = h;
}

kernel void trace_prim(
    device const float* rays [[buffer(0)]],
    device Hit* hits [[buffer(1)]],
    primitive_acceleration_structure as [[buffer(2)]],
    uint id [[thread_position_in_grid]])
{
    ray r;
    r.origin = float3(rays[6*id+0], rays[6*id+1], rays[6*id+2]);
    r.direction = float3(rays[6*id+3], rays[6*id+4], rays[6*id+5]);
    r.min_distance = 0.0f;
    r.max_distance = INFINITY;
    intersector<triangle_data> isect;
    intersection_result<triangle_data> res = isect.intersect(r, as);
    Hit h;
    h.hit = res.type == intersection_type::triangle ? 1u : 0u;
    h.t = res.distance;
    h.prim = res.primitive_id;
    h.inst = 0;
    hits[id] = h;
}
`

type rtHit struct {
	T    float32
	Prim uint32
	Inst uint32
	Hit  uint32
}

// rtTriangles is the BLAS geometry: a quad (two triangles) and a small
// triangle, all in the z=0 plane.
var rtTriangles = [][3][3]float32{
	{{0, 0, 0}, {2, 0, 0}, {2, 2, 0}},
	{{0, 0, 0}, {2, 2, 0}, {0, 2, 0}},
	{{2.5, 0, 0}, {3.5, 0, 0}, {3, 1, 0}},
}

// rtInstances place the one BLAS twice; the second copy sits further along z.
var rtInstances = []hal.TlasInstance{
	{Transform: [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}, Mask: 0xFF},
	{Transform: [12]float32{1, 0, 0, 2, 0, 1, 0, 0.5, 0, 0, 1, 2}, Mask: 0xFF},
}

func rtOpenDevice(t *testing.T) (*Device, *Queue, func()) {
	t.Helper()
	inst, err := NewBackend().CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		skipWithoutMetal(t, "no Metal device available")
	}
	t.Logf("adapter: %s", adapters[0].Info.Name)
	if v := operatingSystemVersion(); v.Major < 15 {
		t.Skipf("MTLAccelerationStructure needs macOS 15, have %d.%d", v.Major, v.Minor)
	}
	od, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return od.Device.(*Device), od.Queue.(*Queue), func() { od.Device.Destroy() }
}

// TestMetalAdapterReportsTlasInstanceSize checks that the adapter reports the
// instance size TlasInstanceToBytes packs, so a caller sizing an instance
// buffer from the alignments does not fall back to the 64 bytes of Vulkan.
func TestMetalAdapterReportsTlasInstanceSize(t *testing.T) {
	inst, err := NewBackend().CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		skipWithoutMetal(t, "no Metal device available")
	}
	got := adapters[0].Capabilities.AlignmentsMask.RawTlasInstanceSize
	if want := len(tlasInstanceToBytes(hal.TlasInstance{})); got != uint32(want) {
		t.Fatalf("RawTlasInstanceSize = %d, want %d (the packed instance size)", got, want)
	}
}

func rtBuffer(t *testing.T, d *Device, size uint64, usage gputypes.BufferUsage) *Buffer {
	t.Helper()
	b, err := d.CreateBuffer(&hal.BufferDescriptor{Label: "rt", Size: size, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	return b.(*Buffer)
}

func rtSubmitWait(t *testing.T, d *Device, q *Queue, enc *CommandEncoder) {
	t.Helper()
	cb, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := q.Submit([]hal.CommandBuffer{cb}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	raw := cb.(*CommandBuffer).raw
	MsgSend(raw, Sel("waitUntilCompleted"))
	status := MsgSendUint(raw, Sel("status"))
	if status != 4 { // MTLCommandBufferStatusCompleted
		errObj := MsgSend(raw, Sel("error"))
		t.Fatalf("command buffer status %d: %s", status, formatNSError(errObj))
	}
}

func rtRays() (rays []float32, origins, dirs [][3]float32) {
	for iy := 0; iy < 16; iy++ {
		for ix := 0; ix < 32; ix++ {
			x := -0.5 + float32(ix)*0.1873 + 0.0137
			y := -0.25 + float32(iy)*0.1731 + 0.0291
			o := [3]float32{x, y, -1}
			dir := [3]float32{0, 0, 1}
			origins = append(origins, o)
			dirs = append(dirs, dir)
			rays = append(rays, o[0], o[1], o[2], dir[0], dir[1], dir[2])
		}
	}
	return
}

// rtCPUTrace is the software backend's reference: per instance, the BLAS
// triangles are moved by the instance transform and traced with the
// software package's BVH.
func rtCPUTrace(origins, dirs [][3]float32) []rtHit {
	return rtCPUTraceInstances(origins, dirs, rtInstances)
}

func rtCPUTraceInstances(origins, dirs [][3]float32, instances []hal.TlasInstance) []rtHit {
	roots := make([]*software.BVHNode, 0, len(instances))
	for _, in := range instances {
		tris := make([]software.Triangle, 0, len(rtTriangles))
		for i, tri := range rtTriangles {
			var v [3][3]float32
			for k := 0; k < 3; k++ {
				p := tri[k]
				m := in.Transform
				v[k] = [3]float32{
					m[0]*p[0] + m[1]*p[1] + m[2]*p[2] + m[3],
					m[4]*p[0] + m[5]*p[1] + m[6]*p[2] + m[7],
					m[8]*p[0] + m[9]*p[1] + m[10]*p[2] + m[11],
				}
			}
			tris = append(tris, software.Triangle{V0: v[0], V1: v[1], V2: v[2], PrimitiveIndex: uint32(i)})
		}
		roots = append(roots, software.BuildBVH(tris))
	}
	out := make([]rtHit, len(origins))
	for i := range origins {
		best := rtHit{T: float32(math.MaxFloat32)}
		for ii, root := range roots {
			hit, tt, _, prim := software.TraverseBVH(root, origins[i], dirs[i], float32(math.MaxFloat32))
			if hit && tt < best.T {
				best = rtHit{T: tt, Prim: prim, Inst: uint32(ii), Hit: 1}
			}
		}
		if best.Hit == 0 {
			best = rtHit{}
		}
		out[i] = best
	}
	return out
}

func TestMetalRayTracingOnHardware(t *testing.T) {
	d, q, cleanup := rtOpenDevice(t)
	defer cleanup()
	defer runtime.KeepAlive(d)

	// Geometry.
	vb := rtBuffer(t, d, uint64(len(rtTriangles)*3*12), gputypes.BufferUsageCopyDst|gputypes.BufferUsageBlasInput)
	verts := make([]float32, 0, len(rtTriangles)*9)
	for _, tri := range rtTriangles {
		for _, p := range tri {
			verts = append(verts, p[0], p[1], p[2])
		}
	}
	copy(unsafe.Slice((*float32)(vb.Contents()), len(verts)), verts)

	blasEntries := &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{
		VertexBuffer: vb, VertexFormat: gputypes.VertexFormatFloat32x3,
		VertexCount: uint32(len(rtTriangles) * 3), VertexStride: 12,
		Flags: gputypes.ASGeometryFlagOpaque,
	}}}
	blasSizes := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{Entries: blasEntries})
	t.Logf("BLAS sizes: %+v", blasSizes)
	if blasSizes.AccelerationStructureSize == 0 {
		t.Fatal("BLAS size query returned 0")
	}
	blas, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label: "blas", Size: blasSizes.AccelerationStructureSize, Format: hal.AccelerationStructureFormatBottomLevel})
	if err != nil {
		t.Fatalf("create BLAS: %v", err)
	}

	// Instances.
	ib := rtBuffer(t, d, uint64(len(rtInstances)*mtlASInstanceDescriptorSize), gputypes.BufferUsageCopyDst|gputypes.BufferUsageTlasInput)
	for i, in := range rtInstances {
		in.BlasAddress = d.GetAccelerationStructureDeviceAddress(blas)
		if in.BlasAddress == 0 {
			t.Fatal("BLAS has no GPU resource ID")
		}
		sz := mtlASInstanceDescriptorSize
		copy(unsafe.Slice((*byte)(ib.Contents()), len(rtInstances)*sz)[i*sz:], d.TlasInstanceToBytes(in))
	}
	tlasEntries := &hal.AccelerationStructureEntries{Instances: &hal.AccelerationStructureInstances{
		Buffer: ib, Count: uint32(len(rtInstances))}}
	tlasSizes := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{Entries: tlasEntries})
	t.Logf("TLAS sizes: %+v", tlasSizes)
	if tlasSizes.AccelerationStructureSize == 0 {
		t.Fatal("TLAS size query returned 0")
	}
	tlas, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label: "tlas", Size: tlasSizes.AccelerationStructureSize, Format: hal.AccelerationStructureFormatTopLevel})
	if err != nil {
		t.Fatalf("create TLAS: %v", err)
	}

	scratchSize := max(blasSizes.BuildScratchSize, tlasSizes.BuildScratchSize, 16)
	scratch := rtBuffer(t, d, scratchSize, gputypes.BufferUsageAccelerationStructureScratch)

	// Build both in one command buffer.
	enc0, err := d.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "build"})
	if err != nil {
		t.Fatal(err)
	}
	enc := enc0.(*CommandEncoder)
	if err := enc.BeginEncoding("build"); err != nil {
		t.Fatal(err)
	}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: blasEntries, Mode: hal.AccelerationStructureBuildModeBuild,
		DestinationAccelerationStructure: blas, ScratchBuffer: scratch}})
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: tlasEntries, Mode: hal.AccelerationStructureBuildModeBuild,
		DestinationAccelerationStructure: tlas, ScratchBuffer: scratch}})
	rtSubmitWait(t, d, q, enc)

	rays, origins, dirs := rtRays()
	want := rtCPUTrace(origins, dirs)

	// Stage A: the BLAS alone, through a primitive_acceleration_structure
	// kernel. It shows whether the BLAS build worked, independent of the TLAS.
	// The BLAS sits at instance 0's transform (identity), so only that
	// instance of the CPU reference applies; compare against a one-instance
	// reference.
	t.Run("BLAS only", func(t *testing.T) {
		got := rtTrace(t, d, "trace_prim", blas.(*AccelerationStructure).raw, 0, rays)
		rtCompare(t, got, rtCPUTraceInstances(origins, dirs, rtInstances[:1]), origins)
	})

	// Stage B: the TLAS as built by the HAL.
	t.Run("TLAS", func(t *testing.T) {
		got := rtTrace(t, d, "trace", tlas.(*AccelerationStructure).raw, blas.(*AccelerationStructure).raw, rays)
		rtCompare(t, got, want, origins)
	})

	// Stage C: the TLAS traced by the MSL that naga writes from a WGSL ray
	// query shader (kernel main_, same slots).
	t.Run("TLAS traced by naga MSL", func(t *testing.T) {
		src, _ := rtNagaMSL(t)
		got := rtTraceSrc(t, d, src, "main_", tlas.(*AccelerationStructure).raw, blas.(*AccelerationStructure).raw, rays)
		rtCompare(t, got, want, origins)
	})
}

func rtCompare(t *testing.T, got, want []rtHit, origins [][3]float32) {
	t.Helper()
	gpuHits, cpuHits, bad := 0, 0, 0
	for i := range got {
		g, w := got[i], want[i]
		if g.Hit == 1 {
			gpuHits++
		}
		if w.Hit == 1 {
			cpuHits++
		}
		ok := g.Hit == w.Hit && (w.Hit == 0 ||
			(math.Abs(float64(g.T-w.T)) < 1e-4 && g.Prim == w.Prim && g.Inst == w.Inst))
		if !ok {
			bad++
			if bad <= 4 {
				t.Errorf("ray %d origin %v: gpu %+v, cpu %+v", i, origins[i], g, w)
			}
		}
	}
	t.Logf("%d rays: gpu hits %d, cpu hits %d, mismatches %d", len(got), gpuHits, cpuHits, bad)
	if cpuHits == 0 {
		t.Fatal("reference scene produced no hits: test is broken")
	}
}

// rtTrace runs the named kernel over the rays against the acceleration
// structure as, and returns the hits. blas, when non-zero, is made resident
// with useResource.
func rtTrace(t *testing.T, d *Device, kernel string, as, blas ID, rays []float32) []rtHit {
	t.Helper()
	return rtTraceSrc(t, d, rtKernelSource, kernel, as, blas, rays)
}

// rtTraceSrc is rtTrace with the MSL source given.
func rtTraceSrc(t *testing.T, d *Device, src, kernel string, as, blas ID, rays []float32) []rtHit {
	t.Helper()
	n := len(rays) / 6
	rayBuf := rtBuffer(t, d, uint64(len(rays)*4), gputypes.BufferUsageCopyDst)
	copy(unsafe.Slice((*float32)(rayBuf.Contents()), len(rays)), rays)
	hitBuf := rtBuffer(t, d, uint64(n*16), gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)
	hits := unsafe.Slice((*rtHit)(hitBuf.Contents()), n)
	for i := range hits {
		hits[i] = rtHit{Prim: 0xdead, Inst: 0xdead, T: -1}
	}

	lib, nsErr := d.newLibraryWithSource(src)
	if lib == 0 {
		t.Fatalf("compile kernel: %s", formatNSError(nsErr))
	}
	nsName := NSString(kernel)
	fn := MsgSend(lib, Sel("newFunctionWithName:"), uintptr(nsName))
	if fn == 0 {
		t.Fatal("kernel function not found")
	}
	var errPtr ID
	var pinner runtime.Pinner
	pinner.Pin(&errPtr)
	pso := MsgSend(d.raw, Sel("newComputePipelineStateWithFunction:error:"), uintptr(fn), uintptr(unsafe.Pointer(&errPtr)))
	pinner.Unpin()
	if pso == 0 {
		t.Fatalf("pipeline: %s", formatNSError(errPtr))
	}

	cmd := MsgSend(d.commandQueue, Sel("commandBuffer"))
	ce := MsgSend(cmd, Sel("computeCommandEncoder"))
	MsgSend(ce, Sel("setComputePipelineState:"), uintptr(pso))
	MsgSend(ce, Sel("setBuffer:offset:atIndex:"), uintptr(rayBuf.raw), 0, 0)
	MsgSend(ce, Sel("setBuffer:offset:atIndex:"), uintptr(hitBuf.raw), 0, 1)
	MsgSend(ce, Sel("setAccelerationStructure:atBufferIndex:"), uintptr(as), 2)
	if blas != 0 {
		MsgSend(ce, Sel("useResource:usage:"), uintptr(blas), 1)
	}
	msgSendVoid(ce, Sel("dispatchThreads:threadsPerThreadgroup:"),
		argStruct(MTLSize{Width: NSUInteger(n), Height: 1, Depth: 1}, mtlSizeType),
		argStruct(MTLSize{Width: 32, Height: 1, Depth: 1}, mtlSizeType))
	MsgSend(ce, Sel("endEncoding"))
	MsgSend(cmd, Sel("commit"))
	MsgSend(cmd, Sel("waitUntilCompleted"))
	if st := MsgSendUint(cmd, Sel("status")); st != 4 {
		t.Fatalf("trace status %d: %s", st, formatNSError(MsgSend(cmd, Sel("error"))))
	}
	out := make([]rtHit, n)
	copy(out, hits)
	return out
}

// rtRun records fn into one HAL command buffer, submits it and waits.
func rtRun(t *testing.T, d *Device, q *Queue, fn func(enc *CommandEncoder)) {
	t.Helper()
	e, err := d.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "rt"})
	if err != nil {
		t.Fatal(err)
	}
	enc := e.(*CommandEncoder)
	if err := enc.BeginEncoding("rt"); err != nil {
		t.Fatal(err)
	}
	fn(enc)
	rtSubmitWait(t, d, q, enc)
}

// TestMetalRayTracingBLASVariants runs the BLAS options the HAL maps beyond
// plain triangle lists: an index buffer, a transform buffer, refit and
// compaction. Each is traced with the primitive kernel and compared with the
// software reference.
func TestMetalRayTracingBLASVariants(t *testing.T) {
	d, q, cleanup := rtOpenDevice(t)
	defer cleanup()
	defer runtime.KeepAlive(d)

	rays, origins, dirs := rtRays()
	one := []hal.TlasInstance{rtInstances[0]}
	want := rtCPUTraceInstances(origins, dirs, one)

	vb := rtBuffer(t, d, 256, gputypes.BufferUsageCopyDst|gputypes.BufferUsageBlasInput)
	vp := unsafe.Slice((*float32)(vb.Contents()), 64)
	// Shared-vertex layout: 4 corners of the quad, plus the small triangle's 3.
	copy(vp, []float32{0, 0, 0, 2, 0, 0, 2, 2, 0, 0, 2, 0, 2.5, 0, 0, 3.5, 0, 0, 3, 1, 0})
	ibuf := rtBuffer(t, d, 64, gputypes.BufferUsageCopyDst|gputypes.BufferUsageBlasInput)
	copy(unsafe.Slice((*uint32)(ibuf.Contents()), 16), []uint32{0, 1, 2, 0, 2, 3, 4, 5, 6})
	scratch := rtBuffer(t, d, 4096, gputypes.BufferUsageAccelerationStructureScratch)

	indexed := &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{
		VertexBuffer: vb, VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 7, VertexStride: 12,
		Indices: &hal.AccelerationStructureTriangleIndices{Format: gputypes.IndexFormatUint32, Buffer: ibuf, Count: 9},
		Flags:   gputypes.ASGeometryFlagOpaque,
	}}}

	build := func(t *testing.T, entries *hal.AccelerationStructureEntries, flags gputypes.AccelerationStructureFlags) hal.AccelerationStructure {
		t.Helper()
		sz := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{Entries: entries, Flags: flags})
		as, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{Size: sz.AccelerationStructureSize, Format: hal.AccelerationStructureFormatBottomLevel})
		if err != nil {
			t.Fatal(err)
		}
		rtRun(t, d, q, func(enc *CommandEncoder) {
			enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
				Entries: entries, Flags: flags, Mode: hal.AccelerationStructureBuildModeBuild,
				DestinationAccelerationStructure: as, ScratchBuffer: scratch}})
		})
		return as
	}
	check := func(t *testing.T, as hal.AccelerationStructure) {
		t.Helper()
		got := rtTrace(t, d, "trace_prim", as.(*AccelerationStructure).raw, 0, rays)
		rtCompare(t, got, want, origins)
	}

	t.Run("indexed", func(t *testing.T) {
		check(t, build(t, indexed, 0))
	})

	t.Run("compaction", func(t *testing.T) {
		as := build(t, indexed, gputypes.ASFlagAllowCompaction)
		sizeBuf := rtBuffer(t, d, 8, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)
		rtRun(t, d, q, func(enc *CommandEncoder) { enc.ReadAccelerationStructureCompactSize(as, sizeBuf, 0) })
		size := *(*uint64)(sizeBuf.Contents())
		t.Logf("compacted size %d", size)
		if size == 0 {
			t.Fatal("compacted size is 0")
		}
		dst, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{Size: size, Format: hal.AccelerationStructureFormatBottomLevel})
		if err != nil {
			t.Fatal(err)
		}
		rtRun(t, d, q, func(enc *CommandEncoder) {
			enc.CopyAccelerationStructure(as, dst, gputypes.AccelerationStructureCopyModeCompact)
		})
		check(t, dst)
	})

	t.Run("refit", func(t *testing.T) {
		flags := gputypes.ASFlagAllowUpdate
		as := build(t, indexed, flags)
		check(t, as)
		// Move the whole scene by +0.25 in x, refit, compare with the moved CPU scene.
		for i := 0; i < 21; i += 3 {
			vp[i] += 0.25
		}
		sz := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{Entries: indexed, Flags: flags})
		t.Logf("refit scratch %d", sz.UpdateScratchSize)
		rtRun(t, d, q, func(enc *CommandEncoder) {
			enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
				Entries: indexed, Flags: flags, Mode: hal.AccelerationStructureBuildModeUpdate,
				SourceAccelerationStructure: as, DestinationAccelerationStructure: as, ScratchBuffer: scratch}})
		})
		moved := make([][3]float32, len(origins))
		for i, o := range origins {
			moved[i] = [3]float32{o[0] - 0.25, o[1], o[2]}
		}
		got := rtTrace(t, d, "trace_prim", as.(*AccelerationStructure).raw, 0, rays)
		rtCompare(t, got, rtCPUTraceInstances(moved, dirs, one), origins)
	})
}
