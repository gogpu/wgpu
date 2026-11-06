//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"
)

// TestStageConstantsToJS checks that each programmable stage passes its
// pipeline constants as the GPUProgrammableStage "constants" record, and
// leaves the member out when there are none.
func TestStageConstantsToJS(t *testing.T) {
	constants := map[string]float64{"a": 1.5, "1000": -2, "flag": 1}
	module := js.Global().Get("Object").New()
	tests := []struct {
		name  string
		stage func(map[string]float64) js.Value
	}{
		{"vertex", func(c map[string]float64) js.Value {
			v := VertexStateJS{ModuleRef: module, EntryPoint: "vs", Constants: c}
			return v.ToJS()
		}},
		{"fragment", func(c map[string]float64) js.Value {
			f := FragmentStateJS{ModuleRef: module, EntryPoint: "fs", Constants: c}
			return f.ToJS()
		}},
		{"compute", func(c map[string]float64) js.Value {
			return BuildComputePipelineDescriptor("", js.Undefined(), module, "main", c).Get("compute")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := tt.stage(constants).Get("constants")
			if record.Type() != js.TypeObject {
				t.Fatalf("constants = %v, want an object", record)
			}
			if n := js.Global().Get("Object").Call("keys", record).Length(); n != len(constants) {
				t.Errorf("constants has %d keys, want %d", n, len(constants))
			}
			for key, want := range constants {
				if got := record.Get(key); got.Type() != js.TypeNumber || got.Float() != want {
					t.Errorf("constants[%q] = %v, want %v", key, got, want)
				}
			}
			for _, empty := range []map[string]float64{nil, {}} {
				if got := tt.stage(empty).Get("constants"); !got.IsUndefined() {
					t.Errorf("constants with %v = %v, want undefined", empty, got)
				}
			}
		})
	}
}
