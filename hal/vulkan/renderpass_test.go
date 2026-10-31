//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestRenderPassKeyMRTFields verifies that RenderPassKey can represent
// multiple color attachments as required by the WebGPU MRT spec.
func TestRenderPassKeyMRTFields(t *testing.T) {
	key := RenderPassKey{
		ColorCount:  2,
		SampleCount: vk.SampleCountFlagBits(1),
	}
	key.Colors[0] = ColorAttachmentKeyEntry{
		Format:      vk.FormatB8g8r8a8Unorm,
		LoadOp:      vk.AttachmentLoadOpClear,
		StoreOp:     vk.AttachmentStoreOpStore,
		FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
	}
	key.Colors[1] = ColorAttachmentKeyEntry{
		Format:      vk.FormatR16g16b16a16Sfloat,
		LoadOp:      vk.AttachmentLoadOpClear,
		StoreOp:     vk.AttachmentStoreOpStore,
		FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
	}

	if key.ColorCount != 2 {
		t.Errorf("expected ColorCount 2, got %d", key.ColorCount)
	}
	if key.Colors[0].Format != vk.FormatB8g8r8a8Unorm {
		t.Errorf("expected Colors[0].Format B8G8R8A8Unorm, got %d", key.Colors[0].Format)
	}
	if key.Colors[1].Format != vk.FormatR16g16b16a16Sfloat {
		t.Errorf("expected Colors[1].Format R16G16B16A16Sfloat, got %d", key.Colors[1].Format)
	}
}

// TestBuildRenderPassLayoutMSAAStoreOp is the regression test for P1-VK-011:
// the MSAA color attachment of a slot with a resolve target used to get
// StoreOp DontCare whatever the caller asked, so a later pass loading the
// MSAA view read undefined contents. WebGPU and Rust wgpu-hal (vulkan
// device.rs make_render_pass: the caller's ops on the base attachment,
// LOAD_CLEAR|STORE on the resolve attachment) keep the caller's StoreOp.
// Intel and NVIDIA drivers keep DontCare contents anyway, so a GPU readback
// does not catch it; this checks the attachment descriptions directly.
func TestBuildRenderPassLayoutMSAAStoreOp(t *testing.T) {
	for _, tc := range []struct {
		name string
		load vk.AttachmentLoadOp
		op   vk.AttachmentStoreOp
	}{
		{"clear-store", vk.AttachmentLoadOpClear, vk.AttachmentStoreOpStore},
		{"load-store", vk.AttachmentLoadOpLoad, vk.AttachmentStoreOpStore},
		{"clear-discard", vk.AttachmentLoadOpClear, vk.AttachmentStoreOpDontCare},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := RenderPassKey{ColorCount: 1, SampleCount: vk.SampleCountFlagBits(4)}
			key.Colors[0] = ColorAttachmentKeyEntry{
				Format:      vk.FormatR8g8b8a8Unorm,
				LoadOp:      tc.load,
				StoreOp:     tc.op,
				FinalLayout: vk.ImageLayoutGeneral, // the resolve target's layout
				HasResolve:  true,
			}
			l := buildRenderPassLayout(key)
			if l.attachmentCount != 2 || !l.hasAnyResolve {
				t.Fatalf("attachments = %d, hasAnyResolve = %v; want 2, true", l.attachmentCount, l.hasAnyResolve)
			}
			msaa, resolve := l.attachments[0], l.attachments[1]
			if msaa.StoreOp != tc.op {
				t.Errorf("MSAA StoreOp = %d, want the caller's %d", msaa.StoreOp, tc.op)
			}
			if msaa.FinalLayout != vk.ImageLayoutColorAttachmentOptimal {
				t.Errorf("MSAA FinalLayout = %d, want COLOR_ATTACHMENT_OPTIMAL", msaa.FinalLayout)
			}
			wantInitial := vk.ImageLayoutUndefined
			if tc.load == vk.AttachmentLoadOpLoad {
				wantInitial = vk.ImageLayoutColorAttachmentOptimal
			}
			if msaa.InitialLayout != wantInitial {
				t.Errorf("MSAA InitialLayout = %d, want %d", msaa.InitialLayout, wantInitial)
			}
			if resolve.LoadOp != vk.AttachmentLoadOpClear || resolve.StoreOp != vk.AttachmentStoreOpStore {
				t.Errorf("resolve ops = (%d, %d), want (CLEAR, STORE)", resolve.LoadOp, resolve.StoreOp)
			}
			if resolve.FinalLayout != vk.ImageLayoutGeneral {
				t.Errorf("resolve FinalLayout = %d, want the key's GENERAL", resolve.FinalLayout)
			}
		})
	}
}

// TestRenderPassKeyEquality verifies that keys with the same content are
// considered equal (map lookup correctness).
func TestRenderPassKeyEquality(t *testing.T) {
	makeKey := func() RenderPassKey {
		k := RenderPassKey{
			ColorCount:  2,
			SampleCount: vk.SampleCountFlagBits(4),
		}
		k.Colors[0] = ColorAttachmentKeyEntry{
			Format:      vk.FormatB8g8r8a8Unorm,
			LoadOp:      vk.AttachmentLoadOpClear,
			StoreOp:     vk.AttachmentStoreOpStore,
			FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
			HasResolve:  true,
		}
		k.Colors[1] = ColorAttachmentKeyEntry{
			Format:      vk.FormatR8g8b8a8Unorm,
			LoadOp:      vk.AttachmentLoadOpLoad,
			StoreOp:     vk.AttachmentStoreOpStore,
			FinalLayout: vk.ImageLayoutGeneral,
		}
		return k
	}

	a := makeKey()
	b := makeKey()
	if a != b {
		t.Error("identical RenderPassKey values should be equal")
	}

	// Verify different count makes keys different.
	c := makeKey()
	c.ColorCount = 1
	if a == c {
		t.Error("RenderPassKey with different ColorCount should not be equal")
	}

	// Verify different format makes keys different.
	d := makeKey()
	d.Colors[1].Format = vk.FormatR32Sfloat
	if a == d {
		t.Error("RenderPassKey with different Colors[1].Format should not be equal")
	}
}

// TestFramebufferKeyMRT verifies that FramebufferKey can hold multiple views.
func TestFramebufferKeyMRT(t *testing.T) {
	fbKey := FramebufferKey{
		RenderPass: 42,
		Width:      800,
		Height:     600,
	}
	// Simulate 2 color views + 1 depth view
	fbKey.Views[0] = 100
	fbKey.Views[1] = 200
	fbKey.Views[2] = 300
	fbKey.ViewCount = 3

	if fbKey.ViewCount != 3 {
		t.Errorf("expected ViewCount 3, got %d", fbKey.ViewCount)
	}
	if fbKey.Views[0] != 100 || fbKey.Views[1] != 200 || fbKey.Views[2] != 300 {
		t.Error("Views not set correctly")
	}
}

// TestFramebufferKeyEquality verifies that keys with the same views match.
func TestFramebufferKeyEquality(t *testing.T) {
	makeKey := func() FramebufferKey {
		k := FramebufferKey{
			RenderPass: 42,
			Width:      800,
			Height:     600,
			ViewCount:  2,
		}
		k.Views[0] = 100
		k.Views[1] = 200
		return k
	}

	a := makeKey()
	b := makeKey()
	if a != b {
		t.Error("identical FramebufferKey values should be equal")
	}

	c := makeKey()
	c.Views[1] = 999
	if a == c {
		t.Error("FramebufferKey with different Views should not be equal")
	}
}

// TestMaxColorAttachmentsConstant verifies the constant matches the WebGPU spec.
func TestMaxColorAttachmentsConstant(t *testing.T) {
	if hal.MaxColorAttachments != 8 {
		t.Errorf("expected MaxColorAttachments = 8, got %d", hal.MaxColorAttachments)
	}
	if hal.MaxTotalAttachments != 17 {
		t.Errorf("expected MaxTotalAttachments = 17, got %d", hal.MaxTotalAttachments)
	}
}

// TestRenderPassKeyMaxAttachments verifies that a key with 8 color attachments
// is representable and hashable (important for cache map lookup).
func TestRenderPassKeyMaxAttachments(t *testing.T) {
	key := RenderPassKey{
		ColorCount:  hal.MaxColorAttachments,
		SampleCount: vk.SampleCountFlagBits(1),
	}
	for i := 0; i < hal.MaxColorAttachments; i++ {
		key.Colors[i] = ColorAttachmentKeyEntry{
			Format:      vk.FormatB8g8r8a8Unorm,
			LoadOp:      vk.AttachmentLoadOpClear,
			StoreOp:     vk.AttachmentStoreOpStore,
			FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
		}
	}

	if key.ColorCount != 8 {
		t.Errorf("expected ColorCount 8, got %d", key.ColorCount)
	}

	// Verify it works as a map key (comparable).
	m := make(map[RenderPassKey]bool)
	m[key] = true
	if !m[key] {
		t.Error("RenderPassKey should be usable as map key")
	}
}

// TestRenderPassKeySingleAttachmentBackwardCompat verifies that the refactored
// key works correctly for the single-attachment case (backward compatibility).
func TestRenderPassKeySingleAttachmentBackwardCompat(t *testing.T) {
	key := RenderPassKey{
		ColorCount:  1,
		SampleCount: vk.SampleCountFlagBits(1),
	}
	key.Colors[0] = ColorAttachmentKeyEntry{
		Format:      vk.FormatB8g8r8a8Srgb,
		LoadOp:      vk.AttachmentLoadOpClear,
		StoreOp:     vk.AttachmentStoreOpStore,
		FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
	}

	if key.ColorCount != 1 {
		t.Error("single attachment: wrong ColorCount")
	}
	if key.Colors[0].Format != vk.FormatB8g8r8a8Srgb {
		t.Error("single attachment: wrong Format")
	}

	// Unused slots should be zero-valued.
	for i := 1; i < hal.MaxColorAttachments; i++ {
		if key.Colors[i].Format != vk.FormatUndefined {
			t.Errorf("unused Colors[%d].Format should be FormatUndefined, got %d", i, key.Colors[i].Format)
		}
	}
}

// TestRenderPassKeyWithMSAAResolve verifies the key handles per-attachment resolve flags.
func TestRenderPassKeyWithMSAAResolve(t *testing.T) {
	key := RenderPassKey{
		ColorCount:  2,
		SampleCount: vk.SampleCountFlagBits(4),
	}
	key.Colors[0] = ColorAttachmentKeyEntry{
		Format:      vk.FormatB8g8r8a8Unorm,
		LoadOp:      vk.AttachmentLoadOpClear,
		StoreOp:     vk.AttachmentStoreOpDontCare,
		FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
		HasResolve:  true,
	}
	key.Colors[1] = ColorAttachmentKeyEntry{
		Format:      vk.FormatR16g16Sfloat,
		LoadOp:      vk.AttachmentLoadOpClear,
		StoreOp:     vk.AttachmentStoreOpDontCare,
		FinalLayout: vk.ImageLayoutColorAttachmentOptimal,
		HasResolve:  true,
	}

	if !key.Colors[0].HasResolve {
		t.Error("Colors[0] should have resolve")
	}
	if !key.Colors[1].HasResolve {
		t.Error("Colors[1] should have resolve")
	}
}

// TestFramebufferKeyMaxViews verifies FramebufferKey supports MaxTotalAttachments views.
func TestFramebufferKeyMaxViews(t *testing.T) {
	fbKey := FramebufferKey{
		Width:  1920,
		Height: 1080,
	}
	for i := 0; i < hal.MaxTotalAttachments; i++ {
		fbKey.Views[i] = vk.ImageView(i + 1)
		fbKey.ViewCount = i + 1
	}

	if fbKey.ViewCount != hal.MaxTotalAttachments {
		t.Errorf("expected ViewCount %d, got %d", hal.MaxTotalAttachments, fbKey.ViewCount)
	}
	for i := 0; i < hal.MaxTotalAttachments; i++ {
		if fbKey.Views[i] != vk.ImageView(i+1) {
			t.Errorf("Views[%d] mismatch", i)
		}
	}
}
