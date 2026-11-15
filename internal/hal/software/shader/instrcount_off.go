// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !swinstrcount

package shader

// countInstr is false without -tags swinstrcount: the counter in run()
// compiles away (instrcount_on.go).
const countInstr = false

// countExecuted does nothing without -tags swinstrcount.
func countExecuted() {}

// ExecutedInstructions returns the number of SPIR-V instructions the
// interpreter has executed in this process. It counts only in a build with
// -tags swinstrcount and returns 0 otherwise.
func ExecutedInstructions() uint64 { return 0 }
