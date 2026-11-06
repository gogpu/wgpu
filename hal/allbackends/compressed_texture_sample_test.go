// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// compressedSize is the width and height of the compressed textures below:
// 2x2 blocks of 4x4 texels.
const compressedSize = 8

// compressedFormat is a block-compressed format of 4x4 blocks with one solid
// block per color it is tested with: blocks[i] holds the bytes of a block
// every texel of which decodes to colors[i], exactly.
type compressedFormat struct {
	format     gputypes.TextureFormat
	feature    gputypes.Feature
	blockBytes int
	blocks     [][]byte
	colors     [][4]byte
}

// bc1Block is a BC1 block whose two endpoints are both the RGB565 color c and
// whose 2-bit indices are all 0, so every texel is endpoint 0 at alpha 1.
func bc1Block(c uint16) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint16(b[0:], c)
	binary.LittleEndian.PutUint16(b[2:], c)
	return b
}

// astcVoidExtentBlock is an ASTC LDR void-extent block: block mode 0x1FC,
// the reserved bits set, every extent coordinate all ones (the whole texture)
// and one UNORM16 color, RGBA, that every texel takes.
func astcVoidExtentBlock(r, g, b, a uint16) []byte {
	block := []byte{0xFC, 0xFD, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0}
	for i, c := range []uint16{r, g, b, a} {
		binary.LittleEndian.PutUint16(block[8+2*i:], c)
	}
	return block
}

// compressedFormats holds a BC1, an ETC2 and an ASTC format. The BC1 colors
// use the full 5- and 6-bit endpoint ranges, which expand to 0 and 255. The
// ETC2 blocks are individual-mode blocks with base color 15 (255) or 0 and
// every texel at the table's largest step, +8 or -8, which clamps to 255 or
// 0. The ASTC colors are 0 or 0xFFFF, which decode to 0 or 255.
var compressedFormats = []compressedFormat{
	{
		format:     gputypes.TextureFormatBC1RGBAUnorm,
		feature:    gputypes.FeatureTextureCompressionBC,
		blockBytes: 8,
		blocks:     [][]byte{bc1Block(0xF800), bc1Block(0x07E0), bc1Block(0x001F), bc1Block(0xFFFF)},
		colors:     [][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}},
	},
	{
		format:     gputypes.TextureFormatETC2RGB8Unorm,
		feature:    gputypes.FeatureTextureCompressionETC2,
		blockBytes: 8,
		blocks: [][]byte{
			{0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0xFF, 0xFF},
			{0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF},
			{0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF},
			{0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0xFF, 0xFF},
		},
		colors: [][4]byte{{255, 255, 255, 255}, {0, 0, 0, 255}, {0, 0, 0, 255}, {255, 255, 255, 255}},
	},
	{
		format:     gputypes.TextureFormatASTC4x4Unorm,
		feature:    gputypes.FeatureTextureCompressionASTC,
		blockBytes: 16,
		blocks: [][]byte{
			astcVoidExtentBlock(0xFFFF, 0, 0, 0xFFFF), astcVoidExtentBlock(0, 0xFFFF, 0, 0xFFFF),
			astcVoidExtentBlock(0, 0, 0xFFFF, 0xFFFF), astcVoidExtentBlock(0xFFFF, 0xFFFF, 0xFFFF, 0),
		},
		colors: [][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 0}},
	},
}

// TestCompressedTextureSample uploads 2x2 solid blocks to a BC1 and an ETC2
// texture, samples them with textureLoad in a render pass and compares every
// texel against the decoded block color. The blocks are written by
// Queue.WriteTexture with tight rows and by CopyBufferToTexture with rows 256
// bytes apart, and into layer 1 of a 2D array, so the upload of whole images,
// of padded block rows and of array layers is covered. A device without the
// format's feature skips it: the feature is advertised only where the backend
// can allocate and upload the format.
func TestCompressedTextureSample(t *testing.T) {
	var optional gputypes.Features
	for _, f := range compressedFormats {
		optional |= gputypes.Features(f.feature)
	}
	forEachOverrideBackendFeatures(t, 0, optional, func(t *testing.T, device *wgpu.Device) {
		for _, f := range compressedFormats {
			t.Run(f.format.String(), func(t *testing.T) {
				if !device.Features().Contains(f.feature) {
					t.Skipf("device lacks %v", f.feature)
				}
				t.Run("write-texture", func(t *testing.T) { sampleCompressed(t, device, f, 1, uploadCompressedWrite) })
				t.Run("copy-buffer", func(t *testing.T) { sampleCompressed(t, device, f, 1, uploadCompressedCopy) })
				t.Run("array-layer", func(t *testing.T) { sampleCompressed(t, device, f, 2, uploadCompressedWrite) })
			})
		}
	})
}

// compressedImage returns the blocks of f as one image, block rows bytesPerRow
// apart, and the image repeated layers times, rowsPerImage block rows apart.
// Layer 0 of a layered texture holds the blocks in reverse order, so the test
// fails if it samples the wrong layer.
func compressedImage(f compressedFormat, layers int, bytesPerRow int) []byte {
	const blockRows = compressedSize / 4
	data := make([]byte, layers*blockRows*bytesPerRow)
	for layer := 0; layer < layers; layer++ {
		for i := range f.blocks {
			block := f.blocks[i]
			if layer < layers-1 {
				block = f.blocks[len(f.blocks)-1-i]
			}
			row, col := i/2, i%2
			copy(data[(layer*blockRows+row)*bytesPerRow+col*f.blockBytes:], block)
		}
	}
	return data
}

type compressedUpload func(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, f compressedFormat, layers int)

// uploadCompressedWrite writes the blocks with Queue.WriteTexture, rows
// tightly packed.
func uploadCompressedWrite(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, f compressedFormat, layers int) {
	t.Helper()
	bytesPerRow := compressedSize / 4 * f.blockBytes
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, compressedImage(f, layers, bytesPerRow),
		&wgpu.ImageDataLayout{BytesPerRow: uint32(bytesPerRow), RowsPerImage: compressedSize / 4},
		&wgpu.Extent3D{Width: compressedSize, Height: compressedSize, DepthOrArrayLayers: uint32(layers)}); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
}

// uploadCompressedCopy copies the blocks from a buffer whose block rows are
// 256 bytes apart, more than the 16 bytes a row of two blocks needs.
func uploadCompressedCopy(t *testing.T, device *wgpu.Device, tex *wgpu.Texture, f compressedFormat, layers int) {
	t.Helper()
	const bytesPerRow = 256
	data := compressedImage(f, layers, bytesPerRow)
	src := newPublicBuffer(t, device, uint64(len(data)), wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(src, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	submitCopy(t, device, func(enc *wgpu.CommandEncoder) {
		enc.CopyBufferToTexture(src, tex, []wgpu.BufferTextureCopy{{
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: compressedSize / 4},
			TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
			Size:         wgpu.Extent3D{Width: compressedSize, Height: compressedSize, DepthOrArrayLayers: uint32(layers)},
		}})
	})
}

// sampleCompressed creates a texture of f with layers layers, fills it with
// upload, samples its last layer and checks every texel.
func sampleCompressed(t *testing.T, device *wgpu.Device, f compressedFormat, layers int, upload compressedUpload) {
	t.Helper()
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: compressedSize, Height: compressedSize, DepthOrArrayLayers: uint32(layers)},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        f.format,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	upload(t, device, tex, f, layers)

	dim := gputypes.TextureViewDimension2D
	if layers > 1 {
		dim = gputypes.TextureViewDimension2DArray
	}
	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Format: f.format, Dimension: dim, Aspect: gputypes.TextureAspectAll,
		MipLevelCount: 1, ArrayLayerCount: uint32(layers),
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	enc := newEncoder(t, device)
	staging := renderCompressedView(t, device, enc, view, dim, layers-1)
	submit(t, device, enc)
	checkCompressedTexels(t, staging, f)
}

const sampleArrayLayerWGSL = `
@group(0) @binding(0) var input: texture_2d_array<f32>;
@group(0) @binding(1) var<uniform> layer: vec4<u32>;

@vertex
fn vs_main(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
    let uv = vec2<f32>(f32((index << 1u) & 2u), f32(index & 2u));
    return vec4<f32>(uv * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn fs_main(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
    return textureLoad(input, vec2<i32>(position.xy), i32(layer.x), 0);
}
`

// renderCompressedView records a render pass that copies layer of view into
// an 8x8 RGBA8 target with textureLoad, then a copy of the target into a
// staging buffer with rows 256 bytes apart, and returns the staging buffer.
func renderCompressedView(t *testing.T, device *wgpu.Device, enc *wgpu.CommandEncoder, view *wgpu.TextureView,
	dim gputypes.TextureViewDimension, layer int) *wgpu.Buffer {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: compressedSize, Height: compressedSize, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	targetView, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(targetView.Release)
	staging := newPublicBuffer(t, device, 256*compressedSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)

	entries := []gputypes.BindGroupLayoutEntry{{
		Binding:    0,
		Visibility: wgpu.ShaderStageFragment,
		Texture:    &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: dim},
	}}
	groupEntries := []wgpu.BindGroupEntry{{Binding: 0, TextureView: view}}
	wgsl := sampleWGSL
	if dim == gputypes.TextureViewDimension2DArray {
		wgsl = sampleArrayLayerWGSL
		uniform := newPublicBuffer(t, device, 16, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
		var words [16]byte
		binary.LittleEndian.PutUint32(words[:], uint32(layer))
		if err := device.Queue().WriteBuffer(uniform, 0, words[:]); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		entries = append(entries, gputypes.BindGroupLayoutEntry{
			Binding:    1,
			Visibility: wgpu.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
		})
		groupEntries = append(groupEntries, wgpu.BindGroupEntry{Binding: 1, Buffer: uniform, Size: 16})
	}
	bgl := newBindGroupLayout(t, device, entries...)
	group := newBindGroup(t, device, bgl, groupEntries...)
	pipeline := newSamplePipeline(t, device, wgsl, bgl)

	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0, B: 1, A: 0},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: compressedSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: compressedSize, Height: compressedSize, DepthOrArrayLayers: 1},
	}})
	return staging
}

// newSamplePipeline creates a render pipeline drawing a full-screen triangle
// with the vs_main and fs_main of wgsl into an RGBA8 target.
func newSamplePipeline(t *testing.T, device *wgpu.Device, wgsl string, bgl *wgpu.BindGroupLayout) *wgpu.RenderPipeline {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "sample-compressed", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout:      layout,
		Vertex:      wgpu.VertexState{Module: shader, EntryPoint: "vs_main"},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.DefaultMultisampleState(),
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// checkCompressedTexels compares every texel of the staging copy against the
// color of the block it lies in.
func checkCompressedTexels(t *testing.T, staging *wgpu.Buffer, f compressedFormat) {
	t.Helper()
	words := readPublicBuffer(t, staging, 256*compressedSize/4)
	bad := 0
	for y := 0; y < compressedSize; y++ {
		for x := 0; x < compressedSize; x++ {
			var got [4]byte
			binary.LittleEndian.PutUint32(got[:], words[(y*256+x*4)/4])
			want := f.colors[(y/4)*2+x/4]
			if got != want {
				if bad < 4 {
					t.Errorf("texel (%d, %d) = %v, want %v", x, y, got, want)
				}
				bad++
			}
		}
	}
	if bad > 4 {
		t.Errorf("%d of %d texels differ", bad, compressedSize*compressedSize)
	}
}
