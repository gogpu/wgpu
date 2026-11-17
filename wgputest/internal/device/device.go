//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
	"github.com/gogpu/wgpu/wgputest/internal/backend"
)

// Observer receives the recorded calls. It has the same shape as
// wgputestbridge.Observer; the public Recorder in package wgputest fills it,
// because that package may not import internal/wgputestbridge (ADR-070 R3).
// New converts it to the bridge's type.
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

// Faults injects errors into the device. Every call of the named method
// returns the configured error; the fields are independent.
type Faults struct {
	CreateCommandEncoder error
	WriteBuffer          error
}

// Config is what a public constructor passes to New.
type Config struct {
	// Name is the backend name the constructor was called for ("software" or
	// "noop"); it labels the device and prefixes the errors.
	Name string
	// Open opens the private backend instance.
	Open backend.Opener
	// Faults and Observer are the public options, field by field.
	Faults   Faults
	Observer Observer
}

// ErrNoFactory means no factory is registered: the native wgpu implementation
// is not in this build. The public constructor maps it to its ErrUnavailable.
var ErrNoFactory = wgputestbridge.ErrNoFactory

// New builds a device and queue around a private backend instance and returns
// the cleanup that releases them together with the instance and adapter.
//
// The errors are formatted for the public package: they carry the
// "wgputest: <name> ..." prefix, so the facade returns them unchanged and only
// maps ErrNoFactory to its own sentinel.
func New(cfg Config) (*wgpu.Device, *wgpu.Queue, func(), error) {
	opened, err := cfg.Open()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("wgputest: %s backend: %w", cfg.Name, err)
	}
	bridgeOpts := wgputestbridge.Options{
		Device:   opened.Device,
		Queue:    opened.Queue,
		Features: opened.Features,
		Limits:   opened.Limits,
		Label:    "wgputest-" + cfg.Name,
		Faults: wgputestbridge.Faults{
			CreateCommandEncoder: cfg.Faults.CreateCommandEncoder,
			WriteBuffer:          cfg.Faults.WriteBuffer,
		},
		Observer: wgputestbridge.Observer(cfg.Observer),
	}

	deviceValue, queueValue, wgpuCleanup, err := wgputestbridge.New(&bridgeOpts)
	if err != nil {
		// No wgpu device exists to own the HAL device, so destroy it here.
		opened.Device.Destroy()
		opened.Cleanup()
		if errors.Is(err, wgputestbridge.ErrNoFactory) {
			return nil, nil, nil, ErrNoFactory
		}
		return nil, nil, nil, fmt.Errorf("wgputest: %s device: %w", cfg.Name, err)
	}
	device, ok := deviceValue.(*wgpu.Device)
	if !ok {
		wgpuCleanup()
		opened.Cleanup()
		return nil, nil, nil, fmt.Errorf("wgputest: %s device: internal error: factory returned %T", cfg.Name, deviceValue)
	}
	queue, ok := queueValue.(*wgpu.Queue)
	if !ok {
		wgpuCleanup()
		opened.Cleanup()
		return nil, nil, nil, fmt.Errorf("wgputest: %s queue: internal error: factory returned %T", cfg.Name, queueValue)
	}
	cleanup := func() {
		wgpuCleanup()
		opened.Cleanup()
	}
	return device, queue, cleanup, nil
}
