// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package shaderir holds the naga IR of a WGSL shader module and specializes
// it for one pipeline stage.
//
// A HAL backend lowers WGSL to IR once, when the shader module is created.
// If the module has no pipeline-overridable constants (WGSL `override`), the
// backend may compile it once and reuse the result for every pipeline. If it
// has overrides, or a pipeline gives constants, the backend calls Specialize
// for every pipeline stage and compiles the result: the override values, the
// expressions that depend on them and workgroup sizes given by overrides are
// known only then. This is Rust wgpu-hal's call to
// naga::back::pipeline_constants::process_overrides before every backend
// (vulkan/device.rs:804, dx12/device.rs:349, gles/device.rs:385,
// metal/device.rs:174).
package shaderir

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gogpu/naga"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/spirv"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
)

// Module is a validated IR module and the ModuleInfo of that validation,
// which naga.ProcessOverrides takes.
type Module struct {
	IR   *ir.Module
	Info valid.ModuleInfo
}

// Lower parses WGSL to IR and validates it.
//
// The validation allows every capability (valid.CapAll): it checks the
// structure and yields the ModuleInfo. The capabilities of the device are
// gated once, in core, before the HAL sees the module (Rust wgpu-core's
// validator, built from features_to_naga_capabilities); a capability gate
// here would refuse features the device has. P1-CORE-013 moves this single
// validation into core and hands the HAL its result.
//
// A parse error keeps naga's *wgsl.ParseError (errors.As works) and is
// prefixed with its line and column: ParseError.Error is the message only.
// A validation error is valid.Errors.
func Lower(source string) (Module, error) {
	module, _, err := wgsl.Parse(source, wgsl.ParseOptions{})
	if err != nil {
		var pe *wgsl.ParseError
		if errors.As(err, &pe) {
			if loc, ok := pe.Location(source); ok {
				return Module{}, fmt.Errorf("parse error: %d:%d: %w", loc.Line, loc.Column, err)
			}
		}
		return Module{}, fmt.Errorf("parse error: %w", err)
	}
	info, err := valid.Validate(module, valid.Options{Capabilities: valid.CapAll})
	if err != nil {
		return Module{}, fmt.Errorf("validation failed: %w", err)
	}
	return Module{IR: module, Info: info}, nil
}

// PerStage reports whether module must be specialized for each pipeline
// stage: it has overrides, or the pipeline gives constants (which must then
// name overrides of the module). Otherwise module can be compiled once.
//
// A module from the WGSL frontend without overrides has no override
// expressions, override workgroup sizes or override-sized arrays (the
// guarantee of naga.ProcessOverrides).
func PerStage(module *ir.Module, constants map[string]float64) bool {
	return len(module.Overrides) > 0 || len(constants) > 0
}

// Specialize returns module for one pipeline stage: only the entry point
// named entryPoint of the given stage is kept (it is EntryPoints[0] of the
// result), and every override takes its value from constants or its default.
//
// module is not modified, and Specialize may run concurrently on the same
// module. The result may share memory with module; treat it as read-only.
//
// A constant error keeps naga's error: errors.Is with naga.ErrOverrideNotFound
// and the other sentinels, and errors.As with *naga.PipelineConstantError,
// work on the result.
func Specialize(module Module, stage ir.ShaderStage, entryPoint string, constants map[string]float64) (Module, error) {
	out, info, err := naga.ProcessOverrides(module.IR, module.Info, naga.OverrideOptions{
		Constants:  constants,
		EntryPoint: &backend.EntryPointSelector{Stage: stage, Name: entryPoint},
	})
	if err != nil {
		return Module{}, fmt.Errorf("pipeline constants of %s entry point %q: %w", StageName(stage), entryPoint, err)
	}
	return Module{IR: out, Info: info}, nil
}

// ErrUnresolvedOverride wraps a naga backend error caused by overrides that
// were not resolved before the backend ran. It is a bug in the HAL backend,
// never in the shader or the pipeline.
var ErrUnresolvedOverride = errors.New("BUG: HAL compiled a shader with unresolved overrides (naga.ProcessOverrides not run)")

// BackendError returns err from a naga shader backend. If err says that
// overrides were not resolved (ir.ErrUnresolvedOverride), the result also
// wraps ErrUnresolvedOverride, which marks it as an internal error.
func BackendError(err error) error {
	if err != nil && errors.Is(err, ir.ErrUnresolvedOverride) {
		return fmt.Errorf("%w: %w", ErrUnresolvedOverride, err)
	}
	return err
}

// SPIRV compiles module to SPIR-V words with the backend's options. Every
// backend passes its own options, as each Rust wgpu-hal backend does: naga's
// zero values (SPIR-V 1.0, Restrict on every bounds check) suit no device.
func SPIRV(module *ir.Module, opts spirv.Options) ([]uint32, error) {
	b, _, err := spirv.Compile(module, opts)
	if err != nil {
		return nil, BackendError(err)
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("SPIR-V output size %d is not a multiple of 4", len(b))
	}
	words := make([]uint32, len(b)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	return words, nil
}

// StageName returns the WGSL attribute name of a shader stage.
func StageName(stage ir.ShaderStage) string {
	switch stage {
	case ir.StageVertex:
		return "vertex"
	case ir.StageFragment:
		return "fragment"
	case ir.StageCompute:
		return "compute"
	case ir.StageTask:
		return "task"
	case ir.StageMesh:
		return "mesh"
	default:
		return fmt.Sprintf("stage(%d)", stage)
	}
}
