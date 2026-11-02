//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	hal      hal.ShaderModule
	device   *Device
	released bool
	// irModule stores the parsed naga IR for this shader module.
	// Used for late buffer binding size validation at draw/dispatch time.
	// Matches Rust wgpu-core's ShaderModule.interface which stores the
	// naga Module for shader introspection.
	// nil when the shader was provided as SPIR-V (no WGSL source to parse).
	irModule *ir.Module
	// irInfo is the ModuleInfo of irModule's validation: the globals each
	// entry point uses (Rust naga's ModuleInfo, kept by wgpu-core's
	// Interface). It is a pointer so ShaderModule stays a comparable struct;
	// it is nil exactly when irModule is.
	irInfo *valid.ModuleInfo
}

// entryPointIndex returns the index in the IR of the entry point a pipeline
// stage names, or -1 if there is none. An empty name selects the only entry
// point of the stage (WebGPU "get the entry point": omitted only when the
// module has exactly one for the stage).
func (m *ShaderModule) entryPointIndex(stage ir.ShaderStage, name string) int {
	if m == nil || m.irModule == nil {
		return -1
	}
	found := -1
	for i := range m.irModule.EntryPoints {
		ep := &m.irModule.EntryPoints[i]
		if ep.Stage != stage || (name != "" && ep.Name != name) {
			continue
		}
		if found >= 0 {
			return -1 // an omitted name is ambiguous; a named one is unique
		}
		found = i
	}
	return found
}

// specializedEntryPoint returns the IR, its ModuleInfo and the index of the
// entry point a pipeline stage names, with the pipeline constants applied:
// a module with overrides, or a pipeline giving constants, is specialized
// for the stage (naga.ProcessOverrides), so override-sized workgroup sizes
// and arrays have their values. The index is -1 without IR or without that
// entry point. An error is a constant that does not apply to the module.
func (m *ShaderModule) specializedEntryPoint(stage ir.ShaderStage, name string, constants map[string]float64) (*ir.Module, valid.ModuleInfo, int, error) {
	i := m.entryPointIndex(stage, name)
	if i < 0 || i >= len(m.irInfo.EntryPoints) {
		return nil, valid.ModuleInfo{}, -1, nil
	}
	if !shaderir.PerStage(m.irModule, constants) {
		return m.irModule, *m.irInfo, i, nil
	}
	specialized, err := shaderir.Specialize(shaderir.Module{IR: m.irModule, Info: *m.irInfo}, stage, m.irModule.EntryPoints[i].Name, constants)
	if err != nil {
		return nil, valid.ModuleInfo{}, -1, err
	}
	return specialized.IR, specialized.Info, 0, nil // Specialize keeps only this entry point
}

// bindingSizes returns the minimum buffer binding sizes of the globals the
// entry point uses, callees included, or nil without IR or without that
// entry point. Rust wgpu-core Interface::check_stage reads the resources of
// one entry point, those its ModuleInfo marks as used
// (wgpu-core/src/validation.rs): two globals of a module may share a binding
// when no entry point uses both, and only the one this entry point uses
// sets the size.
func (m *ShaderModule) bindingSizes(stage ir.ShaderStage, entryPoint string) map[ir.ResourceBinding]uint64 {
	i := m.entryPointIndex(stage, entryPoint)
	if i < 0 || i >= len(m.irInfo.EntryPoints) {
		return nil
	}
	return extractShaderBindingSizes(m.irModule, m.irInfo.EntryPoints[i].GlobalUses)
}

// extractShaderBindingSizes extracts the minimum buffer binding sizes of
// the globals of module that uses marks as used (valid.FunctionInfo
// GlobalUses of one entry point). For each such global with a resource
// binding whose type is a buffer (not a sampler, texture, or other opaque
// type), it computes the size with ir.Layouter and returns a map from
// ResourceBinding to minimum byte size.
//
// Equivalent to Rust wgpu-core's Interface::new (validation.rs), which
// calls TypeInner::size() for buffer types, filtered to one entry point's
// resources as check_stage does.
func extractShaderBindingSizes(module *ir.Module, uses []valid.GlobalUse) map[ir.ResourceBinding]uint64 {
	if module == nil {
		return nil
	}

	sizes := make(map[ir.ResourceBinding]uint64)

	// Update fails only for a type arena that refers outside itself or to
	// itself, which validation rejects; without layouts there are no sizes
	// to check, as for a module without IR.
	var layouter ir.Layouter
	if err := layouter.Update(module); err != nil {
		return nil
	}

	for i := range module.GlobalVariables {
		gv := &module.GlobalVariables[i]
		if gv.Binding == nil || i >= len(uses) || uses[i] == 0 {
			continue
		}

		// Skip opaque types (samplers, images, acceleration structures, ray queries).
		// Only buffer-backed types (scalars, vectors, matrices, arrays, structs) have
		// meaningful sizes for binding validation. Matches Rust validation.rs:1000-1016
		// where Image/Sampler/AccelerationStructure produce ResourceType variants that
		// are NOT ResourceType::Buffer.
		if int(gv.Type) >= len(module.Types) {
			continue
		}
		inner := module.Types[gv.Type].Inner
		switch inner.(type) {
		case ir.ImageType, ir.SamplerType, ir.AccelerationStructureType, ir.RayQueryType:
			continue
		}

		size := uint64(layouter.Layout(gv.Type).Size)
		if size == 0 {
			continue
		}

		rb := *gv.Binding
		// Max across globals with the same binding (matches Rust check_stage
		// which takes max when Entry::Occupied, validation.rs:1131-1133).
		if existing, ok := sizes[rb]; ok {
			if size > existing {
				sizes[rb] = size
			}
		} else {
			sizes[rb] = size
		}
	}

	return sizes
}

// Release destroys the shader module. Destruction is deferred until the GPU
// completes any submission that may reference this shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true

	halDevice := m.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := m.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyShaderModule(m.hal)
		return
	}

	subIdx := m.device.lastSubmissionIndex()
	halModule := m.hal
	dq.Defer(subIdx, "ShaderModule", func() {
		halDevice.DestroyShaderModule(halModule)
	})
}
