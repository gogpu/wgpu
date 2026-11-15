package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

func init() {
	wgputestbridge.RegisterTextureFactory(newDetachedTextureFactory)
}

// newDetachedTextureFactory builds the metadata-only texture behind the public
// wgputest.NewDetachedTexture. It is registered in every build, so the
// constructor exists on every platform; desc is a *TextureDescriptor or nil.
//
// The texture has no device and no backend object: Release on it is a no-op,
// and every operation that needs the GPU reports the validation error wgpu
// reports for a texture without a HAL handle. It replaces the hand-written
// hal.Texture stubs consumers built through NewTextureFromHAL before the HAL
// becomes internal (ADR-070).
func newDetachedTextureFactory(desc any) (any, error) {
	var d *TextureDescriptor
	if desc != nil {
		typed, ok := desc.(*TextureDescriptor)
		if !ok {
			return nil, fmt.Errorf("wgpu: detached texture: descriptor is %T, want *wgpu.TextureDescriptor", desc)
		}
		d = typed
	}
	return newDetachedTexture(d), nil
}
