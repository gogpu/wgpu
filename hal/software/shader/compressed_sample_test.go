//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/software/internal/texcompress"
)

// compressedFormats are the block-compressed formats the software backend
// samples (texture-compression-bc and texture-compression-etc2).
var compressedFormats = []gputypes.TextureFormat{
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

// compressedView builds a 2D-array view of format f, 12x8 texels, 2 layers
// and 3 mip levels (12x8, 6x4 and 3x2: the last is one partial block), as
// the software backend stores it: whole 4x4 blocks, row-major per image,
// images back to back. The blocks are random. It also returns the same
// texels decoded into an RGBA32Float view, addressed by the test itself:
// block (x/4, y/4) of image z, texel (x%4, y%4), sRGB converted to linear.
func compressedView(t testing.TB, f gputypes.TextureFormat, seed uint64) (compressed, decoded *Texture2D) {
	t.Helper()
	const w, h, layers, mips = 12, 8, 2, 3
	size := int(f.BlockCopySize())
	rng := rand.New(rand.NewPCG(seed, uint64(f)))
	compressed = &Texture2D{Width: w, Height: h, Format: uint32(f), BytesPerPixel: uint32(size),
		MipLevels: mips, Layers: layers, Dimension: TexDim2DArray}
	decoded = &Texture2D{Width: w, Height: h, Format: uint32(gputypes.TextureFormatRGBA32Float), BytesPerPixel: 16,
		MipLevels: mips, Layers: layers, Dimension: TexDim2DArray}
	for level := range mips {
		lw, lh := max(w>>level, 1), max(h>>level, 1)
		bw, bh := (lw+3)/4, (lh+3)/4
		blocks := make([]byte, bw*bh*layers*size)
		for i := range blocks {
			blocks[i] = byte(rng.Uint32())
		}
		texels := make([]byte, 0, lw*lh*layers*16)
		for z := range layers {
			for y := range lh {
				for x := range lw {
					off := ((z*bh+y/4)*bw + x/4) * size
					v, ok := texcompress.Decode(f, blocks[off:off+size], x%4, y%4)
					if !ok {
						t.Fatalf("%v: Decode not ok", f)
					}
					if f.IsSrgb() {
						for c := range 3 {
							v[c] = srgbToLinear8[uint8(v[c]*255+0.5)]
						}
					}
					for _, c := range v {
						texels = binary.LittleEndian.AppendUint32(texels, math.Float32bits(c))
					}
				}
			}
		}
		compressed.Levels = append(compressed.Levels, TextureLevel{Width: uint32(lw), Height: uint32(lh), Images: layers, Data: blocks})
		decoded.Levels = append(decoded.Levels, TextureLevel{Width: uint32(lw), Height: uint32(lh), Images: layers, Data: texels})
	}
	compressed.Data, decoded.Data = compressed.Levels[0].Data, decoded.Levels[0].Data
	return compressed, decoded
}

// BenchmarkSampleCompressed is one bilinear sample of level 0 (four texel
// fetches, each decoding its texel from its block) of BC1 and BC7, against
// the same sample of the texels stored decoded as RGBA32Float.
func BenchmarkSampleCompressed(b *testing.B) {
	samp := &Sampler{MinFilter: 1, MagFilter: 1, LodMaxClamp: LodClampNone}
	for _, f := range []gputypes.TextureFormat{gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC7RGBAUnorm} {
		comp, dec := compressedView(b, f, 1)
		for _, tc := range []struct {
			name string
			tex  *Texture2D
		}{{f.String(), comp}, {f.String() + "-decoded-rgba32float", dec}} {
			b.Run(tc.name, func(b *testing.B) {
				b.ReportAllocs()
				var sink Vec4
				i := 0
				for b.Loop() {
					// Footprints straddling the block boundary at x = 4.
					c := Vec4{(3.6 + float32(i&7)*0.1) / 12, 0.3, 0, 0}
					sink = sampleView(tc.tex, samp, c, -1, [3]int{})
					i++
				}
				_ = sink
			})
		}
	}
}

// sameVec4 reports whether a and b are equal channel by channel, a NaN
// equal to a NaN.
func sameVec4(a, b Vec4) bool {
	for i := range a {
		if a[i] != b[i] && !(math.IsNaN(float64(a[i])) && math.IsNaN(float64(b[i]))) {
			return false
		}
	}
	return true
}

// TestSampleCompressedMatchesDecoded samples every block-compressed format
// through the sampled-texel path the interpreter uses (sampleView: filters,
// mip selection and blending, array layers, wrapping) and checks that each
// result equals the sample of the same texels stored decoded as
// RGBA32Float. The coordinates step by fractions of a texel, so linear
// footprints straddle block boundaries (x = 3|4, 7|8, y = 3|4) and the
// repeat wrap reads the block at the opposite edge; the levels of detail
// reach every mip level, including the 3x2 one that is a single partial
// block, and blend between levels. Every texel of every level and layer is
// also read through imageAt and decodeTexel, the textureLoad path.
func TestSampleCompressedMatchesDecoded(t *testing.T) {
	samplers := []Sampler{
		{MinFilter: 1, MagFilter: 1, MipmapFilter: 1, LodMaxClamp: LodClampNone},
		{MinFilter: 0, MagFilter: 0, MipmapFilter: 0, WrapU: 1, WrapV: 1, LodMaxClamp: LodClampNone},
		{MinFilter: 1, MagFilter: 1, MipmapFilter: 0, WrapU: 2, WrapV: 2, LodMaxClamp: LodClampNone},
	}
	lambdas := []float32{-1, 0, 0.4, 1, 1.7, 2, 5}
	for i, f := range compressedFormats {
		t.Run(f.String(), func(t *testing.T) {
			comp, dec := compressedView(t, f, uint64(i)+1)
			for l := range 3 {
				lc, ld := comp.Levels[l], dec.Levels[l]
				for z := range 2 {
					ci, _ := comp.imageAt(lc, z)
					di, _ := dec.imageAt(ld, z)
					for y := range int(lc.Height) {
						for x := range int(lc.Width) {
							got, ok := decodeTexel(&ci, x, y)
							want, _ := decodeTexel(&di, x, y)
							if !ok || !sameVec4(got, want) {
								t.Fatalf("load level %d layer %d (%d, %d) = %v, %v; want %v", l, z, x, y, got, ok, want)
							}
						}
					}
				}
			}
			for si := range samplers {
				samp := &samplers[si]
				for _, lambda := range lambdas {
					for layer := range 2 {
						for v := float32(-0.05); v < 1.05; v += 1.0 / 28 {
							for u := float32(-0.05); u < 1.05; u += 1.0 / 44 {
								c := Vec4{u, v, float32(layer), 0}
								got := sampleView(comp, samp, c, lambda, [3]int{})
								want := sampleView(dec, samp, c, lambda, [3]int{})
								if !sameVec4(got, want) {
									t.Fatalf("sampler %d lambda %v at %v = %v; want %v", si, lambda, c, got, want)
								}
							}
						}
					}
				}
			}
		})
	}
}
