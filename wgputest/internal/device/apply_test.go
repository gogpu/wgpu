//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import "testing"

// TestApplyNilOptionPanics pins the pre-move semantics: an Option is a function, so a nil one panics
// through the call. Skipping it would make NewNoopDevice(nil) succeed where it used to panic — a silent
// public behavior change hidden in an internal move.
func TestApplyNilOptionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Apply with a nil option did not panic; a nil Option must stay a programming error")
		}
	}()
	var cfg struct{}
	Apply([]func(*struct{}){nil}, &cfg)
}

// TestApplyFoldsInOrder proves the fold still applies every non-nil option, in order.
func TestApplyFoldsInOrder(t *testing.T) {
	var cfg struct{ n int }
	Apply([]func(*struct{ n int }){
		func(c *struct{ n int }) { c.n = 1 },
		func(c *struct{ n int }) { c.n = c.n*10 + 2 },
	}, &cfg)
	if cfg.n != 12 {
		t.Fatalf("cfg.n = %d, want 12", cfg.n)
	}
}
