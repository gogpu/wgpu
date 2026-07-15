//go:build !(js && wasm)

package vulkan

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func validSwapchainConfig() *hal.SurfaceConfiguration {
	return &hal.SurfaceConfiguration{
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}
}

func validSwapchainSnapshot() swapchainSurfaceSnapshot {
	return swapchainSurfaceSnapshot{
		capabilities: vk.SurfaceCapabilitiesKHR{
			SupportedTransforms:     vk.SurfaceTransformFlagsKHR(vk.SurfaceTransformIdentityBitKhr),
			CurrentTransform:        vk.SurfaceTransformIdentityBitKhr,
			SupportedCompositeAlpha: vk.CompositeAlphaFlagsKHR(vk.CompositeAlphaOpaqueBitKhr),
			SupportedUsageFlags:     vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
		},
		formats: []vk.SurfaceFormatKHR{{
			Format:     vk.FormatR8g8b8a8Unorm,
			ColorSpace: vk.ColorSpaceSrgbNonlinearKhr,
		}},
		presentModes: []vk.PresentModeKHR{vk.PresentModeFifoKhr},
	}
}

func TestValidateSurfaceSnapshotUsesReportedCapabilities(t *testing.T) {
	format, mode, alpha, usage, err := validateSurfaceSnapshot(validSwapchainSnapshot(), validSwapchainConfig())
	if err != nil {
		t.Fatalf("validateSurfaceSnapshot() error: %v", err)
	}
	if format.Format != vk.FormatR8g8b8a8Unorm || format.ColorSpace != vk.ColorSpaceSrgbNonlinearKhr {
		t.Fatalf("selected format = %+v, want exact reported pair", format)
	}
	if mode != vk.PresentModeFifoKhr {
		t.Fatalf("selected present mode = %v, want FIFO", mode)
	}
	if alpha != vk.CompositeAlphaOpaqueBitKhr {
		t.Fatalf("selected alpha mode = %v, want opaque", alpha)
	}
	if usage != vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit) {
		t.Fatalf("selected image usage = %v, want color attachment", usage)
	}
}

func TestValidateSurfaceSnapshotRejectsUnsupportedRequests(t *testing.T) {
	snapshot := validSwapchainSnapshot()
	config := validSwapchainConfig()
	config.Format = gputypes.TextureFormatBGRA8Unorm
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("unsupported format was accepted")
	}

	config = validSwapchainConfig()
	config.PresentMode = hal.PresentModeImmediate
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("unsupported present mode was accepted")
	}

	config = validSwapchainConfig()
	config.AlphaMode = hal.CompositeAlphaModePremultiplied
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("unsupported alpha mode was accepted")
	}
}

func TestValidateSurfaceSnapshotRejectsIncompleteCapabilities(t *testing.T) {
	config := validSwapchainConfig()
	snapshot := validSwapchainSnapshot()

	snapshot.formats = nil
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("empty format capabilities were accepted")
	}

	snapshot = validSwapchainSnapshot()
	snapshot.presentModes = nil
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("empty present-mode capabilities were accepted")
	}

	snapshot = validSwapchainSnapshot()
	snapshot.capabilities.SupportedCompositeAlpha = 0
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("empty alpha capabilities were accepted")
	}

	snapshot = validSwapchainSnapshot()
	snapshot.capabilities.CurrentTransform = 0
	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, config); err == nil {
		t.Fatal("empty current transform was accepted")
	}

	if _, _, _, _, err := validateSurfaceSnapshot(snapshot, nil); err == nil {
		t.Fatal("nil surface configuration was accepted")
	}
}

func TestFormatSelectionPreservesNonlinearPreferenceAndExactPair(t *testing.T) {
	snapshot := validSwapchainSnapshot()
	snapshot.formats = []vk.SurfaceFormatKHR{
		{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceDisplayP3NonlinearExt},
		{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr},
	}
	selected, err := snapshot.formatFor(gputypes.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatalf("formatFor() error: %v", err)
	}
	if selected.ColorSpace != vk.ColorSpaceSrgbNonlinearKhr {
		t.Fatalf("selected color space = %v, want reported sRGB nonlinear pair", selected.ColorSpace)
	}

	snapshot.formats = snapshot.formats[:1]
	selected, err = snapshot.formatFor(gputypes.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatalf("formatFor(non-sRGB) error: %v", err)
	}
	if selected.ColorSpace != vk.ColorSpaceDisplayP3NonlinearExt {
		t.Fatalf("selected color space = %v, want exact non-sRGB pair", selected.ColorSpace)
	}
}

func TestCompositeAlphaAutoRequiresReportedMode(t *testing.T) {
	mode, err := compositeAlphaFor(vk.CompositeAlphaFlagsKHR(vk.CompositeAlphaPreMultipliedBitKhr), hal.CompositeAlphaModeAuto)
	if err != nil {
		t.Fatalf("compositeAlphaFor() error: %v", err)
	}
	if mode != vk.CompositeAlphaPreMultipliedBitKhr {
		t.Fatalf("auto alpha = %v, want premultiplied", mode)
	}

	if _, err := compositeAlphaFor(0, hal.CompositeAlphaModeAuto); err == nil {
		t.Fatal("auto alpha fabricated an unsupported mode")
	}
}

func TestSwapchainImageUsageChecksEveryRequestedBit(t *testing.T) {
	color := vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit)
	if got, err := swapchainImageUsage(gputypes.TextureUsageNone, color); err != nil {
		t.Fatalf("implicit render usage error: %v", err)
	} else if got != color {
		t.Fatalf("implicit render usage = %v, want %v", got, color)
	}

	if _, err := swapchainImageUsage(gputypes.TextureUsageCopyDst, color); err == nil {
		t.Fatal("unsupported copy-destination usage was accepted")
	}
	if _, err := swapchainImageUsage(gputypes.TextureUsage(1<<20), color); err == nil {
		t.Fatal("unknown texture usage bit was accepted")
	}
}

func TestQuerySwapchainFormatsRetriesIncomplete(t *testing.T) {
	countCalls, fillCalls := 0, 0
	formats, err := querySwapchainFormatsWith(func(count *uint32, formats *vk.SurfaceFormatKHR) vk.Result {
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
		t.Fatalf("querySwapchainFormatsWith() error: %v", err)
	}
	if len(formats) != 1 || countCalls != 2 || fillCalls != 2 {
		t.Fatalf("query calls/results = (%d, %d, %d), want (2, 2, 1)", countCalls, fillCalls, len(formats))
	}
}

func TestQuerySwapchainPresentModesFailsClosed(t *testing.T) {
	_, err := querySwapchainPresentModesWith(func(_ *uint32, _ *vk.PresentModeKHR) vk.Result {
		return vk.ErrorSurfaceLostKhr
	})
	if !errors.Is(err, hal.ErrSurfaceLost) {
		t.Fatalf("present-mode query error = %v, want surface-lost error", err)
	}

	_, err = querySwapchainPresentModesWith(func(count *uint32, modes *vk.PresentModeKHR) vk.Result {
		if modes == nil {
			*count = 1
			return vk.Success
		}
		return vk.Incomplete
	})
	if err == nil {
		t.Fatal("unstable present-mode query was accepted")
	}
}

func TestQuerySwapchainImagesUsesActualReturnedCount(t *testing.T) {
	countCalls, fillCalls := 0, 0
	images, err := querySwapchainImagesWith(func(count *uint32, images *vk.Image) vk.Result {
		if images == nil {
			countCalls++
			*count = 3
			return vk.Success
		}
		fillCalls++
		*images = vk.Image(11)
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("querySwapchainImagesWith() error: %v", err)
	}
	if len(images) != 1 || images[0] != vk.Image(11) || countCalls != 1 || fillCalls != 1 {
		t.Fatalf("image query = len %d, first %v, calls (%d, %d); want len 1, image 11, calls (1, 1)", len(images), images[0], countCalls, fillCalls)
	}
}

func TestQuerySwapchainImagesRetriesIncomplete(t *testing.T) {
	countCalls, fillCalls := 0, 0
	images, err := querySwapchainImagesWith(func(count *uint32, images *vk.Image) vk.Result {
		if images == nil {
			countCalls++
			*count = 1
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			return vk.Incomplete
		}
		*images = vk.Image(17)
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("querySwapchainImagesWith() error: %v", err)
	}
	if len(images) != 1 || images[0] != vk.Image(17) || countCalls != 2 || fillCalls != 2 {
		t.Fatalf("image query = len %d, first %v, calls (%d, %d); want len 1, image 17, calls (2, 2)", len(images), images[0], countCalls, fillCalls)
	}
}

func TestSwapchainLifecycleGuardsFailClosed(t *testing.T) {
	destroyed := &Swapchain{destroyed: true}
	if _, _, err := destroyed.acquireNextImage(); err == nil {
		t.Fatal("acquire on destroyed swapchain was accepted")
	}

	broken := &Swapchain{broken: true, failureErr: errors.New("layout transition failed")}
	if err := broken.present(nil, nil); err == nil {
		t.Fatal("present on broken swapchain was accepted")
	}
	device := &Device{}
	withoutSubmission := &Swapchain{device: device, imageAcquired: true}
	if err := withoutSubmission.present(&Queue{device: device}, nil); err == nil {
		t.Fatal("present without a successful queue submission was accepted")
	}
	if !withoutSubmission.broken {
		t.Fatal("present without a successful queue submission did not fail closed")
	}

	valid := &Swapchain{
		device:             device,
		imageAcquired:      true,
		currentAcquireIdx:  0,
		currentImage:       0,
		acquireSemaphores:  []vk.Semaphore{1},
		acquireFenceValues: []uint64{0},
		presentSemaphores:  []vk.Semaphore{2},
	}
	if err := validateSwapchainSubmission(valid, device); err != nil {
		t.Fatalf("valid submission rejected: %v", err)
	}
	valid.currentAcquireIdx = 1
	if err := validateSwapchainSubmission(valid, device); err == nil {
		t.Fatal("out-of-range acquire semaphore state was accepted")
	}
	valid.currentAcquireIdx = 0
	valid.broken = true
	if err := validateSwapchainSubmission(valid, device); err == nil {
		t.Fatal("broken swapchain submission was accepted")
	}

	withoutDevice := &Swapchain{}
	withoutDevice.Destroy()
	if !withoutDevice.destroyed {
		t.Fatal("Destroy() without a device did not mark the swapchain destroyed")
	}
	withoutDevice.Destroy()
}
