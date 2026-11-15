//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// Apply folds the public option functions over cfg. It lives here so the public
// facade holds no loop of its own (the public-layer rule, ADR-070 §15.10): the
// option type belongs to the caller, so the fold is generic over its element.
func Apply[E ~func(*T), T any](opts []E, cfg *T) {
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
}
