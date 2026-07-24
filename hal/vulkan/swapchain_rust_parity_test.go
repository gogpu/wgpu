//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestFloatSurfaceFormatPrefersExtendedLinearSRGB(t *testing.T) {
	snapshot := swapchainSurfaceSnapshot{formats: []vk.SurfaceFormatKHR{
		{Format: vk.FormatR16g16b16a16Sfloat, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr},
		{Format: vk.FormatR16g16b16a16Sfloat, ColorSpace: vk.ColorSpaceExtendedSrgbLinearExt},
	}}

	got, err := snapshot.formatFor(gputypes.TextureFormatRGBA16Float)
	if err != nil {
		t.Fatalf("formatFor(RGBA16Float) error: %v", err)
	}
	if got.ColorSpace != vk.ColorSpaceExtendedSrgbLinearExt {
		t.Fatalf("color space = %v, want extended linear sRGB", got.ColorSpace)
	}

	snapshot.formats = snapshot.formats[:1]
	if _, err := snapshot.formatFor(gputypes.TextureFormatRGBA16Float); err == nil {
		t.Fatal("RGBA16Float without extended linear sRGB was accepted")
	}
}

func TestSurfaceCapabilitiesFilterInvalidFloatColorSpacePair(t *testing.T) {
	snapshot, err := makeSurfaceSnapshot(
		vk.SurfaceCapabilitiesKHR{SupportedCompositeAlpha: vk.CompositeAlphaFlagsKHR(vk.CompositeAlphaOpaqueBitKhr)},
		[]vk.SurfaceFormatKHR{
			{Format: vk.FormatR16g16b16a16Sfloat, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr},
			{Format: vk.FormatR8g8b8a8Unorm, ColorSpace: vk.ColorSpaceSrgbNonlinearKhr},
		},
		[]vk.PresentModeKHR{vk.PresentModeFifoKhr},
	)
	if err != nil {
		t.Fatalf("makeSurfaceSnapshot() error: %v", err)
	}
	if len(snapshot.public.Formats) != 1 || snapshot.public.Formats[0] != gputypes.TextureFormatRGBA8Unorm {
		t.Fatalf("public formats = %v, want only RGBA8Unorm", snapshot.public.Formats)
	}
}

func TestSelectSwapchainExtentUsesCompositorExtentVerbatim(t *testing.T) {
	capabilities := vk.SurfaceCapabilitiesKHR{
		CurrentExtent:  vk.Extent2D{Width: 1080, Height: 1920},
		MinImageExtent: vk.Extent2D{Width: 1, Height: 1},
		MaxImageExtent: vk.Extent2D{Width: 4096, Height: 4096},
	}

	got, err := selectSwapchainExtent(capabilities, 640, 480)
	if err != nil {
		t.Fatalf("selectSwapchainExtent() error: %v", err)
	}
	if got != capabilities.CurrentExtent {
		t.Fatalf("extent = %+v, want fixed compositor extent %+v", got, capabilities.CurrentExtent)
	}
}

func TestSelectSwapchainExtentClampsApplicationExtent(t *testing.T) {
	capabilities := vk.SurfaceCapabilitiesKHR{
		CurrentExtent:  vk.Extent2D{Width: undefinedSurfaceExtent, Height: undefinedSurfaceExtent},
		MinImageExtent: vk.Extent2D{Width: 320, Height: 240},
		MaxImageExtent: vk.Extent2D{Width: 1920, Height: 1080},
	}

	got, err := selectSwapchainExtent(capabilities, 2048, 100)
	if err != nil {
		t.Fatalf("selectSwapchainExtent() error: %v", err)
	}
	want := vk.Extent2D{Width: 1920, Height: 240}
	if got != want {
		t.Fatalf("extent = %+v, want %+v", got, want)
	}
}

func TestSelectSwapchainExtentRejectsInvalidDriverState(t *testing.T) {
	tests := []vk.SurfaceCapabilitiesKHR{
		{
			CurrentExtent:  vk.Extent2D{Width: 640, Height: undefinedSurfaceExtent},
			MinImageExtent: vk.Extent2D{Width: 1, Height: 1},
			MaxImageExtent: vk.Extent2D{Width: 1920, Height: 1080},
		},
		{
			CurrentExtent:  vk.Extent2D{Width: 2048, Height: 1080},
			MinImageExtent: vk.Extent2D{Width: 1, Height: 1},
			MaxImageExtent: vk.Extent2D{Width: 1920, Height: 1080},
		},
		{
			CurrentExtent:  vk.Extent2D{Width: undefinedSurfaceExtent, Height: undefinedSurfaceExtent},
			MinImageExtent: vk.Extent2D{Width: 800, Height: 600},
			MaxImageExtent: vk.Extent2D{Width: 640, Height: 480},
		},
	}
	for index, capabilities := range tests {
		if _, err := selectSwapchainExtent(capabilities, 640, 480); err == nil {
			t.Fatalf("case %d: invalid driver extent state was accepted", index)
		}
	}

	zero := vk.SurfaceCapabilitiesKHR{
		CurrentExtent:  vk.Extent2D{},
		MinImageExtent: vk.Extent2D{},
		MaxImageExtent: vk.Extent2D{Width: 1, Height: 1},
	}
	if _, err := selectSwapchainExtent(zero, 1, 1); !errors.Is(err, hal.ErrZeroArea) {
		t.Fatalf("zero fixed extent error = %v, want ErrZeroArea", err)
	}
}
