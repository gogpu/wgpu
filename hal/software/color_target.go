// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package software

import (
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/raster"
	"github.com/gogpu/wgpu/hal/software/shader"
)

// colorTarget stores the fragment colors of one color attachment in the
// texture the pass renders into, in the attachment view's format: the
// color output of every draw (raster.Pipeline.SetColorOutput) and the
// target of MRT locations 1 and up.
//
// It follows the WebGPU color output rules ("Output Merging", "Blend State")
// as Vulkan and D3D12 implement them:
//   - a blended format decodes the stored texel to linear (an sRGB view
//     decodes the transfer function), blends, and encodes back;
//   - a unorm or snorm format clamps the source and the blend constant to
//     its range before blending and the result on encode; a float format
//     blends unclamped;
//   - an integer format never blends, and is written as the shader's value;
//   - the write mask keeps the stored bits of the masked-out channels.
//
// The rasterizer carries every fragment color as [4]float32 and never
// computes on it. For an integer target the four values are the bit patterns
// of the shader's 32-bit integers (math.Float32frombits, see fragmentColor),
// not their float values: a float32 holds an integer exactly only up to
// 2^24.
type colorTarget struct {
	tex           *Texture
	codec         shader.ColorCodec
	stride        int
	width, height int
	blend         raster.BlendState
	mask          gputypes.ColorWriteMask
	integer       bool
	normalized    bool
	lo, hi        float32
}

// colorKind is how a color target takes its channels from a fragment shader
// output.
type colorKind uint8

const (
	colorKindFloat colorKind = iota // float values
	colorKindUint                   // u32 bits
	colorKindSint                   // i32 bits
)

// colorKindOf returns the color kind of format.
func colorKindOf(format gputypes.TextureFormat) colorKind {
	codec, ok := shader.NewColorCodec(format)
	switch {
	case !ok || !codec.Integer():
		return colorKindFloat
	case codec.Signed():
		return colorKindSint
	}
	return colorKindUint
}

// colorKinds returns the color kind of every color attachment of the pass,
// by @location.
func (r *RenderPassEncoder) colorKinds() []colorKind {
	kinds := make([]colorKind, len(r.colorTargets))
	for i := range kinds {
		kinds[i] = colorKindOf(r.colorFormat(i))
	}
	return kinds
}

// kindAt returns kinds[loc], float for a location without an attachment.
func kindAt(kinds []colorKind, loc int) colorKind {
	if loc < len(kinds) {
		return kinds[loc]
	}
	return colorKindFloat
}

// intBitsColor carries the integer channels v as the [4]float32 the
// rasterizer passes to an integer color target.
func intBitsColor(v [4]uint32) [4]float32 {
	return [4]float32{
		math.Float32frombits(v[0]), math.Float32frombits(v[1]),
		math.Float32frombits(v[2]), math.Float32frombits(v[3]),
	}
}

// newColorTarget returns the color target of texture tex seen in format,
// with the pipeline's state for the attachment (nil: no blend, every channel
// written) and the pass's blend constant. It returns nil when the format has
// no color codec or the texture's storage is smaller than the format needs;
// the attachment then receives no writes.
func newColorTarget(tex *Texture, format gputypes.TextureFormat, state *gputypes.ColorTargetState, constant [4]float32) *colorTarget {
	if tex == nil {
		return nil
	}
	codec, ok := shader.NewColorCodec(format)
	if !ok {
		hal.Logger().Warn("software: color attachment format has no color writes", "format", format)
		return nil
	}
	c := &colorTarget{
		tex:    tex,
		codec:  codec,
		stride: codec.TexelSize(),
		width:  int(tex.width),
		height: int(tex.height),
		mask:   gputypes.ColorWriteMaskAll,
	}
	if len(tex.data) < c.width*c.height*c.stride {
		hal.Logger().Error("software: color attachment storage smaller than its view format",
			"format", format, "bytes", len(tex.data), "width", c.width, "height", c.height)
		return nil
	}
	c.lo, c.hi, c.normalized = codec.Range()
	c.integer = codec.Integer()
	if state == nil {
		return c
	}
	c.mask = state.WriteMask
	if state.Blend != nil && !c.integer {
		c.blend = convertBlendState(state.Blend)
		c.blend.Constant = constant
		if c.normalized {
			c.blend.Constant = clampColor(constant, c.lo, c.hi)
		}
	}
	return c
}

// write stores the fragment color src at pixel (x, y). It is safe for
// concurrent use (the parallel rasterizer).
func (c *colorTarget) write(x, y int, src [4]float32) {
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return
	}
	off := (y*c.width + x) * c.stride
	c.tex.mu.Lock()
	b := c.tex.data[off : off+c.stride : off+c.stride]
	if c.integer {
		c.codec.EncodeIntMasked(b, [4]uint32{
			math.Float32bits(src[0]), math.Float32bits(src[1]),
			math.Float32bits(src[2]), math.Float32bits(src[3]),
		}, c.mask)
		c.tex.mu.Unlock()
		return
	}
	if c.blend.Enabled {
		dst := c.codec.Decode(b)
		if c.normalized {
			src = clampColor(src, c.lo, c.hi)
		}
		src = raster.BlendUnclamped(src, dst, c.blend)
	}
	c.codec.EncodeMasked(b, src, c.mask)
	c.tex.mu.Unlock()
}

// discardColor is the color output of a draw whose location 0 has no
// color target.
func discardColor(int, int, [4]float32) {}

func clampColor(v [4]float32, lo, hi float32) [4]float32 {
	for i, f := range v {
		v[i] = min(max(f, lo), hi)
	}
	return v
}

// drawColorTargets returns the color target of every color attachment of
// the pass for the current pipeline, by attachment index; an entry is nil
// when the pass writes nothing for that attachment.
func (r *RenderPassEncoder) drawColorTargets() []*colorTarget {
	targets := make([]*colorTarget, len(r.colorTargets))
	var states []gputypes.ColorTargetState
	if r.pipeline != nil && r.pipeline.desc != nil && r.pipeline.desc.Fragment != nil {
		states = r.pipeline.desc.Fragment.Targets
	}
	for i, tex := range r.colorTargets {
		var state *gputypes.ColorTargetState
		if i < len(states) {
			state = &states[i]
		}
		targets[i] = newColorTarget(tex, r.colorFormat(i), state, r.blendConstant)
	}
	return targets
}

// colorFormat returns the view format of color attachment i.
func (r *RenderPassEncoder) colorFormat(i int) gputypes.TextureFormat {
	if i < len(r.colorFormats) && r.colorFormats[i] != gputypes.TextureFormatUndefined {
		return r.colorFormats[i]
	}
	if i < len(r.colorTargets) && r.colorTargets[i] != nil {
		return r.colorTargets[i].format
	}
	return gputypes.TextureFormatUndefined
}

// routeColorOutput sends the location 0 colors of pipe's draws to the first
// color target, and returns the targets for the MRT locations.
func (r *RenderPassEncoder) routeColorOutput(pipe *raster.Pipeline) []*colorTarget {
	targets := r.drawColorTargets()
	if len(targets) > 0 && targets[0] != nil {
		pipe.SetColorOutput(targets[0].write)
	} else {
		pipe.SetColorOutput(discardColor)
	}
	return targets
}

// writeMRTLocations is the raster.MRTOutput of a draw: it stores locations 1
// and up into their color targets (location 0 goes through the color
// output).
func writeMRTLocations(targets []*colorTarget) raster.MRTOutput {
	return func(x, y int, colors [][4]float32) {
		for loc := 1; loc < len(colors) && loc < len(targets); loc++ {
			if targets[loc] != nil {
				targets[loc].write(x, y, colors[loc])
			}
		}
	}
}

// clearAs fills the texture with color in format's encoding, as a render
// pass clears a color attachment viewed in that format. A format without a
// color codec is left as it is.
func (t *Texture) clearAs(format gputypes.TextureFormat, color gputypes.Color) {
	codec, ok := shader.NewColorCodec(format)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	n := codec.TexelSize()
	if len(t.data) < n {
		return
	}
	codec.EncodeClear(t.data[:n], [4]float64{color.R, color.G, color.B, color.A})
	// Fill by doubling: the storage is a whole number of texels.
	for filled := n; filled < len(t.data); filled *= 2 {
		copy(t.data[filled:], t.data[:filled])
	}
}
