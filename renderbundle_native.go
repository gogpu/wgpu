//go:build !rust && !(js && wasm)

package wgpu

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
)

// RenderBundleEncoder records reusable render commands.
type RenderBundleEncoder struct {
	hal      hal.RenderBundleEncoder
	device   *Device
	finished atomic.Bool
}

func (e *RenderBundleEncoder) SetPipeline(pipeline *RenderPipeline) {
	e.hal.SetPipeline(pipeline.hal)
}

func (e *RenderBundleEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	e.hal.SetBindGroup(index, group.hal, offsets)
}

func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	e.hal.SetVertexBuffer(slot, buffer.halBuffer(), offset)
}

func (e *RenderBundleEncoder) SetIndexBuffer(buffer *Buffer, format IndexFormat, offset uint64) {
	e.hal.SetIndexBuffer(buffer.halBuffer(), format, offset)
}

func (e *RenderBundleEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	e.hal.Draw(vertexCount, instanceCount, firstVertex, firstInstance)
}

func (e *RenderBundleEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	e.hal.DrawIndexed(indexCount, instanceCount, firstIndex, baseVertex, firstInstance)
}

// Finish completes recording and returns the reusable render bundle.
func (e *RenderBundleEncoder) Finish() *RenderBundle {
	if e == nil || e.finished.Swap(true) {
		return nil
	}
	return &RenderBundle{hal: e.hal.Finish(), device: e.device}
}

// RenderBundle is a reusable sequence of render commands.
type RenderBundle struct {
	hal      hal.RenderBundle
	device   *Device
	released atomic.Bool
}

// Release destroys the render bundle. It is safe to call Release more than once.
func (b *RenderBundle) Release() {
	if b == nil || b.released.Swap(true) {
		return
	}
	if device := b.device.halDevice(); device != nil {
		device.DestroyRenderBundle(b.hal)
	}
}
