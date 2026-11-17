//go:build !(js && wasm)

// Package noop provides a no-operation GPU backend.
//
// The noop backend implements all HAL interfaces but performs no actual GPU operations.
// It is useful for:
//   - Testing code without GPU hardware
//   - CI/CD environments without GPU access
//   - Reference implementation showing minimal HAL requirements
//   - Fallback when no real backend is available
//
// All operations succeed immediately and return placeholder resources.
// The backend is identified as gputypes.BackendEmpty.
//
// The package does not register itself in init: the no-op backend is a test
// double, and the global registry is what production code sees. Since v0.35.0
// it owns the BackendEmpty variant alone (the software backend reports
// BackendSoftware, ADR-083 §2.3), so an automatic registration would no longer
// collide — it would simply put a no-op backend into every binary that links
// this package. Call Register to add the noop backend to the global HAL
// registry.
package noop
