//go:build !rust && !(js && wasm)

package wgpu

// Fuzz roundtrips for every public/mirror pair (S13 O3).
//
// One target per pair. The property depends on the pair's class
// (PRACTICES-CONSENSUS.md O3, measured in descriptor_drift_test.go):
//
//   - PURE pairs assert public -> mirror -> public identity;
//   - NORMALIZING pairs assert idempotence in the normalized domain,
//     toHAL(fromHAL(toHAL(x))) == toHAL(x), because the first conversion
//     resolves WebGPU defaults that are already values on the way back.
//
// f.Fuzz accepts only basic kinds (testing.F.Fuzz documentation lists []byte,
// string, bool, the integers and the floats), so each target takes []byte and
// fuzzDecode builds the public descriptor from it. fuzzEncode is the exact
// inverse of fuzzDecode and lets a seed be written as an ordinary Go value.
// The committed corpus lives in testdata/fuzz/<Target>/; ordinary `go test`
// runs it and the f.Add seeds, CI never passes -fuzz.
//
// Reachable domain, per pair. A fuzz-created value can never hold a live HAL
// resource, so the mirror fields that carry a resource interface are nil on
// both sides. The adapters below say, per pair, which handle is dropped and why
// the property still holds on what remains; the projection test in
// descriptor_drift_test.go is the coverage proof for the fields no fuzz value
// can reach — it proves every leaf is mapped or justified, not that a
// conversion assigns it. Values are the property below: a fromHAL that forgets
// a field fails the roundtrip rather than the table. The concrete values are
// pinned by descriptor_test.go (ImageCopyTexture Origin included).

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"

	"github.com/gogpu/gputypes"
)

// --- byte codec ------------------------------------------------------------

const (
	fuzzMaxSliceLen  = 3
	fuzzMaxStringLen = 7
	fuzzMaxDepth     = 16
)

type fuzzWriter struct{ buf []byte }

func (w *fuzzWriter) u8(v byte) { w.buf = append(w.buf, v) }

func (w *fuzzWriter) u16(v uint16) { w.buf = append(w.buf, byte(v), byte(v>>8)) }

func (w *fuzzWriter) u32(v uint32) {
	w.buf = append(w.buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (w *fuzzWriter) u64(v uint64) {
	w.u32(uint32(v))
	w.u32(uint32(v >> 32))
}

func (w *fuzzWriter) raw(b []byte) { w.buf = append(w.buf, b...) }

// fuzzEncode serializes v deterministically; fuzzDecode reverses it exactly.
func fuzzEncode[T any](v T) []byte {
	w := &fuzzWriter{}
	writeFuzzValue(reflect.ValueOf(v), w)
	return w.buf
}

func writeFuzzValue(v reflect.Value, w *fuzzWriter) {
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			w.u8(1)
		} else {
			w.u8(0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.u64(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.u64(v.Uint())
	case reflect.Float32:
		w.u32(math.Float32bits(float32(v.Float())))
	case reflect.Float64:
		w.u64(math.Float64bits(v.Float()))
	case reflect.String:
		s := v.String()
		w.u16(uint16(len(s)))
		w.raw([]byte(s))
	case reflect.Pointer:
		if v.IsNil() {
			w.u8(0)
			return
		}
		w.u8(1)
		writeFuzzValue(v.Elem(), w)
	case reflect.Slice:
		if v.IsNil() {
			w.u8(0)
			return
		}
		w.u8(1)
		w.u16(uint16(v.Len()))
		for i := 0; i < v.Len(); i++ {
			writeFuzzValue(v.Index(i), w)
		}
	case reflect.Array:
		w.u16(uint16(v.Len()))
		for i := 0; i < v.Len(); i++ {
			writeFuzzValue(v.Index(i), w)
		}
	case reflect.Map:
		writeFuzzMap(v, w)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue // unexported fields have no value semantics here
			}
			writeFuzzValue(v.Field(i), w)
		}
	default:
		w.u8(0) // interfaces and the remaining kinds are not fuzzed values
	}
}

func writeFuzzMap(v reflect.Value, w *fuzzWriter) {
	if v.IsNil() {
		w.u8(0)
		return
	}
	w.u8(1)
	w.u16(uint16(v.Len()))
	keys := make([]reflect.Value, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		keys = append(keys, iter.Key())
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	for _, key := range keys {
		writeFuzzValue(key, w)
		writeFuzzValue(v.MapIndex(key), w)
	}
}

type fuzzReader struct {
	data []byte
	pos  int
}

func (r *fuzzReader) take(n int) []byte {
	if n <= 0 {
		return nil
	}
	if r.pos+n > len(r.data) {
		r.pos = len(r.data)
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *fuzzReader) u8() byte {
	if b := r.take(1); len(b) == 1 {
		return b[0]
	}
	return 0
}

func (r *fuzzReader) u16() uint16 {
	b := r.take(2)
	if len(b) < 2 {
		return 0
	}
	return uint16(b[0]) | uint16(b[1])<<8
}

func (r *fuzzReader) u32() uint32 { return uint32(r.u16()) | uint32(r.u16())<<16 }

func (r *fuzzReader) u64() uint64 { return uint64(r.u32()) | uint64(r.u32())<<32 }

// fuzzDecode builds a T from data; short input leaves the remaining fields
// zero, which keeps every corpus entry usable.
func fuzzDecode[T any](data []byte) T {
	var v T
	r := &fuzzReader{data: data}
	fillFuzzValue(reflect.ValueOf(&v).Elem(), r, 0)
	return v
}

func fillFuzzValue(v reflect.Value, r *fuzzReader, depth int) {
	if depth > fuzzMaxDepth || !v.CanSet() {
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(r.u8()&1 == 1)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(r.u64()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(r.u64())
	case reflect.Float32:
		v.SetFloat(float64(math.Float32frombits(r.u32())))
	case reflect.Float64:
		v.SetFloat(math.Float64frombits(r.u64()))
	case reflect.String:
		v.SetString(string(r.take(int(r.u16()) % (fuzzMaxStringLen + 1))))
	case reflect.Pointer:
		fillFuzzPointer(v, r, depth)
	case reflect.Slice:
		fillFuzzSlice(v, r, depth)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			fillFuzzValue(v.Index(i), r, depth+1)
		}
	case reflect.Map:
		fillFuzzMap(v, r, depth)
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath != "" {
				continue
			}
			fillFuzzValue(v.Field(i), r, depth+1)
		}
	default:
		// Unsupported kinds (interfaces, functions) stay zero.
	}
}

func fillFuzzPointer(v reflect.Value, r *fuzzReader, depth int) {
	if r.u8()&1 == 0 {
		v.SetZero()
		return
	}
	pointer := reflect.New(v.Type().Elem())
	fillFuzzValue(pointer.Elem(), r, depth+1)
	v.Set(pointer)
}

func fillFuzzSlice(v reflect.Value, r *fuzzReader, depth int) {
	if r.u8()&1 == 0 {
		v.SetZero()
		return
	}
	n := int(r.u16()) % (fuzzMaxSliceLen + 1)
	slice := reflect.MakeSlice(v.Type(), n, n)
	for i := 0; i < n; i++ {
		fillFuzzValue(slice.Index(i), r, depth+1)
	}
	v.Set(slice)
}

func fillFuzzMap(v reflect.Value, r *fuzzReader, depth int) {
	if r.u8()&1 == 0 {
		v.SetZero()
		return
	}
	n := int(r.u16()) % (fuzzMaxSliceLen + 1)
	m := reflect.MakeMapWithSize(v.Type(), n)
	for i := 0; i < n; i++ {
		key := reflect.New(v.Type().Key()).Elem()
		fillFuzzValue(key, r, depth+1)
		value := reflect.New(v.Type().Elem()).Elem()
		fillFuzzValue(value, r, depth+1)
		m.SetMapIndex(key, value)
	}
	v.Set(m)
}

// --- seed helpers ----------------------------------------------------------

func addSeed[T any](f *testing.F, v T) { f.Add(fuzzEncode(v)) }

// NaN bit patterns: the quiet form an arithmetic NaN takes and a negative NaN
// with a different payload, so the bitwise comparator is exercised on both.
const (
	quietNaN32    uint32 = 0x7FC00001
	negativeNaN32 uint32 = 0xFFC00002
	quietNaN64    uint64 = 0x7FF8000000000001
	negativeNaN64 uint64 = 0xFFF8000000000002
)

// numericEdgeSeed sets every integer to its maximum and every float to the
// given bit pattern; pointers, slices and maps stay nil (the zero seed and the
// empty-collections seed cover those).
func numericEdgeSeed[T any](f32bits uint32, f64bits uint64) T {
	var v T
	setNumericEdge(reflect.ValueOf(&v).Elem(), f32bits, f64bits)
	return v
}

func setNumericEdge(v reflect.Value, f32bits uint32, f64bits uint64) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(math.MaxInt64)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(math.MaxUint64)
	case reflect.Float32:
		v.SetFloat(float64(math.Float32frombits(f32bits)))
	case reflect.Float64:
		v.SetFloat(math.Float64frombits(f64bits))
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath == "" {
				setNumericEdge(v.Field(i), f32bits, f64bits)
			}
		}
	}
}

// emptyCollectionsSeed makes every slice and map non-nil and empty, and gives
// every pointer a value, which covers the empty-vs-nil distinction the
// comparator treats as significant.
func emptyCollectionsSeed[T any]() T {
	var v T
	setEmptyCollections(reflect.ValueOf(&v).Elem())
	return v
}

func setEmptyCollections(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		pointer := reflect.New(v.Type().Elem())
		setEmptyCollections(pointer.Elem())
		v.Set(pointer)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).PkgPath == "" {
				setEmptyCollections(v.Field(i))
			}
		}
	}
}

// enumStringer is the gputypes enum shape: a uint32 whose String names every
// declared constant and returns "Unknown" otherwise.
type enumStringer interface {
	~uint32
	String() string
}

// enumValues returns every declared constant of T below limit, discovered
// through String so the seed set follows gputypes without a hand-kept list.
func enumValues[T enumStringer](limit uint32) []T {
	values := make([]T, 0, 8)
	for i := uint32(0); i < limit; i++ {
		if value := T(i); value.String() != "Unknown" {
			values = append(values, value)
		}
	}
	return values
}

func textureUsageSeeds() []gputypes.TextureUsage {
	return []gputypes.TextureUsage{
		gputypes.TextureUsageNone,
		gputypes.TextureUsageCopySrc,
		gputypes.TextureUsageCopyDst,
		gputypes.TextureUsageTextureBinding,
		gputypes.TextureUsageStorageBinding,
		gputypes.TextureUsageRenderAttachment,
	}
}

func bufferUsageSeeds() []gputypes.BufferUsage {
	return []gputypes.BufferUsage{
		gputypes.BufferUsageNone,
		gputypes.BufferUsageMapRead,
		gputypes.BufferUsageMapWrite,
		gputypes.BufferUsageCopySrc,
		gputypes.BufferUsageCopyDst,
		gputypes.BufferUsageIndex,
		gputypes.BufferUsageVertex,
		gputypes.BufferUsageUniform,
		gputypes.BufferUsageStorage,
		gputypes.BufferUsageIndirect,
		gputypes.BufferUsageQueryResolve,
		gputypes.BufferUsageAccelerationStructureScratch,
		gputypes.BufferUsageBlasInput,
		gputypes.BufferUsageTlasInput,
		gputypes.BufferUsageAccelerationStructureQuery,
	}
}

// colorWriteMaskSeeds lists every ColorWriteMask constant: the type is a bit flag without a String method,
// so enumValues cannot discover it (the same reason bufferUsageSeeds and textureUsageSeeds are hand-kept).
func colorWriteMaskSeeds() []gputypes.ColorWriteMask {
	return []gputypes.ColorWriteMask{
		gputypes.ColorWriteMaskNone,
		gputypes.ColorWriteMaskRed,
		gputypes.ColorWriteMaskGreen,
		gputypes.ColorWriteMaskBlue,
		gputypes.ColorWriteMaskAlpha,
		gputypes.ColorWriteMaskAll,
	}
}

// reportDiff fails on every difference between want and got.
func reportDiff[T any](t *testing.T, want, got T) {
	t.Helper()
	for _, problem := range diffValue(want, got) {
		t.Errorf("roundtrip: %s", problem)
	}
}

// --- targets: value-in-parent pairs (PURE) ---------------------------------

func FuzzExtent3DRoundtrip(f *testing.F) {
	addSeed(f, Extent3D{})
	addSeed(f, numericEdgeSeed[Extent3D](quietNaN32, quietNaN64))
	addSeed(f, numericEdgeSeed[Extent3D](negativeNaN32, negativeNaN64))
	addSeed(f, emptyCollectionsSeed[Extent3D]())
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[Extent3D](data)
		h := x.toHAL()
		reportDiff(t, x, extent3DFromHAL(&h))
	})
}

func FuzzOrigin3DRoundtrip(f *testing.F) {
	addSeed(f, Origin3D{})
	addSeed(f, numericEdgeSeed[Origin3D](quietNaN32, quietNaN64))
	addSeed(f, numericEdgeSeed[Origin3D](negativeNaN32, negativeNaN64))
	addSeed(f, emptyCollectionsSeed[Origin3D]())
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[Origin3D](data)
		h := x.toHAL()
		reportDiff(t, x, origin3DFromHAL(&h))
	})
}

func FuzzStencilFaceStateRoundtrip(f *testing.F) {
	addSeed(f, StencilFaceState{})
	addSeed(f, numericEdgeSeed[StencilFaceState](quietNaN32, quietNaN64))
	for _, compare := range enumValues[gputypes.CompareFunction](16) {
		addSeed(f, StencilFaceState{Compare: compare})
	}
	for _, op := range enumValues[gputypes.StencilOperation](16) {
		addSeed(f, StencilFaceState{FailOp: op, DepthFailOp: op, PassOp: op})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[StencilFaceState](data)
		h := x.toHAL()
		reportDiff(t, x, stencilFaceStateFromHAL(&h))
	})
}

// --- targets: PURE descriptors ---------------------------------------------

func FuzzBufferDescriptorRoundtrip(f *testing.F) {
	addSeed(f, BufferDescriptor{})
	addSeed(f, numericEdgeSeed[BufferDescriptor](quietNaN32, quietNaN64))
	for _, usage := range bufferUsageSeeds() {
		addSeed(f, BufferDescriptor{Usage: usage})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[BufferDescriptor](data)
		reportDiff(t, x, bufferDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzTextureDescriptorRoundtrip(f *testing.F) {
	addSeed(f, TextureDescriptor{})
	addSeed(f, numericEdgeSeed[TextureDescriptor](quietNaN32, quietNaN64))
	addSeed(f, emptyCollectionsSeed[TextureDescriptor]())
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, TextureDescriptor{Format: format, ViewFormats: []gputypes.TextureFormat{format}})
	}
	for _, dimension := range enumValues[gputypes.TextureDimension](8) {
		addSeed(f, TextureDescriptor{Dimension: dimension})
	}
	for _, usage := range textureUsageSeeds() {
		addSeed(f, TextureDescriptor{Usage: usage})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[TextureDescriptor](data)
		reportDiff(t, x, textureDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzTextureViewDescriptorRoundtrip(f *testing.F) {
	addSeed(f, TextureViewDescriptor{})
	addSeed(f, numericEdgeSeed[TextureViewDescriptor](quietNaN32, quietNaN64))
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, TextureViewDescriptor{Format: format})
	}
	for _, dimension := range enumValues[gputypes.TextureViewDimension](16) {
		addSeed(f, TextureViewDescriptor{Dimension: dimension})
	}
	for _, aspect := range enumValues[gputypes.TextureAspect](8) {
		addSeed(f, TextureViewDescriptor{Aspect: aspect})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[TextureViewDescriptor](data)
		reportDiff(t, x, textureViewDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzQuerySetDescriptorRoundtrip(f *testing.F) {
	addSeed(f, QuerySetDescriptor{})
	addSeed(f, numericEdgeSeed[QuerySetDescriptor](quietNaN32, quietNaN64))
	addSeed(f, QuerySetDescriptor{Type: QueryTypeOcclusion})
	addSeed(f, QuerySetDescriptor{Type: QueryTypeTimestamp})
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[QuerySetDescriptor](data)
		reportDiff(t, x, querySetDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzShaderModuleDescriptorRoundtrip(f *testing.F) {
	addSeed(f, ShaderModuleDescriptor{})
	addSeed(f, numericEdgeSeed[ShaderModuleDescriptor](quietNaN32, quietNaN64))
	addSeed(f, emptyCollectionsSeed[ShaderModuleDescriptor]())
	addSeed(f, ShaderModuleDescriptor{Label: "l", WGSL: "@compute fn main() {}", SPIRV: []uint32{0x07230203, 0}})
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[ShaderModuleDescriptor](data)
		reportDiff(t, x, shaderModuleDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzCommandEncoderDescriptorRoundtrip(f *testing.F) {
	addSeed(f, CommandEncoderDescriptor{})
	addSeed(f, CommandEncoderDescriptor{Label: "encoder"})
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[CommandEncoderDescriptor](data)
		reportDiff(t, x, commandEncoderDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzBindGroupLayoutDescriptorRoundtrip(f *testing.F) {
	addSeed(f, BindGroupLayoutDescriptor{})
	addSeed(f, emptyCollectionsSeed[BindGroupLayoutDescriptor]())
	addSeed(f, BindGroupLayoutDescriptor{Label: "bgl", Entries: []gputypes.BindGroupLayoutEntry{{Binding: 1}, {Binding: 2}}})
	for _, stages := range enumValues[gputypes.ShaderStages](8) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{Visibility: stages}}})
	}
	for _, binding := range enumValues[gputypes.BufferBindingType](8) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			Buffer: &gputypes.BufferBindingLayout{Type: binding},
		}}})
	}
	for _, binding := range enumValues[gputypes.SamplerBindingType](8) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			Sampler: &gputypes.SamplerBindingLayout{Type: binding},
		}}})
	}
	for _, sample := range enumValues[gputypes.TextureSampleType](16) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			Texture: &gputypes.TextureBindingLayout{SampleType: sample},
		}}})
	}
	for _, dimension := range enumValues[gputypes.TextureViewDimension](16) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			Texture: &gputypes.TextureBindingLayout{ViewDimension: dimension},
		}}})
	}
	for _, access := range enumValues[gputypes.StorageTextureAccess](8) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			StorageTexture: &gputypes.StorageTextureBindingLayout{Access: access},
		}}})
	}
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
			StorageTexture: &gputypes.StorageTextureBindingLayout{Format: format},
		}}})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[BindGroupLayoutDescriptor](data)
		reportDiff(t, x, bindGroupLayoutDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzDepthStencilStateRoundtrip(f *testing.F) {
	addSeed(f, DepthStencilState{})
	addSeed(f, numericEdgeSeed[DepthStencilState](quietNaN32, quietNaN64))
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, DepthStencilState{Format: format})
	}
	for _, compare := range enumValues[gputypes.CompareFunction](16) {
		addSeed(f, DepthStencilState{DepthCompare: compare})
	}
	for _, compare := range enumValues[gputypes.CompareFunction](16) {
		addSeed(f, DepthStencilState{
			StencilFront: StencilFaceState{Compare: compare},
			StencilBack:  StencilFaceState{Compare: compare},
		})
	}
	for _, op := range enumValues[gputypes.StencilOperation](16) {
		addSeed(f, DepthStencilState{
			StencilFront: StencilFaceState{FailOp: op, DepthFailOp: op, PassOp: op},
			StencilBack:  StencilFaceState{FailOp: op, DepthFailOp: op, PassOp: op},
		})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[DepthStencilState](data)
		reportDiff(t, x, *depthStencilStateFromHAL(x.toHAL()))
	})
}

func FuzzSurfaceConfigurationRoundtrip(f *testing.F) {
	addSeed(f, SurfaceConfiguration{})
	addSeed(f, numericEdgeSeed[SurfaceConfiguration](quietNaN32, quietNaN64))
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, SurfaceConfiguration{Format: format})
	}
	for _, usage := range textureUsageSeeds() {
		addSeed(f, SurfaceConfiguration{Usage: usage})
	}
	for _, mode := range enumValues[gputypes.PresentMode](8) {
		addSeed(f, SurfaceConfiguration{PresentMode: mode})
	}
	for _, alpha := range enumValues[gputypes.CompositeAlphaMode](8) {
		addSeed(f, SurfaceConfiguration{AlphaMode: alpha})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[SurfaceConfiguration](data)
		reportDiff(t, x, surfaceConfigurationFromHAL(x.toHAL()))
	})
}

func FuzzRenderPassDescriptorRoundtrip(f *testing.F) {
	addSeed(f, RenderPassDescriptor{})
	addSeed(f, numericEdgeSeed[RenderPassDescriptor](quietNaN32, quietNaN64))
	addSeed(f, emptyCollectionsSeed[RenderPassDescriptor]())
	for _, op := range enumValues[gputypes.LoadOp](8) {
		addSeed(f, RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{LoadOp: op}}})
	}
	for _, op := range enumValues[gputypes.StoreOp](8) {
		addSeed(f, RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{StoreOp: op}}})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[RenderPassDescriptor](data)
		// The mirror carries resource interfaces and drops TimestampWrites
		// without a live query set (descriptor.go:560-567); the reachable
		// public domain is the value fields, and an empty non-nil attachment
		// slice has an empty image (append stays nil).
		for i := range x.ColorAttachments {
			x.ColorAttachments[i].View = nil
			x.ColorAttachments[i].ResolveTarget = nil
		}
		if len(x.ColorAttachments) == 0 {
			x.ColorAttachments = nil
		}
		if x.DepthStencilAttachment != nil {
			x.DepthStencilAttachment.View = nil
		}
		x.TimestampWrites = nil
		reportDiff(t, x, renderPassDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzComputePassDescriptorRoundtrip(f *testing.F) {
	addSeed(f, ComputePassDescriptor{})
	addSeed(f, ComputePassDescriptor{Label: "compute pass"})
	addSeed(f, emptyCollectionsSeed[ComputePassDescriptor]())
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[ComputePassDescriptor](data)
		// Same query-set drop as the render pass: only Label is reachable.
		x.TimestampWrites = nil
		reportDiff(t, x, computePassDescriptorFromHAL(x.toHAL()))
	})
}

func FuzzImageCopyTextureRoundtrip(f *testing.F) {
	addSeed(f, ImageCopyTexture{})
	addSeed(f, numericEdgeSeed[ImageCopyTexture](quietNaN32, quietNaN64))
	for _, aspect := range enumValues[gputypes.TextureAspect](8) {
		addSeed(f, ImageCopyTexture{Aspect: aspect})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[ImageCopyTexture](data)
		// toHAL returns nil unless Texture.impl resolves (descriptor.go:646-653),
		// so without a live texture the image is the nil descriptor. The value
		// fields (Origin, MipLevel, Aspect) convert only with a live texture and
		// are covered by the projection test.
		x.Texture = nil
		h := x.toHAL()
		if h != nil {
			t.Fatalf("a nil texture must convert to a nil HAL descriptor, got %+v", h)
		}
		if back := imageCopyTextureFromHAL(h); back != nil {
			t.Fatalf("a nil HAL descriptor must reverse to nil, got %+v", back)
		}
	})
}

// --- targets: NORMALIZING descriptors (idempotence) ------------------------

func FuzzSamplerDescriptorRoundtrip(f *testing.F) {
	addSeed(f, SamplerDescriptor{})
	addSeed(f, numericEdgeSeed[SamplerDescriptor](quietNaN32, quietNaN64))
	for _, mode := range enumValues[gputypes.AddressMode](8) {
		addSeed(f, SamplerDescriptor{AddressModeU: mode, AddressModeV: mode, AddressModeW: mode})
	}
	for _, filter := range enumValues[gputypes.FilterMode](8) {
		addSeed(f, SamplerDescriptor{MagFilter: filter, MinFilter: filter, MipmapFilter: filter})
	}
	for _, compare := range enumValues[gputypes.CompareFunction](16) {
		addSeed(f, SamplerDescriptor{Compare: compare})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[SamplerDescriptor](data)
		first := x.toHAL()
		back := samplerDescriptorFromHAL(first)
		reportDiff(t, first, back.toHAL())
	})
}

func FuzzRenderPipelineDescriptorRoundtrip(f *testing.F) {
	addSeed(f, RenderPipelineDescriptor{})
	addSeed(f, numericEdgeSeed[RenderPipelineDescriptor](quietNaN32, quietNaN64))
	addSeed(f, emptyCollectionsSeed[RenderPipelineDescriptor]())
	for _, topology := range enumValues[gputypes.PrimitiveTopology](8) {
		addSeed(f, RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{Topology: topology}})
	}
	for _, indexFormat := range enumValues[gputypes.IndexFormat](8) {
		strip := indexFormat
		addSeed(f, RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{StripIndexFormat: &strip}})
	}
	for _, face := range enumValues[gputypes.FrontFace](8) {
		addSeed(f, RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{FrontFace: face}})
	}
	for _, cull := range enumValues[gputypes.CullMode](8) {
		addSeed(f, RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{CullMode: cull}})
	}
	for _, format := range enumValues[gputypes.TextureFormat](512) {
		addSeed(f, RenderPipelineDescriptor{DepthStencil: &DepthStencilState{Format: format}})
		addSeed(f, RenderPipelineDescriptor{Fragment: &FragmentState{Targets: []gputypes.ColorTargetState{{Format: format}}}})
	}
	for _, compare := range enumValues[gputypes.CompareFunction](16) {
		addSeed(f, RenderPipelineDescriptor{DepthStencil: &DepthStencilState{DepthCompare: compare}})
		addSeed(f, RenderPipelineDescriptor{DepthStencil: &DepthStencilState{
			StencilFront: StencilFaceState{Compare: compare},
			StencilBack:  StencilFaceState{Compare: compare},
		}})
	}
	for _, op := range enumValues[gputypes.StencilOperation](16) {
		addSeed(f, RenderPipelineDescriptor{DepthStencil: &DepthStencilState{
			StencilFront: StencilFaceState{FailOp: op, DepthFailOp: op, PassOp: op},
			StencilBack:  StencilFaceState{FailOp: op, DepthFailOp: op, PassOp: op},
		}})
	}
	for _, mode := range enumValues[gputypes.VertexStepMode](8) {
		addSeed(f, RenderPipelineDescriptor{Vertex: VertexState{Buffers: []gputypes.VertexBufferLayout{{StepMode: mode}}}})
	}
	for _, format := range enumValues[gputypes.VertexFormat](64) {
		addSeed(f, RenderPipelineDescriptor{Vertex: VertexState{Buffers: []gputypes.VertexBufferLayout{{
			Attributes: []gputypes.VertexAttribute{{Format: format}},
		}}}})
	}
	for _, factor := range enumValues[gputypes.BlendFactor](32) {
		addSeed(f, RenderPipelineDescriptor{Fragment: &FragmentState{Targets: []gputypes.ColorTargetState{{Blend: &gputypes.BlendState{
			Color: gputypes.BlendComponent{SrcFactor: factor, DstFactor: factor},
			Alpha: gputypes.BlendComponent{SrcFactor: factor, DstFactor: factor},
		}}}}})
	}
	for _, op := range enumValues[gputypes.BlendOperation](8) {
		addSeed(f, RenderPipelineDescriptor{Fragment: &FragmentState{Targets: []gputypes.ColorTargetState{{Blend: &gputypes.BlendState{
			Color: gputypes.BlendComponent{Operation: op},
			Alpha: gputypes.BlendComponent{Operation: op},
		}}}}})
	}
	for _, mask := range colorWriteMaskSeeds() {
		addSeed(f, RenderPipelineDescriptor{Fragment: &FragmentState{Targets: []gputypes.ColorTargetState{{WriteMask: mask}}}})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[RenderPipelineDescriptor](data)
		// Resource handles never resolve in a fuzz value: the mirror's Layout,
		// Vertex.Module and Fragment.Module stay nil, and descriptor.go:393-413
		// drops the stages that depend on them.
		x.Layout = nil
		x.Vertex.Module = nil
		if x.Fragment != nil {
			x.Fragment.Module = nil
		}
		first := x.toHAL()
		back := renderPipelineDescriptorFromHAL(first)
		reportDiff(t, first, back.toHAL())
	})
}

func FuzzComputePipelineDescriptorRoundtrip(f *testing.F) {
	addSeed(f, ComputePipelineDescriptor{})
	addSeed(f, numericEdgeSeed[ComputePipelineDescriptor](quietNaN32, quietNaN64))
	addSeed(f, emptyCollectionsSeed[ComputePipelineDescriptor]())
	f.Fuzz(func(t *testing.T, data []byte) {
		x := fuzzDecode[ComputePipelineDescriptor](data)
		// Same handle rule as the render pipeline: the Compute state is only
		// filled when Module resolves (descriptor.go:461-468).
		x.Layout = nil
		x.Module = nil
		first := x.toHAL()
		back := computePipelineDescriptorFromHAL(first)
		reportDiff(t, first, back.toHAL())
	})
}
