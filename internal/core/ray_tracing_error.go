//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
)

// =============================================================================
// Ray Tracing Errors
// =============================================================================
//
// These mirror the error enums in Rust wgpu-core ray_tracing.rs
// (CreateBlasError, CreateTlasError, BuildAccelerationStructureError,
// ValidateAsActionsError). Each error carries a Kind for programmatic
// inspection and a preformatted message with the details of the variant.

// CreateBlasErrorKind represents the type of BLAS creation error.
type CreateBlasErrorKind int

const (
	// CreateBlasErrorMissingFeature indicates a required feature is not enabled.
	CreateBlasErrorMissingFeature CreateBlasErrorKind = iota
	// CreateBlasErrorMissingIndexData indicates exactly one of IndexFormat and
	// IndexCount was provided.
	CreateBlasErrorMissingIndexData
	// CreateBlasErrorInvalidVertexFormat indicates the vertex format is not
	// allowed for acceleration structures.
	CreateBlasErrorInvalidVertexFormat
	// CreateBlasErrorTooManyGeometries indicates MaxBlasGeometryCount was exceeded.
	CreateBlasErrorTooManyGeometries
	// CreateBlasErrorTooManyPrimitives indicates MaxBlasPrimitiveCount was exceeded.
	CreateBlasErrorTooManyPrimitives
	// CreateBlasErrorGeometryKind indicates both Triangles and AABBs were set.
	CreateBlasErrorGeometryKind
	// CreateBlasErrorHAL indicates the HAL backend failed to create the BLAS.
	CreateBlasErrorHAL
)

// CreateBlasError represents an error during BLAS creation.
type CreateBlasError struct {
	Kind     CreateBlasErrorKind
	Label    string
	Message  string
	HALError error
}

// Error implements the error interface.
func (e *CreateBlasError) Error() string {
	return fmt.Sprintf("create BLAS %q: %s", labelOrUnnamed(e.Label), e.Message)
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateBlasError) Unwrap() error {
	return e.HALError
}

// CreateTlasErrorKind represents the type of TLAS creation error.
type CreateTlasErrorKind int

const (
	// CreateTlasErrorMissingFeature indicates a required feature is not enabled.
	CreateTlasErrorMissingFeature CreateTlasErrorKind = iota
	// CreateTlasErrorDisallowedFlag indicates a flag that is not allowed on a TLAS.
	CreateTlasErrorDisallowedFlag
	// CreateTlasErrorTooManyInstances indicates MaxTlasInstanceCount was exceeded.
	CreateTlasErrorTooManyInstances
	// CreateTlasErrorHAL indicates the HAL backend failed to create the TLAS.
	CreateTlasErrorHAL
)

// CreateTlasError represents an error during TLAS creation.
type CreateTlasError struct {
	Kind     CreateTlasErrorKind
	Label    string
	Message  string
	HALError error
}

// Error implements the error interface.
func (e *CreateTlasError) Error() string {
	return fmt.Sprintf("create TLAS %q: %s", labelOrUnnamed(e.Label), e.Message)
}

// Unwrap returns the underlying HAL error, if any.
func (e *CreateTlasError) Unwrap() error {
	return e.HALError
}

// BuildAccelerationStructureErrorKind represents the type of
// acceleration structure build error.
type BuildAccelerationStructureErrorKind int

const (
	// BuildASErrorMissingFeature indicates FeatureRayQuery is not enabled.
	BuildASErrorMissingFeature BuildAccelerationStructureErrorKind = iota
	// BuildASErrorDestroyedResource indicates a BLAS, TLAS, or buffer was destroyed.
	BuildASErrorDestroyedResource
	// BuildASErrorMissingBufferUsage indicates an input buffer lacks BLAS_INPUT usage.
	BuildASErrorMissingBufferUsage
	// BuildASErrorInsufficientBufferSize indicates a buffer is too small for its region.
	BuildASErrorInsufficientBufferSize
	// BuildASErrorOffsetLimitedTo4GB indicates an offset does not fit in 32 bits.
	BuildASErrorOffsetLimitedTo4GB
	// BuildASErrorUnalignedTransformBufferOffset indicates a misaligned transform offset.
	BuildASErrorUnalignedTransformBufferOffset
	// BuildASErrorInvalidIndexCount indicates an index count not divisible by 3.
	BuildASErrorInvalidIndexCount
	// BuildASErrorMissingAssociatedData indicates a buffer was given without its offset.
	BuildASErrorMissingAssociatedData
	// BuildASErrorIncompatibleBlasBuildSizes indicates more geometries than at creation.
	BuildASErrorIncompatibleBlasBuildSizes
	// BuildASErrorIncompatibleBlasFlags indicates geometry flags differ from creation.
	BuildASErrorIncompatibleBlasFlags
	// BuildASErrorIncompatibleBlasVertexCount indicates more vertices than at creation.
	BuildASErrorIncompatibleBlasVertexCount
	// BuildASErrorDifferentBlasVertexFormats indicates the vertex format differs from creation.
	BuildASErrorDifferentBlasVertexFormats
	// BuildASErrorVertexStrideTooSmall indicates the vertex stride is below the format size.
	BuildASErrorVertexStrideTooSmall
	// BuildASErrorVertexStrideUnaligned indicates the vertex stride is misaligned.
	BuildASErrorVertexStrideUnaligned
	// BuildASErrorBlasIndexCountProvidedMismatch indicates an index count was given at
	// creation or build, but not both.
	BuildASErrorBlasIndexCountProvidedMismatch
	// BuildASErrorIncompatibleBlasIndexCount indicates more indices than at creation.
	BuildASErrorIncompatibleBlasIndexCount
	// BuildASErrorDifferentBlasIndexFormats indicates the index format differs from creation.
	BuildASErrorDifferentBlasIndexFormats
	// BuildASErrorCompactedBlas indicates a compacted BLAS was rebuilt.
	BuildASErrorCompactedBlas
	// BuildASErrorMissingIndexBuffer indicates the size descriptor needs an index buffer.
	BuildASErrorMissingIndexBuffer
	// BuildASErrorTlasInvalidCustomIndex indicates instance custom data of 2^24 or more.
	BuildASErrorTlasInvalidCustomIndex
	// BuildASErrorTlasInstanceCountExceeded indicates more instances than MaxInstances.
	BuildASErrorTlasInstanceCountExceeded
	// BuildASErrorTransformMissing indicates USE_TRANSFORM without a transform buffer.
	BuildASErrorTransformMissing
	// BuildASErrorUseTransformMissing indicates a transform buffer without USE_TRANSFORM.
	BuildASErrorUseTransformMissing
	// BuildASErrorTlasDependentMissingVertexReturn indicates a vertex-return TLAS
	// instance references a BLAS without ALLOW_RAY_HIT_VERTEX_RETURN.
	BuildASErrorTlasDependentMissingVertexReturn
	// BuildASErrorBlasGeometryKindMismatch indicates triangles vs AABBs differ from creation.
	BuildASErrorBlasGeometryKindMismatch
	// BuildASErrorIncompatibleBlasAabbPrimitiveCount indicates more AABBs than at creation.
	BuildASErrorIncompatibleBlasAabbPrimitiveCount
	// BuildASErrorUnalignedAabbPrimitiveOffset indicates an AABB offset not a multiple of 8.
	BuildASErrorUnalignedAabbPrimitiveOffset
	// BuildASErrorInvalidAabbStride indicates an AABB stride below the minimum or misaligned.
	BuildASErrorInvalidAabbStride
	// BuildASErrorGeometryKind indicates both Triangles and AABBs were set on one entry.
	BuildASErrorGeometryKind
	// BuildASErrorHAL indicates a HAL failure (e.g., scratch buffer allocation).
	BuildASErrorHAL
)

// BuildAccelerationStructureError represents an error recorded by
// CommandEncoder.BuildAccelerationStructures.
type BuildAccelerationStructureError struct {
	Kind     BuildAccelerationStructureErrorKind
	Message  string
	HALError error
}

// Error implements the error interface.
func (e *BuildAccelerationStructureError) Error() string {
	return "build acceleration structures: " + e.Message
}

// Unwrap returns the underlying HAL error, if any.
func (e *BuildAccelerationStructureError) Unwrap() error {
	return e.HALError
}

func newBuildASError(kind BuildAccelerationStructureErrorKind, format string, args ...any) *BuildAccelerationStructureError {
	return &BuildAccelerationStructureError{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// ValidateAsActionsErrorKind represents the type of submit-time acceleration
// structure validation error.
type ValidateAsActionsErrorKind int

const (
	// ValidateAsActionsErrorDestroyedResource indicates a used BLAS or TLAS was destroyed.
	ValidateAsActionsErrorDestroyedResource ValidateAsActionsErrorKind = iota
	// ValidateAsActionsErrorUsedUnbuiltTlas indicates a TLAS was used before being built.
	ValidateAsActionsErrorUsedUnbuiltTlas
	// ValidateAsActionsErrorUsedUnbuiltBlas indicates a TLAS references an unbuilt BLAS.
	ValidateAsActionsErrorUsedUnbuiltBlas
	// ValidateAsActionsErrorBlasNewerThanTlas indicates a BLAS was rebuilt after the
	// TLAS that references it.
	ValidateAsActionsErrorBlasNewerThanTlas
)

// ValidateAsActionsError represents a submit-time acceleration structure
// validation error.
//
// Matches Rust wgpu-core ray_tracing.rs ValidateAsActionsError.
type ValidateAsActionsError struct {
	Kind    ValidateAsActionsErrorKind
	Message string
}

// Error implements the error interface.
func (e *ValidateAsActionsError) Error() string {
	return "acceleration structure validation: " + e.Message
}

// IsRayTracingError returns true if err is any of the ray tracing errors.
func IsRayTracingError(err error) bool {
	var (
		blasErr     *CreateBlasError
		tlasErr     *CreateTlasError
		buildErr    *BuildAccelerationStructureError
		validateErr *ValidateAsActionsError
	)
	return errors.As(err, &blasErr) || errors.As(err, &tlasErr) ||
		errors.As(err, &buildErr) || errors.As(err, &validateErr)
}

func labelOrUnnamed(label string) string {
	if label == "" {
		return unnamedLabel
	}
	return label
}
