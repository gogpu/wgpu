// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package texcompress

import (
	"bufio"
	"encoding/hex"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

var formats = []gputypes.TextureFormat{
	gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb,
	gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb,
	gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb,
	gputypes.TextureFormatBC4RUnorm, gputypes.TextureFormatBC4RSnorm,
	gputypes.TextureFormatBC5RGUnorm, gputypes.TextureFormatBC5RGSnorm,
	gputypes.TextureFormatBC6HRGBUfloat, gputypes.TextureFormatBC6HRGBFloat,
	gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb,
	gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb,
	gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.TextureFormatETC2RGB8A1UnormSrgb,
	gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb,
	gputypes.TextureFormatEACR11Unorm, gputypes.TextureFormatEACR11Snorm,
	gputypes.TextureFormatEACRG11Unorm, gputypes.TextureFormatEACRG11Snorm,
}

// goldenBlock is one line of testdata/gpu_readback.txt: a block and the
// RGBA texels a GPU's textureLoad returned for it, row-major.
type goldenBlock struct {
	format gputypes.TextureFormat
	block  []byte
	texels [16][4]float32
}

func readGolden(t *testing.T) []goldenBlock {
	t.Helper()
	byName := map[string]gputypes.TextureFormat{}
	for _, f := range formats {
		byName[f.String()] = f
	}
	file, err := os.Open("testdata/gpu_readback.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	var golden []goldenBlock
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 1<<12), 1<<16)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2+64 {
			t.Fatalf("%d fields in %q", len(fields), line)
		}
		f, ok := byName[fields[0]]
		if !ok {
			t.Fatalf("unknown format %q", fields[0])
		}
		block, err := hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		g := goldenBlock{format: f, block: block}
		for i, s := range fields[2:] {
			bits, err := strconv.ParseUint(s, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			g.texels[i/4][i%4] = math.Float32frombits(uint32(bits))
		}
		golden = append(golden, g)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return golden
}

// srgbCode returns the 8-bit sRGB code whose IEC 61966-2-1 decode is nearest
// to linear l.
func srgbCode(l float32) int {
	v := float64(l)
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return int(math.Round(math.Max(0, math.Min(1, v)) * 255))
}

// TestDecodeGPUReadback checks every texel of every block in
// testdata/gpu_readback.txt bit for bit against Decode. The red, green and
// blue channels of the sRGB formats compare as codes: Decode returns the
// encoded value, the GPU the (approximately) decoded one.
func TestDecodeGPUReadback(t *testing.T) {
	golden := readGolden(t)
	seen := map[gputypes.TextureFormat]int{}
	for _, g := range golden {
		seen[g.format]++
		srgb := g.format.IsSrgb()
		bad := 0
		for i := range 16 {
			got, ok := Decode(g.format, g.block, i%4, i/4)
			if !ok {
				t.Fatalf("%v: not decoded", g.format)
			}
			for c := range 4 {
				want := g.texels[i][c]
				same := math.Float32bits(got[c]) == math.Float32bits(want)
				if srgb && c < 3 {
					same = int(math.Round(float64(got[c])*255)) == srgbCode(want) &&
						math.Float32bits(got[c]) == math.Float32bits(float32(math.Round(float64(got[c])*255))/255)
				}
				if !same {
					bad++
					if bad <= 4 {
						t.Errorf("%v %x texel (%d,%d) channel %d: got %v (%08x), want %v (%08x)",
							g.format, g.block, i%4, i/4, c, got[c], math.Float32bits(got[c]), want, math.Float32bits(want))
					}
				}
			}
		}
	}
	for _, f := range formats {
		if seen[f] == 0 {
			t.Errorf("%v: no golden blocks", f)
		}
		if !Decodes(f) {
			t.Errorf("Decodes(%v) = false", f)
		}
	}
}

// TestMesaS3TC checks the endpoint texels of the DXT1/3/5 blocks of
// gitlab.freedesktop.org/mesa/mesa src/util/format/u_format_tests.c (commit
// 27e63a95) against Mesa's expected values. Mesa truncates the interpolated
// palette entries where GPUs round, so only texels whose index selects an
// endpoint or alpha value Mesa and GPUs agree on are listed.
func TestMesaS3TC(t *testing.T) {
	tests := []struct {
		format gputypes.TextureFormat
		block  []byte
		x, y   int
		want   [4]uint8
	}{
		// DXT1 RGB: texel (0,1) is color0, (3,2) color1 (u_format_tests.c
		// lists rows, [y][x]); Mesa's 0x99 for color2 is 0x9a on GPUs.
		{gputypes.TextureFormatBC1RGBAUnorm, []byte{0xf2, 0xd7, 0xb0, 0x20, 0xae, 0x2c, 0x6f, 0x97}, 0, 1,
			[4]uint8{0xd6, 0xff, 0x94, 0xff}},
		{gputypes.TextureFormatBC1RGBAUnorm, []byte{0xf2, 0xd7, 0xb0, 0x20, 0xae, 0x2c, 0x6f, 0x97}, 3, 2,
			[4]uint8{0x21, 0x14, 0x84, 0xff}},
		// DXT1 RGBA: transparent black and the two endpoints.
		{gputypes.TextureFormatBC1RGBAUnorm, []byte{0xff, 0x2f, 0xa4, 0x72, 0xeb, 0xb2, 0xbd, 0xbe}, 0, 0,
			[4]uint8{0, 0, 0, 0}},
		{gputypes.TextureFormatBC1RGBAUnorm, []byte{0xff, 0x2f, 0xa4, 0x72, 0xeb, 0xb2, 0xbd, 0xbe}, 1, 1,
			[4]uint8{0x29, 0xff, 0xff, 0xff}},
		{gputypes.TextureFormatBC1RGBAUnorm, []byte{0xff, 0x2f, 0xa4, 0x72, 0xeb, 0xb2, 0xbd, 0xbe}, 0, 2,
			[4]uint8{0x73, 0x55, 0x21, 0xff}},
	}
	for _, tt := range tests {
		got, _ := Decode(tt.format, tt.block, tt.x, tt.y)
		for c := range 4 {
			if want := float32(tt.want[c]) / 255; got[c] != want {
				t.Errorf("%v %x texel (%d,%d): got %v, want %v", tt.format, tt.block, tt.x, tt.y, got, tt.want)
				break
			}
		}
	}
}

// TestDecodeUnknownFormat checks that a format without a decoder reports so.
func TestDecodeUnknownFormat(t *testing.T) {
	for _, f := range []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatASTC4x4Unorm} {
		if _, ok := Decode(f, make([]byte, 16), 0, 0); ok || Decodes(f) {
			t.Errorf("%v decoded", f)
		}
	}
}
