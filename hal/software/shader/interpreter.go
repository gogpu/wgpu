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

	interp := &interpreter{
		module: m,
		ep:     ep,
		fn:     fn,
		ctx:    ctx,
		values: make(map[uint32]Value, m.Bound),
		labels: make(map[uint32]int),
	}

	// Seed constants into the value map.
	for id, val := range m.Constants {
		interp.values[id] = val
	}

	// Build label index for OpBranch targets.
	for i, inst := range fn.Instructions {
		if inst.Opcode == OpLabel {
			interp.labels[inst.ResultID] = i
		}
	}

	// Initialize interface variables (inputs/outputs) and resource bindings.
	inputs := ctx.Inputs
	interp.initVariables(inputs)
	interp.initResourceVariables()

	// Execute the function body.
	if err := interp.run(); err != nil {
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
		if ptr, ok := interp.values[varID].(*Pointer); ok {
			outputs[varID] = ptr.Value
		}
	}

	return outputs, nil
}

// maxIterations is the safety limit for loop iterations to prevent infinite loops.
const maxIterations = 100000

// interpreter holds execution state for a single entry point invocation.
type interpreter struct {
	module *Module
	ep     *EntryPoint
	fn     *Function
	ctx    *ExecutionContext
	values map[uint32]Value
	labels map[uint32]int // label ID -> instruction index

	// prevBlock tracks the predecessor block ID for OpPhi resolution.
	prevBlock uint32

	// iterationCount tracks total branch-back iterations for loop safety.
	iterationCount int

	// callDepth tracks function call nesting to prevent stack overflow.
	callDepth int

	// returnValue holds the value from OpReturnValue for function calls.
	returnValue Value
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
			if val == nil {
				// Default zero value based on pointee type.
				val = zeroValueForVar(m, vi.TypeID)
			}
			interp.values[varID] = &Pointer{Value: val}

		case StorageClassOutput:
			interp.values[varID] = &Pointer{Value: zeroValueForVar(m, vi.TypeID)}
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
				interp.values[varID] = &Pointer{Value: zeroValueForVar(m, vi.TypeID)}
				continue
			}
			pointeeType := m.PointeeType(vi.TypeID)
			if pointeeType == nil {
				interp.values[varID] = &Pointer{Value: zeroValueForVar(m, vi.TypeID)}
				continue
			}
			if vi.StorageClass == StorageClassStorageBuffer {
				// Storage buffers use BufferPointer for direct read/write to raw bytes.
				// This ensures OpStore writes are immediately reflected in the buffer.
				interp.values[varID] = &BufferPointer{Buffer: bufData, Offset: 0, Type: pointeeType}
			} else {
				// Uniform and push constant buffers are read-only -- deserialize once.
				val := interp.readValueFromBuffer(bufData, 0, pointeeType)
				interp.values[varID] = &Pointer{Value: val}
			}

		case StorageClassUniformConstant:
			// UniformConstant is used for textures/samplers -- handled separately.
			bk, hasBind := m.GetBinding(varID)
			if !hasBind {
				continue
			}
			// Store the binding key as the value so OpLoad can resolve texture/sampler.
			interp.values[varID] = &Pointer{Value: bk}
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
			return Float32(0)
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		return Float32(math.Float32frombits(bits))

	case TypeInt:
		if offset+4 > uint32(len(data)) {
			if ti.Signed {
				return Int32(0)
			}
			return Uint32(0)
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		if ti.Signed {
			return Int32(int32(bits))
		}
		return Uint32(bits)

	case TypeBool:
		if offset+4 > uint32(len(data)) {
			return false
		}
		bits := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
		return bits != 0

	case TypeVector:
		elemType := m.Types[ti.ElemType]
		if elemType == nil {
			return zeroValue(m.Types, ti.ElemType)
		}
		elemSize := typeByteSize(m, elemType)
		switch ti.Components {
		case 2:
			var v Vec2
			for i := uint32(0); i < 2; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return v
		case 3:
			var v Vec3
			for i := uint32(0); i < 3; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return v
		case 4:
			var v Vec4
			for i := uint32(0); i < 4; i++ {
				f := interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
				v[i] = toFloat32(f)
			}
			return v
		}
		return Uint32(0)

	case TypeArray:
		elemType := m.Types[ti.ElemType]
		if elemType == nil || ti.Length == 0 {
			return Array{}
		}
		// Use ArrayStride decoration if available, otherwise compute.
		elemSize := typeByteSize(m, elemType)
		arr := make(Array, ti.Length)
		for i := uint32(0); i < ti.Length; i++ {
			arr[i] = interp.readValueFromBuffer(data, offset+i*elemSize, elemType)
		}
		return arr

	case TypeStruct:
		members := make(Array, len(ti.MemberIDs))
		// For each struct member, look up its type and use MemberDecorate Offset.
		structTypeID := interp.findTypeID(ti)
		for i, memberTypeID := range ti.MemberIDs {
			memberType := m.Types[memberTypeID]
			if memberType == nil {
				members[i] = Uint32(0)
				continue
			}
			memberOffset := m.GetMemberOffset(structTypeID, uint32(i))
			members[i] = interp.readValueFromBuffer(data, offset+memberOffset, memberType)
		}
		return members

	default:
		return Uint32(0)
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
		// For structs, sum member sizes (respecting offsets if decorated).
		var maxEnd uint32
		for i, memberTypeID := range ti.MemberIDs {
			memberType := m.Types[memberTypeID]
			if memberType == nil {
				continue
			}
			memberSize := typeByteSize(m, memberType)
			// Use the end of this member as candidate for total size.
			end := uint32(i)*4 + memberSize // fallback without decoration
			if end > maxEnd {
				maxEnd = end
			}
		}
		return maxEnd
	default:
		return 4
	}
}

// writeValueToBuffer serializes a SPIR-V typed value into raw buffer bytes.
// Used for storage buffer writes via OpStore.
func writeValueToBuffer(data []byte, offset uint32, val Value) {
	switch v := val.(type) {
	case Float32:
		if offset+4 > uint32(len(data)) {
			return
		}
		bits := math.Float32bits(float32(v))
		data[offset] = byte(bits)
		data[offset+1] = byte(bits >> 8)
		data[offset+2] = byte(bits >> 16)
		data[offset+3] = byte(bits >> 24)
	case Uint32:
		if offset+4 > uint32(len(data)) {
			return
		}
		data[offset] = byte(v)
		data[offset+1] = byte(v >> 8)
		data[offset+2] = byte(v >> 16)
		data[offset+3] = byte(v >> 24)
	case Int32:
		if offset+4 > uint32(len(data)) {
			return
		}
		u := uint32(v)
		data[offset] = byte(u)
		data[offset+1] = byte(u >> 8)
		data[offset+2] = byte(u >> 16)
		data[offset+3] = byte(u >> 24)
	case Vec2:
		writeValueToBuffer(data, offset, Float32(v[0]))
		writeValueToBuffer(data, offset+4, Float32(v[1]))
	case Vec3:
		writeValueToBuffer(data, offset, Float32(v[0]))
		writeValueToBuffer(data, offset+4, Float32(v[1]))
		writeValueToBuffer(data, offset+8, Float32(v[2]))
	case Vec4:
		writeValueToBuffer(data, offset, Float32(v[0]))
		writeValueToBuffer(data, offset+4, Float32(v[1]))
		writeValueToBuffer(data, offset+8, Float32(v[2]))
		writeValueToBuffer(data, offset+12, Float32(v[3]))
	case Array:
		off := offset
		for _, elem := range v {
			writeValueToBuffer(data, off, elem)
			off += valueByteSize(elem)
		}
	}
}

// valueByteSize returns the byte size of a runtime Value.
func valueByteSize(val Value) uint32 {
	switch v := val.(type) {
	case Float32:
		return 4
	case Uint32:
		return 4
	case Int32:
		return 4
	case Vec2:
		return 8
	case Vec3:
		return 12
	case Vec4:
		return 16
	case Array:
		var total uint32
		for _, elem := range v {
			total += valueByteSize(elem)
		}
		return total
	default:
		_ = v
		return 4
	}
}

// zeroValueForVar creates a zero value matching the pointee type of a pointer type.
func zeroValueForVar(m *Module, ptrTypeID uint32) Value {
	ti := m.PointeeType(ptrTypeID)
	if ti == nil {
		return Uint32(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return Float32(0)
	case TypeInt:
		if ti.Signed {
			return Int32(0)
		}
		return Uint32(0)
	case TypeVector:
		switch ti.Components {
		case 2:
			return Vec2{}
		case 3:
			return Vec3{}
		case 4:
			return Vec4{}
		}
	case TypeArray:
		if ti.Length > 0 {
			arr := make(Array, ti.Length)
			for i := range arr {
				arr[i] = zeroValue(m.Types, ti.ElemType)
			}
			return arr
		}
	}
	return Uint32(0)
}

// run executes instructions sequentially, handling OpBranch for jumps.
//
//nolint:maintidx,unparam // Opcode dispatch switch is inherently large. Error return is API contract for future opcodes.
func (interp *interpreter) run() error {
	instructions := interp.fn.Instructions
	pc := 0

	for pc < len(instructions) {
		inst := instructions[pc]
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
				interp.values[inst.ResultID] = &Pointer{Value: val}
			}

		case OpLoad:
			// OpLoad: type resultID pointer [memory access]
			if len(inst.Operands) < 1 {
				break
			}
			ptrID := inst.Operands[0]
			switch p := interp.values[ptrID].(type) {
			case *Pointer:
				interp.values[inst.ResultID] = p.Value
			case *BufferPointer:
				// Read directly from the raw buffer.
				if p.Type != nil {
					interp.values[inst.ResultID] = interp.readValueFromBuffer(p.Buffer, p.Offset, p.Type)
				} else {
					interp.values[inst.ResultID] = Uint32(0)
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
			switch p := interp.values[ptrID].(type) {
			case *Pointer:
				p.Value = interp.values[valID]
			case *BufferPointer:
				// Write directly to the raw buffer.
				writeValueToBuffer(p.Buffer, p.Offset, interp.values[valID])
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
			if idx, ok := interp.labels[targetLabel]; ok {
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
			if len(inst.Operands) >= 3 {
				cond := interp.values[inst.Operands[0]]
				trueLabel := inst.Operands[1]
				falseLabel := inst.Operands[2]
				target := falseLabel
				if toBool(cond) {
					target = trueLabel
				}
				if idx, ok := interp.labels[target]; ok {
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
				interp.values[inst.ResultID] = a == b
			}

		case OpINotEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a != b
			}

		case OpFOrdEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a == b
			}

		case OpFOrdLessThan:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a < b
			}

		case OpFOrdGreaterThan:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a > b
			}

		case OpFOrdLessThanEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a <= b
			}

		case OpFOrdGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := toFloat32(interp.values[inst.Operands[0]])
				b := toFloat32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a >= b
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
				if idx, ok := interp.labels[target]; ok {
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
					interp.values[inst.ResultID] = Int32(a / b)
				} else {
					interp.values[inst.ResultID] = Int32(0)
				}
			}

		case OpUDiv:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				if b != 0 {
					interp.values[inst.ResultID] = Uint32(a / b)
				} else {
					interp.values[inst.ResultID] = Uint32(0)
				}
			}

		case OpSMod:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				if b != 0 {
					interp.values[inst.ResultID] = Int32(a % b)
				} else {
					interp.values[inst.ResultID] = Int32(0)
				}
			}

		case OpUMod:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				if b != 0 {
					interp.values[inst.ResultID] = Uint32(a % b)
				} else {
					interp.values[inst.ResultID] = Uint32(0)
				}
			}

		case OpSRem:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				if b != 0 {
					// SPIR-V SRem: remainder has same sign as dividend.
					interp.values[inst.ResultID] = Int32(a % b)
				} else {
					interp.values[inst.ResultID] = Int32(0)
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
				interp.values[inst.ResultID] = Int32(-a)
			}

		case OpConvertFToS:
			if len(inst.Operands) >= 1 {
				f := toFloat32(interp.values[inst.Operands[0]])
				interp.values[inst.ResultID] = Int32(int32(f))
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
				interp.values[inst.ResultID] = a < b
			}

		case OpUGreaterThan:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a > b
			}

		case OpULessThanEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a <= b
			}

		case OpUGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := toUint32(interp.values[inst.Operands[0]])
				b := toUint32(interp.values[inst.Operands[1]])
				interp.values[inst.ResultID] = a >= b
			}

		case OpSLessThan:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = a < b
			}

		case OpSGreaterThan:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = a > b
			}

		case OpSLessThanEqual:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = a <= b
			}

		case OpSGreaterThanEqual:
			if len(inst.Operands) >= 2 {
				a := int32(toUint32(interp.values[inst.Operands[0]]))
				b := int32(toUint32(interp.values[inst.Operands[1]]))
				interp.values[inst.ResultID] = a >= b
			}

		case OpLogicalAnd:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = toBool(interp.values[inst.Operands[0]]) && toBool(interp.values[inst.Operands[1]])
			}

		case OpLogicalOr:
			if len(inst.Operands) >= 2 {
				interp.values[inst.ResultID] = toBool(interp.values[inst.Operands[0]]) || toBool(interp.values[inst.Operands[1]])
			}

		case OpLogicalNot:
			if len(inst.Operands) >= 1 {
				interp.values[inst.ResultID] = !toBool(interp.values[inst.Operands[0]])
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
				interp.values[inst.ResultID] = Uint32(^a)
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
				interp.values[inst.ResultID] = Int32(a >> b)
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
			interp.values[inst.ResultID] = Uint32(0)

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
				if setName == "GLSL.std.450" {
					interp.values[inst.ResultID] = interp.executeGLSLExtInst(instNum, extOps)
				}
			}

		case OpSampledImage:
			// OpSampledImage: type resultID image sampler
			// Combines a texture and sampler into a SampledImage pair.
			if len(inst.Operands) >= 2 {
				imgVal := interp.values[inst.Operands[0]]
				sampVal := interp.values[inst.Operands[1]]
				interp.values[inst.ResultID] = &SampledImageValue{Image: imgVal, Sampler: sampVal}
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
// module-level constants and context.
func (interp *interpreter) callFunction(funcID uint32, argIDs []uint32) (Value, error) {
	if interp.callDepth >= maxCallDepth {
		return nil, fmt.Errorf("spirv: exceeded maximum call depth (%d)", maxCallDepth)
	}

	fn, ok := interp.module.FunctionsByID[funcID]
	if !ok {
		return nil, fmt.Errorf("spirv: function %d not found", funcID)
	}

	// Create a child interpreter with its own value scope.
	child := &interpreter{
		module:    interp.module,
		ep:        interp.ep,
		fn:        fn,
		ctx:       interp.ctx,
		values:    make(map[uint32]Value, interp.module.Bound),
		labels:    make(map[uint32]int),
		callDepth: interp.callDepth + 1,
	}

	// Copy constants.
	for id, val := range interp.module.Constants {
		child.values[id] = val
	}

	// Build label index.
	for i, inst := range fn.Instructions {
		if inst.Opcode == OpLabel {
			child.labels[inst.ResultID] = i
		}
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
			if v, ok := interp.values[varID]; ok {
				child.values[varID] = v
			}
		}
	}

	// Execute the callee.
	if err := child.run(); err != nil {
		return nil, err
	}

	// Collect the return value. We look for OpReturnValue which stores
	// the return value ID in Operands[0].
	// Since run() returns on OpReturnValue, the last-returned value is
	// whatever OpReturnValue pointed to.
	return child.returnValue, nil
}

// accessChain navigates into a composite value through a chain of indexes,
// returning a Pointer or BufferPointer to the innermost element.
func (interp *interpreter) accessChain(baseID uint32, indexes []uint32) Value {
	baseVal := interp.values[baseID]

	// If base is a BufferPointer, compute byte offset through the chain.
	if bp, ok := baseVal.(*BufferPointer); ok {
		return interp.bufferAccessChain(bp, indexes)
	}

	// If base is a Pointer, unwrap.
	if ptr, ok := baseVal.(*Pointer); ok {
		baseVal = ptr.Value
	}

	// Navigate through each index.
	current := baseVal
	for _, idxID := range indexes {
		idx := toUint32(interp.values[idxID])
		current = indexComposite(current, idx)
	}

	return &Pointer{Value: current}
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
			break
		}
	}

	return &BufferPointer{Buffer: bp.Buffer, Offset: offset, Type: currentType}
}

// indexComposite extracts element at the given index from a composite value.
func indexComposite(composite Value, index uint32) Value {
	switch v := composite.(type) {
	case Array:
		if int(index) < len(v) {
			return v[index]
		}
	case Vec2:
		if index < 2 {
			return Float32(v[index])
		}
	case Vec3:
		if index < 3 {
			return Float32(v[index])
		}
	case Vec4:
		if index < 4 {
			return Float32(v[index])
		}
	}
	return Float32(0)
}

// compositeConstruct builds a composite value from constituent IDs.
func (interp *interpreter) compositeConstruct(typeID uint32, constituentIDs []uint32) Value {
	ti, ok := interp.module.Types[typeID]
	if !ok {
		return Vec4{}
	}

	switch ti.Kind {
	case TypeVector:
		// Flatten constituents: each may be scalar or smaller vector.
		var components []float32
		for _, id := range constituentIDs {
			components = appendComponents(components, interp.values[id])
		}
		switch ti.Components {
		case 2:
			var v Vec2
			for i := 0; i < 2 && i < len(components); i++ {
				v[i] = components[i]
			}
			return v
		case 3:
			var v Vec3
			for i := 0; i < 3 && i < len(components); i++ {
				v[i] = components[i]
			}
			return v
		case 4:
			var v Vec4
			for i := 0; i < 4 && i < len(components); i++ {
				v[i] = components[i]
			}
			return v
		}

	case TypeArray:
		arr := make(Array, len(constituentIDs))
		for i, id := range constituentIDs {
			arr[i] = interp.values[id]
		}
		return arr

	case TypeStruct:
		arr := make(Array, len(constituentIDs))
		for i, id := range constituentIDs {
			arr[i] = interp.values[id]
		}
		return arr
	}

	return Uint32(0)
}

// compositeExtract navigates literal indexes to extract a scalar from a composite.
func (interp *interpreter) compositeExtract(composite Value, indexes []uint32) Value {
	current := composite
	for _, idx := range indexes {
		current = indexComposite(current, idx)
	}
	return current
}

// appendComponents flattens a value into float32 components.
func appendComponents(dst []float32, val Value) []float32 {
	switch v := val.(type) {
	case Float32:
		return append(dst, v)
	case Vec2:
		return append(dst, v[0], v[1])
	case Vec3:
		return append(dst, v[0], v[1], v[2])
	case Vec4:
		return append(dst, v[0], v[1], v[2], v[3])
	case Uint32:
		return append(dst, float32(v))
	case Int32:
		return append(dst, float32(v))
	default:
		return append(dst, 0)
	}
}

// convertToFloat converts an unsigned integer value to float32.
func convertToFloat(val Value) Value {
	switch v := val.(type) {
	case Uint32:
		return Float32(float32(v))
	case Int32:
		return Float32(float32(v))
	case Float32:
		return v
	default:
		return Float32(0)
	}
}

// convertSignedToFloat converts a signed integer value to float32.
func convertSignedToFloat(val Value) Value {
	switch v := val.(type) {
	case Int32:
		return Float32(float32(v))
	case Uint32:
		return Float32(float32(int32(v)))
	case Float32:
		return v
	default:
		return Float32(0)
	}
}

// convertFloatToUint converts a float value to uint32.
func convertFloatToUint(val Value) Value {
	switch v := val.(type) {
	case Float32:
		return uint32(v)
	case Uint32:
		return v
	case Int32:
		return uint32(v)
	default:
		return Uint32(0)
	}
}

// toFloat32 extracts a float32 from a Value.
func toFloat32(val Value) float32 {
	switch v := val.(type) {
	case Float32:
		return v
	case Uint32:
		return float32(v)
	case Int32:
		return float32(v)
	default:
		return 0
	}
}

// toUint32 extracts a uint32 from a Value.
func toUint32(val Value) uint32 {
	switch v := val.(type) {
	case Uint32:
		return v
	case Int32:
		return uint32(v)
	case Float32:
		return uint32(v)
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toBool converts a Value to boolean.
func toBool(val Value) bool {
	switch v := val.(type) {
	case bool:
		return v
	case Uint32:
		return v != 0
	case Int32:
		return v != 0
	case Float32:
		return v != 0
	default:
		return false
	}
}

// floatBinOp applies a binary operation to two float-typed values.
// Works on scalars and vectors component-wise.
func floatBinOp(a, b Value, op func(float32, float32) float32) Value {
	switch av := a.(type) {
	case Float32:
		bv := toFloat32(b)
		return Float32(op(av, bv))
	case Vec2:
		bv, ok := b.(Vec2)
		if !ok {
			return a
		}
		return Vec2{op(av[0], bv[0]), op(av[1], bv[1])}
	case Vec3:
		bv, ok := b.(Vec3)
		if !ok {
			return a
		}
		return Vec3{op(av[0], bv[0]), op(av[1], bv[1]), op(av[2], bv[2])}
	case Vec4:
		bv, ok := b.(Vec4)
		if !ok {
			return a
		}
		return Vec4{op(av[0], bv[0]), op(av[1], bv[1]), op(av[2], bv[2]), op(av[3], bv[3])}
	default:
		return Float32(0)
	}
}

// floatUnaryOp applies a unary operation to a float-typed value.
func floatUnaryOp(a Value, op func(float32) float32) Value {
	switch av := a.(type) {
	case Float32:
		return Float32(op(av))
	case Vec2:
		return Vec2{op(av[0]), op(av[1])}
	case Vec3:
		return Vec3{op(av[0]), op(av[1]), op(av[2])}
	case Vec4:
		return Vec4{op(av[0]), op(av[1]), op(av[2]), op(av[3])}
	default:
		return Float32(0)
	}
}

// intBinOp applies a binary operation to two integer values.
func intBinOp(a, b Value, op func(uint32, uint32) uint32) Value {
	av := toUint32(a)
	bv := toUint32(b)
	return op(av, bv)
}

// Vec4ToFloat32 extracts a Vec4 from a Value, returning zeros if type doesn't match.
func Vec4ToFloat32(val Value) [4]float32 {
	switch v := val.(type) {
	case Vec4:
		return v
	case Vec3:
		return [4]float32{v[0], v[1], v[2], 0}
	case Vec2:
		return [4]float32{v[0], v[1], 0, 0}
	case Float32:
		return [4]float32{v, 0, 0, 0}
	default:
		return [4]float32{}
	}
}

// Float32BitsToUint32 converts a float32 to its bit representation as uint32.
// Used for SPIR-V constant encoding in tests.
func Float32BitsToUint32(f float32) uint32 {
	return math.Float32bits(f)
}

// vectorTimesScalar multiplies each component of a vector by a scalar.
func vectorTimesScalar(vec Value, s float32) Value {
	switch v := vec.(type) {
	case Vec2:
		return Vec2{v[0] * s, v[1] * s}
	case Vec3:
		return Vec3{v[0] * s, v[1] * s, v[2] * s}
	case Vec4:
		return Vec4{v[0] * s, v[1] * s, v[2] * s, v[3] * s}
	default:
		return Float32(toFloat32(vec) * s)
	}
}

// dotProduct computes the dot product of two vectors.
func dotProduct(a, b Value) Value {
	switch av := a.(type) {
	case Vec2:
		bv, ok := b.(Vec2)
		if !ok {
			return Float32(0)
		}
		return Float32(av[0]*bv[0] + av[1]*bv[1])
	case Vec3:
		bv, ok := b.(Vec3)
		if !ok {
			return Float32(0)
		}
		return Float32(av[0]*bv[0] + av[1]*bv[1] + av[2]*bv[2])
	case Vec4:
		bv, ok := b.(Vec4)
		if !ok {
			return Float32(0)
		}
		return Float32(av[0]*bv[0] + av[1]*bv[1] + av[2]*bv[2] + av[3]*bv[3])
	default:
		return Float32(toFloat32(a) * toFloat32(b))
	}
}

// matrixTimesVector multiplies a matrix (Array of column vectors) by a vector.
// SPIR-V stores matrices in column-major order as arrays of column vectors.
func matrixTimesVector(mat, vec Value) Value {
	cols, ok := mat.(Array)
	if !ok {
		return vec
	}
	v, ok := vec.(Vec4)
	if !ok {
		return vec
	}
	numCols := len(cols)
	if numCols < 2 {
		return vec
	}

	// Extract columns as Vec4 (padding with zeros if needed).
	colVecs := make([]Vec4, numCols)
	for i, c := range cols {
		colVecs[i] = Vec4ToFloat32(c)
	}

	// result = col[0]*v[0] + col[1]*v[1] + ...
	var result Vec4
	for i := 0; i < numCols && i < 4; i++ {
		for j := 0; j < 4; j++ {
			result[j] += colVecs[i][j] * v[i]
		}
	}
	return result
}

// matrixTimesScalar multiplies every element of a matrix by a scalar.
func matrixTimesScalar(mat, scalar Value) Value {
	cols, ok := mat.(Array)
	if !ok {
		return mat
	}
	s := toFloat32(scalar)
	result := make(Array, len(cols))
	for i, c := range cols {
		result[i] = vectorTimesScalar(c, s)
	}
	return result
}

// matrixTimesMatrix multiplies two matrices (both stored as Array of column vectors).
// C = A * B means C[j] = A * B[j] for each column j of B.
func matrixTimesMatrix(left, right Value) Value {
	rightCols, ok := right.(Array)
	if !ok {
		return right
	}
	result := make(Array, len(rightCols))
	for j, col := range rightCols {
		result[j] = matrixTimesVector(left, col)
	}
	return result
}

// transposeMatrix transposes a matrix stored as Array of column vectors.
func transposeMatrix(mat Value) Value {
	cols, ok := mat.(Array)
	if !ok {
		return mat
	}
	numCols := len(cols)
	if numCols == 0 {
		return mat
	}

	// Determine the number of rows from the first column.
	var numRows int
	switch cols[0].(type) {
	case Vec2:
		numRows = 2
	case Vec3:
		numRows = 3
	case Vec4:
		numRows = 4
	default:
		return mat
	}

	// Build transposed columns (each row of the original becomes a column).
	result := make(Array, numRows)
	for r := 0; r < numRows; r++ {
		var row Vec4
		for c := 0; c < numCols && c < 4; c++ {
			row[c] = toFloat32(indexComposite(cols[c], uint32(r)))
		}
		// Return the correct vector type based on column count.
		switch numCols {
		case 2:
			result[r] = Vec2{row[0], row[1]}
		case 3:
			result[r] = Vec3{row[0], row[1], row[2]}
		default:
			result[r] = row
		}
	}
	return result
}

// vectorShuffle creates a new vector by selecting components from two input vectors.
// Components are literal indices where 0..N-1 select from vec1 and N..2N-1 from vec2.
// A component value of 0xFFFFFFFF means the result component is undefined (zero).
func vectorShuffle(m *Module, typeID uint32, vec1, vec2 Value, components []uint32) Value {
	// Flatten both vectors into a combined component array.
	var pool []float32
	pool = appendComponents(pool, vec1)
	n1 := uint32(len(pool))
	pool = appendComponents(pool, vec2)
	total := uint32(len(pool))

	var out []float32
	for _, idx := range components {
		if idx == 0xFFFFFFFF || idx >= total {
			out = append(out, 0) // Undefined component.
		} else if idx < n1 {
			out = append(out, pool[idx])
		} else {
			out = append(out, pool[idx])
		}
	}

	// Determine result type from the module type info.
	ti := m.Types[typeID]
	if ti != nil && ti.Kind == TypeVector {
		switch ti.Components {
		case 2:
			var v Vec2
			for i := 0; i < 2 && i < len(out); i++ {
				v[i] = out[i]
			}
			return v
		case 3:
			var v Vec3
			for i := 0; i < 3 && i < len(out); i++ {
				v[i] = out[i]
			}
			return v
		case 4:
			var v Vec4
			for i := 0; i < 4 && i < len(out); i++ {
				v[i] = out[i]
			}
			return v
		}
	}

	// Fallback: infer from component count.
	switch len(out) {
	case 2:
		return Vec2{out[0], out[1]}
	case 3:
		return Vec3{out[0], out[1], out[2]}
	case 4:
		return Vec4{out[0], out[1], out[2], out[3]}
	default:
		return Float32(0)
	}
}

// =============================================================================
// Texture Sampling
// =============================================================================

// resolveTexture looks up a Texture2D from a value that may be a BindingKey
// or already a *Texture2D.
func (interp *interpreter) resolveTexture(val Value) *Texture2D {
	switch v := val.(type) {
	case *Texture2D:
		return v
	case BindingKey:
		if interp.ctx != nil && interp.ctx.Textures != nil {
			return interp.ctx.Textures[v]
		}
	}
	return nil
}

// resolveSampler looks up a Sampler from a value that may be a BindingKey
// or already a *Sampler.
func (interp *interpreter) resolveSampler(val Value) *Sampler {
	switch v := val.(type) {
	case *Sampler:
		return v
	case BindingKey:
		if interp.ctx != nil && interp.ctx.Samplers != nil {
			return interp.ctx.Samplers[v]
		}
	}
	return nil
}

// sampleTexture samples a texture using the given sampled image and UV coordinates.
// lod is the level-of-detail (only LOD 0 is supported).
func (interp *interpreter) sampleTexture(sampledImg Value, coord Value, lod float32) Value {
	_ = lod // LOD levels not implemented; always sample base level.

	si, ok := sampledImg.(*SampledImageValue)
	if !ok {
		return Vec4{1, 0, 1, 1} // Magenta for error.
	}

	tex := interp.resolveTexture(si.Image)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return Vec4{1, 0, 1, 1} // Magenta for missing texture.
	}

	samp := interp.resolveSampler(si.Sampler)
	if samp == nil {
		samp = &Sampler{} // Default: nearest, repeat.
	}

	// Extract UV coordinates.
	var u, v float32
	switch c := coord.(type) {
	case Vec2:
		u, v = c[0], c[1]
	case Vec3:
		u, v = c[0], c[1]
	case Vec4:
		u, v = c[0], c[1]
	default:
		u = toFloat32(coord)
	}

	// Apply wrap mode.
	u = applyWrapMode(u, samp.WrapU)
	v = applyWrapMode(v, samp.WrapV)

	// Determine filter from the magnification filter.
	filter := samp.MagFilter
	if filter == FilterLinear {
		return sampleBilinear(tex, u, v)
	}
	return sampleNearest(tex, u, v)
}

// fetchTexel fetches a single texel by integer coordinates (no filtering).
func (interp *interpreter) fetchTexel(imgVal Value, coord Value) Value {
	tex := interp.resolveTexture(imgVal)
	if tex == nil || tex.Width == 0 || tex.Height == 0 || len(tex.Data) == 0 {
		return Vec4{0, 0, 0, 0}
	}

	var x, y int
	switch c := coord.(type) {
	case Vec2:
		x, y = int(c[0]), int(c[1])
	case Vec3:
		x, y = int(c[0]), int(c[1])
	case Vec4:
		x, y = int(c[0]), int(c[1])
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

	return readTexel(tex, x, y)
}

// queryImageSize returns the size of a texture as a vec2 of uint32 values.
func (interp *interpreter) queryImageSize(imgVal Value) Value {
	tex := interp.resolveTexture(imgVal)
	if tex == nil {
		return Vec2{0, 0}
	}
	return Vec2{float32(tex.Width), float32(tex.Height)}
}

// applyWrapMode wraps a texture coordinate according to the specified mode.
// The result is in [0, 1) for repeat modes and [0, 1] for clamp.
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
		// Mirror: period is 2.0; within [0,1] normal, [1,2] reflected.
		coord = float32(math.Abs(float64(coord)))
		period := int(coord)
		frac := coord - float32(period)
		if period%2 != 0 {
			return 1 - frac
		}
		return frac

	default: // WrapRepeat
		coord = coord - float32(int(coord))
		if coord < 0 {
			coord += 1
		}
		return coord
	}
}

// sampleNearest samples a texture at the given UV using nearest-neighbor filtering.
func sampleNearest(tex *Texture2D, u, v float32) Vec4 {
	x := int(u * float32(tex.Width))
	y := int(v * float32(tex.Height))

	// Clamp to valid pixel range.
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
	// Map UV to texel space (center of pixel 0 is at 0.5/width).
	fx := u*float32(tex.Width) - 0.5
	fy := v*float32(tex.Height) - 0.5

	x0 := int(math.Floor(float64(fx)))
	y0 := int(math.Floor(float64(fy)))
	x1 := x0 + 1
	y1 := y0 + 1

	// Fractional part for interpolation.
	fracX := fx - float32(x0)
	fracY := fy - float32(y0)

	// Clamp coordinates.
	w := int(tex.Width)
	h := int(tex.Height)
	x0 = clampInt(x0, 0, w-1)
	y0 = clampInt(y0, 0, h-1)
	x1 = clampInt(x1, 0, w-1)
	y1 = clampInt(y1, 0, h-1)

	// Fetch the four texels.
	c00 := readTexel(tex, x0, y0)
	c10 := readTexel(tex, x1, y0)
	c01 := readTexel(tex, x0, y1)
	c11 := readTexel(tex, x1, y1)

	// Bilinear interpolation.
	var result Vec4
	for i := 0; i < 4; i++ {
		top := c00[i]*(1-fracX) + c10[i]*fracX
		bot := c01[i]*(1-fracX) + c11[i]*fracX
		result[i] = top*(1-fracY) + bot*fracY
	}
	return result
}

// readTexel reads a single RGBA texel from the texture at pixel coordinates (x, y).
// Returns normalized [0,1] float values.
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

// clampInt clamps an integer to [min, max].
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
