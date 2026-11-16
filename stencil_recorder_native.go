//go:build !rust && !(js && wasm)

package wgpu

// The root-side adapter over the encoder domain's Recorder seam. The queue,
// which stays in this package until S9's Move, constructs it to record its
// staging upload (writeStencilLocked, queue_native.go). The stencil upload
// payload stays here too, and the recording itself is the destination's
// (internal/encoder.CommandEncoder.BlitRecorder): the encoder's own stencil
// lowerings and the queue's upload record through the same implementation, so
// the adapter only adapts the implementation handle the queue's device seam
// hands it to the payload's Recorder view.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// stencilRecorder adapts an encoder implementation handle to the Recorder seam.
type stencilRecorder struct{ e *encoder.CommandEncoder }

var _ encoder.Recorder = stencilRecorder{}

// Own hands a resource the blit created to the encoder, which releases it once
// the command buffer is spent.
func (r stencilRecorder) Own(x encoder.Releasable) { r.e.Own(x) }

// BeginLayerPass opens one blit layer's render pass, binds the layer's bind
// group at index 0 and returns the pass's command recorder.
func (r stencilRecorder) BeginLayerPass(desc *core.RenderPassDescriptor, group *resource.BindGroup) (encoder.PassRecorder, error) {
	return r.e.BlitRecorder().BeginLayerPass(desc, group)
}
