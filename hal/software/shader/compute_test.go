//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"
	"testing"

	naga "github.com/gogpu/naga"
)

// =============================================================================
// Phase 6 Tests: Compute Shaders
// =============================================================================

// buildArraySumComputeSPIRV constructs SPIR-V for a compute shader that
// reads element at global_id.x from an input buffer and adds it to an
// output buffer at position 0 (atomic add).
//
//	@group(0) @binding(0) var<storage, read> input: array<u32>;
//	@group(0) @binding(1) var<storage, read_write> output: array<u32>;
//	@compute @workgroup_size(4)
//	fn cs_main(@builtin(global_invocation_id) gid: vec3<u32>) {
//	    output[0] = output[0] + input[gid.x];
//	}
func buildArraySumComputeSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString

	const (
		idVoid        = 1
		idUint        = 2
		idUvec3       = 3
		idPtrUvec3In  = 4
		idFuncTy      = 5
		idFunc        = 6
		idGIDVar      = 7
		idRtArr       = 8  // runtime array of uint (unbounded)
		idStruct      = 9  // struct { array<u32> }
		idPtrStructSB = 10 // pointer to struct (StorageBuffer)
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
		idLoadOutput  = 21
		idSum         = 22
		idConst4      = 23
		idBound       = 24
	)

	nameWords := str("cs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelGLCompute, idFunc}, nameWords...)
	epInst = append(epInst, idGIDVar)

	words := make([]uint32, 0, 200)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		// LocalSize(4, 1, 1)
		inst(6, OpExecutionMode), idFunc, ExecutionModeLocalSize, 4, 1, 1,

		// Decorations.
		inst(4, OpDecorate), idGIDVar, DecorationBuiltIn, BuiltInGlobalInvocationId,
		inst(4, OpDecorate), idInputVar, DecorationBinding, 0,
		inst(4, OpDecorate), idInputVar, DecorationDescriptorSet, 0,
		inst(4, OpDecorate), idOutputVar, DecorationBinding, 1,
		inst(4, OpDecorate), idOutputVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,
		inst(4, OpDecorate), idRtArr, DecorationArrayStride, 4,

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idUvec3, idUint, 3,
		inst(4, OpTypePointer), idPtrUvec3In, StorageClassInput, idUvec3,
		// Runtime array = array with length 0 (unbounded).
		inst(4, OpConstant), idUint, idConst4, 4,
		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpTypeArray), idRtArr, idUint, idConst4,
		inst(3, OpTypeStruct), idStruct, idRtArr,
		inst(4, OpTypePointer), idPtrStructSB, StorageClassStorageBuffer, idStruct,
		inst(4, OpTypePointer), idPtrUintSB, StorageClassStorageBuffer, idUint,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		// Variables.
		inst(4, OpVariable), idPtrUvec3In, idGIDVar, StorageClassInput,
		inst(4, OpVariable), idPtrStructSB, idInputVar, StorageClassStorageBuffer,
		inst(4, OpVariable), idPtrStructSB, idOutputVar, StorageClassStorageBuffer,

		// Function.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,

		// Load global_invocation_id and extract x.
		inst(4, OpLoad), idUvec3, idLoadGID, idGIDVar,
		inst(5, OpCompositeExtract), idUint, idGIDx, idLoadGID, 0,

		// Load input[gid.x]
		inst(6, OpAccessChain), idPtrUintSB, idChainInput, idInputVar, idConst0, idGIDx,
		inst(4, OpLoad), idUint, idLoadInput, idChainInput,

		// Load output[0]
		inst(6, OpAccessChain), idPtrUintSB, idChainOutput, idOutputVar, idConst0, idConst0,
		inst(4, OpLoad), idUint, idLoadOutput, idChainOutput,

		// output[0] = output[0] + input[gid.x]
		inst(5, OpIAdd), idUint, idSum, idLoadOutput, idLoadInput,
		inst(3, OpStore), idChainOutput, idSum,

		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestComputeArraySum(t *testing.T) {
	words := buildArraySumComputeSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Verify workgroup size.
	wgSize := m.GetWorkgroupSize("cs_main")
	if wgSize != [3]uint32{4, 1, 1} {
		t.Errorf("workgroup size = %v, want {4,1,1}", wgSize)
	}

	// Input buffer: [10, 20, 30, 40]
	inputBuf := make([]byte, 16)
	putUint32LE(inputBuf[0:], 10)
	putUint32LE(inputBuf[4:], 20)
	putUint32LE(inputBuf[8:], 30)
	putUint32LE(inputBuf[12:], 40)

	// Output buffer: [0, 0, 0, 0]
	outputBuf := make([]byte, 16)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: inputBuf,
			{Group: 0, Binding: 1}: outputBuf,
		},
	}

	// Dispatch 1 workgroup of 4 invocations.
	err = m.DispatchCompute("cs_main", ctx, 1, 1, 1)
	if err != nil {
		t.Fatalf("DispatchCompute failed: %v", err)
	}

	// After all 4 invocations: output[0] should be 10+20+30+40 = 100.
	result := readUint32LE(outputBuf[0:])
	if result != 100 {
		t.Errorf("sum = %d, want 100", result)
	}
}

func TestComputeBuiltins(t *testing.T) {
	// Build a minimal compute shader that writes global_id.x to output[0].
	inst := spirvInst
	str := spirvString

	const (
		idVoid       = 1
		idUint       = 2
		idUvec3      = 3
		idPtrUvec3In = 4
		idFuncTy     = 5
		idFunc       = 6
		idGIDVar     = 7
		idStruct     = 8
		idPtrStruct  = 9
		idOutputVar  = 10
		idConst0     = 11
		idPtrUintSB  = 12
		idLabel      = 13
		idLoadGID    = 14
		idGIDx       = 15
		idChain      = 16
		idConst1     = 17
		idArr        = 18
		idBound      = 19
	)

	nameWords := str("cs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelGLCompute, idFunc}, nameWords...)
	epInst = append(epInst, idGIDVar)

	words := make([]uint32, 0, 150)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(6, OpExecutionMode), idFunc, ExecutionModeLocalSize, 1, 1, 1,

		inst(4, OpDecorate), idGIDVar, DecorationBuiltIn, BuiltInGlobalInvocationId,
		inst(4, OpDecorate), idOutputVar, DecorationBinding, 0,
		inst(4, OpDecorate), idOutputVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,
		inst(4, OpDecorate), idArr, DecorationArrayStride, 4,

		inst(2, OpTypeVoid), idVoid,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idUvec3, idUint, 3,
		inst(4, OpTypePointer), idPtrUvec3In, StorageClassInput, idUvec3,
		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idUint, idConst1, 1,
		inst(4, OpTypeArray), idArr, idUint, idConst1,
		inst(3, OpTypeStruct), idStruct, idArr,
		inst(4, OpTypePointer), idPtrStruct, StorageClassStorageBuffer, idStruct,
		inst(4, OpTypePointer), idPtrUintSB, StorageClassStorageBuffer, idUint,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpVariable), idPtrUvec3In, idGIDVar, StorageClassInput,
		inst(4, OpVariable), idPtrStruct, idOutputVar, StorageClassStorageBuffer,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(4, OpLoad), idUvec3, idLoadGID, idGIDVar,
		inst(5, OpCompositeExtract), idUint, idGIDx, idLoadGID, 0,
		inst(6, OpAccessChain), idPtrUintSB, idChain, idOutputVar, idConst0, idConst0,
		inst(3, OpStore), idChain, idGIDx,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	outputBuf := make([]byte, 4)
	ctx := &ExecutionContext{
		GlobalInvocationID: [3]uint32{42, 0, 0},
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: outputBuf,
		},
	}

	err = m.ExecuteCompute("cs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteCompute failed: %v", err)
	}

	result := readUint32LE(outputBuf)
	if result != 42 {
		t.Errorf("global_id.x = %d, want 42", result)
	}
}

func TestComputeWrongModel(t *testing.T) {
	// A fragment shader should fail when called via ExecuteCompute.
	words := buildTriangleFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	err = m.ExecuteCompute("fs_main", &ExecutionContext{})
	if err == nil {
		t.Fatal("expected error for non-compute entry point")
	}
}

func TestGetWorkgroupSize(t *testing.T) {
	words := buildArraySumComputeSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	tests := []struct {
		name       string
		entryPoint string
		want       [3]uint32
	}{
		{"existing", "cs_main", [3]uint32{4, 1, 1}},
		{"nonexistent", "not_found", [3]uint32{1, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetWorkgroupSize(tt.entryPoint)
			if got != tt.want {
				t.Errorf("GetWorkgroupSize(%q) = %v, want %v", tt.entryPoint, got, tt.want)
			}
		})
	}
}

func TestAtomicOps(t *testing.T) {
	// Direct test of atomic operations via executeAtomicOp.
	m := &Module{
		Types:     map[uint32]*TypeInfo{},
		Constants: map[uint32]Value{},
	}

	tests := []struct {
		name    string
		opcode  uint16
		initial uint32
		operand uint32
		wantOld uint32
		wantNew uint32
	}{
		{"iadd", OpAtomicIAdd, 10, 5, 10, 15},
		{"isub", OpAtomicISub, 10, 3, 10, 7},
		{"iinc", OpAtomicIIncrement, 10, 0, 10, 11},
		{"idec", OpAtomicIDecrement, 10, 0, 10, 9},
		{"exchange", OpAtomicExchange, 10, 99, 10, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr := &Pointer{Value: Uint32(tt.initial)}
			interp := &interpreter{
				module: m,
				values: map[uint32]Value{
					1: ptr,
					2: Uint32(tt.operand),
				},
			}

			inst := Instruction{
				Opcode:   tt.opcode,
				TypeID:   0,
				ResultID: 100,
			}
			// Operands: ptr, scope, semantics, value
			switch tt.opcode {
			case OpAtomicIIncrement, OpAtomicIDecrement:
				inst.Operands = []uint32{1, 0, 0}
			default:
				inst.Operands = []uint32{1, 0, 0, 2}
			}

			old := interp.executeAtomicOp(inst)
			gotOld := toUint32(old)
			gotNew := toUint32(ptr.Value)

			if gotOld != tt.wantOld {
				t.Errorf("old value = %d, want %d", gotOld, tt.wantOld)
			}
			if gotNew != tt.wantNew {
				t.Errorf("new value = %d, want %d", gotNew, tt.wantNew)
			}
		})
	}
}

func TestAtomicCompareExchange(t *testing.T) {
	m := &Module{
		Types:     map[uint32]*TypeInfo{},
		Constants: map[uint32]Value{},
	}

	// CAS succeeds: value=10, comparator=10, newValue=42
	ptr := &Pointer{Value: Uint32(10)}
	interp := &interpreter{
		module: m,
		values: map[uint32]Value{
			1: ptr,
			2: Uint32(42), // new value
			3: Uint32(10), // comparator
		},
	}

	inst := Instruction{
		Opcode:   OpAtomicCompareExchange,
		TypeID:   0,
		ResultID: 100,
		// pointer, scope, equal_sem, unequal_sem, value, comparator
		Operands: []uint32{1, 0, 0, 0, 2, 3},
	}

	old := interp.executeAtomicOp(inst)
	if toUint32(old) != 10 {
		t.Errorf("CAS old = %d, want 10", toUint32(old))
	}
	if toUint32(ptr.Value) != 42 {
		t.Errorf("CAS new = %d, want 42 (should have swapped)", toUint32(ptr.Value))
	}

	// CAS fails: value=42, comparator=99 (doesn't match)
	ptr.Value = Uint32(42)
	interp.values[3] = Uint32(99) // wrong comparator
	old = interp.executeAtomicOp(inst)
	if toUint32(ptr.Value) != 42 {
		t.Errorf("failed CAS new = %d, want 42 (should NOT have swapped)", toUint32(ptr.Value))
	}
	_ = old
}

func TestUvec3ToValue(t *testing.T) {
	v := uvec3ToValue([3]uint32{10, 20, 30})
	arr, ok := v.(Array)
	if !ok || len(arr) != 3 {
		t.Fatalf("uvec3ToValue returned %T, want Array of 3", v)
	}
	if toUint32(arr[0]) != 10 || toUint32(arr[1]) != 20 || toUint32(arr[2]) != 30 {
		t.Errorf("uvec3ToValue = %v, want [10, 20, 30]", arr)
	}
}

func TestDispatchComputeMultipleWorkgroups(t *testing.T) {
	// Test dispatching multiple workgroups. Use the builtins shader
	// that writes global_id.x to output.
	// With workgroup_size=1 and 4 workgroups, we should see global_id.x = 0,1,2,3.
	inst := spirvInst
	str := spirvString

	const (
		idVoid       = 1
		idUint       = 2
		idUvec3      = 3
		idPtrUvec3In = 4
		idFuncTy     = 5
		idFunc       = 6
		idGIDVar     = 7
		idStruct     = 8
		idPtrStruct  = 9
		idOutputVar  = 10
		idConst0     = 11
		idPtrUintSB  = 12
		idLabel      = 13
		idLoadGID    = 14
		idGIDx       = 15
		idChain      = 16
		idConst4     = 17
		idArr        = 18
		idBound      = 19
	)

	nameWords := str("cs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelGLCompute, idFunc}, nameWords...)
	epInst = append(epInst, idGIDVar)

	words := make([]uint32, 0, 150)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(6, OpExecutionMode), idFunc, ExecutionModeLocalSize, 1, 1, 1,

		inst(4, OpDecorate), idGIDVar, DecorationBuiltIn, BuiltInGlobalInvocationId,
		inst(4, OpDecorate), idOutputVar, DecorationBinding, 0,
		inst(4, OpDecorate), idOutputVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,
		inst(4, OpDecorate), idArr, DecorationArrayStride, 4,

		inst(2, OpTypeVoid), idVoid,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idUvec3, idUint, 3,
		inst(4, OpTypePointer), idPtrUvec3In, StorageClassInput, idUvec3,
		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idUint, idConst4, 4,
		inst(4, OpTypeArray), idArr, idUint, idConst4,
		inst(3, OpTypeStruct), idStruct, idArr,
		inst(4, OpTypePointer), idPtrStruct, StorageClassStorageBuffer, idStruct,
		inst(4, OpTypePointer), idPtrUintSB, StorageClassStorageBuffer, idUint,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpVariable), idPtrUvec3In, idGIDVar, StorageClassInput,
		inst(4, OpVariable), idPtrStruct, idOutputVar, StorageClassStorageBuffer,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(4, OpLoad), idUvec3, idLoadGID, idGIDVar,
		inst(5, OpCompositeExtract), idUint, idGIDx, idLoadGID, 0,
		// output[gid.x] = gid.x
		inst(6, OpAccessChain), idPtrUintSB, idChain, idOutputVar, idConst0, idGIDx,
		inst(3, OpStore), idChain, idGIDx,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	outputBuf := make([]byte, 16) // 4 uint32s
	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: outputBuf,
		},
	}

	// Dispatch 4 workgroups of size 1.
	err = m.DispatchCompute("cs_main", ctx, 4, 1, 1)
	if err != nil {
		t.Fatalf("DispatchCompute failed: %v", err)
	}

	// Verify output[i] == i for i in 0..3.
	for i := 0; i < 4; i++ {
		got := readUint32LE(outputBuf[i*4:])
		if got != uint32(i) {
			t.Errorf("output[%d] = %d, want %d", i, got, i)
		}
	}
}

// =============================================================================
// Naga Integration Tests: WGSL -> SPIR-V -> interpreter
// =============================================================================

// TestNagaComputeScaledCopy compiles a WGSL compute shader via naga, parses the
// resulting SPIR-V, dispatches through the interpreter, and verifies output.
// This is the definitive integration test: real naga-generated SPIR-V must work.
//
// Naga generates OpTypeRuntimeArray for array<f32> in storage buffers, and uses
// two-step OpAccessChain (struct member -> runtime array element) instead of the
// single-step pattern in hand-built SPIR-V.
func TestNagaComputeScaledCopy(t *testing.T) {
	wgsl := `
@group(0) @binding(0) var<storage, read> input: array<f32>;
@group(0) @binding(1) var<storage, read_write> output: array<f32>;
@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    output[i] = input[i] * 2.5;
}
`
	spirvBytes, err := naga.Compile(wgsl)
	if err != nil {
		t.Fatalf("naga.Compile failed: %v", err)
	}
	if len(spirvBytes)%4 != 0 {
		t.Fatalf("naga SPIR-V bytes not word-aligned: %d bytes", len(spirvBytes))
	}

	words := make([]uint32, len(spirvBytes)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
	}

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Verify workgroup size was parsed.
	wgSize := m.GetWorkgroupSize("main")
	if wgSize != [3]uint32{64, 1, 1} {
		t.Fatalf("workgroup size = %v, want {64, 1, 1}", wgSize)
	}

	// Prepare input buffer: [1.0, 2.0, 3.0, ..., 64.0]
	const numElements = 64
	const bufSize = numElements * 4
	inputBuf := make([]byte, bufSize)
	for i := 0; i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputBuf[i*4:], math.Float32bits(float32(i+1)))
	}

	outputBuf := make([]byte, bufSize)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: inputBuf,
			{Group: 0, Binding: 1}: outputBuf,
		},
	}

	// Dispatch 1 workgroup of 64 invocations.
	err = m.DispatchCompute("main", ctx, 1, 1, 1)
	if err != nil {
		t.Fatalf("DispatchCompute failed: %v", err)
	}

	// Verify: output[i] = input[i] * 2.5 = (i+1) * 2.5
	for i := 0; i < numElements; i++ {
		got := math.Float32frombits(binary.LittleEndian.Uint32(outputBuf[i*4:]))
		want := float32(i+1) * 2.5
		if math.Abs(float64(got-want)) > 1e-6 {
			t.Errorf("output[%d] = %f, want %f", i, got, want)
		}
	}
}

// TestNagaComputeIntegerMul tests a naga-compiled integer multiply compute shader.
// This ensures u32 storage buffers work correctly with naga's OpTypeRuntimeArray.
func TestNagaComputeIntegerMul(t *testing.T) {
	wgsl := `
@group(0) @binding(0) var<storage, read> input: array<u32>;
@group(0) @binding(1) var<storage, read_write> output: array<u32>;
@compute @workgroup_size(4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    output[i] = input[i] * 3u;
}
`
	spirvBytes, err := naga.Compile(wgsl)
	if err != nil {
		t.Fatalf("naga.Compile failed: %v", err)
	}

	words := make([]uint32, len(spirvBytes)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
	}

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	const numElements = 4
	const bufSize = numElements * 4
	inputBuf := make([]byte, bufSize)
	for i := uint32(0); i < numElements; i++ {
		binary.LittleEndian.PutUint32(inputBuf[i*4:], i+10)
	}

	outputBuf := make([]byte, bufSize)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: inputBuf,
			{Group: 0, Binding: 1}: outputBuf,
		},
	}

	err = m.DispatchCompute("main", ctx, 1, 1, 1)
	if err != nil {
		t.Fatalf("DispatchCompute failed: %v", err)
	}

	for i := uint32(0); i < numElements; i++ {
		got := binary.LittleEndian.Uint32(outputBuf[i*4:])
		want := (i + 10) * 3
		if got != want {
			t.Errorf("output[%d] = %d, want %d", i, got, want)
		}
	}
}

// Ensure the Phase 1 triangle tests still pass.
func TestTriangleStillWorks(t *testing.T) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["vs_main"]
	var posVarID, idxVarID uint32
	for _, varID := range ep.InterfaceIDs {
		switch m.GetBuiltIn(varID) {
		case BuiltInVertexIndex:
			idxVarID = varID
		case BuiltInPosition:
			posVarID = varID
		}
	}

	inputs := map[uint32]Value{idxVarID: Uint32(0)}
	outputs, err := m.Execute("vs_main", inputs)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	pos := Vec4ToFloat32(outputs[posVarID])
	// Vertex 0: (0.0, 0.5, 0.0, 1.0)
	if math.Abs(float64(pos[0])) > 1e-6 || math.Abs(float64(pos[1]-0.5)) > 1e-6 ||
		math.Abs(float64(pos[3]-1.0)) > 1e-6 {
		t.Errorf("triangle vertex 0 = %v, want (0, 0.5, 0, 1)", pos)
	}
}
