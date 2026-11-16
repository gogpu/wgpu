//go:build js && wasm

package encoder

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Label                  string
	ColorAttachments       []RenderPassColorAttachment
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	Label string
}
