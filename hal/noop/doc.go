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
// The package does not register itself in init: it shares the BackendEmpty
// variant with the Pure Go software backend, which registers when imported
// (and by default from v0.35.0), so an automatic registration could replace it
// depending on package initialization order. Call Register to add the noop
// backend to the global HAL registry.
package noop
