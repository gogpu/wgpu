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
// The backend is identified as types.BackendEmpty.
package noop
