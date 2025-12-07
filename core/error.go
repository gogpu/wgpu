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
