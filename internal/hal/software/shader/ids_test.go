//go:build !(js && wasm)

package shader

import (
	"strings"
	"testing"
)

// findInst returns the word index of the first instruction with opcode op in
// a SPIR-V module, or fails the test.
func findInst(t *testing.T, words []uint32, op uint16) int {
	t.Helper()
	for pos := 5; pos < len(words); {
		n := int(words[pos] >> 16)
		if n == 0 {
			break
		}
		if words[pos]&0xFFFF == uint32(op) {
			return pos
		}
		pos += n
	}
	t.Fatalf("no %s in the module", opcodeName(op))
	return 0
}

// TestParseModuleRejectsBadIDs checks that ParseModule rejects modules whose
// IDs the interpreter would index its value slice with out of range. Each
// case is a valid compute module (buildArraySumComputeSPIRV, Bound 24) with
// one word changed. Before the check, every case parsed and the first
// dispatch panicked with "index out of range" (P1-SW-039).
func TestParseModuleRejectsBadIDs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, w []uint32)
		want   string
	}{
		{"bound below result IDs", func(t *testing.T, w []uint32) { w[3] = 10 }, "outside [1, 10)"},
		{"bound zero", func(t *testing.T, w []uint32) { w[3] = 0 }, "header ID bound 0"},
		{"bound above SPIR-V limit", func(t *testing.T, w []uint32) { w[3] = 0xFFFFFFFF }, "header ID bound 4294967295"},
		{"operand ID at bound", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpIAdd)+4] = 24 // idLoadInput -> Bound
		}, "not below the ID bound 24"},
		{"operand ID undefined", func(t *testing.T, w []uint32) {
			w[3] = 40
			w[findInst(t, w, OpIAdd)+4] = 35
		}, "does not define"},
		{"result type not a type", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpIAdd)+1] = 13 // idConst0, a constant
		}, "is not a type"},
		{"store pointer out of range", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpStore)+1] = 1000
		}, "not below the ID bound"},
		{"entry point interface out of range", func(t *testing.T, w []uint32) {
			// OpEntryPoint GLCompute idFunc "cs_main" (2 words) idGIDVar
			w[findInst(t, w, OpEntryPoint)+5] = 500
		}, "entry point cs_main uses ID 500"},
		{"type uses a later type", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpTypeVector)+2] = 9 // component type -> idStruct, declared later
		}, "not a type declared before it"},
		{"struct contains itself", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpTypeStruct)+2] = 9 // member -> the struct itself
		}, "not a type declared before it"},
		{"vector of 9 components", func(t *testing.T, w []uint32) {
			w[findInst(t, w, OpTypeVector)+3] = 9
		}, "has 9 components"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := buildArraySumComputeSPIRV()
			tt.mutate(t, words)
			m, err := ParseModule(words)
			if err == nil {
				t.Fatalf("ParseModule accepted the module (Bound %d, %d value slots)", m.Bound, m.valueSlots)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

// TestParseModuleNonSemanticExtInst checks that an OpExtInst of a
// non-semantic set whose operands name an OpString parses: run() never reads
// them, and glslang and DXC emit such debug info (NonSemantic.Shader.
// DebugInfo.100 DebugSource). The deep review of P1-SW-039 found the first
// version of the ID check rejected it.
func TestParseModuleNonSemanticExtInst(t *testing.T) {
	const (
		idSet    = 25
		idString = 26
		idDebug  = 27
		idVoid   = 1
		debugSrc = 35 // DebugSource
	)
	words := buildArraySumComputeSPIRV()
	words[3] = 28
	insert := func(at int, ws ...uint32) { words = append(words[:at], append(ws, words[at:]...)...) }
	set := spirvString("NonSemantic.Shader.DebugInfo.100")
	insert(findInst(t, words, OpMemoryModel), append([]uint32{spirvInst(uint16(2+len(set)), OpExtInstImport), idSet}, set...)...)
	file := spirvString("a.wgsl")
	insert(findInst(t, words, OpDecorate), append([]uint32{spirvInst(uint16(2+len(file)), OpString), idString}, file...)...)
	label := findInst(t, words, OpLabel)
	insert(label+2, spirvInst(6, OpExtInst), idVoid, idDebug, idSet, debugSrc, idString)
	if _, err := ParseModule(words); err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
}

// TestParseModuleValueSlots checks that the value slice is sized by the IDs
// the module defines, not by a header Bound far above them.
func TestParseModuleValueSlots(t *testing.T) {
	words := buildArraySumComputeSPIRV()
	words[3] = maxIDBound
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
	if m.valueSlots != 24 {
		t.Fatalf("valueSlots = %d, want 24 (largest ID 23 plus one)", m.valueSlots)
	}
}

// TestParseModuleLiteralOperandsAreNotIDs checks that literal operands are
// not validated as IDs: an OpSwitch case literal and an OpCompositeExtract
// index far above Bound must parse.
func TestParseModuleLiteralOperandsAreNotIDs(t *testing.T) {
	words := buildSwitchSPIRV()
	sw := findInst(t, words, OpSwitch)
	if int(words[sw]>>16) < 5 {
		t.Fatal("the switch module has no case")
	}
	words[sw+3] = 0xFFFFFFF0 // the first case literal
	if _, err := ParseModule(words); err != nil {
		t.Fatalf("switch case literal: %v", err)
	}

	words = buildArraySumComputeSPIRV()
	words[findInst(t, words, OpCompositeExtract)+4] = 0xFFFF // index literal
	if _, err := ParseModule(words); err != nil {
		t.Fatalf("composite extract index: %v", err)
	}
}
