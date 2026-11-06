//go:build !(js && wasm)

package shader

import "math"

// Interface variables (vertex attributes, varyings) cross the rasterizer as
// []float32 lanes, one per component. A lane is the 32-bit word of its
// component viewed as a float32: a float lane is its value, an integer or
// bool lane the bit pattern of its word (math.Float32frombits), never
// float32(int). Every u32 and i32 crosses exactly (ADR-078 rule 7), as long as
// nothing does arithmetic on the lane: the rasterizer copies the lanes of a
// variable that InterpolatesFlat reports from the provoking vertex.

// InterfaceValue returns the value of interface variable varID from its
// lanes, by the variable's type. Lanes past the type's components are
// ignored; missing lanes are zero. A variable without a 32-bit scalar or
// vector type takes up to four float lanes.
func (m *Module) InterfaceValue(varID uint32, lanes []float32) Value {
	s := m.varShape(varID)
	if s == 0 {
		s = shapeOf(kindFloat, max(min(len(lanes), 4), 1))
	}
	var w [4]uint32
	for i := range min(s.lanes(), len(lanes)) {
		w[i] = math.Float32bits(lanes[i])
	}
	return wordsValue(s, w)
}

// AppendInterfaceLanes appends the lanes of the interface value v to dst, the
// inverse of InterfaceValue. A value that is not a 32-bit scalar or vector
// appends one zero lane.
func AppendInterfaceLanes(dst []float32, v Value) []float32 {
	n := vectorLanes(v.Tag)
	switch {
	case n == 0:
		return append(dst, 0)
	case vectorKind(v.Tag) == kindFloat:
		return append(dst, v.F[:n]...)
	}
	for _, w := range v.U[:n] {
		dst = append(dst, math.Float32frombits(w))
	}
	return dst
}

// InterpolatesFlat reports whether interface variable varID is not
// interpolated: it is decorated Flat, or it has an integer or bool type.
// WGSL requires @interpolate(flat) on integer user-defined inputs and
// outputs, and SPIR-V Flat on integer fragment inputs; the integer check
// also covers a module that leaves the decoration out.
func (m *Module) InterpolatesFlat(varID uint32) bool {
	if _, ok := m.Decorations[decorationKey{TargetID: varID, Decoration: DecorationFlat}]; ok {
		return true
	}
	k := m.varShape(varID).kind()
	return k == kindUint || k == kindSint || k == kindBool
}

// varShape returns the shape of the type interface variable varID points to.
func (m *Module) varShape(varID uint32) shape {
	vi, ok := m.Variables[varID]
	if !ok {
		return 0
	}
	ptr := m.Types[vi.TypeID]
	if ptr == nil || ptr.Kind != TypePointer {
		return 0
	}
	return m.typeShape(ptr.ElemType)
}
