package wgpu

import (
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
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
}

// extractShaderBindingSizes extracts the minimum buffer binding sizes
// from a shader module. For each global variable with a resource binding
// whose type is a buffer (not a sampler, texture, or other opaque type),
// computes ir.TypeSize and returns a map from ResourceBinding to minimum
// byte size.
//
// Global variables in WGSL are module-scoped and shared across all entry
// points, so no entry-point filtering is needed here (unlike Rust's
// check_stage which filters by per-entry-point resource references).
//
// Equivalent to Rust wgpu-core's Interface::new (validation.rs:985-1017)
// which iterates global_variables and calls TypeInner::size() for buffer types.
func extractShaderBindingSizes(module *ir.Module) map[ir.ResourceBinding]uint64 {
	if module == nil {
		return nil
	}

	sizes := make(map[ir.ResourceBinding]uint64)

	for i := range module.GlobalVariables {
		gv := &module.GlobalVariables[i]
		if gv.Binding == nil {
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

		size := uint64(ir.TypeSize(module, gv.Type))
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
