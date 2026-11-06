//go:build !(js && wasm)

// Compute shader execution for the SPIR-V interpreter.
//
// DispatchCompute runs a whole dispatch: the invocations of each workgroup
// share its workgroup memory and synchronize at workgroup barriers.
// ExecuteCompute runs one invocation on its own.

package shader

import (
	"encoding/binary"
	"fmt"
	"sync"
)

// ExecuteCompute runs a compute shader entry point for a single invocation.
//
// The context must have ComputeBuiltins (GlobalInvocationID, LocalInvocationID,
// WorkgroupID, NumWorkgroups, WorkgroupSize, LocalInvocationIndex) populated
// by the caller for each invocation.
//
// Storage buffer writes are reflected in the context's Buffers map. A
// workgroup variable lives in its WorkgroupSharedMemory buffer when the
// context has one of the variable's size, and in zeroed memory of its own
// otherwise. The invocation runs alone, so it does not wait at a barrier:
// use DispatchCompute to run the invocations of a workgroup together.
func (m *Module) ExecuteCompute(entryPoint string, ctx *ExecutionContext) error {
	if ctx == nil {
		ctx = &ExecutionContext{}
	}
	ep, fn, err := m.computeEntryPoint(entryPoint)
	if err != nil {
		return err
	}
	interp := m.startCompute(ep, fn, ctx)
	if err := interp.run(); err != nil {
		m.release(interp)
		return err
	}
	interp.writeStorageBufferBack()
	m.putInterpreter(interp)
	return nil
}

// computeEntryPoint returns the compute entry point named entryPoint and its
// function.
func (m *Module) computeEntryPoint(entryPoint string) (*EntryPoint, *Function, error) {
	ep, ok := m.EntryPoints[entryPoint]
	if !ok {
		return nil, nil, fmt.Errorf("spirv: entry point %q not found", entryPoint)
	}
	if ep.ExecutionModel != ExecutionModelGLCompute {
		return nil, nil, fmt.Errorf("spirv: entry point %q is not a compute shader (model=%d)",
			entryPoint, ep.ExecutionModel)
	}
	fn, ok := m.Functions[entryPoint]
	if !ok {
		return nil, nil, fmt.Errorf("spirv: function body for %q not found", entryPoint)
	}
	return ep, fn, nil
}

// startCompute returns a pooled interpreter set up to run one invocation of
// the compute entry point ep (function fn) in ctx, not yet started.
func (m *Module) startCompute(ep *EntryPoint, fn *Function, ctx *ExecutionContext) *interpreter {
	interp := m.getInterpreter()
	interp.ep = ep
	interp.fn = fn
	interp.ctx = ctx
	interp.prevBlock = 0
	interp.iterationCount = 0
	interp.callDepth = 0
	interp.returnValue = Value{}

	interp.seedConstants()
	interp.initComputeBuiltins()
	interp.initResourceVariables()
	interp.initWorkgroupVariables()
	// Private globals start from their initializer or zero.
	interp.initPrivateVariables()
	return interp
}

// GetWorkgroupSize returns the workgroup size declared via OpExecutionMode LocalSize.
// Returns (1, 1, 1) if not specified.
func (m *Module) GetWorkgroupSize(entryPoint string) [3]uint32 {
	ep, ok := m.EntryPoints[entryPoint]
	if !ok {
		return [3]uint32{1, 1, 1}
	}

	key := executionModeKey{
		FunctionID:    ep.FunctionID,
		ExecutionMode: ExecutionModeLocalSize,
	}
	if operands, ok := m.ExecutionModes[key]; ok && len(operands) >= 3 {
		return [3]uint32{operands[0], operands[1], operands[2]}
	}
	return [3]uint32{1, 1, 1}
}

// initComputeBuiltins seeds compute shader built-in variables from the context.
func (interp *interpreter) initComputeBuiltins() {
	m := interp.module
	ctx := interp.ctx

	for _, varID := range interp.ep.InterfaceIDs {
		vi, ok := m.Variables[varID]
		if !ok {
			continue
		}
		if vi.StorageClass != StorageClassInput {
			continue
		}

		bi := m.GetBuiltIn(varID)
		if bi < 0 {
			continue
		}

		var val Value
		switch bi {
		case BuiltInGlobalInvocationID:
			val = uvec3ToValue(ctx.GlobalInvocationID)
		case BuiltInLocalInvocationID:
			val = uvec3ToValue(ctx.LocalInvocationID)
		case BuiltInWorkgroupID:
			val = uvec3ToValue(ctx.WorkgroupID)
		case BuiltInNumWorkgroups:
			val = uvec3ToValue(ctx.NumWorkgroups)
		case BuiltInWorkgroupSize:
			val = uvec3ToValue(ctx.WorkgroupSize)
		case BuiltInLocalInvocationIdx:
			val = ValUint(ctx.LocalInvocationIndex)
		default:
			continue
		}

		interp.values[varID] = ValPointer(interp.allocPointer(val))
	}
}

// initWorkgroupVariables points the Workgroup storage class variables at
// workgroup memory. Every invocation of a workgroup gets the same
// BufferPointers into the same bytes, so a store by one is seen by the
// others (WGSL: the workgroup address space is shared among the invocations
// of a workgroup). The memory is the context's WorkgroupSharedMemory buffer
// of the variable, or zeroed memory of the invocation's own if the context
// has none of the variable's size.
func (interp *interpreter) initWorkgroupVariables() {
	m := interp.module
	ctx := interp.ctx
	prep := m.prepared()
	if ctx != nil && len(ctx.workgroupPtrs) == len(prep.workgroup) {
		for i, varID := range prep.workgroup {
			interp.values[varID] = ctx.workgroupPtrs[i]
		}
		return
	}
	for _, varID := range prep.workgroup {
		vi := m.Variables[varID]
		pointee := m.PointeeType(vi.TypeID)
		if pointee == nil {
			interp.values[varID] = ValPointer(interp.allocPointer(zeroValueForVar(m, vi.TypeID)))
			continue
		}
		size := typeByteSize(m, pointee)
		var buf []byte
		if ctx != nil {
			buf = ctx.WorkgroupSharedMemory[varID]
		}
		if uint32(len(buf)) < size {
			buf = make([]byte, size)
		}
		interp.values[varID] = ValBufferPointer(&BufferPointer{Buffer: buf, Type: pointee})
	}
}

// uvec3ToValue returns a compute built-in (a vec3<u32>) as a uvec3 value.
func uvec3ToValue(v [3]uint32) Value {
	return ValUVec(3, [4]uint32{v[0], v[1], v[2]})
}

// DispatchCompute executes a compute shader for all invocations in the dispatch.
// groupCountX/Y/Z specify the number of workgroups to dispatch.
// The entry point's LocalSize execution mode determines the workgroup dimensions.
//
// The workgroups run one after another, each with its own zero-initialized
// workgroup memory, which all its invocations share. The invocations of a
// workgroup run on one goroutine, round-robin from barrier to barrier: each
// runs until it reaches a workgroup barrier (OpControlBarrier with Workgroup
// execution scope) or returns, and the invocations at the barrier go on
// past it only once all of them have run (see workgroupRun).
func (m *Module) DispatchCompute(entryPoint string, ctx *ExecutionContext,
	groupCountX, groupCountY, groupCountZ uint32) error {
	ep, fn, err := m.computeEntryPoint(entryPoint)
	if err != nil {
		return err
	}
	wgSize := m.GetWorkgroupSize(entryPoint)

	ctx.NumWorkgroups = [3]uint32{groupCountX, groupCountY, groupCountZ}
	ctx.WorkgroupSize = wgSize
	// Every invocation copies ctx; created here, the resource cache is shared
	// by all of them instead of being rebuilt per copy.
	if ctx.resolved == nil {
		ctx.resolved = &resolvedResources{}
	}
	if uint64(wgSize[0])*uint64(wgSize[1])*uint64(wgSize[2]) == 0 ||
		uint64(groupCountX)*uint64(groupCountY)*uint64(groupCountZ) == 0 {
		return nil
	}

	wg := m.newWorkgroupRun(ep, fn, ctx, wgSize)
	for wgZ := uint32(0); wgZ < groupCountZ; wgZ++ {
		for wgY := uint32(0); wgY < groupCountY; wgY++ {
			for wgX := uint32(0); wgX < groupCountX; wgX++ {
				if err := wg.run([3]uint32{wgX, wgY, wgZ}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// workgroupRun runs the workgroups of one dispatch, one at a time. What it
// holds is reused by every workgroup: the workgroup memory (cleared before
// each), the invocations' contexts and the list of running invocations.
type workgroupRun struct {
	module *Module
	ep     *EntryPoint
	fn     *Function
	base   *ExecutionContext
	size   [3]uint32
	mem    *workgroupMemory
	ctxs   []ExecutionContext // ctxs[i] is the context of invocation i
	live   []*interpreter     // the invocations not finished yet
}

// newWorkgroupRun prepares to run workgroups of size invocations of the
// entry point ep (function fn) in the dispatch context base.
func (m *Module) newWorkgroupRun(ep *EntryPoint, fn *Function, base *ExecutionContext, size [3]uint32) *workgroupRun {
	n := size[0] * size[1] * size[2]
	return &workgroupRun{
		module: m,
		ep:     ep,
		fn:     fn,
		base:   base,
		size:   size,
		mem:    m.newWorkgroupMemory(),
		ctxs:   make([]ExecutionContext, n),
		live:   make([]*interpreter, 0, n),
	}
}

// run runs workgroup id to completion. Each pass of the loop runs every
// unfinished invocation up to its next barrier (or its end), so when a pass
// is over, every invocation has made the workgroup memory writes it makes
// before that barrier, and the next pass releases them all past it.
//
// WGSL requires a workgroupBarrier in uniform control flow, so all of a
// workgroup's invocations reach the same barriers. If they do not (SPIR-V
// that breaks the rule), an invocation waits only for those still running,
// as no GPU defines either.
func (wg *workgroupRun) run(id [3]uint32) error {
	m := wg.module
	clear(wg.mem.bytes)
	sx, sy := wg.size[0], wg.size[1]
	syncBarriers := len(wg.ctxs) > 1
	for i := range wg.ctxs {
		local := [3]uint32{uint32(i) % sx, uint32(i) / sx % sy, uint32(i) / (sx * sy)}
		c := &wg.ctxs[i]
		*c = *wg.base
		c.WorkgroupID = id
		c.LocalInvocationID = local
		c.GlobalInvocationID = [3]uint32{id[0]*sx + local[0], id[1]*sy + local[1], id[2]*wg.size[2] + local[2]}
		c.LocalInvocationIndex = uint32(i)
		c.WorkgroupSharedMemory = wg.mem.byVar
		c.workgroupPtrs = wg.mem.ptrs
		interp := m.startCompute(wg.ep, wg.fn, c)
		interp.syncBarriers = syncBarriers
		wg.live = append(wg.live, interp)
	}

	for len(wg.live) > 0 {
		next := wg.live[:0]
		for j, interp := range wg.live {
			if err := interp.run(); err != nil {
				c := interp.ctx
				m.release(interp)
				wg.abandon(next, wg.live[j+1:])
				return fmt.Errorf("spirv: compute invocation (%d,%d,%d) in workgroup (%d,%d,%d): %w",
					c.LocalInvocationID[0], c.LocalInvocationID[1], c.LocalInvocationID[2],
					id[0], id[1], id[2], err)
			}
			if interp.suspended {
				next = append(next, interp)
				continue
			}
			interp.writeStorageBufferBack()
			m.putInterpreter(interp)
		}
		clear(wg.live[len(next):])
		wg.live = next
	}
	return nil
}

// abandon returns the invocations still waiting at a barrier to the pool
// after another one failed, and empties the list of running invocations.
func (wg *workgroupRun) abandon(lists ...[]*interpreter) {
	for _, list := range lists {
		for _, interp := range list {
			wg.module.release(interp)
		}
	}
	clear(wg.live[:cap(wg.live)])
	wg.live = wg.live[:0]
}

// workgroupMemory is the memory of the Workgroup variables of one workgroup:
// one zeroed byte buffer holding them all, each variable's part of it, and a
// BufferPointer to each part, in the order of modulePrep.workgroup, that
// every invocation of the workgroup uses as the variable's pointer.
type workgroupMemory struct {
	bytes []byte
	byVar map[uint32][]byte
	ptrs  []Value
}

// workgroupVarAlign is the alignment of each variable in workgroupMemory.
const workgroupVarAlign = 16

// newWorkgroupMemory allocates zeroed memory for the module's Workgroup
// variables.
func (m *Module) newWorkgroupMemory() *workgroupMemory {
	vars := m.prepared().workgroup
	offsets := make([]uint32, len(vars))
	sizes := make([]uint32, len(vars))
	var total uint32
	for i, varID := range vars {
		pointee := m.PointeeType(m.Variables[varID].TypeID)
		if pointee == nil {
			continue
		}
		total = (total + workgroupVarAlign - 1) / workgroupVarAlign * workgroupVarAlign
		offsets[i] = total
		sizes[i] = typeByteSize(m, pointee)
		total += sizes[i]
	}
	wm := &workgroupMemory{
		bytes: make([]byte, total),
		byVar: make(map[uint32][]byte, len(vars)),
		ptrs:  make([]Value, len(vars)),
	}
	for i, varID := range vars {
		vi := m.Variables[varID]
		pointee := m.PointeeType(vi.TypeID)
		if pointee == nil {
			// Not a pointer type: nothing to lay out. Shared like the rest.
			wm.ptrs[i] = ValPointer(&Pointer{Val: zeroValueForVar(m, vi.TypeID)})
			continue
		}
		part := wm.bytes[offsets[i] : offsets[i]+sizes[i] : offsets[i]+sizes[i]]
		wm.byVar[varID] = part
		wm.ptrs[i] = ValBufferPointer(&BufferPointer{Buffer: part, Type: pointee})
	}
	return wm
}

// allocateWorkgroupMemory creates zero-initialized shared memory buffers
// for all Workgroup storage class variables, by variable ID.
func (m *Module) allocateWorkgroupMemory() map[uint32][]byte {
	return m.newWorkgroupMemory().byVar
}

// =============================================================================
// Atomic Operations
// =============================================================================

// atomicMu protects atomic operations on shared memory.
// In a single-threaded interpreter this is not strictly necessary,
// but it ensures correctness if we ever add multi-threaded workgroups.
var atomicMu sync.Mutex

// executeAtomicOp performs an atomic read-modify-write on a storage buffer or
// shared memory. It returns the original value before the modification.
func (interp *interpreter) executeAtomicOp(inst Instruction) Value {
	// Atomic ops: OpAtomicIAdd, OpAtomicISub, etc.
	// Operands: pointer, scope, semantics [, value]
	if len(inst.Operands) < 3 {
		return ValUint(0)
	}

	ptrID := inst.Operands[0]
	// scope and semantics are inst.Operands[1] and [2] -- ignored in single-threaded.

	pv := interp.values[ptrID]

	atomicMu.Lock()
	defer atomicMu.Unlock()

	// The pointer is a storage buffer word (BufferPointer), a workgroup or
	// function variable (Pointer) or a member of one (SubPointer); the result
	// is written back through the same pointer so storage buffer atomics land
	// in the buffer.
	oldVal, ok := atomicLoad(pv)
	if !ok {
		return ValUint(0)
	}
	// The result and the stored word have the instruction's integer kind, so
	// an atomic<i32> reads back as i32.
	k := kindUint
	if inst.resShape.kind() == kindSint {
		k = kindSint
	}
	store := func(v Value) { interp.storeThrough(pv, v) }
	storeWord := func(w uint32) { interp.storeThrough(pv, scalarOfKind(k, w)) }

	switch inst.Opcode {
	case OpAtomicIAdd:
		if len(inst.Operands) >= 4 {
			addVal := toUint32(interp.values[inst.Operands[3]])
			storeWord(oldVal + addVal)
		}
	case OpAtomicISub:
		if len(inst.Operands) >= 4 {
			subVal := toUint32(interp.values[inst.Operands[3]])
			storeWord(oldVal - subVal)
		}
	case OpAtomicExchange:
		if len(inst.Operands) >= 4 {
			store(interp.values[inst.Operands[3]])
		}
	case OpAtomicCompareExchange:
		// Operands: pointer, scope, equal_sem, unequal_sem, value, comparator
		if len(inst.Operands) >= 6 {
			newVal := toUint32(interp.values[inst.Operands[4]])
			comparator := toUint32(interp.values[inst.Operands[5]])
			if oldVal == comparator {
				storeWord(newVal)
			}
		}
	case OpAtomicSMin:
		if len(inst.Operands) >= 4 {
			v := int32(toUint32(interp.values[inst.Operands[3]]))
			old := int32(oldVal)
			if v < old {
				storeWord(uint32(v))
			}
		}
	case OpAtomicUMin:
		if len(inst.Operands) >= 4 {
			v := toUint32(interp.values[inst.Operands[3]])
			if v < oldVal {
				storeWord(v)
			}
		}
	case OpAtomicSMax:
		if len(inst.Operands) >= 4 {
			v := int32(toUint32(interp.values[inst.Operands[3]]))
			old := int32(oldVal)
			if v > old {
				storeWord(uint32(v))
			}
		}
	case OpAtomicUMax:
		if len(inst.Operands) >= 4 {
			v := toUint32(interp.values[inst.Operands[3]])
			if v > oldVal {
				storeWord(v)
			}
		}
	case OpAtomicIIncrement:
		storeWord(oldVal + 1)
	case OpAtomicIDecrement:
		storeWord(oldVal - 1)
	case OpAtomicLoad:
		// Load is just a read -- no modification.
	case OpAtomicStore:
		// Store is special: no return value.
		if len(inst.Operands) >= 4 {
			store(interp.values[inst.Operands[3]])
		}
		return Value{}
	}

	return scalarOfKind(k, oldVal)
}

// atomicLoad reads the 32-bit integer an atomic op's pointer points at. An
// atomic is always a 4-byte i32 or u32, so a storage buffer word is read
// directly. ok is false for a value that is not a pointer.
func atomicLoad(pv Value) (uint32, bool) {
	switch pv.Tag {
	case TagPointer:
		if p := pv.AsPointer(); p != nil {
			return toUint32(p.Val), true
		}
	case TagSubPointer:
		if sp := pv.AsSubPointer(); sp != nil {
			return toUint32(subPointerLoad(sp)), true
		}
	case TagBufferPointer:
		if bp := pv.AsBufferPointer(); bp != nil && uint64(bp.Offset)+4 <= uint64(len(bp.Buffer)) {
			return binary.LittleEndian.Uint32(bp.Buffer[bp.Offset:]), true
		}
	}
	return 0, false
}

// writeStorageBufferBack writes modified storage buffer values back to the
// context's raw buffer data. Called after compute shader execution to
// reflect in-memory changes to the bound buffers.
func (interp *interpreter) writeStorageBufferBack() {
	m := interp.module
	ctx := interp.ctx
	if ctx == nil || ctx.Buffers == nil {
		return
	}

	for _, varID := range m.prepared().resources {
		vi := m.Variables[varID]
		if vi.StorageClass != StorageClassStorageBuffer {
			continue
		}
		bk, hasBind := m.GetBinding(varID)
		if !hasBind {
			continue
		}
		bufData := ctx.Buffers[bk]
		if bufData == nil {
			continue
		}
		v := interp.values[varID]
		if v.Tag == TagPointer {
			interp.writeValueToBufferTyped(bufData, 0, v.AsPointer().Val, m.PointeeType(vi.TypeID), 0)
		}
	}
}

// putUint32LE writes a uint32 in little-endian order.
func putUint32LE(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}
