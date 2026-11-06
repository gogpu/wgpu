//go:build !(js && wasm)

package software

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// boundsTexel is the RGBA8 texel at x, y and z (a mip level, an array layer
// or a depth slice) of the textures in TestImageLoadRestrict: every texel of
// a texture is distinct.
func boundsTexel(x, y, z uint32) [4]byte {
	return [4]byte{byte(10 + 60*x), byte(10 + 60*y), byte(10 + 60*z), 255}
}

// imageLoadBoundsWGSL loads texels in and out of bounds of a 4x4 2D texture
// with 3 mip levels, a 2x2 2D array of 3 layers, a 2x2x2 3D texture and a
// 4-texel 1D texture.
const imageLoadBoundsWGSL = `
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var ta: texture_2d_array<f32>;
@group(0) @binding(2) var t3: texture_3d<f32>;
@group(0) @binding(3) var t1: texture_1d<f32>;
@group(0) @binding(4) var<storage, read_write> out: array<vec4<f32>>;

@compute @workgroup_size(1)
fn main() {
    out[0] = textureLoad(t, vec2<i32>(1, 2), 0);
    out[1] = textureLoad(t, vec2<i32>(7, 9), 0);
    out[2] = textureLoad(t, vec2<i32>(-1, 1), 0);
    out[3] = textureLoad(t, vec2<i32>(1, 0), 1);
    out[4] = textureLoad(t, vec2<i32>(0, 0), 5);
    out[5] = textureLoad(t, vec2<i32>(3, 3), 1);
    out[6] = textureLoad(t, vec2<i32>(0, 0), -1);
    out[7] = textureLoad(t, vec2<u32>(9u, 0u), 0u);
    out[8] = textureLoad(ta, vec2<i32>(0, 1), 7, 0);
    out[9] = textureLoad(ta, vec2<i32>(1, 1), -1, 0);
    out[10] = textureLoad(t3, vec3<i32>(5, -3, 1), 0);
    out[11] = textureLoad(t1, -2, 0);
    out[12] = textureLoad(t1, 2, 0);
}
`

// TestImageLoadRestrict checks the software backend's out-of-bounds
// textureLoad (ADR-078 rule 8). naga's Restrict policy, which Rust wgpu-hal
// uses on Vulkan without robust image access, clamps the level, the array
// layer and each coordinate with an unsigned min against the queried size
// minus one, so a negative value reads the last texel, level or layer. The
// WebGPU spec allows any texel inside the texture (WGSL "textureLoad");
// in-bounds loads must read their own texel. Before rule 8 image loads were
// Unchecked and the interpreter's own clamp read texel 0 for a negative
// value.
func TestImageLoadRestrict(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	byLevel := func(level uint32) func(x, y, _ uint32) [4]byte {
		return func(x, y, _ uint32) [4]byte { return boundsTexel(x, y, level) }
	}

	tex := newSubTexture(t, dev, gputypes.TextureDimension2D, 4, 4, 1, 3, gputypes.TextureFormatRGBA8Unorm)
	for level := range uint32(3) {
		w, h, _ := subExtent(gputypes.TextureDimension2D, 4, 4, 1, level)
		fillLevel(t, q, tex, level, 0, w, h, 1, byLevel(level))
	}
	arr := newSubTexture(t, dev, gputypes.TextureDimension2D, 2, 2, 3, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, arr, 0, 0, 2, 2, 3, boundsTexel)
	vol := newSubTexture(t, dev, gputypes.TextureDimension3D, 2, 2, 2, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, vol, 0, 0, 2, 2, 2, boundsTexel)
	line := newSubTexture(t, dev, gputypes.TextureDimension1D, 4, 1, 1, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, line, 0, 0, 4, 1, 1, boundsTexel)

	out := runSubCompute(t, dev, imageLoadBoundsWGSL, 4, 13,
		texBinding(0, newSubView(t, dev, tex, hal.TextureViewDescriptor{})),
		texBinding(1, newSubView(t, dev, arr, hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2DArray})),
		texBinding(2, newSubView(t, dev, vol, hal.TextureViewDescriptor{})),
		texBinding(3, newSubView(t, dev, line, hal.TextureViewDescriptor{})),
	)

	tests := []struct {
		what    string
		x, y, z uint32
	}{
		{"2d (1, 2) level 0, in bounds", 1, 2, 0},
		{"2d (7, 9) clamps to (3, 3)", 3, 3, 0},
		{"2d (-1, 1) clamps to (3, 1)", 3, 1, 0},
		{"2d (1, 0) level 1, in bounds", 1, 0, 1},
		{"2d level 5 clamps to level 2", 0, 0, 2},
		{"2d (3, 3) level 1 clamps to (1, 1)", 1, 1, 1},
		{"2d level -1 clamps to level 2", 0, 0, 2},
		{"2d u32 (9, 0) clamps to (3, 0)", 3, 0, 0},
		{"2d-array layer 7 clamps to layer 2", 0, 1, 2},
		{"2d-array layer -1 clamps to layer 2", 1, 1, 2},
		{"3d (5, -3, 1) clamps to (1, 1, 1)", 1, 1, 1},
		{"1d -2 clamps to 3", 3, 0, 0},
		{"1d 2, in bounds", 2, 0, 0},
	}
	for i, tt := range tests {
		checkVec4(t, out, i, fmt.Sprintf("out[%d] %s", i, tt.what), unormColor(boundsTexel(tt.x, tt.y, tt.z)))
	}
}
