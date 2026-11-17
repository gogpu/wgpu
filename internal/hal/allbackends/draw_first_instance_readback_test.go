// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestDrawFirstInstanceBaseVertex checks the first instance of Draw and
// DrawIndexed, and the index buffer offset and base vertex of DrawIndexed:
// band 0 is drawn as instance 1, band 1 indexed as instance 2 from an index
// buffer bound 8 bytes in, past two indices that must not be drawn. GLES
// ignored the first instance, the base vertex and the index buffer offset.
func TestDrawFirstInstanceBaseVertex(t *testing.T) {
	forEachBackendAdapter(t, gputypes.DownlevelFlagsBaseVertex, func(t *testing.T, _ overrideBackend, device *wgpu.Device) {
		got := drawArgsScene(t, device, func(s *drawArgsResources, pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1, FirstVertex: 0, FirstInstance: 1})
			pass.SetIndexBuffer(s.index, gputypes.IndexFormatUint32, 8)
			pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 6, InstanceCount: 1, BaseVertex: 6, FirstInstance: 2})
		})
		checkDrawArgsBands(t, got, [drawArgsBands]uint32{drawArgsGreen, drawArgsBlue,
			drawArgsClear, drawArgsClear, drawArgsClear, drawArgsClear, drawArgsClear, drawArgsClear})
	})
}
