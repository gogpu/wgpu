//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgputest

import "github.com/gogpu/wgpu/hal/software"

// openSoftware opens a private Pure Go software instance. Importing the
// software package registers it in the global HAL registry, which is correct
// here: a test binary that asks for the software device explicitly.
func openSoftware() (openedBackend, error) {
	return openBackend(software.NewBackend())
}
