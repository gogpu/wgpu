//go:build js && wasm

package browser

import "syscall/js"

// CommandEncoder wraps a browser GPUCommandEncoder with pre-bound methods.
//
// Pre-binding JS methods at construction time avoids repeated .Get("methodName")
// calls on every frame. The browser's GPUCommandEncoder records GPU commands
// that are later submitted via Queue.Submit.
//
// Matches Rust wgpu WebCommandEncoder which holds the webgpu_sys::GpuCommandEncoder.
type CommandEncoder struct {
	// ref_ is the GPUCommandEncoder JavaScript object.
	ref_ js.Value

	// Pre-bound methods for command recording.
	fnBeginRenderPass      js.Value
	fnBeginComputePass     js.Value
	fnCopyBufferToBuffer   js.Value
	fnCopyBufferToTexture  js.Value
	fnCopyTextureToBuffer  js.Value
	fnCopyTextureToTexture js.Value
	fnClearBuffer          js.Value
	fnFinish               js.Value
}

// NewCommandEncoder constructs a CommandEncoder from a GPUCommandEncoder js.Value.
// Pre-binds all recording methods to avoid property lookups on hot paths.
func NewCommandEncoder(ref js.Value) *CommandEncoder {
	return &CommandEncoder{
		ref_:                   ref,
		fnBeginRenderPass:      bindMethod(ref, "beginRenderPass"),
		fnBeginComputePass:     bindMethod(ref, "beginComputePass"),
		fnCopyBufferToBuffer:   bindMethod(ref, "copyBufferToBuffer"),
		fnCopyBufferToTexture:  bindMethod(ref, "copyBufferToTexture"),
		fnCopyTextureToBuffer:  bindMethod(ref, "copyTextureToBuffer"),
		fnCopyTextureToTexture: bindMethod(ref, "copyTextureToTexture"),
		fnClearBuffer:          bindMethod(ref, "clearBuffer"),
		fnFinish:               bindMethod(ref, "finish"),
	}
}

// BeginRenderPass begins a render pass with the given JS descriptor.
// Returns a RenderPassEncoder wrapping the browser GPURenderPassEncoder.
func (e *CommandEncoder) BeginRenderPass(desc js.Value) *RenderPassEncoder {
	jsPass := e.fnBeginRenderPass.Invoke(desc)
	return NewRenderPassEncoder(jsPass)
}

// BeginComputePass begins a compute pass with the given JS descriptor.
// Returns a ComputePassEncoder wrapping the browser GPUComputePassEncoder.
func (e *CommandEncoder) BeginComputePass(desc js.Value) *ComputePassEncoder {
	jsPass := e.fnBeginComputePass.Invoke(desc)
	return NewComputePassEncoder(jsPass)
}

// CopyBufferToBuffer records a buffer-to-buffer copy command.
// Matches Rust wgpu: copy_buffer_to_buffer_with_f64_and_f64_and_f64.
func (e *CommandEncoder) CopyBufferToBuffer(src js.Value, srcOffset uint64, dst js.Value, dstOffset uint64, size uint64) {
	e.fnCopyBufferToBuffer.Invoke(src, float64(srcOffset), dst, float64(dstOffset), float64(size))
}

// CopyBufferToTexture records a buffer-to-texture copy command.
// source = GPUTexelCopyBufferInfo, destination = GPUTexelCopyTextureInfo, copySize = GPUExtent3DDict.
func (e *CommandEncoder) CopyBufferToTexture(source, destination, copySize js.Value) {
	e.fnCopyBufferToTexture.Invoke(source, destination, copySize)
}

// CopyTextureToBuffer records a texture-to-buffer copy command.
// source = GPUTexelCopyTextureInfo, destination = GPUTexelCopyBufferInfo, copySize = GPUExtent3DDict.
func (e *CommandEncoder) CopyTextureToBuffer(source, destination, copySize js.Value) {
	e.fnCopyTextureToBuffer.Invoke(source, destination, copySize)
}

// CopyTextureToTexture records a texture-to-texture copy command.
// source = GPUTexelCopyTextureInfo, destination = GPUTexelCopyTextureInfo, copySize = GPUExtent3DDict.
func (e *CommandEncoder) CopyTextureToTexture(source, destination, copySize js.Value) {
	e.fnCopyTextureToTexture.Invoke(source, destination, copySize)
}

// Finish completes command recording and returns a CommandBuffer.
// An optional descriptor (or js.Undefined()) can be passed for the label.
func (e *CommandEncoder) Finish(desc js.Value) *CommandBuffer {
	var jsBuf js.Value
	if desc.IsUndefined() || desc.IsNull() {
		jsBuf = e.fnFinish.Invoke()
	} else {
		jsBuf = e.fnFinish.Invoke(desc)
	}
	return &CommandBuffer{ref_: jsBuf}
}

// Ref returns the underlying GPUCommandEncoder js.Value.
func (e *CommandEncoder) Ref() js.Value {
	return e.ref_
}

// CommandBuffer wraps a browser GPUCommandBuffer.
// A command buffer is an opaque handle to recorded GPU commands, ready for
// submission via Queue.Submit.
type CommandBuffer struct {
	// ref_ is the GPUCommandBuffer JavaScript object.
	ref_ js.Value
}

// Ref returns the underlying GPUCommandBuffer js.Value.
func (cb *CommandBuffer) Ref() js.Value {
	return cb.ref_
}
