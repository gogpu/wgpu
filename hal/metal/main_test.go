// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"os"
	"strings"
	"testing"
)

// skipWithoutMetal skips a test that needs a Metal device this machine does
// not have. A runner whose WGPU_REQUIRE lists metal fails the test instead,
// so a missing device cannot pass silently there (the ecosystem's
// <REPO>_REQUIRE switch). The tests that need no device run everywhere.
func skipWithoutMetal(tb testing.TB, reason string) {
	tb.Helper()
	if requireSwitch("metal") {
		tb.Fatalf("%s, but WGPU_REQUIRE=%q requires Metal", reason, os.Getenv("WGPU_REQUIRE"))
	}
	tb.Skip(reason)
}

// requireSwitch reports whether the comma-separated WGPU_REQUIRE lists name.
func requireSwitch(name string) bool {
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.TrimSpace(item) == name {
			return true
		}
	}
	return false
}
