//go:build linux && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

import "unsafe"

// noProcs is a proc loader of a driver that exports nothing.
func noProcs(string) unsafe.Pointer { return nil }
