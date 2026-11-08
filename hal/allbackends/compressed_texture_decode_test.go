// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// texcompressGolden is the GPU readback of decodeCases that the software
// backend's decoders are checked against, here and in the unit tests of
// hal/software/internal/texcompress.
const texcompressGolden = "../software/internal/texcompress/testdata/gpu_readback.txt"

// TestCompressedTextureDecode reads every texel of the decodeCases blocks
// with textureLoad on each backend that offers the format. The software
// backend must offer texture-compression-bc and texture-compression-etc2 and
// return the committed GPU readback bit for bit. Each GPU backend must return
// what the software backend returns: bit for bit for BC6H, BC7, ETC2 and EAC,
// and within one 8-bit code of the format for the BC1-BC5 palette entries,
// which GPUs round differently (WARP and Intel differ by one code; the
// tolerances and their sources are in compressed_texture_ideal_test.go).
// Decodes the Khronos Data Format Specification leaves to the
// implementation accept each value it allows. The red, green and blue channels of the sRGB
// formats compare as 8-bit sRGB codes: the GPUs decode the sRGB curve
// approximately (Intel Iris Xe is off the IEC 61966-2-1 value by up to
// ~1e-3), the software backend exactly, and the code is what the block
// stores.
//
// WGPU_UPDATE_TEXCOMPRESS=1 rewrites the golden file from the Vulkan
// readback (a Vulkan adapter with both features is required).
func TestCompressedTextureDecode(t *testing.T) {
	update := os.Getenv("WGPU_UPDATE_TEXCOMPRESS") != ""
	optional := gputypes.Features(gputypes.FeatureTextureCompressionBC) |
		gputypes.Features(gputypes.FeatureTextureCompressionETC2)
	results := map[string]map[int][]float32{}
	adapters := map[string]string{}
	for _, b := range overrideBackends {
		t.Run(b.name, func(t *testing.T) {
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			device, name := openBackendDeviceFeatures(t, b, 0, optional, gputypes.PowerPreferenceLowPower)
			if b.software && !device.Features().Contains(gputypes.FeatureTextureCompressionBC) {
				t.Fatal("software backend does not offer texture-compression-bc")
			}
			if b.software && !device.Features().Contains(gputypes.FeatureTextureCompressionETC2) {
				t.Fatal("software backend does not offer texture-compression-etc2")
			}
			got := map[int][]float32{}
			for i, c := range decodeCases {
				if device.Features().Contains(c.feature) {
					got[i] = decodeOnDevice(t, device, c)
				}
			}
			results[b.name], adapters[b.name] = got, name
		})
	}

	if update {
		writeTexcompressGolden(t, results["vulkan"], adapters["vulkan"])
	}
	golden := readTexcompressGolden(t)
	software := results["software"]
	t.Run("software-vs-golden", func(t *testing.T) {
		if len(software) != len(decodeCases) {
			t.Fatalf("software decoded %d of %d formats", len(software), len(decodeCases))
		}
		for i, c := range decodeCases {
			compareDecode(t, c, "software", software[i], "golden", golden[i])
		}
	})
	for _, b := range overrideBackends {
		got := results[b.name]
		if b.software || len(got) == 0 {
			continue
		}
		t.Run(b.name+"-vs-software", func(t *testing.T) {
			if len(software) != len(decodeCases) {
				t.Fatal("no software readback to compare with")
			}
			for i, c := range decodeCases {
				texels, ok := got[i]
				if !ok {
					continue
				}
				if !compareBC(t, c, b.name+" ("+adapters[b.name]+")", texels, software[i]) {
					compareDecode(t, c, b.name, texels, "software", software[i])
				}
			}
			t.Logf("%s (%s) compared %d of %d formats", b.name, adapters[b.name], len(got), len(decodeCases))
		})
	}
}

// bcInterpolatedCodes is how many codes an interpolated BC1-BC5 palette
// entry may differ by between two decoders. Measured on GitHub's macOS
// runner (CI run 42444020755, Metal on "Apple Paravirtual device"):
// interpolated entries up to two codes away from WARP, Intel Iris Xe and the
// software decoder (BC1 color 155/255 where the formula's ideal is 153.6/255
// and the others give 153 or 154; BC4 unorm code 4 of endpoints 1 and 0:
// ideal 4/7, got 0.578125; BC5 snorm 0.0157). The bugs of our own upload,
// copy and format paths move endpoint texels too, and by tens of codes, so
// endpoints and constants keep their one-code or exact check. A deviation
// above one code is logged with the adapter's name.
const bcInterpolatedCodes = 2

// compareBC checks got, the texels of BC1-BC5 case c, against sw, the
// software decode, within the tolerance of each texel (idealBC), and reports
// false for any other format. An interpolated entry may differ by
// bcInterpolatedCodes; a value the specification leaves to the
// implementation may also be its other decode.
func compareBC(t *testing.T, c decodeCase, name string, got, sw []float32) bool {
	t.Helper()
	width := 4 * len(c.blocks)
	if _, ok := idealBC(c.format, c.blocks[0], 0, 0); !ok {
		return false
	}
	if len(got) != 4*width*4 || len(sw) != len(got) {
		t.Errorf("%v: %s has %d values, software %d, want %d", c.format, name, len(got), len(sw), 4*width*4)
		return true
	}
	srgb := c.format.IsSrgb()
	bad, deviations := 0, 0
	for i := range got {
		texel, ch := i/4, i%4
		x, y := texel%width, texel/width
		ideal, _ := idealBC(c.format, c.blocks[x/4], x%4, y)
		want := ideal[ch]
		g, s := float64(got[i]), float64(sw[i])
		scale := 1.0
		if srgb && ch < 3 {
			// A GPU returns the sRGB code it decoded, linearized; compare the
			// code, whose spacing is what the tolerance is written in.
			g, s, scale = float64(srgbCode(got[i])), float64(srgbCode(sw[i])), 255
		}
		within := func(v, tol float64) bool { return math.Abs(g-v) <= tol*scale+1e-9 }
		// The specification's value is always right: Metal returns the BC4/BC5
		// snorm endpoints as exactly k/127, which the software decoder (exact
		// with Intel Iris Xe) misses by 4e-6, and WARP takes the other mode of
		// a block the specification leaves open.
		if within(s, want.tol) || within(want.want*scale, want.tol) || want.hasAlt && within(want.alt*scale, want.tol) {
			continue
		}
		if want.interp && within(s, bcInterpolatedCodes*want.tol) {
			deviations++
			t.Logf("%v block %d texel (%d,%d) channel %d: %s %v, software %v, ideal %v: more than one code off",
				c.format, x/4, x%4, y, ch, name, got[i], sw[i], want.want)
			continue
		}
		bad++
		if bad <= 8 {
			t.Errorf("%v block %d texel (%d,%d) channel %d: %s %v (%08x), software %v ± %v (ideal %v)",
				c.format, x/4, x%4, y, ch, name, got[i], math.Float32bits(got[i]), sw[i], want.tol, want.want)
		}
	}
	if bad > 8 {
		t.Errorf("%v: %d values differ in all", c.format, bad)
	}
	if deviations > 0 {
		t.Logf("%v: %d interpolated values of %s more than one code off the software decode", c.format, deviations, name)
	}
	return true
}

// compareDecode reports the texels of case c where got differs from want.
func compareDecode(t *testing.T, c decodeCase, gotName string, got []float32, wantName string, want []float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%v: %s has %d values, %s %d", c.format, gotName, len(got), wantName, len(want))
		return
	}
	srgb := c.format.IsSrgb()
	width := 4 * len(c.blocks)
	bad := 0
	for i := range got {
		ch := i % 4
		g, w := got[i], want[i]
		same := math.Float32bits(g) == math.Float32bits(w)
		if srgb && ch < 3 {
			same = srgbCode(g) == srgbCode(w)
		}
		if !same && ch == 3 && bc7Reserved(c, i/4) && (g == 0 || g == 1) {
			// KDFS "BC7": a decoder given the reserved mode "should return 0
			// for all channels of all texels, but may return 1.0 in the A
			// channel".
			same = true
		}
		if same {
			continue
		}
		bad++
		if bad <= 8 {
			texel := i / 4
			x, y := texel%width, texel/width
			t.Errorf("%v block %d texel (%d,%d) channel %d: %s %v (%08x), %s %v (%08x)",
				c.format, x/4, x%4, y, ch, gotName, g, math.Float32bits(g), wantName, w, math.Float32bits(w))
		}
	}
	if bad > 8 {
		t.Errorf("%v: %d values differ in all", c.format, bad)
	}
}

// bc7Reserved reports whether texel of case c lies in a BC7 block of the
// reserved mode (a zero first byte).
func bc7Reserved(c decodeCase, texel int) bool {
	if c.format != gputypes.TextureFormatBC7RGBAUnorm && c.format != gputypes.TextureFormatBC7RGBAUnormSrgb {
		return false
	}
	width := 4 * len(c.blocks)
	return c.blocks[texel%width/4][0] == 0
}

// srgbCode returns the 8-bit sRGB code whose decode is nearest to linear l.
func srgbCode(l float32) int {
	v := float64(l)
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return int(math.Round(math.Max(0, math.Min(1, v)) * 255))
}

// readTexcompressGolden returns the golden texels of decodeCases by case
// index, and fails if the file does not hold exactly the blocks of
// decodeCases.
func readTexcompressGolden(t *testing.T) map[int][]float32 {
	t.Helper()
	f, err := os.Open(texcompressGolden)
	if err != nil {
		t.Fatalf("golden: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("golden: %v", err)
		}
	}()
	lines := map[string][]float32{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<12), 1<<16)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2+64 {
			t.Fatalf("golden: %d fields in %q", len(fields), line)
		}
		texels := make([]float32, 64)
		for i, s := range fields[2:] {
			bits, err := strconv.ParseUint(s, 16, 32)
			if err != nil {
				t.Fatalf("golden: %v", err)
			}
			texels[i] = math.Float32frombits(uint32(bits))
		}
		lines[fields[0]+" "+fields[1]] = texels
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("golden: %v", err)
	}
	golden := map[int][]float32{}
	for i, c := range decodeCases {
		width := 4 * len(c.blocks)
		all := make([]float32, width*4*4)
		for b, block := range c.blocks {
			texels, ok := lines[c.format.String()+" "+hex.EncodeToString(block)]
			if !ok {
				t.Fatalf("golden: no %v block %x", c.format, block)
			}
			for j := range 16 {
				x, y := b*4+j%4, j/4
				copy(all[(y*width+x)*4:][:4], texels[j*4:][:4])
			}
		}
		golden[i] = all
	}
	return golden
}

// writeTexcompressGolden writes the readback of decodeCases, one line per
// block: the format, the block bytes and its 16 texels (row-major) as RGBA
// float32 bits.
func writeTexcompressGolden(t *testing.T, readback map[int][]float32, adapter string) {
	t.Helper()
	if len(readback) != len(decodeCases) {
		t.Fatalf("update: the Vulkan backend decoded %d of %d formats", len(readback), len(decodeCases))
	}
	var sb strings.Builder
	sb.WriteString("# GPU readback of textureLoad over every texel of the blocks of\n")
	sb.WriteString("# hal/allbackends/compressed_texture_decode_cases_test.go. One line per block:\n")
	sb.WriteString("# the format, the block bytes, then its 16 texels in row-major order as RGBA\n")
	sb.WriteString("# float32 bits.\n")
	fmt.Fprintf(&sb, "# Source: %s, Vulkan. On the same GPU DX12 (BC) and GLES (ETC2/EAC)\n", adapter)
	sb.WriteString("# returned the same bits.\n")
	sb.WriteString("# Regenerate: WGPU_UPDATE_TEXCOMPRESS=1 go test -run TestCompressedTextureDecode ./hal/allbackends/\n")
	for i, c := range decodeCases {
		width := 4 * len(c.blocks)
		for b, block := range c.blocks {
			fmt.Fprintf(&sb, "%v %x", c.format, block)
			for j := range 16 {
				x, y := b*4+j%4, j/4
				for _, v := range readback[i][(y*width+x)*4:][:4] {
					fmt.Fprintf(&sb, " %08x", math.Float32bits(v))
				}
			}
			sb.WriteString("\n")
		}
	}
	if err := os.WriteFile(texcompressGolden, []byte(sb.String()), 0o644); err != nil {
		t.Fatalf("update: %v", err)
	}
}

const decodeWGSL = `
@group(0) @binding(0) var input: texture_2d<f32>;
@group(0) @binding(1) var<storage, read_write> output: array<vec4<f32>>;

@compute @workgroup_size(4, 4, 1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let width = textureDimensions(input).x;
    output[id.y * width + id.x] = textureLoad(input, vec2<i32>(id.xy), 0);
}
`

// decodeOnDevice uploads the blocks of c as one row of a texture of c.format,
// reads every texel with textureLoad in a compute shader and returns the
// texels as RGBA floats, row-major.
func decodeOnDevice(t *testing.T, device *wgpu.Device, c decodeCase) []float32 {
	t.Helper()
	blockBytes := int(c.format.BlockCopySize())
	width := uint32(4 * len(c.blocks))
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: width, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        c.format,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	data := make([]byte, 0, len(c.blocks)*blockBytes)
	for _, b := range c.blocks {
		data = append(data, b...)
	}
	if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex}, data,
		&wgpu.ImageDataLayout{BytesPerRow: uint32(len(data)), RowsPerImage: 1},
		&wgpu.Extent3D{Width: width, Height: 4, DepthOrArrayLayers: 1}); err != nil {
		t.Fatalf("WriteTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	n := uint64(width) * 4 * 4 // texels * channels
	output := newPublicBuffer(t, device, n*4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc)
	staging := newPublicBuffer(t, device, n*4, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	bgl := newBindGroupLayout(t, device,
		gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: wgpu.ShaderStageCompute,
			Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeUnfilterableFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
		gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: wgpu.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
	)
	pipeline := newComputePipeline(t, device, decodeWGSL, bgl)
	group := newBindGroup(t, device, bgl,
		wgpu.BindGroupEntry{Binding: 0, TextureView: view},
		wgpu.BindGroupEntry{Binding: 1, Buffer: output, Size: n * 4},
	)
	enc := newEncoder(t, device)
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, group, nil)
	pass.Dispatch(uint32(len(c.blocks)), 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc.CopyBufferToBuffer(output, 0, staging, 0, n*4)
	submit(t, device, enc)
	words := readPublicBuffer(t, staging, int(n))
	texels := make([]float32, len(words))
	for i, w := range words {
		texels[i] = math.Float32frombits(w)
	}
	return texels
}
