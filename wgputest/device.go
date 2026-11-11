//go:build !rust && !(js && wasm)

package wgputest

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

// NewSoftwareDevice returns a device and queue on the Pure Go software
// backend (SPIR-V interpreter + rasterizer), headless. It never uses the
// browser or the Rust backend. The cleanup releases the device, the adapter
// and the instance.
//
// The Pure Go backends build on windows 386/amd64/arm64, linux and darwin
// amd64/arm64, and android arm64. Elsewhere, under -tags rust, or under
// wgpu_nosoftware, the constructor returns ErrUnavailable.
func NewSoftwareDevice(opts ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return newDevice("software", openSoftware, opts)
}

// NewNoopDevice returns a device and queue on the noop backend, headless.
// The cleanup releases the device, the adapter and the instance.
// The noop backend builds on every native platform; under -tags rust the
// constructor returns ErrUnavailable.
func NewNoopDevice(opts ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return newDevice("noop", openNoop, opts)
}

func newDevice(name string, open opener, opts []Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	opened, err := open()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("wgputest: %s backend: %w", name, err)
	}
	bridgeOpts := wgputestbridge.Options{
		Device:   opened.device,
		Queue:    opened.queue,
		Features: opened.features,
		Limits:   opened.limits,
		Label:    "wgputest-" + name,
		Faults: wgputestbridge.Faults{
			CreateCommandEncoder: cfg.faults.CreateCommandEncoder,
			WriteBuffer:          cfg.faults.WriteBuffer,
		},
	}
	if cfg.recorder != nil {
		bridgeOpts.Observer = cfg.recorder.observer()
	}

	deviceValue, queueValue, wgpuCleanup, err := wgputestbridge.New(&bridgeOpts)
	if err != nil {
		// No wgpu device exists to own the HAL device, so destroy it here.
		opened.device.Destroy()
		opened.cleanup()
		if errors.Is(err, wgputestbridge.ErrNoFactory) {
			return nil, nil, nil, fmt.Errorf("wgputest: %s backend: %w", name, ErrUnavailable)
		}
		return nil, nil, nil, fmt.Errorf("wgputest: %s device: %w", name, err)
	}
	device, ok := deviceValue.(*wgpu.Device)
	if !ok {
		wgpuCleanup()
		opened.cleanup()
		return nil, nil, nil, fmt.Errorf("wgputest: %s device: internal error: factory returned %T", name, deviceValue)
	}
	queue, ok := queueValue.(*wgpu.Queue)
	if !ok {
		wgpuCleanup()
		opened.cleanup()
		return nil, nil, nil, fmt.Errorf("wgputest: %s queue: internal error: factory returned %T", name, queueValue)
	}
	cleanup := func() {
		wgpuCleanup()
		opened.cleanup()
	}
	return device, queue, cleanup, nil
}

func (r *Recorder) observer() wgputestbridge.Observer {
	return wgputestbridge.Observer{
		EncoderCreated: func(seq uint64, encoder int, label string) {
			r.record(func(t *Trace) {
				t.Encoders = append(t.Encoders, EncoderCreate{Seq: seq, Index: encoder, Label: label})
			})
		},
		BufferCreated: func(seq uint64, label string, size uint64, usage gputypes.BufferUsage) {
			r.record(func(t *Trace) {
				t.Buffers = append(t.Buffers, BufferCreate{Seq: seq, Label: label, Size: size, Usage: usage})
			})
		},
		TextureCreated: func(seq uint64, label string) {
			r.record(func(t *Trace) {
				t.Textures = append(t.Textures, TextureCreate{Seq: seq, Label: label})
			})
		},
		TextureViewCreated: func(seq uint64, label string) {
			r.record(func(t *Trace) {
				t.TextureViews = append(t.TextureViews, TextureViewCreate{Seq: seq, Label: label})
			})
		},
		PassBegan: func(seq uint64, pass, encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color) {
			r.record(func(t *Trace) {
				t.Passes = append(t.Passes, PassBegin{
					Seq: seq, Index: pass, Encoder: encoder, Compute: compute, Label: label,
					LoadOps: append([]gputypes.LoadOp(nil), loadOps...),
					Clears:  append([]gputypes.Color(nil), clears...),
				})
			})
		},
		BindGroupBound: func(seq uint64, pass int, index uint32, offsets []uint32) {
			r.record(func(t *Trace) {
				t.BindGroups = append(t.BindGroups, BindGroupBind{
					Seq: seq, Pass: pass, Index: index,
					DynamicOffsets: append([]uint32(nil), offsets...),
				})
			})
		},
		VertexBufferBound: func(seq uint64, pass int, slot uint32, label string, offset, size uint64) {
			r.record(func(t *Trace) {
				t.VertexBuffers = append(t.VertexBuffers, VertexBufferBind{
					Seq: seq, Pass: pass, Slot: slot, Label: label, Offset: offset, Size: size,
				})
			})
		},
		IndexBufferBound: func(seq uint64, pass int, label string, format gputypes.IndexFormat, offset, size uint64) {
			r.record(func(t *Trace) {
				t.IndexBuffers = append(t.IndexBuffers, IndexBufferBind{
					Seq: seq, Pass: pass, Label: label, Format: format, Offset: offset, Size: size,
				})
			})
		},
		Drew: func(seq uint64, pass int, args gputypes.DrawArgs) {
			r.record(func(t *Trace) {
				t.Draws = append(t.Draws, Draw{
					Seq: seq, Pass: pass,
					VertexCount: args.VertexCount, InstanceCount: args.InstanceCount,
					FirstVertex: args.FirstVertex, FirstInstance: args.FirstInstance,
				})
			})
		},
		DrewIndexed: func(seq uint64, pass int, args gputypes.DrawIndexedArgs) {
			r.record(func(t *Trace) {
				t.DrawsIndexed = append(t.DrawsIndexed, DrawIndexed{
					Seq: seq, Pass: pass,
					IndexCount: args.IndexCount, InstanceCount: args.InstanceCount,
					FirstIndex: args.FirstIndex, BaseVertex: args.BaseVertex, FirstInstance: args.FirstInstance,
				})
			})
		},
		Submitted: func(seq uint64, submissionIndex uint64, commandBuffers int) {
			r.record(func(t *Trace) {
				t.Submits = append(t.Submits, Submit{Seq: seq, SubmissionIndex: submissionIndex, CommandBuffers: commandBuffers})
			})
		},
	}
}
