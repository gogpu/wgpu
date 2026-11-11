//go:build !rust && !(js && wasm) && !(((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !wgpu_nosoftware)

package wgputest

// openSoftware reports ErrUnavailable where the Pure Go software backend is
// not in the build: outside its supported platforms, or under wgpu_nosoftware.
func openSoftware() (openedBackend, error) {
	return openedBackend{}, ErrUnavailable
}
