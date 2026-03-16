package software

import (
	"encoding/binary"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/software/raster"
)

// executeDraw is the core draw implementation.
// It selects between fullscreen texture blit and vertex-buffer-based rasterization.
func (r *RenderPassEncoder) executeDraw(vertexCount, firstVertex uint32) {
	if r.pipeline == nil {
		return
	}

	target := r.getTargetTexture()
	if target == nil {
		return
	}

	// Apply pending clear before first draw (WebGPU spec).
	if !r.cleared {
		r.applyClear()
	}

	// If no vertex buffer is bound, try fullscreen texture blit.
	if r.vertexBufs[0].buffer == nil {
		r.executeFullscreenBlit(target)
		return
	}

	// Vertex-buffer-based rendering with the raster pipeline.
	r.executeVertexDraw(target, vertexCount, firstVertex)
}

// getTargetTexture returns the texture backing the first color attachment.
func (r *RenderPassEncoder) getTargetTexture() *Texture {
	if len(r.desc.ColorAttachments) == 0 {
		return nil
	}
	view, ok := r.desc.ColorAttachments[0].View.(*TextureView)
	if !ok || view.texture == nil {
		return nil
	}
	return view.texture
}

// executeFullscreenBlit blits the first bound texture to the target.
// This is the fast path for gogpu's renderTexturedQuad (6 vertices, no vertex buffer,
// texture in bind group). If no texture is found, this is a no-op (clear-only pass).
func (r *RenderPassEncoder) executeFullscreenBlit(target *Texture) {
	srcView := r.findBoundTexture()
	if srcView == nil || srcView.texture == nil {
		return
	}

	src := srcView.texture
	src.mu.RLock()
	srcData := src.data
	srcW := int(src.width)
	srcH := int(src.height)
	srcFmt := src.format
	src.mu.RUnlock()

	dstW := int(target.width)
	dstH := int(target.height)
	dstFmt := target.format

	target.mu.Lock()
	defer target.mu.Unlock()

	for dy := 0; dy < dstH; dy++ {
		// Source Y with nearest-neighbor sampling.
		sy := dy * srcH / dstH
		if sy >= srcH {
			sy = srcH - 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx := dx * srcW / dstW
			if sx >= srcW {
				sx = srcW - 1
			}

			srcIdx := (sy*srcW + sx) * 4
			dstIdx := (dy*dstW + dx) * 4

			if srcIdx+3 >= len(srcData) || dstIdx+3 >= len(target.data) {
				continue
			}

			sr, sg, sb, sa := srcData[srcIdx], srcData[srcIdx+1], srcData[srcIdx+2], srcData[srcIdx+3]

			// Handle RGBA<->BGRA conversion.
			if needsSwizzle(srcFmt, dstFmt) {
				sr, sb = sb, sr
			}

			target.data[dstIdx+0] = sr
			target.data[dstIdx+1] = sg
			target.data[dstIdx+2] = sb
			target.data[dstIdx+3] = sa
		}
	}
}

// needsSwizzle returns true when source and destination formats require R/B channel swap.
func needsSwizzle(src, dst gputypes.TextureFormat) bool {
	srcBGRA := src == gputypes.TextureFormatBGRA8Unorm || src == gputypes.TextureFormatBGRA8UnormSrgb
	dstBGRA := dst == gputypes.TextureFormatBGRA8Unorm || dst == gputypes.TextureFormatBGRA8UnormSrgb
	return srcBGRA != dstBGRA
}

// findBoundTexture searches all bind groups for the first texture view binding.
func (r *RenderPassEncoder) findBoundTexture() *TextureView {
	for i := range r.bindGroups {
		bg := r.bindGroups[i]
		if bg == nil {
			continue
		}
		for _, tv := range bg.textureViews {
			if tv != nil {
				return tv
			}
		}
	}
	return nil
}

// executeVertexDraw performs vertex fetch, viewport transform, and triangle rasterization.
func (r *RenderPassEncoder) executeVertexDraw(target *Texture, vertexCount, firstVertex uint32) {
	if r.pipeline.desc == nil {
		return
	}

	layouts := r.pipeline.desc.Vertex.Buffers
	if len(layouts) == 0 {
		return
	}

	w := int(target.width)
	h := int(target.height)

	pipe := raster.NewPipeline(w, h)

	// Copy current framebuffer into the raster pipeline so draws composite.
	target.mu.RLock()
	existingData := make([]byte, len(target.data))
	copy(existingData, target.data)
	target.mu.RUnlock()
	pipe.Clear(0, 0, 0, 0)
	// Overwrite with existing data by setting pixels directly.
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			idx := (py*w + px) * 4
			pipe.SetPixel(px, py, existingData[idx], existingData[idx+1], existingData[idx+2], existingData[idx+3])
		}
	}

	// Fetch vertices and build triangles.
	triangles := r.fetchTriangles(layouts, vertexCount, firstVertex, w, h)

	// Determine fragment color source.
	if r.hasVertexColors(layouts) {
		pipe.DrawTrianglesInterpolated(triangles)
	} else {
		color := r.resolveFragmentColor()
		pipe.DrawTriangles(triangles, color)
	}

	// Write raster result back to texture.
	target.mu.Lock()
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			cr, cg, cb, ca := pipe.GetPixel(px, py)
			idx := (py*w + px) * 4
			target.data[idx+0] = cr
			target.data[idx+1] = cg
			target.data[idx+2] = cb
			target.data[idx+3] = ca
		}
	}
	target.mu.Unlock()
}

// fetchTriangles reads vertex data from bound buffers, applies viewport transform,
// and groups vertices into triangles (TriangleList topology).
func (r *RenderPassEncoder) fetchTriangles(
	layouts []gputypes.VertexBufferLayout,
	vertexCount, firstVertex uint32,
	targetW, targetH int,
) []raster.Triangle {
	if vertexCount < 3 {
		return nil
	}

	layout := layouts[0]
	vb := r.vertexBufs[0]
	if vb.buffer == nil {
		return nil
	}

	vb.buffer.mu.RLock()
	bufData := vb.buffer.data
	vb.buffer.mu.RUnlock()

	stride := layout.ArrayStride
	if stride == 0 {
		return nil
	}

	// Classify attributes: find position (location 0) and others.
	var posAttr *gputypes.VertexAttribute
	var extraAttrs []gputypes.VertexAttribute
	for i := range layout.Attributes {
		attr := &layout.Attributes[i]
		if attr.ShaderLocation == 0 {
			posAttr = attr
		} else {
			extraAttrs = append(extraAttrs, *attr)
		}
	}
	if posAttr == nil {
		return nil
	}

	// Read all vertices.
	vertices := make([]raster.ScreenVertex, 0, vertexCount)
	for i := uint32(0); i < vertexCount; i++ {
		vi := firstVertex + i
		base := vb.offset + uint64(vi)*stride

		// Read position.
		pos := readVertexAttribute(bufData, base+posAttr.Offset, posAttr.Format)

		// NDC to screen transform.
		// Position is expected in NDC: x,y in [-1,1], z in [0,1].
		// Screen: x = (ndcX+1)/2 * width, y = (1-ndcY)/2 * height (Y flipped).
		sx := (pos[0] + 1.0) * 0.5 * float32(targetW)
		sy := (1.0 - pos[1]) * 0.5 * float32(targetH)
		sz := float32(0)
		if len(pos) > 2 {
			sz = pos[2]
		}

		sv := raster.ScreenVertex{
			X: sx,
			Y: sy,
			Z: sz,
			W: 1.0,
		}

		// Read extra attributes (color, UV, etc.).
		for _, attr := range extraAttrs {
			vals := readVertexAttribute(bufData, base+attr.Offset, attr.Format)
			sv.Attributes = append(sv.Attributes, vals...)
		}

		vertices = append(vertices, sv)
	}

	// Group into triangles (TriangleList).
	triCount := len(vertices) / 3
	triangles := make([]raster.Triangle, 0, triCount)
	for i := 0; i < triCount; i++ {
		triangles = append(triangles, raster.Triangle{
			V0: vertices[i*3+0],
			V1: vertices[i*3+1],
			V2: vertices[i*3+2],
		})
	}

	return triangles
}

// readVertexAttribute reads float values from buffer data at the given offset.
func readVertexAttribute(data []byte, offset uint64, format gputypes.VertexFormat) []float32 {
	if int(offset) >= len(data) {
		return nil
	}
	d := data[offset:]

	switch format {
	case gputypes.VertexFormatFloat32:
		if len(d) < 4 {
			return nil
		}
		return []float32{math.Float32frombits(binary.LittleEndian.Uint32(d))}

	case gputypes.VertexFormatFloat32x2:
		if len(d) < 8 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
		}

	case gputypes.VertexFormatFloat32x3:
		if len(d) < 12 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[8:])),
		}

	case gputypes.VertexFormatFloat32x4:
		if len(d) < 16 {
			return nil
		}
		return []float32{
			math.Float32frombits(binary.LittleEndian.Uint32(d[0:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[4:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[8:])),
			math.Float32frombits(binary.LittleEndian.Uint32(d[12:])),
		}

	case gputypes.VertexFormatUnorm8x4:
		if len(d) < 4 {
			return nil
		}
		return []float32{
			float32(d[0]) / 255.0,
			float32(d[1]) / 255.0,
			float32(d[2]) / 255.0,
			float32(d[3]) / 255.0,
		}

	default:
		// Unsupported format, return zeros based on format size.
		n := int(format.Size() / 4)
		if n == 0 {
			n = 1
		}
		return make([]float32, n)
	}
}

// hasVertexColors returns true if the vertex layout has color-like attributes
// (4+ float components) beyond position (location 0).
func (r *RenderPassEncoder) hasVertexColors(layouts []gputypes.VertexBufferLayout) bool {
	if len(layouts) == 0 {
		return false
	}
	for _, attr := range layouts[0].Attributes {
		if attr.ShaderLocation == 0 {
			continue
		}
		// 4-component attribute is likely RGBA color.
		switch attr.Format {
		case gputypes.VertexFormatFloat32x4, gputypes.VertexFormatUnorm8x4:
			return true
		}
	}
	return false
}

// resolveFragmentColor determines the solid color for non-color-interpolated draws.
// Checks uniform buffers in bind groups for color data, falls back to white.
func (r *RenderPassEncoder) resolveFragmentColor() [4]float32 {
	// Try reading a color from the first uniform buffer in any bind group.
	for i := range r.bindGroups {
		bg := r.bindGroups[i]
		if bg == nil {
			continue
		}
		for _, buf := range bg.buffers {
			if buf == nil || len(buf.data) < 16 {
				continue
			}
			// Attempt to read 4 floats as RGBA color.
			buf.mu.RLock()
			d := buf.data
			cr := math.Float32frombits(binary.LittleEndian.Uint32(d[0:]))
			cg := math.Float32frombits(binary.LittleEndian.Uint32(d[4:]))
			cb := math.Float32frombits(binary.LittleEndian.Uint32(d[8:]))
			ca := math.Float32frombits(binary.LittleEndian.Uint32(d[12:]))
			buf.mu.RUnlock()

			// Sanity check: values should be in [0,1] range for normalized color.
			if cr >= 0 && cr <= 1 && cg >= 0 && cg <= 1 && cb >= 0 && cb <= 1 && ca >= 0 && ca <= 1 {
				return [4]float32{cr, cg, cb, ca}
			}
		}
	}

	return [4]float32{1, 1, 1, 1} // Default: white.
}
