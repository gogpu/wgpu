// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build swinstrcount

package shader

import "sync/atomic"

// countInstr turns on the executed-instruction counter. The counter is a
// deterministic performance proxy for the interpreter (ADR-078): unlike
// time, it does not depend on the machine, so a change in it where no
// semantics changed is a finding. Build with -tags swinstrcount to use it.
const countInstr = true

var executedInstructions atomic.Uint64

// countExecuted counts one executed instruction.
func countExecuted() { executedInstructions.Add(1) }

// ExecutedInstructions returns the number of SPIR-V instructions the
// interpreter has executed in this process. It counts only in a build with
// -tags swinstrcount and returns 0 otherwise.
func ExecutedInstructions() uint64 { return executedInstructions.Load() }
