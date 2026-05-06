//go:build !(js && wasm)

// SPIR-V runtime value types for the software backend interpreter.
//
// These types represent values during SPIR-V execution. The interpreter uses
// Go interface{} (aliased as Value) for dynamic typing, matching SPIR-V's
// untyped SSA register model.

package shader

// Value is a runtime value in the SPIR-V interpreter.
// Concrete types: Float32, Uint32, Int32, Vec2, Vec3, Vec4, Array, Pointer.
type Value = any

// Float32 is a 32-bit floating point scalar.
type Float32 = float32

// Uint32 is a 32-bit unsigned integer scalar.
type Uint32 = uint32

// Int32 is a 32-bit signed integer scalar.
type Int32 = int32

// Vec2 is a 2-component float32 vector.
type Vec2 = [2]float32

// Vec3 is a 3-component float32 vector.
type Vec3 = [3]float32

// Vec4 is a 4-component float32 vector.
type Vec4 = [4]float32

// Array is a dynamically-sized collection of values.
type Array = []Value

// Pointer wraps a mutable reference to a Value stored in a variable.
// SPIR-V OpVariable creates a Pointer; OpLoad dereferences it; OpStore writes to it.
type Pointer struct {
	Value Value
}

// Mat4 is a 4x4 column-major matrix stored as an Array of 4 Vec4 columns.
// SPIR-V represents matrices as arrays of column vectors.
type Mat4 = [4]Vec4

// Texture2D represents a 2D texture for sampling in the SPIR-V interpreter.
type Texture2D struct {
	Width  uint32
	Height uint32
	Data   []byte // RGBA pixel data, row-major, 4 bytes per pixel.
	Format uint32 // Texture format identifier (0 = RGBA8).
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
	Image   Value // BindingKey or *Texture2D
	Sampler Value // BindingKey or *Sampler
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
