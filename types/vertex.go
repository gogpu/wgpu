package types

// VertexFormat describes a vertex attribute format.
type VertexFormat uint8

const (
	VertexFormatUint8x2 VertexFormat = iota
	VertexFormatUint8x4
	VertexFormatSint8x2
	VertexFormatSint8x4
	VertexFormatUnorm8x2
	VertexFormatUnorm8x4
	VertexFormatSnorm8x2
	VertexFormatSnorm8x4
	VertexFormatUint16x2
	VertexFormatUint16x4
	VertexFormatSint16x2
	VertexFormatSint16x4
	VertexFormatUnorm16x2
	VertexFormatUnorm16x4
	VertexFormatSnorm16x2
	VertexFormatSnorm16x4
	VertexFormatFloat16x2
	VertexFormatFloat16x4
	VertexFormatFloat32
	VertexFormatFloat32x2
	VertexFormatFloat32x3
	VertexFormatFloat32x4
	VertexFormatUint32
	VertexFormatUint32x2
	VertexFormatUint32x3
	VertexFormatUint32x4
	VertexFormatSint32
	VertexFormatSint32x2
	VertexFormatSint32x3
	VertexFormatSint32x4
	VertexFormatUnorm1010102
)

// Size returns the byte size of the vertex format.
func (f VertexFormat) Size() uint64 {
	switch f {
	case VertexFormatUint8x2, VertexFormatSint8x2, VertexFormatUnorm8x2, VertexFormatSnorm8x2:
		return 2
	case VertexFormatUint8x4, VertexFormatSint8x4, VertexFormatUnorm8x4, VertexFormatSnorm8x4,
		VertexFormatUint16x2, VertexFormatSint16x2, VertexFormatUnorm16x2, VertexFormatSnorm16x2,
		VertexFormatFloat16x2, VertexFormatFloat32, VertexFormatUint32, VertexFormatSint32,
		VertexFormatUnorm1010102:
		return 4
	case VertexFormatUint16x4, VertexFormatSint16x4, VertexFormatUnorm16x4, VertexFormatSnorm16x4,
		VertexFormatFloat16x4, VertexFormatFloat32x2, VertexFormatUint32x2, VertexFormatSint32x2:
		return 8
	case VertexFormatFloat32x3, VertexFormatUint32x3, VertexFormatSint32x3:
		return 12
	case VertexFormatFloat32x4, VertexFormatUint32x4, VertexFormatSint32x4:
		return 16
	default:
		return 0
	}
}

// VertexStepMode describes how vertex data is stepped.
type VertexStepMode uint8

const (
	// VertexStepModeVertex steps per vertex.
	VertexStepModeVertex VertexStepMode = iota
	// VertexStepModeInstance steps per instance.
	VertexStepModeInstance
)

// VertexAttribute describes a vertex attribute.
type VertexAttribute struct {
	// Format is the attribute format.
	Format VertexFormat
	// Offset is the byte offset within the vertex buffer.
	Offset uint64
	// ShaderLocation is the location in the shader.
	ShaderLocation uint32
}

// VertexBufferLayout describes a vertex buffer layout.
type VertexBufferLayout struct {
	// ArrayStride is the stride between vertices in bytes.
	ArrayStride uint64
	// StepMode describes how the buffer is stepped.
	StepMode VertexStepMode
	// Attributes are the vertex attributes.
	Attributes []VertexAttribute
}

// VertexState describes the vertex stage of a render pipeline.
type VertexState struct {
	// Buffers are the vertex buffer layouts.
	Buffers []VertexBufferLayout
}
