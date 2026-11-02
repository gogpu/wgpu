//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// checkTransferKind fails unless err is a *TransferError of the wanted kind
// and side with a non-empty message.
func checkTransferKind(t *testing.T, err error, want TransferErrorKind, side CopySide) {
	t.Helper()
	var te *TransferError
	if !errors.As(err, &te) || te.Kind != want {
		t.Fatalf("error %v (%T), want *TransferError kind %d", err, err, want)
	}
	if te.Side != side {
		t.Errorf("side = %v, want %v", te.Side, side)
	}
	if te.Error() == "" || te.Error() == "copy: unknown error" {
		t.Errorf("kind %d has no message", want)
	}
}

func colorView(usage gputypes.TextureUsage) TextureCopyView {
	return TextureCopyView{
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         usage,
		SampleCount:   1,
		MipLevelCount: 3,
		Dimension:     gputypes.TextureDimension2D,
		Size:          gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 8},
	}
}

// TestValidateBufferTextureCopyTexture pins the texture side of Rust
// wgpu-core copy_texture_to_buffer / copy_buffer_to_texture
// (wgpu-core/src/command/transfer.rs): mip level, aspect, COPY_SRC or
// COPY_DST usage, forbidden depth formats and the single-aspect rule.
func TestValidateBufferTextureCopyTexture(t *testing.T) {
	t.Parallel()
	src, dst := CopySideSource, CopySideDestination
	withFormat := func(v TextureCopyView, f gputypes.TextureFormat, a gputypes.TextureAspect) TextureCopyView {
		v.Format, v.Aspect = f, a
		return v
	}
	tests := []struct {
		name    string
		side    CopySide
		view    TextureCopyView
		want    TransferErrorKind
		wantErr bool
		// noBufferUsage gives the buffer a usage without COPY_SRC and COPY_DST.
		noBufferUsage bool
	}{
		{name: "color to buffer", side: src, view: colorView(gputypes.TextureUsageCopySrc)},
		{name: "buffer without COPY_DST", side: src, view: colorView(gputypes.TextureUsageCopySrc),
			want: TransferErrorMissingBufferUsage, wantErr: true, noBufferUsage: true},
		{name: "buffer without COPY_SRC", side: dst, view: colorView(gputypes.TextureUsageCopyDst),
			want: TransferErrorMissingBufferUsage, wantErr: true, noBufferUsage: true},
		// Rust order: the buffer usage is checked before the texture usage.
		{name: "buffer usage before texture usage", side: src, view: colorView(0),
			want: TransferErrorMissingBufferUsage, wantErr: true, noBufferUsage: true},
		{name: "buffer to color", side: dst, view: colorView(gputypes.TextureUsageCopyDst)},
		{name: "undefined aspect means all", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatRGBA8Unorm, gputypes.TextureAspectUndefined)},
		{name: "source without COPY_SRC", side: src, view: colorView(gputypes.TextureUsageCopyDst),
			want: TransferErrorMissingTextureUsage, wantErr: true},
		{name: "destination without COPY_DST", side: dst, view: colorView(gputypes.TextureUsageCopySrc),
			want: TransferErrorMissingTextureUsage, wantErr: true},
		{name: "mip level out of range", side: src, view: func() TextureCopyView {
			v := colorView(gputypes.TextureUsageCopySrc)
			v.MipLevel = 3
			return v
		}(), want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "depth aspect of a color format", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatRGBA8Unorm, gputypes.TextureAspectDepthOnly),
			want: TransferErrorInvalidTextureAspect, wantErr: true},
		{name: "stencil aspect of a depth-only format", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth32Float, gputypes.TextureAspectStencilOnly),
			want: TransferErrorInvalidTextureAspect, wantErr: true},
		{name: "depth32float to buffer", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth32Float, gputypes.TextureAspectAll)},
		{name: "stencil8 to buffer", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatStencil8, gputypes.TextureAspectAll)},
		{name: "stencil of depth24plus-stencil8 to buffer", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly)},
		{name: "depth24plus from", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth24Plus, gputypes.TextureAspectAll),
			want: TransferErrorCopyFromForbiddenTextureFormat, wantErr: true},
		{name: "depth of depth24plus-stencil8 from", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectDepthOnly),
			want: TransferErrorCopyFromForbiddenTextureFormatAspect, wantErr: true},
		{name: "depth32float to", side: dst, view: withFormat(colorView(gputypes.TextureUsageCopyDst),
			gputypes.TextureFormatDepth32Float, gputypes.TextureAspectAll),
			want: TransferErrorCopyToForbiddenTextureFormat, wantErr: true},
		{name: "depth of depth32float-stencil8 to", side: dst, view: withFormat(colorView(gputypes.TextureUsageCopyDst),
			gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly),
			want: TransferErrorCopyToForbiddenTextureFormatAspect, wantErr: true},
		{name: "stencil of depth32float-stencil8 to", side: dst, view: withFormat(colorView(gputypes.TextureUsageCopyDst),
			gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly)},
		{name: "all aspects of a combined format", side: src, view: withFormat(colorView(gputypes.TextureUsageCopySrc),
			gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectAll),
			want: TransferErrorCopyAspectNotOne, wantErr: true},
		// Rust order: mip level and aspect, then usage, then the format rules.
		{name: "mip level before usage", side: src, view: func() TextureCopyView {
			v := colorView(0)
			v.MipLevel = 9
			return v
		}(), want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "usage before forbidden format", side: src, view: withFormat(colorView(0),
			gputypes.TextureFormatDepth24Plus, gputypes.TextureAspectAll),
			want: TransferErrorMissingTextureUsage, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// The buffer has the usage the copy direction needs unless the case says otherwise.
			bufferUsage := gputypes.BufferUsageCopyDst
			if tt.side == CopySideDestination {
				bufferUsage = gputypes.BufferUsageCopySrc
			}
			if tt.noBufferUsage {
				bufferUsage = gputypes.BufferUsageVertex
			}
			err := ValidateBufferTextureCopyTexture(tt.side, tt.view, bufferUsage, gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			wantSide := CopySideSource
			switch {
			case tt.want == TransferErrorMissingTextureUsage:
				wantSide = tt.side
			case tt.want == TransferErrorMissingBufferUsage && tt.side == CopySideSource:
				wantSide = CopySideDestination // the buffer is the other side
			}
			checkTransferKind(t, err, tt.want, wantSide)
		})
	}
}

// TestValidateTextureToTextureCopy pins Rust wgpu-core
// copy_texture_to_texture (wgpu-core/src/command/transfer.rs) for what is
// checked without the copy range: mip levels, overlap inside one texture,
// all-aspects, sample counts and usage.
func TestValidateTextureToTextureCopy(t *testing.T) {
	t.Parallel()
	srcView := func() TextureCopyView { return colorView(gputypes.TextureUsageCopySrc) }
	dstView := func() TextureCopyView { return colorView(gputypes.TextureUsageCopyDst) }
	both := func() TextureCopyView {
		return colorView(gputypes.TextureUsageCopySrc | gputypes.TextureUsageCopyDst)
	}
	with := func(v TextureCopyView, f func(*TextureCopyView)) TextureCopyView {
		f(&v)
		return v
	}
	tests := []struct {
		name     string
		same     bool
		src, dst TextureCopyView
		layers   uint32
		want     TransferErrorKind
		side     CopySide
		wantErr  bool
	}{
		{name: "valid", src: srcView(), dst: dstView(), layers: 1},
		{name: "valid within one texture, other layers", same: true, layers: 2,
			src: with(both(), func(v *TextureCopyView) { v.Origin.Z = 0 }),
			dst: with(both(), func(v *TextureCopyView) { v.Origin.Z = 2 })},
		{name: "valid within one texture, other mip levels", same: true, layers: 1,
			src: with(both(), func(v *TextureCopyView) { v.MipLevel = 0 }),
			dst: with(both(), func(v *TextureCopyView) { v.MipLevel = 1 })},
		{name: "source mip level out of range", src: with(srcView(), func(v *TextureCopyView) { v.MipLevel = 5 }),
			dst: dstView(), layers: 1, want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "destination mip level out of range", src: srcView(),
			dst: with(dstView(), func(v *TextureCopyView) { v.MipLevel = 5 }), layers: 1,
			want: TransferErrorInvalidTextureMipLevel, wantErr: true},
		{name: "same subresource", same: true, src: both(), dst: both(), layers: 1,
			want: TransferErrorInvalidCopyWithinSameTexture, wantErr: true},
		{name: "overlapping layers", same: true, layers: 3,
			src:  with(both(), func(v *TextureCopyView) { v.Origin.Z = 0 }),
			dst:  with(both(), func(v *TextureCopyView) { v.Origin.Z = 2 }),
			want: TransferErrorInvalidCopyWithinSameTexture, wantErr: true},
		{name: "3D origin z spans one layer", same: true, layers: 4,
			src: with(both(), func(v *TextureCopyView) { v.Dimension = gputypes.TextureDimension3D; v.Origin.Z = 0 }),
			dst: with(both(), func(v *TextureCopyView) { v.Dimension = gputypes.TextureDimension3D; v.Origin.Z = 1 })},
		{name: "different aspects of one texture", same: true, layers: 1,
			src: with(both(), func(v *TextureCopyView) {
				v.Format, v.Aspect = gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectDepthOnly
			}),
			dst: with(both(), func(v *TextureCopyView) {
				v.Format, v.Aspect = gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureAspectStencilOnly
			}), want: TransferErrorCopySrcMissingAspects, wantErr: true},
		{name: "source names one aspect of a combined format", layers: 1,
			src: with(srcView(), func(v *TextureCopyView) {
				v.Format, v.Aspect = gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectDepthOnly
			}), dst: dstView(), want: TransferErrorCopySrcMissingAspects, wantErr: true},
		{name: "destination names one aspect of a combined format", layers: 1, src: srcView(),
			dst: with(dstView(), func(v *TextureCopyView) {
				v.Format, v.Aspect = gputypes.TextureFormatDepth32FloatStencil8, gputypes.TextureAspectStencilOnly
			}), want: TransferErrorCopyDstMissingAspects, wantErr: true},
		{name: "sample counts differ", layers: 1, src: with(srcView(), func(v *TextureCopyView) { v.SampleCount = 4 }),
			dst: dstView(), want: TransferErrorSampleCountNotEqual, wantErr: true},
		{name: "source without COPY_SRC", layers: 1, src: dstView(), dst: dstView(),
			want: TransferErrorMissingTextureUsage, side: CopySideSource, wantErr: true},
		{name: "destination without COPY_DST", layers: 1, src: srcView(), dst: srcView(),
			want: TransferErrorMissingTextureUsage, side: CopySideDestination, wantErr: true},
		// Rust order: aspects, then sample counts, then usage.
		{name: "sample counts before usage", layers: 1, src: with(dstView(), func(v *TextureCopyView) { v.SampleCount = 4 }),
			dst: srcView(), want: TransferErrorSampleCountNotEqual, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTextureToTextureCopy(tt.same, tt.src, tt.dst, gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: tt.layers})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			checkTransferKind(t, err, tt.want, tt.side)
		})
	}
}

// TestTransferErrorTextureCopyText: every texture copy kind has its own message.
func TestTransferErrorTextureCopyText(t *testing.T) {
	t.Parallel()
	seen := map[string]TransferErrorKind{}
	for _, kind := range []TransferErrorKind{
		TransferErrorMissingTextureUsage,
		TransferErrorInvalidTextureMipLevel,
		TransferErrorInvalidTextureAspect,
		TransferErrorCopyAspectNotOne,
		TransferErrorCopySrcMissingAspects,
		TransferErrorCopyDstMissingAspects,
		TransferErrorCopyFromForbiddenTextureFormat,
		TransferErrorCopyFromForbiddenTextureFormatAspect,
		TransferErrorCopyToForbiddenTextureFormat,
		TransferErrorCopyToForbiddenTextureFormatAspect,
		TransferErrorSampleCountNotEqual,
		TransferErrorInvalidCopyWithinSameTexture,
	} {
		text := (&TransferError{Kind: kind}).Error()
		if text == "copy: unknown error" {
			t.Errorf("kind %d has no message", kind)
		}
		if other, dup := seen[text]; dup {
			t.Errorf("kinds %d and %d share the message %q", other, kind, text)
		}
		seen[text] = kind
	}
}
