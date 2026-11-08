//go:build !(js && wasm)

package core

import (
	"math"

	"github.com/gogpu/gputypes"
)

// validateViewport validates a render pass viewport, returning a
// *RenderPassValidationError for the first failed rule.
//
// WebGPU spec: GPURenderPassEncoder.setViewport valid usage
// (spec/index.bs:13695-13708). The IDL takes x, y, width and height as
// restricted floats, so a NaN or infinite argument is a TypeError before the
// algorithm runs; a Go float32 can hold such a value, so the checks below
// reject it explicitly.
//
// Reference: wgpu-core command/render.rs:2986-3026 (set_viewport). Rust checks
// the same ranges, but its f32 API has no IDL boundary in front of it and the
// NaN comparisons all fail open, so a NaN origin or size passes Rust. Here the
// spec is normative, as for every other check in this package.
//
// The rules that depend on maxTextureDimension2D are skipped when the limit was
// never populated (0), the convention the vertex stride checks use too.
func validateViewport(vp gputypes.Viewport, limits gputypes.Limits, label string) error {
	switch {
	case !viewportSizeValid(vp, limits):
		return &RenderPassValidationError{
			Kind:     RenderPassErrorInvalidViewportRectSize,
			Label:    label,
			Viewport: vp,
			Limit:    limits.MaxTextureDimension2D,
		}
	case !viewportPositionValid(vp, limits):
		err := &RenderPassValidationError{
			Kind:     RenderPassErrorInvalidViewportRectPosition,
			Label:    label,
			Viewport: vp,
		}
		// Zero marks the "limit unknown" form of the message.
		if maxDim := float32(limits.MaxTextureDimension2D); maxDim > 0 {
			err.ViewportMin = -2 * maxDim
			err.ViewportMax = 2*maxDim - 1
		}
		return err
	case !viewportDepthValid(vp):
		return &RenderPassValidationError{
			Kind:     RenderPassErrorInvalidViewportDepth,
			Label:    label,
			Viewport: vp,
		}
	}
	return nil
}

// viewportSizeValid reports whether width and height are finite and within
// [0, maxTextureDimension2D].
func viewportSizeValid(vp gputypes.Viewport, limits gputypes.Limits) bool {
	if !isFinite32(vp.Width) || !isFinite32(vp.Height) || vp.Width < 0 || vp.Height < 0 {
		return false
	}
	maxDim := float32(limits.MaxTextureDimension2D)
	return maxDim <= 0 || (vp.Width <= maxDim && vp.Height <= maxDim)
}

// viewportPositionValid reports whether x and y are finite and, when the device
// limit is known, whether the origin is at or above -2 *
// maxTextureDimension2D and the extent ends at or below
// 2 * maxTextureDimension2D - 1.
func viewportPositionValid(vp gputypes.Viewport, limits gputypes.Limits) bool {
	if !isFinite32(vp.X) || !isFinite32(vp.Y) {
		return false
	}
	maxDim := float32(limits.MaxTextureDimension2D)
	if maxDim <= 0 {
		return true
	}
	// The spec's terms are named as it words them: the origin may not go
	// below -maxViewportRange, the extent may not end past
	// maxViewportRange - 1. Both are compared as floats, so the extent bound
	// keeps the spec's <= and is not folded into a < against the range.
	lowestOrigin := -2 * maxDim
	maxExtent := 2*maxDim - 1
	return vp.X >= lowestOrigin && vp.Y >= lowestOrigin &&
		vp.X+vp.Width <= maxExtent && vp.Y+vp.Height <= maxExtent
}

// viewportDepthValid reports whether 0 <= minDepth <= maxDepth <= 1. A NaN
// fails every comparison, so it needs no separate finite test.
func viewportDepthValid(vp gputypes.Viewport) bool {
	return vp.MinDepth >= 0 && vp.MinDepth <= 1 &&
		vp.MaxDepth >= 0 && vp.MaxDepth <= 1 &&
		vp.MinDepth <= vp.MaxDepth
}

// isFinite32 reports whether f is neither NaN nor an infinity.
func isFinite32(f float32) bool {
	return !math.IsNaN(float64(f)) && !math.IsInf(float64(f), 0)
}
