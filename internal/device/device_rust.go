//go:build rust

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file is the device domain's implementation on the rust build, moved out
// of the root package by the S11 Move (ADR-070). The root package keeps a
// declaration layer over it and the descriptor conversion; this type owns the
// wgpu-native calls. No public root type appears in a signature below: the
// descriptors are rwgpu values built by the root, and the handles are internal.

import (
	"fmt"
	"time"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/loststate"
	"github.com/gogpu/wgpu/internal/pipeconst"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device represents a logical GPU device on the rust build: it wraps a
// go-webgpu/webgpu device and holds the instance seam the error scopes read.
type Device struct {
	r        *rwgpu.Device
	instance InstanceHost
	features gputypes.Features
	limits   gputypes.Limits
	released bool

	// lost is the device-lost and uncaptured-error registration state machine
	// (P1-API-042 / #417). NewRust always sets it; a zero-value Device answers
	// the seam methods without one.
	lost *loststate.State[*core.GPUError]
}

// NewRust builds a device over a go-webgpu/webgpu device. The root adapter is
// its only caller; the instance seam is wired with SetInstance right after.
func NewRust(r *rwgpu.Device, features gputypes.Features, limits gputypes.Limits) *Device {
	return &Device{
		r:        r,
		features: features,
		limits:   limits,
		lost:     loststate.New[*core.GPUError](),
	}
}

// SetInstance wires the device to the instance seam its error scopes read.
func (d *Device) SetInstance(h InstanceHost) {
	if d == nil || h == nil {
		return
	}
	d.instance = h
}

// R returns the wgpu-native device, or nil.
func (d *Device) R() *rwgpu.Device {
	if d == nil {
		return nil
	}
	return d.r
}

// IsReleased reports whether the device has been released.
func (d *Device) IsReleased() bool {
	return d != nil && d.released
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	if d == nil {
		return 0
	}
	return d.features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	if d == nil {
		return gputypes.Limits{}
	}
	return d.limits
}

// lostState returns the device's lost/uncaptured registration state machine,
// or nil when the device was not built by NewRust.
func (d *Device) lostState() *loststate.State[*core.GPUError] {
	if d == nil {
		return nil
	}
	return d.lost
}

// SetLostCallback replaces the device-lost registration. A registration made on
// an already-lost device receives the stored info exactly once, at this call's
// return; replacing it afterwards never replays. nil clears the registration.
// Callbacks are invoked after the state machine's lock is released (C4).
//
// The only loss observable at the pinned webgpu v0.5.5 is the one Release
// produces; the driver-initiated source is wired at the webgpu v0.6.0 bump
// (card 0085), where the comment in SetUncapturedErrorCallback points.
func (d *Device) SetLostCallback(fn func(loststate.Info)) {
	s := d.lostState()
	if s == nil {
		return
	}
	fire, ok := s.SetLostCallback(fn)
	if ok {
		fire.Fn(fire.Info)
	}
	d.DispatchPending()
}

// SetUncapturedErrorCallback replaces the uncaptured-error registration; nil
// clears it and a replaced registration never receives a past event again.
//
// WIRING POINT (webgpu v0.6.0, card 0085): wgpu-native's uncaptured-error
// callback is attached here at the binding bump. At the pinned v0.5.5 neither
// source is reachable — DeviceDescriptor carries only Label/RequiredFeatures/
// RequiredLimits, the callback structs are unwired, the trampolines are
// unexported, and the C enum lost its DeviceLost class in v29 — so no fake
// event is synthesized for this registration.
func (d *Device) SetUncapturedErrorCallback(fn func(*core.GPUError)) {
	if s := d.lostState(); s != nil {
		s.SetUncapturedErrorCallback(fn)
	}
}

// LostInfo reports the current lost state; ok is false on a live device.
func (d *Device) LostInfo() (loststate.Info, bool) {
	s := d.lostState()
	if s == nil {
		return loststate.Info{}, false
	}
	return s.LostInfo()
}

// DispatchPending drains the deliveries the recording paths queued and invokes
// them in record order, outside the state machine's lock (C4). It is exported
// for the root declaration layer's entry points.
func (d *Device) DispatchPending() {
	s := d.lostState()
	if s == nil {
		return
	}
	lostFires, errFires := s.TakePending()
	for _, fire := range lostFires {
		fire.Fn(fire.Info)
	}
	for _, fire := range errFires {
		fire.Fn(fire.Err)
	}
}

// markLost latches a loss and dispatches it, recording and invoking in two
// phases so no callback runs under the state machine's lock.
func (d *Device) markLost(reason loststate.Reason, message string) {
	s := d.lostState()
	if s == nil {
		return
	}
	s.MarkLost(reason, message)
	d.DispatchPending()
}

// CreateBuffer creates a GPU buffer from an already-converted descriptor.
func (d *Device) CreateBuffer(desc *rwgpu.BufferDescriptor) (*resource.Buffer, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}

	rb, err := d.r.CreateBuffer(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create buffer: %w", err)
	}

	return resource.NewBufferRust(rb), nil
}

// CreateTexture creates a GPU texture from an already-converted descriptor.
// format is the public descriptor's format, kept on the resource handle.
func (d *Device) CreateTexture(desc *rwgpu.TextureDescriptor, format gputypes.TextureFormat) (*resource.Texture, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}

	rt, err := d.r.CreateTexture(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture: %w", err)
	}

	return resource.NewTexture(rt, format), nil
}

// CreateTextureView creates a view into texture.
func (d *Device) CreateTextureView(texture *resource.Texture, desc *rwgpu.TextureViewDescriptor) (*resource.TextureView, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	if texture == nil || texture.Rust() == nil {
		return nil, fmt.Errorf("wgpu: texture is nil")
	}

	rv, err := texture.Rust().CreateView(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture view: %w", err)
	}

	return resource.NewTextureView(rv, texture), nil
}

// CreateSampler creates a texture sampler from an already-converted descriptor.
func (d *Device) CreateSampler(desc *rwgpu.SamplerDescriptor) (*resource.Sampler, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	rs, err := d.r.CreateSampler(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create sampler: %w", err)
	}

	return resource.NewSampler(rs), nil
}

// CreateShaderModule creates a shader module from WGSL or SPIR-V source.
func (d *Device) CreateShaderModule(label, wgsl string, spirv []uint32) (*resource.ShaderModule, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	var rm *rwgpu.ShaderModule
	var err error

	switch {
	case wgsl != "":
		rm, err = d.r.CreateShaderModuleWGSL(wgsl)
	case len(spirv) > 0:
		rm, err = d.r.CreateShaderModuleSPIRV(label, spirv)
	default:
		return nil, fmt.Errorf("wgpu: shader module descriptor has no source")
	}

	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create shader module: %w", err)
	}

	return resource.NewShaderModule(rm), nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *rwgpu.BindGroupLayoutDescriptor) (*resource.BindGroupLayout, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	rl, err := d.r.CreateBindGroupLayout(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group layout: %w", err)
	}

	return resource.NewBindGroupLayout(rl), nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *rwgpu.PipelineLayoutDescriptor) (*resource.PipelineLayout, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	rl, err := d.r.CreatePipelineLayout(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create pipeline layout: %w", err)
	}

	return resource.NewPipelineLayout(rl), nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *rwgpu.BindGroupDescriptor) (*resource.BindGroup, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	rg, err := d.r.CreateBindGroup(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group: %w", err)
	}

	return resource.NewBindGroup(rg), nil
}

// CreateRenderPipeline creates a render pipeline. The constants are checked
// here, after the released check, exactly as the pre-move body ordered them.
func (d *Device) CreateRenderPipeline(desc *rwgpu.RenderPipelineDescriptor, vertexConstants, fragmentConstants map[string]float64, hasFragment bool) (*pipeline.RenderPipeline, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	if err := checkRustRenderConstants(vertexConstants, fragmentConstants, hasFragment); err != nil {
		return nil, err
	}

	rp, err := d.r.CreateRenderPipeline(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create render pipeline: %w", err)
	}

	return pipeline.NewRenderPipeline(rp), nil
}

// CreateComputePipeline creates a compute pipeline. The constants are checked
// here, after the released check.
func (d *Device) CreateComputePipeline(desc *rwgpu.ComputePipelineDescriptor, constants map[string]float64) (*pipeline.ComputePipeline, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	if err := checkRustStageConstants("compute", constants); err != nil {
		return nil, err
	}

	rp, err := d.r.CreateComputePipeline(desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create compute pipeline: %w", err)
	}

	return pipeline.NewComputePipeline(rp), nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
func (d *Device) CreateCommandEncoder(desc *rwgpu.CommandEncoderDescriptor) (*encoder.CommandEncoder, error) {
	if d.released {
		return nil, core.ErrReleased
	}

	var rDesc *rwgpu.CommandEncoderDescriptor
	if desc != nil {
		cp := *desc
		rDesc = &cp
	}

	re, err := d.r.CreateCommandEncoder(rDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create command encoder: %w", err)
	}

	return encoder.NewCommandEncoder(re), nil
}

// CreateFence creates a no-op fence for API compatibility: wgpu-native exposes
// no fences.
func (d *Device) CreateFence() (*resource.Fence, error) {
	if d.released {
		return nil, core.ErrReleased
	}
	return resource.NewFence(), nil
}

// ResetFence is a no-op on the rust build and always succeeds.
func (d *Device) ResetFence(f *resource.Fence) error {
	if d.released {
		return core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return core.ErrReleased
	}
	return nil
}

// GetFenceStatus always reports signaled on the rust build.
func (d *Device) GetFenceStatus(f *resource.Fence) (bool, error) {
	if d.released {
		return false, core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return false, core.ErrReleased
	}
	return true, nil
}

// WaitForFence polls the device and reports signaled.
func (d *Device) WaitForFence(f *resource.Fence, _ uint64, _ time.Duration) (bool, error) {
	if d.released {
		return false, core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return false, core.ErrReleased
	}
	// Poll device to ensure GPU work has progressed.
	d.r.Poll(true)
	return true, nil
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
// An unmapped filter is an error here and never reaches the binding: wgpu-native
// panics on it and a panic across the FFI boundary aborts the process.
func (d *Device) PushErrorScope(filter core.ErrorFilter) error {
	rFilter, err := errorFilterToRust(filter)
	if err != nil {
		return err
	}
	if d.r != nil {
		d.r.PushErrorScope(rFilter)
	}
	return nil
}

// PopErrorScope pops the most recently pushed error scope. Contract, at most
// one side set: (info, nil) captured; (nil, nil) captured nothing; (nil, nil)
// on a lost device — C2's lost-first rule, the stack untouched; (nil, wrapped)
// when the binding reports a failure, which is also where the not-lost empty
// stack arrives as this build's documented limit.
func (d *Device) PopErrorScope() (*core.GPUError, error) {
	if d == nil {
		return nil, nil //nolint:nilnil // (nil, nil) is the error-scope contract's capture-nothing answer, not a missing error
	}
	if _, lost := d.LostInfo(); lost {
		return nil, nil //nolint:nilnil // the spec's lost-device pop answers null; ErrErrorScopeEmpty is the not-lost empty-stack case
	}
	var instanceR *rwgpu.Instance
	if d.instance != nil {
		instanceR = d.instance.InstanceR()
	}
	if d.r == nil || instanceR == nil {
		return nil, nil //nolint:nilnil // no binding to pop from: the same capture-nothing answer
	}
	errType, message, err := d.r.PopErrorScopeAsync(instanceR)
	return classifyRustPop(errType, message, err)
}

// classifyRustPop turns a settled wgpu-native pop into the four-case contract:
// (nil, nil) captured nothing; (info, nil) captured; (nil, wrapped) the binding
// reported a failure. The not-lost empty stack is that last case on this build
// (the binding raises it), so ErrErrorScopeEmpty is never invented here.
func classifyRustPop(errType rwgpu.ErrorType, message string, err error) (*core.GPUError, error) {
	if err != nil {
		return nil, fmt.Errorf("wgpu: PopErrorScope (rust): %w", err)
	}
	if errType == rwgpu.ErrorTypeNoError {
		return nil, nil //nolint:nilnil // the scope captured nothing: the contract's common answer
	}
	return &core.GPUError{
		Type:    convertRustErrorType(errType),
		Message: message,
	}, nil
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	if d.released {
		return core.ErrReleased
	}
	// Poll with wait=true blocks until all work completes.
	d.r.Poll(true)
	return nil
}

// Poll drives the per-device pending-map triage loop.
func (d *Device) Poll(t core.PollType) bool {
	if d == nil || d.r == nil {
		return false
	}
	return d.r.Poll(t == core.PollWait)
}

// FreeCommandBuffer is a no-op on the rust build.
func (d *Device) FreeCommandBuffer(_ *encoder.CommandBuffer) {}

// Release releases the device. C10: the release produces
// loststate.ReasonDestroyed on every build; first-wins keeps a driver loss
// recorded earlier (none is observable at this pin, card 0085) and makes a
// second Release a no-op.
func (d *Device) Release() {
	if d == nil || d.released {
		return
	}
	d.released = true
	if d.r != nil {
		d.r.Release()
	}
	d.markLost(loststate.ReasonDestroyed, "device released")
}

// errRustConstantsUnmapped is returned for a programmable stage with pipeline
// constants under -tags rust: go-webgpu's ComputePipelineDescriptor has no
// constants and its render stages always send constantCount 0, so the
// constants would be dropped silently and the shader would run with its
// override defaults.
var errRustConstantsUnmapped = fmt.Errorf("wgpu: pipeline constants are not supported with -tags rust: go-webgpu does not map constants yet")

// checkRustStageConstants checks a stage's constants as every build does
// (pipeconst.Validate), then refuses any constant, since go-webgpu cannot
// pass them on.
func checkRustStageConstants(stage string, constants map[string]float64) error {
	if err := pipeconst.Validate(constants); err != nil {
		return fmt.Errorf("wgpu: %s stage: %w", stage, err)
	}
	if len(constants) != 0 {
		return fmt.Errorf("%s stage: %w", stage, errRustConstantsUnmapped)
	}
	return nil
}

// checkRustRenderConstants applies checkRustStageConstants to the vertex and,
// when the pipeline has one, the fragment stage.
func checkRustRenderConstants(vertexConstants, fragmentConstants map[string]float64, hasFragment bool) error {
	if err := checkRustStageConstants("vertex", vertexConstants); err != nil {
		return err
	}
	if hasFragment {
		return checkRustStageConstants("fragment", fragmentConstants)
	}
	return nil
}

// errorFilterToRust maps core's ErrorFilter numbering (Validation = 0, as on
// the native build) to the webgpu.h member of the same name, which counts
// from 1.
func errorFilterToRust(filter core.ErrorFilter) (rwgpu.ErrorFilter, error) {
	switch filter {
	case core.ErrorFilterValidation:
		return rwgpu.ErrorFilterValidation, nil
	case core.ErrorFilterOutOfMemory:
		return rwgpu.ErrorFilterOutOfMemory, nil
	case core.ErrorFilterInternal:
		return rwgpu.ErrorFilterInternal, nil
	}
	return 0, fmt.Errorf("wgpu: PushErrorScope: unknown error filter %d", int(filter))
}

// convertRustErrorType maps go-webgpu ErrorType to core's ErrorFilter.
func convertRustErrorType(et rwgpu.ErrorType) core.ErrorFilter {
	switch et {
	case rwgpu.ErrorTypeValidation:
		return core.ErrorFilterValidation
	case rwgpu.ErrorTypeOutOfMemory:
		return core.ErrorFilterOutOfMemory
	case rwgpu.ErrorTypeInternal:
		return core.ErrorFilterInternal
	default:
		return core.ErrorFilterInternal
	}
}
