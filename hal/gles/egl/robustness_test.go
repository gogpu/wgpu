// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package egl

import (
	"slices"
	"testing"
)

const robustExts = "EGL_KHR_surfaceless_context EGL_EXT_create_context_robustness EGL_KHR_no_config_context"

// TestInitialRobustness checks which robust access request is tried first,
// as Rust wgpu-hal src/gles/egl.rs picks it: core on EGL 1.5+, else the
// extension when the display has it, else none.
func TestInitialRobustness(t *testing.T) {
	tests := []struct {
		name         string
		major, minor EGLInt
		exts         string
		want         robustness
	}{
		{"EGL 1.5", 1, 5, "", robustnessCore},
		{"EGL 1.5 with the extension", 1, 5, robustExts, robustnessCore},
		{"EGL 2.0", 2, 0, "", robustnessCore},
		{"EGL 1.4 with the extension", 1, 4, robustExts, robustnessExt},
		{"EGL 1.4 without it", 1, 4, "EGL_KHR_surfaceless_context", robustnessNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialRobustness(tt.major, tt.minor, tt.exts); got != tt.want {
				t.Errorf("initialRobustness = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestNextRobustness checks the retry order after a refused request: core
// falls back to the extension when the display has it, and anything else to
// no request.
func TestNextRobustness(t *testing.T) {
	tests := []struct {
		name string
		r    robustness
		exts string
		want robustness
	}{
		{"core with the extension", robustnessCore, robustExts, robustnessExt},
		{"core without it", robustnessCore, "", robustnessNone},
		{"extension", robustnessExt, robustExts, robustnessNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRobustness(tt.r, tt.exts); got != tt.want {
				t.Errorf("nextRobustness = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestIsRobustnessRefusal checks that only the errors Rust retries on lower
// the request; any other error fails context creation.
func TestIsRobustnessRefusal(t *testing.T) {
	for _, code := range []EGLInt{BadAttribute, BadMatch, BadConfig} {
		if !isRobustnessRefusal(code) {
			t.Errorf("error %#x: not a refusal, want one", code)
		}
	}
	for _, code := range []EGLInt{BadAlloc, BadDisplay, NotInitialized, Success} {
		if isRobustnessRefusal(code) {
			t.Errorf("error %#x: a refusal, want none", code)
		}
	}
}

// TestContextAttribs checks the eglCreateContext attribute lists: the robust
// access attribute of each request with EGL_TRUE, before EGL_NONE.
func TestContextAttribs(t *testing.T) {
	desktop := ContextConfig{GLVersionMajor: 3, GLVersionMinor: 3, CoreProfile: true}
	gles := ContextConfig{GLVersionMajor: 3, GLVersionMinor: 0, GLES: true}
	tests := []struct {
		name string
		cfg  ContextConfig
		r    robustness
		want []EGLInt
	}{
		{
			name: "desktop core, robust core",
			cfg:  desktop, r: robustnessCore,
			want: []EGLInt{ContextMajorVersion, 3, ContextMinorVersion, 3, ContextOpenGLProfileMask, ContextOpenGLCoreProfileBit, ContextOpenGLRobustAccess, 1, None},
		},
		{
			name: "GLES, robust extension",
			cfg:  gles, r: robustnessExt,
			want: []EGLInt{ContextMajorVersion, 3, ContextMinorVersion, 0, ContextOpenGLRobustAccessExt, 1, None},
		},
		{
			name: "GLES, no request",
			cfg:  gles, r: robustnessNone,
			want: []EGLInt{ContextMajorVersion, 3, ContextMinorVersion, 0, None},
		},
		{
			name: "debug keeps its flags",
			cfg:  ContextConfig{GLVersionMajor: 3, GLVersionMinor: 0, GLES: true, Debug: true}, r: robustnessCore,
			want: []EGLInt{ContextMajorVersion, 3, ContextMinorVersion, 0, ContextFlagsKHR, ContextOpenGLDebugBitKHR, ContextOpenGLRobustAccess, 1, None},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contextAttribs(tt.cfg, tt.r); !slices.Equal(got, tt.want) {
				t.Errorf("contextAttribs =\n %#x\nwant\n %#x", got, tt.want)
			}
		})
	}
}
