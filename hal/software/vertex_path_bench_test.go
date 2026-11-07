//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// smallTrianglesWGSL draws a 32x32 grid of small triangles, one per cell,
// from vertex_index, each with its own color: the per-triangle cost of the
// vertex path (shading, primitive assembly, the viewport transform, clipping)
// outweighs their few fragments.
const smallTrianglesWGSL = `
struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) color: vec4f,
}

@vertex
fn vs_main(@builtin(vertex_index) i: u32) -> VOut {
    let tri = i / 3u;
    let cell = vec2f(f32(tri % 32u), f32(tri / 32u)) / 16.0 - vec2f(1.0);
    var corner = array<vec2f, 3>(vec2f(0.0, 0.0), vec2f(0.05, 0.0), vec2f(0.0, 0.05));
    let p = cell + corner[i % 3u];
    return VOut(vec4f(p, 0.5, 1.0), vec4f(cell * 0.5 + vec2f(0.5), 0.25, 1.0));
}

@fragment
fn fs_main(in: VOut) -> @location(0) vec4f {
    return in.color;
}
`

// BenchmarkSmallTrianglesDraw draws 1024 small triangles into a 128x128
// target through the SPIR-V vertex path.
func BenchmarkSmallTrianglesDraw(b *testing.B) {
	backend := NewBackend()
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	openDev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer openDev.Device.Destroy()
	dev := openDev.Device.(*Device)

	const size = 128
	_, dstView := newShaderTexture(b, dev, size, size, gputypes.TextureFormatRGBA8Unorm, [4]byte{})
	s := &shaderDraw{
		dev:       dev,
		pipeline:  newShaderPipeline(b, dev, smallTrianglesWGSL, gputypes.TextureFormatRGBA8Unorm, nil),
		dstView:   dstView,
		vertCount: 3 * 32 * 32,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		s.draw(b)
	}
}
