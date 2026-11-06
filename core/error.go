//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
)

// unnamedLabel is the default label for resources without a name.
const unnamedLabel = "<unnamed>"

// Base errors for the core package.
var (
	// ErrInvalidID is returned when an ID is invalid or zero.
	ErrInvalidID = errors.New("invalid resource ID")

	// ErrResourceNotFound is returned when a resource is not found in the registry.
	ErrResourceNotFound = errors.New("resource not found")

	// ErrEpochMismatch is returned when the epoch of an ID doesn't match the stored resource.
	ErrEpochMismatch = errors.New("epoch mismatch: resource was recycled")

	// ErrRegistryFull is returned when the registry cannot allocate more IDs.
	ErrRegistryFull = errors.New("registry full: maximum resources reached")

	// ErrResourceInUse is returned when trying to unregister a resource that is still in use.
	ErrResourceInUse = errors.New("resource is still in use")

	// ErrAlreadyDestroyed is returned when operating on an already destroyed resource.
	ErrAlreadyDestroyed = errors.New("resource already destroyed")

	// ErrDeviceLost is returned when the GPU device is lost (e.g., driver crash, GPU reset).
	ErrDeviceLost = errors.New("device lost")

	// ErrDeviceDestroyed is returned when operating on a destroyed device.
	ErrDeviceDestroyed = errors.New("device destroyed")

	// ErrResourceDestroyed is returned when operating on a destroyed resource.
	ErrResourceDestroyed = errors.New("resource destroyed")
)

// ValidationError represents a validation failure with context.
type ValidationError struct {
	Resource string // Resource type (e.g., "Buffer", "Texture")
	Field    string // Field that failed validation
	Message  string // Detailed error message
	Cause    error  // Underlying cause, if any
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s.%s: %s", e.Resource, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Resource, e.Message)
}

// Unwrap returns the underlying cause.
func (e *ValidationError) Unwrap() error {
	return e.Cause
}

// NewValidationError creates a new validation error.
func NewValidationError(resource, field, message string) *ValidationError {
	return &ValidationError{
		Resource: resource,
		Field:    field,
		Message:  message,
	}
}

// NewValidationErrorf creates a new validation error with formatted message.
func NewValidationErrorf(resource, field, format string, args ...any) *ValidationError {
	return &ValidationError{
		Resource: resource,
		Field:    field,
		Message:  fmt.Sprintf(format, args...),
	}
}

// IDError represents an error related to resource IDs.
type IDError struct {
	ID      RawID  // The problematic ID
	Message string // Error description
	Cause   error  // Underlying cause
}

// Error implements the error interface.
func (e *IDError) Error() string {
	index, epoch := e.ID.Unzip()
	return fmt.Sprintf("ID(%d,%d): %s", index, epoch, e.Message)
}

// Unwrap returns the underlying cause.
func (e *IDError) Unwrap() error {
	return e.Cause
}

// NewIDError creates a new ID error.
func NewIDError(id RawID, message string, cause error) *IDError {
	return &IDError{
		ID:      id,
		Message: message,
		Cause:   cause,
	}
}

// LimitError represents exceeding a resource limit.
type LimitError struct {
	Limit    string // Name of the limit
	Actual   uint64 // Actual value
	Maximum  uint64 // Maximum allowed value
	Resource string // Resource type affected
}

// Error implements the error interface.
func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s exceeded (got %d, max %d)",
		e.Resource, e.Limit, e.Actual, e.Maximum)
}

// NewLimitError creates a new limit error.
func NewLimitError(resource, limit string, actual, maximum uint64) *LimitError {
	return &LimitError{
		Limit:    limit,
		Actual:   actual,
		Maximum:  maximum,
		Resource: resource,
	}
}

// FeatureError represents a missing required feature.
type FeatureError struct {
	Feature  string // Name of the missing feature
	Resource string // Resource that requires it
}

// Error implements the error interface.
func (e *FeatureError) Error() string {
	return fmt.Sprintf("%s: requires feature '%s' which is not enabled",
		e.Resource, e.Feature)
}

// NewFeatureError creates a new feature error.
func NewFeatureError(resource, feature string) *FeatureError {
	return &FeatureError{
		Feature:  feature,
		Resource: resource,
	}
}

// IsValidationError returns true if the error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// IsIDError returns true if the error is an IDError.
func IsIDError(err error) bool {
	var ie *IDError
	return errors.As(err, &ie)
}

// IsLimitError returns true if the error is a LimitError.
func IsLimitError(err error) bool {
	var le *LimitError
	return errors.As(err, &le)
}

// IsFeatureError returns true if the error is a FeatureError.
func IsFeatureError(err error) bool {
	var fe *FeatureError
	return errors.As(err, &fe)
}

// CreateBufferErrorKind represents the type of buffer creation error.
type CreateBufferErrorKind int

const (
	// CreateBufferErrorZeroSize indicates buffer size was zero.
	CreateBufferErrorZeroSize CreateBufferErrorKind = iota
	// CreateBufferErrorMaxBufferSize indicates buffer size exceeded device limit.
	CreateBufferErrorMaxBufferSize
	// CreateBufferErrorEmptyUsage indicates no usage flags were specified.
	CreateBufferErrorEmptyUsage
	// CreateBufferErrorInvalidUsage indicates unknown usage flags were specified.
	CreateBufferErrorInvalidUsage
	// CreateBufferErrorMapReadWriteExclusive indicates both MAP_READ and MAP_WRITE were specified.
	CreateBufferErrorMapReadWriteExclusive
	// CreateBufferErrorHAL indicates the HAL backend failed to create the buffer.
	CreateBufferErrorHAL
	// CreateBufferErrorMissingDownlevelFlags indicates the usage needs
	// downlevel capabilities the adapter lacks (MissingFlags).
	CreateBufferErrorMissingDownlevelFlags
)

// CreateBufferError represents an error during buffer creation.
type CreateBufferError struct {
	Kind          CreateBufferErrorKind
	Label         string
	RequestedSize uint64
	MaxSize       uint64
	HALError      error
	// MissingFlags are the downlevel flags the usage needs and the adapter
	// lacks (CreateBufferErrorMissingDownlevelFlags).
	MissingFlags gputypes.DownlevelFlags
}

// Error implements the error interface.
func (e *CreateBufferError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateBufferErrorZeroSize:
		return fmt.Sprintf("buffer %q: size must be greater than 0", label)
	case CreateBufferErrorMaxBufferSize:
		return fmt.Sprintf("buffer %q: size %d exceeds maximum %d",
			label, e.RequestedSize, e.MaxSize)
	case CreateBufferErrorEmptyUsage:
		return fmt.Sprintf("buffer %q: usage must not be empty", label)
	case CreateBufferErrorInvalidUsage:
		return fmt.Sprintf("buffer %q: contains invalid usage flags", label)
	case CreateBufferErrorMapReadWriteExclusive:
		return fmt.Sprintf("buffer %q: MAP_READ and MAP_WRITE are mutually exclusive", label)
	case CreateBufferErrorHAL:
		return fmt.Sprintf("buffer %q: HAL error: %v", label, e.HALError)
	case CreateBufferErrorMissingDownlevelFlags:
		return fmt.Sprintf("buffer %q: usage needs downlevel flags the adapter lacks: %s", label, e.MissingFlags)
	default:
		return fmt.Sprintf("buffer %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateBufferError) Unwrap() error {
	return e.HALError
}

// IsCreateBufferError returns true if the error is a CreateBufferError.
func IsCreateBufferError(err error) bool {
	var cbe *CreateBufferError
	return errors.As(err, &cbe)
}

// =============================================================================
// Command Encoder Errors
// =============================================================================

// CreateCommandEncoderErrorKind represents the type of command encoder creation error.
type CreateCommandEncoderErrorKind int

const (
	// CreateCommandEncoderErrorHAL indicates the HAL backend failed to create the encoder.
	CreateCommandEncoderErrorHAL CreateCommandEncoderErrorKind = iota
)

// CreateCommandEncoderError represents an error during command encoder creation.
type CreateCommandEncoderError struct {
	Kind     CreateCommandEncoderErrorKind
	Label    string
	HALError error
}

// Error implements the error interface.
func (e *CreateCommandEncoderError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateCommandEncoderErrorHAL:
		return fmt.Sprintf("command encoder %q: HAL error: %v", label, e.HALError)
	default:
		return fmt.Sprintf("command encoder %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateCommandEncoderError) Unwrap() error {
	return e.HALError
}

// IsCreateCommandEncoderError returns true if the error is a CreateCommandEncoderError.
func IsCreateCommandEncoderError(err error) bool {
	var cee *CreateCommandEncoderError
	return errors.As(err, &cee)
}

// =============================================================================
// Texture Creation Errors
// =============================================================================

// CreateTextureErrorKind represents the type of texture creation error.
type CreateTextureErrorKind int

const (
	// CreateTextureErrorZeroDimension indicates a texture dimension was zero.
	CreateTextureErrorZeroDimension CreateTextureErrorKind = iota
	// CreateTextureErrorMaxDimension indicates a texture dimension exceeded the device limit.
	CreateTextureErrorMaxDimension
	// CreateTextureErrorMaxArrayLayers indicates array layers exceeded the device limit.
	CreateTextureErrorMaxArrayLayers
	// CreateTextureErrorInvalidMipLevelCount indicates an invalid mip level count.
	CreateTextureErrorInvalidMipLevelCount
	// CreateTextureErrorInvalidSampleCount indicates an invalid sample count (must be 1 or 4).
	CreateTextureErrorInvalidSampleCount
	// CreateTextureErrorMultisampleMipLevel indicates multisampled texture must have mip level count of 1.
	CreateTextureErrorMultisampleMipLevel
	// CreateTextureErrorMultisampleDimension indicates multisampled texture must be 2D.
	CreateTextureErrorMultisampleDimension
	// CreateTextureErrorMultisampleArrayLayers indicates multisampled texture must have 1 array layer.
	CreateTextureErrorMultisampleArrayLayers
	// CreateTextureErrorMultisampleStorageBinding indicates multisampled texture cannot have storage binding.
	CreateTextureErrorMultisampleStorageBinding
	// CreateTextureErrorEmptyUsage indicates no usage flags were specified.
	CreateTextureErrorEmptyUsage
	// CreateTextureErrorInvalidUsage indicates unknown usage flags were specified.
	CreateTextureErrorInvalidUsage
	// CreateTextureErrorInvalidFormat indicates an invalid texture format.
	CreateTextureErrorInvalidFormat
	// CreateTextureErrorInvalidDimension indicates an invalid texture dimension.
	CreateTextureErrorInvalidDimension
	// CreateTextureErrorHAL indicates the HAL backend failed to create the texture.
	CreateTextureErrorHAL
	// CreateTextureErrorInvalidViewFormat indicates a view format that differs
	// from the texture format by more than the sRGB suffix (Rust
	// CreateTextureError::InvalidViewFormat).
	CreateTextureErrorInvalidViewFormat
	// CreateTextureErrorMissingDownlevelFlags indicates a descriptor that needs
	// a downlevel flag the device lacks (Rust CreateTextureError::MissingDownlevelFlags).
	CreateTextureErrorMissingDownlevelFlags
)

// CreateTextureError represents an error during texture creation.
type CreateTextureError struct {
	Kind             CreateTextureErrorKind
	Label            string
	RequestedWidth   uint32
	RequestedHeight  uint32
	RequestedDepth   uint32
	MaxDimension     uint32
	RequestedMips    uint32
	MaxMips          uint32
	RequestedSamples uint32
	HALError         error
	// Format is the texture format and ViewFormat the offending view format
	// (CreateTextureErrorInvalidViewFormat).
	Format     gputypes.TextureFormat
	ViewFormat gputypes.TextureFormat
	// Cause is the downlevel error (CreateTextureErrorMissingDownlevelFlags).
	Cause error
}

// Error implements the error interface.
func (e *CreateTextureError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateTextureErrorZeroDimension:
		return fmt.Sprintf("texture %q: dimensions must be greater than 0 (got %dx%dx%d)",
			label, e.RequestedWidth, e.RequestedHeight, e.RequestedDepth)
	case CreateTextureErrorMaxDimension:
		return fmt.Sprintf("texture %q: dimension %d exceeds maximum %d",
			label, max(e.RequestedWidth, max(e.RequestedHeight, e.RequestedDepth)), e.MaxDimension)
	case CreateTextureErrorMaxArrayLayers:
		return fmt.Sprintf("texture %q: array layers %d exceeds maximum %d",
			label, e.RequestedDepth, e.MaxDimension)
	case CreateTextureErrorInvalidMipLevelCount:
		return fmt.Sprintf("texture %q: mip level count %d exceeds maximum %d",
			label, e.RequestedMips, e.MaxMips)
	case CreateTextureErrorInvalidSampleCount:
		return fmt.Sprintf("texture %q: invalid sample count %d (must be 1 or 4)",
			label, e.RequestedSamples)
	case CreateTextureErrorMultisampleMipLevel:
		return fmt.Sprintf("texture %q: multisampled texture must have mip level count of 1 (got %d)",
			label, e.RequestedMips)
	case CreateTextureErrorMultisampleDimension:
		return fmt.Sprintf("texture %q: multisampled texture must be 2D", label)
	case CreateTextureErrorMultisampleArrayLayers:
		return fmt.Sprintf("texture %q: multisampled texture must have 1 array layer (got %d)",
			label, e.RequestedDepth)
	case CreateTextureErrorMultisampleStorageBinding:
		return fmt.Sprintf("texture %q: multisampled texture cannot have storage binding", label)
	case CreateTextureErrorEmptyUsage:
		return fmt.Sprintf("texture %q: usage must not be empty", label)
	case CreateTextureErrorInvalidUsage:
		return fmt.Sprintf("texture %q: contains invalid usage flags", label)
	case CreateTextureErrorInvalidFormat:
		return fmt.Sprintf("texture %q: format must not be undefined", label)
	case CreateTextureErrorInvalidDimension:
		return fmt.Sprintf("texture %q: dimension must not be undefined", label)
	case CreateTextureErrorHAL:
		return fmt.Sprintf("texture %q: HAL error: %v", label, e.HALError)
	default:
		return e.viewFormatText(label)
	}
}

// viewFormatText describes the view format kinds of CreateTextureError.
func (e *CreateTextureError) viewFormatText(label string) string {
	switch e.Kind {
	case CreateTextureErrorInvalidViewFormat:
		return fmt.Sprintf("texture %q: view format %v is not compatible with texture format %v (they may differ only by the sRGB suffix)",
			label, e.ViewFormat, e.Format)
	case CreateTextureErrorMissingDownlevelFlags:
		return fmt.Sprintf("texture %q: %v", label, e.Cause)
	default:
		return fmt.Sprintf("texture %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL or downlevel error, if any.
func (e *CreateTextureError) Unwrap() error {
	if e.HALError != nil {
		return e.HALError
	}
	return e.Cause
}

// IsCreateTextureError returns true if the error is a CreateTextureError.
func IsCreateTextureError(err error) bool {
	var cte *CreateTextureError
	return errors.As(err, &cte)
}

// CreateTextureViewErrorKind represents the type of texture view creation error.
type CreateTextureViewErrorKind int

const (
	// CreateTextureViewErrorFormatReinterpretation indicates a view format that is
	// neither the texture format nor one of its view formats (Rust
	// CreateTextureViewError::FormatReinterpretation).
	CreateTextureViewErrorFormatReinterpretation CreateTextureViewErrorKind = iota
)

// CreateTextureViewError represents an error during texture view creation.
type CreateTextureViewError struct {
	Kind CreateTextureViewErrorKind
	// TextureFormat is the texture's format and ViewFormat the requested one.
	TextureFormat gputypes.TextureFormat
	ViewFormat    gputypes.TextureFormat
}

// Error implements the error interface.
func (e *CreateTextureViewError) Error() string {
	switch e.Kind {
	case CreateTextureViewErrorFormatReinterpretation:
		return fmt.Sprintf("texture view: format %v is not the texture format %v and was not listed in the texture's view formats",
			e.ViewFormat, e.TextureFormat)
	default:
		return "texture view: unknown error"
	}
}

// =============================================================================
// Sampler Creation Errors
// =============================================================================

// CreateSamplerErrorKind represents the type of sampler creation error.
type CreateSamplerErrorKind int

const (
	// CreateSamplerErrorInvalidLodMinClamp indicates LodMinClamp was negative.
	CreateSamplerErrorInvalidLodMinClamp CreateSamplerErrorKind = iota
	// CreateSamplerErrorInvalidLodMaxClamp indicates LodMaxClamp was less than LodMinClamp.
	CreateSamplerErrorInvalidLodMaxClamp
	// CreateSamplerErrorInvalidAnisotropy indicates anisotropy was zero.
	CreateSamplerErrorInvalidAnisotropy
	// CreateSamplerErrorAnisotropyRequiresLinearFiltering indicates anisotropy > 1 requires linear filtering.
	CreateSamplerErrorAnisotropyRequiresLinearFiltering
	// CreateSamplerErrorHAL indicates the HAL backend failed to create the sampler.
	CreateSamplerErrorHAL
)

// CreateSamplerError represents an error during sampler creation.
type CreateSamplerError struct {
	Kind        CreateSamplerErrorKind
	Label       string
	LodMinClamp float32
	LodMaxClamp float32
	Anisotropy  uint16
	HALError    error
}

// Error implements the error interface.
func (e *CreateSamplerError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateSamplerErrorInvalidLodMinClamp:
		return fmt.Sprintf("sampler %q: LodMinClamp must be >= 0 (got %f)", label, e.LodMinClamp)
	case CreateSamplerErrorInvalidLodMaxClamp:
		return fmt.Sprintf("sampler %q: LodMaxClamp (%f) must be >= LodMinClamp (%f)",
			label, e.LodMaxClamp, e.LodMinClamp)
	case CreateSamplerErrorInvalidAnisotropy:
		return fmt.Sprintf("sampler %q: anisotropy must be >= 1 (got %d)", label, e.Anisotropy)
	case CreateSamplerErrorAnisotropyRequiresLinearFiltering:
		return fmt.Sprintf("sampler %q: anisotropy %d requires linear mag, min, and mipmap filtering",
			label, e.Anisotropy)
	case CreateSamplerErrorHAL:
		return fmt.Sprintf("sampler %q: HAL error: %v", label, e.HALError)
	default:
		return fmt.Sprintf("sampler %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateSamplerError) Unwrap() error {
	return e.HALError
}

// IsCreateSamplerError returns true if the error is a CreateSamplerError.
func IsCreateSamplerError(err error) bool {
	var cse *CreateSamplerError
	return errors.As(err, &cse)
}

// =============================================================================
// Shader Module Creation Errors
// =============================================================================

// CreateShaderModuleErrorKind represents the type of shader module creation error.
type CreateShaderModuleErrorKind int

const (
	// CreateShaderModuleErrorNoSource indicates neither WGSL nor SPIRV source was provided.
	CreateShaderModuleErrorNoSource CreateShaderModuleErrorKind = iota
	// CreateShaderModuleErrorDualSource indicates both WGSL and SPIRV sources were provided.
	CreateShaderModuleErrorDualSource
	// CreateShaderModuleErrorHAL indicates the HAL backend failed to create the shader module.
	CreateShaderModuleErrorHAL
	// CreateShaderModuleErrorParsing indicates the WGSL source does not parse
	// (Rust CreateShaderModuleError::Parsing). Cause holds naga's error.
	CreateShaderModuleErrorParsing
	// CreateShaderModuleErrorValidation indicates naga's validator rejected
	// the module (Rust CreateShaderModuleError::Validation). Cause holds
	// naga's valid.Errors.
	CreateShaderModuleErrorValidation
)

// CreateShaderModuleError represents an error during shader module creation.
type CreateShaderModuleError struct {
	Kind     CreateShaderModuleErrorKind
	Label    string
	HALError error
	// Cause is naga's error for CreateShaderModuleErrorParsing and
	// CreateShaderModuleErrorValidation.
	Cause error
}

// Error implements the error interface.
func (e *CreateShaderModuleError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateShaderModuleErrorNoSource:
		return fmt.Sprintf("shader module %q: must provide either WGSL or SPIRV source", label)
	case CreateShaderModuleErrorDualSource:
		return fmt.Sprintf("shader module %q: must not provide both WGSL and SPIRV source", label)
	case CreateShaderModuleErrorHAL:
		return fmt.Sprintf("shader module %q: HAL error: %v", label, e.HALError)
	case CreateShaderModuleErrorParsing, CreateShaderModuleErrorValidation:
		// naga's text already says "parse error" or "validation failed".
		return fmt.Sprintf("shader module %q: %v", label, e.Cause)
	default:
		return fmt.Sprintf("shader module %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL or naga error, if any.
func (e *CreateShaderModuleError) Unwrap() error {
	if e.HALError != nil {
		return e.HALError
	}
	return e.Cause
}

// IsCreateShaderModuleError returns true if the error is a CreateShaderModuleError.
func IsCreateShaderModuleError(err error) bool {
	var csme *CreateShaderModuleError
	return errors.As(err, &csme)
}

// =============================================================================
// Render Pipeline Creation Errors
// =============================================================================

// CreateRenderPipelineErrorKind represents the type of render pipeline creation error.
type CreateRenderPipelineErrorKind int

const (
	// CreateRenderPipelineErrorMissingVertexModule indicates the vertex shader module was nil.
	CreateRenderPipelineErrorMissingVertexModule CreateRenderPipelineErrorKind = iota
	// CreateRenderPipelineErrorMissingVertexEntryPoint indicates the vertex entry point was empty.
	CreateRenderPipelineErrorMissingVertexEntryPoint
	// CreateRenderPipelineErrorMissingFragmentModule indicates the fragment shader module was nil.
	CreateRenderPipelineErrorMissingFragmentModule
	// CreateRenderPipelineErrorMissingFragmentEntryPoint indicates the fragment entry point was empty.
	CreateRenderPipelineErrorMissingFragmentEntryPoint
	// CreateRenderPipelineErrorNoFragmentTargets indicates the fragment stage had no color
	// targets and the pipeline has no depth-stencil state: no attachment at all.
	CreateRenderPipelineErrorNoFragmentTargets
	// CreateRenderPipelineErrorTooManyColorTargets indicates too many color targets.
	CreateRenderPipelineErrorTooManyColorTargets
	// CreateRenderPipelineErrorInvalidSampleCount indicates an invalid multisample count.
	CreateRenderPipelineErrorInvalidSampleCount
	// CreateRenderPipelineErrorColorTargetDepthFormat indicates a depth/stencil format was used as a color target.
	// WebGPU spec: color target format must have a color aspect.
	// Rust: pipeline::ColorStateError::FormatNotColor
	CreateRenderPipelineErrorColorTargetDepthFormat
	// CreateRenderPipelineErrorDepthStencilColorFormat indicates a color format was used as a depth/stencil format.
	// WebGPU spec: depth/stencil format must be a depth/stencil format.
	// Rust: pipeline::DepthStencilStateError::FormatNotDepth / FormatNotStencil
	CreateRenderPipelineErrorDepthStencilColorFormat
	// CreateRenderPipelineErrorDepthFormatNoDepthAspect indicates depth operations are enabled
	// but the format has no depth aspect (e.g. Stencil8 with depth write enabled).
	// Rust: pipeline::DepthStencilStateError::FormatNotDepth
	CreateRenderPipelineErrorDepthFormatNoDepthAspect
	// CreateRenderPipelineErrorDepthFormatNoStencilAspect indicates stencil operations are enabled
	// but the format has no stencil aspect (e.g. Depth16Unorm with stencil ops != Keep).
	// Rust: pipeline::DepthStencilStateError::FormatNotStencil
	CreateRenderPipelineErrorDepthFormatNoStencilAspect
	// CreateRenderPipelineErrorHAL indicates the HAL backend failed to create the pipeline.
	CreateRenderPipelineErrorHAL
	// CreateRenderPipelineErrorVertexStrideMisaligned indicates a vertex buffer
	// arrayStride that is not a multiple of 4.
	// WebGPU spec: GPUVertexBufferLayout arrayStride must be a multiple of 4.
	// Rust: pipeline::CreateRenderPipelineError::UnalignedVertexStride
	CreateRenderPipelineErrorVertexStrideMisaligned
	// CreateRenderPipelineErrorVertexStrideTooLarge indicates a vertex buffer
	// arrayStride above Limits.MaxVertexBufferArrayStride.
	// Rust: pipeline::CreateRenderPipelineError::VertexStrideTooLarge
	CreateRenderPipelineErrorVertexStrideTooLarge
	// CreateRenderPipelineErrorVertexAttributeOutOfStride indicates a vertex
	// attribute whose offset plus format size runs past its buffer's arrayStride
	// (or past Limits.MaxVertexBufferArrayStride when arrayStride is 0).
	CreateRenderPipelineErrorVertexAttributeOutOfStride
	// CreateRenderPipelineErrorVertexStrideZero indicated a vertex buffer with
	// arrayStride 0, which v0.34.6 rejected.
	//
	// Deprecated: arrayStride 0 (broadcast) is supported on every backend and
	// is no longer rejected; this kind is never returned.
	CreateRenderPipelineErrorVertexStrideZero
	// CreateRenderPipelineErrorTooManyVertexBuffers indicates more vertex
	// buffers than Limits.MaxVertexBuffers (Given, Limit).
	// Rust: pipeline::CreateRenderPipelineError::TooManyVertexBuffers
	CreateRenderPipelineErrorTooManyVertexBuffers
	// CreateRenderPipelineErrorVertexAttributeInvalidOffset indicates a vertex
	// attribute offset that is not a multiple of min(4, format size), or not
	// below 2^28.
	// Rust: pipeline::CreateRenderPipelineError::InvalidVertexAttributeOffset
	CreateRenderPipelineErrorVertexAttributeInvalidOffset
	// CreateRenderPipelineErrorVertexAttributeLocationTooLarge indicates a
	// vertex attribute shaderLocation at or above Limits.MaxVertexAttributes.
	// Rust: pipeline::CreateRenderPipelineError::VertexAttributeLocationTooLarge
	CreateRenderPipelineErrorVertexAttributeLocationTooLarge
	// CreateRenderPipelineErrorShaderLocationClash indicates two vertex
	// attributes of the pipeline at one shaderLocation.
	// Rust: pipeline::CreateRenderPipelineError::ShaderLocationClash
	CreateRenderPipelineErrorShaderLocationClash
	// CreateRenderPipelineErrorMissingDepthCompare indicates a depth/stencil
	// state without a depthCompare (CompareFunctionUndefined) where one is
	// required: the format has a depth aspect and depth writes are enabled or
	// a stencil face's DepthFailOp is not Keep.
	// WebGPU spec: validating GPUDepthStencilState.
	// Rust: pipeline::DepthStencilStateError::MissingDepthCompare
	CreateRenderPipelineErrorMissingDepthCompare
)

// CreateRenderPipelineError represents an error during render pipeline creation.
type CreateRenderPipelineError struct {
	Kind        CreateRenderPipelineErrorKind
	Label       string
	TargetCount uint32
	MaxTargets  uint32
	SampleCount uint32
	// TargetIndex is the color target index for format errors.
	TargetIndex uint32
	// Format is the texture format that caused the error.
	Format string
	// BufferIndex is the vertex buffer index for vertex layout errors.
	BufferIndex uint32
	// ArrayStride is the offending vertex buffer arrayStride for vertex layout errors.
	ArrayStride uint64
	// MaxArrayStride is Limits.MaxVertexBufferArrayStride for vertex layout errors.
	MaxArrayStride uint32
	// AttributeIndex is the attribute index within the vertex buffer for
	// CreateRenderPipelineErrorVertexAttributeOutOfStride.
	AttributeIndex uint32
	// AttributeOffset is the attribute byte offset for
	// CreateRenderPipelineErrorVertexAttributeOutOfStride.
	AttributeOffset uint64
	// AttributeFormat is the attribute vertex format for
	// CreateRenderPipelineErrorVertexAttributeOutOfStride.
	AttributeFormat string
	// ShaderLocation is the attribute's shaderLocation for vertex attribute
	// offset, location and clash errors.
	ShaderLocation uint32
	// Given and Limit are the count and the device limit for
	// CreateRenderPipelineErrorTooManyVertexBuffers and the limit for
	// CreateRenderPipelineErrorVertexAttributeLocationTooLarge.
	Given    int
	Limit    uint32
	HALError error
}

// Error implements the error interface.
func (e *CreateRenderPipelineError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateRenderPipelineErrorMissingVertexModule:
		return fmt.Sprintf("render pipeline %q: vertex shader module must not be nil", label)
	case CreateRenderPipelineErrorMissingVertexEntryPoint:
		return fmt.Sprintf("render pipeline %q: vertex entry point must not be empty", label)
	case CreateRenderPipelineErrorMissingFragmentModule:
		return fmt.Sprintf("render pipeline %q: fragment shader module must not be nil", label)
	case CreateRenderPipelineErrorMissingFragmentEntryPoint:
		return fmt.Sprintf("render pipeline %q: fragment entry point must not be empty", label)
	case CreateRenderPipelineErrorNoFragmentTargets:
		return fmt.Sprintf("render pipeline %q: needs at least one attachment: a color target or a depth-stencil state", label)
	case CreateRenderPipelineErrorTooManyColorTargets:
		return fmt.Sprintf("render pipeline %q: color target count %d exceeds maximum %d",
			label, e.TargetCount, e.MaxTargets)
	case CreateRenderPipelineErrorInvalidSampleCount:
		return fmt.Sprintf("render pipeline %q: invalid sample count %d (must be 1 or 4)",
			label, e.SampleCount)
	case CreateRenderPipelineErrorColorTargetDepthFormat:
		return fmt.Sprintf("render pipeline %q: color target [%d] format %s does not have a color aspect",
			label, e.TargetIndex, e.Format)
	case CreateRenderPipelineErrorDepthStencilColorFormat:
		return fmt.Sprintf("render pipeline %q: depth/stencil format %s is not a depth/stencil format",
			label, e.Format)
	case CreateRenderPipelineErrorDepthFormatNoDepthAspect:
		return fmt.Sprintf("render pipeline %q: depth/stencil format %s does not have a depth aspect but depth operations are enabled",
			label, e.Format)
	case CreateRenderPipelineErrorDepthFormatNoStencilAspect:
		return fmt.Sprintf("render pipeline %q: depth/stencil format %s does not have a stencil aspect but stencil operations are enabled",
			label, e.Format)
	case CreateRenderPipelineErrorHAL:
		return fmt.Sprintf("render pipeline %q: HAL error: %v", label, e.HALError)
	case CreateRenderPipelineErrorVertexStrideMisaligned:
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] arrayStride %d is not a multiple of 4",
			label, e.BufferIndex, e.ArrayStride)
	case CreateRenderPipelineErrorVertexStrideTooLarge:
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] arrayStride %d exceeds maxVertexBufferArrayStride %d",
			label, e.BufferIndex, e.ArrayStride, e.MaxArrayStride)
	case CreateRenderPipelineErrorVertexAttributeOutOfStride:
		if e.ArrayStride == 0 {
			return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute [%d] (%s at offset %d) exceeds maxVertexBufferArrayStride %d",
				label, e.BufferIndex, e.AttributeIndex, e.AttributeFormat, e.AttributeOffset, e.MaxArrayStride)
		}
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute [%d] (%s at offset %d) does not fit in arrayStride %d",
			label, e.BufferIndex, e.AttributeIndex, e.AttributeFormat, e.AttributeOffset, e.ArrayStride)
	case CreateRenderPipelineErrorVertexStrideZero:
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] arrayStride 0 (broadcast) is not supported on native backends yet",
			label, e.BufferIndex)
	default:
		return e.vertexAttributeMessage(label)
	}
}

// vertexAttributeMessage is Error for the vertex buffer count and vertex
// attribute kinds.
func (e *CreateRenderPipelineError) vertexAttributeMessage(label string) string {
	switch e.Kind {
	case CreateRenderPipelineErrorTooManyVertexBuffers:
		return fmt.Sprintf("render pipeline %q: %d vertex buffers exceed maxVertexBuffers %d", label, e.Given, e.Limit)
	case CreateRenderPipelineErrorVertexAttributeInvalidOffset:
		if e.AttributeOffset >= maxVertexAttributeOffset {
			return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute at shaderLocation %d: offset %d is not below 2^28",
				label, e.BufferIndex, e.ShaderLocation, e.AttributeOffset)
		}
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute at shaderLocation %d: offset %d is not a multiple of min(4, size of %s)",
			label, e.BufferIndex, e.ShaderLocation, e.AttributeOffset, e.AttributeFormat)
	case CreateRenderPipelineErrorVertexAttributeLocationTooLarge:
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute shaderLocation %d is not below maxVertexAttributes %d",
			label, e.BufferIndex, e.ShaderLocation, e.Limit)
	case CreateRenderPipelineErrorShaderLocationClash:
		return fmt.Sprintf("render pipeline %q: vertex buffer [%d] attribute shaderLocation %d is used by more than one attribute",
			label, e.BufferIndex, e.ShaderLocation)
	case CreateRenderPipelineErrorMissingDepthCompare:
		return fmt.Sprintf("render pipeline %q: depth/stencil format %s needs a depthCompare when depth writes are enabled or a depthFailOp is not keep",
			label, e.Format)
	default:
		return fmt.Sprintf("render pipeline %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateRenderPipelineError) Unwrap() error {
	return e.HALError
}

// IsCreateRenderPipelineError returns true if the error is a CreateRenderPipelineError.
func IsCreateRenderPipelineError(err error) bool {
	var crpe *CreateRenderPipelineError
	return errors.As(err, &crpe)
}

// =============================================================================
// Compute Pipeline Creation Errors
// =============================================================================

// CreateComputePipelineErrorKind represents the type of compute pipeline creation error.
type CreateComputePipelineErrorKind int

const (
	// CreateComputePipelineErrorMissingModule indicates the compute shader module was nil.
	CreateComputePipelineErrorMissingModule CreateComputePipelineErrorKind = iota
	// CreateComputePipelineErrorMissingEntryPoint indicates the compute entry point was empty.
	CreateComputePipelineErrorMissingEntryPoint
	// CreateComputePipelineErrorHAL indicates the HAL backend failed to create the pipeline.
	CreateComputePipelineErrorHAL
	// CreateComputePipelineErrorWorkgroupSizeExceeded indicates a workgroup dimension exceeds device limits.
	CreateComputePipelineErrorWorkgroupSizeExceeded
	// CreateComputePipelineErrorWorkgroupSizeZero indicates a workgroup dimension is zero.
	CreateComputePipelineErrorWorkgroupSizeZero
	// CreateComputePipelineErrorTooManyInvocations indicates total invocations exceed device limit.
	CreateComputePipelineErrorTooManyInvocations
	// CreateComputePipelineErrorWorkgroupStorageExceeded indicates the
	// workgroup storage the entry point uses exceeds
	// maxComputeWorkgroupStorageSize (WebGPU createComputePipeline).
	CreateComputePipelineErrorWorkgroupStorageExceeded
	// CreateComputePipelineErrorPipelineConstants indicates the pipeline
	// constants do not apply to the module's overrides. Cause holds naga's
	// error.
	CreateComputePipelineErrorPipelineConstants
)

// CreateComputePipelineError represents an error during compute pipeline creation.
type CreateComputePipelineError struct {
	Kind     CreateComputePipelineErrorKind
	Label    string
	HALError error
	// Dimension is the axis name ("X", "Y", "Z") for workgroup size errors.
	Dimension string
	// Size is the workgroup size value that caused the error.
	Size uint32
	// Limit is the device limit that was exceeded.
	Limit uint32
	// TotalInvocations is the product x*y*z for TooManyInvocations errors.
	TotalInvocations uint64
	// WorkgroupStorage is the workgroup storage in bytes the entry point
	// uses, for WorkgroupStorageExceeded errors.
	WorkgroupStorage uint64
	// Cause is naga's error for PipelineConstants errors.
	Cause error
}

// Error implements the error interface.
func (e *CreateComputePipelineError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateComputePipelineErrorMissingModule:
		return fmt.Sprintf("compute pipeline %q: compute shader module must not be nil", label)
	case CreateComputePipelineErrorMissingEntryPoint:
		return fmt.Sprintf("compute pipeline %q: compute entry point must not be empty", label)
	case CreateComputePipelineErrorHAL:
		return fmt.Sprintf("compute pipeline %q: HAL error: %v", label, e.HALError)
	case CreateComputePipelineErrorWorkgroupSizeExceeded:
		return fmt.Sprintf("compute pipeline %q: workgroup_size %s = %d exceeds device limit %d",
			label, e.Dimension, e.Size, e.Limit)
	case CreateComputePipelineErrorWorkgroupSizeZero:
		return fmt.Sprintf("compute pipeline %q: workgroup_size %s must not be zero", label, e.Dimension)
	case CreateComputePipelineErrorTooManyInvocations:
		return fmt.Sprintf("compute pipeline %q: total workgroup invocations %d exceeds device limit %d",
			label, e.TotalInvocations, e.Limit)
	case CreateComputePipelineErrorWorkgroupStorageExceeded:
		return fmt.Sprintf("compute pipeline %q: workgroup storage %d bytes exceeds maxComputeWorkgroupStorageSize %d",
			label, e.WorkgroupStorage, e.Limit)
	case CreateComputePipelineErrorPipelineConstants:
		return fmt.Sprintf("compute pipeline %q: %v", label, e.Cause)
	default:
		return fmt.Sprintf("compute pipeline %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL or naga error, if any.
func (e *CreateComputePipelineError) Unwrap() error {
	if e.HALError != nil {
		return e.HALError
	}
	return e.Cause
}

// IsCreateComputePipelineError returns true if the error is a CreateComputePipelineError.
func IsCreateComputePipelineError(err error) bool {
	var ccpe *CreateComputePipelineError
	return errors.As(err, &ccpe)
}

// =============================================================================
// Bind Group Layout Creation Errors
// =============================================================================

// CreateBindGroupLayoutErrorKind represents the type of bind group layout creation error.
type CreateBindGroupLayoutErrorKind int

const (
	// CreateBindGroupLayoutErrorDuplicateBinding indicates duplicate binding numbers.
	CreateBindGroupLayoutErrorDuplicateBinding CreateBindGroupLayoutErrorKind = iota
	// CreateBindGroupLayoutErrorTooManyBindings indicates too many bindings.
	CreateBindGroupLayoutErrorTooManyBindings
	// CreateBindGroupLayoutErrorHAL indicates the HAL backend failed.
	CreateBindGroupLayoutErrorHAL
)

// CreateBindGroupLayoutError represents an error during bind group layout creation.
type CreateBindGroupLayoutError struct {
	Kind             CreateBindGroupLayoutErrorKind
	Label            string
	DuplicateBinding uint32
	BindingCount     uint32
	MaxBindings      uint32
	HALError         error
}

// Error implements the error interface.
func (e *CreateBindGroupLayoutError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateBindGroupLayoutErrorDuplicateBinding:
		return fmt.Sprintf("bind group layout %q: duplicate binding number %d",
			label, e.DuplicateBinding)
	case CreateBindGroupLayoutErrorTooManyBindings:
		return fmt.Sprintf("bind group layout %q: binding count %d exceeds maximum %d",
			label, e.BindingCount, e.MaxBindings)
	case CreateBindGroupLayoutErrorHAL:
		return fmt.Sprintf("bind group layout %q: HAL error: %v", label, e.HALError)
	default:
		return fmt.Sprintf("bind group layout %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateBindGroupLayoutError) Unwrap() error {
	return e.HALError
}

// IsCreateBindGroupLayoutError returns true if the error is a CreateBindGroupLayoutError.
func IsCreateBindGroupLayoutError(err error) bool {
	var cble *CreateBindGroupLayoutError
	return errors.As(err, &cble)
}

// =============================================================================
// Bind Group Creation Errors
// =============================================================================

// CreateBindGroupErrorKind represents the type of bind group creation error.
type CreateBindGroupErrorKind int

const (
	// CreateBindGroupErrorMissingLayout indicates the layout was nil.
	CreateBindGroupErrorMissingLayout CreateBindGroupErrorKind = iota
	// CreateBindGroupErrorBindingsNumMismatch indicates the number of entries
	// in the bind group descriptor does not match the number of entries
	// defined in the bind group layout.
	// Rust: wgpu-core binding_model.rs CreateBindGroupError::BindingsNumMismatch
	CreateBindGroupErrorBindingsNumMismatch
	// CreateBindGroupErrorMissingBindingDeclaration indicates a bind group entry
	// references a binding number that is not declared in the layout.
	// Rust: wgpu-core binding_model.rs CreateBindGroupError::MissingBindingDeclaration
	CreateBindGroupErrorMissingBindingDeclaration
	// CreateBindGroupErrorDuplicateBinding indicates two or more bind group entries
	// share the same binding number.
	// Rust: wgpu-core binding_model.rs CreateBindGroupError::DuplicateBinding
	CreateBindGroupErrorDuplicateBinding
	// CreateBindGroupErrorBufferUsageMismatch indicates the buffer does not have the
	// usage flag required by the layout entry's buffer binding type.
	// E.g., binding a VERTEX buffer where UNIFORM is required.
	// Rust: wgpu-core resource.rs MissingBufferUsageError
	CreateBindGroupErrorBufferUsageMismatch
	// CreateBindGroupErrorBufferOffsetAlignment indicates the buffer offset is not
	// aligned to the required alignment for its binding type.
	// Uniform: minUniformBufferOffsetAlignment, Storage: minStorageBufferOffsetAlignment.
	// Rust: wgpu-core device/resource.rs UnalignedBufferOffset
	CreateBindGroupErrorBufferOffsetAlignment
	// CreateBindGroupErrorBufferBindingSizeTooLarge indicates the effective binding
	// size exceeds the maximum allowed for its binding type.
	// Uniform: maxUniformBufferBindingSize, Storage: maxStorageBufferBindingSize.
	// Rust: wgpu-core device/resource.rs BufferRangeTooLarge
	CreateBindGroupErrorBufferBindingSizeTooLarge
	// CreateBindGroupErrorBufferBoundsOverflow indicates offset + bindingSize exceeds
	// the buffer's total size.
	// Rust: wgpu-core resource.rs BindingRangeTooLarge / BindingOffsetTooLarge
	CreateBindGroupErrorBufferBoundsOverflow
	// CreateBindGroupErrorBufferBindingZeroSize indicates the effective binding size
	// resolved to zero. WebGPU requires a non-zero binding size.
	// Rust: wgpu-core device/resource.rs BindingZeroSize
	CreateBindGroupErrorBufferBindingZeroSize
	// CreateBindGroupErrorStorageBufferSizeAlignment indicates a storage buffer binding
	// size is not a multiple of 4 bytes, as required by the WebGPU spec.
	// Rust: wgpu-core device/resource.rs UnalignedEffectiveBufferBindingSizeForStorage
	CreateBindGroupErrorStorageBufferSizeAlignment
	// CreateBindGroupErrorMinBindingSizeMismatch indicates the effective binding size
	// is smaller than the MinBindingSize declared in the bind group layout entry.
	// Rust: wgpu-core device/resource.rs:2817-2826 BindingSizeTooSmall
	CreateBindGroupErrorMinBindingSizeMismatch
	// CreateBindGroupErrorHAL indicates the HAL backend failed.
	CreateBindGroupErrorHAL
	// CreateBindGroupErrorTextureViewNotSampleable indicates the backend
	// cannot sample a bound texture view (hal.ErrTextureViewNotSampleable,
	// in HALError with the binding): on GLES, a view whose dimension needs
	// another GL target than the texture was created with, or a view of a
	// subset of its layers. WebGPU compatibility mode has the same rules
	// (textureBindingViewDimension; whole-array views only).
	CreateBindGroupErrorTextureViewNotSampleable
)

// CreateBindGroupError represents an error during bind group creation.
type CreateBindGroupError struct {
	Kind           CreateBindGroupErrorKind
	Label          string
	Expected       int    // expected entry count (for BindingsNumMismatch)
	Actual         int    // actual entry count (for BindingsNumMismatch)
	Binding        uint32 // binding number (for MissingBindingDeclaration, DuplicateBinding, buffer errors)
	ExpectedUsage  uint64 // required buffer usage flag (for BufferUsageMismatch)
	ActualUsage    uint64 // actual buffer usage flags (for BufferUsageMismatch)
	Offset         uint64 // buffer offset (for alignment/bounds errors)
	Size           uint64 // effective binding size (for size-related errors)
	BufferSize     uint64 // total buffer size (for bounds overflow)
	MaxSize        uint64 // maximum allowed binding size (for BindingSizeTooLarge)
	Alignment      uint64 // required alignment (for alignment errors)
	MinBindingSize uint64 // layout-declared minimum binding size (for MinBindingSizeMismatch)
	HALError       error
}

// Error implements the error interface.
func (e *CreateBindGroupError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreateBindGroupErrorMissingLayout:
		return fmt.Sprintf("bind group %q: layout must not be nil", label)
	case CreateBindGroupErrorBindingsNumMismatch:
		return fmt.Sprintf("bind group %q: number of bindings in descriptor (%d) does not match the number defined in the layout (%d)",
			label, e.Actual, e.Expected)
	case CreateBindGroupErrorMissingBindingDeclaration:
		return fmt.Sprintf("bind group %q: unable to find a corresponding declaration for binding %d",
			label, e.Binding)
	case CreateBindGroupErrorDuplicateBinding:
		return fmt.Sprintf("bind group %q: binding %d is used at least twice in the descriptor",
			label, e.Binding)
	case CreateBindGroupErrorBufferUsageMismatch:
		return fmt.Sprintf("bind group %q: binding %d buffer usage mismatch: expected 0x%x, actual 0x%x",
			label, e.Binding, e.ExpectedUsage, e.ActualUsage)
	case CreateBindGroupErrorBufferOffsetAlignment:
		return fmt.Sprintf("bind group %q: binding %d buffer offset %d is not aligned to %d",
			label, e.Binding, e.Offset, e.Alignment)
	case CreateBindGroupErrorBufferBindingSizeTooLarge:
		return fmt.Sprintf("bind group %q: binding %d buffer binding size %d exceeds maximum %d",
			label, e.Binding, e.Size, e.MaxSize)
	case CreateBindGroupErrorBufferBoundsOverflow:
		return fmt.Sprintf("bind group %q: binding %d buffer range overflows: offset %d + size %d > buffer size %d",
			label, e.Binding, e.Offset, e.Size, e.BufferSize)
	case CreateBindGroupErrorBufferBindingZeroSize:
		return fmt.Sprintf("bind group %q: binding %d has zero effective binding size",
			label, e.Binding)
	case CreateBindGroupErrorStorageBufferSizeAlignment:
		return fmt.Sprintf("bind group %q: binding %d storage buffer binding size %d is not a multiple of 4",
			label, e.Binding, e.Size)
	case CreateBindGroupErrorMinBindingSizeMismatch:
		return fmt.Sprintf("bind group %q: binding %d effective binding size %d is less than minimum %d declared in layout",
			label, e.Binding, e.Size, e.MinBindingSize)
	case CreateBindGroupErrorHAL:
		return fmt.Sprintf("bind group %q: HAL error: %v", label, e.HALError)
	case CreateBindGroupErrorTextureViewNotSampleable:
		return fmt.Sprintf("bind group %q: texture view cannot be sampled on this backend: %v", label, e.HALError)
	default:
		return fmt.Sprintf("bind group %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateBindGroupError) Unwrap() error {
	return e.HALError
}

// IsCreateBindGroupError returns true if the error is a CreateBindGroupError.
func IsCreateBindGroupError(err error) bool {
	var cbge *CreateBindGroupError
	return errors.As(err, &cbge)
}

// =============================================================================
// Pipeline Layout Creation Errors
// =============================================================================

// CreatePipelineLayoutErrorKind represents the type of pipeline layout creation error.
type CreatePipelineLayoutErrorKind int

const (
	// CreatePipelineLayoutErrorTooManyGroups indicates the number of bind group layouts
	// exceeds the device limit (maxBindGroups, typically 4).
	// Rust: binding_model::CreatePipelineLayoutError::TooManyGroups
	CreatePipelineLayoutErrorTooManyGroups CreatePipelineLayoutErrorKind = iota
	// CreatePipelineLayoutErrorHAL indicates the HAL backend failed to create the pipeline layout.
	CreatePipelineLayoutErrorHAL
)

// CreatePipelineLayoutError represents an error during pipeline layout creation.
type CreatePipelineLayoutError struct {
	Kind      CreatePipelineLayoutErrorKind
	Label     string
	Count     int
	MaxGroups uint32
	HALError  error
}

// Error implements the error interface.
func (e *CreatePipelineLayoutError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case CreatePipelineLayoutErrorTooManyGroups:
		return fmt.Sprintf("pipeline layout %q: bind group layout count %d exceeds device limit %d",
			label, e.Count, e.MaxGroups)
	case CreatePipelineLayoutErrorHAL:
		return fmt.Sprintf("pipeline layout %q: HAL error: %v", label, e.HALError)
	default:
		return fmt.Sprintf("pipeline layout %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreatePipelineLayoutError) Unwrap() error {
	return e.HALError
}

// IsCreatePipelineLayoutError returns true if the error is a CreatePipelineLayoutError.
func IsCreatePipelineLayoutError(err error) bool {
	var cple *CreatePipelineLayoutError
	return errors.As(err, &cple)
}

// EncoderStateError represents an invalid state transition error.
// When the encoder is in the Error state (due to a deferred validation error),
// the Cause field carries the original error so that errors.Is/errors.As
// work through the chain.
type EncoderStateError struct {
	Operation string
	Status    CommandEncoderStatus
	// Cause is the deferred error that put the encoder into the Error state.
	// Non-nil only when Status == CommandEncoderStatusError.
	Cause error
}

// Error implements the error interface.
func (e *EncoderStateError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("cannot %s: encoder in %v state: %v", e.Operation, e.Status, e.Cause)
	}
	return fmt.Sprintf("cannot %s: encoder in %v state", e.Operation, e.Status)
}

// Unwrap returns the deferred validation error that caused the encoder
// to enter the Error state. Returns nil for non-error state transitions.
func (e *EncoderStateError) Unwrap() error {
	return e.Cause
}

// IsEncoderStateError returns true if the error is an EncoderStateError.
func IsEncoderStateError(err error) bool {
	var ese *EncoderStateError
	return errors.As(err, &ese)
}

// =============================================================================
// Render Pass Validation Errors (MRT — Multiple Render Targets)
// =============================================================================

// RenderPassValidationErrorKind represents the type of render pass validation error.
type RenderPassValidationErrorKind int

const (
	// RenderPassErrorTooManyColorAttachments indicates the number of color
	// attachments exceeds device.limits.maxColorAttachments.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#dom-gpudevice-createcommandencoder
	RenderPassErrorTooManyColorAttachments RenderPassValidationErrorKind = iota

	// RenderPassErrorSampleCountMismatch indicates color attachments have
	// different sample counts. All attachments must have the same sampleCount.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#render-pass-encoder-creation
	RenderPassErrorSampleCountMismatch

	// RenderPassErrorDimensionMismatch indicates color attachments have
	// different width/height. All attachment textures must match dimensions.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#render-pass-encoder-creation
	RenderPassErrorDimensionMismatch

	// RenderPassErrorMissingDepthSlice indicates a color attachment whose view
	// is 3D has no depthSlice.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: ColorAttachmentError::MissingDepthSlice.
	RenderPassErrorMissingDepthSlice

	// RenderPassErrorDepthSliceLimit indicates a color attachment's depthSlice
	// is not less than the depth of its 3D view's base mip level.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: ColorAttachmentError::DepthSliceLimit.
	RenderPassErrorDepthSliceLimit

	// RenderPassErrorUnneededDepthSlice indicates a color attachment has a
	// depthSlice although its view is not 3D.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: ColorAttachmentError::UnneededDepthSlice.
	RenderPassErrorUnneededDepthSlice

	// RenderPassErrorSubresourceOverlap indicates two attachment regions of
	// the pass (a color attachment's view with its depthSlice, or a resolve
	// target) name the same subresource: the same texture, mip level and
	// array layer or depth slice.
	// WebGPU spec: GPURenderPassDescriptor Valid Usage (attachmentRegions
	// must be pairwise disjoint).
	// Rust wgpu-core: ColorAttachmentError::SubresourceOverlap.
	RenderPassErrorSubresourceOverlap

	// RenderPassErrorTextureViewNotRenderable indicates an attachment's view
	// is not a renderable texture view; Reason says which requirement fails.
	// WebGPU spec: "renderable texture view".
	// Rust wgpu-core: RenderPassErrorInner::TextureViewIsNotRenderable.
	RenderPassErrorTextureViewNotRenderable

	// RenderPassErrorInvalidColorAttachmentFormat indicates a color
	// attachment's view format is not a color renderable format.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: ColorAttachmentError::InvalidFormat.
	RenderPassErrorInvalidColorAttachmentFormat

	// RenderPassErrorInvalidDepthStencilAttachmentFormat indicates the
	// depth/stencil attachment's view format is not a depth-or-stencil format.
	// WebGPU spec: GPURenderPassDepthStencilAttachment Valid Usage.
	// Rust wgpu-core: AttachmentError::InvalidDepthStencilAttachmentFormat.
	RenderPassErrorInvalidDepthStencilAttachmentFormat

	// RenderPassErrorInvalidResolveSampleCounts indicates a resolve whose
	// source is not multisampled or whose target is.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: RenderPassErrorInner::InvalidResolveSampleCounts.
	RenderPassErrorInvalidResolveSampleCounts

	// RenderPassErrorMismatchedResolveFormat indicates a resolve target whose
	// view format (or, with TextureFormats, texture format) differs from its
	// attachment's.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: RenderPassErrorInner::MismatchedResolveTextureFormat.
	RenderPassErrorMismatchedResolveFormat

	// RenderPassErrorUnsupportedResolveFormat indicates a resolve target whose
	// format does not support resolve.
	// WebGPU spec: GPURenderPassColorAttachment Valid Usage.
	// Rust wgpu-core: RenderPassErrorInner::UnsupportedResolveTargetFormat.
	RenderPassErrorUnsupportedResolveFormat
)

// RenderPassValidationError represents a validation error during render pass creation.
type RenderPassValidationError struct {
	Kind  RenderPassValidationErrorKind
	Label string

	// For TooManyColorAttachments and DepthSliceLimit (Given is the depth
	// slice, Limit the depth of the view's base mip level):
	Given uint32
	Limit uint32

	// For SampleCountMismatch:
	AttachmentIndex int
	ExpectedSamples uint32
	ActualSamples   uint32
	ReferenceIndex  int // index of the attachment that set the expected value

	// For DimensionMismatch:
	ExpectedWidth  uint32
	ExpectedHeight uint32
	ActualWidth    uint32
	ActualHeight   uint32

	// For SubresourceOverlap: the subresource attached twice, and whether
	// the second use is attachment AttachmentIndex's resolve target.
	MipLevel      uint32
	Layer         uint32
	ResolveTarget bool

	// For TextureViewNotRenderable: the failed requirement and the view's
	// value for it (Given holds a mip or layer count).
	Reason        TextureViewNotRenderableReason
	Usage         gputypes.TextureUsage
	ViewDimension gputypes.TextureViewDimension
	Aspect        gputypes.TextureAspect

	// For the format kinds: the attachment's format, and for
	// MismatchedResolveFormat the resolve target's, comparing texture formats
	// when TextureFormats is set and view formats otherwise.
	Format         gputypes.TextureFormat
	ResolveFormat  gputypes.TextureFormat
	TextureFormats bool

	// For InvalidResolveSampleCounts: ActualSamples is the resolve source's
	// sample count, ResolveSamples the resolve target's.
	ResolveSamples uint32
}

// attachmentLocation names the attachment of e the way Rust wgpu-core's
// AttachmentErrorLocation does: "color attachment at index N's texture
// view", "... resolve texture view", or "depth attachment's texture view"
// for AttachmentIndex -1.
func (e *RenderPassValidationError) attachmentLocation() string {
	switch {
	case e.AttachmentIndex < 0:
		return "depth attachment's texture view"
	case e.ResolveTarget:
		return fmt.Sprintf("color attachment at index %d's resolve texture view", e.AttachmentIndex)
	default:
		return fmt.Sprintf("color attachment at index %d's texture view", e.AttachmentIndex)
	}
}

// referenceLocation names the attachment that set the expected sample count
// or render extent: always a color attachment's view, ReferenceIndex.
func (e *RenderPassValidationError) referenceLocation() string {
	return fmt.Sprintf("color attachment at index %d's texture view", e.ReferenceIndex)
}

// notRenderableReason words e.Reason as Rust wgpu-core's
// TextureViewNotRenderableReason does.
func (e *RenderPassValidationError) notRenderableReason() string {
	switch e.Reason {
	case TextureViewNotRenderableUsage:
		return fmt.Sprintf("the texture this view references doesn't include the RENDER_ATTACHMENT usage; provided usages: %v", e.Usage)
	case TextureViewNotRenderableDimension:
		if e.ResolveTarget {
			return fmt.Sprintf("the dimension of this resolve target view is not 2D or 2D array; view dimension: %v", e.ViewDimension)
		}
		return fmt.Sprintf("the dimension of this texture view is not 2D, 2D array or 3D; view dimension: %v", e.ViewDimension)
	case TextureViewNotRenderableMipLevelCount:
		return fmt.Sprintf("this texture view has more than one mipmap level; view mipmap levels: %d", e.Given)
	case TextureViewNotRenderableArrayLayerCount:
		return fmt.Sprintf("this texture view has more than one array layer; view array layers: %d", e.Given)
	case TextureViewNotRenderableAspects:
		return fmt.Sprintf("the aspects of this texture view are a subset of the aspects in the original texture; aspect: %v", e.Aspect)
	default:
		return "unknown reason"
	}
}

// formatError returns the message of the format and resolve kinds of e.
func (e *RenderPassValidationError) formatError(label string) string {
	const spec = "(WebGPU spec: GPURenderPassColorAttachment Valid Usage)"
	switch e.Kind {
	case RenderPassErrorInvalidColorAttachmentFormat:
		return fmt.Sprintf("render pass %q: the format of the %s (%v) is not a color renderable format %s",
			label, e.attachmentLocation(), e.Format, spec)
	case RenderPassErrorInvalidDepthStencilAttachmentFormat:
		return fmt.Sprintf("render pass %q: the format of the depth-stencil attachment (%v) is not a depth-or-stencil format (WebGPU spec: GPURenderPassDepthStencilAttachment Valid Usage)",
			label, e.Format)
	case RenderPassErrorInvalidResolveSampleCounts:
		return fmt.Sprintf("render pass %q: the resolve source, %s, must be multi-sampled (has %d samples) while the resolve destination must not be multisampled (has %d samples) %s",
			label, e.attachmentLocation(), e.ActualSamples, e.ResolveSamples, spec)
	case RenderPassErrorMismatchedResolveFormat:
		what := "view format"
		if e.TextureFormats {
			what = "texture format"
		}
		return fmt.Sprintf("render pass %q: resolve source, %s, %s (%v) must match the resolve destination %s (%v) %s",
			label, e.attachmentLocation(), what, e.Format, what, e.ResolveFormat, spec)
	case RenderPassErrorUnsupportedResolveFormat:
		return fmt.Sprintf("render pass %q: the format of the %s (%v) is not resolvable %s",
			label, e.attachmentLocation(), e.Format, spec)
	default:
		return fmt.Sprintf("render pass %q: unknown validation error", label)
	}
}

// Error implements the error interface.
func (e *RenderPassValidationError) Error() string {
	label := e.Label
	if label == "" {
		label = unnamedLabel
	}

	switch e.Kind {
	case RenderPassErrorTooManyColorAttachments:
		return fmt.Sprintf(
			"render pass %q: color attachment count %d exceeds device limit maxColorAttachments=%d (WebGPU spec §10.1)",
			label, e.Given, e.Limit)
	case RenderPassErrorSampleCountMismatch:
		return fmt.Sprintf(
			"render pass %q: attachments have differing sample counts: the %s has count %d but is followed by the %s which has count %d (WebGPU spec: GPURenderPassDescriptor Valid Usage)",
			label, e.referenceLocation(), e.ExpectedSamples, e.attachmentLocation(), e.ActualSamples)
	case RenderPassErrorDimensionMismatch:
		return fmt.Sprintf(
			"render pass %q: attachments have differing sizes: the %s has extent %dx%d but is followed by the %s which has %dx%d (WebGPU spec: GPURenderPassDescriptor Valid Usage)",
			label, e.referenceLocation(), e.ExpectedWidth, e.ExpectedHeight, e.attachmentLocation(), e.ActualWidth, e.ActualHeight)
	case RenderPassErrorMissingDepthSlice:
		return fmt.Sprintf(
			"render pass %q: color attachment [%d]: color attachment's view is 3D and requires depth slice to be provided (WebGPU spec: GPURenderPassColorAttachment Valid Usage)",
			label, e.AttachmentIndex)
	case RenderPassErrorDepthSliceLimit:
		return fmt.Sprintf(
			"render pass %q: color attachment [%d]: depth slice must be less than %d but is %d (WebGPU spec: GPURenderPassColorAttachment Valid Usage)",
			label, e.AttachmentIndex, e.Limit, e.Given)
	case RenderPassErrorUnneededDepthSlice:
		return fmt.Sprintf(
			"render pass %q: color attachment [%d]: depth slice was provided but the color attachment's view is not 3D (WebGPU spec: GPURenderPassColorAttachment Valid Usage)",
			label, e.AttachmentIndex)
	case RenderPassErrorSubresourceOverlap:
		what := "color attachment's view"
		if e.ResolveTarget {
			what = "resolve target"
		}
		return fmt.Sprintf(
			"render pass %q: color attachment [%d]: %s's subresource at mip %d and depth/array layer %d is already attached to this render pass (WebGPU spec: GPURenderPassDescriptor Valid Usage)",
			label, e.AttachmentIndex, what, e.MipLevel, e.Layer)
	case RenderPassErrorTextureViewNotRenderable:
		return fmt.Sprintf("render pass %q: the %s is not renderable: %s (WebGPU spec: renderable texture view)",
			label, e.attachmentLocation(), e.notRenderableReason())
	case RenderPassErrorInvalidColorAttachmentFormat, RenderPassErrorInvalidDepthStencilAttachmentFormat,
		RenderPassErrorInvalidResolveSampleCounts, RenderPassErrorMismatchedResolveFormat,
		RenderPassErrorUnsupportedResolveFormat:
		return e.formatError(label)
	default:
		return fmt.Sprintf("render pass %q: unknown validation error", label)
	}
}

// IsRenderPassValidationError returns true if the error is a RenderPassValidationError.
func IsRenderPassValidationError(err error) bool {
	var rpve *RenderPassValidationError
	return errors.As(err, &rpve)
}

// =============================================================================
// Render Pass Compatibility Errors (pipeline vs pass)
// =============================================================================

// RenderPassCompatibilityErrorKind represents the type of pipeline/pass mismatch.
type RenderPassCompatibilityErrorKind int

const (
	// RenderPassCompatibilityErrorColorTargetCount indicates the pipeline's
	// fragment target count does not match the render pass's color attachment count.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#dom-gpurenderpassencoder-setpipeline
	RenderPassCompatibilityErrorColorTargetCount RenderPassCompatibilityErrorKind = iota

	// RenderPassCompatibilityErrorColorTargetFormat indicates a pipeline's
	// fragment target format does not match the render pass's color attachment format.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#dom-gpurenderpassencoder-setpipeline
	RenderPassCompatibilityErrorColorTargetFormat

	// RenderPassCompatibilityErrorSampleCount indicates the pipeline's sample
	// count does not match the render pass's sample count.
	// WebGPU spec: https://www.w3.org/TR/webgpu/#dom-gpurenderpassencoder-setpipeline
	RenderPassCompatibilityErrorSampleCount

	// RenderPassCompatibilityErrorDepthStencilFormat indicates the pipeline's
	// depth/stencil format does not match the render pass's depth/stencil format.
	// Matches Rust wgpu-core check_compatible: self.attachments.depth_stencil != other.attachments.depth_stencil
	RenderPassCompatibilityErrorDepthStencilFormat
)

// RenderPassCompatibilityError represents a compatibility mismatch between
// a render pipeline and the current render pass.
//
// Matches Rust wgpu-core device/mod.rs RenderPassCompatibilityError.
type RenderPassCompatibilityError struct {
	Kind          RenderPassCompatibilityErrorKind
	PipelineLabel string

	// For ColorTargetCount:
	PipelineTargets uint32
	PassAttachments uint32

	// For ColorTargetFormat:
	TargetIndex    int
	PipelineFormat string
	PassFormat     string

	// For SampleCount:
	PipelineSamples uint32
	PassSamples     uint32
}

// Error implements the error interface.
func (e *RenderPassCompatibilityError) Error() string {
	pipelineLabel := e.PipelineLabel
	if pipelineLabel == "" {
		pipelineLabel = unnamedLabel
	}

	switch e.Kind {
	case RenderPassCompatibilityErrorColorTargetCount:
		return fmt.Sprintf(
			"pipeline %q has %d fragment targets but render pass has %d color attachments; counts must match (WebGPU spec §10.3)",
			pipelineLabel, e.PipelineTargets, e.PassAttachments)
	case RenderPassCompatibilityErrorColorTargetFormat:
		return fmt.Sprintf(
			"pipeline %q fragment target [%d] format %s does not match render pass color attachment format %s (WebGPU spec §10.3)",
			pipelineLabel, e.TargetIndex, e.PipelineFormat, e.PassFormat)
	case RenderPassCompatibilityErrorSampleCount:
		return fmt.Sprintf(
			"pipeline %q sample count %d does not match render pass sample count %d (WebGPU spec §10.3)",
			pipelineLabel, e.PipelineSamples, e.PassSamples)
	case RenderPassCompatibilityErrorDepthStencilFormat:
		return fmt.Sprintf(
			"pipeline %q depth/stencil format %s does not match render pass depth/stencil format %s (WebGPU spec §10.3)",
			pipelineLabel, e.PipelineFormat, e.PassFormat)
	default:
		return fmt.Sprintf("pipeline %q: unknown render pass compatibility error", pipelineLabel)
	}
}

// IsRenderPassCompatibilityError returns true if the error is a RenderPassCompatibilityError.
func IsRenderPassCompatibilityError(err error) bool {
	var rpce *RenderPassCompatibilityError
	return errors.As(err, &rpce)
}

// =============================================================================
// Device Request Errors
// =============================================================================

// RequestDeviceErrorKind represents the type of device request error.
type RequestDeviceErrorKind int

const (
	// RequestDeviceErrorUnsupportedFeature indicates the request requires
	// features the adapter does not expose (Rust
	// RequestDeviceError::UnsupportedFeature).
	RequestDeviceErrorUnsupportedFeature RequestDeviceErrorKind = iota
	// RequestDeviceErrorLimitsExceeded indicates a required limit is better
	// than the adapter's (Rust RequestDeviceError::LimitsExceeded).
	RequestDeviceErrorLimitsExceeded
)

// RequestDeviceError represents an error during a device request.
type RequestDeviceError struct {
	Kind RequestDeviceErrorKind
	// MissingFeatures are the required features the adapter does not expose.
	MissingFeatures gputypes.Features
	// Limit is the gputypes.Limits field name of the exceeded limit, with the
	// requested value and the adapter's.
	Limit     string
	Requested uint64
	Allowed   uint64
}

// Error implements the error interface.
func (e *RequestDeviceError) Error() string {
	switch e.Kind {
	case RequestDeviceErrorUnsupportedFeature:
		return fmt.Sprintf("request device: the adapter does not support the required features %v", e.MissingFeatures)
	case RequestDeviceErrorLimitsExceeded:
		return fmt.Sprintf("request device: limit %s = %d is better than the adapter's %d", e.Limit, e.Requested, e.Allowed)
	default:
		return "request device: unknown error"
	}
}

// =============================================================================
// Transfer (copy command) Errors
// =============================================================================

// TransferErrorKind represents the type of copy command error.
type TransferErrorKind int

const (
	// TransferErrorUnalignedBytesPerRow indicates a bytesPerRow that is not a
	// multiple of CopyBytesPerRowAlignment (Rust TransferError::UnalignedBytesPerRow).
	TransferErrorUnalignedBytesPerRow TransferErrorKind = iota
	// TransferErrorUnalignedBufferOffset indicates a buffer offset that is not
	// a multiple of the texel block copy size (Rust
	// TransferError::UnalignedBufferOffset).
	TransferErrorUnalignedBufferOffset
	// TransferErrorInvalidSampleCount indicates a copy of a multisampled
	// texture (Rust TransferError::InvalidSampleCount).
	TransferErrorInvalidSampleCount
	// TransferErrorSameSourceDestinationBuffer indicates a buffer-to-buffer
	// copy whose source and destination are the same buffer (Rust
	// TransferError::SameSourceDestinationBuffer).
	TransferErrorSameSourceDestinationBuffer
	// TransferErrorMissingBufferUsage indicates a copy endpoint buffer that
	// was created without the usage the copy needs (Rust
	// TransferError::MissingBufferUsage).
	TransferErrorMissingBufferUsage
	// TransferErrorUnalignedCopySize indicates a buffer-to-buffer copy size
	// that is not a multiple of CopyBufferAlignment (Rust
	// TransferError::UnalignedCopySize).
	TransferErrorUnalignedCopySize
	// TransferErrorBufferStartOffsetOverrun indicates a copy offset past the
	// end of its buffer (Rust TransferError::BufferStartOffsetOverrun).
	TransferErrorBufferStartOffsetOverrun
	// TransferErrorBufferEndOffsetOverrun indicates a copy range that ends
	// past the end of its buffer (Rust TransferError::BufferEndOffsetOverrun).
	TransferErrorBufferEndOffsetOverrun
	// TransferErrorMissingTextureUsage indicates a copy endpoint texture that
	// was created without COPY_SRC or COPY_DST (Rust
	// TransferError::MissingTextureUsage).
	TransferErrorMissingTextureUsage
	// TransferErrorInvalidTextureMipLevel indicates a mip level the texture
	// does not have (Rust TransferError::InvalidTextureMipLevel).
	TransferErrorInvalidTextureMipLevel
	// TransferErrorInvalidTextureAspect indicates an aspect the texture's
	// format does not have (Rust TransferError::InvalidTextureAspect).
	TransferErrorInvalidTextureAspect
	// TransferErrorCopyAspectNotOne indicates a buffer-texture copy that does
	// not name a single aspect (Rust TransferError::CopyAspectNotOne).
	TransferErrorCopyAspectNotOne
	// TransferErrorCopySrcMissingAspects indicates a texture-to-texture copy
	// source that does not name every aspect of its format (Rust
	// TransferError::CopySrcMissingAspects).
	TransferErrorCopySrcMissingAspects
	// TransferErrorCopyDstMissingAspects is TransferErrorCopySrcMissingAspects
	// for the destination (Rust TransferError::CopyDstMissingAspects).
	TransferErrorCopyDstMissingAspects
	// TransferErrorCopyFromForbiddenTextureFormat indicates a format that can
	// not be copied from (Rust TransferError::CopyFromForbiddenTextureFormat).
	TransferErrorCopyFromForbiddenTextureFormat
	// TransferErrorCopyFromForbiddenTextureFormatAspect indicates a format and
	// aspect that can not be copied from (Rust
	// TransferError::CopyFromForbiddenTextureFormatAspect).
	TransferErrorCopyFromForbiddenTextureFormatAspect
	// TransferErrorCopyToForbiddenTextureFormat indicates a format that can
	// not be copied to (Rust TransferError::CopyToForbiddenTextureFormat).
	TransferErrorCopyToForbiddenTextureFormat
	// TransferErrorCopyToForbiddenTextureFormatAspect indicates a format and
	// aspect that can not be copied to (Rust
	// TransferError::CopyToForbiddenTextureFormatAspect).
	TransferErrorCopyToForbiddenTextureFormatAspect
	// TransferErrorSampleCountNotEqual indicates a texture-to-texture copy
	// between textures of different sample counts (Rust
	// TransferError::SampleCountNotEqual).
	TransferErrorSampleCountNotEqual
	// TransferErrorInvalidCopyWithinSameTexture indicates a copy whose source
	// and destination subresources overlap in one texture (Rust
	// TransferError::InvalidCopyWithinSameTexture).
	TransferErrorInvalidCopyWithinSameTexture
	// TransferErrorTextureOverrun indicates a copy region that ends past the
	// end of its texture's mip level (Rust TransferError::TextureOverrun).
	TransferErrorTextureOverrun
	// TransferErrorUnsupportedPartialTransfer indicates a partial copy of a
	// depth/stencil or multisampled texture, which must copy the whole mip
	// level in x and y (Rust TransferError::UnsupportedPartialTransfer).
	TransferErrorUnsupportedPartialTransfer
	// TransferErrorUnalignedCopyOriginX indicates an origin x that is not a
	// multiple of the block width (Rust TransferError::UnalignedCopyOriginX).
	TransferErrorUnalignedCopyOriginX
	// TransferErrorUnalignedCopyOriginY indicates an origin y that is not a
	// multiple of the block height (Rust TransferError::UnalignedCopyOriginY).
	TransferErrorUnalignedCopyOriginY
	// TransferErrorUnalignedCopyWidth indicates a copy width that is not a
	// multiple of the block width (Rust TransferError::UnalignedCopyWidth).
	TransferErrorUnalignedCopyWidth
	// TransferErrorUnalignedCopyHeight indicates a copy height that is not a
	// multiple of the block height (Rust TransferError::UnalignedCopyHeight).
	TransferErrorUnalignedCopyHeight
	// TransferErrorUnspecifiedBytesPerRow indicates a copy of more than one
	// row without bytesPerRow (Rust TransferError::UnspecifiedBytesPerRow).
	TransferErrorUnspecifiedBytesPerRow
	// TransferErrorUnspecifiedRowsPerImage indicates a copy of more than one
	// image without rowsPerImage (Rust TransferError::UnspecifiedRowsPerImage).
	TransferErrorUnspecifiedRowsPerImage
	// TransferErrorInvalidBytesPerRow indicates a bytesPerRow smaller than one
	// row of the copy (Rust TransferError::InvalidBytesPerRow).
	TransferErrorInvalidBytesPerRow
	// TransferErrorInvalidRowsPerImage indicates a rowsPerImage smaller than
	// the rows of one image of the copy (Rust TransferError::InvalidRowsPerImage).
	TransferErrorInvalidRowsPerImage
	// TransferErrorSizeOverflow indicates a copy whose byte size does not fit
	// in 64 bits (Rust TransferError::SizeOverflow).
	TransferErrorSizeOverflow
	// TransferErrorTextureFormatsNotCopyCompatible indicates a
	// texture-to-texture copy between formats that differ by more than the
	// sRGB suffix (Rust TransferError::TextureFormatsNotCopyCompatible).
	TransferErrorTextureFormatsNotCopyCompatible
	// TransferErrorMissingDownlevelFlags indicates a copy the device does not
	// support (Rust TransferError::MissingDownlevelFlags); Cause says which
	// flags are missing.
	TransferErrorMissingDownlevelFlags
	// TransferErrorStencilCopyNeedsBlit indicates a buffer or queue upload
	// into the stencil aspect of a combined depth-stencil format on a backend
	// that cannot write that aspect alone (hal.Capabilities.StencilCopyNeedsBlit,
	// GLES), when the frontend could not set up the blit it lowers such an
	// upload to (Device.StencilUploadNeedsBlit); Cause says why. Rust has no
	// such kind: it refuses every depth/stencil buffer copy on GL with
	// MissingDownlevelFlags.
	TransferErrorStencilCopyNeedsBlit
)

// TextureErrorDimension names the texture dimension a copy error is about
// (Rust TextureErrorDimension).
type TextureErrorDimension int

const (
	// TextureErrorDimensionX is the width.
	TextureErrorDimensionX TextureErrorDimension = iota
	// TextureErrorDimensionY is the height.
	TextureErrorDimensionY
	// TextureErrorDimensionZ is the depth or array layer count.
	TextureErrorDimensionZ
)

// String returns the name of the dimension.
func (d TextureErrorDimension) String() string {
	switch d {
	case TextureErrorDimensionY:
		return "Y"
	case TextureErrorDimensionZ:
		return "Z"
	default:
		return "X"
	}
}

// CopySide names the endpoint of a copy an error is about (Rust CopySide).
type CopySide int

const (
	// CopySideSource is the buffer or texture a copy reads from.
	CopySideSource CopySide = iota
	// CopySideDestination is the buffer or texture a copy writes to.
	CopySideDestination
)

// String returns the lowercase name of the side.
func (s CopySide) String() string {
	if s == CopySideDestination {
		return "destination"
	}
	return "source"
}

// TransferError represents an invalid copy command on a command encoder.
// Which fields are set depends on Kind.
type TransferError struct {
	Kind        TransferErrorKind
	BytesPerRow uint32
	Offset      uint64
	Alignment   uint32
	// SampleCount is the texture sample count (the source's for
	// TransferErrorSampleCountNotEqual).
	SampleCount uint32
	// DstSampleCount is the destination sample count (TransferErrorSampleCountNotEqual).
	DstSampleCount uint32
	// Side is the endpoint the error is about (usage, offset and overrun kinds).
	Side CopySide
	// Size is the copy size in bytes (size and end-overrun kinds).
	Size uint64
	// BufferSize is the size of the endpoint buffer (overrun kinds).
	BufferSize uint64
	// Usage is the usage the endpoint buffer lacks (TransferErrorMissingBufferUsage).
	Usage gputypes.BufferUsage
	// TextureUsage is the usage the endpoint texture lacks (TransferErrorMissingTextureUsage).
	TextureUsage gputypes.TextureUsage
	// MipLevel and MipLevelCount describe TransferErrorInvalidTextureMipLevel.
	MipLevel, MipLevelCount uint32
	// Format and Aspect describe the invalid aspect and forbidden format kinds;
	// Aspect is also the source aspect of TransferErrorInvalidCopyWithinSameTexture.
	Format gputypes.TextureFormat
	Aspect gputypes.TextureAspect
	// OriginZ, DstOriginZ and Layers describe TransferErrorInvalidCopyWithinSameTexture.
	OriginZ, DstOriginZ, Layers uint32
	// Dimension, StartOffset, EndOffset and TextureSize describe
	// TransferErrorTextureOverrun and TransferErrorUnsupportedPartialTransfer:
	// the copy covers [StartOffset, EndOffset) of a Dimension that is
	// TextureSize texels long. Format, SampleCount and Side are set as well.
	Dimension                           TextureErrorDimension
	StartOffset, EndOffset, TextureSize uint32
	// DstFormat is the destination format of
	// TransferErrorTextureFormatsNotCopyCompatible (Format is the source's).
	DstFormat gputypes.TextureFormat
	// Cause is the underlying error of TransferErrorMissingDownlevelFlags,
	// and of TransferErrorStencilCopyNeedsBlit when the frontend's stencil
	// blit itself failed.
	Cause error
}

// Unwrap returns the underlying error, if any.
func (e *TransferError) Unwrap() error { return e.Cause }

// Error implements the error interface.
func (e *TransferError) Error() string {
	switch e.Kind {
	case TransferErrorUnalignedBytesPerRow:
		return fmt.Sprintf("copy: bytesPerRow %d is not a multiple of %d", e.BytesPerRow, CopyBytesPerRowAlignment)
	case TransferErrorUnalignedBufferOffset:
		return fmt.Sprintf("copy: buffer offset %d is not a multiple of %d", e.Offset, e.Alignment)
	case TransferErrorInvalidSampleCount:
		return fmt.Sprintf("copy: texture sample count %d, a buffer-texture copy needs 1", e.SampleCount)
	case TransferErrorSameSourceDestinationBuffer:
		return "copy: source and destination are the same buffer"
	case TransferErrorMissingBufferUsage:
		return fmt.Sprintf("copy: %s buffer is missing usage %#x", e.Side, e.Usage)
	case TransferErrorUnalignedCopySize:
		return fmt.Sprintf("copy: size %d is not a multiple of %d", e.Size, CopyBufferAlignment)
	case TransferErrorBufferStartOffsetOverrun:
		return fmt.Sprintf("copy: %s offset %d is past the end of the %d-byte buffer", e.Side, e.Offset, e.BufferSize)
	case TransferErrorBufferEndOffsetOverrun:
		return fmt.Sprintf("copy: %s range of %d bytes at offset %d overruns the %d-byte buffer",
			e.Side, e.Size, e.Offset, e.BufferSize)
	default:
		return e.textureErrorText()
	}
}

// textureErrorText is Error for the texture copy kinds.
func (e *TransferError) textureErrorText() string {
	switch e.Kind {
	case TransferErrorMissingTextureUsage:
		return fmt.Sprintf("copy: %s texture is missing usage %#x", e.Side, e.TextureUsage)
	case TransferErrorInvalidTextureMipLevel:
		return fmt.Sprintf("copy: mip level %d does not exist, the texture has %d", e.MipLevel, e.MipLevelCount)
	case TransferErrorInvalidTextureAspect:
		return fmt.Sprintf("copy: format %v has no aspect %v", e.Format, e.Aspect)
	case TransferErrorCopyAspectNotOne:
		return "copy: a buffer-texture copy must name a single aspect of the texture format"
	case TransferErrorCopySrcMissingAspects:
		return "copy: the source must name all aspects of its texture format"
	case TransferErrorCopyDstMissingAspects:
		return "copy: the destination must name all aspects of its texture format"
	case TransferErrorCopyFromForbiddenTextureFormat:
		return fmt.Sprintf("copy: copying from textures of format %v is forbidden", e.Format)
	case TransferErrorCopyFromForbiddenTextureFormatAspect:
		return fmt.Sprintf("copy: copying from format %v with aspect %v is forbidden", e.Format, e.Aspect)
	case TransferErrorCopyToForbiddenTextureFormat:
		return fmt.Sprintf("copy: copying to textures of format %v is forbidden", e.Format)
	case TransferErrorCopyToForbiddenTextureFormatAspect:
		return fmt.Sprintf("copy: copying to format %v with aspect %v is forbidden", e.Format, e.Aspect)
	case TransferErrorSampleCountNotEqual:
		return fmt.Sprintf("copy: source sample count %d differs from destination sample count %d",
			e.SampleCount, e.DstSampleCount)
	case TransferErrorInvalidCopyWithinSameTexture:
		return fmt.Sprintf("copy: source and destination overlap in one texture (aspect %v, origin z %d and %d, %d layers)",
			e.Aspect, e.OriginZ, e.DstOriginZ, e.Layers)
	default:
		return e.rangeErrorText()
	}
}

// rangeErrorText is Error for the texture copy range, buffer layout and
// compatibility kinds.
func (e *TransferError) rangeErrorText() string {
	switch e.Kind {
	case TransferErrorTextureOverrun:
		return fmt.Sprintf("copy: %s %d..%d overruns the %s texture, whose %s size is %d",
			e.Dimension, e.StartOffset, e.EndOffset, e.Side, e.Dimension, e.TextureSize)
	case TransferErrorUnsupportedPartialTransfer:
		return fmt.Sprintf("copy: partial copy of %s %d..%d (size %d) is not supported for the %s texture format %v with %d samples",
			e.Dimension, e.StartOffset, e.EndOffset, e.TextureSize, e.Side, e.Format, e.SampleCount)
	case TransferErrorUnalignedCopyOriginX:
		return "copy: origin x is not a multiple of the block width"
	case TransferErrorUnalignedCopyOriginY:
		return "copy: origin y is not a multiple of the block height"
	case TransferErrorUnalignedCopyWidth:
		return "copy: width is not a multiple of the block width"
	case TransferErrorUnalignedCopyHeight:
		return "copy: height is not a multiple of the block height"
	case TransferErrorUnspecifiedBytesPerRow:
		return "copy: bytesPerRow must be specified because more than one row is copied"
	case TransferErrorUnspecifiedRowsPerImage:
		return "copy: rowsPerImage must be specified because more than one image is copied"
	case TransferErrorInvalidBytesPerRow:
		return fmt.Sprintf("copy: bytesPerRow %d is less than the bytes of one complete row", e.BytesPerRow)
	case TransferErrorInvalidRowsPerImage:
		return "copy: rowsPerImage is less than the rows of one image"
	case TransferErrorSizeOverflow:
		return "copy: overflow while computing the size of the copy"
	case TransferErrorTextureFormatsNotCopyCompatible:
		return fmt.Sprintf("copy: source format %v and destination format %v are not copy-compatible (they may only differ in sRGB-ness)",
			e.Format, e.DstFormat)
	case TransferErrorMissingDownlevelFlags:
		return fmt.Sprintf("copy: the device does not support this copy: %v", e.Cause)
	case TransferErrorStencilCopyNeedsBlit:
		return fmt.Sprintf("copy: the backend cannot write the stencil aspect of %v from a buffer or queue data, and the stencil blit failed: %v", e.Format, e.Cause)
	default:
		return "copy: unknown error"
	}
}

// =============================================================================
// Clear command Errors
// =============================================================================

// ClearErrorKind represents the type of clear command error. Rust wgpu-core
// keeps these in ClearError, apart from TransferError.
type ClearErrorKind int

const (
	// ClearErrorMissingBufferUsage indicates a cleared buffer created without
	// COPY_DST (Rust ClearError::MissingBufferUsage).
	ClearErrorMissingBufferUsage ClearErrorKind = iota
	// ClearErrorUnalignedBufferOffset indicates an offset that is not a
	// multiple of CopyBufferAlignment (Rust ClearError::UnalignedBufferOffset).
	ClearErrorUnalignedBufferOffset
	// ClearErrorUnalignedFillSize indicates a size that is not a multiple of
	// CopyBufferAlignment (Rust ClearError::UnalignedFillSize).
	ClearErrorUnalignedFillSize
	// ClearErrorOffsetPlusSizeExceeds64BitBounds indicates offset plus size
	// above the largest uint64 (Rust ClearError::OffsetPlusSizeExceeds64BitBounds).
	ClearErrorOffsetPlusSizeExceeds64BitBounds
	// ClearErrorBufferOverrun indicates a cleared range that ends past the end
	// of the buffer (Rust ClearError::BufferOverrun).
	ClearErrorBufferOverrun
)

// ClearError represents an invalid clearBuffer command on a command encoder.
// Which fields are set depends on Kind.
type ClearError struct {
	Kind ClearErrorKind
	// Offset is the byte offset the clear starts at.
	Offset uint64
	// Size is the number of bytes to clear.
	Size uint64
	// BufferSize is the size of the buffer (overrun kind).
	BufferSize uint64
	// Usage is the usage the buffer lacks (ClearErrorMissingBufferUsage).
	Usage gputypes.BufferUsage
}

// Error implements the error interface.
func (e *ClearError) Error() string {
	switch e.Kind {
	case ClearErrorMissingBufferUsage:
		return fmt.Sprintf("clear: buffer is missing usage %#x", e.Usage)
	case ClearErrorUnalignedBufferOffset:
		return fmt.Sprintf("clear: buffer offset %d is not a multiple of %d", e.Offset, CopyBufferAlignment)
	case ClearErrorUnalignedFillSize:
		return fmt.Sprintf("clear: size %d is not a multiple of %d", e.Size, CopyBufferAlignment)
	case ClearErrorOffsetPlusSizeExceeds64BitBounds:
		return fmt.Sprintf("clear: offset %d plus size %d exceeds the 64-bit range", e.Offset, e.Size)
	case ClearErrorBufferOverrun:
		return fmt.Sprintf("clear: range of %d bytes at offset %d overruns the %d-byte buffer", e.Size, e.Offset, e.BufferSize)
	default:
		return "clear: unknown error"
	}
}
