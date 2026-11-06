//go:build js && wasm

package wgpu

import (
	"syscall/js"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestConvertRenderPipelineDescriptorSpecDefaults checks the js/wasm
// conversion: zero blend components reach the browser as the WebGPU defaults
// (dstFactor "zero", not "one"), zero stencil faces as "always"/"keep", and
// an undefined depthCompare is left out for the browser to validate.
//
// It needs Node.js: GOOS=js GOARCH=wasm go test -c -o x.wasm, then
// node "$(go env GOROOT)/lib/wasm/wasm_exec_node.js" x.wasm -test.run SpecDefaults
// (or go test -exec go_js_wasm_exec where a POSIX shell runs that script).
func TestConvertRenderPipelineDescriptorSpecDefaults(t *testing.T) {
	blend := &gputypes.BlendState{}
	desc := &RenderPipelineDescriptor{
		DepthStencil: &DepthStencilState{Format: gputypes.TextureFormatDepth24PlusStencil8},
		Fragment: &FragmentState{
			Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, Blend: blend, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	}

	obj := convertRenderPipelineDescriptor(desc)

	for _, comp := range []string{"color", "alpha"} {
		c := obj.Get("fragment").Get("targets").Index(0).Get("blend").Get(comp)
		for key, want := range map[string]string{"srcFactor": "one", "dstFactor": "zero", "operation": "add"} {
			if got := c.Get(key).String(); got != want {
				t.Errorf("blend %s.%s = %q, want %q", comp, key, got, want)
			}
		}
	}
	ds := obj.Get("depthStencil")
	for _, face := range []string{"stencilFront", "stencilBack"} {
		f := ds.Get(face)
		for key, want := range map[string]string{"compare": "always", "failOp": "keep", "depthFailOp": "keep", "passOp": "keep"} {
			if got := f.Get(key).String(); got != want {
				t.Errorf("%s.%s = %q, want %q", face, key, got, want)
			}
		}
	}
	if !ds.Get("depthCompare").IsUndefined() {
		t.Errorf("depthCompare = %v, want it left out", ds.Get("depthCompare"))
	}
	if *blend != (gputypes.BlendState{}) {
		t.Error("the conversion changed the caller's blend state")
	}
}

// TestConvertTextureDescriptorZeroDefaults: a texture descriptor that leaves
// MipLevelCount, SampleCount, Dimension, Size.Height and
// Size.DepthOrArrayLayers at zero reaches the browser as the WebGPU defaults
// (1, 1, "2d", 1, 1). A member left out of the JS object is the browser's
// default, so a missing one counts as its default; an explicit 0 would be a
// validation error there.
func TestConvertTextureDescriptorZeroDefaults(t *testing.T) {
	obj := convertTextureDescriptor(&TextureDescriptor{
		Size:   Extent3D{Width: 16},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageTextureBinding,
	})
	effective := func(v js.Value) float64 {
		if v.IsUndefined() {
			return 1
		}
		return v.Float()
	}
	size := obj.Get("size")
	for name, v := range map[string]js.Value{
		"size.height": size.Get("height"), "size.depthOrArrayLayers": size.Get("depthOrArrayLayers"),
		"mipLevelCount": obj.Get("mipLevelCount"), "sampleCount": obj.Get("sampleCount"),
	} {
		if got := effective(v); got != 1 {
			t.Errorf("%s = %v, want 1", name, got)
		}
	}
	if got := size.Get("width").Float(); got != 16 {
		t.Errorf("size.width = %v, want 16", got)
	}
	if dim := obj.Get("dimension"); !dim.IsUndefined() && dim.String() != "2d" {
		t.Errorf("dimension = %q, want \"2d\"", dim.String())
	}
}

// TestConvertRenderPipelineDescriptorSampleMask: a zero sample mask reaches
// the browser as an explicit every-sample mask, any other mask unchanged.
func TestConvertRenderPipelineDescriptorSampleMask(t *testing.T) {
	for _, c := range []struct{ in, want uint64 }{{0, 0xFFFFFFFF}, {0x1, 0x1}, {0x1_0000_0003, 0x3}} {
		obj := convertRenderPipelineDescriptor(&RenderPipelineDescriptor{Multisample: gputypes.MultisampleState{Count: 1, Mask: c.in}})
		v := obj.Get("multisample").Get("mask")
		if v.IsUndefined() {
			t.Errorf("Mask %#x: mask left out", c.in)
			continue
		}
		if got := uint64(v.Float()); got != c.want {
			t.Errorf("Mask %#x: browser %#x, want %#x", c.in, got, c.want)
		}
	}
}
