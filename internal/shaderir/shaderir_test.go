// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shaderir

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/ir"
)

const overrideShader = `
override wg: u32 = 4u;
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

@compute @workgroup_size(wg, 2)
fn main(@builtin(local_invocation_index) i: u32) {
    out[i] = i;
}

@compute @workgroup_size(1)
fn other() {
    out[0] = 1u;
}
`

const plainShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

@compute @workgroup_size(1)
fn main() {
    out[0] = 1u;
}
`

func TestPerStage(t *testing.T) {
	withOverrides, err := LowerValidated(overrideShader)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := LowerValidated(plainShader)
	if err != nil {
		t.Fatal(err)
	}
	if !PerStage(withOverrides, nil) {
		t.Error("PerStage(module with overrides, nil) = false")
	}
	if PerStage(plain, nil) {
		t.Error("PerStage(module without overrides, nil) = true")
	}
	if !PerStage(plain, map[string]float64{"x": 1}) {
		t.Error("PerStage(module without overrides, constants) = false: unknown keys must be rejected")
	}
}

func TestSpecialize(t *testing.T) {
	module, err := LowerValidated(overrideShader)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Specialize(module, ir.StageCompute, "main", map[string]float64{"wg": 3})
	if err != nil {
		t.Fatalf("Specialize: %v", err)
	}
	if len(out.EntryPoints) != 1 || out.EntryPoints[0].Name != "main" {
		t.Fatalf("entry points = %v, want only main", out.EntryPoints)
	}
	if got := out.EntryPoints[0].Workgroup; got != [3]uint32{3, 2, 1} {
		t.Errorf("workgroup size = %v, want [3 2 1]", got)
	}
	if _, err := SPIRV(out); err != nil {
		t.Errorf("SPIRV of the specialized module: %v", err)
	}

	_, err = Specialize(module, ir.StageCompute, "main", map[string]float64{"nope": 1})
	var pce *naga.PipelineConstantError
	if !errors.Is(err, naga.ErrOverrideNotFound) || !errors.As(err, &pce) || pce.Key != "nope" {
		t.Errorf("unknown constant: error %v, want a *naga.PipelineConstantError for key nope", err)
	}
	if err != nil && !strings.Contains(err.Error(), `compute entry point "main"`) {
		t.Errorf("error %q does not name the stage and entry point", err)
	}
	if _, err := Specialize(module, ir.StageVertex, "main", nil); !errors.Is(err, naga.ErrEntryPointNotFound) {
		t.Errorf("wrong stage: error %v, want naga.ErrEntryPointNotFound", err)
	}
}

func TestBackendError(t *testing.T) {
	if BackendError(nil) != nil {
		t.Error("BackendError(nil) != nil")
	}
	other := errors.New("other")
	if got := BackendError(other); !errors.Is(got, other) || got.Error() != other.Error() {
		t.Errorf("BackendError(other) = %v, want it unchanged", got)
	}
	unresolved := fmt.Errorf("spirv: %w", ir.ErrUnresolvedOverride)
	got := BackendError(unresolved)
	if !errors.Is(got, ErrUnresolvedOverride) || !errors.Is(got, ir.ErrUnresolvedOverride) {
		t.Errorf("BackendError(unresolved) = %v, want both ErrUnresolvedOverride sentinels", got)
	}

	// A backend given a module with overrides that was not specialized.
	module, err := LowerValidated(overrideShader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SPIRV(module); !errors.Is(err, ErrUnresolvedOverride) {
		t.Errorf("SPIRV of an unprocessed module: error %v, want ErrUnresolvedOverride", err)
	}
}
