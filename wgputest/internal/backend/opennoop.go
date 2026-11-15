//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package backend

import "github.com/gogpu/wgpu/internal/hal/noop"

// OpenNoop opens a private noop instance. The noop backend does not register
// itself, so this leaves the global HAL registry untouched.
func OpenNoop() (Opened, error) {
	return Open(noop.NewBackend())
}
