//go:build !rust && !(js && wasm) && !(((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !wgpu_nosoftware)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package backend

// OpenSoftware reports ErrUnavailable where the Pure Go software backend is
// not in the build: outside its supported platforms, or under wgpu_nosoftware.
func OpenSoftware() (Opened, error) {
	return Opened{}, ErrUnavailable
}
