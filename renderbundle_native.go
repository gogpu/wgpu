//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// RenderBundleEncoder records reusable render commands.
type RenderBundleEncoder struct {
	hal      hal.RenderBundleEncoder
	device   *Device
	finished atomic.Bool
}

func (e *RenderBundleEncoder) SetPipeline(pipeline *RenderPipeline) {
	if e.finished.Load() {
		return
	}

	e.hal.SetPipeline(pipeline.hal)
}

func (e *RenderBundleEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	if e.finished.Load() {
		return
	}

	e.hal.SetBindGroup(index, group.hal, offsets)
}

func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	if e.finished.Load() {
		return
	}

	e.hal.SetVertexBuffer(slot, buffer.halBuffer(), offset)
}

func (e *RenderBundleEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
	if e.finished.Load() {
		return
	}

	e.hal.SetIndexBuffer(buffer.halBuffer(), format, offset)
}

func (e *RenderBundleEncoder) Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32) {
	if e.finished.Load() {
		return
	}

	e.hal.Draw(gputypes.DrawArgs{
		VertexCount:   vertexCount,
		InstanceCount: instanceCount,
		FirstVertex:   firstVertex,
		FirstInstance: firstInstance,
	})
}

func (e *RenderBundleEncoder) DrawIndexed(indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	if e.finished.Load() {
		return
	}

	e.hal.DrawIndexed(gputypes.DrawIndexedArgs{
		IndexCount:    indexCount,
		InstanceCount: instanceCount,
		FirstIndex:    firstIndex,
		BaseVertex:    baseVertex,
		FirstInstance: firstInstance,
	})
}

// Finish completes recording and returns the reusable render bundle.
func (e *RenderBundleEncoder) Finish(desc *RenderBundleDescriptor) (*RenderBundle, error) {
	if e == nil || e.hal == nil || e.finished.Swap(true) {
		return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: %w", ErrReleased)
	}

	label := ""
	if desc != nil {
		label = desc.Label
	}
	return &RenderBundle{hal: e.hal.Finish(), device: e.device, label: label}, nil
}

// RenderBundle is a reusable sequence of render commands.
type RenderBundle struct {
	hal      hal.RenderBundle
	device   *Device
	label    string
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
