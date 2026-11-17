//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

// TestReleased reports whether the buffer has been released (testing only).
//
// This is the hook relocated from the root package's export_test.go: a
// test-only method cannot stay behind the type it is declared on, so it moved
// with the declaration (ADR-070 §11 grain note). The root keeps a projection
// over the public wrapper, which is what its public-path tests exercise.
func (b *Buffer) TestReleased() bool { return b.IsReleased() }
