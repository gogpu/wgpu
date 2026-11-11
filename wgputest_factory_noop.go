//go:build !((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm)

package wgpu

import "github.com/gogpu/wgpu/internal/wgputestbridge"

// Outside the platforms where the Pure Go backends build, the noop backend
// still builds and keeps wgputest.NewNoopDevice available; NewSoftwareDevice
// reports ErrUnavailable.
func init() {
	wgputestbridge.Register(newTestDeviceFactory(nil))
}
