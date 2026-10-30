//go:build windows && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

// noProcs is a proc loader of a driver that exports nothing.
func noProcs(string) uintptr { return 0 }
