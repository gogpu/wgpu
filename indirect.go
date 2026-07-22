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

func drawIndirectRangeFits(bufferSize, offset uint64, drawCount uint32) bool {
	return indirectRangeFits(bufferSize, offset, drawIndirectRecordSize, drawCount)
}

// indexedIndirectRangeFits reports whether drawCount consecutive indexed
// indirect argument records fit in a buffer without overflowing uint64 math.
func indexedIndirectRangeFits(bufferSize, offset uint64, drawCount uint32) bool {
	if offset > bufferSize {
		return false
	}
	return indirectRangeFits(bufferSize, offset, drawIndexedIndirectRecordSize, drawCount)
}
