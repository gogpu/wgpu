//go:build !rust && !(js && wasm)

package wgpu

// Test-only reverse conversions and the identity comparator (S13 O3).
//
// No production fromHAL exists: the public descriptors convert into the HAL
// mirrors and the implementations live in internal. These reverse conversions
// exist so the fuzz targets in descriptor_fuzz_test.go can prove that the
// forward projection is lossless where it is lossless (PURE pairs) and
// idempotent in the normalized domain where it normalizes (NORMALIZING pairs).
//
// They are enumerated by the projection test: TestDescriptorProjection runs
// AssertNoDrift[hal.X, wgpu.X] over invertDrift(spec), so every mirror leaf is
// mapped or ignored with a justification and a mirror field added later has no
// row until the table follows. That enumeration is COVERAGE. The VALUES are what
// the fuzz targets prove: a reverse conversion that forgets to assign a mapped
// field passes the table and fails the roundtrip. The concrete values are pinned
// by descriptor_test.go (ImageCopyTexture Origin included).
//
// Identity semantics. This comparator is deliberately STRICTER than
// Kubernetes' apiequality.Semantic.DeepEqual, which treats nil and empty
// slices/maps as equal (PRACTICES-CONSENSUS.md, Amendment 2 item 3):
//
//   - nil and empty slices/maps are different values;
//   - a nil pointer and a non-nil pointer to a zero struct are different;
//   - floats compare by their bit pattern (math.Float32bits/Float64bits), so
//     NaN equals NaN bit for bit and no NaN is silently equal to another;
//   - unexported fields are not compared: the public handles carry no exported
//     value (their impl is unexported), and the reachable fuzz domain keeps
//     them nil. That decision is per pair and stated at each adapter.
//
// Aliasing. The forward toHAL is a read-only projection and shares slice, map
// and pointer backing with its input (descriptor.go); the reverse conversions
// copy, deeply over every reference field the public descriptors carry — slices
// are reallocated, and each nested slice or pointer (a layout's Attributes, an
// entry's binding layouts, a target's Blend, a Primitive's StripIndexFormat) is
// copied by its typed helper. The aliasing test below proves it at the top
// level and in the nested fields. The boundary follows from the reachable
// domain: unexported handle fields stay nil and are neither copied nor
// compared.

import (
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// --- identity comparator ---------------------------------------------------

// diffValue returns the differences between a and b, or nil when they are
// identical under the semantics stated in the file header.
func diffValue[T any](a, b T) []string {
	problems := make([]string, 0)
	diffReflect(reflect.ValueOf(a), reflect.ValueOf(b), "", &problems)
	return problems
}

func diffReflect(a, b reflect.Value, path string, problems *[]string) {
	if a.Kind() != b.Kind() {
		*problems = append(*problems, fmt.Sprintf("%s: kind %s vs %s", path, a.Kind(), b.Kind()))
		return
	}
	switch a.Kind() {
	case reflect.Bool:
		diffCompare(a.Bool() == b.Bool(), path, "bool", problems)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		diffCompare(a.Int() == b.Int(), path, "int", problems)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		diffCompare(a.Uint() == b.Uint(), path, "uint", problems)
	case reflect.Float32:
		diffCompare(math.Float32bits(float32(a.Float())) == math.Float32bits(float32(b.Float())), path, "float32 bits", problems)
	case reflect.Float64:
		diffCompare(math.Float64bits(a.Float()) == math.Float64bits(b.Float()), path, "float64 bits", problems)
	case reflect.String:
		diffCompare(a.String() == b.String(), path, "string", problems)
	case reflect.Pointer:
		diffPointer(a, b, path, problems)
	case reflect.Interface:
		diffInterface(a, b, path, problems)
	case reflect.Slice:
		diffSlice(a, b, path, problems)
	case reflect.Array:
		diffArray(a, b, path, problems)
	case reflect.Map:
		diffMap(a, b, path, problems)
	case reflect.Struct:
		diffStruct(a, b, path, problems)
	default:
		diffCompare(a.IsNil() == b.IsNil(), path, "nil-ness", problems)
	}
}

func diffCompare(equal bool, path, what string, problems *[]string) {
	if !equal {
		*problems = append(*problems, fmt.Sprintf("%s: %s differs", path, what))
	}
}

func diffPointer(a, b reflect.Value, path string, problems *[]string) {
	if a.IsNil() != b.IsNil() {
		*problems = append(*problems, fmt.Sprintf("%s: pointer nil-ness differs", path))
		return
	}
	if !a.IsNil() {
		diffReflect(a.Elem(), b.Elem(), path, problems)
	}
}

func diffInterface(a, b reflect.Value, path string, problems *[]string) {
	if a.IsNil() != b.IsNil() {
		*problems = append(*problems, fmt.Sprintf("%s: interface nil-ness differs", path))
		return
	}
	if a.IsNil() {
		return
	}
	if a.Elem().Type() != b.Elem().Type() {
		*problems = append(*problems, fmt.Sprintf("%s: interface dynamic type %s vs %s", path, a.Elem().Type(), b.Elem().Type()))
		return
	}
	diffReflect(a.Elem(), b.Elem(), path, problems)
}

func diffSlice(a, b reflect.Value, path string, problems *[]string) {
	if a.IsNil() != b.IsNil() {
		*problems = append(*problems, fmt.Sprintf("%s: slice nil-ness differs (nil vs empty is a difference here)", path))
		return
	}
	if a.Len() != b.Len() {
		*problems = append(*problems, fmt.Sprintf("%s: slice length %d vs %d", path, a.Len(), b.Len()))
		return
	}
	for i := 0; i < a.Len(); i++ {
		diffReflect(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i), problems)
	}
}

func diffArray(a, b reflect.Value, path string, problems *[]string) {
	for i := 0; i < a.Len(); i++ {
		diffReflect(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i), problems)
	}
}

func diffMap(a, b reflect.Value, path string, problems *[]string) {
	if a.IsNil() != b.IsNil() {
		*problems = append(*problems, fmt.Sprintf("%s: map nil-ness differs (nil vs empty is a difference here)", path))
		return
	}
	if a.IsNil() {
		return
	}
	if a.Len() != b.Len() {
		*problems = append(*problems, fmt.Sprintf("%s: map length %d vs %d", path, a.Len(), b.Len()))
		return
	}
	iter := a.MapRange()
	for iter.Next() {
		key := iter.Key()
		other := b.MapIndex(key)
		if !other.IsValid() {
			*problems = append(*problems, fmt.Sprintf("%s: key %v missing", path, key))
			continue
		}
		diffReflect(iter.Value(), other, fmt.Sprintf("%s[%v]", path, key), problems)
	}
}

func diffStruct(a, b reflect.Value, path string, problems *[]string) {
	t := a.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported: no value semantics (see the file header)
		}
		name := field.Name
		if field.Anonymous {
			name = ""
		}
		diffReflect(a.Field(i), b.Field(i), joinPath(path, name), problems)
	}
}

// --- reverse conversions ---------------------------------------------------

// copySlice copies a slice of values that carry no reference fields of their own. An element that holds a
// slice, map or pointer needs its own copy below: a shallow element copy leaves the mirror reachable through
// the output (F-B1 of the S13 trial review).
func copySlice[T any](in []T) []T {
	if in == nil {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// copyMap copies a map whose values carry no reference fields of their own (the pipeline constants are
// float64). A value type with references would need a typed copy, as the slice helpers below do.
func copyMap[K comparable, V any](in map[K]V) map[K]V {
	if in == nil {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// copyValuePtr copies the pointed-to value, so a mutation through the output never reaches the mirror.
func copyValuePtr[T any](in *T) *T {
	if in == nil {
		return nil
	}
	value := *in
	return &value
}

func copyBoolPtr(in bool) *bool {
	value := in
	return &value
}

// copyBindGroupLayoutEntries deep-copies the entry slice, including each entry's binding-layout pointer.
func copyBindGroupLayoutEntries(in []gputypes.BindGroupLayoutEntry) []gputypes.BindGroupLayoutEntry {
	if in == nil {
		return nil
	}
	out := make([]gputypes.BindGroupLayoutEntry, len(in))
	for i, entry := range in {
		out[i] = entry
		out[i].Buffer = copyValuePtr(entry.Buffer)
		out[i].Sampler = copyValuePtr(entry.Sampler)
		out[i].Texture = copyValuePtr(entry.Texture)
		out[i].StorageTexture = copyValuePtr(entry.StorageTexture)
		out[i].AccelerationStructure = copyValuePtr(entry.AccelerationStructure)
	}
	return out
}

// copyVertexBufferLayouts deep-copies the buffer layouts, including each layout's attribute slice.
func copyVertexBufferLayouts(in []gputypes.VertexBufferLayout) []gputypes.VertexBufferLayout {
	if in == nil {
		return nil
	}
	out := make([]gputypes.VertexBufferLayout, len(in))
	for i, layout := range in {
		out[i] = layout
		out[i].Attributes = copySlice(layout.Attributes)
	}
	return out
}

// copyColorTargetStates deep-copies the color targets, including each target's blend pointer.
func copyColorTargetStates(in []gputypes.ColorTargetState) []gputypes.ColorTargetState {
	if in == nil {
		return nil
	}
	out := make([]gputypes.ColorTargetState, len(in))
	for i, target := range in {
		out[i] = target
		out[i].Blend = copyValuePtr(target.Blend)
	}
	return out
}

// copyPrimitiveState deep-copies the primitive state, including its optional strip index format pointer.
func copyPrimitiveState(in gputypes.PrimitiveState) gputypes.PrimitiveState {
	out := in
	out.StripIndexFormat = copyValuePtr(in.StripIndexFormat)
	return out
}

func extent3DFromHAL(h *hal.Extent3D) Extent3D {
	return Extent3D{Width: h.Width, Height: h.Height, DepthOrArrayLayers: h.DepthOrArrayLayers}
}

func origin3DFromHAL(h *hal.Origin3D) Origin3D {
	return Origin3D{X: h.X, Y: h.Y, Z: h.Z}
}

func bufferDescriptorFromHAL(h *hal.BufferDescriptor) BufferDescriptor {
	return BufferDescriptor{Label: h.Label, Size: h.Size, Usage: h.Usage, MappedAtCreation: h.MappedAtCreation}
}

func textureDescriptorFromHAL(h *hal.TextureDescriptor) TextureDescriptor {
	return TextureDescriptor{
		Label:         h.Label,
		Size:          extent3DFromHAL(&h.Size),
		MipLevelCount: h.MipLevelCount,
		SampleCount:   h.SampleCount,
		Dimension:     h.Dimension,
		Format:        h.Format,
		Usage:         h.Usage,
		ViewFormats:   copySlice(h.ViewFormats),
	}
}

func textureViewDescriptorFromHAL(h *hal.TextureViewDescriptor) TextureViewDescriptor {
	return TextureViewDescriptor{
		Label:           h.Label,
		Format:          h.Format,
		Dimension:       h.Dimension,
		Aspect:          h.Aspect,
		BaseMipLevel:    h.BaseMipLevel,
		MipLevelCount:   h.MipLevelCount,
		BaseArrayLayer:  h.BaseArrayLayer,
		ArrayLayerCount: h.ArrayLayerCount,
	}
}

func samplerDescriptorFromHAL(h *hal.SamplerDescriptor) SamplerDescriptor {
	return SamplerDescriptor{
		Label:        h.Label,
		AddressModeU: h.AddressModeU,
		AddressModeV: h.AddressModeV,
		AddressModeW: h.AddressModeW,
		MagFilter:    h.MagFilter,
		MinFilter:    h.MinFilter,
		MipmapFilter: h.MipmapFilter,
		LodMinClamp:  h.LodMinClamp,
		LodMaxClamp:  h.LodMaxClamp,
		Compare:      h.Compare,
		Anisotropy:   h.Anisotropy,
	}
}

func querySetDescriptorFromHAL(h *hal.QuerySetDescriptor) QuerySetDescriptor {
	return QuerySetDescriptor{Label: h.Label, Type: QueryType(h.Type), Count: h.Count}
}

func shaderModuleDescriptorFromHAL(h *hal.ShaderModuleDescriptor) ShaderModuleDescriptor {
	return ShaderModuleDescriptor{Label: h.Label, WGSL: h.Source.WGSL, SPIRV: copySlice(h.Source.SPIRV)}
}

func commandEncoderDescriptorFromHAL(h *hal.CommandEncoderDescriptor) CommandEncoderDescriptor {
	return CommandEncoderDescriptor{Label: h.Label}
}

func bindGroupLayoutDescriptorFromHAL(h *hal.BindGroupLayoutDescriptor) BindGroupLayoutDescriptor {
	return BindGroupLayoutDescriptor{Label: h.Label, Entries: copyBindGroupLayoutEntries(h.Entries)}
}

func stencilFaceStateFromHAL(h *hal.StencilFaceState) StencilFaceState {
	return StencilFaceState{Compare: h.Compare, FailOp: h.FailOp, DepthFailOp: h.DepthFailOp, PassOp: h.PassOp}
}

func depthStencilStateFromHAL(h *hal.DepthStencilState) *DepthStencilState {
	if h == nil {
		return nil
	}
	return &DepthStencilState{
		Format:              h.Format,
		DepthWriteEnabled:   h.DepthWriteEnabled,
		DepthCompare:        h.DepthCompare,
		StencilFront:        stencilFaceStateFromHAL(&h.StencilFront),
		StencilBack:         stencilFaceStateFromHAL(&h.StencilBack),
		StencilReadMask:     h.StencilReadMask,
		StencilWriteMask:    h.StencilWriteMask,
		DepthBias:           h.DepthBias,
		DepthBiasSlopeScale: h.DepthBiasSlopeScale,
		DepthBiasClamp:      h.DepthBiasClamp,
	}
}

func renderPipelineDescriptorFromHAL(h *hal.RenderPipelineDescriptor) RenderPipelineDescriptor {
	descriptor := RenderPipelineDescriptor{
		Label: h.Label,
		Vertex: VertexState{
			EntryPoint: h.Vertex.EntryPoint,
			Constants:  copyMap(h.Vertex.Constants),
			Buffers:    copyVertexBufferLayouts(h.Vertex.Buffers),
		},
		Primitive:    copyPrimitiveState(h.Primitive),
		DepthStencil: depthStencilStateFromHAL(h.DepthStencil),
		Multisample:  h.Multisample,
	}
	if h.Fragment != nil {
		descriptor.Fragment = &FragmentState{
			EntryPoint: h.Fragment.EntryPoint,
			Constants:  copyMap(h.Fragment.Constants),
			Targets:    copyColorTargetStates(h.Fragment.Targets),
		}
	}
	return descriptor
}

func computePipelineDescriptorFromHAL(h *hal.ComputePipelineDescriptor) ComputePipelineDescriptor {
	return ComputePipelineDescriptor{
		Label:                         h.Label,
		EntryPoint:                    h.Compute.EntryPoint,
		Constants:                     copyMap(h.Compute.Constants),
		ZeroInitializeWorkgroupMemory: copyBoolPtr(h.Compute.ZeroInitializeWorkgroupMemory),
	}
}

func renderPassDescriptorFromHAL(h *hal.RenderPassDescriptor) RenderPassDescriptor {
	descriptor := RenderPassDescriptor{Label: h.Label}
	if h.ColorAttachments != nil {
		descriptor.ColorAttachments = make([]RenderPassColorAttachment, len(h.ColorAttachments))
		for i := range h.ColorAttachments {
			source := &h.ColorAttachments[i]
			descriptor.ColorAttachments[i] = RenderPassColorAttachment{
				DepthSlice: copyValuePtr(source.DepthSlice),
				LoadOp:     source.LoadOp,
				StoreOp:    source.StoreOp,
				ClearValue: source.ClearValue,
			}
		}
	}
	if h.DepthStencilAttachment != nil {
		descriptor.DepthStencilAttachment = renderPassDepthStencilFromHAL(h.DepthStencilAttachment)
	}
	if h.TimestampWrites != nil {
		descriptor.TimestampWrites = &RenderPassTimestampWrites{
			BeginningOfPassWriteIndex: copyValuePtr(h.TimestampWrites.BeginningOfPassWriteIndex),
			EndOfPassWriteIndex:       copyValuePtr(h.TimestampWrites.EndOfPassWriteIndex),
		}
	}
	return descriptor
}

func renderPassDepthStencilFromHAL(h *hal.RenderPassDepthStencilAttachment) *RenderPassDepthStencilAttachment {
	return &RenderPassDepthStencilAttachment{
		DepthLoadOp:       h.DepthLoadOp,
		DepthStoreOp:      h.DepthStoreOp,
		DepthClearValue:   h.DepthClearValue,
		DepthReadOnly:     h.DepthReadOnly,
		StencilLoadOp:     h.StencilLoadOp,
		StencilStoreOp:    h.StencilStoreOp,
		StencilClearValue: h.StencilClearValue,
		StencilReadOnly:   h.StencilReadOnly,
	}
}

func computePassDescriptorFromHAL(h *hal.ComputePassDescriptor) ComputePassDescriptor {
	descriptor := ComputePassDescriptor{Label: h.Label}
	if h.TimestampWrites != nil {
		descriptor.TimestampWrites = &ComputePassTimestampWrites{
			BeginningOfPassWriteIndex: copyValuePtr(h.TimestampWrites.BeginningOfPassWriteIndex),
			EndOfPassWriteIndex:       copyValuePtr(h.TimestampWrites.EndOfPassWriteIndex),
		}
	}
	return descriptor
}

func surfaceConfigurationFromHAL(h *hal.SurfaceConfiguration) SurfaceConfiguration {
	return SurfaceConfiguration{
		Width:       h.Width,
		Height:      h.Height,
		Format:      h.Format,
		Usage:       h.Usage,
		PresentMode: h.PresentMode,
		AlphaMode:   h.AlphaMode,
	}
}

func imageCopyTextureFromHAL(h *hal.ImageCopyTexture) *ImageCopyTexture {
	if h == nil {
		return nil
	}
	return &ImageCopyTexture{
		MipLevel: h.MipLevel,
		Origin:   origin3DFromHAL(&h.Origin),
		Aspect:   h.Aspect,
	}
}

// --- aliasing: the reverse conversions must not share memory ----------------

// TestReverseConversionDoesNotAlias mutates each mutable field of a reverse
// conversion's output — top-level and nested — and asserts the mirror input is
// unchanged. The forward toHAL is a read-only projection and deliberately
// shares slice, map and pointer backing with its input (descriptor.go); the
// reverse conversion is a copy, deep over the reference fields the public
// descriptors carry, so a roundtrip result never aliases the mirror it was
// built from. The nested cases are the ones a shallow element copy would have
// left shared (Attributes, Blend, a binding layout, the strip index format).
func TestReverseConversionDoesNotAlias(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func()
	}{
		{"TextureDescriptor.ViewFormats", func() {
			mirror := &hal.TextureDescriptor{ViewFormats: []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}}
			public := textureDescriptorFromHAL(mirror)
			public.ViewFormats[0] = gputypes.TextureFormatRGBA32Float
			if mirror.ViewFormats[0] != gputypes.TextureFormatRGBA8Unorm {
				t.Error("ViewFormats aliases the mirror")
			}
		}},
		{"ShaderModuleDescriptor.SPIRV", func() {
			mirror := &hal.ShaderModuleDescriptor{Source: hal.ShaderSource{SPIRV: []uint32{7}}}
			public := shaderModuleDescriptorFromHAL(mirror)
			public.SPIRV[0] = 9
			if mirror.Source.SPIRV[0] != 7 {
				t.Error("SPIRV aliases the mirror")
			}
		}},
		{"BindGroupLayoutDescriptor.Entries", func() {
			mirror := &hal.BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{Binding: 1}}}
			public := bindGroupLayoutDescriptorFromHAL(mirror)
			public.Entries[0].Binding = 2
			if mirror.Entries[0].Binding != 1 {
				t.Error("Entries aliases the mirror")
			}
		}},
		{"RenderPipelineDescriptor.VertexConstants", func() {
			mirror := &hal.RenderPipelineDescriptor{Vertex: hal.VertexState{Constants: map[string]float64{"a": 1}}}
			public := renderPipelineDescriptorFromHAL(mirror)
			public.Vertex.Constants["a"] = 2
			if mirror.Vertex.Constants["a"] != 1 {
				t.Error("Vertex.Constants aliases the mirror")
			}
		}},
		{"ComputePipelineDescriptor.Constants", func() {
			mirror := &hal.ComputePipelineDescriptor{Compute: hal.ComputeState{Constants: map[string]float64{"a": 1}}}
			public := computePipelineDescriptorFromHAL(mirror)
			public.Constants["a"] = 2
			if mirror.Compute.Constants["a"] != 1 {
				t.Error("Constants aliases the mirror")
			}
		}},
		{"RenderPassDescriptor.Pointers", func() {
			slice := uint32(3)
			begin := uint32(4)
			mirror := &hal.RenderPassDescriptor{
				ColorAttachments: []hal.RenderPassColorAttachment{{DepthSlice: &slice}},
				TimestampWrites:  &hal.RenderPassTimestampWrites{BeginningOfPassWriteIndex: &begin},
			}
			public := renderPassDescriptorFromHAL(mirror)
			*public.ColorAttachments[0].DepthSlice = 30
			*public.TimestampWrites.BeginningOfPassWriteIndex = 40
			if *mirror.ColorAttachments[0].DepthSlice != 3 || *mirror.TimestampWrites.BeginningOfPassWriteIndex != 4 {
				t.Error("a pointer field aliases the mirror")
			}
		}},
		{"RenderPipelineDescriptor.VertexBuffers.Attributes", func() {
			mirror := &hal.RenderPipelineDescriptor{Vertex: hal.VertexState{Buffers: []gputypes.VertexBufferLayout{{
				Attributes: []gputypes.VertexAttribute{{ShaderLocation: 1}},
			}}}}
			public := renderPipelineDescriptorFromHAL(mirror)
			public.Vertex.Buffers[0].Attributes[0].ShaderLocation = 2
			if mirror.Vertex.Buffers[0].Attributes[0].ShaderLocation != 1 {
				t.Error("a nested Attributes slice aliases the mirror")
			}
		}},
		{"RenderPipelineDescriptor.FragmentTargets.Blend", func() {
			blend := &gputypes.BlendState{Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne}}
			mirror := &hal.RenderPipelineDescriptor{Fragment: &hal.FragmentState{Targets: []gputypes.ColorTargetState{{Blend: blend}}}}
			public := renderPipelineDescriptorFromHAL(mirror)
			public.Fragment.Targets[0].Blend.Color.SrcFactor = gputypes.BlendFactorZero
			if mirror.Fragment.Targets[0].Blend.Color.SrcFactor != gputypes.BlendFactorOne {
				t.Error("a nested Blend pointer aliases the mirror")
			}
		}},
		{"BindGroupLayoutDescriptor.Entries.Buffer", func() {
			mirror := &hal.BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
				Binding: 1,
				Buffer:  &gputypes.BufferBindingLayout{MinBindingSize: 4},
			}}}
			public := bindGroupLayoutDescriptorFromHAL(mirror)
			public.Entries[0].Buffer.MinBindingSize = 8
			if mirror.Entries[0].Buffer.MinBindingSize != 4 {
				t.Error("a nested binding-layout pointer aliases the mirror")
			}
		}},
		{"RenderPipelineDescriptor.Primitive.StripIndexFormat", func() {
			strip := gputypes.IndexFormatUint32
			mirror := &hal.RenderPipelineDescriptor{Primitive: gputypes.PrimitiveState{StripIndexFormat: &strip}}
			public := renderPipelineDescriptorFromHAL(mirror)
			*public.Primitive.StripIndexFormat = gputypes.IndexFormatUint16
			if *mirror.Primitive.StripIndexFormat != gputypes.IndexFormatUint32 {
				t.Error("the strip index format pointer aliases the mirror")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.mutate()
		})
	}
}

// TestReverseConversionConcurrent is the signed Amendment 2.3 adoption, "-race for shared-slice mirrors":
// the forward toHAL shares slice, map and pointer backing with its input, so several goroutines must be
// able to convert one shared mirror at the same time without touching it. Without -race this test proves
// the values stay right under concurrency; the memory question belongs to the race detector, and that leg
// runs in CI and in WSL — this Windows host cannot run -race at all (no gcc).
func TestReverseConversionConcurrent(t *testing.T) {
	t.Parallel()
	blend := &gputypes.BlendState{Color: gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne}}
	strip := gputypes.IndexFormatUint32
	texture := &hal.TextureDescriptor{ViewFormats: []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm}}
	layout := &hal.BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
		Binding: 1,
		Buffer:  &gputypes.BufferBindingLayout{MinBindingSize: 4},
	}}}
	pipeline := &hal.RenderPipelineDescriptor{
		Vertex:    hal.VertexState{Buffers: []gputypes.VertexBufferLayout{{Attributes: []gputypes.VertexAttribute{{ShaderLocation: 1}}}}},
		Primitive: gputypes.PrimitiveState{StripIndexFormat: &strip},
		Fragment:  &hal.FragmentState{Targets: []gputypes.ColorTargetState{{Blend: blend}}},
	}
	wantTexture := textureDescriptorFromHAL(texture)
	wantLayout := bindGroupLayoutDescriptorFromHAL(layout)
	wantPipeline := renderPipelineDescriptorFromHAL(pipeline)

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < 32; round++ {
				if problems := diffValue(wantTexture, textureDescriptorFromHAL(texture)); len(problems) != 0 {
					t.Errorf("worker %d: TextureDescriptor round %d differs: %v", worker, round, problems)
				}
				if problems := diffValue(wantLayout, bindGroupLayoutDescriptorFromHAL(layout)); len(problems) != 0 {
					t.Errorf("worker %d: BindGroupLayoutDescriptor round %d differs: %v", worker, round, problems)
				}
				if problems := diffValue(wantPipeline, renderPipelineDescriptorFromHAL(pipeline)); len(problems) != 0 {
					t.Errorf("worker %d: RenderPipelineDescriptor round %d differs: %v", worker, round, problems)
				}
			}
		}()
	}
	wg.Wait()
}

// --- committed zero-value expectations -------------------------------------

// TestZeroValueExpectations pins the WebGPU defaults the NORMALIZING chains
// apply, so a change to a normalize* function is a visible test change. The
// defaults are the ones documented in descriptor.go, pipeline_defaults.go and
// texture_sampler_defaults.go.
func TestZeroValueExpectations(t *testing.T) {
	t.Parallel()

	t.Run("SamplerDescriptor", func(t *testing.T) {
		t.Parallel()
		want := hal.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MagFilter:    gputypes.FilterModeNearest,
			MinFilter:    gputypes.FilterModeNearest,
			MipmapFilter: gputypes.FilterModeNearest,
			LodMaxClamp:  32,
			Anisotropy:   1,
		}
		assertNoDiff(t, (&SamplerDescriptor{}).toHAL(), &want)
	})

	t.Run("RenderPipelineDescriptor", func(t *testing.T) {
		t.Parallel()
		want := hal.RenderPipelineDescriptor{Multisample: gputypes.MultisampleState{Mask: 0xFFFFFFFF}}
		assertNoDiff(t, (&RenderPipelineDescriptor{}).toHAL(), &want)
	})

	t.Run("TextureDescriptorPath", func(t *testing.T) {
		t.Parallel()
		// The pair's toHAL is pure (zero stays zero); the public CreateTexture
		// path resolves the defaults first (device_native.go:82).
		normalized := normalizeTextureDescriptor(&TextureDescriptor{})
		want := hal.TextureDescriptor{
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     gputypes.TextureDimension2D,
			Size:          hal.Extent3D{Height: 1, DepthOrArrayLayers: 1},
		}
		assertNoDiff(t, normalized.toHAL(), &want)
	})

	// ComputePipelineDescriptor's nil -> true default is only read when the
	// module resolves (descriptor.go:454-468); without a live shader module the
	// mirror's Compute state stays zero, so there is no observable expectation
	// here. The normalization itself is covered by the idempotence fuzz target.
}

func assertNoDiff[T any](t *testing.T, got, want *T) {
	t.Helper()
	for _, problem := range diffValue(*got, *want) {
		t.Error(problem)
	}
}

// --- zero-allocation conversions -------------------------------------------

// Alloc sinks keep the compiler from eliminating the conversions under test.
var (
	sinkExtent3D       hal.Extent3D
	sinkOrigin3D       hal.Origin3D
	sinkStencilFace    hal.StencilFaceState
	sinkExtent3DFrom   Extent3D
	sinkOrigin3DFrom   Origin3D
	sinkStencilFaceRev StencilFaceState
)

// TestZeroAllocConversions pins the conversions that are provably zero-alloc:
// the three value-in-parent pairs, whose toHAL returns a struct by value and
// whose reverse copy is a plain struct assignment. Every other pair allocates
// (toHAL returns a pointer; the reverse conversions allocate their copies), so
// no assertion is invented for them.
func TestZeroAllocConversions(t *testing.T) {
	// AllocsPerRun must not run under t.Parallel: it pins GOMAXPROCS itself.
	const runs = 100
	extent := Extent3D{Width: 1, Height: 2, DepthOrArrayLayers: 3}
	halExtent := hal.Extent3D{Width: 1, Height: 2, DepthOrArrayLayers: 3}
	origin := Origin3D{X: 1, Y: 2, Z: 3}
	halOrigin := hal.Origin3D{X: 1, Y: 2, Z: 3}
	face := StencilFaceState{Compare: gputypes.CompareFunctionAlways}
	halFace := hal.StencilFaceState{Compare: gputypes.CompareFunctionAlways}

	checks := []struct {
		name string
		fn   func()
	}{
		{"Extent3D.toHAL", func() { sinkExtent3D = extent.toHAL() }},
		{"Origin3D.toHAL", func() { sinkOrigin3D = origin.toHAL() }},
		{"StencilFaceState.toHAL", func() { sinkStencilFace = face.toHAL() }},
		{"extent3DFromHAL", func() { sinkExtent3DFrom = extent3DFromHAL(&halExtent) }},
		{"origin3DFromHAL", func() { sinkOrigin3DFrom = origin3DFromHAL(&halOrigin) }},
		{"stencilFaceStateFromHAL", func() { sinkStencilFaceRev = stencilFaceStateFromHAL(&halFace) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if allocs := testing.AllocsPerRun(runs, check.fn); allocs != 0 {
				t.Errorf("%s allocates %v times per run, want 0", check.name, allocs)
			}
		})
	}
}
