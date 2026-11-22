// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestPlaceholderIsMarked pins P1-API-040 R1 at its source: the GLES
// placeholder returned without an EGL context is marked, so the public
// enumeration hides it instead of showing a GPU that cannot open. Its other
// characteristics stay as they are — adapter selection still sees it.
func TestPlaceholderIsMarked(t *testing.T) {
	exposed := placeholderExposedAdapter("test")
	if !exposed.Placeholder {
		t.Fatal("placeholderExposedAdapter returned an unmarked adapter; the public enumeration would show an adapter that cannot open")
	}
	if exposed.Adapter == nil {
		t.Fatal("placeholder has no adapter handle")
	}
	if exposed.Info.Backend != gputypes.BackendGL {
		t.Fatalf("placeholder backend = %v, want BackendGL", exposed.Info.Backend)
	}
}
