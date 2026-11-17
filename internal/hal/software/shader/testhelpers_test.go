//go:build !(js && wasm)

package shader

import "unsafe"

// testval converts old-style Go values to tagged union Value for test compatibility.
// This avoids rewriting every test that creates Values from raw Go types.
// For test-only types like *Texture2D, we store the pointer in Ref with TagPointer
// so resolveTexture/resolveSampler can find it.
func testval(v any) Value {
	switch x := v.(type) {
	case float32:
		return ValFloat(x)
	case uint32:
		return ValUint(x)
	case int32:
		return ValInt(x)
	case int:
		return ValUint(uint32(x))
	case bool:
		return ValBool(x)
	case [2]float32:
		return ValVec2(x[0], x[1])
	case [3]float32:
		return ValVec3(x[0], x[1], x[2])
	case [4]float32:
		return ValVec4(x[0], x[1], x[2], x[3])
	case []Value:
		return ValArray(x)
	case *Pointer:
		return ValPointer(x)
	case *SubPointer:
		return ValSubPointer(x)
	case *BufferPointer:
		return ValBufferPointer(x)
	case *SampledImageValue:
		return ValSampledImage(x)
	case BindingKey:
		return ValBindingKey(x)
	case *Texture2D:
		// Store texture pointer using Ref with a special tag for test use.
		return Value{Tag: TagPointer, Ref: unsafe.Pointer(x)}
	case *Sampler:
		return Value{Tag: TagPointer, Ref: unsafe.Pointer(x)}
	case Value:
		return x
	default:
		return Value{}
	}
}

// testMakeValues creates a []Value slice from a map of ID->any entries.
// Values are automatically converted from raw Go types (float32, uint32, etc.)
// to tagged union Values.
func testMakeValues(entries map[uint32]any) []Value {
	var maxID uint32
	for id := range entries {
		if id > maxID {
			maxID = id
		}
	}
	// Add headroom for result IDs that tests may write into (e.g., ResultID=100).
	size := maxID + 1
	if size < 128 {
		size = 128
	}
	values := make([]Value, size)
	for id, val := range entries {
		values[id] = testval(val)
	}
	return values
}

// testIsFloat32 checks if a Value is a float32 scalar and returns it.
func testIsFloat32(v Value) (float32, bool) {
	return v.F[0], v.Tag == TagFloat32
}

// testIsUint32 checks if a Value is a uint32 scalar and returns it.
func testIsUint32(v Value) (uint32, bool) {
	return v.U[0], v.Tag == TagUint32
}

// testIsVec2 checks if a Value is a Vec2 and returns it.
func testIsVec2(v Value) (Vec2, bool) {
	return v.AsVec2(), v.Tag == TagVec2
}

// testIsVec3 checks if a Value is a Vec3 and returns it.
func testIsVec3(v Value) (Vec3, bool) {
	return v.AsVec3(), v.Tag == TagVec3
}

// testIsVec4 checks if a Value is a Vec4 and returns it.
func testIsVec4(v Value) (Vec4, bool) {
	return v.AsVec4(), v.Tag == TagVec4
}

// testIsBool checks if a Value is a bool and returns it.
func testIsBool(v Value) (bool, bool) {
	return v.AsBool(), v.Tag == TagBool
}

// testIsArray checks if a Value is an Array and returns it.
func testIsArray(v Value) ([]Value, bool) {
	return v.AsArray(), v.Tag == TagArray
}

// valueApproxEqual compares two Values for approximate equality.
func valueApproxEqual(a, b Value, eps float64) bool {
	if a.Tag != b.Tag {
		return false
	}
	switch a.Tag {
	case TagFloat32:
		return abs64(float64(a.F[0]-b.F[0])) <= eps
	case TagVec2:
		return abs64(float64(a.F[0]-b.F[0])) <= eps && abs64(float64(a.F[1]-b.F[1])) <= eps
	case TagVec3:
		return abs64(float64(a.F[0]-b.F[0])) <= eps && abs64(float64(a.F[1]-b.F[1])) <= eps &&
			abs64(float64(a.F[2]-b.F[2])) <= eps
	case TagVec4:
		return abs64(float64(a.F[0]-b.F[0])) <= eps && abs64(float64(a.F[1]-b.F[1])) <= eps &&
			abs64(float64(a.F[2]-b.F[2])) <= eps && abs64(float64(a.F[3]-b.F[3])) <= eps
	default:
		return valuesEqual(a, b)
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
