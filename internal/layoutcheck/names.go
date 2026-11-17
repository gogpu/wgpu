//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package layoutcheck

import "strings"

// The vocabulary this package shares with scripts/inversion-graph: the build-context tokens, the golden
// kinds, the claims phases, the test classes, the package names and the directory prefixes the rules talk
// about. A typo in one of these strings would silently disable a rule, so each is spelled once and the
// checks and their tests name the same constants.
const (
	tokenAll        = "all"
	tokenNative     = "native"
	tokenRust       = "rust"
	tokenWasm       = "wasm"
	tokenNativeRust = "native+rust"

	// atomIntegration is the `integration` build tag: true only in the native build.
	atomIntegration = "integration"

	// sccNoneContext is scc.awk's marker for a context with zero edges: a failed scan, not a cycle.
	sccNoneContext = "(none)"

	phaseOpen  = "open"
	phaseFinal = "final"

	kindTarget   = "target"
	kindInverted = "inverted"

	kindNonTest = "N"
	kindIntTest = "INT"
	kindExtTest = "EXT"

	pkgRoot     = "wgpu"
	pkgHal      = "hal"
	pkgWgpuTest = "wgputest"

	dirInternal         = "internal"
	dirCmd              = "cmd"
	dirExamples         = "examples"
	dirInternalCore     = "internal/core"
	dirInternalInstance = "internal/instance"
	dirInternalDevice   = "internal/device"
	dirInternalHal      = "internal/hal"
	dirInternalDesc     = "internal/desc"
	dirInternalAb       = "internal/hal/allbackends"
	dirWgpuTestInternal = "wgputest/internal"
	dirWgpuTestBridge   = "internal/wgputestbridge"
)

// inTree reports whether pkg is dir itself or a package below it.
func inTree(pkg, dir string) bool {
	return pkg == dir || strings.HasPrefix(pkg, dir+"/")
}
