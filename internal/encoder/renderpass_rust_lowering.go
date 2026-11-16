//go:build rust

package encoder

import "github.com/gogpu/wgpu/internal/indirect"

// lowerRustIndexedIndirect lowers one counted span through the Rust adapter's
// single-record interface. Invalid positive spans delegate exactly one failing
// record before any valid record can be emitted.
//
// The declaration lived in the root package from the S5 Prepare until the
// Move (it stayed there only so the planner would accept the moving set); it
// relocates here with the rust render pass, and the white-box test
// renderpass_indirect_rust_test.go follows it.
func lowerRustIndexedIndirect(bufferSize, offset uint64, drawCount uint32, draw func(uint64)) {
	if drawCount == 0 {
		return
	}
	if !indirect.RangeFits(bufferSize, offset, indirect.DrawIndexedIndirectRecordSize, drawCount) {
		draw(indirect.DelegatedValidationOffset(bufferSize, offset, indirect.DrawIndexedIndirectRecordSize, drawCount))
		return
	}
	for i := uint32(0); i < drawCount; i++ {
		recordOffset, _ := indirect.RecordOffset(offset, indirect.DrawIndexedIndirectRecordSize, i)
		draw(recordOffset)
	}
}
