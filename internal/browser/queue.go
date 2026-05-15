//go:build js && wasm

package browser

import "syscall/js"

// Queue wraps a browser GPUQueue with pre-bound submission and write methods.
//
// Pre-binding JS methods at construction time avoids repeated property lookups
// on every submit/write call. Matches Rust wgpu WebQueue.
type Queue struct {
	// ref_ is the GPUQueue JavaScript object.
	ref_ js.Value

	// Pre-bound methods for submission and data transfer.
	fnSubmit       js.Value
	fnWriteBuffer  js.Value
	fnWriteTexture js.Value
}

// NewQueue constructs a Queue from a GPUQueue js.Value.
// Pre-binds submit, writeBuffer, and writeTexture methods.
func NewQueue(ref js.Value) *Queue {
	return &Queue{
		ref_:           ref,
		fnSubmit:       bindMethod(ref, "submit"),
		fnWriteBuffer:  bindMethod(ref, "writeBuffer"),
		fnWriteTexture: bindMethod(ref, "writeTexture"),
	}
}

// Ref returns the underlying GPUQueue js.Value.
func (q *Queue) Ref() js.Value {
	return q.ref_
}

// Submit submits an array of GPUCommandBuffer js.Values for execution.
// Rust wgpu collects command buffers into a js_sys::Array and calls queue.submit(&array).
func (q *Queue) Submit(commandBuffers []js.Value) {
	arr := js.Global().Get("Array").New(len(commandBuffers))
	for i, cb := range commandBuffers {
		arr.SetIndex(i, cb)
	}
	q.fnSubmit.Invoke(arr)
}

// WriteBuffer writes Go byte data to a GPU buffer.
//
// Rust wgpu creates a Uint8Array from the data, then passes its .buffer() (ArrayBuffer)
// to writeBuffer. We use js.CopyBytesToJS for the Go-to-JS data transfer, which is the
// standard Go WASM pattern (equivalent to Rust's Uint8Array::from(data)).
//
// Signature: queue.writeBuffer(buffer, bufferOffset, data, dataOffset, size)
func (q *Queue) WriteBuffer(buffer js.Value, bufferOffset uint64, data []byte) {
	jsArray := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsArray, data)
	q.fnWriteBuffer.Invoke(
		buffer,
		float64(bufferOffset),
		jsArray,
		0,
		len(data),
	)
}

// WriteTexture writes Go byte data to a GPU texture.
//
// destination = GPUTexelCopyTextureInfo, dataLayout = GPUTexelCopyBufferLayout,
// size = GPUExtent3DDict.
//
// Rust wgpu creates a Uint8Array from the data, passes .buffer() (ArrayBuffer) to
// writeTexture. We use js.CopyBytesToJS for the Go→JS transfer.
func (q *Queue) WriteTexture(destination js.Value, data []byte, dataLayout js.Value, size js.Value) {
	jsArray := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(jsArray, data)
	q.fnWriteTexture.Invoke(destination, jsArray, dataLayout, size)
}
