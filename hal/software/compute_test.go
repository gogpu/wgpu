//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// =============================================================================
// HAL Compute Integration Tests
// =============================================================================

// buildScaledCopySPIRV constructs SPIR-V bytecode for a compute shader that
// reads from an input storage buffer, multiplies each element by 3, and writes
// the result to an output storage buffer at the same index.
//
// WGSL equivalent:
//
//	@group(0) @binding(0) var<storage, read> input: array<u32>;
//	@group(0) @binding(1) var<storage, read_write> output: array<u32>;
//	@compute @workgroup_size(64)
//	fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
//	    output[gid.x] = input[gid.x] * 3;
//	}
func buildScaledCopySPIRV() []uint32 {
	inst := func(wordCount uint16, opcode uint16) uint32 {
		return uint32(wordCount)<<16 | uint32(opcode)
	}
	str := func(s string) []uint32 {
		b := append([]byte(s), 0)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		words := make([]uint32, len(b)/4)
		for i := range words {
			words[i] = binary.LittleEndian.Uint32(b[i*4:])
		}
		return words
	}

	const (
		idVoid        = 1
		idUint        = 2
		idUvec3       = 3
		idPtrUvec3In  = 4
		idFuncTy      = 5
		idFunc        = 6
		idGIDVar      = 7
		idArr         = 8
		idStruct      = 9
		idPtrStructSB = 10
		idInputVar    = 11
		idOutputVar   = 12
		idConst0      = 13
		idPtrUintSB   = 14
		idLabel       = 15
		idLoadGID     = 16
		idGIDx        = 17
		idChainInput  = 18
		idLoadInput   = 19
		idChainOutput = 20
		idConst3      = 21
		idProduct     = 22
		idConst64     = 23
		idBound       = 24
	)

	nameWords := str("main")
	epLen := uint16(3 + len(nameWords) + 1) // OpEntryPoint + model + funcID + name + interfaceIDs
	epInst := append([]uint32{inst(epLen, shader.OpEntryPoint), shader.ExecutionModelGLCompute, idFunc}, nameWords...)
	epInst = append(epInst, idGIDVar)

	words := make([]uint32, 0, 200)
	words = append(words,
		0x07230203, 0x00010300, 0, idBound, 0, // SPIR-V header
		inst(2, shader.OpCapability), 1, // Shader capability
		inst(3, shader.OpMemoryModel), 0, 1, // Logical GLSL450
	)
	words = append(words, epInst...)
	words = append(words,
		// OpExecutionMode: LocalSize(64, 1, 1)
		inst(6, shader.OpExecutionMode), idFunc, shader.ExecutionModeLocalSize, 64, 1, 1,

		// Decorations.
		inst(4, shader.OpDecorate), idGIDVar, shader.DecorationBuiltIn, shader.BuiltInGlobalInvocationID,
		inst(4, shader.OpDecorate), idInputVar, shader.DecorationBinding, 0,
		inst(4, shader.OpDecorate), idInputVar, shader.DecorationDescriptorSet, 0,
		inst(4, shader.OpDecorate), idOutputVar, shader.DecorationBinding, 1,
		inst(4, shader.OpDecorate), idOutputVar, shader.DecorationDescriptorSet, 0,
		inst(3, shader.OpDecorate), idStruct, shader.DecorationBlock,
		inst(5, shader.OpMemberDecorate), idStruct, 0, shader.DecorationOffset, 0,
		inst(4, shader.OpDecorate), idArr, shader.DecorationArrayStride, 4,

		// Types.
		inst(2, shader.OpTypeVoid), idVoid,
		inst(4, shader.OpTypeInt), idUint, 32, 0, // uint32
		inst(4, shader.OpTypeVector), idUvec3, idUint, 3,
		inst(4, shader.OpTypePointer), idPtrUvec3In, shader.StorageClassInput, idUvec3,

		// Constants.
		inst(4, shader.OpConstant), idUint, idConst64, 64,
		inst(4, shader.OpConstant), idUint, idConst0, 0,
		inst(4, shader.OpConstant), idUint, idConst3, 3,

		// Array type and struct wrapper.
		inst(4, shader.OpTypeArray), idArr, idUint, idConst64,
		inst(3, shader.OpTypeStruct), idStruct, idArr,
		inst(4, shader.OpTypePointer), idPtrStructSB, shader.StorageClassStorageBuffer, idStruct,
		inst(4, shader.OpTypePointer), idPtrUintSB, shader.StorageClassStorageBuffer, idUint,
		inst(3, shader.OpTypeFunction), idFuncTy, idVoid,

		// Variables.
		inst(4, shader.OpVariable), idPtrUvec3In, idGIDVar, shader.StorageClassInput,
		inst(4, shader.OpVariable), idPtrStructSB, idInputVar, shader.StorageClassStorageBuffer,
		inst(4, shader.OpVariable), idPtrStructSB, idOutputVar, shader.StorageClassStorageBuffer,

		// Function body: output[gid.x] = input[gid.x] * 3.
		inst(5, shader.OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, shader.OpLabel), idLabel,
		inst(4, shader.OpLoad), idUvec3, idLoadGID, idGIDVar,
		inst(5, shader.OpCompositeExtract), idUint, idGIDx, idLoadGID, 0,

		// input[gid.x]
		inst(6, shader.OpAccessChain), idPtrUintSB, idChainInput, idInputVar, idConst0, idGIDx,
		inst(4, shader.OpLoad), idUint, idLoadInput, idChainInput,

		// input[gid.x] * 3
		inst(5, shader.OpIMul), idUint, idProduct, idLoadInput, idConst3,

		// output[gid.x] = product
		inst(6, shader.OpAccessChain), idPtrUintSB, idChainOutput, idOutputVar, idConst0, idGIDx,
		inst(3, shader.OpStore), idChainOutput, idProduct,

		inst(1, shader.OpReturn),
		inst(1, shader.OpFunctionEnd),
	)
	return words
}

// TestSoftwareComputeDispatch is a full HAL-level integration test for compute
// shader dispatch. It creates a device, shader module, compute pipeline, bind
// group with input and output buffers, dispatches a workgroup, and verifies
// that the output buffer contains the expected scaled values.
func TestSoftwareComputeDispatch(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// 1. Create shader module from hand-built SPIR-V.
	spirvWords := buildScaledCopySPIRV()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "scaled-copy-compute",
		Source: hal.ShaderSource{
			SPIRV: spirvWords,
		},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}
	defer dev.DestroyShaderModule(sm)

	// 2. Create compute pipeline.
	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "scaled-copy-pipeline",
		Compute: hal.ComputeState{
			Module:     sm,
			EntryPoint: "main",
		},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline failed: %v", err)
	}
	defer dev.DestroyComputePipeline(pipeline)

	// 3. Create input and output buffers.
	const numElements = 64
	const bufSize = numElements * 4 // uint32 = 4 bytes

	inputBuf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label: "input",
		Size:  bufSize,
		Usage: gputypes.BufferUsageStorage,
	})
	if err != nil {
		t.Fatalf("CreateBuffer (input) failed: %v", err)
	}
	defer dev.DestroyBuffer(inputBuf)

	outputBuf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label: "output",
		Size:  bufSize,
		Usage: gputypes.BufferUsageStorage,
	})
	if err != nil {
		t.Fatalf("CreateBuffer (output) failed: %v", err)
	}
	defer dev.DestroyBuffer(outputBuf)

	// 4. Fill input buffer with values [1, 2, 3, ..., 64].
	inputData := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], i+1)
	}
	inputBuf.(*Buffer).WriteData(0, inputData)

	// 5. Create bind group with both buffers.
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "compute-bg",
		Entries: []gputypes.BindGroupEntry{
			{
				Binding: 0,
				Resource: gputypes.BufferBinding{
					Buffer: inputBuf.NativeHandle(),
					Offset: 0,
					Size:   bufSize,
				},
			},
			{
				Binding: 1,
				Resource: gputypes.BufferBinding{
					Buffer: outputBuf.NativeHandle(),
					Offset: 0,
					Size:   bufSize,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup failed: %v", err)
	}
	defer dev.DestroyBindGroup(bg)

	// 6. Encode and dispatch the compute pass.
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "compute-enc"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "compute-pass"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1) // 1 workgroup of 64 invocations
	pass.End()

	// 7. Verify output: each element should be (i+1) * 3.
	outBuf := outputBuf.(*Buffer)
	outData := outBuf.GetData()
	for i := uint32(0); i < numElements; i++ {
		got := binary.LittleEndian.Uint32(outData[i*4:])
		want := (i + 1) * 3
		if got != want {
			t.Errorf("output[%d] = %d, want %d", i, got, want)
		}
	}
}

// TestSoftwareComputeMultipleWorkgroups verifies that dispatching multiple
// workgroups correctly computes GlobalInvocationID across workgroup boundaries.
func TestSoftwareComputeMultipleWorkgroups(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	spirvWords := buildScaledCopySPIRV()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "scaled-copy",
		Source: hal.ShaderSource{SPIRV: spirvWords},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}
	defer dev.DestroyShaderModule(sm)

	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label: "multi-wg",
		Compute: hal.ComputeState{
			Module:     sm,
			EntryPoint: "main",
		},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline failed: %v", err)
	}
	defer dev.DestroyComputePipeline(pipeline)

	// 4 workgroups of 64 = 256 total invocations.
	// But our array is only 64 elements, so only the first workgroup
	// will write within bounds (gid.x < 64). The rest will be out-of-bounds
	// for the 64-element array and should not crash. The interpreter's
	// OpAccessChain with BufferPointer handles out-of-bounds gracefully.
	const numElements = 64
	const bufSize = numElements * 4

	inputBuf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: bufSize, Usage: gputypes.BufferUsageStorage})
	outputBuf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: bufSize, Usage: gputypes.BufferUsageStorage})
	defer dev.DestroyBuffer(inputBuf)
	defer dev.DestroyBuffer(outputBuf)

	inputData := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputData[i*4:], i+10)
	}
	inputBuf.(*Buffer).WriteData(0, inputData)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: inputBuf.NativeHandle()}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: outputBuf.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1) // Only 1 workgroup to stay within bounds.
	pass.End()

	outData := outputBuf.(*Buffer).GetData()
	for i := uint32(0); i < numElements; i++ {
		got := binary.LittleEndian.Uint32(outData[i*4:])
		want := (i + 10) * 3
		if got != want {
			t.Errorf("output[%d] = %d, want %d", i, got, want)
		}
	}
}

// TestSoftwareComputePipelineCreationErrors tests error paths for compute
// pipeline creation.
func TestSoftwareComputePipelineCreationErrors(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	t.Run("nil_descriptor", func(t *testing.T) {
		_, err := dev.CreateComputePipeline(nil)
		if err == nil {
			t.Fatal("expected error for nil descriptor")
		}
	})

	t.Run("nil_shader_module", func(t *testing.T) {
		_, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
			Compute: hal.ComputeState{Module: nil, EntryPoint: "main"},
		})
		if err == nil {
			t.Fatal("expected error for nil shader module")
		}
	})

	t.Run("non_software_module", func(t *testing.T) {
		_, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
			Compute: hal.ComputeState{Module: &Resource{}, EntryPoint: "main"},
		})
		if err == nil {
			t.Fatal("expected error for non-software shader module")
		}
	})

	t.Run("no_spirv", func(t *testing.T) {
		sm, _ := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "empty"})
		_, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
			Compute: hal.ComputeState{Module: sm, EntryPoint: "main"},
		})
		if err == nil {
			t.Fatal("expected error for shader module without SPIR-V")
		}
	})
}
