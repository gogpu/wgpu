//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgputest

import "github.com/gogpu/wgpu/hal/software"

// openSoftware opens a private Pure Go software instance.
//
// Importing hal/software also registers the software backend in the global HAL
// registry: that is a side effect of this file's import, not of calling the
// constructor. A test binary that imports wgputest therefore has software
// registered for every code path, even when it only calls NewNoopDevice, and
// until v0.35.0 the BackendEmpty rule admits software whatever mask a request
// names (ADR-083 2.3). The package doc states this.
func openSoftware() (openedBackend, error) {
	return openBackend(software.NewBackend())
}
