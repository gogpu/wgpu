//go:build !(js && wasm)

package core

import (
	"fmt"
	"log/slog"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
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
	// and the params uniform buffer binding.
	dstBindGroupLayout hal.BindGroupLayout

	// dstBuffer is the 12-byte destination buffer (3 x uint32) that receives
	// validated workgroup counts. Reused across frames.
	dstBuffer hal.Buffer

	// dstBindGroup is the pre-built bind group for the destination buffer and
	// params uniform buffer (group 0).
	dstBindGroup hal.BindGroup

	// paramsBuffer is the 8-byte uniform buffer holding Params (max_workgroups + src_offset).
	// Written before each validation dispatch.
	paramsBuffer hal.Buffer

	// maxComputeWorkgroupsPerDimension is cached from device limits.
	maxWorkgroups uint32

	// device is the parent HAL device for resource cleanup.
	device hal.Device
}

// dispatchIndirectValidationWGSL generates the WGSL validation shader source.
// The max_workgroups limit is baked into the shader as a constant, matching
// Rust wgpu-core which format!() the limit into the shader source at device
// creation time (dispatch.rs:48-72).
func dispatchIndirectValidationWGSL(maxWorkgroups uint32) string {
	return fmt.Sprintf(`@group(0) @binding(0) var<storage, read_write> dst: array<u32, 3>;
@group(0) @binding(1) var<uniform> params: Params;
@group(1) @binding(0) var<storage, read> src: array<u32>;

struct Params {
    max_workgroups: u32,
    src_offset: u32,
}

@compute @workgroup_size(1)
fn main() {
    let offset = params.src_offset;
    let x = src[offset];
    let y = src[offset + 1u];
    let z = src[offset + 2u];

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
	paramsBuffer       hal.Buffer
	dstBindGroup       hal.BindGroup
}

// cleanup destroys all resources accumulated so far.
func (b *indirectValidationBuilder) cleanup() {
	if b.dstBindGroup != nil {
		b.device.DestroyBindGroup(b.dstBindGroup)
	}
	if b.paramsBuffer != nil {
		b.device.DestroyBuffer(b.paramsBuffer)
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
	iv, err := buildIndirectValidation(halDevice, maxWorkgroups, b)
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
	maxWorkgroups uint32,
	b *indirectValidationBuilder,
) (*IndirectValidation, error) {
	var err error

	// 1. Create shader module.
	b.shaderModule, err = halDevice.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "wgpu_indirect_dispatch_validation",
		Source: hal.ShaderSource{WGSL: dispatchIndirectValidationWGSL(maxWorkgroups)},
	})
	if err != nil {
		return nil, fmt.Errorf("shader module: %w", err)
	}

	// 2. Create bind group layouts.
	// Group 0: dst buffer (RW storage) + params uniform.
	// Group 1: src indirect buffer (RO storage).
	b.dstBindGroupLayout, err = halDevice.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "wgpu_indirect_dispatch_dst_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeStorage, MinBindingSize: 12,
			}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 8,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dst bind group layout: %w", err)
	}

	b.srcBindGroupLayout, err = halDevice.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
		Label: "wgpu_indirect_dispatch_src_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeReadOnlyStorage, MinBindingSize: 12,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("src bind group layout: %w", err)
	}

	// 3. Create pipeline layout.
	b.pipelineLayout, err = halDevice.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "wgpu_indirect_dispatch_pipeline_layout",
		BindGroupLayouts: []hal.BindGroupLayout{b.dstBindGroupLayout, b.srcBindGroupLayout},
	})
	if err != nil {
		return nil, fmt.Errorf("pipeline layout: %w", err)
	}

	// 4. Create compute pipeline.
	b.pipeline, err = halDevice.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  "wgpu_indirect_dispatch_validation",
		Layout: b.pipelineLayout,
		Compute: hal.ComputeState{
			Module:     b.shaderModule,
			EntryPoint: validationShaderEntryPoint,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("compute pipeline: %w", err)
	}

	// 5. Create destination buffer (12 bytes = 3 x uint32).
	b.dstBuffer, err = halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label: "wgpu_indirect_dispatch_dst",
		Size:  12,
		Usage: gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("dst buffer: %w", err)
	}

	// 6. Create params uniform buffer (8 bytes = 2 x uint32).
	b.paramsBuffer, err = halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label: "wgpu_indirect_dispatch_params",
		Size:  8,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("params buffer: %w", err)
	}

	// 7. Create destination bind group (group 0: dst buffer + params uniform).
	if b.dstBuffer == nil || b.paramsBuffer == nil {
		return nil, fmt.Errorf("buffer creation returned nil")
	}
	b.dstBindGroup, err = halDevice.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "wgpu_indirect_dispatch_dst_group",
		Layout: b.dstBindGroupLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: b.dstBuffer.NativeHandle(), Offset: 0, Size: 12,
			}},
			{Binding: 1, Resource: gputypes.BufferBinding{
				Buffer: b.paramsBuffer.NativeHandle(), Offset: 0, Size: 8,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dst bind group: %w", err)
	}

	return &IndirectValidation{
		shaderModule:       b.shaderModule,
		pipeline:           b.pipeline,
		pipelineLayout:     b.pipelineLayout,
		srcBindGroupLayout: b.srcBindGroupLayout,
		dstBindGroupLayout: b.dstBindGroupLayout,
		dstBuffer:          b.dstBuffer,
		dstBindGroup:       b.dstBindGroup,
		paramsBuffer:       b.paramsBuffer,
		maxWorkgroups:      maxWorkgroups,
		device:             halDevice,
	}, nil
}

// CreateSrcBindGroup creates a bind group for the user's source indirect buffer.
// Called per-DispatchIndirect with the user's buffer. The bind group is
// ephemeral and must be destroyed by the caller after the validation dispatch.
//
// Matches Rust wgpu-core dispatch.rs:264-296 (create_src_bind_group).
func (iv *IndirectValidation) CreateSrcBindGroup(
	halDevice hal.Device,
	srcBuffer hal.Buffer,
	bufferSize uint64,
) (hal.BindGroup, error) {
	if bufferSize < 12 {
		return nil, fmt.Errorf("indirect dispatch buffer too small: %d < 12 bytes", bufferSize)
	}

	return halDevice.CreateBindGroup(&hal.BindGroupDescriptor{
		Label:  "wgpu_indirect_dispatch_src_group",
		Layout: iv.srcBindGroupLayout,
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{
				Buffer: srcBuffer.NativeHandle(), Offset: 0, Size: bufferSize,
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

// DstBindGroup returns the pre-built destination bind group (group 0).
func (iv *IndirectValidation) DstBindGroup() hal.BindGroup {
	return iv.dstBindGroup
}

// ParamsBuffer returns the uniform buffer for per-dispatch parameters.
func (iv *IndirectValidation) ParamsBuffer() hal.Buffer {
	return iv.paramsBuffer
}

// MaxWorkgroups returns the baked-in maximum workgroup count per dimension.
func (iv *IndirectValidation) MaxWorkgroups() uint32 {
	return iv.maxWorkgroups
}

// Dispose releases all GPU resources owned by this IndirectValidation.
// Called during Device.Release(). Matches Rust dispatch.rs:331-351 (dispose).
func (iv *IndirectValidation) Dispose() {
	if iv == nil || iv.device == nil {
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
		paramsBuffer:       iv.paramsBuffer,
		dstBindGroup:       iv.dstBindGroup,
	}
	b.cleanup()
	iv.device = nil
}
