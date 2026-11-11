//go:build !(js && wasm)

// Package wgputestbridge is the internal seam behind the public wgputest
// package.
//
// Go does not let a package construct a type with unexported fields from
// another package, so wgputest cannot build a *wgpu.Device itself. The root
// package registers a Factory here at init; wgputest calls New and
// type-asserts the opaque result. This keeps the root package's public API
// free of test-only names and needs no //go:linkname.
package wgputestbridge

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/gogpu/gputypes"
)

// Faults injects errors into the device. Every call of the named method
// returns the configured error; the fields are independent.
type Faults struct {
	CreateCommandEncoder error
	WriteBuffer          error
}

// Observer receives the recorded calls. A nil field is not called.
//
// A call is recorded only when the public method accepted it: argument
// validation passed and its effect on the encoder or pass state was applied.
// A call rejected with a validation error is not recorded; a bind group whose
// usage merge fails is still bound (the failure is recorded on the encoder),
// so it is recorded like any accepted bind.
//
// Seq is one global counter per device, assigned when the call is recorded, so
// cross-kind order is preserved. Encoder is the dense index of the creating
// command encoder. Pass is the dense index of the enclosing BeginRenderPass or
// BeginComputePass; every record that carries Pass happens inside a pass, so
// it is never negative.
type Observer struct {
	EncoderCreated     func(seq uint64, encoder int, label string)
	BufferCreated      func(seq uint64, label string, size uint64, usage gputypes.BufferUsage)
	TextureCreated     func(seq uint64, label string)
	TextureViewCreated func(seq uint64, label string)
	PassBegan          func(seq uint64, pass, encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color)
	BindGroupBound     func(seq uint64, pass int, index uint32, offsets []uint32)
	VertexBufferBound  func(seq uint64, pass int, slot uint32, label string, offset, size uint64)
	IndexBufferBound   func(seq uint64, pass int, label string, format gputypes.IndexFormat, offset, size uint64)
	Drew               func(seq uint64, pass int, args gputypes.DrawArgs)
	DrewIndexed        func(seq uint64, pass int, args gputypes.DrawIndexedArgs)
	Submitted          func(seq uint64, submissionIndex uint64, commandBuffers int)
}

// Options is what a wgputest constructor passes to the factory. Device and
// Queue are the hal.Device and hal.Queue of a private backend instance opened
// by wgputest; typing them as any keeps the bridge free of the hal import and
// lets the root factory assert them.
type Options struct {
	Device   any
	Queue    any
	Features gputypes.Features
	Limits   gputypes.Limits
	Label    string
	Faults   Faults
	Observer Observer
}

// Factory builds a *wgpu.Device and *wgpu.Queue around a private HAL backend
// instance. The values are returned as any to break the import cycle: the
// root package registers the factory, wgputest type-asserts the result. The
// cleanup releases the device, adapter and instance.
type Factory func(opts *Options) (device, queue any, cleanup func(), err error)

var (
	// ErrNoFactory means no factory is registered: the native wgpu
	// implementation is not in this build (-tags rust).
	ErrNoFactory = errors.New("wgputestbridge: no factory registered; the native implementation is not in this build")
)

var (
	registryMu sync.RWMutex
	factory    Factory
)

// Register installs the factory. Root calls it from init.
func Register(f Factory) {
	registryMu.Lock()
	factory = f
	registryMu.Unlock()
}

// New calls the registered factory, or returns ErrNoFactory.
func New(opts *Options) (device, queue any, cleanup func(), err error) {
	registryMu.RLock()
	f := factory
	registryMu.RUnlock()
	if f == nil {
		return nil, nil, nil, ErrNoFactory
	}
	return f(opts)
}

// Hooks is the per-device recorder state shared by the wrappers.
type Hooks struct {
	Observer Observer
	Faults   Faults

	seq      atomic.Uint64
	encoders atomic.Int64
	passes   atomic.Int64
}

// NewHooks returns the recorder state for one device.
func NewHooks(observer Observer, faults Faults) *Hooks {
	return &Hooks{Observer: observer, Faults: faults}
}

// EncoderCreated records one public Device.CreateCommandEncoder call and
// returns the caller's dense encoder index. The root package calls it; the
// core's pooled encoders never reach it.
func (h *Hooks) EncoderCreated(label string) int {
	index := h.nextEncoder()
	if f := h.Observer.EncoderCreated; f != nil {
		f(h.nextSeq(), index, label)
	}
	return index
}

// BufferCreated records one public Device.CreateBuffer call.
func (h *Hooks) BufferCreated(label string, size uint64, usage gputypes.BufferUsage) {
	if f := h.Observer.BufferCreated; f != nil {
		f(h.nextSeq(), label, size, usage)
	}
}

// TextureCreated records one public Device.CreateTexture call.
func (h *Hooks) TextureCreated(label string) {
	if f := h.Observer.TextureCreated; f != nil {
		f(h.nextSeq(), label)
	}
}

// TextureViewCreated records one public Device.CreateTextureView call.
func (h *Hooks) TextureViewCreated(label string) {
	if f := h.Observer.TextureViewCreated; f != nil {
		f(h.nextSeq(), label)
	}
}

// PassBegan records one public BeginRenderPass or BeginComputePass call and
// returns the pass's dense index. encoder is the calling user encoder's index.
func (h *Hooks) PassBegan(encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color) int {
	pass := h.nextPass()
	if f := h.Observer.PassBegan; f != nil {
		f(h.nextSeq(), pass, encoder, compute, label, loadOps, clears)
	}
	return pass
}

// BindGroupBound records one public SetBindGroup call.
func (h *Hooks) BindGroupBound(pass int, index uint32, offsets []uint32) {
	if f := h.Observer.BindGroupBound; f != nil {
		f(h.nextSeq(), pass, index, offsets)
	}
}

// VertexBufferBound records one public SetVertexBuffer call.
func (h *Hooks) VertexBufferBound(pass int, slot uint32, label string, offset, size uint64) {
	if f := h.Observer.VertexBufferBound; f != nil {
		f(h.nextSeq(), pass, slot, label, offset, size)
	}
}

// IndexBufferBound records one public SetIndexBuffer call.
func (h *Hooks) IndexBufferBound(pass int, label string, format gputypes.IndexFormat, offset, size uint64) {
	if f := h.Observer.IndexBufferBound; f != nil {
		f(h.nextSeq(), pass, label, format, offset, size)
	}
}

// Drew records one public Draw call.
func (h *Hooks) Drew(pass int, args gputypes.DrawArgs) {
	if f := h.Observer.Drew; f != nil {
		f(h.nextSeq(), pass, args)
	}
}

// DrewIndexed records one public DrawIndexed call.
func (h *Hooks) DrewIndexed(pass int, args gputypes.DrawIndexedArgs) {
	if f := h.Observer.DrewIndexed; f != nil {
		f(h.nextSeq(), pass, args)
	}
}

// Submitted records one public Queue.Submit call with the caller's
// command-buffer count; internal pending-write flushes never reach it.
func (h *Hooks) Submitted(submissionIndex uint64, commandBuffers int) {
	if f := h.Observer.Submitted; f != nil {
		f(h.nextSeq(), submissionIndex, commandBuffers)
	}
}

// FaultCreateCommandEncoder returns the configured fault, if any.
func (h *Hooks) FaultCreateCommandEncoder() error { return h.Faults.CreateCommandEncoder }

// FaultWriteBuffer returns the configured fault, if any.
func (h *Hooks) FaultWriteBuffer() error { return h.Faults.WriteBuffer }

// nextSeq returns the next global sequence number, starting at 1.
func (h *Hooks) nextSeq() uint64 { return h.seq.Add(1) }

// nextEncoder returns the next dense encoder index, starting at 0.
func (h *Hooks) nextEncoder() int { return int(h.encoders.Add(1)) - 1 }

// nextPass returns the next dense pass index, starting at 0.
func (h *Hooks) nextPass() int { return int(h.passes.Add(1)) - 1 }
