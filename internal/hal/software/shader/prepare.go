//go:build !(js && wasm)

package shader

import (
	"slices"
	"sync"
)

// modulePrep holds what every invocation of a module needs and what does not
// change once the module is parsed. It is built on the first invocation, so
// an invocation iterates slices instead of the module's maps, and finds a
// type's ID without scanning the type map.
type modulePrep struct {
	typeIDs   map[*TypeInfo]uint32
	constIDs  []uint32
	constVals []Value  // constVals[i] is the value of constIDs[i]
	private   []uint32 // Private variables
	workgroup []uint32 // Workgroup variables
	resources []uint32 // Uniform, StorageBuffer, PushConstant and UniformConstant variables
	// quadEntries holds the entry points that reach a derivative or an
	// implicit-LOD sample (see Module.NeedsQuads).
	quadEntries map[string]bool
}

// modulePrepState is the lazily built modulePrep of a Module.
type modulePrepState struct {
	once sync.Once
	prep *modulePrep
}

// prepared returns the module's modulePrep, building it on first use.
func (m *Module) prepared() *modulePrep {
	m.prepState.once.Do(func() { m.prepState.prep = m.buildPrep() })
	return m.prepState.prep
}

func (m *Module) buildPrep() *modulePrep {
	p := &modulePrep{typeIDs: make(map[*TypeInfo]uint32, len(m.Types))}
	for id, ti := range m.Types {
		p.typeIDs[ti] = id
	}
	for id := range m.Constants {
		p.constIDs = append(p.constIDs, id)
	}
	for id, vi := range m.Variables {
		switch vi.StorageClass {
		case StorageClassPrivate:
			p.private = append(p.private, id)
		case StorageClassWorkgroup:
			p.workgroup = append(p.workgroup, id)
		case StorageClassUniform, StorageClassStorageBuffer, StorageClassPushConstant, StorageClassUniformConstant:
			p.resources = append(p.resources, id)
		}
	}
	// Sorted, so every invocation sets its variables up in the same order.
	slices.Sort(p.constIDs)
	p.constVals = make([]Value, len(p.constIDs))
	for i, id := range p.constIDs {
		p.constVals[i] = m.Constants[id]
	}
	slices.Sort(p.private)
	slices.Sort(p.workgroup)
	slices.Sort(p.resources)
	p.quadEntries = make(map[string]bool)
	for name, fn := range m.Functions {
		if m.reachesQuadOp(fn, make(map[*Function]bool)) {
			p.quadEntries[name] = true
		}
	}
	return p
}

// typeID returns the ID of the type ti, or 0 if it is not in the module.
func (m *Module) typeID(ti *TypeInfo) uint32 {
	if id, ok := m.prepared().typeIDs[ti]; ok {
		return id
	}
	// A type added after the module was prepared (only test modules build
	// their types by hand).
	for id, t := range m.Types {
		if t == ti {
			return id
		}
	}
	return 0
}

// resolvedResources caches, per module, the starting values of the resource
// variables of one ExecutionContext (prep.resources order): a uniform or push
// constant block deserialized from its buffer, a storage buffer's
// BufferPointer, a texture or sampler's binding key. Every invocation of a
// draw or dispatch shares one context, so they are read once instead of once
// per vertex, fragment or invocation. A zero Value marks a variable without
// a binding, which keeps its zero value.
type resolvedResources struct {
	byModule map[*Module][]Value
}

// resourceValues returns the resource values of the interpreter's module in
// its context, resolving them on the context's first use with the module.
func (interp *interpreter) resourceValues(prep *modulePrep) []Value {
	ctx := interp.ctx
	if ctx.resolved == nil {
		ctx.resolved = &resolvedResources{}
	}
	if vals, ok := ctx.resolved.byModule[interp.module]; ok {
		return vals
	}
	vals := make([]Value, len(prep.resources))
	for i, varID := range prep.resources {
		vals[i] = interp.resolveResource(varID)
	}
	if ctx.resolved.byModule == nil {
		ctx.resolved.byModule = make(map[*Module][]Value)
	}
	ctx.resolved.byModule[interp.module] = vals
	return vals
}

// resolveResource returns the starting value of resource variable varID in
// the interpreter's context (see resolvedResources).
func (interp *interpreter) resolveResource(varID uint32) Value {
	m := interp.module
	ctx := interp.ctx
	vi := m.Variables[varID]
	bk, hasBind := m.GetBinding(varID)
	if !hasBind {
		return Value{}
	}
	if vi.StorageClass == StorageClassUniformConstant {
		// Textures and samplers: OpLoad resolves the binding key.
		return ValBindingKey(bk)
	}
	var bufData []byte
	if ctx.Buffers != nil {
		bufData = ctx.Buffers[bk]
	}
	pointeeType := m.PointeeType(vi.TypeID)
	if bufData == nil || pointeeType == nil {
		return zeroValueForVar(m, vi.TypeID)
	}
	if vi.StorageClass == StorageClassStorageBuffer {
		// Storage buffers use BufferPointer for direct read/write to raw bytes,
		// so OpStore writes are immediately reflected in the buffer. Access
		// chains derive new pointers, so every invocation shares this one.
		return ValBufferPointer(&BufferPointer{Buffer: bufData, Offset: 0, Type: pointeeType})
	}
	// Uniform and push constant buffers are read-only: deserialize once. A
	// store into a composite copies it, so invocations can share the value.
	return interp.readValueFromBuffer(bufData, 0, pointeeType, 0)
}
