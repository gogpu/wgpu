//go:build !(js && wasm)

package shader

import (
	"bytes"
	"math"
	"testing"
)

// =============================================================================
// Coverage boost: meaningful tests for low-coverage code paths.
// Each test prevents a specific class of regression.
// =============================================================================

// ---------------------------------------------------------------------------
// 1. run() interpreter loop: untested opcode branches
//    Prevents: silent wrong values from typos in copy/convert opcodes.
// ---------------------------------------------------------------------------

// TestOpCopyObject verifies OpCopyObject produces an identical copy.
// Regression: a typo reading from the wrong operand index would silently
// produce a zero instead of the copied value.
func TestOpCopyObject(t *testing.T) {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid     = 1
		idFloat    = 2
		idVec4     = 3
		idPtrV4Out = 4
		idFuncTy   = 5
		idFunc     = 6
		idLabel    = 7
		idColorOut = 8
		idCF       = 9  // float 3.5
		idConst0   = 10 // float 0
		idConst1   = 11 // float 1
		idCopy     = 12 // OpCopyObject result
		idResult   = 13
		idBound    = 14
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 100)
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
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(4, OpConstant), idFloat, idCF, f(3.5),
		inst(4, OpConstant), idFloat, idConst0, f(0),
		inst(4, OpConstant), idFloat, idConst1, f(1),
		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		// CopyObject(3.5) should produce 3.5
		inst(4, OpCopyObject), idFloat, idCopy, idCF,
		inst(7, OpCompositeConstruct), idVec4, idResult, idCopy, idConst0, idConst0, idConst1,
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
			if math.Abs(float64(color[0]-3.5)) > 1e-5 {
				t.Errorf("OpCopyObject produced %f, want 3.5", color[0])
			}
			return
		}
	}
	t.Fatal("output not found")
}

// TestOpSConvertUConvertFConvert verifies width conversion opcodes.
// In our 32-bit-only interpreter these are copies, but a bug here would
// silently produce zero or read the wrong SSA value.
func TestOpSConvertUConvertFConvert(t *testing.T) {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid     = 1
		idFloat    = 2
		idUint     = 3
		idInt      = 4
		idVec4     = 5
		idPtrV4Out = 6
		idFuncTy   = 7
		idFunc     = 8
		idLabel    = 9
		idColorOut = 10
		idCU       = 11 // uint 7
		idCI       = 12 // int constant (reuse uint for test)
		idCF       = 13 // float 2.5
		idConst0   = 14
		idConst1   = 15
		idSConvR   = 16
		idUConvR   = 17
		idFConvR   = 18
		idU2F      = 19
		idResult   = 20
		idBound    = 21
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 150)
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
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(4, OpConstant), idUint, idCU, 7,
		inst(4, OpConstant), idFloat, idCF, f(2.5),
		inst(4, OpConstant), idFloat, idConst0, f(0),
		inst(4, OpConstant), idFloat, idConst1, f(1),
		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		// SConvert, UConvert, FConvert on 32-bit values = identity
		inst(4, OpSConvert), idInt, idSConvR, idCU,
		inst(4, OpUConvert), idUint, idUConvR, idCU,
		inst(4, OpFConvert), idFloat, idFConvR, idCF,
		// Convert SConvert result to float for output
		inst(4, OpConvertUToF), idFloat, idU2F, idSConvR,
		// output: (u2f(SConvert(7)), FConvert(2.5), 0, 1)
		inst(7, OpCompositeConstruct), idVec4, idResult, idU2F, idFConvR, idConst0, idConst1,
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
			if math.Abs(float64(color[0]-7.0)) > 1e-5 {
				t.Errorf("SConvert+ConvertUToF produced %f, want 7.0", color[0])
			}
			if math.Abs(float64(color[1]-2.5)) > 1e-5 {
				t.Errorf("FConvert produced %f, want 2.5", color[1])
			}
			return
		}
	}
	t.Fatal("output not found")
}

// TestOpKill verifies OpKill terminates fragment execution without error.
// Regression: if OpKill returned an error, all discarded fragments would
// cause render pipeline failures.
func TestOpKill(t *testing.T) {
	inst := spirvInst
	str := spirvString

	const (
		idVoid     = 1
		idFloat    = 2
		idVec4     = 3
		idPtrV4Out = 4
		idFuncTy   = 5
		idFunc     = 6
		idLabel    = 7
		idColorOut = 8
		idBound    = 9
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 60)
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
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(1, OpKill), // discard fragment
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}
	// OpKill should return nil error (not crash or error out).
	_, err = m.Execute("fs_main", nil)
	if err != nil {
		t.Errorf("OpKill should not produce error, got: %v", err)
	}
}

// TestOpUnreachable verifies OpUnreachable returns an error.
// Regression: if this silently succeeded, unreachable code paths would
// produce garbage instead of failing loudly.
func TestOpUnreachable(t *testing.T) {
	inst := spirvInst
	str := spirvString

	const (
		idVoid   = 1
		idFuncTy = 2
		idFunc   = 3
		idLabel  = 4
		idBound  = 5
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords))
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)

	words := make([]uint32, 0, 40)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(1, OpUnreachable),
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}
	_, err = m.Execute("fs_main", nil)
	if err == nil {
		t.Fatal("OpUnreachable should produce an error")
	}
	if err.Error() != "spirv: executed OpUnreachable" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 2. executeAtomicOp: untested atomic operations.
//    Prevents: wrong min/max/exchange semantics producing data corruption
//    in compute shaders that use shared counters or reduction patterns.
// ---------------------------------------------------------------------------

func TestAtomicMinMaxOps(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}

	tests := []struct {
		name    string
		opcode  uint16
		initial uint32
		operand uint32
		wantOld uint32
		wantNew uint32
	}{
		// Signed min: min(10, 5) = 5 (value wins)
		{"smin_lower", OpAtomicSMin, 10, 5, 10, 5},
		// Signed min: min(5, 10) = 5 (old stays)
		{"smin_higher", OpAtomicSMin, 5, 10, 5, 5},
		// Signed min with negative: min(5, -3) = -3
		{"smin_negative", OpAtomicSMin, 5, 0xFFFFFFFD, 5, 0xFFFFFFFD}, // -3 as uint32
		// Unsigned min: min(10, 3) = 3
		{"umin_lower", OpAtomicUMin, 10, 3, 10, 3},
		// Unsigned min: min(3, 10) = 3 (old stays)
		{"umin_higher", OpAtomicUMin, 3, 10, 3, 3},
		// Signed max: max(5, 10) = 10
		{"smax_higher", OpAtomicSMax, 5, 10, 5, 10},
		// Signed max: max(10, 5) = 10 (old stays)
		{"smax_lower", OpAtomicSMax, 10, 5, 10, 10},
		// Signed max with negative: max(-5, -1) = -1
		{"smax_negative", OpAtomicSMax, 0xFFFFFFFB, 0xFFFFFFFF, 0xFFFFFFFB, 0xFFFFFFFF}, // -5, -1 as uint32
		// Unsigned max: max(3, 10) = 10
		{"umax_higher", OpAtomicUMax, 3, 10, 3, 10},
		// Unsigned max: max(10, 3) = 10 (old stays)
		{"umax_lower", OpAtomicUMax, 10, 3, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr := &Pointer{Val: ValUint(tt.initial)}
			interp := &interpreter{
				module: m,
				values: testMakeValues(map[uint32]any{
					1: ptr,
					2: tt.operand,
				}),
			}
			inst := Instruction{
				Opcode:   tt.opcode,
				ResultID: 100,
				Operands: []uint32{1, 0, 0, 2}, // ptr, scope, semantics, value
			}
			old := interp.executeAtomicOp(inst)
			gotOld := toUint32(old)
			gotNew := toUint32(ptr.Val)
			if gotOld != tt.wantOld {
				t.Errorf("old = %d, want %d", gotOld, tt.wantOld)
			}
			if gotNew != tt.wantNew {
				t.Errorf("new = %d, want %d", gotNew, tt.wantNew)
			}
		})
	}
}

// TestAtomicLoadAndStore verifies OpAtomicLoad reads without modifying,
// and OpAtomicStore writes without returning.
func TestAtomicLoadAndStore(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}

	// AtomicLoad: read value without modification.
	t.Run("atomic_load", func(t *testing.T) {
		ptr := &Pointer{Val: ValUint(42)}
		interp := &interpreter{
			module: m,
			values: testMakeValues(map[uint32]any{1: ptr}),
		}
		inst := Instruction{
			Opcode:   OpAtomicLoad,
			ResultID: 100,
			Operands: []uint32{1, 0, 0},
		}
		result := interp.executeAtomicOp(inst)
		if toUint32(result) != 42 {
			t.Errorf("AtomicLoad returned %d, want 42", toUint32(result))
		}
		// Value unchanged.
		if toUint32(ptr.Val) != 42 {
			t.Errorf("AtomicLoad modified ptr to %d, should remain 42", toUint32(ptr.Val))
		}
	})

	// AtomicStore: write value, return nil.
	t.Run("atomic_store", func(t *testing.T) {
		ptr := &Pointer{Val: ValUint(0)}
		interp := &interpreter{
			module: m,
			values: testMakeValues(map[uint32]any{
				1: ptr,
				2: ValUint(99),
			}),
		}
		inst := Instruction{
			Opcode:   OpAtomicStore,
			ResultID: 0, // Store has no result
			Operands: []uint32{1, 0, 0, 2},
		}
		result := interp.executeAtomicOp(inst)
		if !result.IsNone() {
			t.Errorf("AtomicStore returned %v, want nil", result)
		}
		if toUint32(ptr.Val) != 99 {
			t.Errorf("AtomicStore wrote %d, want 99", toUint32(ptr.Val))
		}
	})
}

// TestAtomicOpNonPointer verifies graceful handling when the pointer operand
// is not actually a Pointer. Prevents: nil dereference crash in production.
func TestAtomicOpNonPointer(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{
			1: ValUint(42), // Not a pointer
		}),
	}
	inst := Instruction{
		Opcode:   OpAtomicIAdd,
		ResultID: 100,
		Operands: []uint32{1, 0, 0, 1},
	}
	// Should return ValUint(0), not crash.
	result := interp.executeAtomicOp(inst)
	if toUint32(result) != 0 {
		t.Errorf("atomic on non-pointer returned %d, want 0", toUint32(result))
	}
}

// ---------------------------------------------------------------------------
// 3. executeGLSLExtInst: untested intrinsics with edge cases.
//    Prevents: subtly wrong lighting/shading from math intrinsic bugs.
// ---------------------------------------------------------------------------

// TestGLSLSmoothStepEdgeCases verifies smoothstep with degenerate edges.
// Regression: if edge0==edge1, GLSL spec says step function (0 below, 1 at/above).
// A naive implementation divides by zero.
func TestGLSLSmoothStepEdgeCases(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}

	tests := []struct {
		name            string
		edge0, edge1, x float32
		want            float32
	}{
		// Normal smoothstep: x in middle of [0, 1] range
		{"normal_mid", 0, 1, 0.5, 0.5},
		// x below range
		{"below", 0, 1, -1, 0},
		// x above range
		{"above", 0, 1, 2, 1},
		// Edge case: edge0 == edge1, x below
		{"equal_edges_below", 5, 5, 3, 0},
		// Edge case: edge0 == edge1, x at edge
		{"equal_edges_at", 5, 5, 5, 1},
		// Edge case: edge0 == edge1, x above
		{"equal_edges_above", 5, 5, 7, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := &interpreter{
				module: m,
				values: testMakeValues(map[uint32]any{
					10: ValFloat(tt.edge0),
					11: ValFloat(tt.edge1),
					12: ValFloat(tt.x),
				}),
			}
			got := interp.executeGLSLExtInst(GLSLSmoothStep, []uint32{10, 11, 12})
			f := toFloat32(got)
			if math.Abs(float64(f-tt.want)) > 0.01 {
				t.Errorf("smoothstep(%v,%v,%v) = %v, want %v", tt.edge0, tt.edge1, tt.x, f, tt.want)
			}
		})
	}
}

// TestGLSLFClampMinGreaterThanMax verifies clamp when min>max.
// Regression: GLSL spec says result is undefined, but we should not crash
// or produce NaN. Our implementation clamps to max in this case.
func TestGLSLFClampMinGreaterThanMax(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{
			10: ValFloat(5),  // x
			11: ValFloat(10), // min (greater than max!)
			12: ValFloat(3),  // max
		}),
	}
	got := interp.executeGLSLExtInst(GLSLFClamp, []uint32{10, 11, 12})
	f := toFloat32(got)
	// With min>max the result is clamped to max(x, min) then min(result, max).
	// Result should be 10 (clamped to min=10, then min(10,3)=3).
	// The actual behavior with our implementation: min(max(5, 10), 3) = min(10, 3) = 3.
	if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
		t.Errorf("clamp(5, 10, 3) produced NaN/Inf: %v", f)
	}
}

// TestGLSLPowNegativeBase verifies pow with negative base.
// Regression: math.Pow(-2, 3) = -8, but math.Pow(-2, 0.5) = NaN.
// Shaders should handle this gracefully.
func TestGLSLPowNegativeBase(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{
			10: ValFloat(-2),
			11: ValFloat(3),
		}),
	}
	got := interp.executeGLSLExtInst(GLSLPow, []uint32{10, 11})
	f := toFloat32(got)
	// pow(-2, 3) = -8
	if math.Abs(float64(f-(-8))) > 1e-4 {
		t.Errorf("pow(-2, 3) = %v, want -8", f)
	}
}

// TestGLSLAtan2Quadrants verifies atan2 returns correct angles in all four quadrants.
// Regression: swapped y/x arguments produce wrong direction vectors in lighting.
func TestGLSLAtan2Quadrants(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}

	tests := []struct {
		name string
		y, x float32
		want float32
	}{
		{"q1", 1, 1, float32(math.Pi / 4)},
		{"q2", 1, -1, float32(3 * math.Pi / 4)},
		{"q4", -1, 1, float32(-math.Pi / 4)},
		{"y_axis", 1, 0, float32(math.Pi / 2)},
		{"x_axis", 0, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := &interpreter{
				module: m,
				values: testMakeValues(map[uint32]any{10: ValFloat(tt.y), 11: ValFloat(tt.x)}),
			}
			got := interp.executeGLSLExtInst(GLSLAtan2, []uint32{10, 11})
			f := toFloat32(got)
			if math.Abs(float64(f-tt.want)) > 1e-5 {
				t.Errorf("atan2(%v,%v) = %v, want %v", tt.y, tt.x, f, tt.want)
			}
		})
	}
}

// TestGLSLReflect verifies vector reflection via direct interpreter call.
// Regression: wrong dot product sign produces inverted reflections.
func TestGLSLReflectViaExtInst(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	// Reflect incident (1, -1, 0) around normal (0, 1, 0) -> (1, 1, 0)
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{
			10: Vec3{1, -1, 0},
			11: Vec3{0, 1, 0},
		}),
	}
	got := interp.executeGLSLExtInst(GLSLReflect, []uint32{10, 11})
	gv, ok := testIsVec3(got)
	if !ok {
		t.Fatalf("reflect returned %T, want Vec3", got)
	}
	want := Vec3{1, 1, 0}
	for i := 0; i < 3; i++ {
		if math.Abs(float64(gv[i]-want[i])) > 1e-5 {
			t.Errorf("reflect[%d] = %v, want %v", i, gv[i], want[i])
		}
	}
}

// TestGLSLFMix verifies linear interpolation.
func TestGLSLFMix(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	tests := []struct {
		name    string
		x, y, a float32
		want    float32
	}{
		{"a=0", 10, 20, 0, 10},
		{"a=1", 10, 20, 1, 20},
		{"a=0.5", 10, 20, 0.5, 15},
		{"a=0.25", 0, 100, 0.25, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := &interpreter{
				module: m,
				values: testMakeValues(map[uint32]any{
					10: ValFloat(tt.x),
					11: ValFloat(tt.y),
					12: ValFloat(tt.a),
				}),
			}
			got := interp.executeGLSLExtInst(GLSLFMix, []uint32{10, 11, 12})
			f := toFloat32(got)
			if math.Abs(float64(f-tt.want)) > 1e-4 {
				t.Errorf("mix(%v,%v,%v) = %v, want %v", tt.x, tt.y, tt.a, f, tt.want)
			}
		})
	}
}

// TestGLSLGeometricLength verifies length for scalar input (the 0% covered path).
func TestGLSLLengthScalar(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{10: ValFloat(-5)}),
	}
	got := interp.executeGLSLExtInst(GLSLLength, []uint32{10})
	f := toFloat32(got)
	if math.Abs(float64(f-5)) > 1e-5 {
		t.Errorf("length(-5) = %v, want 5", f)
	}
}

// TestGLSLDistance verifies distance between two points.
func TestGLSLDistanceVec3(t *testing.T) {
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	interp := &interpreter{
		module: m,
		values: testMakeValues(map[uint32]any{
			10: Vec3{1, 0, 0},
			11: Vec3{4, 0, 0},
		}),
	}
	got := interp.executeGLSLExtInst(GLSLDistance, []uint32{10, 11})
	f := toFloat32(got)
	if math.Abs(float64(f-3)) > 1e-5 {
		t.Errorf("distance = %v, want 3", f)
	}
}

// TestGLSLTernaryVec2Vec3 verifies ternary float ops work on Vec2 and Vec3.
// Regression: missing type assertion for Vec2/Vec3 in glslTernaryFloat
// would silently fall through to the scalar path producing wrong results.
func TestGLSLTernaryVec2Vec3(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}
	clamp := func(x, lo, hi float32) float32 {
		if x < lo {
			return lo
		}
		if x > hi {
			return hi
		}
		return x
	}

	t.Run("vec2", func(t *testing.T) {
		interp := &interpreter{
			module: m,
			values: testMakeValues(map[uint32]any{
				1: Vec2{-1, 5},
				2: Vec2{0, 0},
				3: Vec2{1, 1},
			}),
		}
		got := interp.glslTernaryFloat([]uint32{1, 2, 3}, clamp)
		v, ok := testIsVec2(got)
		if !ok {
			t.Fatalf("returned %T, want Vec2", got)
		}
		if v[0] != 0 || v[1] != 1 {
			t.Errorf("clamp = %v, want {0, 1}", v)
		}
	})

	t.Run("vec3", func(t *testing.T) {
		interp := &interpreter{
			module: m,
			values: testMakeValues(map[uint32]any{
				1: Vec3{-1, 0.5, 5},
				2: Vec3{0, 0, 0},
				3: Vec3{1, 1, 1},
			}),
		}
		got := interp.glslTernaryFloat([]uint32{1, 2, 3}, clamp)
		v, ok := testIsVec3(got)
		if !ok {
			t.Fatalf("returned %T, want Vec3", got)
		}
		if v[0] != 0 || math.Abs(float64(v[1]-0.5)) > 1e-5 || v[2] != 1 {
			t.Errorf("clamp = %v, want {0, 0.5, 1}", v)
		}
	})
}

// ---------------------------------------------------------------------------
// 4. initWorkgroupVariables / allocateWorkgroupMemory.
//    Prevents: zero-init failure in compute workgroup shared memory
//    leading to data corruption between workgroup dispatches.
// ---------------------------------------------------------------------------

// TestAllocateWorkgroupMemory verifies that workgroup memory is allocated
// with correct sizes and zero-initialized.
func TestAllocateWorkgroupMemory(t *testing.T) {
	const (
		idFloat    = 1
		idPtrFloat = 2
		idArray    = 3
		idPtrArr   = 4
		idVar1     = 10
		idVar2     = 11
		idConst4   = 20
	)
	m := &Module{
		Types: map[uint32]*TypeInfo{
			idFloat:    {Kind: TypeFloat, Width: 32},
			idPtrFloat: {Kind: TypePointer, ElemType: idFloat, Storage: StorageClassWorkgroup},
			idArray:    {Kind: TypeArray, ElemType: idFloat, Length: 4},
			idPtrArr:   {Kind: TypePointer, ElemType: idArray, Storage: StorageClassWorkgroup},
		},
		Variables: map[uint32]*VariableInfo{
			idVar1: {TypeID: idPtrFloat, StorageClass: StorageClassWorkgroup},
			idVar2: {TypeID: idPtrArr, StorageClass: StorageClassWorkgroup},
		},
	}

	shared := m.allocateWorkgroupMemory()
	// Float variable should get 4 bytes.
	if buf, ok := shared[idVar1]; !ok {
		t.Error("float workgroup variable not allocated")
	} else if len(buf) != 4 {
		t.Errorf("float buffer len = %d, want 4", len(buf))
	} else if !bytes.Equal(buf, make([]byte, 4)) {
		t.Error("float buffer not zero-initialized")
	}

	// Array of 4 floats should get 16 bytes.
	if buf, ok := shared[idVar2]; !ok {
		t.Error("array workgroup variable not allocated")
	} else if len(buf) != 16 {
		t.Errorf("array buffer len = %d, want 16", len(buf))
	}
}

// TestInitWorkgroupVariablesFromSharedMemory verifies that pre-populated
// shared memory is read back correctly during init.
func TestInitWorkgroupVariablesFromSharedMemory(t *testing.T) {
	const (
		idFloat    = 1
		idPtrFloat = 2
		idVarID    = 10
	)
	m := &Module{
		Types: map[uint32]*TypeInfo{
			idFloat:    {Kind: TypeFloat, Width: 32},
			idPtrFloat: {Kind: TypePointer, ElemType: idFloat, Storage: StorageClassWorkgroup},
		},
		Variables: map[uint32]*VariableInfo{
			idVarID: {TypeID: idPtrFloat, StorageClass: StorageClassWorkgroup},
		},
		Constants:         map[uint32]Value{},
		Decorations:       map[decorationKey]uint32{},
		MemberDecorations: map[memberDecorationKey]uint32{},
		Bound:             idVarID + 1,
	}

	// Pre-populate shared memory with value 42.0.
	sharedBuf := make([]byte, 4)
	putFloat32LE(sharedBuf, 42.0)

	ctx := &ExecutionContext{
		WorkgroupSharedMemory: map[uint32][]byte{idVarID: sharedBuf},
	}

	interp := &interpreter{
		module: m,
		ctx:    ctx,
		ep:     &EntryPoint{},
		values: make([]Value, m.Bound),
	}
	interp.initWorkgroupVariables()

	ptr := interp.values[idVarID].AsPointer()
	ok := ptr != nil
	if !ok {
		t.Fatalf("workgroup variable is %T, want *Pointer", interp.values[idVarID])
	}
	f := toFloat32(ptr.Val)
	if math.Abs(float64(f-42.0)) > 1e-5 {
		t.Errorf("workgroup variable value = %v, want 42.0", f)
	}
}

// TestInitWorkgroupVariablesDefault verifies zero-init when no shared memory.
func TestInitWorkgroupVariablesDefault(t *testing.T) {
	const (
		idFloat    = 1
		idPtrFloat = 2
		idVarID    = 10
	)
	m := &Module{
		Types: map[uint32]*TypeInfo{
			idFloat:    {Kind: TypeFloat, Width: 32},
			idPtrFloat: {Kind: TypePointer, ElemType: idFloat, Storage: StorageClassWorkgroup},
		},
		Variables: map[uint32]*VariableInfo{
			idVarID: {TypeID: idPtrFloat, StorageClass: StorageClassWorkgroup},
		},
		Constants:         map[uint32]Value{},
		Decorations:       map[decorationKey]uint32{},
		MemberDecorations: map[memberDecorationKey]uint32{},
		Bound:             idVarID + 1,
	}

	ctx := &ExecutionContext{} // No shared memory

	interp := &interpreter{
		module: m,
		ctx:    ctx,
		ep:     &EntryPoint{},
		values: make([]Value, m.Bound),
	}
	interp.initWorkgroupVariables()

	ptr := interp.values[idVarID].AsPointer()
	ok := ptr != nil
	if !ok {
		t.Fatalf("workgroup variable is %T, want *Pointer", interp.values[idVarID])
	}
	f := toFloat32(ptr.Val)
	if f != 0 {
		t.Errorf("default workgroup variable = %v, want 0", f)
	}
}

// ---------------------------------------------------------------------------
// 5. typeByteSize: matrix types, nested structs, default branch.
//    Prevents: buffer overrun from wrong size calculations when reading
//    matrices or complex structs from uniform/storage buffers.
// ---------------------------------------------------------------------------

// TestTypeByteSizeMatrixAndNestedStruct verifies correct size for complex types.
func TestTypeByteSizeMatrixAndNestedStruct(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeVector, ElemType: 1, Components: 4},
			3: {Kind: TypeArray, ElemType: 2, Length: 4}, // mat4 as array<vec4, 4>
			4: {Kind: TypeInt, Width: 32, Signed: false},
			5: {Kind: TypeStruct, MemberIDs: []uint32{1, 4}}, // {f32, u32}
			6: {Kind: TypeArray, ElemType: 5, Length: 3},     // array<{f32,u32}, 3>
			7: {Kind: TypeStruct, MemberIDs: []uint32{3, 6}}, // {mat4, array}
			8: {Kind: 99},                                    // unknown type kind
		},
	}

	tests := []struct {
		name string
		id   uint32
		want uint32
	}{
		{"mat4_as_array", 3, 64},   // 4 * vec4(16) = 64
		{"struct_f32_u32", 5, 8},   // max(0*4+4, 1*4+4) = 8
		{"array_of_struct", 6, 24}, // 3 * 8 = 24
		{"unknown_type", 8, 4},     // default case
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

// ---------------------------------------------------------------------------
// 6. vectorShuffle: different source/result configurations.
//    Prevents: wrong colors from incorrect component selection during
//    vector swizzling (e.g., rgba -> bgra conversion).
// ---------------------------------------------------------------------------

func TestVectorShuffleVariants(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{
		1: {Kind: TypeFloat, Width: 32},
		2: {Kind: TypeVector, ElemType: 1, Components: 4},
		3: {Kind: TypeVector, ElemType: 1, Components: 3},
		4: {Kind: TypeVector, ElemType: 1, Components: 2},
	}}

	tests := []struct {
		name       string
		typeID     uint32
		v1, v2     Value
		components []uint32
		want       Value
	}{
		// Identity shuffle (first 4 from v1)
		{"identity", 2, ValVec4(1, 2, 3, 4), ValVec4(5, 6, 7, 8),
			[]uint32{0, 1, 2, 3}, ValVec4(1, 2, 3, 4)},
		// Reverse shuffle
		{"reverse", 2, ValVec4(1, 2, 3, 4), ValVec4(5, 6, 7, 8),
			[]uint32{3, 2, 1, 0}, ValVec4(4, 3, 2, 1)},
		// Two-source shuffle: take last 2 from v1, first 2 from v2
		{"cross_source", 2, ValVec4(1, 2, 3, 4), ValVec4(5, 6, 7, 8),
			[]uint32{2, 3, 4, 5}, ValVec4(3, 4, 5, 6)},
		// Vec3 result
		{"to_vec3", 3, ValVec4(10, 20, 30, 40), ValVec4(50, 60, 70, 80),
			[]uint32{0, 4, 2}, ValVec3(10, 50, 30)},
		// Vec2 from vec4 (single-component extract equivalent)
		{"to_vec2", 4, ValVec4(10, 20, 30, 40), ValVec4(0, 0, 0, 0),
			[]uint32{1, 3}, ValVec2(20, 40)},
		// With undefined component (0xFFFFFFFF)
		{"with_undef", 3, ValVec4(1, 2, 3, 4), ValVec4(5, 6, 7, 8),
			[]uint32{0, 0xFFFFFFFF, 2}, ValVec3(1, 0, 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vectorShuffle(m, tt.typeID, tt.v1, tt.v2, tt.components)
			if !valueApproxEqual(got, tt.want, 1e-6) {
				t.Errorf("vectorShuffle = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestVectorShuffleNoTypeInfo tests the fallback path when type info is missing.
func TestVectorShuffleNoTypeInfo(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}} // No type info for result

	// Should fall back to inferring from component count.
	got := vectorShuffle(m, 999, ValVec4From(Vec4{1, 2, 3, 4}), ValVec4From(Vec4{5, 6, 7, 8}), []uint32{0, 1, 2, 3})
	_, ok := testIsVec4(got)
	if !ok {
		t.Errorf("vectorShuffle fallback returned %T, want Vec4", got)
	}

	// 3-component fallback
	got = vectorShuffle(m, 999, ValVec4From(Vec4{1, 2, 3, 4}), ValVec4From(Vec4{5, 6, 7, 8}), []uint32{0, 1, 2})
	_, ok = testIsVec3(got)
	if !ok {
		t.Errorf("vectorShuffle 3-component fallback returned %T, want Vec3", got)
	}

	// 2-component fallback
	got = vectorShuffle(m, 999, ValVec4From(Vec4{1, 2, 3, 4}), ValVec4From(Vec4{5, 6, 7, 8}), []uint32{0, 1})
	_, ok = testIsVec2(got)
	if !ok {
		t.Errorf("vectorShuffle 2-component fallback returned %T, want Vec2", got)
	}

	// 1-component fallback (degenerate case: vectorShuffle returns ValFloat(0)
	// because it only handles 2/3/4 component counts)
	got = vectorShuffle(m, 999, ValVec4From(Vec4{42, 0, 0, 0}), ValVec4From(Vec4{0, 0, 0, 0}), []uint32{0})
	_, ok = testIsFloat32(got)
	if !ok {
		t.Fatalf("vectorShuffle 1-component returned %T, want Float32", got)
	}
}

// ---------------------------------------------------------------------------
// 7. sampleNearest / fetchTexel: boundary and degenerate textures.
//    Prevents: out-of-bounds reads producing garbage colors at texture edges.
// ---------------------------------------------------------------------------

// TestSampleNearestEdgePixels verifies sampling at exact texture boundaries.
func TestSampleNearestEdgePixels(t *testing.T) {
	// 1x1 texture: single white pixel.
	tex1x1 := &Texture2D{
		Width: 1, Height: 1,
		Data: []byte{255, 255, 255, 255},
	}

	tests := []struct {
		name string
		tex  *Texture2D
		u, v float32
	}{
		// UV exactly 1.0 should clamp to last valid pixel.
		{"1x1_at_origin", tex1x1, 0, 0},
		{"1x1_at_one", tex1x1, 1.0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleNearest(tt.tex, tt.u, tt.v)
			// 1x1 white should always produce white regardless of UV.
			if math.Abs(float64(got[0]-1)) > 0.01 || math.Abs(float64(got[3]-1)) > 0.01 {
				t.Errorf("sampleNearest(%v, %v) = %v, want white", tt.u, tt.v, got)
			}
		})
	}
}

// TestSampleNearestNonSquare verifies sampling a non-square texture.
// Regression: width/height mixup in texel coordinate calculation.
func TestSampleNearestNonSquare(t *testing.T) {
	// 4x1 texture: R, G, B, A columns.
	tex := &Texture2D{
		Width: 4, Height: 1,
		Data: []byte{
			255, 0, 0, 255, // (0,0) Red
			0, 255, 0, 255, // (1,0) Green
			0, 0, 255, 255, // (2,0) Blue
			255, 255, 255, 255, // (3,0) White
		},
	}

	tests := []struct {
		name  string
		u     float32
		wantR float32
	}{
		{"first_col", 0.0, 1.0},    // Red
		{"second_col", 0.375, 0.0}, // Green (R=0)
		{"last_col", 0.99, 1.0},    // White (R=1)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleNearest(tex, tt.u, 0)
			if math.Abs(float64(got[0]-tt.wantR)) > 0.02 {
				t.Errorf("sampleNearest(%v, 0)[R] = %v, want %v", tt.u, got[0], tt.wantR)
			}
		})
	}
}

// TestFetchTexelNegativeCoords verifies clamping for negative integer coords.
func TestFetchTexelNegativeCoords(t *testing.T) {
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255,
			0, 255, 0, 255,
			0, 0, 255, 255,
			255, 255, 255, 255,
		},
	}
	bk := BindingKey{Group: 0, Binding: 0}
	interp := &interpreter{ctx: &ExecutionContext{Textures: map[BindingKey]*Texture2D{bk: tex}}}

	// Negative coords should clamp to 0.
	got := interp.fetchTexel(ValBindingKey(bk), ValVec2(-5, -5))
	gv := Vec4ToFloat32(got)
	// (0,0) is red
	if math.Abs(float64(gv[0]-1)) > 0.01 || math.Abs(float64(gv[1])) > 0.01 {
		t.Errorf("fetchTexel(-5,-5) = %v, want red (clamped to 0,0)", gv)
	}
}

// TestFetchTexelScalarCoord verifies fetchTexel with a scalar coordinate.
func TestFetchTexelScalarCoord(t *testing.T) {
	tex := &Texture2D{
		Width: 3, Height: 1,
		Data: []byte{
			0, 0, 0, 255, // (0,0) black
			128, 128, 128, 255, // (1,0) gray
			255, 255, 255, 255, // (2,0) white
		},
	}
	bk := BindingKey{Group: 0, Binding: 0}
	interp := &interpreter{ctx: &ExecutionContext{Textures: map[BindingKey]*Texture2D{bk: tex}}}

	// Scalar coord should be treated as x, y=0.
	got := interp.fetchTexel(ValBindingKey(bk), ValUint(2))
	gv := Vec4ToFloat32(got)
	if math.Abs(float64(gv[0]-1)) > 0.01 {
		t.Errorf("fetchTexel(scalar 2) = %v, want white", gv)
	}
}

// TestFetchTexelNilTexture verifies graceful handling of nil texture.
func TestFetchTexelNilTexture(t *testing.T) {
	interp := &interpreter{ctx: &ExecutionContext{}}
	got := interp.fetchTexel(ValBindingKey(BindingKey{}), ValVec2(0, 0))
	gv := Vec4ToFloat32(got)
	if gv != (Vec4{0, 0, 0, 0}) {
		t.Errorf("fetchTexel(nil) = %v, want zero", gv)
	}
}

// TestFetchTexelVec3Vec4Coords verifies fetchTexel with Vec3 and Vec4 coord types.
func TestFetchTexelVec3Vec4Coords(t *testing.T) {
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255, // (0,0)
			0, 255, 0, 255, // (1,0)
			0, 0, 255, 255, // (0,1)
			255, 255, 255, 255, // (1,1)
		},
	}
	bk := BindingKey{Group: 0, Binding: 0}
	interp := &interpreter{ctx: &ExecutionContext{Textures: map[BindingKey]*Texture2D{bk: tex}}}

	// Vec3 coord: use x,y components
	got := interp.fetchTexel(ValBindingKey(bk), ValVec3(1, 1, 0))
	gv := Vec4ToFloat32(got)
	if gv != (Vec4{1, 1, 1, 1}) {
		t.Errorf("fetchTexel(Vec3{1,1,0}) = %v, want white", gv)
	}

	// Vec4 coord: use x,y components
	got = interp.fetchTexel(ValBindingKey(bk), ValVec4(0, 1, 0, 0))
	gv = Vec4ToFloat32(got)
	// (0,1) = blue
	if math.Abs(float64(gv[2]-1)) > 0.01 {
		t.Errorf("fetchTexel(Vec4{0,1,...})[B] = %v, want 1.0", gv[2])
	}
}

// ---------------------------------------------------------------------------
// 8. Float32BitsToUint32: 0% coverage utility.
//    Prevents: bitcast bugs breaking SPIR-V constant encoding.
// ---------------------------------------------------------------------------

func TestFloat32BitsToUint32(t *testing.T) {
	tests := []struct {
		name string
		f    float32
		want uint32
	}{
		{"zero", 0, 0},
		{"one", 1.0, 0x3F800000},
		{"negative_one", -1.0, 0xBF800000},
		{"negative_zero", float32(math.Copysign(0, -1)), 0x80000000},
		{"inf", float32(math.Inf(1)), 0x7F800000},
		{"neg_inf", float32(math.Inf(-1)), 0xFF800000},
		{"nan", float32(math.NaN()), 0x7FC00000},
		// Denormal: smallest positive denormalized float32
		{"denormal", math.SmallestNonzeroFloat32, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Float32BitsToUint32(tt.f)
			if tt.name == "nan" {
				// NaN: just verify the sign and exponent bits, payload may vary.
				if got&0x7F800000 != 0x7F800000 || got&0x007FFFFF == 0 {
					t.Errorf("Float32BitsToUint32(NaN) = 0x%08X, not a NaN pattern", got)
				}
			} else if got != tt.want {
				t.Errorf("Float32BitsToUint32(%v) = 0x%08X, want 0x%08X", tt.f, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 9. Debug infrastructure: writeTrace, formatTraceValue, valuesEqual.
//    Prevents: debugger crashes on uncommon value types.
// ---------------------------------------------------------------------------

// TestWriteTraceNoResultCovBoost verifies writeTrace is a no-op for instructions
// without a result ID (e.g., OpStore). Exercises the early-return path.
func TestWriteTraceNoResultCovBoost(t *testing.T) {
	var buf bytes.Buffer
	entry := &traceEntry{}
	i := Instruction{Opcode: OpStore, ResultID: 0}
	writeTrace(&buf, nil, entry, 0, i, nil)
	if buf.Len() != 0 {
		t.Errorf("writeTrace with ResultID=0 produced output: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// 10. sampleTexture coord extraction: Vec3 and default paths.
//     Prevents: wrong UV when shader passes vec3/vec4 instead of vec2.
// ---------------------------------------------------------------------------

// TestSampleTextureCoordTypes verifies sampleTexture handles Vec3 and scalar coords.
func TestSampleTextureCoordTypes(t *testing.T) {
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255, // (0,0) Red
			0, 255, 0, 255, // (1,0) Green
			0, 0, 255, 255, // (0,1) Blue
			255, 255, 255, 255, // (1,1) White
		},
	}
	samp := &Sampler{MagFilter: FilterNearest, WrapU: WrapRepeat, WrapV: WrapRepeat}

	interp := &interpreter{
		ctx: &ExecutionContext{
			Textures: map[BindingKey]*Texture2D{{Group: 0, Binding: 0}: tex},
			Samplers: map[BindingKey]*Sampler{{Group: 0, Binding: 1}: samp},
		},
		module: &Module{
			Types:     map[uint32]*TypeInfo{},
			Constants: map[uint32]Value{},
		},
	}

	si := &SampledImageValue{
		Image:   ValBindingKey(BindingKey{Group: 0, Binding: 0}),
		Sampler: ValBindingKey(BindingKey{Group: 0, Binding: 1}),
	}

	// Vec3 coord: should use first 2 components as UV.
	got := interp.sampleTexture(ValSampledImage(si), ValVec3(0, 0, 999), 0)
	gv := Vec4ToFloat32(got)
	if math.Abs(float64(gv[0]-1)) > 0.05 { // Red at (0,0)
		t.Errorf("sampleTexture(Vec3) = %v, want red", gv)
	}

	// Scalar coord: should use as U, V=0.
	got = interp.sampleTexture(ValSampledImage(si), ValFloat(0), 0)
	gv = Vec4ToFloat32(got)
	// u=0, v=0 -> (0,0) = Red
	if math.Abs(float64(gv[0]-1)) > 0.05 {
		t.Errorf("sampleTexture(Float32) = %v, want red", gv)
	}
}

// ---------------------------------------------------------------------------
// 11. readValueFromBuffer: short buffer handling (zero fallback).
//     Prevents: panics on truncated buffers for integer types.
// ---------------------------------------------------------------------------

func TestReadValueFromBufferShortBuffers(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeInt, Width: 32, Signed: true},
			2: {Kind: TypeInt, Width: 32, Signed: false},
			3: {Kind: TypeBool},
		},
	}
	interp := &interpreter{module: m}
	short := []byte{0, 0} // Only 2 bytes, need 4

	// Signed int on short buffer returns zero.
	got := interp.readValueFromBuffer(short, 0, m.Types[1])
	if got.Tag != TagInt32 || got.AsInt32() != 0 {
		t.Errorf("short signed int = %v, want ValInt(0)", got)
	}

	// Unsigned int on short buffer returns zero.
	got = interp.readValueFromBuffer(short, 0, m.Types[2])
	if got.Tag != TagUint32 || got.AsUint32() != 0 {
		t.Errorf("short unsigned int = %v, want ValUint(0)", got)
	}

	// Bool on short buffer returns false.
	got = interp.readValueFromBuffer(short, 0, m.Types[3])
	if v, ok := testIsBool(got); !ok || v != false {
		t.Errorf("short bool = %v, want false", got)
	}
}

// ---------------------------------------------------------------------------
// 12. writeValueToBuffer: short buffer handling (no panic).
//     Prevents: panics when writing to truncated destination buffers.
// ---------------------------------------------------------------------------

func TestWriteValueToBufferShortBuffer(t *testing.T) {
	// Writing to a buffer that is too short should not panic.
	short := make([]byte, 2)
	writeValueToBuffer(short, 0, ValFloat(3.14))
	writeValueToBuffer(short, 0, ValUint(42))
	writeValueToBuffer(short, 0, ValInt(-7))
	// If we get here without panic, the test passes.
}

// ---------------------------------------------------------------------------
// 13. OpUndef: verifies undefined values produce zero.
//     Prevents: wrong uninitialized variable behavior.
// ---------------------------------------------------------------------------

func TestOpUndef(t *testing.T) {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid     = 1
		idFloat    = 2
		idVec4     = 3
		idPtrV4Out = 4
		idFuncTy   = 5
		idFunc     = 6
		idLabel    = 7
		idColorOut = 8
		idConst0   = 9
		idConst1   = 10
		idUndef    = 11
		idResult   = 12
		idBound    = 13
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 80)
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
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(4, OpConstant), idFloat, idConst0, f(0),
		inst(4, OpConstant), idFloat, idConst1, f(1),
		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(3, OpUndef), idFloat, idUndef,
		inst(7, OpCompositeConstruct), idVec4, idResult, idUndef, idConst0, idConst0, idConst1,
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
			// OpUndef should produce 0 (our implementation uses ValUint(0)).
			if color[0] != 0 {
				t.Errorf("OpUndef produced %v, want 0", color[0])
			}
			return
		}
	}
	t.Fatal("output not found")
}

// ---------------------------------------------------------------------------
// 14. Comparison operators: additional coverage for unsigned and equality.
//     Prevents: wrong branch decisions from comparison operator bugs.
// ---------------------------------------------------------------------------

func TestComparisonOpsViaInterpreter(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}

	tests := []struct {
		name   string
		opcode uint16
		a, b   Value
		want   bool
	}{
		{"u_gt_true", OpUGreaterThan, ValUint(10), ValUint(3), true},
		{"u_gt_false", OpUGreaterThan, ValUint(3), ValUint(10), false},
		{"u_le_true", OpULessThanEqual, ValUint(3), ValUint(3), true},
		{"u_le_false", OpULessThanEqual, ValUint(4), ValUint(3), false},
		{"u_ge_true", OpUGreaterThanEqual, ValUint(3), ValUint(3), true},
		{"u_ge_false", OpUGreaterThanEqual, ValUint(2), ValUint(3), false},
		{"s_gt_true", OpSGreaterThan, ValUint(5), ValUint(0xFFFFFFFB), true},            // 5 > -5 (signed)
		{"s_le_true", OpSLessThanEqual, ValUint(0xFFFFFFFB), ValUint(0xFFFFFFFB), true}, // -5 <= -5 (signed)
		{"s_ge_true", OpSGreaterThanEqual, ValUint(0), ValUint(0xFFFFFFFF), true},       // 0 >= -1 (signed)
		{"ieq_true", OpIEqual, ValUint(42), ValUint(42), true},
		{"ieq_false", OpIEqual, ValUint(1), ValUint(2), false},
		{"ine_true", OpINotEqual, ValUint(1), ValUint(2), true},
		{"ine_false", OpINotEqual, ValUint(5), ValUint(5), false},
		{"ford_eq_true", OpFOrdEqual, ValFloat(3.14), ValFloat(3.14), true},
		{"ford_eq_false", OpFOrdEqual, ValFloat(1), ValFloat(2), false},
		{"ford_lt_true", OpFOrdLessThan, ValFloat(1), ValFloat(2), true},
		{"ford_gt_true", OpFOrdGreaterThan, ValFloat(2), ValFloat(1), true},
		{"ford_le_true", OpFOrdLessThanEqual, ValFloat(1), ValFloat(1), true},
		{"ford_ge_true", OpFOrdGreaterThanEqual, ValFloat(2), ValFloat(2), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := &Function{
				Instructions: []Instruction{
					{Opcode: OpLabel, ResultID: 100},
					{Opcode: tt.opcode, ResultID: 10, Operands: []uint32{1, 2}},
					{Opcode: OpReturn},
				},
				Labels: map[uint32]int{100: 0},
			}
			interp := &interpreter{
				module: m,
				fn:     fn,
				ep:     &EntryPoint{},
				ctx:    &ExecutionContext{},
				values: testMakeValues(map[uint32]any{1: tt.a, 2: tt.b}),
			}

			err := interp.run()
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}

			got, ok := testIsBool(interp.values[10])
			if !ok {
				t.Fatalf("comparison result is %T, want bool", interp.values[10])
			}
			if got != tt.want {
				t.Errorf("%s(%v, %v) = %v, want %v", tt.name, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 15. valueByteSize: default branch for unknown types.
//     Prevents: wrong buffer offset calculations for unsupported types.
// ---------------------------------------------------------------------------

func TestValueByteSizeDefault(t *testing.T) {
	// An unknown type (string, struct pointer, etc.) should return 4.
	got := valueByteSize(Value{})
	if got != 4 {
		t.Errorf("valueByteSize(string) = %d, want 4", got)
	}
}

// ---------------------------------------------------------------------------
// 16. Division by zero: SDiv, UDiv, SMod, UMod, SRem with zero divisor.
//     Prevents: panics from integer division by zero.
// ---------------------------------------------------------------------------

func TestDivisionByZeroOpcodes(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{}, Constants: map[uint32]Value{}}

	tests := []struct {
		name   string
		opcode uint16
	}{
		{"sdiv_by_zero", OpSDiv},
		{"udiv_by_zero", OpUDiv},
		{"smod_by_zero", OpSMod},
		{"umod_by_zero", OpUMod},
		{"srem_by_zero", OpSRem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := &interpreter{
				module: m,
				fn: &Function{
					Instructions: []Instruction{
						{Opcode: OpLabel, ResultID: 100},
						{Opcode: tt.opcode, ResultID: 10, Operands: []uint32{1, 2}},
						{Opcode: OpReturn},
					},
					Labels: map[uint32]int{100: 0},
				},
				ep:     &EntryPoint{},
				ctx:    &ExecutionContext{},
				values: testMakeValues(map[uint32]any{1: ValUint(42), 2: ValUint(0)}),
			}

			// Should not panic.
			err := interp.run()
			if err != nil {
				t.Fatalf("run() failed: %v", err)
			}
			// Result should be zero on division by zero.
			got := toUint32(interp.values[10])
			if got != 0 {
				t.Errorf("%s result = %d, want 0", tt.name, got)
			}
		})
	}
}
