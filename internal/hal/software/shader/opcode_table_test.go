//go:build !(js && wasm)

package shader

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// opcodeConstants returns the value of every integer constant declared in
// opcodes.go, by name.
func opcodeConstants(t *testing.T) map[string]uint16 {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "opcodes.go", nil, 0)
	if err != nil {
		t.Fatalf("parse opcodes.go: %v", err)
	}
	consts := make(map[string]uint16)
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				v, err := strconv.ParseUint(lit.Value, 0, 16)
				if err == nil {
					consts[name.Name] = uint16(v)
				}
			}
		}
	}
	return consts
}

// runCaseLabels returns the identifiers of the case labels of the
// `switch inst.Opcode` statement in (*interpreter).run.
func runCaseLabels(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "interpreter.go", nil, 0)
	if err != nil {
		t.Fatalf("parse interpreter.go: %v", err)
	}
	var labels []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "run" || fn.Recv == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			sel, ok := sw.Tag.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Opcode" {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "inst" {
				return true
			}
			for _, stmt := range sw.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					id, ok := e.(*ast.Ident)
					if !ok {
						t.Errorf("run(): case label %T is not an opcode constant", e)
						continue
					}
					labels = append(labels, id.Name)
				}
			}
			return false
		})
	}
	if len(labels) == 0 {
		t.Fatal("no `switch inst.Opcode` found in (*interpreter).run")
	}
	return labels
}

// TestSupportedOps checks the three lists of ADR-078 rule 6 against each
// other: supportedOps (what ParseModule accepts in a function body),
// decodeInstruction's typed layout (taken from the table) and run()'s case
// labels (what the interpreter executes). An opcode added to one and not the
// others fails here, so an accepted opcode can never fall into run()'s
// default.
func TestSupportedOps(t *testing.T) {
	consts := opcodeConstants(t)
	labels := runCaseLabels(t)

	tableNames := make([]string, 0, len(supportedOps))
	for op, info := range supportedOps {
		tableNames = append(tableNames, info.name)
		if v, ok := consts[info.name]; !ok || v != op {
			t.Errorf("supportedOps[%d] is named %q, which is not the constant of opcode %d", op, info.name, op)
		}
	}
	slices.Sort(tableNames)
	slices.Sort(labels)
	if !slices.Equal(tableNames, labels) {
		for _, n := range labels {
			if _, found := slices.BinarySearch(tableNames, n); !found {
				t.Errorf("run() has a case for %s, which supportedOps does not list", n)
			}
		}
		for _, n := range tableNames {
			if _, found := slices.BinarySearch(labels, n); !found {
				t.Errorf("supportedOps lists %s, which run() has no case for", n)
			}
		}
	}

	for op, info := range supportedOps {
		inst := decodeInstruction(op, []uint32{7, 8, 9})
		switch {
		case info.typed && (inst.TypeID != 7 || inst.ResultID != 8 || !slices.Equal(inst.Operands, []uint32{9})):
			t.Errorf("%s is typed, decoded as %+v", info.name, inst)
		case !info.typed && (inst.TypeID != 0 || inst.ResultID != 0 || !slices.Equal(inst.Operands, []uint32{7, 8, 9})):
			t.Errorf("%s is untyped, decoded as %+v", info.name, inst)
		}
	}
}

// minimalModule returns a SPIR-V module with one function whose body is body
// between OpLabel and OpReturn; extra goes before the function's types.
func minimalModule(extra, body []uint32) []uint32 {
	words := []uint32{
		spirvMagic, 0x00010300, 0, 100, 0,
		spirvInst(2, OpCapability), 1, // Shader
		spirvInst(3, OpMemoryModel), 0, 1,
	}
	words = append(words, extra...)
	words = append(words,
		spirvInst(2, OpTypeVoid), 1,
		spirvInst(3, OpTypeFunction), 2, 1,
		spirvInst(5, OpFunction), 1, 3, 0, 2,
		spirvInst(2, OpLabel), 4,
	)
	words = append(words, body...)
	return append(words, spirvInst(1, OpReturn), spirvInst(1, OpFunctionEnd))
}

// TestParseModuleRejectsUnsupported checks that valid SPIR-V the interpreter
// does not execute fails ParseModule with an error that wraps ErrUnsupported
// and names what is missing. Before ADR-078 step 7 such an opcode was skipped
// and its result left undefined.
func TestParseModuleRejectsUnsupported(t *testing.T) {
	tests := []struct {
		name        string
		extra, body []uint32
		want        string
	}{
		{
			name: "image_query_lod",
			body: []uint32{spirvInst(5, 105), 1, 10, 11, 12},
			want: "software: SPIR-V opcode 105 (OpImageQueryLod) is not supported",
		},
		{
			name: "proj_sample",
			body: []uint32{spirvInst(5, 91), 1, 10, 11, 12},
			want: "SPIR-V opcode 91 (OpImageSampleProjImplicitLod) is not supported",
		},
		{
			name: "ray_query_proceed",
			body: []uint32{spirvInst(4, 4477), 1, 10, 11},
			want: "SPIR-V opcode 4477 (OpRayQueryProceedKHR) is not supported",
		},
		{
			name: "unknown",
			body: []uint32{spirvInst(1, 0x7777)},
			want: "SPIR-V opcode 30583 (OpUnknown) is not supported",
		},
		{
			name:  "float16_capability",
			extra: []uint32{spirvInst(2, OpCapability), 9},
			want:  "SPIR-V capability 9 (Float16) is not supported",
		},
		{
			name:  "f16_type",
			extra: []uint32{spirvInst(3, OpTypeFloat), 20, 16},
			want:  "SPIR-V type OpTypeFloat 16 (f16) is not supported",
		},
		{
			name:  "ray_query_capability",
			extra: []uint32{spirvInst(2, OpCapability), 4472},
			want:  "SPIR-V capability 4472 (RayQueryKHR) is not supported",
		},
		{
			name:  "ray_query_type",
			extra: []uint32{spirvInst(2, opTypeRayQueryKHR), 20},
			want:  "SPIR-V type OpTypeRayQueryKHR is not supported",
		},
		{
			name:  "acceleration_structure_type",
			extra: []uint32{spirvInst(2, opTypeAccelerationStructureKHR), 20},
			want:  "SPIR-V type OpTypeAccelerationStructureKHR is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModule(minimalModule(tt.extra, tt.body))
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("ParseModule error %v, want one that wraps ErrUnsupported", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseModule error %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestParseModuleAcceptsNoOps checks that OpNop, OpLine and OpNoLine in a
// function body parse and are dropped: they carry no semantics, so they are
// not in supportedOps and must not fail the module.
func TestParseModuleAcceptsNoOps(t *testing.T) {
	body := []uint32{
		spirvInst(1, OpNop),
		spirvInst(4, OpLine), 5, 1, 1,
		spirvInst(1, OpNoLine),
	}
	m, err := ParseModule(minimalModule(nil, body))
	if err != nil {
		t.Fatalf("ParseModule: %v", err)
	}
	fn := m.FunctionsByID[3]
	if fn == nil {
		t.Fatal("function 3 not recorded")
	}
	for _, inst := range fn.Instructions {
		if inst.Opcode != OpLabel && inst.Opcode != OpReturn {
			t.Errorf("recorded %s, want only OpLabel and OpReturn", opcodeName(inst.Opcode))
		}
	}
}

// TestRunRejectsUnsupportedOpcode checks run()'s default: an instruction
// that reaches it without ParseModule's check fails the dispatch with the
// opcode's name instead of being skipped.
func TestRunRejectsUnsupportedOpcode(t *testing.T) {
	m := compileSoftwareSPIRV(t, typedHeader+`@compute @workgroup_size(1) fn main() {
	dst[0] = src[0];
}`)
	fn := m.Functions["main"]
	fn.Instructions = slices.Insert(fn.Instructions, 1, Instruction{Opcode: 105})
	ctx, _ := typedContext(&typedCase{src: []uint32{1}, dstLen: 1})
	err := m.DispatchCompute("main", ctx, 1, 1, 1)
	if !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "SPIR-V opcode 105 (OpImageQueryLod)") {
		t.Fatalf("DispatchCompute error %v, want the unsupported OpImageQueryLod", err)
	}
}

// TestAllAny checks OpAll and OpAny over bool vectors of every width and a
// bool scalar.
func TestAllAny(t *testing.T) {
	bvec := func(lanes ...uint32) Value {
		v := Value{Tag: laneTag(kindBool, len(lanes))}
		copy(v.U[:], lanes)
		return v
	}
	tests := []struct {
		v        Value
		all, any bool
	}{
		{bvec(1, 1), true, true},
		{bvec(1, 0), false, true},
		{bvec(0, 0), false, false},
		{bvec(1, 1, 1), true, true},
		{bvec(0, 0, 1), false, true},
		{bvec(1, 1, 1, 1), true, true},
		{bvec(1, 1, 1, 0), false, true},
		{bvec(0, 0, 0, 0), false, false},
		{ValBool(true), true, true},
		{ValBool(false), false, false},
	}
	for _, tt := range tests {
		if got := allAny(true, tt.v); toBool(got) != tt.all || got.Tag != TagBool {
			t.Errorf("OpAll %v = %v, want %v", tt.v.U, got, tt.all)
		}
		if got := allAny(false, tt.v); toBool(got) != tt.any || got.Tag != TagBool {
			t.Errorf("OpAny %v = %v, want %v", tt.v.U, got, tt.any)
		}
	}
}
