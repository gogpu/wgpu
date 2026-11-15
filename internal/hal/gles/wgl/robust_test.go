// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package wgl

import (
	"slices"
	"testing"
)

// TestRobustContextAttribs checks the wglCreateContextAttribsARB attribute
// list: robust access in WGL_CONTEXT_FLAGS_ARB, no reset notification, no
// version or profile, zero-terminated.
func TestRobustContextAttribs(t *testing.T) {
	want := []int32{0x2094, 0x4, 0x8256, 0x8261, 0}
	if got := robustContextAttribs(); !slices.Equal(got, want) {
		t.Errorf("robustContextAttribs = %#x, want %#x", got, want)
	}
}

// TestHasExtension checks whole-token matching of WGL extension strings.
func TestHasExtension(t *testing.T) {
	exts := "WGL_ARB_extensions_string WGL_ARB_create_context_robustness WGL_EXT_swap_control"
	tests := []struct {
		name string
		want bool
	}{
		{"WGL_ARB_create_context_robustness", true},
		{"WGL_EXT_swap_control", true},
		{"WGL_ARB_create_context", false}, // a prefix of a listed name, not listed itself
		{"WGL_ARB_create", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := hasExtension(exts, tt.name); got != tt.want {
			t.Errorf("hasExtension(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestValidProcAddress checks the wglGetProcAddress failure values.
func TestValidProcAddress(t *testing.T) {
	for _, addr := range []uintptr{0, 1, 2, 3, ^uintptr(0)} {
		if validProcAddress(addr) {
			t.Errorf("validProcAddress(%#x) = true, want false", addr)
		}
	}
	if !validProcAddress(0x7ffd1234) {
		t.Error("validProcAddress(0x7ffd1234) = false, want true")
	}
}
