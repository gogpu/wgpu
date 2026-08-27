package raytracing

import "fmt"

// Operation name constants for ValidationError.Op.
const (
	opRayTracing = "RayTracing"
	opCreateBlas = "CreateBlas"
	opCreateTlas = "CreateTlas"
	opBuildBlas  = "BuildBlas"
	opBuildTlas  = "BuildTlas"
	opCompact    = "Compact"
)

// ValidationError represents a ray tracing validation failure.
//
// Op identifies the operation that failed (e.g., "CreateBlas", "BuildTlas").
// Message describes the specific validation rule that was violated.
type ValidationError struct {
	Op      string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("ray tracing %s: %s", e.Op, e.Message)
}
