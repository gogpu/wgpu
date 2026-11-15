//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package backend

import "github.com/gogpu/wgpu/internal/hal/software"

// OpenSoftware opens a private Pure Go software instance.
//
// Importing the software package also registers the software backend in the
// global HAL registry: that is a side effect of this file's import, not of
// calling the constructor. Since v0.35.0 the software backend registers by
// default in a normal binary anyway (ADR-083 §2.3), and it is selected only
// through the BackendsSoftware mask; the root package doc states the contract.
func OpenSoftware() (Opened, error) {
	return Open(software.NewBackend())
}
