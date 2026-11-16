//go:build rust

package wgpu

import "github.com/gogpu/wgpu/internal/indirect"

// lowerRustIndexedIndirect lowers one counted span through the Rust adapter's
// single-record interface. Invalid positive spans delegate exactly one failing
// record before any valid record can be emitted.
//
// The declaration lived in renderpass_rust.go until the S5 Prepare (2026-11-16)
// and stays here because renderpass_indirect_rust_test.go names it: the
// ADR-070 planner's `unexported-boundary` class refuses a plan whose staying
// tree names an unexported declaration that moves. The moving file and the test
// reach it through handRewrite-deferred references.
//
// Move worklist (S5): the function relocates into internal/encoder with the
// render pass; the white-box test renderpass_indirect_rust_test.go follows it
// into the destination.
func lowerRustIndexedIndirect(bufferSize, offset uint64, drawCount uint32, draw func(uint64)) {
	if drawCount == 0 {
		return
	}
	if !indexedIndirectRangeFits(bufferSize, offset, drawCount) {
		draw(indirect.DelegatedValidationOffset(bufferSize, offset, drawIndexedIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, drawIndexedIndirectRecordSize, i)
		draw(recordOffset)
	}
}
