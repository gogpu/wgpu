//go:build !rust && !(js && wasm)

package wgpu

// Descriptor projection guard (S13 O2).
//
// The public descriptors convert to their HAL mirrors by hand (descriptor.go).
// A hand-written conversion drifts silently: a public field can be added and
// never carried, a mirror field added and never set, or a field renamed on one
// side only. This file makes the projection explicit, compiled and reviewed.
//
// The shape follows toolhive's AssertNoDrift (stacklok/toolhive PR #5209,
// cmd/thv-operator/internal/testutil/drift.go): the exported leaf paths of both
// types are enumerated, and every leaf must be either mapped to its counterpart
// or ignored with a non-empty justification. The sanity set here is the full
// one and goes beyond toolhive, which has no non-empty table/ignore check
// (recorded in PRACTICES-CONSENSUS.md, Amendment 1/2).
//
// Leaf-path rules. These are stated because a naive port under-enumerates
// silently:
//
//  1. A path is a dot-separated GO field path, not a JSON path: the
//     descriptors carry no struct tags.
//  2. Containers are transparent: a pointer, slice, array or map contributes
//     no path element of its own, and recursion continues into the element
//     type (for a map, the value type; keys are never enumerated). So
//     "ViewFormats" is one leaf whether it is []T or T, and
//     "ColorAttachments.View" names a field inside the slice element.
//  3. Structs declared in this package or in internal/hal are recursed into.
//     Embedded structs are flattened: their fields take the outer path.
//     Unexported fields are excluded entirely.
//  4. A struct with no exported fields is a leaf. That is the opaque public
//     handle (TextureView, QuerySet, ShaderModule, PipelineLayout): the
//     conversion carries the handle, not its internals.
//  5. A struct declared in another package (gputypes, naga ir, ...) is a leaf.
//     Both sides hold the same type there, so the compiler already forces the
//     conversion to carry it whole; recursing would enumerate fields no
//     conversion touches (ir.Module alone would add hundreds).
//  6. A type already on the current recursion path is a leaf (cycle guard).
//  7. Interfaces and function types are leaves.
//
// The same generic check runs in the reverse direction over invertDrift, so the test-only reverse
// conversions in descriptor_roundtrip_test.go are enumerated too. This table proves COVERAGE, in both
// directions: every leaf is mapped or ignored with a justification, and a leaf added later has no row and
// fails here. It does not prove VALUES: a conversion that leaves a mapped field unassigned still has its row
// and passes here. Values are proved by the roundtrip property in descriptor_fuzz_test.go — a fromHAL that
// forgets a field fails that roundtrip, not this table — and the concrete per-field values are pinned by
// descriptor_test.go (ImageCopyTexture Origin included).
//
// This test classifies paths; it says nothing about VALUE semantics. nil vs
// empty identity is decided by the fuzz comparator in
// descriptor_roundtrip_test.go, which is deliberately stricter than
// Kubernetes' apiequality.Semantic.DeepEqual (that one treats nil and empty
// slices/maps as equal).

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

const (
	wgpuPackagePath = "github.com/gogpu/wgpu"
	halPackagePath  = "github.com/gogpu/wgpu/internal/hal"
)

// DriftSpec classifies every leaf path of one public/mirror pair.
//
// Mappings is public leaf path -> mirror leaf path. IgnoredOnPublicOnly holds
// public leaves with no mirror counterpart (path -> justification);
// IgnoredOnMirrorOnly holds mirror leaves with no public counterpart. A path
// appears in exactly one of the three maps.
type DriftSpec struct {
	Mappings            map[string]string
	IgnoredOnPublicOnly map[string]string
	IgnoredOnMirrorOnly map[string]string
}

// AssertNoDrift enumerates the exported leaf paths of Public and Mirror and
// reports every classification problem in spec.
func AssertNoDrift[Public, Mirror any](t *testing.T, spec DriftSpec) {
	t.Helper()
	public := leafPaths(reflect.TypeOf((*Public)(nil)).Elem())
	mirror := leafPaths(reflect.TypeOf((*Mirror)(nil)).Elem())
	for _, problem := range checkDrift(public, mirror, spec) {
		t.Error(problem)
	}
}

// leafPaths returns the sorted exported leaf paths of t.
func leafPaths(t reflect.Type) []string {
	out := make([]string, 0, 16)
	collectLeafPaths(t, "", make(map[reflect.Type]bool), &out)
	sort.Strings(out)
	return out
}

// collectLeafPaths appends the leaf paths of t below path. pathTypes holds the
// structs on the current recursion path, which is the cycle guard.
func collectLeafPaths(t reflect.Type, path string, pathTypes map[reflect.Type]bool, out *[]string) {
	for isTransparentContainer(t.Kind()) {
		t = t.Elem()
	}
	if !shouldRecurse(t, pathTypes) {
		*out = append(*out, path)
		return
	}
	pathTypes[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		name := field.Name
		if field.Anonymous {
			name = "" // embedded structs are flattened
		}
		collectLeafPaths(field.Type, joinPath(path, name), pathTypes, out)
	}
	delete(pathTypes, t)
}

// isTransparentContainer reports whether kind contributes no path element.
func isTransparentContainer(kind reflect.Kind) bool {
	switch kind {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return true
	default:
		return false
	}
}

// shouldRecurse reports whether t is a struct this test looks inside.
func shouldRecurse(t reflect.Type, pathTypes map[reflect.Type]bool) bool {
	if t.Kind() != reflect.Struct || pathTypes[t] {
		return false
	}
	if !ownedStructType(t) {
		return false
	}
	return hasExportedField(t)
}

// ownedStructType reports whether t is declared in one of the two packages the
// conversion joins (rule 5: any other package's struct is a leaf).
func ownedStructType(t reflect.Type) bool {
	pkg := t.PkgPath()
	return pkg == "" || pkg == wgpuPackagePath || pkg == halPackagePath
}

func hasExportedField(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).PkgPath == "" {
			return true
		}
	}
	return false
}

func joinPath(prefix, name string) string {
	switch {
	case prefix == "":
		return name
	case name == "":
		return prefix
	default:
		return prefix + "." + name
	}
}

// checkDrift returns every classification problem, sorted for a stable report.
func checkDrift(public, mirror []string, spec DriftSpec) []string {
	publicSet := stringSet(public)
	mirrorSet := stringSet(mirror)
	problems := checkSpecShape(public, mirror, spec)
	problems = append(problems, checkStaleEntries(publicSet, mirrorSet, spec)...)
	problems = append(problems, checkCoverage(public, mirror, spec)...)
	sort.Strings(problems)
	return problems
}

// checkSpecShape checks the table itself: non-empty, no empty or duplicate
// entries, no path both mapped and ignored, no path ignored on both sides,
// and a justification on every ignored entry.
func checkSpecShape(public, mirror []string, spec DriftSpec) []string {
	var problems []string
	if len(public) > 0 && len(mirror) > 0 && len(spec.Mappings) == 0 {
		problems = append(problems, "empty mapping table: every leaf would be unclassified")
	}
	problems = append(problems, checkMappings(spec.Mappings)...)
	problems = append(problems, checkIgnores("public", spec.IgnoredOnPublicOnly)...)
	problems = append(problems, checkIgnores("mirror", spec.IgnoredOnMirrorOnly)...)
	for path := range spec.IgnoredOnPublicOnly {
		if _, ok := spec.IgnoredOnMirrorOnly[path]; ok {
			problems = append(problems, fmt.Sprintf("path %q is ignored on both sides (cross-pollination)", path))
		}
		if _, ok := spec.Mappings[path]; ok {
			problems = append(problems, fmt.Sprintf("path %q is both mapped and ignored on the public side", path))
		}
	}
	for _, target := range spec.Mappings {
		if _, ok := spec.IgnoredOnMirrorOnly[target]; ok {
			problems = append(problems, fmt.Sprintf("path %q is both mapped and ignored on the mirror side", target))
		}
	}
	return problems
}

func checkMappings(mappings map[string]string) []string {
	var problems []string
	targets := make(map[string]string, len(mappings))
	for source, target := range mappings {
		if source == "" {
			problems = append(problems, "empty mapping source path")
		}
		if target == "" {
			problems = append(problems, fmt.Sprintf("empty mapping target path for source %q", source))
		}
		if first, ok := targets[target]; ok && target != "" {
			problems = append(problems, fmt.Sprintf("duplicate mapping target %q (sources %q and %q)", target, first, source))
		}
		targets[target] = source
	}
	return problems
}

func checkIgnores(side string, ignores map[string]string) []string {
	var problems []string
	for path, justification := range ignores {
		if path == "" {
			problems = append(problems, fmt.Sprintf("empty ignored path on the %s side", side))
		}
		if justification == "" {
			problems = append(problems, fmt.Sprintf("empty justification for %q on the %s side", path, side))
		}
	}
	return problems
}

// checkStaleEntries fails on an entry naming a path that no longer exists.
func checkStaleEntries(publicSet, mirrorSet map[string]bool, spec DriftSpec) []string {
	var problems []string
	for source, target := range spec.Mappings {
		if !publicSet[source] {
			problems = append(problems, fmt.Sprintf("stale mapping source %q: not a public leaf", source))
		}
		if !mirrorSet[target] {
			problems = append(problems, fmt.Sprintf("stale mapping target %q: not a mirror leaf", target))
		}
	}
	for path := range spec.IgnoredOnPublicOnly {
		if !publicSet[path] {
			problems = append(problems, fmt.Sprintf("stale ignored-on-public entry %q: not a public leaf", path))
		}
	}
	for path := range spec.IgnoredOnMirrorOnly {
		if !mirrorSet[path] {
			problems = append(problems, fmt.Sprintf("stale ignored-on-mirror entry %q: not a mirror leaf", path))
		}
	}
	return problems
}

// checkCoverage fails on a leaf that is neither mapped nor ignored.
func checkCoverage(public, mirror []string, spec DriftSpec) []string {
	var problems []string
	targeted := make(map[string]bool, len(spec.Mappings))
	for _, target := range spec.Mappings {
		targeted[target] = true
	}
	for _, path := range public {
		if _, ok := spec.Mappings[path]; ok {
			continue
		}
		if _, ok := spec.IgnoredOnPublicOnly[path]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("unclassified public leaf %q", path))
	}
	for _, path := range mirror {
		if targeted[path] {
			continue
		}
		if _, ok := spec.IgnoredOnMirrorOnly[path]; ok {
			continue
		}
		problems = append(problems, fmt.Sprintf("unclassified mirror leaf %q", path))
	}
	return problems
}

func stringSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[path] = true
	}
	return set
}

// invertDrift returns the spec for the reverse direction (Mirror -> Public).
// Mappings swap, and the ignore sides swap with them; the result is exact by
// construction, so the reverse conversion cannot drift from the forward table.
func invertDrift(spec DriftSpec) DriftSpec {
	inverted := DriftSpec{
		Mappings:            make(map[string]string, len(spec.Mappings)),
		IgnoredOnPublicOnly: copyJustifications(spec.IgnoredOnMirrorOnly),
		IgnoredOnMirrorOnly: copyJustifications(spec.IgnoredOnPublicOnly),
	}
	for source, target := range spec.Mappings {
		inverted.Mappings[target] = source
	}
	return inverted
}

func copyJustifications(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for path, justification := range in {
		out[path] = justification
	}
	return out
}

// same builds the identity mapping: every listed leaf maps to its namesake on
// the mirror. The list is the reviewed artifact; the map is its expansion.
func same(paths ...string) map[string]string {
	mappings := make(map[string]string, len(paths))
	for _, path := range paths {
		mappings[path] = path
	}
	return mappings
}

// --- pair inventory and projection specs -----------------------------------
//
// Class per pair (PRACTICES-CONSENSUS.md O2/O3): NORMALIZING when its
// conversion chain applies normalize*, PURE otherwise. Every class below was
// measured by reading the cited lines; an unclear pair would be NORMALIZING
// until measured. The spec carries the mapping; the class matters to the fuzz
// property in descriptor_roundtrip_test.go.

// Extent3D — PURE (descriptor.go:17-19). Value-in-parent pair.
var extent3DDrift = DriftSpec{
	Mappings: same("Width", "Height", "DepthOrArrayLayers"),
}

// Origin3D — PURE (descriptor.go:28-30). Value-in-parent pair.
var origin3DDrift = DriftSpec{
	Mappings: same("X", "Y", "Z"),
}

// BufferDescriptor — PURE (descriptor.go:52-59).
var bufferDescriptorDrift = DriftSpec{
	Mappings: same("Label", "Size", "Usage", "MappedAtCreation"),
}

// TextureDescriptor — PURE pair, NORMALIZING public path (descriptor.go:78-89;
// device_native.go:82 calls normalizeTextureDescriptor, texture_sampler_defaults.go:50,
// before the conversion). The zero-value defaults therefore never reach this
// toHAL from a public CreateTexture call.
var textureDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label", "Size.Width", "Size.Height", "Size.DepthOrArrayLayers",
		"MipLevelCount", "SampleCount", "Dimension", "Format", "Usage", "ViewFormats",
	),
}

// TextureViewDescriptor — PURE (descriptor.go:104-115).
var textureViewDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label", "Format", "Dimension", "Aspect",
		"BaseMipLevel", "MipLevelCount", "BaseArrayLayer", "ArrayLayerCount",
	),
}

// SamplerDescriptor — NORMALIZING (descriptor.go:144-158): normalizeAddressMode,
// normalizeFilterMode, normalizeLodMaxClamp (0 -> 32) and normalizeAnisotropy
// (0 -> 1) run inside toHAL.
var samplerDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label", "AddressModeU", "AddressModeV", "AddressModeW",
		"MagFilter", "MinFilter", "MipmapFilter",
		"LodMinClamp", "LodMaxClamp", "Compare", "Anisotropy",
	),
}

// QuerySetDescriptor — PURE (descriptor.go:183-189). Type is a named type on
// both sides (root QueryType vs hal.QueryType) and crosses by conversion.
var querySetDescriptorDrift = DriftSpec{
	Mappings: same("Label", "Type", "Count"),
}

// ShaderModuleDescriptor — PURE (descriptor.go:199-207). The mirror carries IR
// and Info, which core fills after parsing; no public field produces them.
var shaderModuleDescriptorDrift = DriftSpec{
	Mappings: map[string]string{
		"Label": "Label",
		"WGSL":  "Source.WGSL",
		"SPIRV": "Source.SPIRV",
	},
	IgnoredOnMirrorOnly: map[string]string{
		"IR":   "naga IR of Source.WGSL, parsed and validated once by core (hal/descriptor.go:370); no public field produces it",
		"Info": "valid.ModuleInfo of IR (hal/descriptor.go:374); no public field produces it",
	},
}

// CommandEncoderDescriptor — PURE (descriptor.go:231-235).
var commandEncoderDescriptorDrift = DriftSpec{
	Mappings: same("Label"),
}

// BindGroupLayoutDescriptor — PURE (descriptor.go:244-249). Entries is a
// gputypes slice shared by both sides, hence one leaf (rule 5).
var bindGroupLayoutDescriptorDrift = DriftSpec{
	Mappings: same("Label", "Entries"),
}

// StencilFaceState — PURE (descriptor.go:295-297). Value-in-parent pair.
var stencilFaceStateDrift = DriftSpec{
	Mappings: same("Compare", "FailOp", "DepthFailOp", "PassOp"),
}

// DepthStencilState — PURE (descriptor.go:317-333): toHAL passes every field
// through and delegates the two faces to StencilFaceState.toHAL.
var depthStencilStateDrift = DriftSpec{
	Mappings: same(
		"Format", "DepthWriteEnabled", "DepthCompare",
		"StencilFront.Compare", "StencilFront.FailOp", "StencilFront.DepthFailOp", "StencilFront.PassOp",
		"StencilBack.Compare", "StencilBack.FailOp", "StencilBack.DepthFailOp", "StencilBack.PassOp",
		"StencilReadMask", "StencilWriteMask",
		"DepthBias", "DepthBiasSlopeScale", "DepthBiasClamp",
	),
}

// RenderPipelineDescriptor — NORMALIZING (descriptor.go:385-416):
// normalizeMultisample (:389), normalizeDepthStencilState (:390) and
// normalizeColorTargets (:411) run inside toHAL.
var renderPipelineDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label", "Layout", "Primitive", "Multisample",
		"Vertex.Module", "Vertex.EntryPoint", "Vertex.Constants", "Vertex.Buffers",
		"DepthStencil.Format", "DepthStencil.DepthWriteEnabled", "DepthStencil.DepthCompare",
		"DepthStencil.StencilFront.Compare", "DepthStencil.StencilFront.FailOp",
		"DepthStencil.StencilFront.DepthFailOp", "DepthStencil.StencilFront.PassOp",
		"DepthStencil.StencilBack.Compare", "DepthStencil.StencilBack.FailOp",
		"DepthStencil.StencilBack.DepthFailOp", "DepthStencil.StencilBack.PassOp",
		"DepthStencil.StencilReadMask", "DepthStencil.StencilWriteMask",
		"DepthStencil.DepthBias", "DepthStencil.DepthBiasSlopeScale", "DepthStencil.DepthBiasClamp",
		"Fragment.Module", "Fragment.EntryPoint", "Fragment.Constants", "Fragment.Targets",
	),
}

// ComputePipelineDescriptor — NORMALIZING (descriptor.go:445-471): a nil
// ZeroInitializeWorkgroupMemory becomes true (:454-459). The public stage
// fields live inside the mirror's Compute state, so the paths cross.
var computePipelineDescriptorDrift = DriftSpec{
	Mappings: map[string]string{
		"Label":                         "Label",
		"Layout":                        "Layout",
		"Module":                        "Compute.Module",
		"EntryPoint":                    "Compute.EntryPoint",
		"Constants":                     "Compute.Constants",
		"ZeroInitializeWorkgroupMemory": "Compute.ZeroInitializeWorkgroupMemory",
	},
}

// RenderPassDescriptor — PURE (descriptor.go:518-576; no normalize* in the
// chain). Path note: TimestampWrites is dropped entirely when no non-nil query
// set resolves (:560-567), and ColorAttachments.View/ResolveTarget require a
// live view (:533-538); the fuzz property records that reachability.
var renderPassDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label",
		"ColorAttachments.View", "ColorAttachments.ResolveTarget", "ColorAttachments.DepthSlice",
		"ColorAttachments.LoadOp", "ColorAttachments.StoreOp", "ColorAttachments.ClearValue",
		"DepthStencilAttachment.View",
		"DepthStencilAttachment.DepthLoadOp", "DepthStencilAttachment.DepthStoreOp",
		"DepthStencilAttachment.DepthClearValue", "DepthStencilAttachment.DepthReadOnly",
		"DepthStencilAttachment.StencilLoadOp", "DepthStencilAttachment.StencilStoreOp",
		"DepthStencilAttachment.StencilClearValue", "DepthStencilAttachment.StencilReadOnly",
		"TimestampWrites.QuerySet", "TimestampWrites.BeginningOfPassWriteIndex",
		"TimestampWrites.EndOfPassWriteIndex",
	),
}

// ComputePassDescriptor — PURE (descriptor.go:596-614). Same timestamp-write
// drop as the render pass when no query set resolves.
var computePassDescriptorDrift = DriftSpec{
	Mappings: same(
		"Label",
		"TimestampWrites.QuerySet", "TimestampWrites.BeginningOfPassWriteIndex",
		"TimestampWrites.EndOfPassWriteIndex",
	),
}

// SurfaceConfiguration — PURE (descriptor.go:627-636). The mirror's
// EnableDamagePresent has no public source: the DX12 surface reads it
// (hal/dx12/surface.go:65), but this conversion never sets it.
var surfaceConfigurationDrift = DriftSpec{
	Mappings: same("Width", "Height", "Format", "Usage", "PresentMode", "AlphaMode"),
	IgnoredOnMirrorOnly: map[string]string{
		"EnableDamagePresent": "no public SurfaceConfiguration field produces it; this conversion leaves it false (internal/hal/descriptor.go:185-195)",
	},
}

// ImageCopyTexture — PURE (descriptor.go:646-657; no normalize*). Path note:
// toHAL returns nil unless Texture.impl resolves, so Origin/MipLevel/Aspect
// cross only with a live texture.
var imageCopyTextureDrift = DriftSpec{
	Mappings: same("Texture", "MipLevel", "Origin.X", "Origin.Y", "Origin.Z", "Aspect"),
}

// TestDescriptorProjection checks every pair in both directions. The reverse
// direction enumerates the test-only fromHAL conversion (descriptor_roundtrip_test.go),
// so a mirror leaf no reverse conversion maps has no row and fails here. That is
// coverage, not values: a field the conversion merely forgets to assign passes
// this table and is caught by the roundtrip property.
func TestDescriptorProjection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		assert func(*testing.T)
	}{
		{"Extent3D", func(t *testing.T) { assertPair[Extent3D, hal.Extent3D](t, extent3DDrift) }},
		{"Origin3D", func(t *testing.T) { assertPair[Origin3D, hal.Origin3D](t, origin3DDrift) }},
		{"BufferDescriptor", func(t *testing.T) { assertPair[BufferDescriptor, hal.BufferDescriptor](t, bufferDescriptorDrift) }},
		{"TextureDescriptor", func(t *testing.T) { assertPair[TextureDescriptor, hal.TextureDescriptor](t, textureDescriptorDrift) }},
		{"TextureViewDescriptor", func(t *testing.T) {
			assertPair[TextureViewDescriptor, hal.TextureViewDescriptor](t, textureViewDescriptorDrift)
		}},
		{"SamplerDescriptor", func(t *testing.T) { assertPair[SamplerDescriptor, hal.SamplerDescriptor](t, samplerDescriptorDrift) }},
		{"QuerySetDescriptor", func(t *testing.T) { assertPair[QuerySetDescriptor, hal.QuerySetDescriptor](t, querySetDescriptorDrift) }},
		{"ShaderModuleDescriptor", func(t *testing.T) {
			assertPair[ShaderModuleDescriptor, hal.ShaderModuleDescriptor](t, shaderModuleDescriptorDrift)
		}},
		{"CommandEncoderDescriptor", func(t *testing.T) {
			assertPair[CommandEncoderDescriptor, hal.CommandEncoderDescriptor](t, commandEncoderDescriptorDrift)
		}},
		{"BindGroupLayoutDescriptor", func(t *testing.T) {
			assertPair[BindGroupLayoutDescriptor, hal.BindGroupLayoutDescriptor](t, bindGroupLayoutDescriptorDrift)
		}},
		{"StencilFaceState", func(t *testing.T) { assertPair[StencilFaceState, hal.StencilFaceState](t, stencilFaceStateDrift) }},
		{"DepthStencilState", func(t *testing.T) { assertPair[DepthStencilState, hal.DepthStencilState](t, depthStencilStateDrift) }},
		{"RenderPipelineDescriptor", func(t *testing.T) {
			assertPair[RenderPipelineDescriptor, hal.RenderPipelineDescriptor](t, renderPipelineDescriptorDrift)
		}},
		{"ComputePipelineDescriptor", func(t *testing.T) {
			assertPair[ComputePipelineDescriptor, hal.ComputePipelineDescriptor](t, computePipelineDescriptorDrift)
		}},
		{"RenderPassDescriptor", func(t *testing.T) {
			assertPair[RenderPassDescriptor, hal.RenderPassDescriptor](t, renderPassDescriptorDrift)
		}},
		{"ComputePassDescriptor", func(t *testing.T) {
			assertPair[ComputePassDescriptor, hal.ComputePassDescriptor](t, computePassDescriptorDrift)
		}},
		{"SurfaceConfiguration", func(t *testing.T) {
			assertPair[SurfaceConfiguration, hal.SurfaceConfiguration](t, surfaceConfigurationDrift)
		}},
		{"ImageCopyTexture", func(t *testing.T) { assertPair[ImageCopyTexture, hal.ImageCopyTexture](t, imageCopyTextureDrift) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t)
		})
	}
}

// assertPair checks the forward spec and the inverted spec of the reverse
// conversion.
func assertPair[Public, Mirror any](t *testing.T, spec DriftSpec) {
	t.Helper()
	AssertNoDrift[Public, Mirror](t, spec)
	t.Run("reverse", func(t *testing.T) {
		t.Helper()
		AssertNoDrift[Mirror, Public](t, invertDrift(spec))
	})
}

// --- synthetic-pair self-test (permanent negative control) -----------------
//
// A deliberately broken fixture pair runs the enumerator and the whole sanity
// set, so a later refactor of the test itself cannot quietly turn the guard
// off. It is in-tree; the temporary field-add/rename runs against the real
// types are recorded in the S13 record, not performed from the test.

type driftFixturePublic struct {
	Mapped           int
	Unmapped         int
	DupA             int
	DupB             int
	MappedAndIgnored int
	BothIgnored      int
	PublicOnly       int
}

type driftFixtureMirror struct {
	Mapped           int
	MirrorA          int
	MirrorB          int
	MirrorC          int
	MappedAndIgnored int
	BothIgnored      int
	MirrorOnly       int
}

func validFixtureSpec() DriftSpec {
	return DriftSpec{
		Mappings: map[string]string{
			"Mapped":           "Mapped",
			"Unmapped":         "MirrorA",
			"DupA":             "MirrorB",
			"DupB":             "MirrorC",
			"MappedAndIgnored": "MappedAndIgnored",
			"BothIgnored":      "BothIgnored",
		},
		IgnoredOnPublicOnly: map[string]string{"PublicOnly": "fixture: public-only field"},
		IgnoredOnMirrorOnly: map[string]string{"MirrorOnly": "fixture: mirror-only field"},
	}
}

func TestDriftSanitySetBites(t *testing.T) {
	t.Parallel()
	public := leafPaths(reflect.TypeOf(driftFixturePublic{}))
	mirror := leafPaths(reflect.TypeOf(driftFixtureMirror{}))
	if problems := checkDrift(public, mirror, validFixtureSpec()); len(problems) != 0 {
		t.Fatalf("the valid fixture spec must be clean, got %v", problems)
	}

	cases := []struct {
		name   string
		mutate func(*DriftSpec)
		want   string
	}{
		{"unmapped leaf", func(s *DriftSpec) { delete(s.Mappings, "Unmapped") }, `unclassified public leaf "Unmapped"`},
		{"stale mapping source", func(s *DriftSpec) { s.Mappings["Ghost"] = s.Mappings["Unmapped"]; delete(s.Mappings, "Unmapped") }, `stale mapping source "Ghost"`},
		{"stale mapping target", func(s *DriftSpec) { s.Mappings["DupB"] = "Ghost" }, `stale mapping target "Ghost"`},
		{"stale ignore", func(s *DriftSpec) { s.IgnoredOnPublicOnly["Ghost"] = "fixture" }, `stale ignored-on-public entry "Ghost"`},
		{"duplicate mapping", func(s *DriftSpec) { s.Mappings["DupB"] = s.Mappings["Unmapped"] }, `duplicate mapping target`},
		{"mapped and ignored", func(s *DriftSpec) { s.IgnoredOnPublicOnly["Mapped"] = "fixture" }, `both mapped and ignored on the public side`},
		{"ignored on both sides", func(s *DriftSpec) {
			s.IgnoredOnPublicOnly["BothIgnored"] = "fixture"
			s.IgnoredOnMirrorOnly["BothIgnored"] = "fixture"
		}, `ignored on both sides`},
		{"empty justification", func(s *DriftSpec) { s.IgnoredOnPublicOnly["PublicOnly"] = "" }, `empty justification`},
		{"empty mapping source", func(s *DriftSpec) { s.Mappings[""] = "MirrorA" }, `empty mapping source path`},
		{"empty mapping target", func(s *DriftSpec) { s.Mappings["DupB"] = "" }, `empty mapping target path`},
		{"empty table", func(s *DriftSpec) { s.Mappings = nil }, `empty mapping table`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := validFixtureSpec()
			tc.mutate(&spec)
			problems := strings.Join(checkDrift(public, mirror, spec), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("checkDrift must fail with %q, got:\n%s", tc.want, problems)
			}
		})
	}
}
