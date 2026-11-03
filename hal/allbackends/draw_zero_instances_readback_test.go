// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDrawZeroInstances checks that a draw with an instance count of zero
// draws nothing, direct or from an indirect record (a record a culling pass
// zeroed, for example): bands 2, 4 and 6 come from records or a call with
// zero instances and must stay clear, bands 3 and 5 are the same records
// with one instance. The software backend drew one instance for zero.
func TestDrawZeroInstances(t *testing.T) {
	required := gputypes.DownlevelFlagsIndirectExecution | gputypes.DownlevelFlagsBaseVertex
	forEachBackendAdapter(t, required, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		records := newDrawArgsBuffer(t, device, wgpu.BufferUsageIndirect, []uint32{
			// DrawIndirectArgs: vertexCount, instanceCount, firstVertex, firstInstance.
			6, 0, 12, 0, // byte 0: band 2, no instance
			6, 1, 18, 0, // byte 16: band 3
			// DrawIndexedIndirectArgs: indexCount, instanceCount, firstIndex, baseVertex, firstInstance.
			6, 0, 2, 24, 0, // byte 32: band 4, no instance
			6, 1, 2, 30, 0, // byte 52: band 5
		})
		got := drawArgsScene(t, device, func(s *drawArgsResources, pass *wgpu.RenderPassEncoder) {
			pass.DrawIndirect(records, 0)
			pass.DrawIndirect(records, 16)
			pass.SetIndexBuffer(s.index, gputypes.IndexFormatUint32, 0)
			pass.DrawIndexedIndirect(records, 32)
			pass.DrawIndexedIndirect(records, 52)
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 0, FirstVertex: 36})
		})
		checkDrawArgsBands(t, got, [drawArgsBands]uint32{drawArgsClear, drawArgsClear, drawArgsClear,
			drawArgsRed, drawArgsClear, drawArgsRed, drawArgsClear, drawArgsClear})
	})
}
