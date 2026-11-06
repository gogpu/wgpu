//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"
	"testing"
	"unsafe"
)

// Type IDs of tagTableModule: the four 32-bit scalars, then base+n for the
// n-lane vector of each.
const (
	ttF32  = 1
	ttU32  = 2
	ttI32  = 3
	ttBool = 4
	ttVecF = 10
	ttVecU = 20
	ttVecI = 30
	ttVecB = 40
)

// tagTableModule returns a module that declares every 32-bit scalar and
// vector type, for the boundary functions that read by type.
func tagTableModule() *Module {
	m := &Module{Types: map[uint32]*TypeInfo{
		ttF32:  {Kind: TypeFloat, Width: 32},
		ttU32:  {Kind: TypeInt, Width: 32},
		ttI32:  {Kind: TypeInt, Width: 32, Signed: true},
		ttBool: {Kind: TypeBool},
	}}
	for n := uint32(2); n <= 4; n++ {
		m.Types[ttVecF+n] = &TypeInfo{Kind: TypeVector, ElemType: ttF32, Components: n}
		m.Types[ttVecU+n] = &TypeInfo{Kind: TypeVector, ElemType: ttU32, Components: n}
		m.Types[ttVecI+n] = &TypeInfo{Kind: TypeVector, ElemType: ttI32, Components: n}
		m.Types[ttVecB+n] = &TypeInfo{Kind: TypeVector, ElemType: ttBool, Components: n}
	}
	return m
}

// tagCase is a value of one tag and, for a 32-bit scalar or vector, the type
// it is read back as and the words it is stored as.
type tagCase struct {
	val    Value
	typeID uint32   // 0: not a buffer value
	words  []uint32 // stored words, one per lane
}

// tagTable returns one case for every tag. The lanes use values that a
// float32 cannot hold exactly (above 2^24, negative, the top bit), so a
// function that converts a word lane to float loses them.
func tagTable() map[ValueTag]tagCase {
	big := [4]uint32{1<<24 + 1, 0xFFFFFFFF, 0x80000001, 7}
	signed := [4]int32{-(1<<24 + 1), math.MinInt32, math.MaxInt32, -7}
	sbits := [4]uint32{uint32(signed[0]), uint32(signed[1]), uint32(signed[2]), uint32(signed[3])}
	bools := [4]bool{true, false, true, true}
	f := [4]float32{1.5, -2.25, 3e30, 0.125}
	fbits := [4]uint32{math.Float32bits(f[0]), math.Float32bits(f[1]), math.Float32bits(f[2]), math.Float32bits(f[3])}
	p := &Pointer{Val: ValUint(3)}
	return map[ValueTag]tagCase{
		TagNone:          {val: ValNone()},
		TagFloat32:       {val: ValFloat(f[0]), typeID: ttF32, words: fbits[:1]},
		TagUint32:        {val: ValUint(big[0]), typeID: ttU32, words: big[:1]},
		TagInt32:         {val: ValInt(signed[0]), typeID: ttI32, words: sbits[:1]},
		TagBool:          {val: ValBool(true), typeID: ttBool, words: []uint32{1}},
		TagVec2:          {val: ValVec2(f[0], f[1]), typeID: ttVecF + 2, words: fbits[:2]},
		TagVec3:          {val: ValVec3(f[0], f[1], f[2]), typeID: ttVecF + 3, words: fbits[:3]},
		TagVec4:          {val: ValVec4From(f), typeID: ttVecF + 4, words: fbits[:4]},
		TagArray:         {val: ValArray([]Value{ValUint(1), ValUint(2)})},
		TagPointer:       {val: ValPointer(p)},
		TagSubPointer:    {val: ValSubPointer(&SubPointer{Parent: p})},
		TagBufferPointer: {val: ValBufferPointer(&BufferPointer{Buffer: make([]byte, 4)})},
		TagSampledImage:  {val: ValSampledImage(&SampledImageValue{})},
		TagBindingKey:    {val: ValBindingKey(BindingKey{Group: 1, Binding: 2})},
		TagFloat64:       {val: ValFloat64(math.Pi)},
		TagUVec2:         {val: ValUVec(2, big), typeID: ttVecU + 2, words: big[:2]},
		TagUVec3:         {val: ValUVec(3, big), typeID: ttVecU + 3, words: big[:3]},
		TagUVec4:         {val: ValUVec(4, big), typeID: ttVecU + 4, words: big[:4]},
		TagIVec2:         {val: ValIVec(2, signed), typeID: ttVecI + 2, words: sbits[:2]},
		TagIVec3:         {val: ValIVec(3, signed), typeID: ttVecI + 3, words: sbits[:3]},
		TagIVec4:         {val: ValIVec(4, signed), typeID: ttVecI + 4, words: sbits[:4]},
		TagBVec2:         {val: ValBVec(2, bools), typeID: ttVecB + 2, words: []uint32{1, 0}},
		TagBVec3:         {val: ValBVec(3, bools), typeID: ttVecB + 3, words: []uint32{1, 0, 1}},
		TagBVec4:         {val: ValBVec(4, bools), typeID: ttVecB + 4, words: []uint32{1, 0, 1, 1}},
	}
}

// TestValueTagTable runs a value of every tag through each boundary
// function: a tag one of them drops (writes nothing, sizes wrong, prints
// nothing, loses a lane) fails here. A new tag without a table entry fails
// too.
func TestValueTagTable(t *testing.T) {
	table := tagTable()
	m := tagTableModule()
	interp := &interpreter{module: m}
	for tag := TagNone; tag < tagCount; tag++ {
		c, ok := table[tag]
		if !ok {
			t.Errorf("tag %d has no tag table entry", tag)
			continue
		}
		if c.val.Tag != tag {
			t.Errorf("tag %d: the table's value has tag %d", tag, c.val.Tag)
			continue
		}
		checkClone(t, c)
		if c.typeID == 0 {
			continue
		}
		checkLanes(t, c)
		checkBufferRoundTrip(t, interp, m, c)
		checkPrinted(t, c)
	}
}

func checkClone(t *testing.T, c tagCase) {
	t.Helper()
	got := cloneValue(c.val)
	if got.Tag != c.val.Tag || got.F != c.val.F || got.U != c.val.U {
		t.Errorf("tag %d: cloneValue = %+v, want %+v", c.val.Tag, got, c.val)
	}
}

// checkLanes checks the lane tables, valueByteSize, indexComposite and
// setCompositeElement of a 32-bit scalar or vector.
func checkLanes(t *testing.T, c tagCase) {
	t.Helper()
	tag, n := c.val.Tag, len(c.words)
	if vectorLanes(tag) != n || c.val.Lanes() != n {
		t.Errorf("tag %d: vectorLanes = %d, want %d", tag, vectorLanes(tag), n)
	}
	s := shapeOf(vectorKind(tag), n)
	if s.tag() != tag || s.lanes() != n || s.kind() != vectorKind(tag) {
		t.Errorf("tag %d: shape %d gives tag %d, %d lanes", tag, s, s.tag(), s.lanes())
	}
	if got := valueByteSize(c.val); got != uint32(4*n) {
		t.Errorf("tag %d: valueByteSize = %d, want %d", tag, got, 4*n)
	}
	if n == 1 {
		return
	}
	for i, w := range c.words {
		e := indexComposite(c.val, uint32(i))
		if laneWord(vectorKind(tag), e) != w {
			t.Errorf("tag %d: lane %d = %+v, want word %#x", tag, i, e, w)
		}
		// Store the last lane's value into lane i and read it back.
		last := indexComposite(c.val, uint32(n-1))
		set := setCompositeElement(c.val, []uint32{uint32(i)}, last)
		if got := indexComposite(set, uint32(i)); laneWord(vectorKind(tag), got) != c.words[n-1] {
			t.Errorf("tag %d: setCompositeElement lane %d = %+v, want word %#x", tag, i, got, c.words[n-1])
		}
	}
}

// checkBufferRoundTrip writes the value to bytes, untyped and typed, checks
// the stored words and reads it back by its type.
func checkBufferRoundTrip(t *testing.T, interp *interpreter, m *Module, c tagCase) {
	t.Helper()
	tag := c.val.Tag
	for _, typed := range []bool{false, true} {
		buf := make([]byte, 16)
		if typed {
			interp.writeValueToBufferTyped(buf, 0, c.val, m.Types[c.typeID], 0)
		} else {
			writeValueToBuffer(buf, 0, c.val)
		}
		for i, w := range c.words {
			if got := binary.LittleEndian.Uint32(buf[4*i:]); got != w {
				t.Errorf("tag %d (typed %v): stored word %d = %#x, want %#x", tag, typed, i, got, w)
			}
		}
	}
	buf := make([]byte, 16)
	writeValueToBuffer(buf, 0, c.val)
	got := interp.readValueFromBuffer(buf, 0, m.Types[c.typeID], 0)
	want := c.val
	if isWordVector(tag) {
		// Buffer loads still build integer and bool vectors as float
		// vectors: they become word vectors with the producers (ADR-078
		// step 3). Until then a load is exact only up to 2^24, and a bool
		// lane (workgroup memory) reads as 0.
		want = Value{Tag: vectorTagOf(vectorLanes(tag))}
		if vectorKind(tag) != kindBool {
			want.F = Vec4ToFloat32(c.val)
		}
	}
	if got.Tag != want.Tag || got.F != want.F || got.U != want.U {
		t.Errorf("tag %d: read back %+v, want %+v", tag, got, want)
	}
}

// vectorTagOf is vectorTag for the float form of a word vector in
// checkBufferRoundTrip.
func vectorTagOf(n int) ValueTag { return vectorTag(n) }

// checkPrinted checks that the debugger prints the value and names its type.
func checkPrinted(t *testing.T, c tagCase) {
	t.Helper()
	if formatTraceValue(c.val) == nil {
		t.Errorf("tag %d: formatTraceValue = nil", c.val.Tag)
	}
	if valueTypeName(c.val) == "" {
		t.Errorf("tag %d: valueTypeName is empty", c.val.Tag)
	}
	if !valuesEqual(c.val, cloneValue(c.val)) {
		t.Errorf("tag %d: valuesEqual(v, clone) = false", c.val.Tag)
	}
}

// TestTypeShapes checks the shape ParseModule precomputes for each result
// type, and that the shape byte fits in Instruction's padding.
func TestTypeShapes(t *testing.T) {
	if got := unsafe.Sizeof(Instruction{}); got != 40 && unsafe.Sizeof(uintptr(0)) == 8 {
		t.Errorf("Instruction is %d bytes, want 40", got)
	}
	m := tagTableModule()
	m.Types[90] = &TypeInfo{Kind: TypeFloat, Width: 16}
	m.Types[91] = &TypeInfo{Kind: TypeVector, ElemType: 90, Components: 2}
	m.Types[92] = &TypeInfo{Kind: TypeMatrix, ElemType: ttVecF + 2, Components: 2}
	m.Types[93] = &TypeInfo{Kind: TypeFloat, Width: 64}
	tests := []struct {
		typeID uint32
		want   ValueTag
	}{
		{0, TagNone}, {ttF32, TagFloat32}, {ttU32, TagUint32}, {ttI32, TagInt32}, {ttBool, TagBool},
		{ttVecF + 3, TagVec3}, {ttVecU + 2, TagUVec2}, {ttVecI + 4, TagIVec4}, {ttVecB + 3, TagBVec3},
		{90, TagNone}, {91, TagNone}, {92, TagNone}, {93, TagNone}, {999, TagNone},
	}
	for _, tt := range tests {
		if got := m.typeShape(tt.typeID).tag(); got != tt.want {
			t.Errorf("typeShape(%d) gives tag %d, want %d", tt.typeID, got, tt.want)
		}
	}
	if got := laneValue(kindUint, 5, [4]uint32{1}); got.Tag != TagNone {
		t.Errorf("laneValue with 5 lanes = %+v, want none", got)
	}
	if got := ValUVec(1, [4]uint32{9, 8}); got != ValUint(9) {
		t.Errorf("ValUVec(1) = %+v, want a u32 scalar", got)
	}
}
