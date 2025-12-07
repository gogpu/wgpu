package types

// BufferUsage describes how a buffer can be used.
type BufferUsage uint32

const (
	// BufferUsageMapRead allows mapping the buffer for reading.
	BufferUsageMapRead BufferUsage = 1 << iota
	// BufferUsageMapWrite allows mapping the buffer for writing.
	BufferUsageMapWrite
	// BufferUsageCopySrc allows the buffer to be a copy source.
	BufferUsageCopySrc
	// BufferUsageCopyDst allows the buffer to be a copy destination.
	BufferUsageCopyDst
	// BufferUsageIndex allows use as an index buffer.
	BufferUsageIndex
	// BufferUsageVertex allows use as a vertex buffer.
	BufferUsageVertex
	// BufferUsageUniform allows use as a uniform buffer.
	BufferUsageUniform
	// BufferUsageStorage allows use as a storage buffer.
	BufferUsageStorage
	// BufferUsageIndirect allows use for indirect draw/dispatch.
	BufferUsageIndirect
	// BufferUsageQueryResolve allows use for query result resolution.
	BufferUsageQueryResolve
)

// BufferDescriptor describes a buffer.
type BufferDescriptor struct {
	// Label is a debug label.
	Label string
	// Size is the buffer size in bytes.
	Size uint64
	// Usage describes how the buffer will be used.
	Usage BufferUsage
	// MappedAtCreation indicates if the buffer is mapped at creation.
	MappedAtCreation bool
}

// BufferMapState describes the map state of a buffer.
type BufferMapState uint8

const (
	// BufferMapStateUnmapped means the buffer is not mapped.
	BufferMapStateUnmapped BufferMapState = iota
	// BufferMapStatePending means a map operation is pending.
	BufferMapStatePending
	// BufferMapStateMapped means the buffer is mapped.
	BufferMapStateMapped
)

// MapMode describes the access mode for buffer mapping.
type MapMode uint8

const (
	// MapModeRead maps the buffer for reading.
	MapModeRead MapMode = 1 << iota
	// MapModeWrite maps the buffer for writing.
	MapModeWrite
)

// BufferBindingType describes how a buffer is bound.
type BufferBindingType uint8

const (
	// BufferBindingTypeUndefined is an undefined binding type.
	BufferBindingTypeUndefined BufferBindingType = iota
	// BufferBindingTypeUniform binds as a uniform buffer.
	BufferBindingTypeUniform
	// BufferBindingTypeStorage binds as a storage buffer (read-write).
	BufferBindingTypeStorage
	// BufferBindingTypeReadOnlyStorage binds as a read-only storage buffer.
	BufferBindingTypeReadOnlyStorage
)

// IndexFormat describes the format of index buffer data.
type IndexFormat uint8

const (
	// IndexFormatUint16 uses 16-bit unsigned integers.
	IndexFormatUint16 IndexFormat = iota
	// IndexFormatUint32 uses 32-bit unsigned integers.
	IndexFormatUint32
)
