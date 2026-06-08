// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestAdapter_Open_NilGLCtxReturnsDescriptiveError verifies that calling
// Open() on an Adapter with nil glCtx returns a clear error message instead
// of panicking at GenVertexArrays. This is the defense-in-depth guard for the
// path where EnumerateAdapters(nil) returns a zero-value Adapter.
func TestAdapter_Open_NilGLCtxReturnsDescriptiveError(t *testing.T) {
	a := &Adapter{
		glCtx:  nil, // zero-value: no EGL context was created
		eglCtx: nil,
	}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Open() panicked instead of returning an error: %v", rec)
		}
	}()

	_, err := a.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("Open() with nil glCtx should return an error, got nil")
	}
	if !strings.Contains(err.Error(), "surface hint") {
		t.Errorf("error %q should mention 'surface hint' to guide the caller", err.Error())
	}
}

// TestAdapter_Open_NilGLCtxErrorMentionsSurfaceHint verifies the error message
// from the nil-glCtx guard is actionable (tells the caller what to do).
func TestAdapter_Open_NilGLCtxErrorIsActionable(t *testing.T) {
	a := &Adapter{glCtx: nil}

	_, err := a.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	msg := err.Error()
	wantSubstrings := []string{"gles:", "surface hint"}
	for _, s := range wantSubstrings {
		if !strings.Contains(msg, s) {
			t.Errorf("error %q should contain %q", msg, s)
		}
	}
}
