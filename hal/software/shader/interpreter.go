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

	// Initialize interface variables (inputs/outputs).
	interp.initVariables(inputs)

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

// interpreter holds execution state for a single entry point invocation.
type interpreter struct {
	module *Module
	ep     *EntryPoint
	fn     *Function
	values map[uint32]Value
	labels map[uint32]int // label ID -> instruction index
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
			if ptr, ok := interp.values[ptrID].(*Pointer); ok {
				interp.values[inst.ResultID] = ptr.Value
			} else {
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
			if ptr, ok := interp.values[ptrID].(*Pointer); ok {
				ptr.Value = interp.values[valID]
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
			// For void-returning entry points this shouldn't happen,
			// but handle it gracefully.
			return nil

		case OpBranch:
			// OpBranch: target label
			if len(inst.Operands) < 1 {
				break
			}
			targetLabel := inst.Operands[0]
			if idx, ok := interp.labels[targetLabel]; ok {
				pc = idx + 1 // +1 to skip the label instruction itself
			}

		case OpFunctionParameter, OpFunctionEnd:
			// No-op during execution.

		case OpSelectionMerge, OpLoopMerge:
			// Structured control flow hints — no-op for the interpreter.

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
			// Phi nodes are not needed for the triangle shader (single basic block per branch).
			// For safety, treat as no-op and let the value remain from whichever predecessor ran.

		default:
			// Unknown opcodes are skipped. The triangle shader uses a small subset.
		}
	}

	return nil
}

// accessChain navigates into a composite value through a chain of indexes,
// returning a Pointer to the innermost element.
func (interp *interpreter) accessChain(baseID uint32, indexes []uint32) *Pointer {
	baseVal := interp.values[baseID]

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
