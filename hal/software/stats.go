package software

import (
	"image"

	"github.com/gogpu/gputypes"
)

// RenderPassStats holds observable state from a recorded software render pass.
// Gathered as RenderPassEncoder records the pass's commands, so it is known
// without submitting the command buffer. Available via
// RenderPassEncoder.Stats() after End().
//
// Designed for CI e2e test assertions:
//
//	stats := pass.(*software.RenderPassEncoder).Stats()
//	if stats.DrawCount != 2 { t.Error(...) }
//	if !stats.HasScissor { t.Error(...) }
//
// See ADR: docs/dev/research/ADR-SOFTWARE-RENDER-PASS-INSTRUMENTATION.md
type RenderPassStats struct {
	DrawCount   uint32
	HasScissor  bool
	ScissorRect image.Rectangle
	Width       uint32
	Height      uint32
	ColorLoadOp gputypes.LoadOp
}
