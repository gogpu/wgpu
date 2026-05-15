//go:build js && wasm

package browser

import "syscall/js"

// Texture wraps a browser GPUTexture.
type Texture struct {
	// ref_ is the GPUTexture JavaScript object.
	ref_ js.Value

	// Cached properties to avoid repeated JS lookups.
	width  uint32
	height uint32
	format string
}

// NewTexture constructs a Texture from a GPUTexture js.Value.
func NewTexture(ref js.Value) *Texture {
	return &Texture{
		ref_:   ref,
		width:  uint32(ref.Get("width").Int()),  //nolint:gosec // JS API returns safe integers
		height: uint32(ref.Get("height").Int()), //nolint:gosec // JS API returns safe integers
		format: ref.Get("format").String(),
	}
}

// Ref returns the underlying GPUTexture js.Value.
func (t *Texture) Ref() js.Value { return t.ref_ }

// Width returns the texture width in pixels.
func (t *Texture) Width() uint32 { return t.width }

// Height returns the texture height in pixels.
func (t *Texture) Height() uint32 { return t.height }

// Format returns the texture format as a WebGPU string (e.g. "rgba8unorm").
func (t *Texture) Format() string { return t.format }

// CreateView calls GPUTexture.createView() with the given descriptor.
// Pass js.Undefined() for default view parameters.
func (t *Texture) CreateView(desc js.Value) *TextureView {
	var jsView js.Value
	if desc.IsUndefined() || desc.IsNull() {
		jsView = t.ref_.Call("createView")
	} else {
		jsView = t.ref_.Call("createView", desc)
	}
	return &TextureView{ref_: jsView}
}

// Destroy calls GPUTexture.destroy() to release GPU memory.
func (t *Texture) Destroy() {
	destroy := t.ref_.Get("destroy")
	if !destroy.IsUndefined() && !destroy.IsNull() {
		t.ref_.Call("destroy")
	}
}

// TextureView wraps a browser GPUTextureView.
type TextureView struct {
	// ref_ is the GPUTextureView JavaScript object.
	ref_ js.Value
}

// Ref returns the underlying GPUTextureView js.Value.
func (v *TextureView) Ref() js.Value { return v.ref_ }
