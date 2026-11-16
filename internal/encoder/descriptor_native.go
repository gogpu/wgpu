//go:build !rust && !(js && wasm)

package encoder

import "github.com/gogpu/wgpu/internal/resource"

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	Label                  string
	ColorAttachments       []RenderPassColorAttachment
	DepthStencilAttachment *RenderPassDepthStencilAttachment
	TimestampWrites        *RenderPassTimestampWrites
}

// RenderPassTimestampWrites describes timestamp query writes at render pass boundaries.
type RenderPassTimestampWrites struct {
	QuerySet                  *resource.QuerySet
	BeginningOfPassWriteIndex *uint32
	EndOfPassWriteIndex       *uint32
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	Label string
	// TimestampWrites, when set, writes timestamps at the pass boundaries.
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes at compute pass
// boundaries (WebGPU GPUComputePassTimestampWrites).
type ComputePassTimestampWrites struct {
	QuerySet                  *resource.QuerySet
	BeginningOfPassWriteIndex *uint32
	EndOfPassWriteIndex       *uint32
}
