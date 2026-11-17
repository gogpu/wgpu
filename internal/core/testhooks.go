package core

import "github.com/gogpu/gputypes"

// TestHooks is the recording seam behind package wgputest. The public methods
// report what the caller did: one EncoderCreated per Device.CreateCommandEncoder
// call and one Submitted per Queue.Submit call. Work the core does internally —
// pooled command encoders, pending-write flushes — is never reported, so the
// trace does not depend on core's pooling.
//
// Implemented by internal/wgputestbridge.Hooks; nil means no recording.
//
// The interface lives here rather than in the root package (ADR-070 B9) so the
// extracted domains (internal/encoder, internal/pipeline, …) can name it
// without importing their host.
type TestHooks interface {
	// EncoderCreated records a public CreateCommandEncoder and returns the
	// user encoder's dense index.
	EncoderCreated(label string) int
	BufferCreated(label string, size uint64, usage gputypes.BufferUsage)
	TextureCreated(label string)
	TextureViewCreated(label string)
	// PassBegan records a public BeginRenderPass or BeginComputePass and
	// returns the pass's dense index. encoder is the user encoder's index.
	PassBegan(encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color) int
	BindGroupBound(pass int, index uint32, offsets []uint32)
	VertexBufferBound(pass int, slot uint32, label string, offset, size uint64)
	IndexBufferBound(pass int, label string, format gputypes.IndexFormat, offset, size uint64)
	Drew(pass int, args gputypes.DrawArgs)
	DrewIndexed(pass int, args gputypes.DrawIndexedArgs)
	// Submitted records a public Queue.Submit with the caller's command-buffer
	// count and the submission index it returned.
	Submitted(submissionIndex uint64, commandBuffers int)
	FaultCreateCommandEncoder() error
	FaultWriteBuffer() error
}
