//go:build !rust && !(js && wasm)

package wgpu

// The root-side adapter over the encoder domain's Recorder seam. The queue,
// which stays in this package until S9, constructs it to record its staging
// upload (writeStencilLocked, queue_native.go). The stencil upload payload
// stays here too, and the recording itself is the destination's
// (internal/encoder.CommandEncoder.BlitRecorder): the encoder's own stencil
// lowerings and the queue's upload record through the same implementation, so
// the adapter has nothing of its own to do.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// stencilRecorder adapts a CommandEncoder to the destination's Recorder seam.
type stencilRecorder struct{ e *CommandEncoder }

var _ encoder.Recorder = stencilRecorder{}

// Own hands a resource the blit created to the encoder, which releases it once
// the command buffer is spent.
func (r stencilRecorder) Own(x encoder.Releasable) { r.e.impl.BlitRecorder().Own(x) }

// BeginLayerPass opens one blit layer's render pass, binds the layer's bind
// group at index 0 and returns the pass's command recorder.
func (r stencilRecorder) BeginLayerPass(desc *core.RenderPassDescriptor, group *resource.BindGroup) (encoder.PassRecorder, error) {
	return r.e.impl.BlitRecorder().BeginLayerPass(desc, group)
}
