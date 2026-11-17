//go:build !rust && !(js && wasm)

package encoder

import "github.com/gogpu/gputypes"

// The render bundle descriptors. Bundles exist in the native context only —
// the browser and rust contexts have no bundle types, exactly as their public
// API carries none — so this file carries the native build tag. The root
// public descriptors have the same shapes; the declaration layer converts at
// the call boundary (ADR-070 S6).

// RenderBundleEncoderDescriptor describes the render pass compatibility of a
// render bundle recorded by a RenderBundleEncoder.
type RenderBundleEncoderDescriptor struct {
	Label              string
	ColorFormats       []gputypes.TextureFormat
	DepthStencilFormat gputypes.TextureFormat
	SampleCount        uint32
	DepthReadOnly      bool
	StencilReadOnly    bool
}

// RenderBundleDescriptor describes a completed render bundle.
type RenderBundleDescriptor struct {
	Label string
}
