//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/spirv"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// alwaysAvailableCapabilities are the capabilities every Vulkan device
// accepts (Vulkan spec, "SPIR-V Environment", capabilities table; Rust
// wgpu-hal vulkan/adapter.rs, the start of Adapter::open's capability list).
var alwaysAvailableCapabilities = []spirv.Capability{
	spirv.CapabilityShader, spirv.CapabilityMatrix, spirv.CapabilitySampled1D, spirv.CapabilityImage1D,
	spirv.CapabilityImageQuery, spirv.CapabilityDerivativeControl, spirv.CapabilityStorageImageExtendedFormats,
}

var dotProductCapabilities = []spirv.Capability{
	spirv.CapabilityDotProductInputAll, spirv.CapabilityDotProductInput4x8Bit,
	spirv.CapabilityDotProductInput4x8BitPacked, spirv.CapabilityDotProduct,
}

func capabilityList(set map[spirv.Capability]struct{}) []spirv.Capability {
	list := make([]spirv.Capability, 0, len(set))
	for c := range set {
		list = append(list, c)
	}
	slices.Sort(list)
	return list
}

// TestSPIRVCapabilitiesPerDevice pins the capability set naga may use to
// Rust wgpu-hal's (vulkan/adapter.rs, Adapter::open): the always-available
// list, plus each capability only when the device has, or the request
// enables, what grants it. naga polyfills the packed dot product when the
// set lacks the DotProduct capabilities (P2-VK-014): that is the case the set
// changes today, and a nil set (every capability) gave
// VUID-VkShaderModuleCreateInfo-pCode-08740 on devices without
// shaderIntegerDotProduct.
func TestSPIRVCapabilitiesPerDevice(t *testing.T) {
	on := vk.Bool32(vk.True)
	dot := &vk.PhysicalDeviceShaderIntegerDotProductFeatures{ShaderIntegerDotProduct: on}
	demote := &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{ShaderDemoteToHelperInvocation: on}
	subgroupCapabilities := []spirv.Capability{spirv.CapabilityGroupNonUniform, spirv.CapabilityGroupNonUniformVote,
		spirv.CapabilityGroupNonUniformArithmetic, spirv.CapabilityGroupNonUniformBallot,
		spirv.CapabilityGroupNonUniformShuffle, spirv.CapabilityGroupNonUniformShuffleRel,
		spirv.CapabilityGroupNonUniformQuad}
	for _, c := range []struct {
		// name labels a row that no single requested feature names; a row
		// with requested features is named after them (Feature.String).
		name      string
		requested []gputypes.Feature
		downlevel gputypes.DownlevelFlags
		ext       extendedFeatureSupport
		want      []spirv.Capability // beyond alwaysAvailableCapabilities
	}{
		{name: "bare device"},
		{name: "cube arrays and sample shading",
			downlevel: gputypes.DownlevelFlagsCubeArrayTextures | gputypes.DownlevelFlagsMultisampledShading,
			want:      []spirv.Capability{spirv.CapabilitySampledCubeArray, spirv.CapabilitySampleRateShading}},
		{requested: []gputypes.Feature{gputypes.FeaturePrimitiveIndex},
			want: []spirv.Capability{spirv.CapabilityGeometry}},
		{requested: []gputypes.Feature{gputypes.FeatureSubgroupOperations}, want: subgroupCapabilities},
		{requested: []gputypes.Feature{gputypes.FeatureSubgroupVertex}, want: subgroupCapabilities},
		{requested: []gputypes.Feature{gputypes.FeatureShaderInt64, gputypes.FeatureShaderF16, gputypes.FeatureShaderI16},
			want: []spirv.Capability{spirv.CapabilityInt64, spirv.CapabilityFloat16, spirv.CapabilityInt16}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderInt64AtomicMinMax},
			want: []spirv.Capability{spirv.CapabilityInt64Atomics}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderInt64AtomicAllOps},
			want: []spirv.Capability{spirv.CapabilityInt64Atomics}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderFloat32Atomic},
			want: []spirv.Capability{spirv.CapabilityAtomicFloat32AddEXT}},
		{requested: []gputypes.Feature{gputypes.FeatureClipDistances},
			want: []spirv.Capability{spirv.CapabilityClipDistance}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderBarycentrics},
			want: []spirv.Capability{spirv.CapabilityFragmentBarycentricKHR}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderPerVertex},
			want: []spirv.Capability{spirv.CapabilityFragmentBarycentricKHR}},
		{requested: []gputypes.Feature{gputypes.FeatureShaderDrawIndex},
			want: []spirv.Capability{spirv.CapabilityDrawParameters}},
		{requested: []gputypes.Feature{gputypes.FeatureBGRA8UnormStorage},
			want: []spirv.Capability{spirv.CapabilityStorageImageWriteWithoutFormat}},
		{requested: []gputypes.Feature{gputypes.FeatureRayQuery},
			want: []spirv.Capability{spirv.CapabilityRayQueryKHR}},
		{name: "integer dot product", ext: extendedFeatureSupport{integerDot: dot}, want: dotProductCapabilities},
		{name: "integer dot product struct, feature off",
			ext: extendedFeatureSupport{integerDot: &vk.PhysicalDeviceShaderIntegerDotProductFeatures{}}},
		{name: "demote to helper", ext: extendedFeatureSupport{demote: demote},
			want: []spirv.Capability{spirv.CapabilityDemoteToHelperInvocation}},
	} {
		var requested gputypes.Features
		name := c.name
		for i, f := range c.requested {
			requested.Insert(f)
			if i > 0 {
				name += "+"
			}
			name += f.String()
		}
		t.Run(name, func(t *testing.T) {
			got := capabilityList(spirvCapabilities(requested, c.downlevel, &c.ext))
			want := slices.Concat(alwaysAvailableCapabilities, c.want)
			slices.Sort(want)
			want = slices.Compact(want)
			if !slices.Equal(got, want) {
				t.Errorf("capabilities %v, want %v", got, want)
			}
		})
	}
}

// TestShaderFeaturesEnabledWhereSupported checks that Open enables integer
// dot products (P2-VK-014) and demote-to-helper (P2-VK-018) wherever the
// device supports them, whatever was requested, as Rust wgpu-hal enables
// shader_integer_dot_product: below Vulkan 1.3, which promoted both, with
// their extensions. Our instance is Vulkan 1.2, so every device takes the
// extensions today.
func TestShaderFeaturesEnabledWhereSupported(t *testing.T) {
	on := vk.Bool32(vk.True)
	for _, c := range []struct {
		name    string
		version uint32
		wantExt []string
	}{
		{"Vulkan 1.2", vkMakeVersion(1, 2, 0), []string{extShaderIntegerDotProduct + "\x00", extShaderDemoteToHelperInvocation + "\x00"}},
		{"Vulkan 1.3", vkMakeVersion(1, 3, 0), nil},
	} {
		s := extendedFeatureSupport{
			apiVersion: c.version,
			integerDot: &vk.PhysicalDeviceShaderIntegerDotProductFeatures{ShaderIntegerDotProduct: on},
			demote:     &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{ShaderDemoteToHelperInvocation: on},
		}
		var chain deviceFeatureChain
		pNext, extensions := s.enable(&chain, 0, false)
		if pNext == nil {
			t.Fatalf("%s: no feature struct chained", c.name)
		}
		if chain.integerDot.ShaderIntegerDotProduct != on || chain.integerDot.SType != vk.StructureTypePhysicalDeviceShaderIntegerDotProductFeatures {
			t.Errorf("%s: shaderIntegerDotProduct not enabled: %+v", c.name, chain.integerDot)
		}
		if chain.demote.ShaderDemoteToHelperInvocation != on || chain.demote.SType != vk.StructureTypePhysicalDeviceShaderDemoteToHelperInvocationFeatures {
			t.Errorf("%s: shaderDemoteToHelperInvocation not enabled: %+v", c.name, chain.demote)
		}
		slices.Sort(extensions)
		slices.Sort(c.wantExt)
		if !slices.Equal(extensions, c.wantExt) {
			t.Errorf("%s: extensions %q, want %q", c.name, extensions, c.wantExt)
		}
	}

	// A device without them, or that reports the structs with the features
	// off, enables neither.
	for _, s := range []extendedFeatureSupport{
		{apiVersion: vkMakeVersion(1, 2, 0)},
		{apiVersion: vkMakeVersion(1, 2, 0), integerDot: &vk.PhysicalDeviceShaderIntegerDotProductFeatures{},
			demote: &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{}},
	} {
		var chain deviceFeatureChain
		if _, ext := s.enable(&chain, 0, false); len(ext) != 0 {
			t.Errorf("device without the features enables %q", ext)
		}
		if chain.integerDot.ShaderIntegerDotProduct != 0 || chain.demote.ShaderDemoteToHelperInvocation != 0 {
			t.Errorf("features enabled on a device without them: %+v %+v", chain.integerDot, chain.demote)
		}
	}
}

// spirvInfo is what TestDeviceOptionsReachNaga reads from a SPIR-V module:
// the declared capabilities and whether an opcode occurs.
type spirvInfo struct {
	capabilities map[spirv.Capability]bool
	opcodes      map[uint32]bool
}

// SPIR-V opcodes the test looks for (SPIR-V specification, Instructions).
const (
	opCapability                 = 17
	opKill                       = 252
	opDemoteToHelperInvocation   = 5380
	opSDot, opUDot               = 4450, 4451
	spirvHeaderWords             = 5
	spirvOpcodeMask, spirvWCount = 0xFFFF, 16
)

func readSPIRV(t *testing.T, words []uint32) spirvInfo {
	t.Helper()
	info := spirvInfo{capabilities: map[spirv.Capability]bool{}, opcodes: map[uint32]bool{}}
	for i := spirvHeaderWords; i < len(words); {
		op, n := words[i]&spirvOpcodeMask, int(words[i]>>spirvWCount)
		if n == 0 || i+n > len(words) {
			t.Fatalf("malformed SPIR-V at word %d", i)
		}
		info.opcodes[op] = true
		if op == opCapability {
			info.capabilities[spirv.Capability(words[i+1])] = true
		}
		i += n
	}
	return info
}

// TestDeviceOptionsReachNaga compiles WGSL through the same path as
// compileStage (shaderir.SPIRV with the device's options) for a device with
// and without the two features, and checks what naga writes: the native
// packed dot product only with shaderIntegerDotProduct (the polyfill, with no
// DotProduct capability, without it: VUID-VkShaderModuleCreateInfo-pCode-08740
// otherwise), and OpDemoteToHelperInvocation only with
// shaderDemoteToHelperInvocation (OpKill without it). Hermetic, so CI checks
// the wgpu-to-naga contract on every platform; the readback corpus checks the
// values on real devices.
func TestDeviceOptionsReachNaga(t *testing.T) {
	const dotWGSL = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 2>;
@compute @workgroup_size(1)
fn main() {
    out[0] = bitcast<u32>(dot4I8Packed(out[0], out[1]));
    out[1] = dot4U8Packed(out[0], out[1]);
}`
	const discardWGSL = `
@fragment
fn main(@builtin(position) p: vec4<f32>) -> @location(0) vec4<f32> {
    if p.x < 1.0 { discard; }
    return vec4<f32>(dpdx(p.x), 0.0, 0.0, 1.0);
}`
	on := vk.Bool32(vk.True)
	with := extendedFeatureSupport{
		apiVersion: vkMakeVersion(1, 2, 0),
		integerDot: &vk.PhysicalDeviceShaderIntegerDotProductFeatures{ShaderIntegerDotProduct: on},
		demote:     &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{ShaderDemoteToHelperInvocation: on},
	}
	without := extendedFeatureSupport{apiVersion: vkMakeVersion(1, 2, 0)}
	compile := func(t *testing.T, ext *extendedFeatureSupport, wgsl string) spirvInfo {
		t.Helper()
		opts := spirvOptions(vkMakeVersion(1, 2, 0), ext)
		opts.AllowedCapabilities = spirvCapabilities(0, 0, ext)
		module, err := shaderir.Lower(wgsl)
		if err != nil {
			t.Fatalf("Lower: %v", err)
		}
		words, err := shaderir.SPIRV(module.IR, stageSPIRVOptions(opts, true))
		if err != nil {
			t.Fatalf("SPIRV: %v", err)
		}
		return readSPIRV(t, words)
	}

	native := compile(t, &with, dotWGSL)
	if !native.capabilities[spirv.CapabilityDotProduct] || !native.opcodes[opSDot] || !native.opcodes[opUDot] {
		t.Errorf("device with shaderIntegerDotProduct: DotProduct capability %v, OpSDot %v, OpUDot %v; want the native dot product",
			native.capabilities[spirv.CapabilityDotProduct], native.opcodes[opSDot], native.opcodes[opUDot])
	}
	polyfill := compile(t, &without, dotWGSL)
	if polyfill.capabilities[spirv.CapabilityDotProduct] || polyfill.capabilities[spirv.CapabilityDotProductInput4x8BitPacked] ||
		polyfill.opcodes[opSDot] || polyfill.opcodes[opUDot] {
		t.Errorf("device without shaderIntegerDotProduct: the module uses the DotProduct capabilities %v", polyfill.capabilities)
	}

	demoted := compile(t, &with, discardWGSL)
	if !demoted.opcodes[opDemoteToHelperInvocation] || demoted.opcodes[opKill] ||
		!demoted.capabilities[spirv.CapabilityDemoteToHelperInvocation] {
		t.Errorf("device with shaderDemoteToHelperInvocation: OpDemoteToHelperInvocation %v, OpKill %v; want demote only",
			demoted.opcodes[opDemoteToHelperInvocation], demoted.opcodes[opKill])
	}
	killed := compile(t, &without, discardWGSL)
	if !killed.opcodes[opKill] || killed.opcodes[opDemoteToHelperInvocation] ||
		killed.capabilities[spirv.CapabilityDemoteToHelperInvocation] {
		t.Errorf("device without shaderDemoteToHelperInvocation: OpKill %v, OpDemoteToHelperInvocation %v; want OpKill only",
			killed.opcodes[opKill], killed.opcodes[opDemoteToHelperInvocation])
	}
}

// TestSPIRVDemotePerDevice pins naga's discard lowering to the device:
// OpDemoteToHelperInvocation (WGSL's helper-invocation discard) where the
// device enabled shaderDemoteToHelperInvocation, as Dawn does with
// use_demote_to_helper_invocation (native/vulkan/ShaderModuleVk.cpp); OpKill
// elsewhere, which is what Rust naga always emits.
func TestSPIRVDemotePerDevice(t *testing.T) {
	on := vk.Bool32(vk.True)
	for _, c := range []struct {
		name string
		ext  extendedFeatureSupport
		want bool
	}{
		{"not reportable", extendedFeatureSupport{}, false},
		{"feature off", extendedFeatureSupport{demote: &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{}}, false},
		{"feature on", extendedFeatureSupport{demote: &vk.PhysicalDeviceShaderDemoteToHelperInvocationFeatures{ShaderDemoteToHelperInvocation: on}}, true},
	} {
		if got := spirvOptions(vkMakeVersion(1, 2, 0), &c.ext).DemoteToHelperInvocation; got != c.want {
			t.Errorf("%s: DemoteToHelperInvocation %v, want %v", c.name, got, c.want)
		}
	}
}
