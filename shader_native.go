//go:build !rust && !(js && wasm)

package wgpu

import (
	"bytes"
	"encoding/binary"

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
	// spirvWorkgroups holds the LocalSize of each compute entry point of a
	// module given as SPIR-V, which has no IR, so that pipeline creation
	// checks it against the limits as it does for WGSL (Rust wgpu-core runs
	// check_stage on SPIR-V modules too, through naga's SPIR-V frontend).
	// A pointer, like irInfo, so ShaderModule stays comparable; nil for a
	// WGSL module.
	spirvWorkgroups *spirvWorkgroups
}

// spirvWorkgroups maps a compute entry point's name to its LocalSize.
type spirvWorkgroups struct {
	byName map[string][3]uint32
}

// SPIR-V words spirvComputeWorkgroups reads (SPIR-V 1.6, sections 3.6 and 3.32).
const (
	spirvOpEntryPoint       = 15
	spirvOpExecutionMode    = 16
	spirvExecModelGLCompute = 5
	spirvExecModeLocalSize  = 17
	spirvHeaderWords        = 5
	spirvEntryPointName     = 2 // operand index of the OpEntryPoint name (after model, function)
)

// spirvComputeWorkgroups returns the LocalSize execution mode of each GLCompute
// entry point of a SPIR-V module, by name. An entry point without one is
// absent; a malformed module returns what was read before the fault, the
// HAL rejecting the module itself.
func spirvComputeWorkgroups(words []uint32) *spirvWorkgroups {
	names := map[uint32]string{}
	sizes := map[uint32][3]uint32{}
	for i := spirvHeaderWords; i < len(words); {
		n := int(words[i] >> 16)
		if n == 0 || i+n > len(words) {
			break
		}
		ops := words[i+1 : i+n]
		switch words[i] & 0xFFFF {
		case spirvOpEntryPoint:
			if len(ops) > spirvEntryPointName && ops[0] == spirvExecModelGLCompute {
				names[ops[1]] = spirvString(ops[spirvEntryPointName:])
			}
		case spirvOpExecutionMode:
			if len(ops) >= 5 && ops[1] == spirvExecModeLocalSize {
				sizes[ops[0]] = [3]uint32{ops[2], ops[3], ops[4]}
			}
		}
		i += n
	}
	out := make(map[string][3]uint32, len(names))
	for fn, name := range names {
		if size, ok := sizes[fn]; ok {
			out[name] = size
		}
	}
	return &spirvWorkgroups{byName: out}
}

// spirvString decodes a nul-terminated SPIR-V literal string.
func spirvString(words []uint32) string {
	b := make([]byte, 4*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint32(b[4*i:], w)
	}
	if end := bytes.IndexByte(b, 0); end >= 0 {
		b = b[:end]
	}
	return string(b)
}

// spirvWorkgroupSize returns the LocalSize of the compute entry point name of
// a SPIR-V module; an empty name selects the only one.
func (m *ShaderModule) spirvWorkgroupSize(name string) ([3]uint32, bool) {
	if m.spirvWorkgroups == nil {
		return [3]uint32{}, false
	}
	sizes := m.spirvWorkgroups.byName
	if name == "" && len(sizes) == 1 {
		for _, size := range sizes {
			return size, true
		}
	}
	size, ok := sizes[name]
	return size, ok
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
