//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgpu

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

func init() {
	wgputestbridge.Register(newTestDeviceFactory(func() hal.Backend { return software.NewBackend() }))
}
