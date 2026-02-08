// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package gl provides OpenGL constants and types for the GLES backend.
package gl

// OpenGL ES 3.0 / OpenGL 3.3 constants.
// These are the subset needed for WebGPU implementation.
// OpenGL constants use ALL_CAPS by industry convention.
//
//nolint:revive
const (
	// Boolean values
	FALSE = 0
	TRUE  = 1

	// Data types
	BYTE           = 0x1400
	UNSIGNED_BYTE  = 0x1401
	SHORT          = 0x1402
	UNSIGNED_SHORT = 0x1403
	INT            = 0x1404
	UNSIGNED_INT   = 0x1405
	FLOAT          = 0x1406
	HALF_FLOAT     = 0x140B

	// Errors
	NO_ERROR                      = 0
	INVALID_ENUM                  = 0x0500
	INVALID_VALUE                 = 0x0501
	INVALID_OPERATION             = 0x0502
	OUT_OF_MEMORY                 = 0x0505
	INVALID_FRAMEBUFFER_OPERATION = 0x0506

	// Capabilities
	BLEND        = 0x0BE2
	CULL_FACE    = 0x0B44
	DEPTH_TEST   = 0x0B71
	DITHER       = 0x0BD0
	SCISSOR_TEST = 0x0C11
	STENCIL_TEST = 0x0B90

	// Buffer targets
	ARRAY_BUFFER              = 0x8892
	ELEMENT_ARRAY_BUFFER      = 0x8893
	UNIFORM_BUFFER            = 0x8A11
	COPY_READ_BUFFER          = 0x8F36
	COPY_WRITE_BUFFER         = 0x8F37
	PIXEL_PACK_BUFFER         = 0x88EB
	PIXEL_UNPACK_BUFFER       = 0x88EC
	TRANSFORM_FEEDBACK_BUFFER = 0x8C8E

	// Buffer access modes
	READ_ONLY  = 0x88B8
	WRITE_ONLY = 0x88B9
	READ_WRITE = 0x88BA

	// Buffer usage
	STREAM_DRAW  = 0x88E0
	STREAM_READ  = 0x88E1
	STREAM_COPY  = 0x88E2
	STATIC_DRAW  = 0x88E4
	STATIC_READ  = 0x88E5
	STATIC_COPY  = 0x88E6
	DYNAMIC_DRAW = 0x88E8
	DYNAMIC_READ = 0x88E9
	DYNAMIC_COPY = 0x88EA

	// Texture targets
	TEXTURE_2D                  = 0x0DE1
	TEXTURE_3D                  = 0x806F
	TEXTURE_2D_ARRAY            = 0x8C1A
	TEXTURE_CUBE_MAP            = 0x8513
	TEXTURE_CUBE_MAP_POSITIVE_X = 0x8515
	TEXTURE_CUBE_MAP_NEGATIVE_X = 0x8516
	TEXTURE_CUBE_MAP_POSITIVE_Y = 0x8517
	TEXTURE_CUBE_MAP_NEGATIVE_Y = 0x8518
	TEXTURE_CUBE_MAP_POSITIVE_Z = 0x8519
	TEXTURE_CUBE_MAP_NEGATIVE_Z = 0x851A

	// Texture parameters
	TEXTURE_MAG_FILTER = 0x2800
	TEXTURE_MIN_FILTER = 0x2801
	TEXTURE_WRAP_S     = 0x2802
	TEXTURE_WRAP_T     = 0x2803
	TEXTURE_WRAP_R     = 0x8072

	// Texture filter modes
	NEAREST                = 0x2600
	LINEAR                 = 0x2601
	NEAREST_MIPMAP_NEAREST = 0x2700
	LINEAR_MIPMAP_NEAREST  = 0x2701
	NEAREST_MIPMAP_LINEAR  = 0x2702
	LINEAR_MIPMAP_LINEAR   = 0x2703

	// Texture wrap modes
	REPEAT          = 0x2901
	CLAMP_TO_EDGE   = 0x812F
	MIRRORED_REPEAT = 0x8370

	// Pixel formats
	DEPTH_COMPONENT   = 0x1902
	RED               = 0x1903
	RG                = 0x8227
	RGB               = 0x1907
	RGBA              = 0x1908
	DEPTH_STENCIL     = 0x84F9
	RED_INTEGER       = 0x8D94
	RG_INTEGER        = 0x8228
	RGB_INTEGER       = 0x8D98
	RGBA_INTEGER      = 0x8D99
	BGRA              = 0x80E1
	DEPTH_COMPONENT16 = 0x81A5
	DEPTH_COMPONENT24 = 0x81A6
	DEPTH_COMPONENT32 = 0x81A7
	DEPTH24_STENCIL8  = 0x88F0

	// Internal formats
	R8                = 0x8229
	R16F              = 0x822D
	R32F              = 0x822E
	RG8               = 0x822B
	RG16F             = 0x822F
	RG32F             = 0x8230
	RGB8              = 0x8051
	RGBA8             = 0x8058
	SRGB8             = 0x8C41
	SRGB8_ALPHA8      = 0x8C43
	RGB16F            = 0x881B
	RGBA16F           = 0x881A
	RGB32F            = 0x8815
	RGBA32F           = 0x8814
	R8I               = 0x8231
	R8UI              = 0x8232
	R16I              = 0x8233
	R16UI             = 0x8234
	R32I              = 0x8235
	R32UI             = 0x8236
	RG8I              = 0x8237
	RG8UI             = 0x8238
	RG16I             = 0x8239
	RG16UI            = 0x823A
	RG32I             = 0x823B
	RG32UI            = 0x823C
	RGBA8I            = 0x8D8E
	RGBA8UI           = 0x8D7C
	RGBA16I           = 0x8D88
	RGBA16UI          = 0x8D76
	RGBA32I           = 0x8D82
	RGBA32UI          = 0x8D70
	DEPTH32F_STENCIL8 = 0x8CAD

	// Shader types
	FRAGMENT_SHADER = 0x8B30
	VERTEX_SHADER   = 0x8B31
	COMPUTE_SHADER  = 0x91B9

	// Shader parameters
	COMPILE_STATUS       = 0x8B81
	LINK_STATUS          = 0x8B82
	INFO_LOG_LENGTH      = 0x8B84
	ACTIVE_UNIFORMS      = 0x8B86
	ACTIVE_ATTRIBUTES    = 0x8B89
	SHADER_SOURCE_LENGTH = 0x8B88

	// Draw modes
	POINTS         = 0x0000
	LINES          = 0x0001
	LINE_LOOP      = 0x0002
	LINE_STRIP     = 0x0003
	TRIANGLES      = 0x0004
	TRIANGLE_STRIP = 0x0005
	TRIANGLE_FAN   = 0x0006

	// Blend factors
	ZERO                     = 0
	ONE                      = 1
	SRC_COLOR                = 0x0300
	ONE_MINUS_SRC_COLOR      = 0x0301
	SRC_ALPHA                = 0x0302
	ONE_MINUS_SRC_ALPHA      = 0x0303
	DST_ALPHA                = 0x0304
	ONE_MINUS_DST_ALPHA      = 0x0305
	DST_COLOR                = 0x0306
	ONE_MINUS_DST_COLOR      = 0x0307
	SRC_ALPHA_SATURATE       = 0x0308
	CONSTANT_COLOR           = 0x8001
	ONE_MINUS_CONSTANT_COLOR = 0x8002
	CONSTANT_ALPHA           = 0x8003
	ONE_MINUS_CONSTANT_ALPHA = 0x8004

	// Blend equations
	FUNC_ADD              = 0x8006
	FUNC_SUBTRACT         = 0x800A
	FUNC_REVERSE_SUBTRACT = 0x800B
	MIN                   = 0x8007
	MAX                   = 0x8008

	// Depth functions
	NEVER    = 0x0200
	LESS     = 0x0201
	EQUAL    = 0x0202
	LEQUAL   = 0x0203
	GREATER  = 0x0204
	NOTEQUAL = 0x0205
	GEQUAL   = 0x0206
	ALWAYS   = 0x0207

	// Stencil operations
	KEEP      = 0x1E00
	REPLACE   = 0x1E01
	INCR      = 0x1E02
	DECR      = 0x1E03
	INVERT    = 0x150A
	INCR_WRAP = 0x8507
	DECR_WRAP = 0x8508

	// Face culling
	FRONT          = 0x0404
	BACK           = 0x0405
	FRONT_AND_BACK = 0x0408
	CW             = 0x0900
	CCW            = 0x0901

	// Framebuffer
	FRAMEBUFFER              = 0x8D40
	READ_FRAMEBUFFER         = 0x8CA8
	DRAW_FRAMEBUFFER         = 0x8CA9
	RENDERBUFFER             = 0x8D41
	COLOR_ATTACHMENT0        = 0x8CE0
	DEPTH_ATTACHMENT         = 0x8D00
	STENCIL_ATTACHMENT       = 0x8D20
	DEPTH_STENCIL_ATTACHMENT = 0x821A
	FRAMEBUFFER_COMPLETE     = 0x8CD5

	// Clear bits
	COLOR_BUFFER_BIT   = 0x00004000
	DEPTH_BUFFER_BIT   = 0x00000100
	STENCIL_BUFFER_BIT = 0x00000400

	// Get parameters
	VENDOR                           = 0x1F00
	RENDERER                         = 0x1F01
	VERSION                          = 0x1F02
	SHADING_LANGUAGE_VERSION         = 0x8B8C
	EXTENSIONS                       = 0x1F03
	MAX_TEXTURE_SIZE                 = 0x0D33
	MAX_TEXTURE_IMAGE_UNITS          = 0x8872
	MAX_VERTEX_ATTRIBS               = 0x8869
	MAX_VERTEX_UNIFORM_COMPONENTS    = 0x8B4A
	MAX_FRAGMENT_UNIFORM_COMPONENTS  = 0x8B49
	MAX_UNIFORM_BUFFER_BINDINGS      = 0x8A2F
	MAX_UNIFORM_BLOCK_SIZE           = 0x8A30
	MAX_COMBINED_TEXTURE_IMAGE_UNITS = 0x8B4D
	MAX_COLOR_ATTACHMENTS            = 0x8CDF
	MAX_DRAW_BUFFERS                 = 0x8824
	MAX_RENDERBUFFER_SIZE            = 0x84E8
	MAX_SAMPLES                      = 0x8D57

	// VAO
	VERTEX_ARRAY_BINDING = 0x85B5

	// Sync objects
	SYNC_GPU_COMMANDS_COMPLETE = 0x9117
	ALREADY_SIGNALED           = 0x911A
	TIMEOUT_EXPIRED            = 0x911B
	CONDITION_SATISFIED        = 0x911C
	WAIT_FAILED                = 0x911D
	SYNC_FLUSH_COMMANDS_BIT    = 0x00000001
	TIMEOUT_IGNORED            = 0xFFFFFFFFFFFFFFFF

	// Compute shader constants (OpenGL ES 3.1+ / OpenGL 4.3+)
	MAX_COMPUTE_WORK_GROUP_COUNT       = 0x91BE
	MAX_COMPUTE_WORK_GROUP_SIZE        = 0x91BF
	MAX_COMPUTE_WORK_GROUP_INVOCATIONS = 0x90EB

	// Memory barrier bits (OpenGL ES 3.1+ / OpenGL 4.2+)
	VERTEX_ATTRIB_ARRAY_BARRIER_BIT = 0x00000001
	ELEMENT_ARRAY_BARRIER_BIT       = 0x00000002
	UNIFORM_BARRIER_BIT             = 0x00000004
	TEXTURE_FETCH_BARRIER_BIT       = 0x00000008
	SHADER_IMAGE_ACCESS_BARRIER_BIT = 0x00000020
	COMMAND_BARRIER_BIT             = 0x00000040
	PIXEL_BUFFER_BARRIER_BIT        = 0x00000080
	TEXTURE_UPDATE_BARRIER_BIT      = 0x00000100
	BUFFER_UPDATE_BARRIER_BIT       = 0x00000200
	FRAMEBUFFER_BARRIER_BIT         = 0x00000400
	TRANSFORM_FEEDBACK_BARRIER_BIT  = 0x00000800
	ATOMIC_COUNTER_BARRIER_BIT      = 0x00001000
	SHADER_STORAGE_BARRIER_BIT      = 0x00002000
	ALL_BARRIER_BITS                = 0xFFFFFFFF

	// Indirect dispatch buffer (OpenGL ES 3.1+ / OpenGL 4.3+)
	DISPATCH_INDIRECT_BUFFER = 0x90EE

	// Shader storage buffer (OpenGL ES 3.1+ / OpenGL 4.3+)
	SHADER_STORAGE_BUFFER = 0x90D2
)
