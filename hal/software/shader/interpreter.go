//go:build !(js && wasm)

// SPIR-V interpreter for the software backend.
//
// Executes a single SPIR-V entry point with provided inputs and returns outputs.
// Supported features:
//   - OpLoad / OpStore for variable access
//   - OpAccessChain for array/composite indexing
//   - OpCompositeConstruct / OpCompositeExtract for building/decomposing vectors
//   - OpVariable for local storage
//   - Control flow: OpBranch, OpBranchConditional, OpSelectionMerge, OpLoopMerge,
//     OpPhi, OpSwitch, OpSelect (loops, conditionals, switch fully supported)
//   - OpFunctionCall for function invocation
//   - OpConvertUToF for uint-to-float conversion
//   - Basic arithmetic: OpFAdd, OpFSub, OpFMul, OpFDiv, OpFNegate
//   - Basic integer arithmetic: OpIAdd, OpISub, OpIMul
//   - Comparison: OpFOrd*, OpIEqual, OpINotEqual
//   - GLSL.std.450 extended instruction set (math intrinsics)

package shader

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// ErrDiscarded is returned by Execute, ExecuteWithContext and ExecuteWithDebug
// when a fragment invocation executed OpKill or OpTerminateInvocation (WGSL
// discard). The caller must write no color, depth or stencil for the fragment.
var ErrDiscarded = errors.New("spirv: fragment invocation discarded")

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

	interp.seedConstants()

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

// fRem is SPIR-V OpFRem (WGSL f32 %): a - b*trunc(a/b), with the sign of a.
// math.Remainder rounds the quotient to nearest instead, which gives
// -7.5 % 2 = 0.5 rather than -1.5.
func fRem(a, b float32) float32 {
	return float32(math.Mod(float64(a), float64(b)))
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

	// Workgroup barrier suspension (P1-SW-025). With syncBarriers set, run
	// returns at a workgroup-scope OpControlBarrier with suspended set and
	// resumePC at the instruction after it; the next run continues there.
	// A barrier reached inside a called function suspends the whole call
	// chain: each caller keeps its suspended callee in pendingCall and its
	// own resumePC after the OpFunctionCall, whose result ID is pendingResult.
	syncBarriers  bool
	suspended     bool
	resumePC      int
	pendingCall   *interpreter
	pendingResult uint32

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
	interp.syncBarriers = false
	interp.suspended = false
	interp.resumePC = 0
	interp.pendingCall = nil
	interp.pendingResult = 0
	m.interpPool.Put(interp)
}

// release returns interp and the chain of callees it is suspended in to the
// pool. An invocation abandoned at a barrier (another one failed) owns them.
func (m *Module) release(interp *interpreter) {
	for interp != nil {
		next := interp.pendingCall
		m.putInterpreter(interp)
		interp = next
	}
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
	interp.initPrivateVariables()
}

// initPrivateVariables sets up every Private global from its initializer or
// zero (WGSL zero-initializes a var<private> without one), so each
// invocation starts from that value. An entry point's interface lists
// Private variables only from SPIR-V 1.4, so all of them are set up.
func (interp *interpreter) initPrivateVariables() {
	m := interp.module
	for _, varID := range m.prepared().private {
		vi := m.Variables[varID]
		interp.values[varID] = ValPointer(interp.allocPointer(interp.initialValue(vi.TypeID, vi.Initializer)))
	}
}

// seedConstants puts the module's constants into the value slice.
func (interp *interpreter) seedConstants() {
	prep := interp.module.prepared()
	for i, id := range prep.constIDs {
		interp.values[id] = prep.constVals[i]
	}
}

// initialValue returns the starting value of a variable of pointer type
// ptrTypeID: a copy of its initializer constant, or zero without one.
func (interp *interpreter) initialValue(ptrTypeID, initializer uint32) Value {
	if initializer != 0 {
		if c, ok := interp.module.Constants[initializer]; ok {
			return cloneValue(c)
		}
	}
	return zeroValueForVar(interp.module, ptrTypeID)
}

// cloneValue copies v deeply enough that stores through a pointer to the
// copy never reach the original (array elements are copied).
func cloneValue(v Value) Value {
	if v.Tag != TagArray {
		return v
	}
	src := v.AsArray()
	dst := make([]Value, len(src))
	for i, e := range src {
		dst[i] = cloneValue(e)
	}
	return ValArray(dst)
}

// initResourceVariables sets up Uniform/StorageBuffer/PushConstant and
// UniformConstant variables from the context's bindings. Their values are
// resolved once per context and module (resourceValues); each invocation
// only points its variables at them.
func (interp *interpreter) initResourceVariables() {
	if interp.ctx == nil {
		return
	}
	prep := interp.module.prepared()
	vals := interp.resourceValues(prep)
	for i, varID := range prep.resources {
		switch v := vals[i]; v.Tag {
		case TagNone:
			// No binding: the variable keeps its zero value.
		case TagBufferPointer:
			interp.values[varID] = v
		default:
			interp.values[varID] = ValPointer(interp.allocPointer(v))
		}
	}
}

// readValueFromBuffer deserializes a SPIR-V typed value from raw buffer bytes.
// offset is the starting byte offset into data. matStride is the column
// stride of the matrices in ti (BufferPointer.MatrixStride), 0 if unknown.
func (interp *interpreter) readValueFromBuffer(data []byte, offset uint32, ti *TypeInfo, matStride uint32) Value {
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
		if k := scalarKind(elemType); k != kindNone && ti.Components >= 2 && ti.Components <= 4 {
			// A 32-bit vector loads its words as they are: exact for every
			// integer and float bit pattern; a bool lane is any nonzero word.
			var w [4]uint32
			for i := range ti.Components {
				o := offset + i*elemSize
				if o+4 > uint32(len(data)) {
					continue
				}
				w[i] = binary.LittleEndian.Uint32(data[o:])
				if k == kindBool {
					w[i] = b2u(w[i] != 0)
				}
			}
			return wordsValue(shapeOf(k, int(ti.Components)), w)
		}
		switch ti.Components {
		case 2:
			var v [2]float32
			for i := uint32(0); i < 2; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType, 0)
				v[i] = toFloat32(f)
			}
			return ValVec2(v[0], v[1])
		case 3:
			var v [3]float32
			for i := uint32(0); i < 3; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType, 0)
				v[i] = toFloat32(f)
			}
			return ValVec3(v[0], v[1], v[2])
		case 4:
			var v [4]float32
			for i := uint32(0); i < 4; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType, 0)
				v[i] = toFloat32(f)
			}
			return ValVec4(v[0], v[1], v[2], v[3])
		}
		return ValUint(0)

	case TypeMatrix:
		// Matrix is stored as an array of column vectors, matStride apart.
		colType := m.Types[ti.ElemType]
		if colType == nil || ti.Components == 0 {
			return ValArray(nil)
		}
		colStride := matrixColumnStride(m, ti, matStride)
		cols := make([]Value, ti.Components)
		for i := uint32(0); i < ti.Components; i++ {
			cols[i] = interp.readValueFromBuffer(data, offset+i*colStride, colType, 0)
		}
		return ValArray(cols)

	case TypeArray:
		elemType := m.Types[ti.ElemType]
		if elemType == nil || ti.Length == 0 {
			return ValArray(nil)
		}
		stride := arrayStride(m, ti)
		arr := make([]Value, ti.Length)
		for i := uint32(0); i < ti.Length; i++ {
			arr[i] = interp.readValueFromBuffer(data, offset+i*stride, elemType, matStride)
		}
		return ValArray(arr)

	case TypeStruct:
		members := make([]Value, len(ti.MemberIDs))
		// For each struct member, look up its type and use MemberDecorate Offset.
		structTypeID := m.typeID(ti)
		for i, memberTypeID := range ti.MemberIDs {
			memberType := m.Types[memberTypeID]
			if memberType == nil {
				members[i] = ValUint(0)
				continue
			}
			memberOffset := m.GetMemberOffset(structTypeID, uint32(i))
			members[i] = interp.readValueFromBuffer(data, offset+memberOffset, memberType,
				m.memberMatrixStride(structTypeID, uint32(i)))
		}
		return ValArray(members)

	default:
		return ValUint(0)
	}
}

// writeValueToBufferTyped serializes val as a value of SPIR-V type ti at
// offset, the inverse of readValueFromBuffer: vector components packed,
// matrix columns matStride apart, array elements ArrayStride apart, struct
// members at their Offset. A store never touches the padding between them.
// Without a type, or with a value that does not match it, it falls back to
// the untyped writeValueToBuffer.
func (interp *interpreter) writeValueToBufferTyped(data []byte, offset uint32, val Value, ti *TypeInfo, matStride uint32) {
	m := interp.module
	if ti == nil {
		writeValueToBuffer(data, offset, val)
		return
	}
	switch ti.Kind {
	case TypeVector:
		elemType := m.Types[ti.ElemType]
		if elemType == nil {
			writeValueToBuffer(data, offset, val)
			return
		}
		elemSize := typeByteSize(m, elemType)
		for i := uint32(0); i < ti.Components; i++ {
			interp.writeValueToBufferTyped(data, offset+i*elemSize, indexComposite(val, i), elemType, 0)
		}

	case TypeMatrix, TypeArray, TypeStruct:
		if val.Tag != TagArray {
			writeValueToBuffer(data, offset, val)
			return
		}
		interp.writeCompositeToBuffer(data, offset, val.AsArray(), ti, matStride)

	default:
		writeScalarToBuffer(data, offset, val, ti)
	}
}

// writeCompositeToBuffer writes the elements of a matrix, array or struct
// value of type ti (see writeValueToBufferTyped).
func (interp *interpreter) writeCompositeToBuffer(data []byte, offset uint32, elems []Value, ti *TypeInfo, matStride uint32) {
	m := interp.module
	switch ti.Kind {
	case TypeMatrix:
		colType := m.Types[ti.ElemType]
		stride := matrixColumnStride(m, ti, matStride)
		for i := uint32(0); i < ti.Components && int(i) < len(elems); i++ {
			interp.writeValueToBufferTyped(data, offset+i*stride, elems[i], colType, 0)
		}

	case TypeArray:
		elemType := m.Types[ti.ElemType]
		stride := arrayStride(m, ti)
		for i, elem := range elems {
			if ti.Length != 0 && uint32(i) >= ti.Length {
				break
			}
			interp.writeValueToBufferTyped(data, offset+uint32(i)*stride, elem, elemType, matStride)
		}

	case TypeStruct:
		structTypeID := m.typeID(ti)
		for i, memberTypeID := range ti.MemberIDs {
			if i >= len(elems) {
				break
			}
			member := uint32(i)
			interp.writeValueToBufferTyped(data, offset+m.GetMemberOffset(structTypeID, member), elems[i],
				m.Types[memberTypeID], m.memberMatrixStride(structTypeID, member))
		}
	}
}

// arrayStride returns the byte distance between the elements of the array
// type ti: its ArrayStride decoration, which SPIR-V requires on every array
// in Uniform, StorageBuffer and PushConstant memory and which can exceed the
// element size (array<vec3<f32>>: 16, not 12). An undecorated array (one in
// Workgroup memory) is packed at the element size.
func arrayStride(m *Module, ti *TypeInfo) uint32 {
	if ti.ArrayStride != 0 {
		return ti.ArrayStride
	}
	elemType := m.Types[ti.ElemType]
	if elemType == nil {
		return 0
	}
	return typeByteSize(m, elemType)
}

// matrixColumnStride returns the byte distance between the columns of the
// matrix type ti: matStride, the MatrixStride decoration of the struct
// member holding it, when known (mat3x3<f32>: 16, not 12); otherwise the
// column size.
func matrixColumnStride(m *Module, ti *TypeInfo, matStride uint32) uint32 {
	if matStride != 0 {
		return matStride
	}
	colType := m.Types[ti.ElemType]
	if colType == nil {
		return 0
	}
	return typeByteSize(m, colType)
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
	case TypeMatrix:
		// Matrix = columns * columnByteSize. Each column is a vector.
		colType := m.Types[ti.ElemType]
		if colType == nil {
			return 0
		}
		return typeByteSize(m, colType) * ti.Components
	case TypeArray:
		return arrayStride(m, ti) * ti.Length
	case TypeStruct:
		// Compute struct size from the highest (offset + size) across all members.
		// Look up the struct type ID for MemberDecorate offset decorations.
		structTypeID := m.typeID(ti)

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
// With syncBarriers set it may stop early at a workgroup barrier (suspended
// set, see interpreter); calling run again resumes the invocation there.
func (interp *interpreter) run() error {
	instructions := interp.fn.Instructions
	pc := interp.resumePC
	interp.resumePC = 0
	interp.suspended = false
	if interp.pendingCall != nil {
		if err := interp.resumeCall(); err != nil || interp.suspended {
			interp.resumePC = pc
			return err
		}
	}
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
		if debug != nil {
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
		if countInstr {
			countExecuted()
		}

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
				var initializer uint32
				if len(inst.Operands) > 1 {
					initializer = inst.Operands[1]
				}
				val := interp.initialValue(inst.TypeID, initializer)
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
					interp.values[inst.ResultID] = interp.readValueFromBuffer(bp.Buffer, bp.Offset, bp.Type, bp.MatrixStride)
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
				// Write directly to the raw buffer, laid out by the pointee type.
				bp := pv.AsBufferPointer()
				interp.writeValueToBufferTyped(bp.Buffer, bp.Offset, interp.values[valID], bp.Type, bp.MatrixStride)
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
			interp.values[inst.ResultID] = interp.compositeConstruct(inst.resShape, inst.TypeID, inst.Operands)

		case OpCompositeExtract:
			// OpCompositeExtract: type resultID composite indexes...
			if len(inst.Operands) < 2 {
				break
			}
			compositeID := inst.Operands[0]
			indexes := inst.Operands[1:]
			interp.values[inst.ResultID] = interp.compositeExtract(interp.values[compositeID], indexes)

		case OpCompositeInsert:
			// OpCompositeInsert: type resultID object composite indexes...
			// The result is a copy of composite with object at indexes.
			if len(inst.Operands) >= 3 {
				obj := interp.values[inst.Operands[0]]
				interp.values[inst.ResultID] = setCompositeElement(interp.values[inst.Operands[1]], inst.Operands[2:], obj)
			}

		case OpVectorExtractDynamic:
			// OpVectorExtractDynamic: type resultID vector index. The index is
			// a run-time value (WGSL v[i] on a vector value); naga clamps it, and
			// an index past the end reads zero (SPIR-V leaves it undefined).
			if len(inst.Operands) >= 2 {
				idx := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = indexComposite(interp.values[inst.Operands[0]], idx)
			}

		case OpVectorInsertDynamic:
			// OpVectorInsertDynamic: type resultID vector component index. An
			// index past the end leaves the vector unchanged.
			if len(inst.Operands) >= 3 {
				idx := [1]uint32{toUint32(interp.values[inst.Operands[2]])}
				comp := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = setCompositeElement(interp.values[inst.Operands[0]], idx[:], comp)
			}

		case OpConvertUToF:
			// OpConvertUToF: type resultID value
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertLanes(interp.values[inst.Operands[0]], inst.resShape, convertToFloat)

		case OpConvertSToF:
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertLanes(interp.values[inst.Operands[0]], inst.resShape, convertSignedToFloat)

		case OpConvertFToU:
			if len(inst.Operands) < 1 {
				break
			}
			interp.values[inst.ResultID] = convertLanes(interp.values[inst.Operands[0]], inst.resShape, convertFloatToUint)

		case OpBitcast:
			if len(inst.Operands) < 1 {
				break
			}
			src := inst.Operands[0]
			var srcType uint32
			if interp.module != nil {
				srcType = interp.module.ResultTypes[src]
			}
			interp.values[inst.ResultID] = interp.bitcast(interp.values[src], srcType, inst.resShape, inst.TypeID)

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

		case OpIAdd, OpISub, OpIMul, OpBitwiseAnd, OpBitwiseOr, OpBitwiseXor,
			OpShiftLeftLogical, OpShiftRightLogical, OpShiftRightArithmetic:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intBinary(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
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

		case OpFunctionParameter:
			// Bound by the caller (callFunction); no-op during execution.

		case OpSelectionMerge:
			// Selection merge hint -- no-op for the interpreter.

		case OpLoopMerge:
			// Loop merge hint -- no-op. Operands: merge block, continue target, loop control.
			// The actual loop is driven by OpBranchConditional.

		case OpBranchConditional:
			if len(inst.Operands) >= 3 {
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
				interp.values[inst.ResultID] = selectLanes(interp.values[inst.Operands[0]],
					interp.values[inst.Operands[1]], interp.values[inst.Operands[2]])
			}

		case OpIEqual, OpINotEqual, OpULessThan, OpUGreaterThan, OpULessThanEqual, OpUGreaterThanEqual,
			OpSLessThan, OpSGreaterThan, OpSLessThanEqual, OpSGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intCompare(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpFOrdEqual, OpFOrdLessThan, OpFOrdGreaterThan, OpFOrdLessThanEqual, OpFOrdGreaterThanEqual,
			OpFOrdNotEqual, OpFUnordNotEqual:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = floatCompare(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
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
				result, callee, err := interp.callFunction(funcID, args)
				if err != nil {
					return err
				}
				if callee != nil {
					// The callee stopped at a barrier: suspend with it and
					// take its result when it returns (resumeCall).
					interp.pendingCall = callee
					interp.pendingResult = inst.ResultID
					interp.resumePC = pc
					interp.suspended = true
					return nil
				}
				interp.values[inst.ResultID] = result
			}

		case OpKill, OpTerminateInvocation:
			// Fragment discard ends the whole invocation, not only the current
			// function, and the fragment must write nothing (P1-SW-016).
			return ErrDiscarded

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

		case OpVectorTimesMatrix:
			// OpVectorTimesMatrix: type resultID vector matrix
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = vectorTimesMatrix(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpIsNan, OpIsInf:
			// OpIsNan / OpIsInf: type resultID x
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = floatClass(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]])
			}

		case OpQuantizeToF16:
			// OpQuantizeToF16: type resultID value
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = floatUnaryOp(interp.values[inst.Operands[0]], quantizeToF16)
			}

		case OpSDotKHR, OpUDotKHR:
			// OpSDotKHR / OpUDotKHR: type resultID vector1 vector2
			// [PackedVectorFormat]. naga emits them only for dot4I8Packed and
			// dot4U8Packed: two u32 operands and the 4x8-bit packed format.
			// The vector forms (no format operand) take integer vectors.
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = interp.integerDot(inst)
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
				interp.values[inst.ResultID] = vectorShuffle(interp.module, inst.resShape, inst.TypeID, vec1, vec2, components)
			}

		case OpCopyObject:
			// OpCopyObject: type resultID operand
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.values[inst.Operands[0]]
			}

		case OpSDiv, OpUDiv, OpSMod, OpUMod, OpSRem:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = intDivRem(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
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
					interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], fRem)
			}

		case OpSNegate, OpNot:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = intUnary(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]])
			}

		case OpConvertFToS:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = convertLanes(interp.values[inst.Operands[0]], inst.resShape, convertFloatToSigned)
			}

		case OpSConvert, OpUConvert, OpFConvert:
			// Type width conversions -- treat as copy for 32-bit only interpreter.
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.values[inst.Operands[0]]
			}

		case OpLogicalAnd, OpLogicalOr, OpLogicalEqual, OpLogicalNotEqual:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = logicalOp(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]])
			}

		case OpLogicalNot:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = logicalOp(inst.Opcode, inst.resShape, interp.values[inst.Operands[0]], Value{})
			}

		case OpAtomicIAdd, OpAtomicISub, OpAtomicExchange, OpAtomicCompareExchange,
			OpAtomicSMin, OpAtomicUMin, OpAtomicSMax, OpAtomicUMax,
			OpAtomicAnd, OpAtomicOr, OpAtomicXor,
			OpAtomicIIncrement, OpAtomicIDecrement, OpAtomicLoad:
			interp.values[inst.ResultID] = interp.executeAtomicOp(inst)

		case OpAtomicStore:
			interp.executeAtomicOp(inst)

		case OpControlBarrier:
			// OpControlBarrier: execution scope, memory scope, semantics.
			// A workgroup-wide barrier stops the invocation here until every
			// invocation of the workgroup has reached it (DispatchCompute
			// runs them round-robin from barrier to barrier). Memory needs
			// no fence: the invocations run one at a time, so every write
			// before the barrier is visible after it.
			if interp.syncBarriers && len(inst.Operands) >= 1 && syncsWorkgroup(toUint32(interp.values[inst.Operands[0]])) {
				interp.resumePC = pc
				interp.suspended = true
				return nil
			}

		case OpMemoryBarrier:
			// The invocations run one at a time and switch only at control
			// barriers, so memory is always coherent: nothing to do.

		case OpUndef:
			// An undefined value may be anything; it is the zero of its type,
			// so it has the type's shape.
			interp.values[inst.ResultID] = zeroValue(interp.module.Types, inst.TypeID)

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
					interp.values[inst.ResultID] = interp.glslExtInst(instNum, extOps, inst.resShape)
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
				// The implicit level of detail comes from the screen-space
				// derivatives of the coordinate across the 2x2 quad a
				// fragment runs in on a GPU. The interpreter runs fragments
				// one at a time and has no quad (P2-SW-020, see
				// zeroDerivative), so it takes λ_base = 0: the view's base
				// level, plus the Bias operand. Inventing derivatives would
				// pick levels no GPU picks.
				ops := interp.parseImageOperands(inst.Operands[2:])
				ops.lod = 0
				interp.values[inst.ResultID] = interp.sampleTexture(sampledImg, coord, ops)
			}

		case OpImageSampleExplicitLod:
			// OpImageSampleExplicitLod: type resultID sampledImage coordinate ImageOperands (Lod | Grad) ...
			if len(inst.Operands) >= 2 {
				sampledImg := interp.values[inst.Operands[0]]
				coord := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = interp.sampleTexture(sampledImg, coord, interp.parseImageOperands(inst.Operands[2:]))
			}

		case OpImageSampleDrefImplicitLod, OpImageSampleDrefExplicitLod:
			// OpImageSampleDref*Lod: type resultID sampledImage coordinate Dref [ImageOperands...]
			// The implicit form takes λ_base = 0 plus the Bias operand, as
			// OpImageSampleImplicitLod does.
			if len(inst.Operands) >= 3 {
				ops := interp.parseImageOperands(inst.Operands[3:])
				if inst.Opcode == OpImageSampleDrefImplicitLod {
					ops.lod, ops.hasGrad = 0, false
				}
				ref := toFloat32(interp.values[inst.Operands[2]])
				interp.values[inst.ResultID] = interp.sampleCompare(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], ref, ops)
			}

		case OpImageGather, OpImageDrefGather:
			// OpImageGather: type resultID sampledImage coordinate component [ImageOperands...]
			// OpImageDrefGather: type resultID sampledImage coordinate Dref [ImageOperands...]
			// A gather of an integer texture gives an integer vector.
			if len(inst.Operands) >= 3 {
				ops := interp.parseImageOperands(inst.Operands[3:])
				g := interp.gather(inst.Opcode, interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], interp.values[inst.Operands[2]], ops)
				interp.values[inst.ResultID] = texelResult(inst.resShape, g)
			}

		case OpImageFetch:
			// OpImageFetch: type resultID image coordinate [ImageOperands...]
			// Direct texel fetch by integer coordinates (no filtering), at
			// the level the Lod operand names.
			if len(inst.Operands) >= 2 {
				imgVal := interp.values[inst.Operands[0]]
				coord := interp.values[inst.Operands[1]]
				ops := interp.parseImageOperands(inst.Operands[2:])
				interp.values[inst.ResultID] = texelResult(inst.resShape, interp.fetchTexelLevel(imgVal, coord, int(ops.lod)))
			}

		case OpBitCount, OpBitReverse:
			// OpBitCount / OpBitReverse: type resultID base
			if len(inst.Operands) >= 1 {
				f := bitCount
				if inst.Opcode == OpBitReverse {
					f = bitReverse
				}
				interp.values[inst.ResultID] = mapIntLanes(inst.resShape, interp.values[inst.Operands[0]], f)
			}

		case OpBitFieldInsert:
			// OpBitFieldInsert: type resultID base insert offset count. Base
			// and insert have the result type; offset and count are scalars.
			if len(inst.Operands) >= 4 {
				base := interp.values[inst.Operands[0]]
				s := shapeOr(inst.resShape, base, kindUint)
				k, n := s.kind(), s.lanes()
				b, ins := laneWords(base, k), laneWords(interp.values[inst.Operands[1]], k)
				offset := toUint32(interp.values[inst.Operands[2]])
				count := toUint32(interp.values[inst.Operands[3]])
				for i := range n {
					b[i] = bitFieldInsert(b[i], ins[i], offset, count)
				}
				interp.values[inst.ResultID] = laneValue(k, n, b)
			}

		case OpBitFieldSExtract, OpBitFieldUExtract:
			// OpBitField[SU]Extract: type resultID base offset count
			if len(inst.Operands) >= 3 {
				offset := toUint32(interp.values[inst.Operands[1]])
				count := toUint32(interp.values[inst.Operands[2]])
				signed := inst.Opcode == OpBitFieldSExtract
				interp.values[inst.ResultID] = mapIntLanes(inst.resShape, interp.values[inst.Operands[0]],
					func(base uint32) uint32 { return bitFieldExtract(base, offset, count, signed) })
			}

		case OpDPdx, OpDPdy, OpFwidth, OpDPdxFine, OpDPdyFine, OpFwidthFine,
			OpDPdxCoarse, OpDPdyCoarse, OpFwidthCoarse:
			// Derivatives need the 2x2 quad a fragment runs in on a GPU;
			// see zeroDerivative.
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = zeroDerivative(interp.values[inst.Operands[0]])
			}

		case OpArrayLength:
			// OpArrayLength: type resultID structPointer memberIndex
			// The element count of a storage buffer's runtime array, which
			// naga's Restrict bounds checks clamp indices with.
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = ValUint(interp.arrayLength(interp.values[inst.Operands[0]], inst.Operands[1]))
			}

		case OpImageRead:
			// OpImageRead: type resultID image coordinate [ImageOperands...]
			// A storage texture load (textureLoad of a read-only or
			// read-write storage texture).
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = texelResult(inst.resShape, interp.readImage(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]]))
			}

		case OpImageWrite:
			// OpImageWrite: image coordinate texel [ImageOperands...]
			// A storage texture store (textureStore); no result.
			if len(inst.Operands) >= 3 {
				interp.writeImage(interp.values[inst.Operands[0]], interp.values[inst.Operands[1]], interp.values[inst.Operands[2]])
			}

		case OpImageQuerySize:
			// OpImageQuerySize: type resultID image
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.queryImageSize(interp.values[inst.Operands[0]], 0, inst.resShape)
			}

		case OpImageQuerySizeLod:
			// OpImageQuerySizeLod: type resultID image lod
			if len(inst.Operands) >= 2 {
				lod := scalarInt(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = interp.queryImageSize(interp.values[inst.Operands[0]], lod, inst.resShape)
			}

		case OpImageQueryLevels:
			// OpImageQueryLevels: type resultID image
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.intResult(inst.TypeID, interp.queryImageCount(interp.values[inst.Operands[0]], false))
			}

		case OpImageQuerySamples:
			// OpImageQuerySamples: type resultID image
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = interp.intResult(inst.TypeID, interp.queryImageCount(interp.values[inst.Operands[0]], true))
			}

		case OpAny, OpAll:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = allAny(inst.Opcode == OpAll, interp.values[inst.Operands[0]])
			}

		default:
			// ParseModule rejects every opcode outside supportedOps, so this
			// is reached only by an instruction built without it.
			return unsupportedOpcode(inst.Opcode)
		}

		// --- Debug: post-instruction hooks ---
		if debug != nil {
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
//
// If the callee stops at a workgroup barrier (syncBarriers), it is returned
// as callee, still holding its state: the caller suspends with it and
// resumeCall continues it. Otherwise callee is nil and the callee is back in
// the pool.
func (interp *interpreter) callFunction(funcID uint32, argIDs []uint32) (result Value, callee *interpreter, err error) {
	if interp.callDepth >= maxCallDepth {
		return Value{}, nil, fmt.Errorf("spirv: exceeded maximum call depth (%d)", maxCallDepth)
	}

	m := interp.module
	fn, ok := m.FunctionsByID[funcID]
	if !ok {
		return Value{}, nil, fmt.Errorf("spirv: function %d not found", funcID)
	}

	// Acquire a pooled child interpreter to avoid allocating per call.
	child := m.getInterpreter()
	child.ep = interp.ep
	child.fn = fn
	child.ctx = interp.ctx
	child.callDepth = interp.callDepth + 1
	child.prevBlock = 0
	child.iterationCount = 0
	child.returnValue = Value{}
	child.syncBarriers = interp.syncBarriers

	child.seedConstants()

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

	// The callee sees the invocation's module-scope variables: the resource
	// bindings, and the Private and Workgroup variables, whose pointers are
	// shared so its stores reach the caller (and, for Workgroup memory, the
	// other invocations).
	prep := m.prepared()
	for _, ids := range [...][]uint32{prep.resources, prep.private, prep.workgroup} {
		for _, varID := range ids {
			if v := interp.values[varID]; !v.IsNone() {
				child.values[varID] = v
			}
		}
	}

	return child.finishCall()
}

// finishCall runs interp, a callee, (or resumes it) and returns its result,
// or interp itself if it stopped at a barrier (see callFunction).
func (interp *interpreter) finishCall() (Value, *interpreter, error) {
	if err := interp.run(); err != nil {
		interp.module.release(interp)
		return Value{}, nil, err
	}
	if interp.suspended {
		return Value{}, interp, nil
	}
	retVal := interp.returnValue
	interp.module.putInterpreter(interp)
	return retVal, nil, nil
}

// resumeCall continues the callee interp is suspended in. When the callee
// returns, its result becomes the value of the OpFunctionCall; when it
// stops at the next barrier, interp stays suspended in it.
func (interp *interpreter) resumeCall() error {
	child := interp.pendingCall
	interp.pendingCall = nil
	result, callee, err := child.finishCall()
	if err != nil {
		return err
	}
	if callee != nil {
		interp.pendingCall = callee
		interp.suspended = true
		return nil
	}
	interp.values[interp.pendingResult] = result
	return nil
}

// syncsWorkgroup reports whether an OpControlBarrier with execution scope
// scope makes the invocations of a workgroup wait for each other: a
// Workgroup scope or a wider one (CrossDevice 0, Device 1, Workgroup 2,
// QueueFamily 5). A Subgroup (3) or Invocation (4) barrier does not: the
// interpreter has no subgroup operations, and suspending there could let an
// invocation pass a later workgroup barrier before the others reach it.
func syncsWorkgroup(scope uint32) bool {
	const (
		scopeSubgroup   = 3
		scopeInvocation = 4
	)
	return scope != scopeSubgroup && scope != scopeInvocation
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

// arrayLength returns the element count of the runtime array that is member
// member of the struct a storage buffer pointer points at: the bytes of the
// binding past the member's offset, divided by the array stride and
// truncated (WGSL NRuntime). 0 when the pointer is not a storage buffer
// pointer to such a struct.
func (interp *interpreter) arrayLength(ptr Value, member uint32) uint32 {
	if ptr.Tag != TagBufferPointer {
		return 0
	}
	bp := ptr.AsBufferPointer()
	m := interp.module
	st := bp.Type
	if st == nil || st.Kind != TypeStruct || int(member) >= len(st.MemberIDs) {
		return 0
	}
	arr := m.Types[st.MemberIDs[member]]
	if arr == nil || arr.Kind != TypeArray {
		return 0
	}
	stride := arrayStride(m, arr)
	start := uint64(bp.Offset) + uint64(m.GetMemberOffset(m.typeID(st), member))
	if stride == 0 || start >= uint64(len(bp.Buffer)) {
		return 0
	}
	return uint32((uint64(len(bp.Buffer)) - start) / uint64(stride))
}

// bufferAccessChain computes a byte offset through a chain of indexes into
// a raw buffer, returning a BufferPointer at the final element position.
func (interp *interpreter) bufferAccessChain(bp *BufferPointer, indexes []uint32) *BufferPointer {
	m := interp.module
	offset := bp.Offset
	currentType := bp.Type
	// The MatrixStride of a struct member applies to the matrices it holds,
	// through any arrays, until a matrix is indexed.
	matStride := bp.MatrixStride

	for _, idxID := range indexes {
		idx := toUint32(interp.values[idxID])

		if currentType == nil {
			break
		}

		switch currentType.Kind {
		case TypeStruct:
			// For struct members, use the MemberDecorate Offset if available.
			structTypeID := m.typeID(currentType)
			memberOffset := m.GetMemberOffset(structTypeID, idx)
			offset += memberOffset
			matStride = m.memberMatrixStride(structTypeID, idx)
			if int(idx) < len(currentType.MemberIDs) {
				currentType = m.Types[currentType.MemberIDs[idx]]
			} else {
				currentType = nil
			}

		case TypeArray:
			offset += idx * arrayStride(m, currentType)
			currentType = m.Types[currentType.ElemType]

		case TypeMatrix:
			// Indexing into a matrix selects a column vector.
			offset += idx * matrixColumnStride(m, currentType, matStride)
			matStride = 0
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

	return &BufferPointer{Buffer: bp.Buffer, Offset: offset, Type: currentType, MatrixStride: matStride}
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

// compositeConstruct builds a composite value of result type typeID, shape
// s, from constituent IDs. A vector takes the lanes of its constituents in
// order, each scalar one lane and each vector all of its lanes (SPIR-V
// OpCompositeConstruct), as a value of its own kind: a u32, i32 or bool
// vector keeps raw words, never floats.
func (interp *interpreter) compositeConstruct(s shape, typeID uint32, constituentIDs []uint32) Value {
	if s == 0 && interp.module != nil {
		s = interp.module.typeShape(typeID)
	}
	if s.lanes() >= 2 {
		var p lanePool
		for _, id := range constituentIDs {
			p.add(interp.values[id], s.kind())
		}
		return wordsValue(s, [4]uint32(p.w[:4]))
	}
	ti, ok := interp.module.Types[typeID]
	if !ok {
		return ValVec4(0, 0, 0, 0)
	}

	switch ti.Kind {
	case TypeVector:
		// A vector of no 32-bit lane kind (f16, f64): float lanes.
		var p lanePool
		for _, id := range constituentIDs {
			p.add(interp.values[id], kindFloat)
		}
		return wordsValue(shapeOf(kindFloat, int(ti.Components)), [4]uint32(p.w[:4]))

	case TypeMatrix:
		// Matrix is constructed from column vectors.
		cols := make([]Value, len(constituentIDs))
		for i, id := range constituentIDs {
			cols[i] = interp.values[id]
		}
		return ValArray(cols)

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
//
// The matrix has C columns of R components and the vector C components; the
// result has R components (matCxR * vecC = vecR). Every size from 2 to 4 is
// handled: only 4-component vectors were multiplied before, so a mat2x2 or
// mat3x2 product returned the vector unchanged.
func matrixTimesVector(mat, vec Value) Value {
	if mat.Tag != TagArray {
		return vec
	}
	cols := mat.AsArray()
	numCols := len(cols)
	if numCols < 2 || numCols > 4 || vectorSize(vec.Tag) < numCols {
		return vec
	}
	rows := vectorSize(cols[0].Tag)
	if rows < 2 {
		return vec
	}

	// result = col[0]*v[0] + col[1]*v[1] + ...
	var result [4]float32
	for i, c := range cols {
		col := Vec4ToFloat32(c)
		for j := range rows {
			result[j] += col[j] * vec.F[i]
		}
	}
	switch rows {
	case 2:
		return ValVec2(result[0], result[1])
	case 3:
		return ValVec3(result[0], result[1], result[2])
	default:
		return ValVec4From(result)
	}
}

// vectorSize returns the component count of a float vector tag, 0 for
// anything else.
func vectorSize(tag ValueTag) int {
	switch tag {
	case TagVec2:
		return 2
	case TagVec3:
		return 3
	case TagVec4:
		return 4
	default:
		return 0
	}
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

// vectorShuffle creates a new vector of shape s (result type typeID) by
// selecting lanes from two input vectors of its lane kind. A component
// index of 0xFFFFFFFF (undefined) or past the inputs reads zero.
func vectorShuffle(m *Module, s shape, typeID uint32, vec1, vec2 Value, components []uint32) Value {
	if s == 0 && m != nil {
		s = m.typeShape(typeID)
	}
	if s.lanes() < 2 {
		k := vectorKind(vec1.Tag)
		if k == kindNone {
			k = kindFloat
		}
		s = shapeOf(k, len(components))
	}
	var p lanePool
	p.add(vec1, s.kind())
	p.add(vec2, s.kind())
	var w [4]uint32
	for i, idx := range components {
		if i == len(w) {
			break
		}
		if idx < uint32(p.n) {
			w[i] = p.w[idx]
		}
	}
	return wordsValue(s, w)
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

// sampleTexture samples a sampled image at coordinate coord with image
// operands ops: the level of detail is ops.lod (or the one ops' derivatives
// give) plus ops.bias.
func (interp *interpreter) sampleTexture(sampledImg Value, coord Value, ops imageOperands) Value {
	tex, samp, ok := interp.sampledResources(sampledImg)
	if !ok {
		return ValVec4(1, 0, 1, 1) // Magenta for a missing texture.
	}
	c := texelComponents(coord)
	return ValVec4From(sampleView(tex, samp, c, ops.lambda(tex, c), ops.offset))
}

// sampledResources resolves the texture and sampler of a sampled image; ok
// is false when the value is not a sampled image or its texture is missing.
// An unbound sampler reads as the default one: nearest, repeat, no LOD
// clamp.
func (interp *interpreter) sampledResources(sampledImg Value) (*Texture2D, *Sampler, bool) {
	if sampledImg.Tag != TagSampledImage {
		return nil, nil, false
	}
	si := sampledImg.AsSampledImage()
	tex := interp.resolveTexture(si.Image)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return nil, nil, false
	}
	samp := interp.resolveSampler(si.Sampler)
	if samp == nil {
		samp = &Sampler{LodMaxClamp: LodClampNone}
	}
	return tex, samp, true
}

// fetchTexel fetches a single texel of the view's base level by integer
// coordinates (no filtering).
func (interp *interpreter) fetchTexel(imgVal Value, coord Value) Value {
	return interp.fetchTexelLevel(imgVal, coord, 0)
}

// fetchTexelLevel fetches a single texel of mip level level of the view by
// integer coordinates: x for 1D, x, y for 2D, then the array layer or the
// depth slice. Each coordinate and the level are clamped to the view, as
// naga's Restrict bounds checks would clamp them.
func (interp *interpreter) fetchTexelLevel(imgVal Value, coord Value, level int) Value {
	tex := interp.resolveTexture(imgVal)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return ValVec4(0, 0, 0, 0)
	}
	lv := tex.level(level)
	xyz := imageCoord3(coord)
	img, _ := tex.imageAt(lv, clampInt(tex.imageIndex(xyz), int(max(lv.Images, 1))-1))
	if img.Width == 0 || img.Height == 0 {
		return ValVec4(0, 0, 0, 0)
	}
	x := clampInt(xyz[0], int(img.Width)-1)
	y := clampInt(xyz[1], int(img.Height)-1)
	return ValVec4From(readTexel(&img, x, y))
}

// scalarInt converts an integer or float scalar to int; a signed integer
// keeps its sign.
func scalarInt(v Value) int {
	switch v.Tag {
	case TagFloat32:
		return int(v.F[0])
	case TagUint32:
		return int(v.U[0])
	default:
		return int(int32(toUint32(v)))
	}
}

// readImage loads a storage texture texel (OpImageRead), decoded from the
// texture format. An out-of-bounds or undecodable texel reads zero.
func (interp *interpreter) readImage(imgVal, coord Value) Value {
	img, x, y, ok := interp.storageImage(imgVal, coord)
	if !ok {
		return ValVec4(0, 0, 0, 0)
	}
	v, ok := decodeTexel(&img, x, y)
	if !ok {
		return ValVec4(0, 0, 0, 0)
	}
	return ValVec4From(v)
}

// writeImage stores a texel into a storage texture (OpImageWrite), encoded
// in the texture format. The texture's Data aliases its storage, so the write
// is visible to later passes. Out-of-bounds writes are discarded.
func (interp *interpreter) writeImage(imgVal, coord, texel Value) {
	img, x, y, ok := interp.storageImage(imgVal, coord)
	if !ok {
		return
	}
	encodeTexel(&img, x, y, texelComponents(texel))
}

// storageImage returns the image of a storage texture view that coordinate
// coord addresses (the array layer of a 2D-array view, the depth slice of a
// 3D view; a storage view has one mip level) and the texel's x, y in it, or
// false when the layer or slice is out of bounds.
func (interp *interpreter) storageImage(imgVal, coord Value) (img Texture2D, x, y int, ok bool) {
	tex := interp.resolveTexture(imgVal)
	if tex == nil {
		return Texture2D{}, 0, 0, false
	}
	xyz := imageCoord3(coord)
	img, ok = tex.imageAt(tex.level(0), tex.imageIndex(xyz))
	return img, xyz[0], xyz[1], ok
}

// texelComponents returns the four components of a texel value (a float
// vector, an integer vector by value, or a scalar filling the first one).
func texelComponents(v Value) Vec4 {
	switch v.Tag {
	case TagVec2, TagVec3, TagVec4:
		return v.F
	case TagUVec2, TagUVec3, TagUVec4, TagIVec2, TagIVec3, TagIVec4, TagBVec2, TagBVec3, TagBVec4:
		var out Vec4
		for i := range vectorLanes(v.Tag) {
			out[i] = scalarFloat(indexComposite(v, uint32(i)))
		}
		return out
	default:
		return Vec4{scalarFloat(v), 0, 0, 0}
	}
}

// scalarFloat converts a scalar to float32 by value; a signed integer keeps
// its sign.
func scalarFloat(v Value) float32 {
	switch v.Tag {
	case TagFloat32:
		return v.F[0]
	case TagUint32:
		return float32(v.U[0])
	case TagInt32:
		return float32(int32(v.U[0]))
	default:
		return toFloat32(v)
	}
}

// queryImageSize returns the size of a texture view as a vec2 of uint32
// values.
//
// It answers OpImageQuerySize (lod 0) and OpImageQuerySizeLod: the size of
// mip level lod of the view, each dimension at least 1. The third component,
// when the result type has one, is the depth of that level for a 3D view,
// the number of cubes of a cube-array view, and the array layer count
// otherwise. The result has shape s, the instruction's: a u32 or i32 scalar
// for 1D, a vector otherwise; a uvec2 when s is zero (no module). A missing
// texture reads zeros.
func (interp *interpreter) queryImageSize(imgVal Value, lod int, s shape) Value {
	if s == 0 {
		s = shapeOf(kindUint, 2)
	}
	tex := interp.resolveTexture(imgVal)
	var size [4]uint32
	if tex != nil {
		size = [4]uint32{mipSize(tex.Width, lod), mipSize(tex.Height, lod), max(tex.Layers, 1)}
		switch tex.Dimension {
		case TexDim3D:
			size[2] = mipSize(tex.Depth, lod)
		case TexDimCubeArray:
			size[2] = max(tex.Layers/6, 1)
		}
	}
	k := s.kind()
	if k != kindSint {
		k = kindUint
	}
	return laneValue(k, s.lanes(), size)
}

// mipSize is the extent of mip level lod of a dimension of extent base.
func mipSize(base uint32, lod int) uint32 {
	if lod < 0 || lod > 31 {
		return 1
	}
	return max(base>>uint(lod), 1)
}

// queryImageCount answers OpImageQueryLevels (the mip level count) or, with
// samples, OpImageQuerySamples (the sample count); 0 without a texture.
func (interp *interpreter) queryImageCount(imgVal Value, samples bool) uint32 {
	tex := interp.resolveTexture(imgVal)
	if tex == nil {
		return 0
	}
	if samples {
		return max(tex.SampleCount, 1)
	}
	return max(tex.MipLevels, 1)
}

// intResult returns n as the integer scalar type typeID names: signed or
// unsigned.
func (interp *interpreter) intResult(typeID, n uint32) Value {
	if ti := interp.typeInfo(typeID); ti != nil && ti.Kind == TypeInt && ti.Signed {
		return ValInt(int32(n))
	}
	return ValUint(n)
}

// typeInfo returns the type typeID names, or nil.
func (interp *interpreter) typeInfo(typeID uint32) *TypeInfo {
	if interp.module == nil {
		return nil
	}
	return interp.module.Types[typeID]
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

// readTexel reads texel (x, y) of the texture decoded by its format
// (decodeTexel): missing channels read (0, 0, 0, 1), and a texel out of
// bounds or of a format without a layout reads zero.
func readTexel(tex *Texture2D, x, y int) Vec4 {
	v, _ := decodeTexel(tex, x, y)
	return v
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
