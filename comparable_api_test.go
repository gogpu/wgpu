//go:build !rust && !(js && wasm)

package wgpu_test

import "github.com/gogpu/wgpu"

// The public resource types are comparable, so a value can be a map key or be
// compared with ==. Adding a slice, map or func field to one of them is an
// incompatible API change (apidiff: "old is comparable, new is not"); keep
// such state behind a pointer instead. These map types fail to compile if a
// type stops being comparable.
var (
	_ map[wgpu.Adapter]struct{}
	_ map[wgpu.Device]struct{}
	_ map[wgpu.Texture]struct{}
	_ map[wgpu.ShaderModule]struct{}
	_ map[wgpu.RenderBundle]struct{}
	_ map[wgpu.RenderBundleEncoder]struct{}
)
