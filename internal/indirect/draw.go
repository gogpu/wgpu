// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package indirect

// The argument-record sizes of the indirect draw commands (WebGPU spec,
// drawIndirect and drawIndexedIndirect). They live with the span arithmetic
// so every caller shares one definition: the render pass uses them for the
// "does the whole span fit" checks and for the per-record offsets.
const (
	// DrawIndirectRecordSize is the size of one DrawIndirect argument record:
	// vertexCount, instanceCount, firstVertex, firstInstance (4 × uint32).
	DrawIndirectRecordSize uint64 = 16
	// DrawIndexedIndirectRecordSize is the size of one DrawIndexedIndirect
	// argument record: indexCount, instanceCount, firstIndex, baseVertex,
	// firstInstance (5 × uint32).
	DrawIndexedIndirectRecordSize uint64 = 20
)
