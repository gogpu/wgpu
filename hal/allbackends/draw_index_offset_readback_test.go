// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDrawIndexedIndirectIndexBufferOffset checks that the offset given to
// SetIndexBuffer applies to indexed indirect draws: the index buffer starts
// with two indices that must never be drawn, and an 8-byte offset skips them,
// so firstIndex 0 of each record means index 2 of the buffer. Bands 4 and 5
// come from two DrawIndexedIndirect calls, band 6 from a DrawIndexed after
// them, which must still see the index buffer at its offset.
//
// glDrawElementsIndirect has no index offset parameter, so GLES read the
// records from the start of the buffer (P1-GLES-018).
func TestDrawIndexedIndirectIndexBufferOffset(t *testing.T) {
	required := gputypes.DownlevelFlagsIndirectExecution | gputypes.DownlevelFlagsBaseVertex
	forEachBackendAdapter(t, required, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		records := newDrawArgsBuffer(t, device, wgpu.BufferUsageIndirect, []uint32{
			// DrawIndexedIndirectArgs: indexCount, instanceCount, firstIndex, baseVertex, firstInstance.
			6, 1, 0, 24, 0, // byte 0: band 4
			6, 1, 0, 30, 0, // byte 20: band 5
		})
		got := drawArgsScene(t, device, func(s *drawArgsResources, pass *wgpu.RenderPassEncoder) {
			pass.SetIndexBuffer(s.index, gputypes.IndexFormatUint32, 8)
			pass.DrawIndexedIndirect(records, 0)
			pass.DrawIndexedIndirect(records, 20)
			pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 6, InstanceCount: 1, BaseVertex: 36})
		})
		checkDrawArgsBands(t, got, [drawArgsBands]uint32{drawArgsClear, drawArgsClear, drawArgsClear, drawArgsClear,
			drawArgsRed, drawArgsRed, drawArgsRed, drawArgsClear})
	})
}
