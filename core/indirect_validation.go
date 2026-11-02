//go:build !(js && wasm)

package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/indirect"
)

// validationShaderEntryPoint is the entry point name for the indirect dispatch
// validation compute shader.
const validationShaderEntryPoint = "main"

// IndirectValidation holds GPU resources for validating indirect dispatch
// workgroup counts before the actual dispatch executes. This prevents GPU
// hangs or TDR from invalid indirect buffers whose workgroup counts exceed
// maxComputeWorkgroupsPerDimension.
//
// Architecture: a single-workgroup compute shader reads the user's indirect
// buffer (x, y, z), checks each dimension against the device limit, and writes
// either the original values or (0, 0, 0) to a validated destination buffer.
// The actual DispatchIndirect then reads from the validated buffer.
//
// Every input of one validation dispatch is selected by binding a bind group
// built before recording, so commands recorded in any order, in any number of
// encoders, each read their own arguments. Rust wgpu-core passes the
// remainder of the offset as an immediate and the aligned offset as a dynamic
// offset; our HAL has neither on every backend, so core binds the aligned
// offset statically (one source bind group per arguments buffer and aligned
// offset, see Buffer.indirectSources) and selects the remainder from a
// constant table (one destination bind group per remainder).
//
// Created once at Device initialization and reused for every DispatchIndirect call.
// Matches Rust wgpu-core's indirect_validation::Dispatch (dispatch.rs).
type IndirectValidation struct {
	// shaderModule is the compiled validation compute shader.
	shaderModule hal.ShaderModule

	// pipeline is the compute pipeline running the validation shader.
	pipeline hal.ComputePipeline

	// pipelineLayout is the pipeline layout for the validation pipeline.
	pipelineLayout hal.PipelineLayout

	// srcBindGroupLayout describes the source indirect buffer binding (read-only storage).
	srcBindGroupLayout hal.BindGroupLayout

	// dstBindGroupLayout describes the destination buffer binding (read-write storage)
	// and the source index uniform binding.
	dstBindGroupLayout hal.BindGroupLayout

	// dstBuffer is the 12-byte destination buffer (3 x uint32) that receives
	// validated workgroup counts. Reused across frames.
	dstBuffer hal.Buffer

	// tableMu guards the lazy creation of indexTable and dstBindGroups on
	// the first DispatchIndirect (destinationGroups) and their release.
	tableMu sync.Mutex

	// indexTable is a constant uniform buffer: entry i, at i*indexStride,
	// holds the u32 i, for every index a DispatchSource can have.
	indexTable hal.Buffer

	// dstBindGroups[i] binds the destination buffer and index table entry i
	// (group 0).
	dstBindGroups []hal.BindGroup

	// storageAlignment is the device's minStorageBufferOffsetAlignment.
	storageAlignment uint32

	// indexStride is the device's minUniformBufferOffsetAlignment, the
	// distance between index table entries.
	indexStride uint64

	// maxComputeWorkgroupsPerDimension is cached from device limits.
	maxWorkgroups uint32

	// device is the parent HAL device for resource cleanup.
	device hal.Device
}

// IndirectDispatchParams are the resources of one validation dispatch.
// Matches Rust wgpu-core indirect_validation::dispatch::Params.
type IndirectDispatchParams struct {
	// Pipeline is the validation compute pipeline.
	Pipeline hal.ComputePipeline
	// DstBindGroup is group 0: the destination buffer and the index of x
	// in the source binding.
	DstBindGroup hal.BindGroup
	// SrcBindGroup is group 1: the arguments buffer at an aligned offset.
	SrcBindGroup hal.BindGroup
	// DstBuffer receives the validated arguments; the dispatch reads it at
	// offset 0.
	DstBuffer hal.Buffer
}

// dispatchIndirectValidationWGSL generates the WGSL validation shader source.
// The max_workgroups limit is baked into the shader as a constant, matching
// Rust wgpu-core which format!() the limit into the shader source at device
// creation time (dispatch.rs:48-72).
func dispatchIndirectValidationWGSL(maxWorkgroups uint32) string {
	return fmt.Sprintf(`@group(0) @binding(0) var<storage, read_write> dst: array<u32, 3>;
@group(0) @binding(1) var<uniform> src_index: SrcIndex;
@group(1) @binding(0) var<storage, read> src: array<u32>;

struct SrcIndex {
    inner: u32,
}

@compute @workgroup_size(1)
fn main() {
    let i = src_index.inner;
    let x = src[i];
    let y = src[i + 1u];
    let z = src[i + 2u];

    let limit = %du;
    if (x > limit || y > limit || z > limit) {
        dst[0] = 0u;
        dst[1] = 0u;
        dst[2] = 0u;
    } else {
        dst[0] = x;
        dst[1] = y;
        dst[2] = z;
    }
}
`, maxWorkgroups)
}

// indirectValidationBuilder accumulates GPU resources during construction so
// that partial builds can be cleaned up on failure. This avoids the deeply
// nested error handling that would otherwise push NewIndirectValidation past
// the funlen threshold.
type indirectValidationBuilder struct {
	device             hal.Device
	shaderModule       hal.ShaderModule
	dstBindGroupLayout hal.BindGroupLayout
	srcBindGroupLayout hal.BindGroupLayout
	pipelineLayout     hal.PipelineLayout
	pipeline           hal.ComputePipeline
	dstBuffer          hal.Buffer
	indexTable         hal.Buffer
	dstBindGroups      []hal.BindGroup
}

// cleanup destroys all resources accumulated so far.
func (b *indirectValidationBuilder) cleanup() {
	for _, g := range b.dstBindGroups {
		b.device.DestroyBindGroup(g)
	}
	if b.indexTable != nil {
		b.device.DestroyBuffer(b.indexTable)
	}
	if b.dstBuffer != nil {
		b.device.DestroyBuffer(b.dstBuffer)
	}
	if b.pipeline != nil {
		b.device.DestroyComputePipeline(b.pipeline)
	}
	if b.pipelineLayout != nil {
		b.device.DestroyPipelineLayout(b.pipelineLayout)
	}
	if b.srcBindGroupLayout != nil {
		b.device.DestroyBindGroupLayout(b.srcBindGroupLayout)
	}
	if b.dstBindGroupLayout != nil {
		b.device.DestroyBindGroupLayout(b.dstBindGroupLayout)
	}
	if b.shaderModule != nil {
		b.device.DestroyShaderModule(b.shaderModule)
	}
}

// NewIndirectValidation creates the GPU resources needed for indirect dispatch
// validation. Called once during Device initialization.
//
// Returns nil (not an error) if the HAL device does not support compute
// pipelines or if resource creation fails. Validation is optional -- when
// nil, DispatchIndirect falls through to the unvalidated path.
//
// Matches Rust wgpu-core device/resource.rs:439-454 where indirect_validation
// is created during Device::new and stored as Option<IndirectValidation>.
func NewIndirectValidation(halDevice hal.Device, limits gputypes.Limits) *IndirectValidation {
	if halDevice == nil {
		return nil
	}

	maxWorkgroups := limits.MaxComputeWorkgroupsPerDimension
	if maxWorkgroups == 0 {
		return nil
	}

	// Check that the backend supports compute shaders (noop/test backends don't).
	// Rust wgpu checks DownlevelFlags::INDIRECT_EXECUTION + instance flag.
	if limits.MaxComputeWorkgroupSizeX == 0 {
		return nil
	}

	b := &indirectValidationBuilder{device: halDevice}
	iv, err := buildIndirectValidation(halDevice, limits, b)
	if err != nil {
		slog.Debug("indirect validation: creation failed, disabling", "error", err)
		b.cleanup()
		return nil
	}
	return iv
}

// buildIndirectValidation creates all GPU resources for indirect dispatch validation.
// On any failure, the caller is responsible for calling b.cleanup() to release
// partially created resources.
func buildIndirectValidation(
	halDevice hal.Device,
	limits gputypes.Limits,
	b *indirectValidationBuilder,
) (*IndirectValidation, error) {
	storageAlign := limits.MinStorageBufferOffsetAlignment
	indexStride := uint64(limits.MinUniformBufferOffsetAlignment)
	if storageAlign < 4 || storageAlign&(storageAlign-1) != 0 || indexStride < 4 {
		return nil, fmt.Errorf("unusable offset alignments: storage %d, uniform %d", storageAlign, indexStride)
	}

	if err := b.createPipeline(halDevice, limits.MaxComputeWorkgroupsPerDimension); err != nil {
		return nil, err
	}

	// Destination buffer (12 bytes = 3 x uint32).
	var err error
	b.dstBuffer, err = halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label: "wgpu_indirect_dispatch_dst",
		Size:  indirect.DispatchArgsSize,
		Usage: gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("dst buffer: %w", err)
	}
	if b.dstBuffer == nil {
		return nil, errors.New("dst buffer: HAL returned nil")
	}

	return &IndirectValidation{
		shaderModule:       b.shaderModule,
		pipeline:           b.pipeline,
		pipelineLayout:     b.pipelineLayout,
		srcBindGroupLayout: b.srcBindGroupLayout,
		dstBindGroupLayout: b.dstBindGroupLayout,
		dstBuffer:          b.dstBuffer,
		storageAlignment:   storageAlign,
		indexStride:        indexStride,
		maxWorkgroups:      limits.MaxComputeWorkgroupsPerDimension,
		device:             halDevice,
	}, nil
}

// destinationGroups returns the destination bind groups, creating the index
// table and the groups on the first call: a device that never dispatches
// indirectly never allocates them. Dispose releases them with the device.
func (iv *IndirectValidation) destinationGroups() ([]hal.BindGroup, error) {
	iv.tableMu.Lock()
	defer iv.tableMu.Unlock()
	if iv.dstBindGroups != nil {
		return iv.dstBindGroups, nil
	}
	if iv.device == nil {
		return nil, ErrResourceDestroyed
	}
	// The builder holds only what this call creates, so its cleanup on
	// failure leaves the pipeline and the destination buffer alone.
	b := &indirectValidationBuilder{device: iv.device}
	entries := iv.storageAlignment / 4
	if err := b.createIndexTable(iv.device, entries, iv.indexStride); err != nil {
		b.cleanup()
		return nil, err
	}
	if err := b.createDstBindGroups(iv.device, iv.dstBindGroupLayout, iv.dstBuffer, entries, iv.indexStride); err != nil {
		b.cleanup()
		return nil, err
	}
	iv.indexTable, iv.dstBindGroups = b.indexTable, b.dstBindGroups
	return iv.dstBindGroups, nil
}

// createPipeline creates the shader module, the bind group layouts, the
// pipeline layout and the compute pipeline.
func (b *indirectValidationBuilder) createPipeline(halDevice hal.Device, maxWorkgroups uint32) error {
	var err error
	b.shaderModule, err = halDevice.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "wgpu_indirect_dispatch_validation",
		Source: hal.ShaderSource{WGSL: dispatchIndirectValidationWGSL(maxWorkgroups)},
	})
	if err != nil {
		return fmt.Errorf("shader module: %w", err)
	}

	// Group 0: dst buffer (RW storage) + source index uniform.
	// Group 1: src indirect buffer (RO storage).
	b.dstBindGroupLayout, err = halDevice.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "wgpu_indirect_dispatch_dst_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeStorage, MinBindingSize: indirect.DispatchArgsSize,
			}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 4,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("dst bind group layout: %w", err)
	}

	b.srcBindGroupLayout, err = halDevice.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "wgpu_indirect_dispatch_src_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeReadOnlyStorage, MinBindingSize: indirect.DispatchArgsSize,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("src bind group layout: %w", err)
	}

	b.pipelineLayout, err = halDevice.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "wgpu_indirect_dispatch_pipeline_layout",
		BindGroupLayouts: []hal.BindGroupLayout{b.dstBindGroupLayout, b.srcBindGroupLayout},
	})
	if err != nil {
		return fmt.Errorf("pipeline layout: %w", err)
	}

	b.pipeline, err = halDevice.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "wgpu_indirect_dispatch_validation",
		Layout: b.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     b.shaderModule,
			EntryPoint: validationShaderEntryPoint,
		},
	})
	if err != nil {
		return fmt.Errorf("compute pipeline: %w", err)
	}
	return nil
}

// createIndexTable creates the constant source index table, written once
// through a mapping at creation; nothing writes it afterwards.
func (b *indirectValidationBuilder) createIndexTable(halDevice hal.Device, entries uint32, stride uint64) error {
	size := uint64(entries) * stride
	var err error
	b.indexTable, err = halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label:            "wgpu_indirect_dispatch_src_index",
		Size:             size,
		Usage:            gputypes.BufferUsageUniform,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("src index table: %w", err)
	}
	if b.indexTable == nil {
		return errors.New("src index table: HAL returned nil")
	}
	mapping, err := halDevice.MapBuffer(b.indexTable, 0, size)
	if err != nil {
		return fmt.Errorf("src index table: map: %w", err)
	}
	if mapping.Ptr == nil {
		return errors.New("src index table: HAL returned no mapping")
	}
	data := unsafe.Slice((*byte)(mapping.Ptr), size) //nolint:gosec // HAL mapping of the whole buffer, size bytes
	clear(data)
	for i := uint32(0); i < entries; i++ {
		binary.LittleEndian.PutUint32(data[uint64(i)*stride:], i)
	}
	if err := halDevice.UnmapBuffer(b.indexTable); err != nil {
		return fmt.Errorf("src index table: unmap: %w", err)
	}
	return nil
}

// createDstBindGroups creates one destination bind group per source index.
func (b *indirectValidationBuilder) createDstBindGroups(
	halDevice hal.Device,
	layout hal.BindGroupLayout,
	dst hal.Buffer,
	entries uint32,
	stride uint64,
) error {
	b.dstBindGroups = make([]hal.BindGroup, 0, entries)
	for i := uint32(0); i < entries; i++ {
		g, err := halDevice.CreateBindGroup(&hal.BindGroupDescriptor{
			Label:  "wgpu_indirect_dispatch_dst_group",
			Layout: layout,
			Entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.BufferBinding{
					Buffer: dst.NativeHandle(), Offset: 0, Size: indirect.DispatchArgsSize,
				}},
				{Binding: 1, Resource: gputypes.BufferBinding{
					Buffer: b.indexTable.NativeHandle(), Offset: uint64(i) * stride, Size: stride,
				}},
			},
		})
		if err != nil {
			return fmt.Errorf("dst bind group %d: %w", i, err)
		}
		b.dstBindGroups = append(b.dstBindGroups, g)
	}
	return nil
}

// DispatchParams returns the resources of the validation dispatch for the
// arguments at offset in buffer. The caller has validated offset (a multiple
// of 4, arguments inside the buffer) and holds a reference to buffer that
// keeps it alive until the recorded commands complete.
//
// The source bind group is created on first use of the buffer at that
// aligned offset and lives as long as the buffer (Buffer.Destroy destroys
// it), so commands recorded with it stay valid until they complete.
// Matches Rust wgpu-core indirect_validation::Dispatch::params.
func (iv *IndirectValidation) DispatchParams(buffer *Buffer, offset uint64) (IndirectDispatchParams, error) {
	if buffer == nil || buffer.indirectSources == nil {
		return IndirectDispatchParams{}, errors.New("indirect validation: buffer was not created with INDIRECT usage")
	}
	dstGroups, err := iv.destinationGroups()
	if err != nil {
		return IndirectDispatchParams{}, fmt.Errorf("indirect validation: %w", err)
	}
	src := indirect.DispatchSourceFor(offset, buffer.Size(), iv.storageAlignment)
	if int(src.Index) >= len(dstGroups) {
		return IndirectDispatchParams{}, fmt.Errorf("indirect validation: source index %d out of range", src.Index)
	}
	srcGroup, err := buffer.indirectSources.Get(src.BindingOffset, func() (hal.BindGroup, error) {
		return iv.createSrcBindGroup(buffer, src)
	})
	if err != nil {
		return IndirectDispatchParams{}, err
	}
	return IndirectDispatchParams{
		Pipeline:     iv.pipeline,
		DstBindGroup: dstGroups[src.Index],
		SrcBindGroup: srcGroup,
		DstBuffer:    iv.dstBuffer,
	}, nil
}

// createSrcBindGroup binds the arguments buffer at the source's aligned offset.
// Matches Rust wgpu-core dispatch.rs create_src_bind_group.
func (iv *IndirectValidation) createSrcBindGroup(buffer *Buffer, src indirect.DispatchSource) (hal.BindGroup, error) {
	if buffer.device == nil {
		return nil, ErrResourceDestroyed
	}
	guard := buffer.device.SnatchLock().Read()
	defer guard.Release()
	raw := buffer.Raw(guard)
	if raw == nil {
		return nil, ErrResourceDestroyed
	}
	return iv.device.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "wgpu_indirect_dispatch_src_group",
		Layout: iv.srcBindGroupLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: raw.NativeHandle(), Offset: src.BindingOffset, Size: src.BindingSize,
			}},
		},
	})
}

// DstBuffer returns the validated destination buffer for the actual
// DispatchIndirect call. The HAL DispatchIndirect should use this buffer
// at offset 0 instead of the user's original buffer.
func (iv *IndirectValidation) DstBuffer() hal.Buffer {
	return iv.dstBuffer
}

// Pipeline returns the validation compute pipeline.
func (iv *IndirectValidation) Pipeline() hal.ComputePipeline {
	return iv.pipeline
}

// PipelineLayout returns the validation pipeline layout.
func (iv *IndirectValidation) PipelineLayout() hal.PipelineLayout {
	return iv.pipelineLayout
}

// MaxWorkgroups returns the baked-in maximum workgroup count per dimension.
func (iv *IndirectValidation) MaxWorkgroups() uint32 {
	return iv.maxWorkgroups
}

// Dispose releases all GPU resources owned by this IndirectValidation.
// Called during Device.Release(). Matches Rust dispatch.rs:331-351 (dispose).
func (iv *IndirectValidation) Dispose() {
	if iv == nil {
		return
	}
	iv.tableMu.Lock()
	defer iv.tableMu.Unlock()
	if iv.device == nil {
		return
	}
	b := &indirectValidationBuilder{
		device:             iv.device,
		shaderModule:       iv.shaderModule,
		dstBindGroupLayout: iv.dstBindGroupLayout,
		srcBindGroupLayout: iv.srcBindGroupLayout,
		pipelineLayout:     iv.pipelineLayout,
		pipeline:           iv.pipeline,
		dstBuffer:          iv.dstBuffer,
		indexTable:         iv.indexTable,
		dstBindGroups:      iv.dstBindGroups,
	}
	b.cleanup()
	iv.indexTable, iv.dstBindGroups = nil, nil
	iv.device = nil
}
