//go:build !(js && wasm)

package vulkan

import (
	"slices"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
)

// rayQuerySupport returns a Vulkan 1.2 device with the ray query extensions
// and every feature bit ray query needs.
func rayQuerySupport() extendedFeatureSupport {
	s := extendedFeatureSupport{
		apiVersion: vkMakeVersion(1, 2, 0),
		extensions: map[string]bool{},
	}
	for _, name := range rayQueryDeviceExtensions {
		s.extensions[name] = true
	}
	var next *uintptr
	s.chainRayQueryFeatures(&next)
	s.bufferDeviceAddress.BufferDeviceAddress = vk.Bool32(vk.True)
	s.accelerationStructure.AccelerationStructure = vk.Bool32(vk.True)
	s.rayQueryFeatures.RayQuery = vk.Bool32(vk.True)
	return s
}

// TestRayQueryFeatureDetection checks that FeatureRayQuery is offered only
// with Vulkan 1.2, all three extensions and all three feature bits, and that
// a request for it is refused otherwise.
func TestRayQueryFeatureDetection(t *testing.T) {
	s := rayQuerySupport()
	if !s.shaderCapabilityFeatures().Contains(gputypes.FeatureRayQuery) {
		t.Error("full support: FeatureRayQuery not offered")
	}
	if missing := s.unsupportedRequest(features(gputypes.FeatureRayQuery)); missing != 0 {
		t.Errorf("full support: request refused, missing %v", missing)
	}

	cases := map[string]func(*extendedFeatureSupport){
		"Vulkan 1.1": func(s *extendedFeatureSupport) {
			s.apiVersion = vkMakeVersion(1, 1, 0)
			s.bufferDeviceAddress, s.accelerationStructure, s.rayQueryFeatures = nil, nil, nil
			var next *uintptr
			s.chainRayQueryFeatures(&next)
		},
		"no VK_KHR_ray_query": func(s *extendedFeatureSupport) {
			delete(s.extensions, extRayQuery)
			s.bufferDeviceAddress, s.accelerationStructure, s.rayQueryFeatures = nil, nil, nil
			var next *uintptr
			s.chainRayQueryFeatures(&next)
		},
		"no bufferDeviceAddress":   func(s *extendedFeatureSupport) { s.bufferDeviceAddress.BufferDeviceAddress = 0 },
		"no accelerationStructure": func(s *extendedFeatureSupport) { s.accelerationStructure.AccelerationStructure = 0 },
		"no rayQuery":              func(s *extendedFeatureSupport) { s.rayQueryFeatures.RayQuery = 0 },
	}
	for name, mutate := range cases {
		s := rayQuerySupport()
		mutate(&s)
		if s.shaderCapabilityFeatures().Contains(gputypes.FeatureRayQuery) {
			t.Errorf("%s: FeatureRayQuery offered", name)
		}
		if s.unsupportedRequest(features(gputypes.FeatureRayQuery)) == 0 {
			t.Errorf("%s: request accepted", name)
		}
	}
}

// TestChainRayQueryFeatures checks the query chain links BDA ->
// acceleration structure -> ray query and returns the ray query tail.
func TestChainRayQueryFeatures(t *testing.T) {
	s := rayQuerySupport()
	var head *uintptr
	tail := s.chainRayQueryFeatures(&head)
	if head != (*uintptr)(unsafe.Pointer(s.bufferDeviceAddress)) ||
		s.bufferDeviceAddress.PNext != (*uintptr)(unsafe.Pointer(s.accelerationStructure)) ||
		s.accelerationStructure.PNext != (*uintptr)(unsafe.Pointer(s.rayQueryFeatures)) ||
		tail != &s.rayQueryFeatures.PNext {
		t.Error("ray query query chain is linked wrongly")
	}
	if s.bufferDeviceAddress.SType != vk.StructureTypePhysicalDeviceBufferDeviceAddressFeatures ||
		s.accelerationStructure.SType != vk.StructureTypePhysicalDeviceAccelerationStructureFeaturesKhr ||
		s.rayQueryFeatures.SType != vk.StructureTypePhysicalDeviceRayQueryFeaturesKhr {
		t.Error("ray query query chain has a wrong sType")
	}
}

// TestEnableRayQuery checks that a ray query request enables
// bufferDeviceAddress through the Vulkan 1.2 struct (never the standalone
// struct, VUID-VkDeviceCreateInfo-pNext-02830), chains the acceleration
// structure and ray query structs, and adds the three extensions.
func TestEnableRayQuery(t *testing.T) {
	s := rayQuerySupport()
	var c deviceFeatureChain
	pNext, exts := s.enable(&c, features(gputypes.FeatureRayQuery), false)
	if pNext != (*uintptr)(unsafe.Pointer(&c.vulkan12)) ||
		c.vulkan12.PNext != (*uintptr)(unsafe.Pointer(&c.accelerationStructure)) ||
		c.accelerationStructure.PNext != (*uintptr)(unsafe.Pointer(&c.rayQuery)) ||
		c.rayQuery.PNext != nil {
		t.Fatal("ray query device chain is linked wrongly")
	}
	if c.vulkan12.BufferDeviceAddress == 0 || c.accelerationStructure.AccelerationStructure == 0 || c.rayQuery.RayQuery == 0 {
		t.Errorf("a ray query feature is off: %+v %+v %+v", c.vulkan12, c.accelerationStructure, c.rayQuery)
	}
	for _, name := range rayQueryDeviceExtensions {
		if !slices.Contains(exts, name+"\x00") {
			t.Errorf("extensions %q lack %s", exts, name)
		}
	}

	c = deviceFeatureChain{}
	_, exts = s.enable(&c, 0, true)
	if c.vulkan12.BufferDeviceAddress != 0 || c.accelerationStructure.SType != 0 || c.rayQuery.SType != 0 {
		t.Error("ray query enabled without being requested")
	}
	for _, name := range rayQueryDeviceExtensions {
		if slices.Contains(exts, name+"\x00") {
			t.Errorf("unrequested ray query: extensions %q include %s", exts, name)
		}
	}
}
