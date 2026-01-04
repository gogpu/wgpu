// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Skip on CI - Metal is not available on GitHub Actions macOS runners
	// due to Apple Virtualization Framework limitations.
	// See: https://github.com/actions/runner-images/discussions/6138
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		os.Exit(0)
	}

	os.Exit(m.Run())
}
