// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package vulkan

import (
	"context"
	"encoding/binary"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

// rayQueryShaderWGSL casts one ray per invocation along +Z from z = -1 and
// writes (kind, instance_custom_data, t as f32 bits) for the committed hit.
// Rays 0 and 1 start inside the triangle's XY footprint; rays 2 and 3 miss.
const rayQueryShaderWGSL = `
enable wgpu_ray_query;

@group(0) @binding(0) var acc: acceleration_structure;
@group(0) @binding(1) var<storage, read_write> results: array<u32>;

@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    var origins = array<vec3<f32>, 4>(
        vec3<f32>(0.0, 0.0, -1.0),
        vec3<f32>(0.2, -0.5, -1.0),
        vec3<f32>(5.0, 5.0, -1.0),
        vec3<f32>(-5.0, 0.0, -1.0),
    );
    var rq: ray_query;
    rayQueryInitialize(&rq, acc, RayDesc(0u, 0xFFu, 0.0, 100.0, origins[id.x], vec3<f32>(0.0, 0.0, 1.0)));
    rayQueryProceed(&rq);
    let hit = rayQueryGetCommittedIntersection(&rq);
    results[id.x * 3u + 0u] = hit.kind;
    results[id.x * 3u + 1u] = hit.instance_custom_data;
    results[id.x * 3u + 2u] = bitcast<u32>(hit.t);
}
`

// validationRecorder is an slog.Handler that records Vulkan validation-layer
// errors (logged at Error level by vulkanDebugCallback).
type validationRecorder struct {
	mu     sync.Mutex
	errors []string
}

func (r *validationRecorder) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (r *validationRecorder) Handle(_ context.Context, rec slog.Record) error { //nolint:gocritic // signature fixed by slog.Handler
	if strings.HasPrefix(rec.Message, "vulkan: ") {
		r.mu.Lock()
		r.errors = append(r.errors, rec.Message)
		r.mu.Unlock()
	}
	return nil
}

func (r *validationRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *validationRecorder) WithGroup(string) slog.Handler      { return r }

// tryCreateRayQueryDevice opens a Vulkan device with FeatureRayQuery and
// validation layers enabled, or skips the test if no adapter supports it.
func tryCreateRayQueryDevice(t *testing.T) (hal.Device, hal.Queue, hal.Capabilities, func()) {
	t.Helper()

	if err := vk.Init(); err != nil {
		t.Skipf("Vulkan not available: %v", err)
	}

	instance, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.BackendsVulkan,
		Flags:    gputypes.InstanceFlagsDebug,
	})
	if err != nil {
		t.Skipf("Vulkan instance creation failed: %v", err)
	}

	adapters := instance.EnumerateAdapters(nil)
	for i := range adapters {
		exposed := &adapters[i]
		if !exposed.Features.Contains(gputypes.FeatureRayQuery) {
			continue
		}
		caps := exposed.Capabilities
		if caps.Limits.MaxTlasInstanceCount == 0 || caps.AlignmentsMask.RawTlasInstanceSize != 64 {
			t.Errorf("adapter %q reports FeatureRayQuery but limits/alignments are unset: %+v",
				exposed.Info.Name, caps.AlignmentsMask)
		}
		openDev, err := exposed.Adapter.Open(gputypes.Features(gputypes.FeatureRayQuery), caps.Limits)
		if err != nil {
			instance.Destroy()
			t.Fatalf("Open with FeatureRayQuery on %q: %v", exposed.Info.Name, err)
		}
		t.Logf("using adapter %q", exposed.Info.Name)
		cleanup := func() {
			_ = openDev.Device.WaitIdle()
			openDev.Device.Destroy()
			instance.Destroy()
		}
		return openDev.Device, openDev.Queue, caps, cleanup
	}

	instance.Destroy()
	t.Skip("no Vulkan adapter supports FeatureRayQuery")
	return nil, nil, hal.Capabilities{}, nil
}

func alignUp(v, align uint64) uint64 {
	if align <= 1 {
		return v
	}
	return (v + align - 1) / align * align
}

// TestRayQuerySmoke builds a one-triangle BLAS and a one-instance TLAS, then
// traces rays against it from a compute shader. It exercises the whole Vulkan
// ray tracing path below core: extension/feature enablement, proc loading,
// device-address memory, AS bind groups, builds, and barriers.
func TestRayQuerySmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}

	recorder := &validationRecorder{}
	prevLogger := hal.Logger()
	hal.SetLogger(slog.New(recorder))
	defer hal.SetLogger(prevLogger)

	device, queue, caps, cleanup := tryCreateRayQueryDevice(t)
	// Registered first so it runs after the buffer cleanups below.
	t.Cleanup(cleanup)

	mustBuffer := func(label string, size uint64, usage gputypes.BufferUsage) hal.Buffer {
		t.Helper()
		buf, err := device.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: usage})
		if err != nil {
			t.Fatalf("CreateBuffer(%s): %v", label, err)
		}
		t.Cleanup(func() { device.DestroyBuffer(buf) })
		return buf
	}

	// Triangle in the z = 0 plane.
	vertices := []float32{
		-1, -1, 0,
		1, -1, 0,
		0, 1, 0,
	}
	vertexBytes := make([]byte, len(vertices)*4)
	for i, v := range vertices {
		binary.LittleEndian.PutUint32(vertexBytes[i*4:], math.Float32bits(v))
	}
	vertexBuffer := mustBuffer("rt-vertices", uint64(len(vertexBytes)),
		gputypes.BufferUsageBlasInput|gputypes.BufferUsageMapWrite)
	if err := queue.WriteBuffer(vertexBuffer, 0, vertexBytes); err != nil {
		t.Fatalf("WriteBuffer(vertices): %v", err)
	}

	// BLAS.
	blasEntries := &hal.AccelerationStructureEntries{
		Triangles: []hal.AccelerationStructureTriangles{{
			VertexBuffer: vertexBuffer,
			VertexFormat: gputypes.VertexFormatFloat32x3,
			VertexCount:  3,
			VertexStride: 12,
			Flags:        gputypes.ASGeometryFlagOpaque,
		}},
	}
	blasFlags := gputypes.ASFlagPreferFastTrace
	blasSizes := device.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: blasEntries,
		Flags:   blasFlags,
	})
	if blasSizes.AccelerationStructureSize == 0 || blasSizes.BuildScratchSize == 0 {
		t.Fatalf("BLAS build sizes are zero: %+v", blasSizes)
	}
	blas, err := device.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "rt-blas",
		Size:   blasSizes.AccelerationStructureSize,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAccelerationStructure(BLAS): %v", err)
	}
	defer device.DestroyAccelerationStructure(blas)

	blasAddress := device.GetAccelerationStructureDeviceAddress(blas)
	if blasAddress == 0 {
		t.Fatal("BLAS device address is 0")
	}

	// TLAS instance data.
	const customData = 7
	instanceBytes := device.TlasInstanceToBytes(hal.TlasInstance{
		Transform:   [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0},
		CustomData:  customData,
		Mask:        0xFF,
		BlasAddress: blasAddress,
	})
	instanceBuffer := mustBuffer("rt-instances", uint64(len(instanceBytes)),
		gputypes.BufferUsageTlasInput|gputypes.BufferUsageMapWrite)
	if err := queue.WriteBuffer(instanceBuffer, 0, instanceBytes); err != nil {
		t.Fatalf("WriteBuffer(instances): %v", err)
	}

	tlasEntries := &hal.AccelerationStructureEntries{
		Instances: &hal.AccelerationStructureInstances{Buffer: instanceBuffer, Count: 1},
	}
	tlasFlags := gputypes.ASFlagPreferFastTrace
	tlasSizes := device.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: tlasEntries,
		Flags:   tlasFlags,
	})
	if tlasSizes.AccelerationStructureSize == 0 {
		t.Fatalf("TLAS build sizes are zero: %+v", tlasSizes)
	}
	tlas, err := device.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "rt-tlas",
		Size:   tlasSizes.AccelerationStructureSize,
		Format: hal.AccelerationStructureFormatTopLevel,
	})
	if err != nil {
		t.Fatalf("CreateAccelerationStructure(TLAS): %v", err)
	}
	defer device.DestroyAccelerationStructure(tlas)

	// One scratch buffer, with the TLAS region aligned as core will do it.
	scratchAlign := uint64(caps.AlignmentsMask.RayTracingScratchBufferAlignment)
	tlasScratchOffset := alignUp(blasSizes.BuildScratchSize, scratchAlign)
	scratchBuffer := mustBuffer("rt-scratch", tlasScratchOffset+tlasSizes.BuildScratchSize,
		gputypes.BufferUsageAccelerationStructureScratch)

	// Compute pipeline.
	shader, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "rt-query",
		Source: hal.ShaderSource{WGSL: rayQueryShaderWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer device.DestroyShaderModule(shader)

	const rayCount = 4
	const resultSize = rayCount * 3 * 4
	resultBuffer := mustBuffer("rt-results", resultSize, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc)
	readbackBuffer := mustBuffer("rt-readback", resultSize, gputypes.BufferUsageCopyDst|gputypes.BufferUsageMapRead)

	bgl, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "rt-bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:               0,
				Visibility:            gputypes.ShaderStageCompute,
				AccelerationStructure: &gputypes.AccelerationStructureBindingLayout{},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer device.DestroyBindGroupLayout(bgl)

	bg, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "rt-bg",
		Layout: bgl,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.AccelerationStructureBinding{AccelerationStructure: tlas.NativeHandle()}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: resultBuffer.NativeHandle(), Size: resultSize}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer device.DestroyBindGroup(bg)

	pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "rt-pl",
		BindGroupLayouts: []hal.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer device.DestroyPipelineLayout(pipelineLayout)

	pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:   "rt-pipeline",
		Layout:  pipelineLayout,
		Compute: hal.ComputeState{Module: shader, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	defer device.DestroyComputePipeline(pipeline)

	// Record: build BLAS -> barrier -> build TLAS -> barrier -> dispatch -> copy.
	encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "rt-encoder"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if err := encoder.BeginEncoding("rt-smoke"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}

	encoder.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries:                          blasEntries,
		Mode:                             hal.AccelerationStructureBuildModeBuild,
		Flags:                            blasFlags,
		DestinationAccelerationStructure: blas,
		ScratchBuffer:                    scratchBuffer,
	}})
	encoder.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{
		Usage: hal.AccelerationStructureUsageTransition{
			OldUsage: hal.AccelerationStructureUsesBuildOutput,
			NewUsage: hal.AccelerationStructureUsesBuildInput,
		},
	})
	encoder.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries:                          tlasEntries,
		Mode:                             hal.AccelerationStructureBuildModeBuild,
		Flags:                            tlasFlags,
		DestinationAccelerationStructure: tlas,
		ScratchBuffer:                    scratchBuffer,
		ScratchBufferOffset:              tlasScratchOffset,
	}})
	encoder.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{
		Usage: hal.AccelerationStructureUsageTransition{
			OldUsage: hal.AccelerationStructureUsesBuildOutput,
			NewUsage: hal.AccelerationStructureUsesShaderInput,
		},
	})

	pass := encoder.BeginComputePass(&hal.ComputePassDescriptor{Label: "rt-query"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	pass.End()

	encoder.TransitionBuffers([]hal.BufferBarrier{{
		Buffer: resultBuffer,
		Usage: hal.BufferUsageTransition{
			OldUsage: gputypes.BufferUsageStorage,
			NewUsage: gputypes.BufferUsageCopySrc,
		},
	}})
	encoder.CopyBufferToBuffer(resultBuffer, readbackBuffer, []hal.BufferCopy{{Size: resultSize}})

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{cmdBuffer}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := device.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}

	mapping, err := device.MapBuffer(readbackBuffer, 0, resultSize)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	raw := make([]byte, resultSize)
	copy(raw, unsafe.Slice((*byte)(mapping.Ptr), resultSize))
	_ = device.UnmapBuffer(readbackBuffer)

	const (
		kindNone     = 0
		kindTriangle = 1
	)
	want := []struct {
		kind uint32
		t    float32
	}{
		{kindTriangle, 1},
		{kindTriangle, 1},
		{kindNone, 0},
		{kindNone, 0},
	}
	for i, w := range want {
		kind := binary.LittleEndian.Uint32(raw[i*12:])
		custom := binary.LittleEndian.Uint32(raw[i*12+4:])
		hitT := math.Float32frombits(binary.LittleEndian.Uint32(raw[i*12+8:]))
		if kind != w.kind {
			t.Errorf("ray %d: kind = %d, want %d", i, kind, w.kind)
			continue
		}
		if kind == kindTriangle {
			if custom != customData {
				t.Errorf("ray %d: instance_custom_data = %d, want %d", i, custom, customData)
			}
			if math.Abs(float64(hitT-w.t)) > 1e-4 {
				t.Errorf("ray %d: t = %v, want %v", i, hitT, w.t)
			}
		}
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, msg := range recorder.errors {
		t.Errorf("validation error: %s", msg)
	}
}
