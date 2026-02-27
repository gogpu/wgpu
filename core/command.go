package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ComputePassDescriptor describes how to create a compute pass.
type ComputePassDescriptor struct {
	// Label is an optional debug name for the compute pass.
	Label string

	// TimestampWrites are timestamp queries to write at pass boundaries (optional).
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes for a compute pass.
type ComputePassTimestampWrites struct {
	// QuerySet is the query set to write timestamps to.
	QuerySet QuerySetID

	// BeginningOfPassWriteIndex is the query index for pass start.
	// Use nil to skip.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	// Use nil to skip.
	EndOfPassWriteIndex *uint32
}

// =============================================================================
// HAL-Integrated Command Encoder (CORE-005)
// =============================================================================

// CommandEncoderStatus represents the current state of a command encoder.
//
// State machine transitions:
//
//	Recording -> (BeginRenderPass/BeginComputePass) -> Locked
//	Locked    -> (EndRenderPass/EndComputePass)     -> Recording
//	Recording -> Finish()                           -> Finished
//	Finished  -> (submitted to queue)               -> Consumed
//	Any state -> (error)                            -> Error
type CommandEncoderStatus int32

const (
	// CommandEncoderStatusRecording - ready to record commands.
	CommandEncoderStatusRecording CommandEncoderStatus = iota

	// CommandEncoderStatusLocked - a pass is in progress.
	CommandEncoderStatusLocked

	// CommandEncoderStatusFinished - encoding complete, ready for submit.
	CommandEncoderStatusFinished

	// CommandEncoderStatusError - an error occurred.
	CommandEncoderStatusError

	// CommandEncoderStatusConsumed - submitted to queue.
	CommandEncoderStatusConsumed
)

// String returns a human-readable representation of the status.
func (s CommandEncoderStatus) String() string {
	switch s {
	case CommandEncoderStatusRecording:
		return "Recording"
	case CommandEncoderStatusLocked:
		return "Locked"
	case CommandEncoderStatusFinished:
		return "Finished"
	case CommandEncoderStatusError:
		return "Error"
	case CommandEncoderStatusConsumed:
		return "Consumed"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// CommandBufferMutable holds mutable state during encoding.
//
// This tracks resources used within a command buffer for validation
// and synchronization purposes.
type CommandBufferMutable struct {
	// pendingBufferBarriers are buffer barriers to emit.
	// Used in CORE-007 for barrier tracking.
	pendingBufferBarriers []hal.BufferBarrier //nolint:unused // Will be used in CORE-007

	// pendingTextureBarriers are texture barriers to emit.
	// Used in CORE-007 for barrier tracking.
	pendingTextureBarriers []hal.TextureBarrier //nolint:unused // Will be used in CORE-007

	// usedBuffers tracks buffer usage within this command buffer.
	usedBuffers map[*Buffer]BufferUses

	// usedTextures tracks texture usage within this command buffer.
	usedTextures map[*Texture]TextureUses

	// activePass is the current pass encoder (if any).
	// This is either *CoreRenderPassEncoder or *CoreComputePassEncoder.
	activePass any
}

// BufferUses tracks how a buffer is used within a command buffer.
type BufferUses uint32

const (
	// BufferUsesNone indicates no usage.
	BufferUsesNone BufferUses = 0
	// BufferUsesVertex indicates vertex buffer usage.
	BufferUsesVertex BufferUses = 1 << iota
	// BufferUsesIndex indicates index buffer usage.
	BufferUsesIndex
	// BufferUsesUniform indicates uniform buffer usage.
	BufferUsesUniform
	// BufferUsesStorage indicates storage buffer usage.
	BufferUsesStorage
	// BufferUsesIndirect indicates indirect buffer usage.
	BufferUsesIndirect
	// BufferUsesCopySrc indicates copy source usage.
	BufferUsesCopySrc
	// BufferUsesCopyDst indicates copy destination usage.
	BufferUsesCopyDst
)

// TextureUses tracks how a texture is used within a command buffer.
type TextureUses uint32

const (
	// TextureUsesNone indicates no usage.
	TextureUsesNone TextureUses = 0
	// TextureUsesSampled indicates sampled texture usage.
	TextureUsesSampled TextureUses = 1 << iota
	// TextureUsesStorage indicates storage texture usage.
	TextureUsesStorage
	// TextureUsesRenderAttachment indicates render attachment usage.
	TextureUsesRenderAttachment
	// TextureUsesCopySrc indicates copy source usage.
	TextureUsesCopySrc
	// TextureUsesCopyDst indicates copy destination usage.
	TextureUsesCopyDst
)

// CoreCommandEncoder records GPU commands for submission.
//
// This is the HAL-integrated command encoder that bridges core command
// recording to HAL command encoders. The state machine ensures commands
// are recorded in the correct order and validates encoder state transitions.
//
// CoreCommandEncoder is thread-safe for concurrent access.
type CoreCommandEncoder struct {
	// raw is the HAL encoder wrapped for safe destruction.
	raw *Snatchable[hal.CommandEncoder]

	// device is the parent device.
	device *Device

	// status is the current encoder status (atomic for lock-free reads).
	status atomic.Int32

	// mu protects mutable state.
	mu sync.Mutex

	// mutable holds the mutable encoding state.
	mutable *CommandBufferMutable

	// error holds the error that caused the Error state.
	error error

	// label is the debug label for this encoder.
	label string
}

// CreateCommandEncoder creates a new command encoder on this device.
//
// The encoder is created in the Recording state, ready to record commands.
//
// Parameters:
//   - label: Debug label for the encoder.
//
// Returns the encoder and nil on success.
// Returns nil and an error if the device is destroyed or HAL creation fails.
func (d *Device) CreateCommandEncoder(label string) (*CoreCommandEncoder, error) {
	// 1. Check device validity
	if err := d.checkValid(); err != nil {
		return nil, err
	}

	// 2. Acquire snatch guard for HAL access
	guard := d.snatchLock.Read()
	defer guard.Release()

	halDevice := d.raw.Get(guard)
	if halDevice == nil {
		return nil, ErrDeviceDestroyed
	}

	// 3. Create HAL command encoder
	halEncoder, err := (*halDevice).CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: label,
	})
	if err != nil {
		return nil, &CreateCommandEncoderError{
			Kind:     CreateCommandEncoderErrorHAL,
			Label:    label,
			HALError: err,
		}
	}

	// 4. Begin encoding
	if err := halEncoder.BeginEncoding(label); err != nil {
		return nil, &CreateCommandEncoderError{
			Kind:     CreateCommandEncoderErrorHAL,
			Label:    label,
			HALError: fmt.Errorf("failed to begin encoding: %w", err),
		}
	}

	// 5. Create core encoder
	enc := &CoreCommandEncoder{
		raw:    NewSnatchable(halEncoder),
		device: d,
		mutable: &CommandBufferMutable{
			usedBuffers:  make(map[*Buffer]BufferUses),
			usedTextures: make(map[*Texture]TextureUses),
		},
		label: label,
	}
	enc.status.Store(int32(CommandEncoderStatusRecording))

	trackResource(uintptr(unsafe.Pointer(enc)), "CommandEncoder") //nolint:gosec // debug tracking uses pointer as unique ID
	return enc, nil
}

// RawEncoder returns the underlying HAL command encoder for direct HAL access.
// Requires the device's snatch lock to be held. Returns nil if the encoder
// has been snatched or the device is destroyed.
func (e *CoreCommandEncoder) RawEncoder() hal.CommandEncoder {
	guard := e.device.snatchLock.Read()
	defer guard.Release()
	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		return nil
	}
	return *halEncoder
}

// Status returns the current encoder status.
func (e *CoreCommandEncoder) Status() CommandEncoderStatus {
	return CommandEncoderStatus(e.status.Load())
}

// Label returns the encoder's debug label.
func (e *CoreCommandEncoder) Label() string {
	return e.label
}

// Device returns the parent device.
func (e *CoreCommandEncoder) Device() *Device {
	return e.device
}

// Error returns the error that caused the Error state, or nil.
func (e *CoreCommandEncoder) Error() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.error
}

// BeginRenderPass begins a render pass.
//
// The encoder must be in the Recording state.
// After this call, the encoder transitions to the Locked state.
//
// Returns the render pass encoder and nil on success.
// Returns nil and an error if the encoder is not in Recording state.
func (e *CoreCommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*CoreRenderPassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusRecording {
		return nil, e.statusError("begin render pass")
	}

	// Validate descriptor
	if desc == nil {
		err := fmt.Errorf("render pass descriptor is nil")
		e.setError(err)
		return nil, err
	}

	// Convert to HAL descriptor
	halDesc := e.convertRenderPassDescriptor(desc)

	// Get HAL encoder
	guard := e.device.snatchLock.Read()
	defer guard.Release()

	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		err := ErrResourceDestroyed
		e.setError(err)
		return nil, err
	}

	// Begin HAL render pass
	halPass := (*halEncoder).BeginRenderPass(halDesc)

	// Transition to locked state
	e.status.Store(int32(CommandEncoderStatusLocked))

	pass := &CoreRenderPassEncoder{
		raw:     halPass,
		encoder: e,
		device:  e.device,
	}
	e.mutable.activePass = pass

	return pass, nil
}

// EndRenderPass ends the current render pass.
//
// The encoder must be in the Locked state with an active render pass.
// After this call, the encoder transitions back to the Recording state.
//
// This is called internally by CoreRenderPassEncoder.End().
func (e *CoreCommandEncoder) EndRenderPass(pass *CoreRenderPassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusLocked {
		return e.statusError("end render pass")
	}
	if e.mutable.activePass != pass {
		return fmt.Errorf("wrong pass being ended")
	}

	// End HAL render pass (already called by CoreRenderPassEncoder.End())

	// Return to recording state
	e.status.Store(int32(CommandEncoderStatusRecording))
	e.mutable.activePass = nil

	return nil
}

// BeginComputePass begins a compute pass.
//
// The encoder must be in the Recording state.
// After this call, the encoder transitions to the Locked state.
//
// Returns the compute pass encoder and nil on success.
// Returns nil and an error if the encoder is not in Recording state.
func (e *CoreCommandEncoder) BeginComputePass(desc *CoreComputePassDescriptor) (*CoreComputePassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusRecording {
		return nil, e.statusError("begin compute pass")
	}

	// Convert to HAL descriptor
	halDesc := &hal.ComputePassDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label
		// TimestampWrites conversion would go here
	}

	// Get HAL encoder
	guard := e.device.snatchLock.Read()
	defer guard.Release()

	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		err := ErrResourceDestroyed
		e.setError(err)
		return nil, err
	}

	// Begin HAL compute pass
	halPass := (*halEncoder).BeginComputePass(halDesc)

	// Transition to locked state
	e.status.Store(int32(CommandEncoderStatusLocked))

	pass := &CoreComputePassEncoder{
		raw:     halPass,
		encoder: e,
		device:  e.device,
	}
	e.mutable.activePass = pass

	return pass, nil
}

// EndComputePass ends the current compute pass.
//
// The encoder must be in the Locked state with an active compute pass.
// After this call, the encoder transitions back to the Recording state.
//
// This is called internally by CoreComputePassEncoder.End().
func (e *CoreCommandEncoder) EndComputePass(pass *CoreComputePassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusLocked {
		return e.statusError("end compute pass")
	}
	if e.mutable.activePass != pass {
		return fmt.Errorf("wrong pass being ended")
	}

	// End HAL compute pass (already called by CoreComputePassEncoder.End())

	// Return to recording state
	e.status.Store(int32(CommandEncoderStatusRecording))
	e.mutable.activePass = nil

	return nil
}

// Finish completes encoding and returns a command buffer.
//
// The encoder must be in the Recording state (not in a pass).
// After this call, the encoder transitions to the Finished state.
//
// Returns the command buffer and nil on success.
// Returns nil and an error if the encoder is not in Recording state.
func (e *CoreCommandEncoder) Finish() (*CoreCommandBuffer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Status() != CommandEncoderStatusRecording {
		return nil, e.statusError("finish")
	}

	// Get HAL encoder
	guard := e.device.snatchLock.Read()
	defer guard.Release()

	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		return nil, ErrResourceDestroyed
	}

	// End encoding
	halCmdBuffer, err := (*halEncoder).EndEncoding()
	if err != nil {
		e.setError(err)
		return nil, err
	}

	// Transition to finished
	e.status.Store(int32(CommandEncoderStatusFinished))

	untrackResource(uintptr(unsafe.Pointer(e))) //nolint:gosec // debug tracking uses pointer as unique ID

	return &CoreCommandBuffer{
		raw:     halCmdBuffer,
		device:  e.device,
		mutable: e.mutable,
		label:   e.label,
	}, nil
}

// MarkConsumed marks the encoder as consumed after submission.
//
// This is called by the queue after successful submission.
func (e *CoreCommandEncoder) MarkConsumed() {
	e.status.Store(int32(CommandEncoderStatusConsumed))
}

// setError transitions to error state.
func (e *CoreCommandEncoder) setError(err error) {
	e.error = err
	e.status.Store(int32(CommandEncoderStatusError))
}

// statusError returns an error for invalid status.
func (e *CoreCommandEncoder) statusError(operation string) error {
	return &EncoderStateError{
		Operation: operation,
		Status:    e.Status(),
	}
}

// convertRenderPassDescriptor converts a core descriptor to HAL descriptor.
func (e *CoreCommandEncoder) convertRenderPassDescriptor(desc *RenderPassDescriptor) *hal.RenderPassDescriptor {
	halDesc := &hal.RenderPassDescriptor{
		Label: desc.Label,
	}

	// Convert color attachments
	for _, ca := range desc.ColorAttachments {
		halCA := hal.RenderPassColorAttachment{
			LoadOp:     ca.LoadOp,
			StoreOp:    ca.StoreOp,
			ClearValue: ca.ClearValue,
		}
		// Note: TextureView HAL integration pending (requires core.TextureView with HAL).
		// halCA.View = ca.View.Raw(guard)
		halDesc.ColorAttachments = append(halDesc.ColorAttachments, halCA)
	}

	// Convert depth/stencil attachment if present
	if desc.DepthStencilAttachment != nil {
		halDS := &hal.RenderPassDepthStencilAttachment{
			DepthLoadOp:       desc.DepthStencilAttachment.DepthLoadOp,
			DepthStoreOp:      desc.DepthStencilAttachment.DepthStoreOp,
			DepthClearValue:   desc.DepthStencilAttachment.DepthClearValue,
			DepthReadOnly:     desc.DepthStencilAttachment.DepthReadOnly,
			StencilLoadOp:     desc.DepthStencilAttachment.StencilLoadOp,
			StencilStoreOp:    desc.DepthStencilAttachment.StencilStoreOp,
			StencilClearValue: desc.DepthStencilAttachment.StencilClearValue,
			StencilReadOnly:   desc.DepthStencilAttachment.StencilReadOnly,
		}
		halDesc.DepthStencilAttachment = halDS
	}

	return halDesc
}

// =============================================================================
// Core Render Pass Encoder
// =============================================================================

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ColorAttachments are the color render targets.
	ColorAttachments []RenderPassColorAttachment

	// DepthStencilAttachment is the depth/stencil target (optional).
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	// View is the texture view to render to.
	View *TextureView

	// ResolveTarget is the MSAA resolve target (optional).
	ResolveTarget *TextureView

	// LoadOp specifies what to do at pass start.
	LoadOp gputypes.LoadOp

	// StoreOp specifies what to do at pass end.
	StoreOp gputypes.StoreOp

	// ClearValue is the clear color (used if LoadOp is Clear).
	ClearValue gputypes.Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	// View is the texture view to use.
	View *TextureView

	// DepthLoadOp specifies what to do with depth at pass start.
	DepthLoadOp gputypes.LoadOp

	// DepthStoreOp specifies what to do with depth at pass end.
	DepthStoreOp gputypes.StoreOp

	// DepthClearValue is the depth clear value.
	DepthClearValue float32

	// DepthReadOnly makes the depth aspect read-only.
	DepthReadOnly bool

	// StencilLoadOp specifies what to do with stencil at pass start.
	StencilLoadOp gputypes.LoadOp

	// StencilStoreOp specifies what to do with stencil at pass end.
	StencilStoreOp gputypes.StoreOp

	// StencilClearValue is the stencil clear value.
	StencilClearValue uint32

	// StencilReadOnly makes the stencil aspect read-only.
	StencilReadOnly bool
}

// CoreRenderPassEncoder records render commands within a pass.
//
// This is the HAL-integrated render pass encoder that bridges core
// render commands to HAL render pass encoder.
type CoreRenderPassEncoder struct {
	// raw is the HAL render pass encoder.
	raw hal.RenderPassEncoder

	// encoder is the parent command encoder.
	encoder *CoreCommandEncoder

	// device is the parent device.
	device *Device

	// pipeline is the currently bound render pipeline.
	pipeline *RenderPipeline

	// ended indicates whether End() has been called.
	ended bool
}

// RawPass returns the underlying HAL render pass encoder for direct HAL access.
func (p *CoreRenderPassEncoder) RawPass() hal.RenderPassEncoder {
	return p.raw
}

// SetPipeline sets the render pipeline.
func (p *CoreRenderPassEncoder) SetPipeline(pipeline *RenderPipeline) {
	if p.ended {
		return
	}
	p.pipeline = pipeline
	// Note: HAL SetPipeline pending (requires core.RenderPipeline with HAL).
	// if p.raw != nil && pipeline.Raw() != nil {
	//     p.raw.SetPipeline(pipeline.Raw())
	// }
}

// SetVertexBuffer sets a vertex buffer.
func (p *CoreRenderPassEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if p.ended {
		return
	}
	if p.raw != nil && buffer != nil {
		guard := p.device.snatchLock.Read()
		defer guard.Release()
		halBuffer := buffer.Raw(guard)
		if halBuffer != nil {
			p.raw.SetVertexBuffer(slot, halBuffer, offset)
		}
	}
}

// SetIndexBuffer sets the index buffer.
func (p *CoreRenderPassEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
	if p.ended {
		return
	}
	if p.raw != nil && buffer != nil {
		guard := p.device.snatchLock.Read()
		defer guard.Release()
		halBuffer := buffer.Raw(guard)
		if halBuffer != nil {
			p.raw.SetIndexBuffer(halBuffer, format, offset)
		}
	}
}

// SetViewport sets the viewport.
func (p *CoreRenderPassEncoder) SetViewport(x, y, width, height, minDepth, maxDepth float32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.SetViewport(x, y, width, height, minDepth, maxDepth)
	}
}

// SetScissorRect sets the scissor rectangle.
func (p *CoreRenderPassEncoder) SetScissorRect(x, y, width, height uint32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.SetScissorRect(x, y, width, height)
	}
}

// SetBlendConstant sets the blend constant color.
func (p *CoreRenderPassEncoder) SetBlendConstant(color *gputypes.Color) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.SetBlendConstant(color)
	}
}

// SetStencilReference sets the stencil reference value.
func (p *CoreRenderPassEncoder) SetStencilReference(reference uint32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.SetStencilReference(reference)
	}
}

// Draw draws primitives.
func (p *CoreRenderPassEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
	}
}

// DrawIndexed draws indexed primitives.
func (p *CoreRenderPassEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
	}
}

// DrawIndirect draws primitives with GPU-generated parameters.
func (p *CoreRenderPassEncoder) DrawIndirect(buffer *Buffer, offset uint64) {
	if p.ended {
		return
	}
	if p.raw != nil && buffer != nil {
		guard := p.device.snatchLock.Read()
		defer guard.Release()
		halBuffer := buffer.Raw(guard)
		if halBuffer != nil {
			p.raw.DrawIndirect(halBuffer, offset)
		}
	}
}

// DrawIndexedIndirect draws indexed primitives with GPU-generated parameters.
func (p *CoreRenderPassEncoder) DrawIndexedIndirect(buffer *Buffer, offset uint64) {
	if p.ended {
		return
	}
	if p.raw != nil && buffer != nil {
		guard := p.device.snatchLock.Read()
		defer guard.Release()
		halBuffer := buffer.Raw(guard)
		if halBuffer != nil {
			p.raw.DrawIndexedIndirect(halBuffer, offset)
		}
	}
}

// End ends the render pass.
func (p *CoreRenderPassEncoder) End() error {
	if p.ended {
		return nil
	}
	p.ended = true

	if p.raw != nil {
		p.raw.End()
	}

	return p.encoder.EndRenderPass(p)
}

// =============================================================================
// Core Compute Pass Encoder
// =============================================================================

// CoreComputePassDescriptor describes a compute pass for HAL-integrated API.
type CoreComputePassDescriptor struct {
	// Label is an optional debug name.
	Label string
}

// CoreComputePassEncoder records compute commands within a pass.
//
// This is the HAL-integrated compute pass encoder that bridges core
// compute commands to HAL compute pass encoder.
type CoreComputePassEncoder struct {
	// raw is the HAL compute pass encoder.
	raw hal.ComputePassEncoder

	// encoder is the parent command encoder.
	encoder *CoreCommandEncoder

	// device is the parent device.
	device *Device

	// pipeline is the currently bound compute pipeline.
	pipeline *ComputePipeline

	// ended indicates whether End() has been called.
	ended bool
}

// RawPass returns the underlying HAL compute pass encoder for direct HAL access.
func (p *CoreComputePassEncoder) RawPass() hal.ComputePassEncoder {
	return p.raw
}

// SetPipeline sets the compute pipeline.
func (p *CoreComputePassEncoder) SetPipeline(pipeline *ComputePipeline) {
	if p.ended {
		return
	}
	p.pipeline = pipeline
	// Note: HAL SetPipeline pending (requires core.ComputePipeline with HAL).
}

// Dispatch dispatches compute work.
func (p *CoreComputePassEncoder) Dispatch(x, y, z uint32) {
	if p.ended {
		return
	}
	if p.raw != nil {
		p.raw.Dispatch(x, y, z)
	}
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
func (p *CoreComputePassEncoder) DispatchIndirect(buffer *Buffer, offset uint64) {
	if p.ended {
		return
	}
	if p.raw != nil && buffer != nil {
		guard := p.device.snatchLock.Read()
		defer guard.Release()
		halBuffer := buffer.Raw(guard)
		if halBuffer != nil {
			p.raw.DispatchIndirect(halBuffer, offset)
		}
	}
}

// End ends the compute pass.
func (p *CoreComputePassEncoder) End() error {
	if p.ended {
		return nil
	}
	p.ended = true

	if p.raw != nil {
		p.raw.End()
	}

	return p.encoder.EndComputePass(p)
}

// =============================================================================
// Core Command Buffer
// =============================================================================

// CoreCommandBuffer is a finished command recording ready for submission.
//
// This is created by CoreCommandEncoder.Finish() and can be submitted
// to a queue for execution.
type CoreCommandBuffer struct {
	// raw is the HAL command buffer.
	raw hal.CommandBuffer

	// device is the parent device.
	device *Device

	// mutable holds the resource tracking state from encoding.
	mutable *CommandBufferMutable

	// label is the debug label.
	label string
}

// Raw returns the underlying HAL command buffer.
func (cb *CoreCommandBuffer) Raw() hal.CommandBuffer {
	return cb.raw
}

// Device returns the parent device.
func (cb *CoreCommandBuffer) Device() *Device {
	return cb.device
}

// Label returns the debug label.
func (cb *CoreCommandBuffer) Label() string {
	return cb.label
}

// =============================================================================
// ID-Based API (Backward Compatibility)
// =============================================================================

// ComputePassEncoder records compute commands within a compute pass.
// It wraps hal.ComputePassEncoder with validation and ID-based resource lookup.
type ComputePassEncoder struct {
	raw    hal.ComputePassEncoder
	device *Device
	ended  bool
}

// SetPipeline sets the active compute pipeline for subsequent dispatch calls.
// The pipeline must have been created on the same device as this encoder.
//
// Returns an error if the pipeline ID is invalid.
func (e *ComputePassEncoder) SetPipeline(pipeline ComputePipelineID) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	hub := GetGlobal().Hub()
	rawPipeline, err := hub.GetComputePipeline(pipeline)
	if err != nil {
		return fmt.Errorf("invalid compute pipeline: %w", err)
	}

	// Note: HAL integration pending. When core.ComputePipeline has HAL,
	// convert rawPipeline to hal.ComputePipeline and call e.raw.SetPipeline.
	_ = rawPipeline
	// e.raw.SetPipeline(halPipeline)

	return nil
}

// SetBindGroup sets a bind group for the given index.
// The bind group provides resources (buffers, textures, samplers) to shaders.
//
// Parameters:
//   - index: The bind group index (0, 1, 2, or 3).
//   - group: The bind group ID to bind.
//   - offsets: Dynamic offsets for dynamic uniform/storage buffers (can be nil).
//
// Returns an error if the bind group ID is invalid or if the encoder has ended.
func (e *ComputePassEncoder) SetBindGroup(index uint32, group BindGroupID, offsets []uint32) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	// WebGPU spec: max 4 bind groups (0-3)
	if index > 3 {
		return fmt.Errorf("bind group index %d exceeds maximum (3)", index)
	}

	hub := GetGlobal().Hub()
	rawGroup, err := hub.GetBindGroup(group)
	if err != nil {
		return fmt.Errorf("invalid bind group: %w", err)
	}

	// Note: HAL integration pending. When core.BindGroup has HAL,
	// convert rawGroup to hal.BindGroup and call e.raw.SetBindGroup.
	_ = rawGroup
	// e.raw.SetBindGroup(index, halGroup, offsets)

	return nil
}

// Dispatch dispatches compute work.
// This executes the compute shader with the specified number of workgroups.
//
// Parameters:
//   - x, y, z: The number of workgroups to dispatch in each dimension.
//
// Each workgroup runs the compute shader's workgroup_size threads.
// The total threads = x * y * z * workgroup_size.
//
// Note: This method does not return an error. Dispatch errors are deferred
// to command buffer submission time, matching the WebGPU error model.
func (e *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if e.ended {
		// Record error for deferred validation
		return
	}

	if e.raw != nil {
		e.raw.Dispatch(x, y, z)
	}
}

// DispatchIndirect dispatches compute work with GPU-generated parameters.
// The dispatch parameters are read from the specified buffer.
//
// Parameters:
//   - buffer: The buffer containing DispatchIndirectArgs at the given offset.
//   - offset: The byte offset into the buffer (must be 4-byte aligned).
//
// The buffer must contain the following structure at the offset:
//
//	struct DispatchIndirectArgs {
//	    x: u32,     // Number of workgroups in X
//	    y: u32,     // Number of workgroups in Y
//	    z: u32,     // Number of workgroups in Z
//	}
//
// Returns an error if the buffer ID is invalid or the offset is not aligned.
func (e *ComputePassEncoder) DispatchIndirect(buffer BufferID, offset uint64) error {
	if e.ended {
		return fmt.Errorf("compute pass has already ended")
	}

	// Indirect dispatch requires 4-byte alignment
	if offset%4 != 0 {
		return fmt.Errorf("indirect dispatch offset must be 4-byte aligned, got %d", offset)
	}

	hub := GetGlobal().Hub()
	rawBuffer, err := hub.GetBuffer(buffer)
	if err != nil {
		return fmt.Errorf("invalid buffer: %w", err)
	}

	// Note: HAL integration pending. When core.Buffer lookup returns HAL buffer,
	// convert rawBuffer to hal.Buffer and call e.raw.DispatchIndirect.
	_ = rawBuffer
	// e.raw.DispatchIndirect(halBuffer, offset)

	return nil
}

// End finishes the compute pass.
// After this call, the encoder cannot be used again.
// Any subsequent method calls will return errors.
func (e *ComputePassEncoder) End() {
	if e.ended {
		return
	}

	e.ended = true

	if e.raw != nil {
		e.raw.End()
	}
}

// CommandEncoderState tracks the state of a command encoder.
type CommandEncoderState int

const (
	// CommandEncoderStateRecording means the encoder is actively recording commands.
	CommandEncoderStateRecording CommandEncoderState = iota

	// CommandEncoderStateEnded means the encoder has finished and produced a command buffer.
	CommandEncoderStateEnded

	// CommandEncoderStateError means the encoder encountered an error.
	CommandEncoderStateError
)

// CommandEncoderImpl provides command encoder functionality.
// It wraps hal.CommandEncoder with validation and ID-based resource lookup.
type CommandEncoderImpl struct {
	raw    hal.CommandEncoder
	device *Device
	state  CommandEncoderState
	label  string
}

// BeginComputePass begins a new compute pass within this command encoder.
// The returned ComputePassEncoder is used to record compute commands.
//
// Parameters:
//   - desc: Optional descriptor with label and timestamp writes.
//     Pass nil for default settings.
//
// The compute pass must be ended with End() before:
//   - Beginning another pass (compute or render)
//   - Finishing the command encoder
//
// Returns the compute pass encoder and any error encountered.
func (e *CommandEncoderImpl) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	if e.state != CommandEncoderStateRecording {
		return nil, fmt.Errorf("command encoder is not in recording state")
	}

	// Convert core descriptor to HAL descriptor
	halDesc := &hal.ComputePassDescriptor{}
	if desc != nil {
		halDesc.Label = desc.Label

		if desc.TimestampWrites != nil {
			// Note: QuerySet HAL integration pending.
			// Skipping timestamp writes until core.QuerySet has HAL.
			halDesc.TimestampWrites = nil
		}
	}

	// Begin the compute pass on the underlying HAL encoder
	var rawPass hal.ComputePassEncoder
	if e.raw != nil {
		rawPass = e.raw.BeginComputePass(halDesc)
	}

	return &ComputePassEncoder{
		raw:    rawPass,
		device: e.device,
		ended:  false,
	}, nil
}

// DeviceCreateCommandEncoder creates a new command encoder for recording GPU commands.
// This is the entry point for recording command buffers.
//
// Parameters:
//   - id: The device ID to create the encoder on.
//   - label: Optional debug label for the encoder.
//
// Returns the command encoder ID and any error encountered.
func DeviceCreateCommandEncoder(id DeviceID, label string) (CommandEncoderID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(id)
	if err != nil {
		return CommandEncoderID{}, fmt.Errorf("invalid device: %w", err)
	}

	// Create a placeholder command encoder
	// In a full implementation, this would create the HAL command encoder
	encoder := CommandEncoder{}
	encoderID := hub.RegisterCommandEncoder(encoder)

	return encoderID, nil
}

// CommandEncoderFinish finishes recording and returns a command buffer.
// The command encoder cannot be used after this call.
//
// Parameters:
//   - id: The command encoder ID to finish.
//
// Returns the command buffer ID and any error encountered.
func CommandEncoderFinish(id CommandEncoderID) (CommandBufferID, error) {
	hub := GetGlobal().Hub()

	// Verify the encoder exists
	_, err := hub.GetCommandEncoder(id)
	if err != nil {
		return CommandBufferID{}, fmt.Errorf("invalid command encoder: %w", err)
	}

	// Note: This is the ID-based API. HAL integration is in CoreCommandEncoder.Finish().

	// Create a placeholder command buffer (ID-based API does not have HAL).
	cmdBuffer := CommandBuffer{}
	cmdBufferID := hub.RegisterCommandBuffer(cmdBuffer)

	// Unregister the encoder (it's consumed)
	_, _ = hub.UnregisterCommandEncoder(id)

	return cmdBufferID, nil
}
