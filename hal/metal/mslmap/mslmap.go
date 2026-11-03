// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package mslmap builds the naga MSL binding map for the Metal backend.
//
// Metal binds resources by slot in three argument tables: buffers, textures
// and samplers. The slots of a pipeline's shaders come from its pipeline
// layout, as in Rust wgpu-hal: every stage numbers the bindings visible to it
// in binding order, group after group (StageLayout), and the encoder binds a
// bind group at the same slots (GroupSlots). The slots never depend on which
// bindings a shader declares, so one shader module serves pipeline layouts
// that place its bindings differently.
//
// naga translates WGSL arrayLength() and the bounds checks of runtime-sized
// arrays into arithmetic on an extra entry point argument named
// `_buffer_sizes`. It goes to the buffer slot after the layout's buffers.
//
// The package has no build tags and does not call Objective-C, so its tests run
// on any CI runner.
//
// Reference: Rust wgpu-hal metal/device.rs create_pipeline_layout
// (per_stage_map, sizes_buffer) and load_shader (one entry point per stage
// with the layout's map).
package mslmap

import (
	"fmt"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
)

// maxBufferSlot is the largest slot naga can encode, because msl.BindTarget
// stores the slot in a uint8. Real Metal devices allow only 31 buffers, so a
// value above this limit means the caller made a mistake.
const maxBufferSlot = 255

// BaseOptions returns the MSL options every Metal shader starts from, before
// any binding map. The bounds checks are Rust wgpu-hal's
// (metal/device.rs:186-222): Restrict on indices, buffers and image loads,
// Unchecked on binding arrays. naga's zero value would check binding arrays
// too.
func BaseOptions() msl.Options {
	return msl.Options{
		BoundsCheckPolicies: backend.BoundsCheckPolicies{
			Index:        backend.BoundsCheckRestrict,
			Buffer:       backend.BoundsCheckRestrict,
			ImageLoad:    backend.BoundsCheckRestrict,
			BindingArray: backend.BoundsCheckUnchecked,
		},
	}
}

// RuntimeArrayBindings returns the bindings of the global variables whose type
// holds a runtime-sized array. The order is the IR declaration order.
//
// naga builds one uint32 field per such global of the module, in this same
// order, whichever entry point it writes. Each field must hold the byte size
// of the binding that is bound to it.
//
// Reference: naga v0.18.0 msl/internal/codegen/writer.go:786-797
// (scanBufferSizeGlobals).
func RuntimeArrayBindings(module *ir.Module) ([]ir.ResourceBinding, error) {
	if module == nil {
		return nil, nil
	}
	var out []ir.ResourceBinding
	for i, global := range module.GlobalVariables {
		if !typeNeedsArrayLength(module, global.Type) {
			continue
		}
		if global.Binding == nil {
			// Such a global has a field in the sizes struct, but no
			// size that the encoder can find. Skipping it would
			// shift all later fields. WGSL cannot declare this
			// today, so report an error instead of writing the
			// sizes to the wrong fields.
			return nil, fmt.Errorf("mslmap: global %d has a runtime-sized array but no binding", i)
		}
		out = append(out, *global.Binding)
	}
	return out, nil
}

// typeNeedsArrayLength reports whether the type is a runtime-sized array, or a
// struct whose last member ends in one.
//
// Reference: naga v0.18.0 msl/internal/codegen/writer.go:770-784
// (needsArrayLength).
func typeNeedsArrayLength(module *ir.Module, handle ir.TypeHandle) bool {
	if int(handle) >= len(module.Types) {
		return false
	}
	switch inner := module.Types[handle].Inner.(type) {
	case ir.ArrayType:
		// A size given by an override (Pending) is not runtime-sized.
		return inner.Size.IsRuntimeSized()
	case ir.StructType:
		if len(inner.Members) == 0 {
			return false
		}
		return typeNeedsArrayLength(module, inner.Members[len(inner.Members)-1].Type)
	default:
		return false
	}
}
