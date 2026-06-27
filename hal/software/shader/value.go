//go:build !(js && wasm)

// SPIR-V runtime value types for the software backend interpreter.
//
// Values are stored as a tagged union struct instead of Go interfaces.
// This eliminates heap allocations for scalars and vectors (the hot path
// in per-pixel shading), keeping all data inline in a fixed-size struct
// that fits in a single cache line.

package shader

import (
	"math"
	"unsafe"
)

// ValueTag identifies the concrete type stored in a Value.
type ValueTag byte

const (
	TagNone          ValueTag = iota
	TagFloat32                // scalar float32 in F[0]
	TagUint32                 // scalar uint32 in U[0]
	TagInt32                  // scalar int32 (stored as uint32 in U[0])
	TagBool                   // boolean (U[0]: 0=false, 1=true)
	TagVec2                   // [2]float32 in F[0..1]
	TagVec3                   // [3]float32 in F[0..2]
	TagVec4                   // [4]float32 in F[0..3]
	TagArray                  // []Value stored via Ref
	TagPointer                // *Pointer stored via Ref
	TagSubPointer             // *SubPointer stored via Ref
	TagBufferPointer          // *BufferPointer stored via Ref
	TagSampledImage           // *SampledImageValue stored via Ref
	TagBindingKey             // BindingKey stored in U[0..1]
)

// Value is a tagged union representing a runtime value in the SPIR-V interpreter.
// Scalars and vectors (up to vec4) are stored inline with zero heap allocation.
// Composite types (arrays, pointers) use the Ref field.
//
// Size: 40 bytes (1 tag + 3 pad + 16 F + 16 U + 8 Ref = 44, but compiler may align to 48).
// This is intentionally larger than an interface (16 bytes) because it eliminates
// the heap allocation that every interface boxing requires.
type Value struct {
	Tag ValueTag
	F   [4]float32     // float scalars + vectors
	U   [4]uint32      // uint/int scalars, bool, binding key
	Ref unsafe.Pointer // Array, Pointer, SubPointer, BufferPointer, SampledImageValue
}

// Type aliases kept for backward compatibility with external code and tests.
// These are the underlying Go types used for construction only.
type (
	Float32 = float32
	Uint32  = uint32
	Int32   = int32
	Vec2    = [2]float32
	Vec3    = [3]float32
	Vec4    = [4]float32
	Array   = []Value
)

// Pointer wraps a mutable reference to a Value stored in a variable.
// SPIR-V OpVariable creates a Pointer; OpLoad dereferences it; OpStore writes to it.
type Pointer struct {
	Val Value
}

// Mat4 is a 4x4 column-major matrix stored as an Array of 4 Vec4 columns.
// SPIR-V represents matrices as arrays of column vectors.
type Mat4 = [4]Vec4

// Texture2D represents a 2D texture for sampling in the SPIR-V interpreter.
type Texture2D struct {
	Width  uint32
	Height uint32
	Data   []byte // Pixel data, row-major, BytesPerPixel bytes per texel.
	Format uint32 // Texture format identifier (0 = RGBA8).
	// BytesPerPixel is the storage size of a single texel (1 for R8, 2 for
	// RG8/R16, 4 for RGBA8/BGRA8). When zero, sampling assumes 4 (RGBA8) for
	// backward compatibility. Single-channel formats unpack as (r, 0, 0, 1).
	BytesPerPixel uint32
}

// Sampler describes texture sampling parameters.
type Sampler struct {
	MinFilter uint32 // 0 = Nearest, 1 = Linear.
	MagFilter uint32 // 0 = Nearest, 1 = Linear.
	WrapU     uint32 // 0 = Repeat, 1 = ClampToEdge, 2 = MirroredRepeat.
	WrapV     uint32 // 0 = Repeat, 1 = ClampToEdge, 2 = MirroredRepeat.
}

// Wrap mode constants.
const (
	WrapRepeat         = 0
	WrapClampToEdge    = 1
	WrapMirroredRepeat = 2
)

// Filter constants.
const (
	FilterNearest = 0
	FilterLinear  = 1
)

// BufferPointer provides direct read/write access to a position within a raw byte buffer.
// Used for storage buffer access chains where OpStore must write back to the buffer.
type BufferPointer struct {
	Buffer []byte    // raw buffer data
	Offset uint32    // byte offset within the buffer
	Type   *TypeInfo // type of the pointed-to element
}

// SubPointer provides write-through access to a member of a composite value
// stored in a parent Pointer. When SPIR-V uses OpAccessChain on a function-local
// variable (e.g. var p: Particle; p.pos), OpStore to the resulting pointer must
// update the parent composite -- not just a disconnected copy.
//
// Without SubPointer, accessChain creates a new Pointer wrapping a copy of the
// sub-element, so OpStore modifies the copy but not the original struct. This
// bug caused compute shaders with struct member updates (p.pos += ...) to
// silently discard writes.
//
// SubPointer solves this by storing a reference to the parent Pointer and the
// index path. OpLoad reads the current value through the parent. OpStore
// writes back through the parent, rebuilding the composite at each level.
type SubPointer struct {
	Parent  *Pointer // root variable pointer
	Indexes []uint32 // index path from parent to sub-element (literal values, not SSA IDs)
}

// SampledImageValue combines a texture reference and a sampler reference.
// Created by OpSampledImage, consumed by OpImageSample* opcodes.
type SampledImageValue struct {
	Image   Value // BindingKey or *Texture2D (stored as Value)
	Sampler Value // BindingKey or *Sampler (stored as Value)
}

// ExecutionContext provides bound resources for shader execution.
// Shaders read uniform/storage buffers, textures, and samplers through this context.
type ExecutionContext struct {
	// Inputs maps variable ID to its value (builtin inputs like vertex_index).
	Inputs map[uint32]Value

	// Buffers maps (group, binding) to raw buffer data for uniform/storage buffers.
	Buffers map[BindingKey][]byte

	// Textures maps (group, binding) to a 2D texture.
	Textures map[BindingKey]*Texture2D

	// Samplers maps (group, binding) to sampler parameters.
	Samplers map[BindingKey]*Sampler

	// WorkgroupSharedMemory is shared memory for compute shader workgroups.
	// Maps variable ID to a byte slice shared across all invocations.
	WorkgroupSharedMemory map[uint32][]byte

	// ComputeBuiltins provides compute shader built-in values.
	GlobalInvocationID   [3]uint32
	LocalInvocationID    [3]uint32
	WorkgroupID          [3]uint32
	NumWorkgroups        [3]uint32
	WorkgroupSize        [3]uint32
	LocalInvocationIndex uint32
}

// =============================================================================
// Value Constructors -- zero allocation for scalars and vectors
// =============================================================================

// ValNone returns the zero/nil Value.
func ValNone() Value { return Value{} }

// ValFloat returns a float32 scalar Value.
func ValFloat(f float32) Value { return Value{Tag: TagFloat32, F: [4]float32{f}} }

// ValUint returns a uint32 scalar Value.
func ValUint(u uint32) Value { return Value{Tag: TagUint32, U: [4]uint32{u}} }

// ValInt returns an int32 scalar Value.
func ValInt(i int32) Value { return Value{Tag: TagInt32, U: [4]uint32{uint32(i)}} }

// ValBool returns a boolean Value.
func ValBool(b bool) Value {
	v := Value{Tag: TagBool}
	if b {
		v.U[0] = 1
	}
	return v
}

// ValVec2 returns a 2-component float vector Value.
func ValVec2(x, y float32) Value { return Value{Tag: TagVec2, F: [4]float32{x, y}} }

// ValVec3 returns a 3-component float vector Value.
func ValVec3(x, y, z float32) Value { return Value{Tag: TagVec3, F: [4]float32{x, y, z}} }

// ValVec4 returns a 4-component float vector Value.
func ValVec4(x, y, z, w float32) Value { return Value{Tag: TagVec4, F: [4]float32{x, y, z, w}} }

// ValVec2From returns a Value from a Vec2 array.
func ValVec2From(v Vec2) Value { return Value{Tag: TagVec2, F: [4]float32{v[0], v[1]}} }

// ValVec3From returns a Value from a Vec3 array.
func ValVec3From(v Vec3) Value { return Value{Tag: TagVec3, F: [4]float32{v[0], v[1], v[2]}} }

// ValVec4From returns a Value from a Vec4 array.
func ValVec4From(v Vec4) Value { return Value{Tag: TagVec4, F: [4]float32{v[0], v[1], v[2], v[3]}} }

// ValArray returns an Array Value. The slice is stored on the heap via Ref.
func ValArray(a []Value) Value {
	return Value{Tag: TagArray, Ref: unsafe.Pointer(&a)}
}

// ValPointer returns a Pointer Value.
func ValPointer(p *Pointer) Value {
	return Value{Tag: TagPointer, Ref: unsafe.Pointer(p)}
}

// ValSubPointer returns a SubPointer Value.
func ValSubPointer(sp *SubPointer) Value {
	return Value{Tag: TagSubPointer, Ref: unsafe.Pointer(sp)}
}

// ValBufferPointer returns a BufferPointer Value.
func ValBufferPointer(bp *BufferPointer) Value {
	return Value{Tag: TagBufferPointer, Ref: unsafe.Pointer(bp)}
}

// ValSampledImage returns a SampledImageValue.
func ValSampledImage(si *SampledImageValue) Value {
	return Value{Tag: TagSampledImage, Ref: unsafe.Pointer(si)}
}

// ValBindingKey returns a BindingKey Value stored inline in U[0..1].
func ValBindingKey(bk BindingKey) Value {
	return Value{Tag: TagBindingKey, U: [4]uint32{bk.Group, bk.Binding}}
}

// =============================================================================
// Value Accessors
// =============================================================================

// IsNone returns true if the value is uninitialized/nil.
func (v Value) IsNone() bool { return v.Tag == TagNone }

// AsFloat32 extracts a float32. Returns 0 for non-float tags.
func (v Value) AsFloat32() float32 { return v.F[0] }

// AsUint32 extracts a uint32. Returns 0 for non-uint tags.
func (v Value) AsUint32() uint32 { return v.U[0] }

// AsInt32 extracts an int32. Returns 0 for non-int tags.
func (v Value) AsInt32() int32 { return int32(v.U[0]) }

// AsBool extracts a bool. Returns false for non-bool tags.
func (v Value) AsBool() bool { return v.U[0] != 0 }

// AsVec2 extracts a [2]float32.
func (v Value) AsVec2() Vec2 { return Vec2{v.F[0], v.F[1]} }

// AsVec3 extracts a [3]float32.
func (v Value) AsVec3() Vec3 { return Vec3{v.F[0], v.F[1], v.F[2]} }

// AsVec4 extracts a [4]float32.
func (v Value) AsVec4() Vec4 { return v.F }

// AsArray extracts a []Value from the Ref field.
func (v Value) AsArray() []Value {
	if v.Ref == nil {
		return nil
	}
	return *(*[]Value)(v.Ref)
}

// AsPointer extracts a *Pointer from the Ref field.
func (v Value) AsPointer() *Pointer {
	if v.Ref == nil {
		return nil
	}
	return (*Pointer)(v.Ref)
}

// AsSubPointer extracts a *SubPointer from the Ref field.
func (v Value) AsSubPointer() *SubPointer {
	if v.Ref == nil {
		return nil
	}
	return (*SubPointer)(v.Ref)
}

// AsBufferPointer extracts a *BufferPointer from the Ref field.
func (v Value) AsBufferPointer() *BufferPointer {
	if v.Ref == nil {
		return nil
	}
	return (*BufferPointer)(v.Ref)
}

// AsSampledImage extracts a *SampledImageValue from the Ref field.
func (v Value) AsSampledImage() *SampledImageValue {
	if v.Ref == nil {
		return nil
	}
	return (*SampledImageValue)(v.Ref)
}

// AsBindingKey extracts a BindingKey from U[0..1].
func (v Value) AsBindingKey() BindingKey {
	return BindingKey{Group: v.U[0], Binding: v.U[1]}
}

// =============================================================================
// Conversion helpers -- used throughout the interpreter
// =============================================================================

// toFloat32 extracts a float32 from a Value, converting if needed.
func toFloat32(v Value) float32 {
	switch v.Tag {
	case TagFloat32:
		return v.F[0]
	case TagUint32:
		return float32(v.U[0])
	case TagInt32:
		return float32(int32(v.U[0]))
	default:
		return 0
	}
}

// toUint32 extracts a uint32 from a Value, converting if needed.
func toUint32(v Value) uint32 {
	switch v.Tag {
	case TagUint32:
		return v.U[0]
	case TagInt32:
		return v.U[0] // same bits
	case TagFloat32:
		return uint32(v.F[0])
	case TagBool:
		return v.U[0]
	default:
		return 0
	}
}

// toBool converts a Value to boolean.
func toBool(v Value) bool {
	switch v.Tag {
	case TagBool:
		return v.U[0] != 0
	case TagUint32, TagInt32:
		return v.U[0] != 0
	case TagFloat32:
		return v.F[0] != 0
	default:
		return false
	}
}

// =============================================================================
// Arithmetic / vector helpers
// =============================================================================

// floatBinOp applies a binary operation to two float-typed values.
// Works on scalars and vectors component-wise.
func floatBinOp(a, b Value, op func(float32, float32) float32) Value {
	switch a.Tag {
	case TagFloat32:
		return ValFloat(op(a.F[0], toFloat32(b)))
	case TagVec2:
		return ValVec2(op(a.F[0], b.F[0]), op(a.F[1], b.F[1]))
	case TagVec3:
		return ValVec3(op(a.F[0], b.F[0]), op(a.F[1], b.F[1]), op(a.F[2], b.F[2]))
	case TagVec4:
		return ValVec4(op(a.F[0], b.F[0]), op(a.F[1], b.F[1]), op(a.F[2], b.F[2]), op(a.F[3], b.F[3]))
	default:
		return ValFloat(0)
	}
}

// floatUnaryOp applies a unary operation to a float-typed value.
func floatUnaryOp(a Value, op func(float32) float32) Value {
	switch a.Tag {
	case TagFloat32:
		return ValFloat(op(a.F[0]))
	case TagVec2:
		return ValVec2(op(a.F[0]), op(a.F[1]))
	case TagVec3:
		return ValVec3(op(a.F[0]), op(a.F[1]), op(a.F[2]))
	case TagVec4:
		return ValVec4(op(a.F[0]), op(a.F[1]), op(a.F[2]), op(a.F[3]))
	default:
		return ValFloat(0)
	}
}

// intBinOp applies a binary operation to two integer values.
func intBinOp(a, b Value, op func(uint32, uint32) uint32) Value {
	return ValUint(op(toUint32(a), toUint32(b)))
}

// vectorTimesScalar multiplies each component of a vector by a scalar.
func vectorTimesScalar(vec Value, s float32) Value {
	switch vec.Tag {
	case TagVec2:
		return ValVec2(vec.F[0]*s, vec.F[1]*s)
	case TagVec3:
		return ValVec3(vec.F[0]*s, vec.F[1]*s, vec.F[2]*s)
	case TagVec4:
		return ValVec4(vec.F[0]*s, vec.F[1]*s, vec.F[2]*s, vec.F[3]*s)
	default:
		return ValFloat(toFloat32(vec) * s)
	}
}

// dotProduct computes the dot product of two vectors.
func dotProduct(a, b Value) Value {
	switch a.Tag {
	case TagVec2:
		return ValFloat(a.F[0]*b.F[0] + a.F[1]*b.F[1])
	case TagVec3:
		return ValFloat(a.F[0]*b.F[0] + a.F[1]*b.F[1] + a.F[2]*b.F[2])
	case TagVec4:
		return ValFloat(a.F[0]*b.F[0] + a.F[1]*b.F[1] + a.F[2]*b.F[2] + a.F[3]*b.F[3])
	default:
		return ValFloat(toFloat32(a) * toFloat32(b))
	}
}

// Vec4ToFloat32 extracts a Vec4 from a Value, returning zeros if type doesn't match.
func Vec4ToFloat32(val Value) [4]float32 {
	switch val.Tag {
	case TagVec4:
		return val.F
	case TagVec3:
		return [4]float32{val.F[0], val.F[1], val.F[2], 0}
	case TagVec2:
		return [4]float32{val.F[0], val.F[1], 0, 0}
	case TagFloat32:
		return [4]float32{val.F[0], 0, 0, 0}
	default:
		return [4]float32{}
	}
}

// Float32BitsToUint32 converts a float32 to its bit representation as uint32.
// Used for SPIR-V constant encoding in tests.
func Float32BitsToUint32(f float32) uint32 {
	return math.Float32bits(f)
}

// convertToFloat converts an unsigned integer value to float32.
func convertToFloat(val Value) Value {
	switch val.Tag {
	case TagUint32:
		return ValFloat(float32(val.U[0]))
	case TagInt32:
		return ValFloat(float32(int32(val.U[0])))
	case TagFloat32:
		return val
	default:
		return ValFloat(0)
	}
}

// convertSignedToFloat converts a signed integer value to float32.
func convertSignedToFloat(val Value) Value {
	switch val.Tag {
	case TagInt32:
		return ValFloat(float32(int32(val.U[0])))
	case TagUint32:
		return ValFloat(float32(int32(val.U[0])))
	case TagFloat32:
		return val
	default:
		return ValFloat(0)
	}
}

// convertFloatToUint converts a float value to uint32.
func convertFloatToUint(val Value) Value {
	switch val.Tag {
	case TagFloat32:
		return ValUint(uint32(val.F[0]))
	case TagUint32:
		return val
	case TagInt32:
		return ValUint(val.U[0])
	default:
		return ValUint(0)
	}
}

// =============================================================================
// Composite operations
// =============================================================================

// indexComposite extracts element at the given index from a composite value.
func indexComposite(composite Value, index uint32) Value {
	switch composite.Tag {
	case TagArray:
		arr := composite.AsArray()
		if int(index) < len(arr) {
			return arr[index]
		}
	case TagVec2:
		if index < 2 {
			return ValFloat(composite.F[index])
		}
	case TagVec3:
		if index < 3 {
			return ValFloat(composite.F[index])
		}
	case TagVec4:
		if index < 4 {
			return ValFloat(composite.F[index])
		}
	}
	return ValFloat(0)
}

// setCompositeElement returns a copy of composite with the element at the
// given index path replaced by val. Handles Array, Vec2, Vec3, Vec4 composites.
func setCompositeElement(composite Value, indexes []uint32, val Value) Value {
	if len(indexes) == 0 {
		return val
	}
	idx := indexes[0]
	rest := indexes[1:]

	switch composite.Tag {
	case TagArray:
		arr := composite.AsArray()
		if int(idx) >= len(arr) {
			return composite
		}
		newArr := make([]Value, len(arr))
		copy(newArr, arr)
		if len(rest) == 0 {
			newArr[idx] = val
		} else {
			newArr[idx] = setCompositeElement(arr[idx], rest, val)
		}
		return ValArray(newArr)

	case TagVec2:
		if idx >= 2 {
			return composite
		}
		v := composite
		if len(rest) == 0 {
			v.F[idx] = toFloat32(val)
		}
		return v

	case TagVec3:
		if idx >= 3 {
			return composite
		}
		v := composite
		if len(rest) == 0 {
			v.F[idx] = toFloat32(val)
		}
		return v

	case TagVec4:
		if idx >= 4 {
			return composite
		}
		v := composite
		if len(rest) == 0 {
			v.F[idx] = toFloat32(val)
		}
		return v

	default:
		return composite
	}
}

// appendComponents flattens a value into float32 components.
func appendComponents(dst []float32, val Value) []float32 {
	switch val.Tag {
	case TagFloat32:
		return append(dst, val.F[0])
	case TagVec2:
		return append(dst, val.F[0], val.F[1])
	case TagVec3:
		return append(dst, val.F[0], val.F[1], val.F[2])
	case TagVec4:
		return append(dst, val.F[0], val.F[1], val.F[2], val.F[3])
	case TagUint32:
		return append(dst, float32(val.U[0]))
	case TagInt32:
		return append(dst, float32(int32(val.U[0])))
	default:
		return append(dst, 0)
	}
}

// writeValueToBuffer serializes a SPIR-V typed value into raw buffer bytes.
// Used for storage buffer writes via OpStore.
func writeValueToBuffer(data []byte, offset uint32, val Value) {
	switch val.Tag {
	case TagFloat32:
		if offset+4 > uint32(len(data)) {
			return
		}
		bits := math.Float32bits(val.F[0])
		data[offset] = byte(bits)
		data[offset+1] = byte(bits >> 8)
		data[offset+2] = byte(bits >> 16)
		data[offset+3] = byte(bits >> 24)
	case TagUint32:
		if offset+4 > uint32(len(data)) {
			return
		}
		v := val.U[0]
		data[offset] = byte(v)
		data[offset+1] = byte(v >> 8)
		data[offset+2] = byte(v >> 16)
		data[offset+3] = byte(v >> 24)
	case TagInt32:
		if offset+4 > uint32(len(data)) {
			return
		}
		v := val.U[0]
		data[offset] = byte(v)
		data[offset+1] = byte(v >> 8)
		data[offset+2] = byte(v >> 16)
		data[offset+3] = byte(v >> 24)
	case TagVec2:
		writeValueToBuffer(data, offset, ValFloat(val.F[0]))
		writeValueToBuffer(data, offset+4, ValFloat(val.F[1]))
	case TagVec3:
		writeValueToBuffer(data, offset, ValFloat(val.F[0]))
		writeValueToBuffer(data, offset+4, ValFloat(val.F[1]))
		writeValueToBuffer(data, offset+8, ValFloat(val.F[2]))
	case TagVec4:
		writeValueToBuffer(data, offset, ValFloat(val.F[0]))
		writeValueToBuffer(data, offset+4, ValFloat(val.F[1]))
		writeValueToBuffer(data, offset+8, ValFloat(val.F[2]))
		writeValueToBuffer(data, offset+12, ValFloat(val.F[3]))
	case TagArray:
		arr := val.AsArray()
		off := offset
		for _, elem := range arr {
			writeValueToBuffer(data, off, elem)
			off += valueByteSize(elem)
		}
	}
}

// valueByteSize returns the byte size of a runtime Value.
func valueByteSize(val Value) uint32 {
	switch val.Tag {
	case TagFloat32, TagUint32, TagInt32, TagBool:
		return 4
	case TagVec2:
		return 8
	case TagVec3:
		return 12
	case TagVec4:
		return 16
	case TagArray:
		arr := val.AsArray()
		var total uint32
		for _, elem := range arr {
			total += valueByteSize(elem)
		}
		return total
	default:
		return 4
	}
}

// zeroValueForVar creates a zero value matching the pointee type of a pointer type.
func zeroValueForVar(m *Module, ptrTypeID uint32) Value {
	ti := m.PointeeType(ptrTypeID)
	if ti == nil {
		return ValUint(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return ValFloat(0)
	case TypeInt:
		if ti.Signed {
			return ValInt(0)
		}
		return ValUint(0)
	case TypeVector:
		switch ti.Components {
		case 2:
			return ValVec2(0, 0)
		case 3:
			return ValVec3(0, 0, 0)
		case 4:
			return ValVec4(0, 0, 0, 0)
		}
	case TypeMatrix:
		if ti.Components > 0 {
			cols := make([]Value, ti.Components)
			for i := range cols {
				cols[i] = zeroValue(m.Types, ti.ElemType)
			}
			return ValArray(cols)
		}
	case TypeArray:
		if ti.Length > 0 {
			arr := make([]Value, ti.Length)
			for i := range arr {
				arr[i] = zeroValue(m.Types, ti.ElemType)
			}
			return ValArray(arr)
		}
	case TypeStruct:
		members := make([]Value, len(ti.MemberIDs))
		for i, memberTypeID := range ti.MemberIDs {
			members[i] = zeroValue(m.Types, memberTypeID)
		}
		return ValArray(members)
	case TypeBool:
		return ValBool(false)
	}
	return ValUint(0)
}

// zeroValue returns the zero/default value for a given type.
func zeroValue(types map[uint32]*TypeInfo, typeID uint32) Value {
	ti, ok := types[typeID]
	if !ok {
		return ValUint(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return ValFloat(0)
	case TypeInt:
		if ti.Signed {
			return ValInt(0)
		}
		return ValUint(0)
	case TypeVector:
		switch ti.Components {
		case 2:
			return ValVec2(0, 0)
		case 3:
			return ValVec3(0, 0, 0)
		case 4:
			return ValVec4(0, 0, 0, 0)
		}
	case TypeMatrix:
		if ti.Components > 0 {
			cols := make([]Value, ti.Components)
			for i := range cols {
				cols[i] = zeroValue(types, ti.ElemType)
			}
			return ValArray(cols)
		}
	case TypeArray:
		if ti.Length > 0 {
			arr := make([]Value, ti.Length)
			for i := range arr {
				arr[i] = zeroValue(types, ti.ElemType)
			}
			return ValArray(arr)
		}
	case TypeBool:
		return ValBool(false)
	}
	return ValUint(0)
}
