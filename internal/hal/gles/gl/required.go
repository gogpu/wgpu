//go:build (windows || linux) && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

import "fmt"

// requiredProc is a GL entry point the backend calls without a nil check.
//
// Every one of them is core in OpenGL 3.3 and OpenGL ES 3.0, the minimum
// versions the backend accepts. Rust wgpu-hal relies on its version gate alone
// (gles/adapter.rs expose; glow loads every proc those versions guarantee). Our
// wrappers call through the raw pointers — on Windows syscall.SyscallN(0, …)
// is an access violation, on Linux goffi returns zeros — so a context that
// lacks one of them, such as Windows' GDI generic OpenGL 1.1 implementation
// (the windows-latest CI runner crashed in glGenVertexArrays), must fail Load
// instead. Entry points that are optional and checked before use (sync objects,
// samplers, queries, compute, buffer mapping, framebuffer blits,
// buffer-to-buffer copies) are not listed.
type requiredProc[P comparable] struct {
	name string
	addr P
}

// missingProcs returns the names of the required entry points that were not
// loaded.
func missingProcs[P comparable](procs []requiredProc[P]) []string {
	var zero P
	var missing []string
	for _, p := range procs {
		if p.addr == zero {
			missing = append(missing, p.name)
		}
	}
	return missing
}

// checkRequiredProcs returns an error naming every missing required entry point.
func checkRequiredProcs[P comparable](procs []requiredProc[P]) error {
	if missing := missingProcs(procs); len(missing) > 0 {
		return fmt.Errorf("gl: context lacks entry points required by OpenGL 3.3 / OpenGL ES 3.0: %v", missing)
	}
	return nil
}
