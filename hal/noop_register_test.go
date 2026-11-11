//go:build !(js && wasm)

package hal_test

import "github.com/gogpu/wgpu/hal/noop"

// The noop backend does not register itself in init: it shares the
// BackendEmpty variant with the software backend, which registers by default.
// The hal tests that inspect the registry opt in here.
func init() {
	noop.Register()
}
