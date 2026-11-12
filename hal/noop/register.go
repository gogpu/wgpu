//go:build !(js && wasm)

package noop

import "github.com/gogpu/wgpu/hal"

// Register registers the noop backend with the HAL registry.
//
// The package does not register itself in init. The noop backend shares the
// BackendEmpty variant with the Pure Go software backend, which registers when
// imported (and by default from v0.35.0, ADR-083); an init registration here
// would replace it depending on package initialization order. Tests and tools
// that need the noop backend in the global registry call Register explicitly;
// production code never does.
func Register() {
	hal.RegisterBackend(Backend{})
}
