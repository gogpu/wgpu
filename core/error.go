package core

import (
	"errors"
	"fmt"
)

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
		label = "<unnamed>"
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
		label = "<unnamed>"
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
