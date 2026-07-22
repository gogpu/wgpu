package wgpu

import "github.com/gogpu/wgpu/internal/indirect"

const (
	drawIndirectRecordSize        = uint64(16)
	drawIndexedIndirectRecordSize = uint64(20)
	indexedIndirectRecordSize     = drawIndexedIndirectRecordSize
)

func indirectRangeFits(bufferSize, offset, recordSize uint64, drawCount uint32) bool {
	return indirect.RangeFits(bufferSize, offset, recordSize, drawCount)
}

func indirectRecordOffset(offset, recordSize uint64, index uint32) (uint64, bool) {
	return indirect.RecordOffset(offset, recordSize, index)
}

func drawIndirectRangeFits(bufferSize, offset uint64, drawCount uint32) bool {
	return indirectRangeFits(bufferSize, offset, drawIndirectRecordSize, drawCount)
}

func drawIndirectRecordOffset(offset uint64, index uint32) (uint64, bool) {
	return indirectRecordOffset(offset, drawIndirectRecordSize, index)
}

func indirectDelegatedValidationOffset(bufferSize, offset, recordSize uint64, drawCount uint32) uint64 {
	lastOffset, ok := indirectRecordOffset(offset, recordSize, drawCount-1)
	if !ok {
		return bufferSize
	}
	return lastOffset
}

// indexedIndirectRangeFits reports whether drawCount consecutive indexed
// indirect argument records fit in a buffer without overflowing uint64 math.
func indexedIndirectRangeFits(bufferSize, offset uint64, drawCount uint32) bool {
	if offset > bufferSize {
		return false
	}
	return indirectRangeFits(bufferSize, offset, drawIndexedIndirectRecordSize, drawCount)
}

// indexedIndirectRecordOffset returns the byte offset of one record in a
// counted indexed-indirect span without allowing uint64 wraparound.
func indexedIndirectRecordOffset(offset uint64, index uint32) (uint64, bool) {
	return indirectRecordOffset(offset, drawIndexedIndirectRecordSize, index)
}
