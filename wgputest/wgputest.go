// Package wgputest provides real devices for tests: the Pure Go software
// backend and a recording noop backend.
//
// The constructors return a device and its queue plus a cleanup function that
// releases the device, the adapter and the instance. They never skip: a
// backend that is not in the build is reported as ErrUnavailable, and any
// other failure as its cause. The noop device records the calls a test needs
// to assert on (buffer, texture and encoder creation, render pass begins with
// their load ops, bind groups with dynamic offsets, vertex and index binds,
// draws and submissions) in cross-kind order; see Recorder.
//
// The recording observes the public wgpu API: one EncoderCreate per
// Device.CreateCommandEncoder and one Submit per Queue.Submit, with the
// caller's label and command-buffer count. Work the core does internally —
// pooled command encoders, pending-write flushes — never appears. Validation
// is unchanged.
//
// Only accepted calls are recorded: a call the public method rejects with a
// validation error changes no state and leaves no record, while every action
// the call actually applied (including a bind whose usage merge reports an
// error on the encoder) does. The exception is a known limitation: on GLES,
// Queue.WriteTexture uploads stencil data through the public pass methods, so
// such a trace would carry those binds; the software and noop devices this
// package provides never take that path.
package wgputest

import (
	"errors"
	"sync"

	"github.com/gogpu/gputypes"
)

// ErrUnavailable is returned (wrapped with the constructor and build detail)
// when the backend a constructor needs is not in this build: software outside
// the platforms where the Pure Go backends build, under the wgpu_nosoftware
// build tag, or under -tags rust; noop under -tags rust.
var ErrUnavailable = errors.New("the backend this constructor needs is not in this build")

// Faults injects errors into the noop device. Every call of the named method
// returns the configured error; the fields are independent. A later
// "after N calls" field can be added without breaking this shape.
type Faults struct {
	// CreateCommandEncoder is returned by every Device.CreateCommandEncoder.
	CreateCommandEncoder error
	// WriteBuffer is returned by every Queue.WriteBuffer.
	WriteBuffer error
}

// Option configures a constructor.
type Option func(*config)

type config struct {
	recorder *Recorder
	faults   Faults
}

// WithRecorder records the device's calls into r.
func WithRecorder(r *Recorder) Option {
	return func(c *config) { c.recorder = r }
}

// WithFaults injects the configured errors.
func WithFaults(f Faults) Option {
	return func(c *config) { c.faults = f }
}

// Recorder collects the calls of a device created with WithRecorder.
// It is safe for concurrent use.
type Recorder struct {
	mu    sync.Mutex
	trace Trace
}

// NewRecorder returns an empty recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// Trace returns a copy of everything recorded so far.
func (r *Recorder) Trace() Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.trace.clone()
}

// Reset drops everything recorded so far.
func (r *Recorder) Reset() {
	r.mu.Lock()
	r.trace = Trace{}
	r.mu.Unlock()
}

func (r *Recorder) record(fn func(*Trace)) {
	r.mu.Lock()
	fn(&r.trace)
	r.mu.Unlock()
}

// Trace is a copy of the recorded calls.
//
// Every record carries Seq, one global counter assigned when the call is
// recorded, so cross-kind order is preserved. Records inside a render or
// compute pass carry Pass, the dense index of the enclosing PassBegin; every
// record that has the field happens inside a pass, so it is never negative. A
// PassBegin carries its encoder's dense index, so passes can be attributed to
// encoders.
//
// Encoders holds one record per public Device.CreateCommandEncoder call and
// Submits one per public Queue.Submit call. The core's pooled encoders and its
// internal pending-write flushes never appear.
type Trace struct {
	Encoders      []EncoderCreate
	Buffers       []BufferCreate
	Textures      []TextureCreate
	TextureViews  []TextureViewCreate
	VertexBuffers []VertexBufferBind
	IndexBuffers  []IndexBufferBind
	BindGroups    []BindGroupBind
	Passes        []PassBegin
	Draws         []Draw
	DrawsIndexed  []DrawIndexed
	Submits       []Submit
}

// EncoderCreate records a public Device.CreateCommandEncoder call.
type EncoderCreate struct {
	Seq   uint64
	Index int
	Label string
}

// BufferCreate records a buffer creation.
type BufferCreate struct {
	Seq   uint64
	Label string
	Size  uint64
	Usage gputypes.BufferUsage
}

// TextureCreate records a texture creation.
type TextureCreate struct {
	Seq   uint64
	Label string
}

// TextureViewCreate records a texture view creation.
type TextureViewCreate struct {
	Seq   uint64
	Label string
}

// VertexBufferBind records a SetVertexBuffer call.
type VertexBufferBind struct {
	Seq    uint64
	Pass   int
	Slot   uint32
	Label  string
	Offset uint64
	Size   uint64
}

// IndexBufferBind records a SetIndexBuffer call.
type IndexBufferBind struct {
	Seq    uint64
	Pass   int
	Label  string
	Format gputypes.IndexFormat
	Offset uint64
	Size   uint64
}

// BindGroupBind records a SetBindGroup call.
type BindGroupBind struct {
	Seq            uint64
	Pass           int
	Index          uint32
	DynamicOffsets []uint32
}

// PassBegin records a BeginRenderPass or BeginComputePass call.
// LoadOps and Clears have one entry per color attachment, in descriptor order;
// they are nil for compute passes.
type PassBegin struct {
	Seq     uint64
	Index   int
	Encoder int
	Compute bool
	Label   string
	LoadOps []gputypes.LoadOp
	Clears  []gputypes.Color
}

// Draw records a Draw call.
type Draw struct {
	Seq           uint64
	Pass          int
	VertexCount   uint32
	InstanceCount uint32
	FirstVertex   uint32
	FirstInstance uint32
}

// DrawIndexed records a DrawIndexed call.
type DrawIndexed struct {
	Seq           uint64
	Pass          int
	IndexCount    uint32
	InstanceCount uint32
	FirstIndex    uint32
	BaseVertex    int32
	FirstInstance uint32
}

// Submit records a public Queue.Submit call with the caller's command-buffer count.
type Submit struct {
	Seq             uint64
	SubmissionIndex uint64
	CommandBuffers  int
}

func (t *Trace) clone() Trace {
	out := *t
	out.Encoders = append([]EncoderCreate(nil), t.Encoders...)
	out.Buffers = append([]BufferCreate(nil), t.Buffers...)
	out.Textures = append([]TextureCreate(nil), t.Textures...)
	out.TextureViews = append([]TextureViewCreate(nil), t.TextureViews...)
	out.VertexBuffers = append([]VertexBufferBind(nil), t.VertexBuffers...)
	out.IndexBuffers = append([]IndexBufferBind(nil), t.IndexBuffers...)
	out.Draws = append([]Draw(nil), t.Draws...)
	out.DrawsIndexed = append([]DrawIndexed(nil), t.DrawsIndexed...)
	out.Submits = append([]Submit(nil), t.Submits...)
	out.BindGroups = make([]BindGroupBind, len(t.BindGroups))
	for i := range t.BindGroups {
		out.BindGroups[i] = t.BindGroups[i]
		out.BindGroups[i].DynamicOffsets = append([]uint32(nil), t.BindGroups[i].DynamicOffsets...)
	}
	out.Passes = make([]PassBegin, len(t.Passes))
	for i := range t.Passes {
		out.Passes[i] = t.Passes[i]
		out.Passes[i].LoadOps = append([]gputypes.LoadOp(nil), t.Passes[i].LoadOps...)
		out.Passes[i].Clears = append([]gputypes.Color(nil), t.Passes[i].Clears...)
	}
	return out
}
