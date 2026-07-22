//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestSelectPresentGraphicsQueueFamilyRejectsSplitQueues(t *testing.T) {
	families := []vk.QueueFamilyProperties{
		{QueueFlags: vk.QueueFlags(vk.QueueGraphicsBit), QueueCount: 1},
		{QueueFlags: vk.QueueFlags(vk.QueueTransferBit), QueueCount: 1},
	}

	if _, err := selectPresentGraphicsQueueFamily(families, []bool{false, true}); err == nil {
		t.Fatal("split graphics/present queues were accepted")
	}
}

func TestSelectPresentGraphicsQueueFamilyUsesSameFamily(t *testing.T) {
	families := []vk.QueueFamilyProperties{
		{QueueFlags: vk.QueueFlags(vk.QueueGraphicsBit), QueueCount: 1},
		{QueueFlags: vk.QueueFlags(vk.QueueGraphicsBit), QueueCount: 1},
	}

	got, err := selectPresentGraphicsQueueFamily(families, []bool{false, true})
	if err != nil {
		t.Fatalf("selectPresentGraphicsQueueFamily() error: %v", err)
	}
	if got != 1 {
		t.Fatalf("selected queue family = %d, want 1", got)
	}
}

func TestQuerySurfaceFormatsFailsClosed(t *testing.T) {
	_, err := querySurfaceFormatsWith(func(_ *uint32, _ *vk.SurfaceFormatKHR) vk.Result {
		return vk.ErrorSurfaceLostKhr
	})
	if err == nil {
		t.Fatal("failed surface format query was accepted")
	}
}

func TestQuerySurfaceFormatsRetriesIncomplete(t *testing.T) {
	countCalls := 0
	fillCalls := 0
	formats, err := querySurfaceFormatsWith(func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result {
		if formats == nil {
			countCalls++
			*count = 1
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			return vk.Incomplete
		}
		*formats = vk.SurfaceFormatKHR{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr}
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("querySurfaceFormatsWith() error: %v", err)
	}
	if len(formats) != 1 || countCalls != 2 || fillCalls != 2 {
		t.Fatalf("query calls/results = (%d, %d, %d), want (2, 2, 1)", countCalls, fillCalls, len(formats))
	}
}

func TestMakeSurfaceSnapshotDoesNotFabricateEmptyCapabilities(t *testing.T) {
	_, err := makeSurfaceSnapshot(vk.SurfaceCapabilitiesKHR{}, nil, []vk.PresentModeKHR{vk.PresentModeFifoKhr})
	if err == nil {
		t.Fatal("empty format query produced fabricated capabilities")
	}

	_, err = makeSurfaceSnapshot(vk.SurfaceCapabilitiesKHR{}, []vk.SurfaceFormatKHR{{Format: vk.FormatR8g8b8a8Unorm}}, nil)
	if err == nil {
		t.Fatal("empty present-mode query produced fabricated capabilities")
	}
}

func TestMakeSurfaceSnapshotPreservesFormatColorSpacePairs(t *testing.T) {
	formats := []vk.SurfaceFormatKHR{
		{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr},
		{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceDisplayP3NonlinearExt},
	}
	snapshot, err := makeSurfaceSnapshot(
		vk.SurfaceCapabilitiesKHR{SupportedCompositeAlpha: vk.CompositeAlphaFlagsKHR(vk.CompositeAlphaOpaqueBitKhr)},
		formats,
		[]vk.PresentModeKHR{vk.PresentModeFifoKhr},
	)
	if err != nil {
		t.Fatalf("makeSurfaceSnapshot() error: %v", err)
	}
	if len(snapshot.formats) != len(formats) {
		t.Fatalf("raw format pair count = %d, want %d", len(snapshot.formats), len(formats))
	}
	for i := range formats {
		if snapshot.formats[i] != formats[i] {
			t.Errorf("raw format pair %d = %+v, want %+v", i, snapshot.formats[i], formats[i])
		}
	}
	if len(snapshot.public.Formats) != 2 {
		t.Fatalf("public format projection count = %d, want 2", len(snapshot.public.Formats))
	}
	if snapshot.public.Formats[0] != gputypes.TextureFormatRGBA8Unorm || snapshot.public.Formats[1] != gputypes.TextureFormatRGBA8Unorm {
		t.Fatalf("public formats = %v, want two RGBA8 entries", snapshot.public.Formats)
	}
}

func TestMakeSurfaceSnapshotRejectsUnknownMappings(t *testing.T) {
	_, err := makeSurfaceSnapshot(
		vk.SurfaceCapabilitiesKHR{SupportedCompositeAlpha: vk.CompositeAlphaFlagsKHR(vk.CompositeAlphaOpaqueBitKhr)},
		[]vk.SurfaceFormatKHR{{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr}},
		[]vk.PresentModeKHR{vk.PresentModeKHR(0x7fffffff)},
	)
	if err == nil {
		t.Fatal("unknown present mode was converted into a fabricated FIFO capability")
	}

	_, err = makeSurfaceSnapshot(
		vk.SurfaceCapabilitiesKHR{},
		[]vk.SurfaceFormatKHR{{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr}},
		[]vk.PresentModeKHR{vk.PresentModeFifoKhr},
	)
	if err == nil {
		t.Fatal("unknown composite alpha flags were converted into fabricated opaque capability")
	}
}
