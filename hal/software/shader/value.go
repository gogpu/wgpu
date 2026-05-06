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
