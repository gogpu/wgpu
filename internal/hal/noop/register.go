//go:build !(js && wasm)

package noop

import "github.com/gogpu/wgpu/internal/hal"

// Register registers the noop backend with the HAL registry.
//
// The package does not register itself in init: the no-op backend is a test
// double, and the global registry is what production code sees. Since v0.35.0
// it owns the BackendEmpty variant alone (software reports BackendSoftware,
// ADR-083 §2.3), so an init registration would no longer collide — it would
// simply put a no-op backend into every binary that links this package. Tests
// and tools that need it in the global registry call Register explicitly.
func Register() {
	hal.RegisterBackend(Backend{})
}
