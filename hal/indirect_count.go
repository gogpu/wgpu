//go:build !(js && wasm)

package hal

// IndirectCountRecorder issues a multi-draw indirect call with a fixed draw count.
type IndirectCountRecorder func(buffer Buffer, offset uint64, drawCount uint32)

// RecordIndirectCountMax records an indirect draw using maxDrawCount when the
// backend cannot consume a GPU-provided count buffer.
func RecordIndirectCountMax(
	record IndirectCountRecorder,
	buffer Buffer,
	offset uint64,
	countBuffer Buffer,
	countOffset uint64,
	maxDrawCount uint32,
) {
	_ = countBuffer
	_ = countOffset
	if maxDrawCount == 0 {
		return
	}
	record(buffer, offset, maxDrawCount)
}
