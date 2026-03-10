package core

import (
	"errors"
	"fmt"
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
)

// CreateBufferError represents an error during buffer creation.
type CreateBufferError struct {
	Kind          CreateBufferErrorKind
	Label         string
	RequestedSize uint64
	MaxSize       uint64
	HALError      error
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
		return fmt.Sprintf("texture %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateTextureError) Unwrap() error {
	return e.HALError
}

// IsCreateTextureError returns true if the error is a CreateTextureError.
func IsCreateTextureError(err error) bool {
	var cte *CreateTextureError
	return errors.As(err, &cte)
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
)

// CreateShaderModuleError represents an error during shader module creation.
type CreateShaderModuleError struct {
	Kind     CreateShaderModuleErrorKind
	Label    string
	HALError error
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
	default:
		return fmt.Sprintf("shader module %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateShaderModuleError) Unwrap() error {
	return e.HALError
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
	// CreateRenderPipelineErrorNoFragmentTargets indicates the fragment stage had no color targets.
	CreateRenderPipelineErrorNoFragmentTargets
	// CreateRenderPipelineErrorTooManyColorTargets indicates too many color targets.
	CreateRenderPipelineErrorTooManyColorTargets
	// CreateRenderPipelineErrorInvalidSampleCount indicates an invalid multisample count.
	CreateRenderPipelineErrorInvalidSampleCount
	// CreateRenderPipelineErrorHAL indicates the HAL backend failed to create the pipeline.
	CreateRenderPipelineErrorHAL
)

// CreateRenderPipelineError represents an error during render pipeline creation.
type CreateRenderPipelineError struct {
	Kind        CreateRenderPipelineErrorKind
	Label       string
	TargetCount uint32
	MaxTargets  uint32
	SampleCount uint32
	HALError    error
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
		return fmt.Sprintf("render pipeline %q: fragment stage must have at least one color target", label)
	case CreateRenderPipelineErrorTooManyColorTargets:
		return fmt.Sprintf("render pipeline %q: color target count %d exceeds maximum %d",
			label, e.TargetCount, e.MaxTargets)
	case CreateRenderPipelineErrorInvalidSampleCount:
		return fmt.Sprintf("render pipeline %q: invalid sample count %d (must be 1 or 4)",
			label, e.SampleCount)
	case CreateRenderPipelineErrorHAL:
		return fmt.Sprintf("render pipeline %q: HAL error: %v", label, e.HALError)
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
)

// CreateComputePipelineError represents an error during compute pipeline creation.
type CreateComputePipelineError struct {
	Kind     CreateComputePipelineErrorKind
	Label    string
	HALError error
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
	default:
		return fmt.Sprintf("compute pipeline %q: unknown error", label)
	}
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateComputePipelineError) Unwrap() error {
	return e.HALError
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
	// CreateBindGroupErrorHAL indicates the HAL backend failed.
	CreateBindGroupErrorHAL
)

// CreateBindGroupError represents an error during bind group creation.
type CreateBindGroupError struct {
	Kind     CreateBindGroupErrorKind
	Label    string
	HALError error
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
	case CreateBindGroupErrorHAL:
		return fmt.Sprintf("bind group %q: HAL error: %v", label, e.HALError)
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

// EncoderStateError represents an invalid state transition error.
type EncoderStateError struct {
	Operation string
	Status    CommandEncoderStatus
}

// Error implements the error interface.
func (e *EncoderStateError) Error() string {
	return fmt.Sprintf("cannot %s: encoder in %v state", e.Operation, e.Status)
}

// IsEncoderStateError returns true if the error is an EncoderStateError.
func IsEncoderStateError(err error) bool {
	var ese *EncoderStateError
	return errors.As(err, &ese)
}
