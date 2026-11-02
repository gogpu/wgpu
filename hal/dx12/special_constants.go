// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

// specialConstantsCount is the number of 32-bit values in the special
// constants root parameter.
const specialConstantsCount = 3

// specialConstants are the values naga reads from the special constants root
// parameter: (first_vertex, first_instance, unused) for a draw and the
// workgroup count (x, y, z) for a dispatch, as Rust wgpu-hal's
// SpecialConstants (dx12/mod.rs). first_vertex is a draw's first vertex or an
// indexed draw's base vertex, stored as the bits of an int32.
type specialConstants [specialConstantsCount]uint32

func drawSpecialConstants(firstVertex int32, firstInstance uint32) specialConstants {
	return specialConstants{uint32(firstVertex), firstInstance, 0} // the int32's bits
}

// prepareDraw binds the dirty vertex buffers and bind groups and writes the
// draw's special constants when they changed, as Rust wgpu-hal's prepare_draw
// (dx12/command.rs).
func (e *RenderPassEncoder) prepareDraw(firstVertex int32, firstInstance uint32) {
	e.prepareVertexBuffers()
	if e.pipeline == nil {
		return
	}
	e.bindGroups.flush(e.encoder, false, e.pipeline.groupMappings)
	if e.pipeline.specialConstantsRootIndex < 0 {
		return
	}
	sc := drawSpecialConstants(firstVertex, firstInstance)
	if e.specialConstantsSet && e.specialConstants == sc {
		return
	}
	root := uint32(e.pipeline.specialConstantsRootIndex)
	for i, v := range sc {
		e.encoder.cmdList.SetGraphicsRoot32BitConstant(root, v, uint32(i))
	}
	e.specialConstants, e.specialConstantsSet = sc, true
}

// prepareDispatch binds the dirty bind groups and writes the dispatch's
// workgroup count into the special constants when it changed, as Rust
// wgpu-hal's prepare_dispatch.
func (e *ComputePassEncoder) prepareDispatch(x, y, z uint32) {
	if e.pipeline == nil {
		return
	}
	e.bindGroups.flush(e.encoder, true, e.pipeline.groupMappings)
	if e.pipeline.specialConstantsRootIndex < 0 {
		return
	}
	sc := specialConstants{x, y, z}
	if e.specialConstantsSet && e.specialConstants == sc {
		return
	}
	root := uint32(e.pipeline.specialConstantsRootIndex)
	for i, v := range sc {
		e.encoder.cmdList.SetComputeRoot32BitConstant(root, v, uint32(i))
	}
	e.specialConstants, e.specialConstantsSet = sc, true
}
