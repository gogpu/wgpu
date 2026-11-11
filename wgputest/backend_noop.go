//go:build !rust && !(js && wasm)

package wgputest

import "github.com/gogpu/wgpu/hal/noop"

// openNoop opens a private noop instance. The noop backend does not register
// itself, so this leaves the global HAL registry untouched.
func openNoop() (openedBackend, error) {
	return openBackend(noop.NewBackend())
}
