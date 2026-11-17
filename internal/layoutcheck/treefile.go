//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import "os"

// readTreeFile reads one file of the checked tree. Every caller passes the module root it was given plus a
// fixed relative name (the graph tool's out-dir, the golden files, the API goldens): a directory named by
// Config.Root, never a value taken from the tree itself. That is why the gosec path check does not apply,
// and why this is the package's only file read.
func readTreeFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // G304: the path is the caller-provided module root, not tree data
}

// cleanupDir removes a temporary directory. A failure leaves it to the OS temp-area cleanup and cannot
// change the result of the check, so it is deliberately not reported.
func cleanupDir(dir string) {
	_ = os.RemoveAll(dir) // best effort: a temporary graph directory
}
