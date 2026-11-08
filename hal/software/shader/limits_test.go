//go:build !(js && wasm)

package shader

import (
	"strings"
	"testing"
)

// limitsSPIRV is a module with ids 1 void, 2 f32, 3 u32, 4 void(),
// 5 OpConstant u32 length, 6 array<f32, length>, followed by decls (types,
// constants and globals from id 10 on) and a function %7 whose body, after
// its label %8, holds body (Function variables).
func limitsSPIRV(length uint32, decls, body []uint32) []uint32 {
	w := []uint32{
		spirvMagic, 0x00010300, 0, 100, 0,
		spirvInst(2, OpCapability), 1,
		spirvInst(3, OpMemoryModel), 0, 1,
		spirvInst(2, OpTypeVoid), 1,
		spirvInst(3, OpTypeFloat), 2, 32,
		spirvInst(4, OpTypeInt), 3, 32, 0,
		spirvInst(3, OpTypeFunction), 4, 1,
		spirvInst(4, OpConstant), 3, 5, length,
		spirvInst(4, OpTypeArray), 6, 2, 5,
	}
	w = append(w, decls...)
	w = append(w, spirvInst(5, OpFunction), 1, 7, 0, 4, spirvInst(2, OpLabel), 8)
	w = append(w, body...)
	return append(w, spirvInst(1, OpReturn), spirvInst(1, OpFunctionEnd))
}

// undefs returns n OpUndef of type %6, result IDs from first on.
func undefs(n int, first uint32) []uint32 {
	w := make([]uint32, 0, 3*n)
	for i := range n {
		w = append(w, spirvInst(3, OpUndef), 6, first+uint32(i))
	}
	return w
}

// TestWorkgroupLiveBytes checks the bound compute pipeline creation applies:
// one OpUndef of a 1 MiB array is about 12 MiB live per invocation, so a
// 256-invocation workgroup exceeds MaxWorkgroupLiveBytes.
func TestWorkgroupLiveBytes(t *testing.T) {
	m, err := ParseModule(limitsSPIRV(262144, nil, undefs(1, 30)))
	if err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
	if got := m.WorkgroupLiveBytes(1); got < 12<<20 || got > 13<<20 {
		t.Fatalf("WorkgroupLiveBytes(1) = %d, want about 12 MiB", got)
	}
	if got := m.WorkgroupLiveBytes(256); got <= MaxWorkgroupLiveBytes {
		t.Fatalf("WorkgroupLiveBytes(256) = %d, want above %d", got, MaxWorkgroupLiveBytes)
	}
}

// TestParseModuleLimits checks that ParseModule rejects a module that would
// make the interpreter allocate beyond Limits, before allocating it. Before
// P1-SW-040 the first case asked for 206 GB inside ParseModule
// (OpConstantNull of array<f32, 0xFFFFFFFF>) and the process died of a fatal
// out of memory, so these cases are run only with the limits in place.
func TestParseModuleLimits(t *testing.T) {
	ptr := func(id, storage, elem uint32) []uint32 {
		return []uint32{spirvInst(4, OpTypePointer), id, storage, elem}
	}
	variable := func(ptrID, id, storage uint32) []uint32 {
		return []uint32{spirvInst(4, OpVariable), ptrID, id, storage}
	}
	cat := func(parts ...[]uint32) []uint32 {
		var w []uint32
		for _, p := range parts {
			w = append(w, p...)
		}
		return w
	}
	tests := []struct {
		name   string
		length uint32
		decls  []uint32
		body   []uint32
		want   string // "" = must parse
	}{
		{"null constant of a 16 GB array", 0xFFFFFFFF, []uint32{spirvInst(3, OpConstantNull), 6, 10}, nil, "above the device's maxBufferSize"},
		{"nested array above maxBufferSize", 65536,
			[]uint32{spirvInst(4, OpTypeArray), 10, 6, 5}, nil, "type 10 is 17179869184 bytes"},
		{"private variables above the limit", 16385, // 65540 bytes
			cat(ptr(10, StorageClassPrivate, 6), variable(10, 11, StorageClassPrivate)), nil, "Private variables exceed"},
		{"function variables above the limit", 16385,
			ptr(10, StorageClassFunction, 6), variable(10, 11, StorageClassFunction), "Function variables of a function exceed"},
		{"workgroup variables above the limit", 262145, // 1 MiB + 4 bytes
			cat(ptr(10, StorageClassWorkgroup, 6), variable(10, 11, StorageClassWorkgroup)), nil, "Workgroup variables take"},
		{"null constants above the limit", 131072, // 512 KiB each
			[]uint32{spirvInst(3, OpConstantNull), 6, 10, spirvInst(3, OpConstantNull), 6, 11, spirvInst(3, OpConstantNull), 6, 12},
			nil, "constants exceed"},
		// The second deep review of P1-SW-040: each bypassed the first budget.
		{"array of images, zero bytes per element", 1,
			[]uint32{spirvInst(9, OpTypeImage), 10, 2, 1, 0, 0, 0, 1, 0, spirvInst(4, OpConstant), 3, 13, 0xFFFFFFFF,
				spirvInst(4, OpTypeArray), 11, 10, 13, spirvInst(3, OpConstantNull), 11, 12}, nil, "type 11 is"},
		{"type ID defined twice", 16, []uint32{spirvInst(4, OpTypeArray), 6, 2, 5}, nil, "defined more than once"},
		{"workgroup array with a huge ArrayStride", 16,
			cat([]uint32{spirvInst(4, OpDecorate), 6, DecorationArrayStride, 0xFFFFFFFF}, ptr(10, StorageClassWorkgroup, 6),
				variable(10, 11, StorageClassWorkgroup)), nil, "Workgroup variables take"},
		// The third: a decoration after the variable, and void elements.
		{"ArrayStride declared after the workgroup variable", 16,
			cat(ptr(10, StorageClassWorkgroup, 6), variable(10, 11, StorageClassWorkgroup),
				[]uint32{spirvInst(4, OpDecorate), 6, DecorationArrayStride, 0xFFFFFFFF}), nil, "Workgroup variables take"},
		{"workgroup array with an ArrayStride, as Rust naga emits", 16,
			cat([]uint32{spirvInst(4, OpDecorate), 6, DecorationArrayStride, 4}, ptr(10, StorageClassWorkgroup, 6),
				variable(10, 11, StorageClassWorkgroup)), nil, ""},
		{"array of void", 1,
			[]uint32{spirvInst(4, OpConstant), 3, 13, 0xFFFFFFFF, spirvInst(4, OpTypeArray), 11, 1, 13,
				spirvInst(3, OpConstantNull), 11, 12}, nil, "type 11 is"},
		{"two entry points of one name", 1,
			[]uint32{spirvInst(4, OpEntryPoint), ExecutionModelGLCompute, 7, 0x6D, spirvInst(4, OpEntryPoint), ExecutionModelGLCompute, 7, 0x6D},
			nil, "two entry points"},
		{"recursion", 1, nil, []uint32{spirvInst(4, OpFunctionCall), 1, 20, 7}, "recursive"},
		{"results of an invocation above the limit", 262144, nil, undefs(22, 30), "an invocation would hold"},
		{"WGSL guarantees: 8192 private and 8192 function bytes", 2048,
			cat(ptr(10, StorageClassPrivate, 6), variable(10, 11, StorageClassPrivate), ptr(12, StorageClassFunction, 6)),
			variable(12, 13, StorageClassFunction), ""},
		{"limits reached exactly", 16384,
			cat(ptr(10, StorageClassPrivate, 6), variable(10, 11, StorageClassPrivate), ptr(12, StorageClassFunction, 6)),
			variable(12, 13, StorageClassFunction), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModule(limitsSPIRV(tt.length, tt.decls, tt.body))
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("ParseModule: %v", err)
			case tt.want != "" && err == nil:
				t.Fatal("ParseModule accepted the module")
			case tt.want != "" && !strings.Contains(err.Error(), tt.want):
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}
