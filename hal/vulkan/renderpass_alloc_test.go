//go:build !(js && wasm)

package vulkan

import (
	"testing"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
)

func TestMRTStructSizes(t *testing.T) {
	var entry ColorAttachmentKeyEntry
	var rpKey RenderPassKey
	var fbKey FramebufferKey

	entrySize := unsafe.Sizeof(entry)
	rpSize := unsafe.Sizeof(rpKey)
	fbSize := unsafe.Sizeof(fbKey)

	t.Logf("ColorAttachmentKeyEntry: %d bytes", entrySize)
	t.Logf("RenderPassKey: %d bytes (8 entries × %d + overhead)", rpSize, entrySize)
	t.Logf("FramebufferKey: %d bytes (17 views × 8 + overhead)", fbSize)

	if rpSize > 512 {
		t.Errorf("RenderPassKey too large for stack: %d bytes (max 512)", rpSize)
	}
	if fbSize > 512 {
		t.Errorf("FramebufferKey too large for stack: %d bytes (max 512)", fbSize)
	}
}

func BenchmarkRenderPassKeyCreation(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var key RenderPassKey
		key.ColorCount = 2
		key.Colors[0] = ColorAttachmentKeyEntry{Format: 44, LoadOp: 0, StoreOp: 0}
		key.Colors[1] = ColorAttachmentKeyEntry{Format: 44, LoadOp: 0, StoreOp: 0}
		key.SampleCount = 1
		_ = key
	}
}

func BenchmarkFramebufferKeyCreation(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var key FramebufferKey
		key.ViewCount = 3
		key.Width = 800
		key.Height = 600
		_ = key
	}
}

func BenchmarkCreateRenderPassArrays(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var attachments [hal.MaxTotalAttachments]int
		var colorRefs [hal.MaxColorAttachments]int
		var resolveRefs [hal.MaxColorAttachments]int
		attachments[0] = 1
		colorRefs[0] = 1
		resolveRefs[0] = 1
		_ = attachments
		_ = colorRefs
		_ = resolveRefs
	}
}
