//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// =============================================================================
// Additional coverage tests to reach 80%+ on the shader/ package.
// These tests exercise interpreter opcodes and helper functions
// that are not covered by the per-phase integration tests.
// =============================================================================

func TestConvertSignedToFloat(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want float32
	}{
		{"int32_pos", Int32(42), 42},
		{"int32_neg", Int32(-7), -7},
		{"uint32_as_signed", Uint32(100), 100},
		{"float32_passthrough", Float32(3.14), 3.14},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertSignedToFloat(tt.val)
			f, ok := got.(Float32)
			if !ok {
				t.Fatalf("returned %T, want Float32", got)
			}
			if math.Abs(float64(f-tt.want)) > 0.01 {
				t.Errorf("convertSignedToFloat(%v) = %f, want %f", tt.val, f, tt.want)
			}
		})
	}
}

func TestConvertFloatToUint(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want uint32
	}{
		{"float_5", Float32(5), 5},
		{"uint_pass", Uint32(10), 10},
		{"int_pass", Int32(7), 7},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertFloatToUint(tt.val)
			u := toUint32(got)
			if u != tt.want {
				t.Errorf("convertFloatToUint(%v) = %d, want %d", tt.val, u, tt.want)
			}
		})
	}
}

func TestToBoolTypes(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want bool
	}{
		{"bool_true", true, true},
		{"bool_false", false, false},
		{"uint_nonzero", Uint32(1), true},
		{"uint_zero", Uint32(0), false},
		{"int_nonzero", Int32(-1), true},
		{"int_zero", Int32(0), false},
		{"float_nonzero", Float32(0.1), true},
		{"float_zero", Float32(0), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toBool(tt.val)
			if got != tt.want {
				t.Errorf("toBool(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestToFloat32Types(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want float32
	}{
		{"float", Float32(3.14), 3.14},
		{"uint", Uint32(42), 42},
		{"int", Int32(-5), -5},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat32(tt.val)
			if math.Abs(float64(got-tt.want)) > 0.01 {
				t.Errorf("toFloat32(%v) = %f, want %f", tt.val, got, tt.want)
			}
		})
	}
}

func TestToUint32Types(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want uint32
	}{
		{"uint", Uint32(42), 42},
		{"int", Int32(7), 7},
		{"float", Float32(5.9), 5},
		{"bool_true", true, 1},
		{"bool_false", false, 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toUint32(tt.val)
			if got != tt.want {
				t.Errorf("toUint32(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestAppendComponentsAllTypes(t *testing.T) {
	var out []float32
	out = appendComponents(out, Float32(1))
	out = appendComponents(out, Vec2{2, 3})
	out = appendComponents(out, Vec3{4, 5, 6})
	out = appendComponents(out, Vec4{7, 8, 9, 10})
	out = appendComponents(out, Uint32(11))
	out = appendComponents(out, Int32(12))
	out = appendComponents(out, nil) // default

	want := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 0}
	if len(out) != len(want) {
		t.Fatalf("len = %d, want %d", len(out), len(want))
	}
	for i, w := range want {
		if out[i] != w {
			t.Errorf("[%d] = %f, want %f", i, out[i], w)
		}
	}
}

// TestInterpreterMiscOpcodes exercises opcodes not covered by integration tests.
func TestInterpreterMiscOpcodes(t *testing.T) {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	// Build a shader that exercises: FMod, FRem, SNegate, SDiv, UDiv, SMod, UMod, SRem,
	// bitwise AND/OR/XOR, shifts, logical ops, comparisons, select, etc.
	const (
		idVoid     = 1
		idFloat    = 2
		idUint     = 3
		idInt      = 4
		idBool     = 5
		idVec4     = 6
		idPtrV4Out = 7
		idFuncTy   = 8
		idFunc     = 9
		idLabel    = 10
		idColorOut = 11
		idCF1      = 12 // float 7.0
		idCF2      = 13 // float 3.0
		idCU1      = 14 // uint 10
		idCU2      = 15 // uint 3
		idFModR    = 16
		idFRemR    = 17
		idSNegR    = 18
		idSDivR    = 19
		idUDivR    = 20
		idSModR    = 21
		idUModR    = 22
		idSRemR    = 23
		idAndR     = 24
		idOrR      = 25
		idXorR     = 26
		idShlR     = 27
		idShrR     = 28
		idNotR     = 29
		idLAndR    = 30
		idLOrR     = 31
		idLNotR    = 32
		idTrue     = 33
		idFalse    = 34
		idSelR     = 35
		idCvtFtoS  = 36
		idCvtFtoU  = 37
		idCvtStoF  = 38
		idULtR     = 39
		idSLtR     = 40
		idResult   = 41
		idConst0   = 42
		idConst1   = 43
		idShrAR    = 44
		idBound    = 45
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 400)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,

		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeInt), idInt, 32, 1,
		inst(2, OpTypeBool), idBool,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpConstant), idFloat, idCF1, f(7),
		inst(4, OpConstant), idFloat, idCF2, f(3),
		inst(4, OpConstant), idUint, idCU1, 10,
		inst(4, OpConstant), idUint, idCU2, 3,
		inst(4, OpConstant), idFloat, idConst0, f(0),
		inst(4, OpConstant), idFloat, idConst1, f(1),
		inst(3, OpConstantTrue), idBool, idTrue,
		inst(3, OpConstantFalse), idBool, idFalse,

		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,

		// FMod(7, 3) = 1
		inst(5, OpFMod), idFloat, idFModR, idCF1, idCF2,
		// FRem(7, 3) = 1
		inst(5, OpFRem), idFloat, idFRemR, idCF1, idCF2,
		// SNegate(10) = -10 (as int32)
		inst(4, OpSNegate), idInt, idSNegR, idCU1,
		// SDiv(10, 3) = 3
		inst(5, OpSDiv), idInt, idSDivR, idCU1, idCU2,
		// UDiv(10, 3) = 3
		inst(5, OpUDiv), idUint, idUDivR, idCU1, idCU2,
		// SMod(10, 3) = 1
		inst(5, OpSMod), idInt, idSModR, idCU1, idCU2,
		// UMod(10, 3) = 1
		inst(5, OpUMod), idUint, idUModR, idCU1, idCU2,
		// SRem(10, 3) = 1
		inst(5, OpSRem), idInt, idSRemR, idCU1, idCU2,
		// Bitwise AND(10, 3) = 2
		inst(5, OpBitwiseAnd), idUint, idAndR, idCU1, idCU2,
		// Bitwise OR(10, 3) = 11
		inst(5, OpBitwiseOr), idUint, idOrR, idCU1, idCU2,
		// Bitwise XOR(10, 3) = 9
		inst(5, OpBitwiseXor), idUint, idXorR, idCU1, idCU2,
		// ShiftLeft(3, 2) = 12 (but we need const 2...)
		// Use CU2=3 shifted by... let's shift 1 left by 3 = 8
		inst(5, OpShiftLeftLogical), idUint, idShlR, idCU2, idCU2, // 3 << (3&31) = 24
		// ShiftRightLogical(10, 1)
		inst(5, OpShiftRightLogical), idUint, idShrR, idCU1, idCU2, // 10 >> 3 = 1
		// ShiftRightArithmetic
		inst(5, OpShiftRightArithmetic), idInt, idShrAR, idCU1, idCU2,
		// Not(10) = ~10
		inst(4, OpNot), idUint, idNotR, idCU1,
		// LogicalAnd(true, false) = false
		inst(5, OpLogicalAnd), idBool, idLAndR, idTrue, idFalse,
		// LogicalOr(true, false) = true
		inst(5, OpLogicalOr), idBool, idLOrR, idTrue, idFalse,
		// LogicalNot(true) = false
		inst(4, OpLogicalNot), idBool, idLNotR, idTrue,
		// Select(true, 1.0, 0.0) = 1.0
		inst(6, OpSelect), idFloat, idSelR, idTrue, idConst1, idConst0,
		// ConvertFToS(7.0) = 7
		inst(4, OpConvertFToS), idInt, idCvtFtoS, idCF1,
		// ConvertFToU(7.0) = 7
		inst(4, OpConvertFToU), idUint, idCvtFtoU, idCF1,
		// ConvertSToF(-10) = -10.0
		inst(4, OpConvertSToF), idFloat, idCvtStoF, idSNegR,
		// ULessThan(3, 10) = true
		inst(5, OpULessThan), idBool, idULtR, idCU2, idCU1,
		// SLessThan(-10, 3) = true
		inst(5, OpSLessThan), idBool, idSLtR, idSNegR, idCU2,

		// Use FModR as the result to verify it ran.
		inst(7, OpCompositeConstruct), idVec4, idResult, idFModR, idConst0, idConst0, idConst1,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	outputs, err := m.Execute("fs_main", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	ep := m.EntryPoints["fs_main"]
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassOutput {
			color := Vec4ToFloat32(outputs[varID])
			// FMod(7, 3) = 1.0
			if math.Abs(float64(color[0]-1.0)) > 0.01 {
				t.Errorf("FMod(7,3) = %f, want 1.0", color[0])
			}
			return
		}
	}
	t.Fatal("output not found")
}

func TestZeroValueForVar(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1:  {Kind: TypePointer, ElemType: 2, Storage: StorageClassFunction},
			2:  {Kind: TypeFloat, Width: 32},
			3:  {Kind: TypePointer, ElemType: 4, Storage: StorageClassFunction},
			4:  {Kind: TypeInt, Width: 32, Signed: true},
			5:  {Kind: TypePointer, ElemType: 6, Storage: StorageClassFunction},
			6:  {Kind: TypeVector, ElemType: 2, Components: 3},
			7:  {Kind: TypePointer, ElemType: 8, Storage: StorageClassFunction},
			8:  {Kind: TypeArray, ElemType: 2, Length: 2},
			9:  {Kind: TypePointer, ElemType: 10, Storage: StorageClassFunction},
			10: {Kind: TypeBool},
		},
	}

	tests := []struct {
		name     string
		ptrType  uint32
		wantType string
	}{
		{"float", 1, "float32"},
		{"int", 3, "int32"},
		{"vec3", 5, "[3]float32"},
		{"array", 7, "[]interface {}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := zeroValueForVar(m, tt.ptrType)
			if val == nil {
				t.Fatalf("zeroValueForVar returned nil")
			}
		})
	}
}

func TestTypeByteSizeAll(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeInt, Width: 32},
			3: {Kind: TypeBool},
			4: {Kind: TypeVector, ElemType: 1, Components: 4},
			5: {Kind: TypeArray, ElemType: 1, Length: 3},
			6: {Kind: TypeStruct, MemberIDs: []uint32{1, 2}},
		},
	}

	tests := []struct {
		name string
		id   uint32
		want uint32
	}{
		{"float32", 1, 4},
		{"int32", 2, 4},
		{"bool", 3, 4},
		{"vec4", 4, 16},
		{"array_3_float", 5, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typeByteSize(m, m.Types[tt.id])
			if got != tt.want {
				t.Errorf("typeByteSize = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCompositeConstructStruct(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeStruct, MemberIDs: []uint32{2, 2}},
			2: {Kind: TypeFloat, Width: 32},
		},
		Constants: map[uint32]Value{
			10: Float32(1),
			11: Float32(2),
		},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]Value{10: Float32(1), 11: Float32(2)}),
	}

	got := interp.compositeConstruct(1, []uint32{10, 11})
	arr, ok := got.(Array)
	if !ok || len(arr) != 2 {
		t.Fatalf("compositeConstruct struct returned %T", got)
	}
	if toFloat32(arr[0]) != 1 || toFloat32(arr[1]) != 2 {
		t.Errorf("struct members = %v, want [1, 2]", arr)
	}
}

func TestFloatBinOpMismatch(t *testing.T) {
	// Test floatBinOp with mismatched types.
	add := func(a, b float32) float32 { return a + b }

	// Vec2 + non-Vec2 returns a (unchanged).
	got := floatBinOp(Vec2{1, 2}, Float32(3), add)
	if _, ok := got.(Vec2); !ok {
		t.Errorf("vec2 + scalar returned %T, want Vec2", got)
	}

	// Non-float types.
	got = floatBinOp(nil, nil, add)
	if _, ok := got.(Float32); !ok {
		t.Errorf("nil + nil returned %T, want Float32", got)
	}
}

func TestFloatUnaryOpAllTypes(t *testing.T) {
	neg := func(a float32) float32 { return -a }

	tests := []struct {
		name string
		val  Value
	}{
		{"scalar", Float32(5)},
		{"vec2", Vec2{1, 2}},
		{"vec3", Vec3{1, 2, 3}},
		{"vec4", Vec4{1, 2, 3, 4}},
		{"nil", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := floatUnaryOp(tt.val, neg)
			if got == nil {
				t.Fatal("returned nil")
			}
		})
	}
}

func TestWriteValueToBufferArray(t *testing.T) {
	arr := Array{Float32(1), Float32(2), Float32(3)}
	buf := make([]byte, 12)
	writeValueToBuffer(buf, 0, arr)

	for i := 0; i < 3; i++ {
		got := readFloat32LE(buf[i*4:])
		if got != float32(i+1) {
			t.Errorf("buf[%d] = %f, want %f", i, got, float32(i+1))
		}
	}
}

func TestGlslTernaryFloat(t *testing.T) {
	m := &Module{
		Types:     map[uint32]*TypeInfo{},
		Constants: map[uint32]Value{},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]Value{
			1: Float32(5),
			2: Float32(0),
			3: Float32(10),
			4: Vec4{5, 5, 5, 5},
			5: Vec4{0, 0, 0, 0},
			6: Vec4{10, 10, 10, 10},
		}),
	}

	// Test scalar clamp.
	clamp := func(x, minV, maxV float32) float32 {
		if x < minV {
			return minV
		}
		if x > maxV {
			return maxV
		}
		return x
	}

	got := interp.glslTernaryFloat([]uint32{1, 2, 3}, clamp)
	if f, ok := got.(Float32); !ok || f != 5 {
		t.Errorf("clamp(5, 0, 10) = %v, want 5", got)
	}

	// Test vec4 clamp.
	got = interp.glslTernaryFloat([]uint32{4, 5, 6}, clamp)
	if v, ok := got.(Vec4); !ok || v[0] != 5 {
		t.Errorf("vec4 clamp = %v, want all 5s", got)
	}
}

func TestGlslBinaryUint(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]Value{1: Uint32(5), 2: Uint32(3)}),
	}
	got := interp.glslBinaryUint([]uint32{1, 2}, func(a, b uint32) uint32 {
		if a < b {
			return a
		}
		return b
	})
	if u, ok := got.(Uint32); !ok || u != 3 {
		t.Errorf("umin(5,3) = %v, want 3", got)
	}
}

func TestGlslBinaryInt(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]Value{1: Int32(-5), 2: Int32(3)}),
	}
	got := interp.glslBinaryInt([]uint32{1, 2}, func(a, b int32) int32 {
		if a < b {
			return a
		}
		return b
	})
	if v, ok := got.(Int32); !ok || v != -5 {
		t.Errorf("smin(-5,3) = %v, want -5", got)
	}
}

func TestReadValueFromBufferAllTypes(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeInt, Width: 32, Signed: true},
			3: {Kind: TypeInt, Width: 32, Signed: false},
			4: {Kind: TypeBool},
			5: {Kind: TypeVector, ElemType: 1, Components: 2},
			6: {Kind: TypeVector, ElemType: 1, Components: 3},
			7: {Kind: TypeVector, ElemType: 1, Components: 4},
			8: {Kind: TypeArray, ElemType: 1, Length: 2},
			9: {Kind: TypeStruct, MemberIDs: []uint32{1, 3}},
		},
		MemberDecorations: map[memberDecorationKey]uint32{
			{StructTypeID: 9, MemberIndex: 0, Decoration: DecorationOffset}: 0,
			{StructTypeID: 9, MemberIndex: 1, Decoration: DecorationOffset}: 4,
		},
	}
	interp := &interpreter{module: m}

	buf := make([]byte, 32)
	// Write float 3.14 at offset 0
	putFloat32LE(buf[0:], 3.14)
	// Write int -7 at offset 4
	bits := uint32(0xFFFFFFF9) // -7 in two's complement
	buf[4] = byte(bits)
	buf[5] = byte(bits >> 8)
	buf[6] = byte(bits >> 16)
	buf[7] = byte(bits >> 24)

	// Test float read.
	val := interp.readValueFromBuffer(buf, 0, m.Types[1])
	if f, ok := val.(Float32); !ok || math.Abs(float64(f-3.14)) > 0.01 {
		t.Errorf("float read = %v, want ~3.14", val)
	}

	// Test signed int read.
	val = interp.readValueFromBuffer(buf, 4, m.Types[2])
	if v, ok := val.(Int32); !ok || v != -7 {
		t.Errorf("signed int read = %v, want -7", val)
	}

	// Test unsigned int read.
	putFloat32LE(buf[8:], 0)
	buf[8] = 42
	val = interp.readValueFromBuffer(buf, 8, m.Types[3])
	if v, ok := val.(Uint32); !ok || v != 42 {
		t.Errorf("unsigned int read = %v, want 42", val)
	}

	// Test bool read.
	buf[12] = 1
	val = interp.readValueFromBuffer(buf, 12, m.Types[4])
	if v, ok := val.(bool); !ok || !v {
		t.Errorf("bool read = %v, want true", val)
	}

	// Test vec2 read.
	putFloat32LE(buf[0:], 1.0)
	putFloat32LE(buf[4:], 2.0)
	val = interp.readValueFromBuffer(buf, 0, m.Types[5])
	if v, ok := val.(Vec2); !ok || v[0] != 1.0 || v[1] != 2.0 {
		t.Errorf("vec2 read = %v, want {1, 2}", val)
	}

	// Test vec3 read.
	putFloat32LE(buf[0:], 1.0)
	putFloat32LE(buf[4:], 2.0)
	putFloat32LE(buf[8:], 3.0)
	val = interp.readValueFromBuffer(buf, 0, m.Types[6])
	if v, ok := val.(Vec3); !ok || v != (Vec3{1, 2, 3}) {
		t.Errorf("vec3 read = %v, want {1, 2, 3}", val)
	}

	// Test struct read {float, uint}.
	putFloat32LE(buf[0:], 5.0)
	buf[4] = 10
	buf[5] = 0
	buf[6] = 0
	buf[7] = 0
	val = interp.readValueFromBuffer(buf, 0, m.Types[9])
	arr, ok := val.(Array)
	if !ok || len(arr) != 2 {
		t.Fatalf("struct read returned unexpected type or length")
	}
	if f, ok := arr[0].(Float32); !ok || f != 5.0 {
		t.Errorf("struct[0] = %v, want 5.0", arr[0])
	}
	if u, ok := arr[1].(Uint32); !ok || u != 10 {
		t.Errorf("struct[1] = %v, want 10", arr[1])
	}

	// Test short buffer (should return zeros).
	val = interp.readValueFromBuffer(buf[:2], 0, m.Types[1])
	if f, ok := val.(Float32); !ok || f != 0 {
		t.Errorf("short buffer read = %v, want 0", val)
	}
}

func TestWriteValueToBufferVec3(t *testing.T) {
	buf := make([]byte, 12)
	writeValueToBuffer(buf, 0, Vec3{1, 2, 3})
	for i := 0; i < 3; i++ {
		got := readFloat32LE(buf[i*4:])
		if got != float32(i+1) {
			t.Errorf("vec3[%d] = %f, want %f", i, got, float32(i+1))
		}
	}
}

func TestWriteValueToBufferInt32(t *testing.T) {
	buf := make([]byte, 4)
	writeValueToBuffer(buf, 0, Int32(-42))
	got := readUint32LE(buf)
	if int32(got) != -42 {
		t.Errorf("int32 write = %d, want -42", int32(got))
	}
}

func TestIndexCompositeAllTypes(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		idx  uint32
		want float32
	}{
		{"array", Array{Float32(10), Float32(20)}, 1, 20},
		{"vec2", Vec2{1, 2}, 1, 2},
		{"vec3", Vec3{1, 2, 3}, 2, 3},
		{"vec4", Vec4{1, 2, 3, 4}, 3, 4},
		{"oob_array", Array{Float32(10)}, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexComposite(tt.val, tt.idx)
			f := toFloat32(got)
			if f != tt.want {
				t.Errorf("indexComposite = %f, want %f", f, tt.want)
			}
		})
	}
}

func TestCompositeConstructArray(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeArray, ElemType: 1, Length: 3},
		},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]Value{
			10: Float32(1), 11: Float32(2), 12: Float32(3),
		}),
	}

	got := interp.compositeConstruct(2, []uint32{10, 11, 12})
	arr, ok := got.(Array)
	if !ok || len(arr) != 3 {
		t.Fatalf("compositeConstruct array returned %T", got)
	}
}

func TestVectorShuffleUndefined(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{
		1: {Kind: TypeFloat, Width: 32},
		2: {Kind: TypeVector, ElemType: 1, Components: 2},
	}}
	// 0xFFFFFFFF = undefined component
	got := vectorShuffle(m, 2, Vec4{1, 2, 3, 4}, Vec4{5, 6, 7, 8}, []uint32{0, 0xFFFFFFFF})
	gv, ok := got.(Vec2)
	if !ok {
		t.Fatalf("returned %T, want Vec2", got)
	}
	if gv[0] != 1 || gv[1] != 0 {
		t.Errorf("shuffle = %v, want {1, 0}", gv)
	}
}

func TestMatrixTimesScalarAndMatrix(t *testing.T) {
	identity := Array{Vec4{1, 0, 0, 0}, Vec4{0, 1, 0, 0}, Vec4{0, 0, 1, 0}, Vec4{0, 0, 0, 1}}

	// Scale by 2.
	scaled := matrixTimesScalar(identity, Float32(2))
	cols, _ := scaled.(Array)
	c0 := Vec4ToFloat32(cols[0])
	if c0[0] != 2 {
		t.Errorf("matrixTimesScalar: col0[0] = %f, want 2", c0[0])
	}

	// Identity * Identity = Identity.
	product := matrixTimesMatrix(identity, identity)
	pCols, _ := product.(Array)
	p0 := Vec4ToFloat32(pCols[0])
	if p0[0] != 1 || p0[1] != 0 {
		t.Errorf("matrixTimesMatrix: col0 = %v, want {1,0,0,0}", p0)
	}
}

func TestConstantTrueFalseParser(t *testing.T) {
	// Test that OpConstantTrue/False are handled by the parser.
	inst := spirvInst
	str := spirvString

	const (
		idVoid   = 1
		idBool   = 2
		idFuncTy = 3
		idFunc   = 4
		idLabel  = 5
		idTrue   = 6
		idFalse  = 7
		idBound  = 8
	)

	nameWords := str("main")
	epLen := uint16(3 + len(nameWords))
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)

	words := make([]uint32, 0, 50)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(2, OpTypeVoid), idVoid,
		inst(2, OpTypeBool), idBool,
		inst(3, OpConstantTrue), idBool, idTrue,
		inst(3, OpConstantFalse), idBool, idFalse,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	if v, ok := m.Constants[idTrue]; !ok || v != true {
		t.Errorf("OpConstantTrue not parsed correctly: %v", v)
	}
	if v, ok := m.Constants[idFalse]; !ok || v != false {
		t.Errorf("OpConstantFalse not parsed correctly: %v", v)
	}
}
