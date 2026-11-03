//go:build !(js && wasm)

package software

import (
	"bytes"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestSoftwareComputeStorageTextureWrite dispatches a compute shader that
// stores a texel into every pixel of an rgba8unorm storage texture and checks
// the texture's bytes: the dispatch must bind the texture view and the
// interpreter must execute OpImageWrite into the texture's storage
// (P1-SW-018).
func TestSoftwareComputeStorageTextureWrite(t *testing.T) {
	const wgsl = `
@group(0) @binding(0) var output: texture_storage_2d<rgba8unorm, write>;

@compute @workgroup_size(4, 4)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    textureStore(output, vec2<i32>(id.xy), vec4<f32>(0.25, 1.0, 0.0, 1.0));
}
`
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "store", Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer dev.DestroyShaderModule(sm)
	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Compute: hal.ComputeState{Module: sm, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	defer dev.DestroyComputePipeline(pipeline)

	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageStorageBinding | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer dev.DestroyTexture(tex)
	view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()}}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer dev.DestroyBindGroup(bg)

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	pass.End()

	data := tex.(*Texture).GetData()
	want := []byte{64, 255, 0, 255} // 0.25 * 255 rounds to 64
	for i := 0; i < 16; i++ {
		got := data[i*4 : i*4+4]
		if !bytes.Equal(got, want) {
			t.Fatalf("texel %d = %v, want %v", i, got, want)
		}
	}
}
