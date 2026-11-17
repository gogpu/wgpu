//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package browser

// Hermetic unit tests for the browser error-scope mappings (stage A4): the
// filter→JS-name table and the JS-error-name→category table are pure and run
// under node in CI; the live GPU path is exercised only in a real browser and
// is a documented limit, not claimed as covered here.

import "testing"

func TestFilterToJSName(t *testing.T) {
	cases := []struct {
		filter ErrorFilter
		want   string
		ok     bool
	}{
		{ErrorFilterValidation, "validation", true},
		{ErrorFilterOutOfMemory, "out-of-memory", true},
		{ErrorFilterInternal, "internal", true},
		{ErrorFilter(42), "", false},
		{ErrorFilter(-1), "", false},
	}
	for _, c := range cases {
		got, ok := FilterToJSName(c.filter)
		if got != c.want || ok != c.ok {
			t.Errorf("FilterToJSName(%d) = (%q, %v), want (%q, %v)", int(c.filter), got, ok, c.want, c.ok)
		}
	}
}

func TestErrorNameToFilter(t *testing.T) {
	cases := []struct {
		name string
		want ErrorFilter
		ok   bool
	}{
		{"GPUValidationError", ErrorFilterValidation, true},
		{"GPUOutOfMemoryError", ErrorFilterOutOfMemory, true},
		{"GPUInternalError", ErrorFilterInternal, true},
		{"OperationError", 0, false},
		{"TypeError", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := ErrorNameToFilter(c.name)
		if got != c.want || ok != c.ok {
			t.Errorf("ErrorNameToFilter(%q) = (%d, %v), want (%d, %v)", c.name, int(got), ok, int(c.want), c.ok)
		}
	}
}

func TestErrErrorScopeEmptyIsDefined(t *testing.T) {
	if ErrErrorScopeEmpty == nil {
		t.Fatal("ErrErrorScopeEmpty must be a defined sentinel the root package can bind")
	}
}
