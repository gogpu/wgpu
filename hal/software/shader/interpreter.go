//go:build !(js && wasm)

// SPIR-V interpreter for the software backend.
//
// Executes a single SPIR-V entry point with provided inputs and returns outputs.
// This is a minimal interpreter sufficient for the gogpu triangle shader:
//   - OpLoad / OpStore for variable access
//   - OpAccessChain for array/composite indexing
//   - OpCompositeConstruct / OpCompositeExtract for building/decomposing vectors
//   - OpVariable for local storage
//   - OpBranch for unconditional jumps between basic blocks
//   - OpConvertUToF for uint-to-float conversion
//   - Basic arithmetic: OpFAdd, OpFSub, OpFMul, OpFDiv, OpFNegate
//   - Basic integer arithmetic: OpIAdd, OpISub, OpIMul
//
// Control flow beyond OpBranch (loops, conditionals) is NOT implemented.

package shader

import (
	"encoding/json"
	"fmt"
	"math"
)

// Execute runs the named entry point with the given input variable values.
//
// inputs maps variable ID (decorated with BuiltIn or Location) to its value.
// Returns a map of output variable ID to its value after execution.
//
// For a vertex shader, inputs typically contain:
//   - vertex_index variable ID -> Uint32 value
//
// Outputs typically contain:
//   - position variable ID -> Vec4 value
func (m *Module) Execute(entryPoint string, inputs map[uint32]Value) (map[uint32]Value, error) {
	ctx := &ExecutionContext{Inputs: inputs}
	return m.ExecuteWithContext(entryPoint, ctx)
}

// ExecuteWithContext runs the named entry point with full resource context.
// The context provides bound buffers, textures, and samplers for the shader.
func (m *Module) ExecuteWithContext(entryPoint string, ctx *ExecutionContext) (map[uint32]Value, error) {
	return m.ExecuteWithDebug(entryPoint, ctx, nil)
}

// ExecuteWithDebug runs the named entry point with full resource context and
// optional debug support. When debug is nil, execution proceeds at full speed
// with zero overhead -- the existing Execute and ExecuteWithContext methods
// call this with debug=nil.
//
// When debug is non-nil, the interpreter fires callbacks for breakpoints,
// variable watches, and instruction tracing. See DebugContext for details.
func (m *Module) ExecuteWithDebug(entryPoint string, ctx *ExecutionContext, debug *DebugContext) (map[uint32]Value, error) {
	if ctx == nil {
		ctx = &ExecutionContext{}
	}

	ep, ok := m.EntryPoints[entryPoint]
	if !ok {
		return nil, fmt.Errorf("spirv: entry point %q not found", entryPoint)
	}

	fn, ok := m.Functions[entryPoint]
	if !ok {
		return nil, fmt.Errorf("spirv: function body for %q not found", entryPoint)
	}

	// Acquire a pooled interpreter to avoid allocating per call.
	interp := m.getInterpreter()

	interp.ep = ep
	interp.fn = fn
	interp.ctx = ctx
	interp.debug = debug
	interp.prevBlock = 0
	interp.iterationCount = 0
	interp.callDepth = 0
	interp.returnValue = Value{}

	// Seed constants into the value slice.
	for id, val := range m.Constants {
		interp.values[id] = val
	}

	// Initialize interface variables (inputs/outputs) and resource bindings.
	inputs := ctx.Inputs
	interp.initVariables(inputs)
	interp.initResourceVariables()

	// Execute the function body.
	if err := interp.run(); err != nil {
		m.putInterpreter(interp)
		return nil, err
	}

	// Collect output values.
	outputs := make(map[uint32]Value)
	for _, varID := range ep.InterfaceIDs {
		vi, ok := m.Variables[varID]
		if !ok {
			continue
		}
		if vi.StorageClass != StorageClassOutput {
			continue
		}
		v := interp.values[varID]
		if v.Tag == TagPointer {
			outputs[varID] = v.AsPointer().Val
		}
	}

	// Return interpreter to pool for reuse.
	m.putInterpreter(interp)

	return outputs, nil
}

// maxIterations is the safety limit for loop iterations to prevent infinite loops.
const maxIterations = 100000

// interpreter holds execution state for a single entry point invocation.
//
// Values are stored in a flat slice indexed by SSA ID (pre-allocated to module Bound size)
// instead of a map. This eliminates map creation, hashing, and growth allocations on
// the hot path. The Bound field from the SPIR-V header gives the exact size needed.
type interpreter struct {
	module *Module
	ep     *EntryPoint
	fn     *Function
	ctx    *ExecutionContext
	values []Value // indexed by SSA ID, length == module.Bound

	// prevBlock tracks the predecessor block ID for OpPhi resolution.
	prevBlock uint32

	// iterationCount tracks total branch-back iterations for loop safety.
	iterationCount int

	// callDepth tracks function call nesting to prevent stack overflow.
	callDepth int

	// returnValue holds the value from OpReturnValue for function calls.
	returnValue Value

	// debug holds the debug context for breakpoints, tracing, and watch variables.
	// When nil, all debug logic is skipped (zero overhead on the hot path).
	debug *DebugContext

	// ptrPool is a pre-allocated pool of Pointer objects to avoid heap
	// allocation for the common case of OpVariable and OpAccessChain.
	// ptrPoolIdx tracks the next available slot.
	ptrPool    []Pointer
	ptrPoolIdx int
}

// ptrPoolSize is the initial capacity of the Pointer pool per interpreter.
// Most shaders use fewer than 32 pointer allocations per Execute call.
const ptrPoolSize = 32

// allocPointer returns a Pointer initialized with the given value.
// Uses the pre-allocated pool when possible, falling back to heap allocation.
func (interp *interpreter) allocPointer(val Value) *Pointer {
	if interp.ptrPoolIdx < len(interp.ptrPool) {
		p := &interp.ptrPool[interp.ptrPoolIdx]
		p.Val = val
		interp.ptrPoolIdx++
		return p
	}
	return &Pointer{Val: val}
}

// getInterpreter returns a pooled interpreter, allocating one if the pool is empty.
// The returned interpreter has its values slice cleared and ready for reuse.
func (m *Module) getInterpreter() *interpreter {
	if v := m.interpPool.Get(); v != nil {
		interp := v.(*interpreter)
		// Clear all values from previous execution without reallocating.
		// This is O(n) but avoids map creation which is the dominant cost.
		clear(interp.values)
		// Reset pointer pool index so pooled Pointers are reused.
		interp.ptrPoolIdx = 0
		return interp
	}
	return &interpreter{
		module:  m,
		values:  make([]Value, m.Bound),
		ptrPool: make([]Pointer, ptrPoolSize),
	}
}

// putInterpreter returns an interpreter to the pool for reuse.
func (m *Module) putInterpreter(interp *interpreter) {
	// Clear references to help GC, but keep the values slice.
	interp.ep = nil
	interp.fn = nil
	interp.ctx = nil
	interp.debug = nil
	interp.returnValue = Value{}
	m.interpPool.Put(interp)
}

// initVariables sets up input, output, and function-local variables.
func (interp *interpreter) initVariables(inputs map[uint32]Value) {
	m := interp.module

	for _, varID := range interp.ep.InterfaceIDs {
		vi, ok := m.Variables[varID]
		if !ok {
			continue
		}

		switch vi.StorageClass {
		case StorageClassInput:
			// Seed from caller-provided inputs.
			val := inputs[varID]
			if val.IsNone() {
				// Default zero value based on pointee type.
				val = zeroValueForVar(m, vi.TypeID)
			}
			interp.values[varID] = ValPointer(interp.allocPointer(val))

		case StorageClassOutput:
			interp.values[varID] = ValPointer(interp.allocPointer(zeroValueForVar(m, vi.TypeID)))
		}
	}
}

// initResourceVariables sets up Uniform/StorageBuffer/UniformConstant variables
// by reading data from bound buffers in the execution context.
func (interp *interpreter) initResourceVariables() {
	m := interp.module
	ctx := interp.ctx
	if ctx == nil {
		return
	}

	for varID, vi := range m.Variables {
		switch vi.StorageClass {
		case StorageClassUniform, StorageClassStorageBuffer, StorageClassPushConstant:
			bk, hasBind := m.GetBinding(varID)
			if !hasBind {
				continue
			}
			var bufData []byte
			if ctx.Buffers != nil {
				bufData = ctx.Buffers[bk]
			}
			if bufData == nil {
				// No buffer bound -- initialize as zero.
				interp.values[varID] = ValPointer(interp.allocPointer(zeroValueForVar(m, vi.TypeID)))
				continue
			}
			pointeeType := m.PointeeType(vi.TypeID)
			if pointeeType == nil {
				interp.values[varID] = ValPointer(interp.allocPointer(zeroValueForVar(m, vi.TypeID)))
				continue
			}
			if vi.StorageClass == StorageClassStorageBuffer {
				// Storage buffers use BufferPointer for direct read/write to raw bytes.
				// This ensures OpStore writes are immediately reflected in the buffer.
				interp.values[varID] = ValBufferPointer(&BufferPointer{Buffer: bufData, Offset: 0, Type: pointeeType})
			} else {
				// Uniform and push constant buffers are read-only -- deserialize once.
				val := interp.readValueFromBuffer(bufData, 0, pointeeType)
				interp.values[varID] = ValPointer(interp.allocPointer(val))
			}

		case StorageClassUniformConstant:
			// UniformConstant is used for textures/samplers -- handled separately.
			bk, hasBind := m.GetBinding(varID)
			if !hasBind {
				continue
			}
			// Store the binding key as the value so OpLoad can resolve texture/sampler.
			interp.values[varID] = ValPointer(interp.allocPointer(ValBindingKey(bk)))
		}
	}
}

// readValueFromBuffer deserializes a SPIR-V typed value from raw buffer bytes.
// offset is the starting byte offset into data.
func (interp *interpreter) readValueFromBuffer(data []byte, offset uint32, ti *TypeInfo) Value {
	m := interp.module
	switch ti.Kind {
	case TypeFloat:
		if offset+4 > uint32(len(data)) {
			return ValFloat(0)
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		return ValFloat(math.Float32frombits(bits))

	case TypeInt:
		if offset+4 > uint32(len(data)) {
			if ti.Signed {
				return ValInt(0)
			}
			return ValUint(0)
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		if ti.Signed {
			return ValInt(int32(bits))
		}
		return ValUint(bits)

	case TypeBool:
		if offset+4 > uint32(len(data)) {
			return ValBool(false)
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		return ValBool(bits != 0)

	case TypeVector:
		elemType := m.Types[ti.ElemType]
		if elemType == nil {
			return zeroValue(m.Types, ti.ElemType)
		}
		elemSize := typeByteSize(m, elemType)
		switch ti.Components {
		case 2:
			var v [2]float32
			for i := uint32(0); i < 2; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return ValVec2(v[0], v[1])
		case 3:
			var v [3]float32
			for i := uint32(0); i < 3; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return ValVec3(v[0], v[1], v[2])
		case 4:
			var v [4]float32
			for i := uint32(0); i < 4; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return ValVec4(v[0], v[1], v[2], v[3])
		}
		return ValUint(0)

	case TypeArray:
		elemType := m.Types[ti.ElemType]
		if elemType == nil || ti.Length == 0 {
			return ValArray(nil)
		}
		// Use ArrayStride decoration if available, otherwise compute.
		elemSize := typeByteSize(m, elemType)
		arr := make([]Value, ti.Length)
		for i := uint32(0); i < ti.Length; i++ {
			arr[i] = interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
		}
		return ValArray(arr)

	case TypeStruct:
		members := make([]Value, len(ti.MemberIDs))
		// For each struct member, look up its type and use MemberDecorate Offset.
		structTypeID := interp.findTypeID(ti)
		for i, memberTypeID := range ti.MemberIDs {
			memberType := m.Types[memberTypeID]
			if memberType == nil {
				members[i] = ValUint(0)
				continue
			}
			memberOffset := m.GetMemberOffset(structTypeID, uint32(i))
			members[i] = interp.readValueFromBuffer(data, offset+memberOffset, memberType)
		}
		return ValArray(members)

	default:
		return ValUint(0)
	}
}

// findTypeID returns the type ID for a given TypeInfo by scanning the type map.
// This is needed for MemberDecorate lookups which key on the type ID.
func (interp *interpreter) findTypeID(target *TypeInfo) uint32 {
	for id, ti := range interp.module.Types {
		if ti == target {
			return id
		}
	}
	return 0
}

// typeByteSize returns the byte size of a SPIR-V type for buffer reads.
func typeByteSize(m *Module, ti *TypeInfo) uint32 {
	switch ti.Kind {
	case TypeFloat:
		return ti.Width / 8
	case TypeInt:
		return ti.Width / 8
	case TypeBool:
		return 4 // SPIR-V bools are 32-bit in buffers.
	case TypeVector:
		elemType := m.Types[ti.ElemType]
		if elemType == nil {
			return 4 * ti.Components
		}
		return typeByteSize(m, elemType) * ti.Components
	case TypeArray:
		elemType := m.Types[ti.ElemType]
		if elemType == nil {
			return 0
		}
		return typeByteSize(m, elemType) * ti.Length
	case TypeStruct:
		// Compute struct size from the highest (offset + size) across all members.
		// Look up the struct type ID for MemberDecorate offset decorations.
		structTypeID := uint32(0)
		for id, t := range m.Types {
			if t == ti {
				structTypeID = id
				break
			}
		}

		var maxEnd uint32
		for i, memberTypeID := range ti.MemberIDs {
			memberType := m.Types[memberTypeID]
			if memberType == nil {
				continue
			}
			memberSize := typeByteSize(m, memberType)

			// Use the decorated offset if available, otherwise fall back to
			// sequential packing (4 bytes per prior member).
			memberOffset := uint32(0)
			if structTypeID != 0 {
				memberOffset = m.GetMemberOffset(structTypeID, uint32(i))
			}
			if memberOffset == 0 && i > 0 {
				// No decoration: estimate offset from index. This fallback is
				// rarely correct for non-trivial structs, but keeps backward
				// compatibility with undecorated types.
				memberOffset = uint32(i) * 4
			}

			end := memberOffset + memberSize
			if end > maxEnd {
				maxEnd = end
			}
		}
		return maxEnd
	default:
		return 4
	}
}

// errDebugAbort is a sentinel error returned when a debug callback requests DebugAbort.
var errDebugAbort = fmt.Errorf("spirv: execution aborted by debug callback")

// run executes instructions sequentially, handling OpBranch for jumps.
//
//nolint:maintidx // Opcode dispatch switch is inherently large.
func (interp *interpreter) run() error {
	instructions := interp.fn.Instructions
	pc := 0
	debug := interp.debug

	// Debug state -- only initialized when debug is non-nil.
	// This keeps the hot path (debug==nil) completely free of debug overhead.
	var (
		stepping bool
		traceEnc *json.Encoder
		traceEnt *traceEntry
	)
	if debug != nil {
		if debug.TraceEnabled && debug.TraceWriter != nil {
			traceEnc = json.NewEncoder(debug.TraceWriter)
			traceEnt = &traceEntry{}
		}
	}

	for pc < len(instructions) {
		inst := instructions[pc]

		// --- Debug: pre-instruction hooks ---
		if debug != nil { //nolint:nestif // Debug context checks require nested conditionals for breakpoints and stepping.
			blockLabel := interp.currentBlockLabel(pc)
			event := InstructionEvent{
				PC:          pc,
				Instruction: inst,
				Values:      interp.values,
				BlockLabel:  blockLabel,
			}

			// Breakpoint check.
			if debug.Breakpoints != nil && debug.Breakpoints[pc] {
				if debug.OnBreakpoint != nil {
					debug.OnBreakpoint(event)
				}
				// After a breakpoint, switch to stepping mode so OnInstruction
				// fires and the caller decides what to do next.
				stepping = true
			}

			// Stepping or watch-triggered: fire OnInstruction.
			if stepping && debug.OnInstruction != nil {
				action := debug.OnInstruction(event)
				switch action {
				case DebugAbort:
					return errDebugAbort
				case DebugContinue:
					stepping = false
				case DebugStep:
					stepping = true
				}
			}
		}

		// Capture the old value of the result ID for watch variable tracking.
		// Only done when debug is active and watches are configured.
		var watchOldValue Value
		var watchResultID uint32
		if debug != nil && len(debug.WatchVariables) > 0 && inst.ResultID != 0 {
			if debug.WatchVariables[inst.ResultID] {
				watchResultID = inst.ResultID
				watchOldValue = interp.values[watchResultID]
			}
		}

		pc++

		switch inst.Opcode {
		case OpLabel:
			// No-op at execution time; labels are already indexed.

		case OpVariable:
			// Function-local variable: allocate a Pointer with zero value.
			if len(inst.Operands) < 1 {
				break
			}
			storageClass := inst.Operands[0]
			if storageClass == StorageClassFunction {
				val := zeroValueForVar(interp.module, inst.TypeID)
				interp.values[inst.ResultID] = ValPointer(interp.allocPointer(val))
			}

		case OpLoad:
			// OpLoad: type resultID pointer [memory access]
			if len(inst.Operands) < 1 {
				break
			}
			ptrID := inst.Operands[0]
			pv := interp.values[ptrID]
			switch pv.Tag {
			case TagPointer:
				interp.values[inst.ResultID] = pv.AsPointer().Val
			case TagSubPointer:
				// Read through parent pointer, navigating the index path.
				interp.values[inst.ResultID] = subPointerLoad(pv.AsSubPointer())
			case TagBufferPointer:
				// Read directly from the raw buffer.
				bp := pv.AsBufferPointer()
				if bp.Type != nil {
					interp.values[inst.ResultID] = interp.readValueFromBuffer(bp.Buffer, bp.Offset, bp.Type)
				} else {
					interp.values[inst.ResultID] = ValUint(0)
				}
			default:
				// Direct value fallback (shouldn't happen in valid SPIR-V).
				interp.values[inst.ResultID] = interp.values[ptrID]
			}

		case OpStore:
			// OpStore: pointer value [memory access]
			if len(inst.Operands) < 2 {
				break
			}
			ptrID := inst.Operands[0]
			valID := inst.Operands[1]
			pv := interp.values[ptrID]
			switch pv.Tag {
			case TagPointer:
				pv.AsPointer().Val = interp.values[valID]
			case TagSubPointer:
				// Write through parent pointer, updating the composite at each level.
				subPointerStore(pv.AsSubPointer(), interp.values[valID])
			case TagBufferPointer:
				// Write directly to the raw buffer.
				writeValueToBuffer(pv.AsBufferPointer().Buffer, pv.AsBufferPointer().Offset, interp.values[valID])
			}

		case OpAccessChain:
			// OpAccessChain: type resultID base indexes...
			if len(inst.Operands) < 2 {
				break
			}
			baseID := inst.Operands[0]
			indexes := inst.Operands[1:]
			interp.values[inst.ResultID] = interp.accessChain(baseID, indexes)

		case OpCompositeConstruct:
			// OpCompositeConstruct: type resultID constituents...
			interp.values[inst.ResultID] = interp.compositeConstruct(inst.TypeID, inst.Operands)

		case OpCompositeExtract:
			// OpCompositeExtract: type resultID composite indexes...
			if len(inst.Operands) < 2 {
				break
			}
			compositeID := inst.Operands[0]
			indexes := inst.Operands[1:]
			interp.values[inst.ResultID] = interp.compositeExtract(interp.values[compositeID], indexes)

		case OpConvertUToF:
			// OpConvertUToF: type resultID value
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertToFloat(interp.values[inst.Operands[0]])

		case OpConvertSToF:
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertSignedToFloat(interp.values[inst.Operands[0]])

		case OpConvertFToU:
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertFloatToUint(interp.values[inst.Operands[0]])

		case OpBitcast:
			if len(inst.Operands) < 1 {
				break
			}
			// For now, bitcast just copies the value (works for same-width types).
			interp.values[inst.ResultID] = interp.values[inst.Operands[0]]

		case OpFAdd:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b float32) float32 { return a + b })
			}

		case OpFSub:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b float32) float32 { return a - b })
			}

		case OpFMul:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b float32) float32 { return a * b })
			}

		case OpFDiv:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b float32) float32 { return a / b })
			}

		case OpFNegate:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = floatUnaryOp(interp.values[inst.Operands[0]], func(a float32) float32 { return -a })
			}

		case OpIAdd:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a + b })
			}

		case OpISub:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a - b })
			}

		case OpIMul:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a * b })
			}

		case OpReturn:
			return nil

		case OpReturnValue:
			// OpReturnValue: valueID
			// Store the return value for function call return.
			if len(inst.Operands) >= 1 {
				interp.returnValue = interp.values[inst.Operands[0]]
			}
			return nil

		case OpBranch:
			// OpBranch: target label
			if len(inst.Operands) < 1 {
				break
			}
			targetLabel := inst.Operands[0]
			if idx, ok := interp.fn.Labels[targetLabel]; ok {
				// Track predecessor block for OpPhi.
				interp.prevBlock = interp.currentBlockLabel(pc - 1)
				// Detect backward branches (loops) and enforce iteration limit.
				if idx < pc {
					interp.iterationCount++
					if interp.iterationCount > maxIterations {
						return fmt.Errorf("spirv: exceeded maximum iterations (%d)", maxIterations)
					}
				}
				pc = idx + 1 // +1 to skip the label instruction itself
			}

		case OpFunctionParameter, OpFunctionEnd:
			// No-op during execution.

		case OpSelectionMerge:
			// Selection merge hint -- no-op for the interpreter.

		case OpLoopMerge:
			// Loop merge hint -- no-op. Operands: merge block, continue target, loop control.
			// The actual loop is driven by OpBranchConditional.

		case OpBranchConditional:
			if len(inst.Operands) >= 3 { //nolint:nestif // Branch + loop iteration guard requires nested depth checks.
				cond := interp.values[inst.Operands[0]]
				trueLabel := inst.Operands[1]
				falseLabel := inst.Operands[2]
				target := falseLabel
				if toBool(cond) {
					target = trueLabel
				}
				if idx, ok := interp.fn.Labels[target]; ok {
					interp.prevBlock = interp.currentBlockLabel(pc - 1)
					if idx < pc {
						interp.iterationCount++
						if interp.iterationCount > maxIterations {
							return fmt.Errorf("spirv: exceeded maximum iterations (%d)", maxIterations)
						}
					}
					pc = idx + 1
				}
			}

		case OpSelect:
			if len(inst.Operands) >= 3 {
				cond := interp.values[inst.Operands[0]]
				trueVal := interp.values[inst.Operands[1]]
				falseVal := interp.values[inst.Operands[2]]
				if toBool(cond) {
					interp.values[inst.ResultID] = trueVal
				} else {
					interp.values[inst.ResultID] = falseVal
				}
			}

		case OpIEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a == b)
			}

		case OpINotEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a != b)
			}

		case OpFOrdEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a == b)
			}

		case OpFOrdLessThan:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a < b)
			}

		case OpFOrdGreaterThan:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a > b)
			}

		case OpFOrdLessThanEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a <= b)
			}

		case OpFOrdGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a >= b)
			}

		case OpPhi:
			// OpPhi: type resultID (value parent)...
			// Select the value corresponding to the predecessor block we came from.
			if len(inst.Operands) >= 2 {
				// Operands are pairs: [value0, parent0, value1, parent1, ...]
				var resolved bool
				for i := 0; i+1 < len(inst.Operands); i += 2 {
					valID := inst.Operands[i]
					parentLabel := inst.Operands[i+1]
					if parentLabel == interp.prevBlock {
						interp.values[inst.ResultID] = interp.values[valID]
						resolved = true
						break
					}
				}
				// Fallback: if no matching parent found, use the first value.
				if !resolved && len(inst.Operands) >= 2 {
					interp.values[inst.ResultID] = interp.values[inst.Operands[0]]
				}
			}

		case OpSwitch:
			// OpSwitch: selector default (literal target)...
			// Operands: selector, defaultLabel, lit0, target0, lit1, target1, ...
			if len(inst.Operands) >= 2 {
				selector := toUint32(interp.values[inst.Operands[0]])
				defaultLabel := inst.Operands[1]
				target := defaultLabel
				for i := 2; i+1 < len(inst.Operands); i += 2 {
					lit := inst.Operands[i]
					lbl := inst.Operands[i+1]
					if selector == lit {
						target = lbl
						break
					}
				}
				if idx, ok := interp.fn.Labels[target]; ok {
					interp.prevBlock = interp.currentBlockLabel(pc - 1)
					pc = idx + 1
				}
			}

		case OpFunctionCall:
			// OpFunctionCall: type resultID function arg0 arg1 ...
			if len(inst.Operands) >= 1 {
				funcID := inst.Operands[0]
				args := inst.Operands[1:]
				result, err := interp.callFunction(funcID, args)
				if err != nil {
					return err
				}
				interp.values[inst.ResultID] = result
			}

		case OpKill:
			// Fragment shader discard -- stop execution with no error.
			return nil

		case OpUnreachable:
			// Should never be reached in valid SPIR-V.
			return fmt.Errorf("spirv: executed OpUnreachable")

		case OpVectorTimesScalar:
			// OpVectorTimesScalar: type resultID vector scalar
			if len(inst.Operands) >= 2 {
				vec := interp.values[inst.Operands[0]]
				s := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = vectorTimesScalar(vec, s)
			}

		case OpDot:
			// OpDot: type resultID vector1 vector2
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = dotProduct(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpMatrixTimesVector:
			// OpMatrixTimesVector: type resultID matrix vector
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = matrixTimesVector(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpMatrixTimesScalar:
			// OpMatrixTimesScalar: type resultID matrix scalar
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = matrixTimesScalar(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpMatrixTimesMatrix:
			// OpMatrixTimesMatrix: type resultID left right
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = matrixTimesMatrix(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpTranspose:
			// OpTranspose: type resultID matrix
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = transposeMatrix(interp.values[inst.Operands[0]])
			}

		case OpVectorShuffle:
			// OpVectorShuffle: type resultID vec1 vec2 components...
			if len(inst.Operands) >= 2 {
				vec1 := interp.values[inst.Operands[0]]
				vec2 := interp.values[inst.Operands[1]]
				components := inst.Operands[2:]
				interp.values[inst.ResultID] = vectorShuffle(interp.module, inst.TypeID, vec1, vec2, components)
			}

		case OpCopyObject:
			// OpCopyObject: type resultID operand
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.values[inst.Operands[0]]
			}

		case OpSDiv:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				if b != 0 {
					interp.values[inst.ResultID] = ValInt(a / b)
				} else {
					interp.values[inst.ResultID] = ValInt(0)
				}
			}

		case OpUDiv:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				if b != 0 {
					interp.values[inst.ResultID] = ValUint(a / b)
				} else {
					interp.values[inst.ResultID] = ValUint(0)
				}
			}

		case OpSMod:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				if b != 0 {
					interp.values[inst.ResultID] = ValInt(a % b)
				} else {
					interp.values[inst.ResultID] = ValInt(0)
				}
			}

		case OpUMod:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				if b != 0 {
					interp.values[inst.ResultID] = ValUint(a % b)
				} else {
					interp.values[inst.ResultID] = ValUint(0)
				}
			}

		case OpSRem:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				if b != 0 {
					// SPIR-V SRem: remainder has same sign as dividend.
					interp.values[inst.ResultID] = ValInt(a % b)
				} else {
					interp.values[inst.ResultID] = ValInt(0)
				}
			}

		case OpFMod:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(
					interp.values[inst.Operands[0]], interp.values[inst.Operands[1]],
					func(a, b float32) float32 {
						if b == 0 {
							return 0
						}
						// SPIR-V FMod: result has same sign as b (GLSL mod).
						r := float32(math.Mod(float64(a), float64(b)))
						if (r > 0 && b < 0) || (r < 0 && b > 0) {
							r += b
						}
						return r
					})
			}

		case OpFRem:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatBinOp(
					interp.values[inst.Operands[0]], interp.values[inst.Operands[1]],
					func(a, b float32) float32 {
						if b == 0 {
							return 0
						}
						return float32(math.Remainder(float64(a), float64(b)))
					})
			}

		case OpSNegate:
			if len(inst.Operands) >= 1 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				interp.values[inst.ResultID] = ValInt(-a)
			}

		case OpConvertFToS:
			if len(inst.Operands) >= 1 {
				f := toFloat32(interp.values[inst.Operands[0]])
				interp.values[inst.ResultID] = ValInt(int32(f))
			}

		case OpSConvert, OpUConvert, OpFConvert:
			// Type width conversions -- treat as copy for 32-bit only interpreter.
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.values[inst.Operands[0]]
			}

		case OpULessThan:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a < b)
			}

		case OpUGreaterThan:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a > b)
			}

		case OpULessThanEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a <= b)
			}

		case OpUGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = ValBool(a >= b)
			}

		case OpSLessThan:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = ValBool(a < b)
			}

		case OpSGreaterThan:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = ValBool(a > b)
			}

		case OpSLessThanEqual:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = ValBool(a <= b)
			}

		case OpSGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = ValBool(a >= b)
			}

		case OpLogicalAnd:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = ValBool(toBool(interp.values[inst.Operands[0]]) && toBool(interp.values[inst.Operands[1]]))
			}

		case OpLogicalOr:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = ValBool(toBool(interp.values[inst.Operands[0]]) || toBool(interp.values[inst.Operands[1]]))
			}

		case OpLogicalNot:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = ValBool(!toBool(interp.values[inst.Operands[0]]))
			}

		case OpBitwiseAnd:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a & b })
			}

		case OpBitwiseOr:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a | b })
			}

		case OpBitwiseXor:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a ^ b })
			}

		case OpNot:
			if len(inst.Operands) >= 1 {
				a := toUint32(interp.values[inst.Operands[0]])
				interp.values[inst.ResultID] = ValUint(^a)
			}

		case OpShiftLeftLogical:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a << (b & 31) })
			}

		case OpShiftRightLogical:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinOp(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], func(a, b uint32) uint32 { return a >> (b & 31) })
			}

		case OpShiftRightArithmetic:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := toUint32(interp.values[inst.Operands[1]]) & 31
				interp.values[inst.ResultID] = ValInt(a >> b)
			}

		case OpAtomicIAdd, OpAtomicISub, OpAtomicExchange, OpAtomicCompareExchange,
			OpAtomicSMin, OpAtomicUMin, OpAtomicSMax, OpAtomicUMax,
			OpAtomicIIncrement, OpAtomicIDecrement, OpAtomicLoad:
			interp.values[inst.ResultID] = interp.executeAtomicOp(inst)

		case OpAtomicStore:
			interp.executeAtomicOp(inst)

		case OpControlBarrier, OpMemoryBarrier:
			// In a single-threaded interpreter, barriers are no-ops.
			// All invocations within a workgroup run sequentially,
			// so there is no need for synchronization.

		case OpUndef:
			// OpUndef produces an undefined value -- use zero.
			interp.values[inst.ResultID] = ValUint(0)

		case OpExtInst:
			// OpExtInst: type resultID setID instructionNumber operands...
			// Operands[0] = setID (result of OpExtInstImport)
			// Operands[1] = instruction number
			// Operands[2:] = instruction-specific operands
			if len(inst.Operands) >= 2 {
				setID := inst.Operands[0]
				instNum := inst.Operands[1]
				extOps := inst.Operands[2:]
				setName := interp.module.ExtInstImports[setID]
				if setName == glslExtSetName {
					interp.values[inst.ResultID] = interp.executeGLSLExtInst(instNum, extOps)
				}
			}

		case OpSampledImage:
			// OpSampledImage: type resultID image sampler
			// Combines a texture and sampler into a SampledImage pair.
			if len(inst.Operands) >= 2 {
				imgVal := interp.values[inst.Operands[0]]
				sampVal := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = ValSampledImage(&SampledImageValue{Image: imgVal, Sampler: sampVal})
			}

		case OpImageSampleImplicitLod:
			// OpImageSampleImplicitLod: type resultID sampledImage coordinate [ImageOperands...]
			if len(inst.Operands) >= 2 {
				sampledImg := interp.values[inst.Operands[0]]
				coord := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = interp.sampleTexture(sampledImg, coord, 0)
			}

		case OpImageSampleExplicitLod:
			// OpImageSampleExplicitLod: type resultID sampledImage coordinate ImageOperands lod
			if len(inst.Operands) >= 2 {
				sampledImg := interp.values[inst.Operands[0]]
				coord := interp.values[inst.Operands[1]]
				// For the software backend we only support LOD 0.
				var lod float32
				if len(inst.Operands) >= 4 && (inst.Operands[2]&ImageOperandLodMask) != 0 {
					lod = toFloat32(interp.values[inst.Operands[3]])
				}
				interp.values[inst.ResultID] = interp.sampleTexture(sampledImg, coord, lod)
			}

		case OpImageFetch:
			// OpImageFetch: type resultID image coordinate [ImageOperands...]
			// Direct texel fetch by integer coordinates (no filtering).
			if len(inst.Operands) >= 2 {
				imgVal := interp.values[inst.Operands[0]]
				coord := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = interp.fetchTexel(imgVal, coord)
			}

		case OpImageQuerySize:
			// OpImageQuerySize: type resultID image
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.queryImageSize(interp.values[inst.Operands[0]])
			}

		default:
			// Unknown opcodes are skipped. The triangle shader uses a small subset.
		}

		// --- Debug: post-instruction hooks ---
		if debug != nil { //nolint:nestif // Debug trace and watch variable checks require nested conditionals.
			// Trace output: write one JSON line per result-producing instruction.
			if traceEnc != nil && inst.ResultID != 0 {
				writeTrace(debug.TraceWriter, traceEnc, traceEnt, pc-1, inst, interp.values)
			}

			// Watch variable tracking: check if a watched SSA ID changed value.
			if watchResultID != 0 {
				newValue := interp.values[watchResultID]
				if !valuesEqual(watchOldValue, newValue) {
					// Value changed -- force stepping so OnInstruction fires
					// on the next iteration.
					stepping = true
				}
			}
		}
	}

	return nil
}

// currentBlockLabel returns the label ID of the basic block containing the
// instruction at the given index. This is used for OpPhi predecessor tracking.
func (interp *interpreter) currentBlockLabel(instIdx int) uint32 {
	// Walk backward from instIdx to find the most recent OpLabel.
	for i := instIdx; i >= 0; i-- {
		if interp.fn.Instructions[i].Opcode == OpLabel {
			return interp.fn.Instructions[i].ResultID
		}
	}
	return 0
}

// maxCallDepth is the maximum function call nesting depth.
const maxCallDepth = 64

// callFunction invokes a non-entry-point function by ID with the given arguments.
// Each call creates a fresh value scope (local variables) while sharing the
// module-level constants and context. Uses the interpreter pool to avoid
// allocating a new values slice per call.
func (interp *interpreter) callFunction(funcID uint32, argIDs []uint32) (Value, error) {
	if interp.callDepth >= maxCallDepth {
		return Value{}, fmt.Errorf("spirv: exceeded maximum call depth (%d)", maxCallDepth)
	}

	fn, ok := interp.module.FunctionsByID[funcID]
	if !ok {
		return Value{}, fmt.Errorf("spirv: function %d not found", funcID)
	}

	// Acquire a pooled child interpreter to avoid allocating per call.
	child := interp.module.getInterpreter()
	child.ep = interp.ep
	child.fn = fn
	child.ctx = interp.ctx
	child.callDepth = interp.callDepth + 1
	child.prevBlock = 0
	child.iterationCount = 0
	child.returnValue = Value{}

	// Seed constants into the child value slice.
	for id, val := range interp.module.Constants {
		child.values[id] = val
	}

	// Bind parameters: find OpFunctionParameter instructions and assign argument values.
	paramIdx := 0
	for _, inst := range fn.Instructions {
		if inst.Opcode == OpFunctionParameter {
			if paramIdx < len(argIDs) {
				child.values[inst.ResultID] = interp.values[argIDs[paramIdx]]
			}
			paramIdx++
		}
	}

	// Copy resource variable bindings from parent so the callee can access buffers.
	for varID, vi := range interp.module.Variables {
		if vi.StorageClass == StorageClassUniform || vi.StorageClass == StorageClassStorageBuffer ||
			vi.StorageClass == StorageClassPushConstant || vi.StorageClass == StorageClassUniformConstant {
			v := interp.values[varID]
			if !v.IsNone() {
				child.values[varID] = v
			}
		}
	}

	// Execute the callee.
	if err := child.run(); err != nil {
		interp.module.putInterpreter(child)
		return Value{}, err
	}

	retVal := child.returnValue
	interp.module.putInterpreter(child)
	return retVal, nil
}

// accessChain navigates into a composite value through a chain of indexes,
// returning a Pointer, SubPointer, or BufferPointer to the innermost element.
//
// For BufferPointers (storage buffers), byte-offset navigation is used so writes
// go directly to the raw buffer.
//
// For Pointers (function-local variables), a SubPointer is returned that maintains
// a reference back to the root Pointer. This ensures OpStore writes propagate
// through to the parent composite (e.g. updating p.pos modifies the struct in p).
func (interp *interpreter) accessChain(baseID uint32, indexes []uint32) Value {
	baseVal := interp.values[baseID]

	// If base is a BufferPointer, compute byte offset through the chain.
	if baseVal.Tag == TagBufferPointer {
		return ValBufferPointer(interp.bufferAccessChain(baseVal.AsBufferPointer(), indexes))
	}

	// If base is a SubPointer, extend the index path.
	if baseVal.Tag == TagSubPointer {
		sp := baseVal.AsSubPointer()
		resolvedIndexes := make([]uint32, len(sp.Indexes), len(sp.Indexes)+len(indexes))
		copy(resolvedIndexes, sp.Indexes)
		for _, idxID := range indexes {
			resolvedIndexes = append(resolvedIndexes, toUint32(interp.values[idxID]))
		}
		return ValSubPointer(&SubPointer{Parent: sp.Parent, Indexes: resolvedIndexes})
	}

	// If base is a Pointer to a composite, return a SubPointer for write-through.
	if baseVal.Tag == TagPointer {
		resolvedIndexes := make([]uint32, 0, len(indexes))
		for _, idxID := range indexes {
			resolvedIndexes = append(resolvedIndexes, toUint32(interp.values[idxID]))
		}
		return ValSubPointer(&SubPointer{Parent: baseVal.AsPointer(), Indexes: resolvedIndexes})
	}

	// Fallback: navigate through each index on a raw value.
	current := baseVal
	for _, idxID := range indexes {
		idx := toUint32(interp.values[idxID])
		current = indexComposite(current, idx)
	}

	return ValPointer(interp.allocPointer(current))
}

// bufferAccessChain computes a byte offset through a chain of indexes into
// a raw buffer, returning a BufferPointer at the final element position.
func (interp *interpreter) bufferAccessChain(bp *BufferPointer, indexes []uint32) *BufferPointer {
	m := interp.module
	offset := bp.Offset
	currentType := bp.Type

	for _, idxID := range indexes {
		idx := toUint32(interp.values[idxID])

		if currentType == nil {
			break
		}

		switch currentType.Kind {
		case TypeStruct:
			// For struct members, use the MemberDecorate Offset if available.
			structTypeID := interp.findTypeID(currentType)
			memberOffset := m.GetMemberOffset(structTypeID, idx)
			offset += memberOffset
			if int(idx) < len(currentType.MemberIDs) {
				currentType = m.Types[currentType.MemberIDs[idx]]
			} else {
				currentType = nil
			}

		case TypeArray:
			elemType := m.Types[currentType.ElemType]
			if elemType != nil {
				elemSize := typeByteSize(m, elemType)
				offset += idx * elemSize
			}
			currentType = m.Types[currentType.ElemType]

		case TypeVector:
			// Indexing into a vector component.
			elemType := m.Types[currentType.ElemType]
			if elemType != nil {
				elemSize := typeByteSize(m, elemType)
				offset += idx * elemSize
				currentType = elemType
			} else {
				offset += idx * 4
				currentType = &TypeInfo{Kind: TypeFloat, Width: 32}
			}

		default:
			currentType = nil
		}
	}

	return &BufferPointer{Buffer: bp.Buffer, Offset: offset, Type: currentType}
}

// subPointerLoad reads the current value of a sub-element through the parent
// Pointer by navigating the index path.
func subPointerLoad(sp *SubPointer) Value {
	current := sp.Parent.Val
	for _, idx := range sp.Indexes {
		current = indexComposite(current, idx)
	}
	return current
}

// subPointerStore writes a value to a sub-element of the parent Pointer's
// composite, rebuilding the composite at each level so the parent reflects the change.
func subPointerStore(sp *SubPointer, val Value) {
	if len(sp.Indexes) == 0 {
		sp.Parent.Val = val
		return
	}
	sp.Parent.Val = setCompositeElement(sp.Parent.Val, sp.Indexes, val)
}

// compositeConstruct builds a composite value from constituent IDs.
func (interp *interpreter) compositeConstruct(typeID uint32, constituentIDs []uint32) Value {
	ti, ok := interp.module.Types[typeID]
	if !ok {
		return ValVec4(0, 0, 0, 0)
	}

	switch ti.Kind {
	case TypeVector:
		// Fast path: if constituent count matches component count and all are
		// scalars, build the vector directly without allocating a temporary slice.
		if uint32(len(constituentIDs)) == ti.Components {
			allScalar := true
			for _, id := range constituentIDs {
				tag := interp.values[id].Tag
				if tag != TagFloat32 && tag != TagUint32 && tag != TagInt32 {
					allScalar = false
					break
				}
			}
			if allScalar {
				switch ti.Components {
				case 2:
					return ValVec2(
						toFloat32(interp.values[constituentIDs[0]]),
						toFloat32(interp.values[constituentIDs[1]]),
					)
				case 3:
					return ValVec3(
						toFloat32(interp.values[constituentIDs[0]]),
						toFloat32(interp.values[constituentIDs[1]]),
						toFloat32(interp.values[constituentIDs[2]]),
					)
				case 4:
					return ValVec4(
						toFloat32(interp.values[constituentIDs[0]]),
						toFloat32(interp.values[constituentIDs[1]]),
						toFloat32(interp.values[constituentIDs[2]]),
						toFloat32(interp.values[constituentIDs[3]]),
					)
				}
			}
		}

		// General path: flatten constituents (may include sub-vectors) using
		// a stack-allocated array to avoid heap allocation for small vectors.
		var buf [4]float32
		n := 0
		for _, id := range constituentIDs {
			val := interp.values[id]
			switch val.Tag {
			case TagFloat32:
				if n < len(buf) {
					buf[n] = val.F[0]
					n++
				}
			case TagUint32:
				if n < len(buf) {
					buf[n] = float32(val.U[0])
					n++
				}
			case TagInt32:
				if n < len(buf) {
					buf[n] = float32(int32(val.U[0]))
					n++
				}
			case TagVec2:
				for j := 0; j < 2 && n < len(buf); j++ {
					buf[n] = val.F[j]
					n++
				}
			case TagVec3:
				for j := 0; j < 3 && n < len(buf); j++ {
					buf[n] = val.F[j]
					n++
				}
			case TagVec4:
				for j := 0; j < 4 && n < len(buf); j++ {
					buf[n] = val.F[j]
					n++
				}
			default:
				if n < len(buf) {
					buf[n] = 0
					n++
				}
			}
		}
		switch ti.Components {
		case 2:
			return ValVec2(buf[0], buf[1])
		case 3:
			return ValVec3(buf[0], buf[1], buf[2])
		case 4:
			return ValVec4(buf[0], buf[1], buf[2], buf[3])
		}

	case TypeArray:
		arr := make([]Value, len(constituentIDs))
		for i, id := range constituentIDs {
			arr[i] = interp.values[id]
		}
		return ValArray(arr)

	case TypeStruct:
		arr := make([]Value, len(constituentIDs))
		for i, id := range constituentIDs {
			arr[i] = interp.values[id]
		}
		return ValArray(arr)
	}

	return ValUint(0)
}

// compositeExtract navigates literal indexes to extract a scalar from a composite.
func (interp *interpreter) compositeExtract(composite Value, indexes []uint32) Value {
	current := composite
	for _, idx := range indexes {
		current = indexComposite(current, idx)
	}
	return current
}

// =============================================================================
// Matrix Operations
// =============================================================================

// matrixTimesVector multiplies a matrix (Array of column vectors) by a vector.
// SPIR-V stores matrices in column-major order as arrays of column vectors.
func matrixTimesVector(mat, vec Value) Value {
	if mat.Tag != TagArray {
		return vec
	}
	cols := mat.AsArray()
	if vec.Tag != TagVec4 {
		return vec
	}
	v := vec.F
	numCols := len(cols)
	if numCols < 2 {
		return vec
	}

	// Extract columns as Vec4 (padding with zeros if needed).
	colVecs := make([][4]float32, numCols)
	for i, c := range cols {
		colVecs[i] = Vec4ToFloat32(c)
	}

	// result = col[0]*v[0] + col[1]*v[1] + ...
	var result [4]float32
	for i := 0; i < numCols && i < 4; i++ {
		for j := 0; j < 4; j++ {
			result[j] += colVecs[i][j] * v[i]
		}
	}
	return ValVec4(result[0], result[1], result[2], result[3])
}

// matrixTimesScalar multiplies every element of a matrix by a scalar.
func matrixTimesScalar(mat, scalar Value) Value {
	if mat.Tag != TagArray {
		return mat
	}
	cols := mat.AsArray()
	s := toFloat32(scalar)
	result := make([]Value, len(cols))
	for i, c := range cols {
		result[i] = vectorTimesScalar(c, s)
	}
	return ValArray(result)
}

// matrixTimesMatrix multiplies two matrices (both stored as Array of column vectors).
// C = A * B means C[j] = A * B[j] for each column j of B.
func matrixTimesMatrix(left, right Value) Value {
	if right.Tag != TagArray {
		return right
	}
	rightCols := right.AsArray()
	result := make([]Value, len(rightCols))
	for j, col := range rightCols {
		result[j] = matrixTimesVector(left, col)
	}
	return ValArray(result)
}

// transposeMatrix transposes a matrix stored as Array of column vectors.
func transposeMatrix(mat Value) Value {
	if mat.Tag != TagArray {
		return mat
	}
	cols := mat.AsArray()
	numCols := len(cols)
	if numCols == 0 {
		return mat
	}

	// Determine the number of rows from the first column.
	var numRows int
	switch cols[0].Tag {
	case TagVec2:
		numRows = 2
	case TagVec3:
		numRows = 3
	case TagVec4:
		numRows = 4
	default:
		return mat
	}

	// Build transposed columns (each row of the original becomes a column).
	result := make([]Value, numRows)
	for r := 0; r < numRows; r++ {
		var row [4]float32
		for c := 0; c < numCols && c < 4; c++ {
			row[c] = toFloat32(indexComposite(cols[c], uint32(r)))
		}
		// Return the correct vector type based on column count.
		switch numCols {
		case 2:
			result[r] = ValVec2(row[0], row[1])
		case 3:
			result[r] = ValVec3(row[0], row[1], row[2])
		default:
			result[r] = ValVec4(row[0], row[1], row[2], row[3])
		}
	}
	return ValArray(result)
}

// vectorShuffle creates a new vector by selecting components from two input vectors.
func vectorShuffle(m *Module, typeID uint32, vec1, vec2 Value, components []uint32) Value {
	// Flatten both vectors into a combined component array.
	var pool []float32
	pool = appendComponents(pool, vec1)
	pool = appendComponents(pool, vec2)
	total := uint32(len(pool))

	var out []float32
	for _, idx := range components {
		switch {
		case idx == 0xFFFFFFFF || idx >= total:
			out = append(out, 0) // Undefined component.
		default:
			out = append(out, pool[idx])
		}
	}

	// Determine result type from the module type info.
	ti := m.Types[typeID]
	if ti != nil && ti.Kind == TypeVector {
		switch ti.Components {
		case 2:
			var v [2]float32
			for i := 0; i < 2 && i < len(out); i++ {
				v[i] = out[i]
			}
			return ValVec2(v[0], v[1])
		case 3:
			var v [3]float32
			for i := 0; i < 3 && i < len(out); i++ {
				v[i] = out[i]
			}
			return ValVec3(v[0], v[1], v[2])
		case 4:
			var v [4]float32
			for i := 0; i < 4 && i < len(out); i++ {
				v[i] = out[i]
			}
			return ValVec4(v[0], v[1], v[2], v[3])
		}
	}

	// Fallback: infer from component count.
	switch len(out) {
	case 2:
		return ValVec2(out[0], out[1])
	case 3:
		return ValVec3(out[0], out[1], out[2])
	case 4:
		return ValVec4(out[0], out[1], out[2], out[3])
	default:
		return ValFloat(0)
	}
}

// =============================================================================
// Texture Sampling
// =============================================================================

// resolveTexture looks up a Texture2D from a value that may be a BindingKey
// or already stored via another mechanism.
func (interp *interpreter) resolveTexture(val Value) *Texture2D {
	if val.Tag == TagBindingKey {
		bk := val.AsBindingKey()
		if interp.ctx != nil && interp.ctx.Textures != nil {
			return interp.ctx.Textures[bk]
		}
	}
	return nil
}

// resolveSampler looks up a Sampler from a value that may be a BindingKey.
func (interp *interpreter) resolveSampler(val Value) *Sampler {
	if val.Tag == TagBindingKey {
		bk := val.AsBindingKey()
		if interp.ctx != nil && interp.ctx.Samplers != nil {
			return interp.ctx.Samplers[bk]
		}
	}
	return nil
}

// sampleTexture samples a texture using the given sampled image and UV coordinates.
func (interp *interpreter) sampleTexture(sampledImg Value, coord Value, lod float32) Value {
	_ = lod // LOD levels not implemented; always sample base level.

	if sampledImg.Tag != TagSampledImage {
		return ValVec4(1, 0, 1, 1) // Magenta for error.
	}
	si := sampledImg.AsSampledImage()

	tex := interp.resolveTexture(si.Image)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return ValVec4(1, 0, 1, 1) // Magenta for missing texture.
	}

	samp := interp.resolveSampler(si.Sampler)
	if samp == nil {
		samp = &Sampler{} // Default: nearest, repeat.
	}

	// Extract UV coordinates.
	var u, v float32
	switch coord.Tag {
	case TagVec2:
		u, v = coord.F[0], coord.F[1]
	case TagVec3:
		u, v = coord.F[0], coord.F[1]
	case TagVec4:
		u, v = coord.F[0], coord.F[1]
	default:
		u = toFloat32(coord)
	}

	// Apply wrap mode.
	u = applyWrapMode(u, samp.WrapU)
	v = applyWrapMode(v, samp.WrapV)

	// Determine filter from the magnification filter.
	filter := samp.MagFilter
	if filter == FilterLinear {
		return ValVec4From(sampleBilinear(tex, u, v))
	}
	return ValVec4From(sampleNearest(tex, u, v))
}

// fetchTexel fetches a single texel by integer coordinates (no filtering).
func (interp *interpreter) fetchTexel(imgVal Value, coord Value) Value {
	tex := interp.resolveTexture(imgVal)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return ValVec4(0, 0, 0, 0)
	}

	var x, y int
	switch coord.Tag {
	case TagVec2:
		x, y = int(coord.F[0]), int(coord.F[1])
	case TagVec3:
		x, y = int(coord.F[0]), int(coord.F[1])
	case TagVec4:
		x, y = int(coord.F[0]), int(coord.F[1])
	default:
		x = int(toUint32(coord))
	}

	// Clamp to texture bounds.
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= int(tex.Width) {
		x = int(tex.Width) - 1
	}
	if y >= int(tex.Height) {
		y = int(tex.Height) - 1
	}

	return ValVec4From(readTexel(tex, x, y))
}

// queryImageSize returns the size of a texture as a vec2 of uint32 values.
func (interp *interpreter) queryImageSize(imgVal Value) Value {
	tex := interp.resolveTexture(imgVal)
	if tex == nil {
		return ValVec2(0, 0)
	}
	return ValVec2(float32(tex.Width), float32(tex.Height))
}

// applyWrapMode wraps a texture coordinate according to the specified mode.
func applyWrapMode(coord float32, mode uint32) float32 {
	switch mode {
	case WrapClampToEdge:
		if coord < 0 {
			return 0
		}
		if coord > 1 {
			return 1
		}
		return coord

	case WrapMirroredRepeat:
		coord = float32(math.Abs(float64(coord)))
		period := int(coord)
		frac := coord - float32(period)
		if period%2 != 0 {
			return 1 - frac
		}
		return frac

	default: // WrapRepeat
		coord -= float32(int(coord))
		if coord < 0 {
			coord++
		}
		return coord
	}
}

// sampleNearest samples a texture at the given UV using nearest-neighbor filtering.
func sampleNearest(tex *Texture2D, u, v float32) Vec4 {
	x := int(u * float32(tex.Width))
	y := int(v * float32(tex.Height))

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= int(tex.Width) {
		x = int(tex.Width) - 1
	}
	if y >= int(tex.Height) {
		y = int(tex.Height) - 1
	}

	return readTexel(tex, x, y)
}

// sampleBilinear samples a texture at the given UV using bilinear (4-tap) filtering.
func sampleBilinear(tex *Texture2D, u, v float32) Vec4 {
	fx := u*float32(tex.Width) - 0.5
	fy := v*float32(tex.Height) - 0.5

	x0 := int(math.Floor(float64(fx)))
	y0 := int(math.Floor(float64(fy)))
	x1 := x0 + 1
	y1 := y0 + 1

	fracX := fx - float32(x0)
	fracY := fy - float32(y0)

	w := int(tex.Width)
	h := int(tex.Height)
	x0 = clampInt(x0, w-1)
	y0 = clampInt(y0, h-1)
	x1 = clampInt(x1, w-1)
	y1 = clampInt(y1, h-1)

	c00 := readTexel(tex, x0, y0)
	c10 := readTexel(tex, x1, y0)
	c01 := readTexel(tex, x0, y1)
	c11 := readTexel(tex, x1, y1)

	var result Vec4
	for i := 0; i < 4; i++ {
		top := c00[i]*(1-fracX) + c10[i]*fracX
		bot := c01[i]*(1-fracX) + c11[i]*fracX
		result[i] = top*(1-fracY) + bot*fracY
	}
	return result
}

// readTexel reads a single RGBA texel from the texture at pixel coordinates (x, y).
func readTexel(tex *Texture2D, x, y int) Vec4 {
	idx := (y*int(tex.Width) + x) * 4
	if idx+3 >= len(tex.Data) {
		return Vec4{0, 0, 0, 0}
	}
	return Vec4{
		float32(tex.Data[idx+0]) / 255.0,
		float32(tex.Data[idx+1]) / 255.0,
		float32(tex.Data[idx+2]) / 255.0,
		float32(tex.Data[idx+3]) / 255.0,
	}
}

// clampInt clamps an integer to [0, hi].
func clampInt(v, hi int) int {
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}
