//go:build !(js && wasm)

// Compute shader execution for the SPIR-V interpreter.
//
// Executes a compute entry point for a single invocation within a workgroup.
// The caller is responsible for iterating over all invocations in the dispatch.

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
// Storage buffer writes are reflected in the context's Buffers map.
func (m *Module) ExecuteCompute(entryPoint string, ctx *ExecutionContext) error {
	if ctx == nil {
		ctx = &ExecutionContext{}
	}

	ep, ok := m.EntryPoints[entryPoint]
	if !ok {
		return fmt.Errorf("spirv: entry point %q not found", entryPoint)
	}
	if ep.ExecutionModel != ExecutionModelGLCompute {
		return fmt.Errorf("spirv: entry point %q is not a compute shader (model=%d)",
			entryPoint, ep.ExecutionModel)
	}

	fn, ok := m.Functions[entryPoint]
	if !ok {
		return fmt.Errorf("spirv: function body for %q not found", entryPoint)
	}

	// Acquire a pooled interpreter to avoid allocating per call.
	interp := m.getInterpreter()
	interp.ep = ep
	interp.fn = fn
	interp.ctx = ctx
	interp.prevBlock = 0
	interp.iterationCount = 0
	interp.callDepth = 0
	interp.returnValue = Value{}

	// Seed constants.
	for id, val := range m.Constants {
		interp.values[id] = val
	}

	// Initialize compute built-in inputs.
	interp.initComputeBuiltins()

	// Initialize resource bindings (uniform, storage buffers).
	interp.initResourceVariables()

	// Initialize workgroup shared memory variables.
	interp.initWorkgroupVariables()

	// Execute.
	if err := interp.run(); err != nil {
		m.putInterpreter(interp)
		return err
	}

	// Write storage buffer changes back to the context's raw buffer data.
	interp.writeStorageBufferBack()

	m.putInterpreter(interp)
	return nil
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

// initWorkgroupVariables sets up Workgroup storage class variables using
// shared memory from the execution context.
func (interp *interpreter) initWorkgroupVariables() {
	m := interp.module
	ctx := interp.ctx
	if ctx == nil {
		return
	}

	for varID, vi := range m.Variables {
		if vi.StorageClass != StorageClassWorkgroup {
			continue
		}

		if ctx.WorkgroupSharedMemory != nil {
			if sharedBuf, ok := ctx.WorkgroupSharedMemory[varID]; ok {
				// Read the shared memory into a structured value.
				pointeeType := m.PointeeType(vi.TypeID)
				if pointeeType != nil {
					val := interp.readValueFromBuffer(sharedBuf, 0, pointeeType)
					interp.values[varID] = ValPointer(interp.allocPointer(val))
					continue
				}
			}
		}

		// Default: zero-initialized.
		interp.values[varID] = ValPointer(interp.allocPointer(zeroValueForVar(m, vi.TypeID)))
	}
}

// uvec3ToValue converts a [3]uint32 to an Array of 3 Uint32 values.
// SPIR-V represents compute built-ins as uvec3 (vector of 3 uint32).
func uvec3ToValue(v [3]uint32) Value {
	// Store as an Array of 3 Uint32 values for proper integer handling.
	arr := []Value{ValUint(v[0]), ValUint(v[1]), ValUint(v[2])}
	return ValArray(arr)
}

// DispatchCompute executes a compute shader for all invocations in the dispatch.
// groupCountX/Y/Z specify the number of workgroups to dispatch.
// The entry point's LocalSize execution mode determines the workgroup dimensions.
//
// This is a single-threaded execution: invocations run sequentially within each
// workgroup. OpControlBarrier is a no-op since there is no true parallelism.
func (m *Module) DispatchCompute(entryPoint string, ctx *ExecutionContext,
	groupCountX, groupCountY, groupCountZ uint32) error {
	wgSize := m.GetWorkgroupSize(entryPoint)

	ctx.NumWorkgroups = [3]uint32{groupCountX, groupCountY, groupCountZ}
	ctx.WorkgroupSize = wgSize

	for wgZ := uint32(0); wgZ < groupCountZ; wgZ++ {
		for wgY := uint32(0); wgY < groupCountY; wgY++ {
			for wgX := uint32(0); wgX < groupCountX; wgX++ {
				// Allocate shared memory for this workgroup.
				sharedMem := m.allocateWorkgroupMemory()

				// Execute all invocations in the workgroup sequentially.
				for lz := uint32(0); lz < wgSize[2]; lz++ {
					for ly := uint32(0); ly < wgSize[1]; ly++ {
						for lx := uint32(0); lx < wgSize[0]; lx++ {
							invCtx := *ctx // Copy context for this invocation.
							invCtx.WorkgroupID = [3]uint32{wgX, wgY, wgZ}
							invCtx.LocalInvocationID = [3]uint32{lx, ly, lz}
							invCtx.GlobalInvocationID = [3]uint32{
								wgX*wgSize[0] + lx,
								wgY*wgSize[1] + ly,
								wgZ*wgSize[2] + lz,
							}
							invCtx.LocalInvocationIndex = lz*wgSize[0]*wgSize[1] + ly*wgSize[0] + lx
							invCtx.WorkgroupSharedMemory = sharedMem

							if err := m.ExecuteCompute(entryPoint, &invCtx); err != nil {
								return fmt.Errorf("spirv: compute invocation (%d,%d,%d) in workgroup (%d,%d,%d): %w",
									lx, ly, lz, wgX, wgY, wgZ, err)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// allocateWorkgroupMemory creates zero-initialized shared memory buffers
// for all Workgroup storage class variables.
func (m *Module) allocateWorkgroupMemory() map[uint32][]byte {
	shared := make(map[uint32][]byte)
	for varID, vi := range m.Variables {
		if vi.StorageClass != StorageClassWorkgroup {
			continue
		}
		pointeeType := m.PointeeType(vi.TypeID)
		if pointeeType == nil {
			continue
		}
		size := typeByteSize(m, pointeeType)
		if size > 0 {
			shared[varID] = make([]byte, size)
		}
	}
	return shared
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
	if pv.Tag != TagPointer {
		return ValUint(0)
	}
	ptr := pv.AsPointer()

	atomicMu.Lock()
	defer atomicMu.Unlock()

	oldVal := toUint32(ptr.Val)

	switch inst.Opcode {
	case OpAtomicIAdd:
		if len(inst.Operands) >= 4 {
			addVal := toUint32(interp.values[inst.Operands[3]])
			ptr.Val = ValUint(oldVal + addVal)
		}
	case OpAtomicISub:
		if len(inst.Operands) >= 4 {
			subVal := toUint32(interp.values[inst.Operands[3]])
			ptr.Val = ValUint(oldVal - subVal)
		}
	case OpAtomicExchange:
		if len(inst.Operands) >= 4 {
			ptr.Val = interp.values[inst.Operands[3]]
		}
	case OpAtomicCompareExchange:
		// Operands: pointer, scope, equal_sem, unequal_sem, value, comparator
		if len(inst.Operands) >= 6 {
			newVal := toUint32(interp.values[inst.Operands[4]])
			comparator := toUint32(interp.values[inst.Operands[5]])
			if oldVal == comparator {
				ptr.Val = ValUint(newVal)
			}
		}
	case OpAtomicSMin:
		if len(inst.Operands) >= 4 {
			v := int32(toUint32(interp.values[inst.Operands[3]]))
			old := int32(oldVal)
			if v < old {
				ptr.Val = ValUint(uint32(v))
			}
		}
	case OpAtomicUMin:
		if len(inst.Operands) >= 4 {
			v := toUint32(interp.values[inst.Operands[3]])
			if v < oldVal {
				ptr.Val = ValUint(v)
			}
		}
	case OpAtomicSMax:
		if len(inst.Operands) >= 4 {
			v := int32(toUint32(interp.values[inst.Operands[3]]))
			old := int32(oldVal)
			if v > old {
				ptr.Val = ValUint(uint32(v))
			}
		}
	case OpAtomicUMax:
		if len(inst.Operands) >= 4 {
			v := toUint32(interp.values[inst.Operands[3]])
			if v > oldVal {
				ptr.Val = ValUint(v)
			}
		}
	case OpAtomicIIncrement:
		ptr.Val = ValUint(oldVal + 1)
	case OpAtomicIDecrement:
		ptr.Val = ValUint(oldVal - 1)
	case OpAtomicLoad:
		// Load is just a read -- no modification.
	case OpAtomicStore:
		// Store is special: no return value.
		if len(inst.Operands) >= 4 {
			ptr.Val = interp.values[inst.Operands[3]]
		}
		return Value{}
	}

	return ValUint(oldVal)
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

	for varID, vi := range m.Variables {
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
			writeValueToBuffer(bufData, 0, v.AsPointer().Val)
		}
	}
}

// putUint32LE writes a uint32 in little-endian order.
func putUint32LE(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}
