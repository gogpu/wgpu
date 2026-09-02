//go:build !(js && wasm)

package core

import "github.com/gogpu/wgpu/hal"

// indirectCountInactive reports whether an indirect-count draw should be skipped.
func (p *CoreRenderPassEncoder) indirectCountInactive(maxDrawCount uint32) bool {
	return p.ended || maxDrawCount == 0
}

type indirectCountHALForwarder func(halIndirect, halCount hal.Buffer, indirectOffset, countOffset uint64, maxDrawCount uint32)

func (p *CoreRenderPassEncoder) forwardIndirectCount(
	indirectBuffer, countBuffer *Buffer,
	indirectOffset, countOffset uint64,
	maxDrawCount uint32,
	forward indirectCountHALForwarder,
) {
	if p.indirectCountInactive(maxDrawCount) || p.raw == nil || indirectBuffer == nil || countBuffer == nil {
		return
	}
	guard := p.device.snatchLock.Read()
	defer guard.Release()
	halIndirect := indirectBuffer.Raw(guard)
	halCount := countBuffer.Raw(guard)
	if halIndirect != nil && halCount != nil {
		forward(halIndirect, halCount, indirectOffset, countOffset, maxDrawCount)
	}
}
