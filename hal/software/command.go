//go:build !(js && wasm)

package software

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"log/slog"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software/raster"
)

// CommandEncoder implements hal.CommandEncoder for the software backend.
// It holds a device reference so that compute passes can resolve bind group
// resources during dispatch.
type CommandEncoder struct {
	device *Device
}

// BeginEncoding is a no-op.
func (c *CommandEncoder) BeginEncoding(_ string) error {
	return nil
}

// EndEncoding returns a placeholder command buffer.
func (c *CommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	return &Resource{}, nil
}

// DiscardEncoding is a no-op.
func (c *CommandEncoder) DiscardEncoding() {}

// ResetAll is a no-op.
func (c *CommandEncoder) ResetAll(_ []hal.CommandBuffer) {}

// Destroy is a no-op for the software backend.
func (c *CommandEncoder) Destroy() {}

// TransitionBuffers is a no-op (software backend doesn't need explicit transitions).
func (c *CommandEncoder) TransitionBuffers(_ []hal.BufferBarrier) {}

// TransitionTextures is a no-op (software backend doesn't need explicit transitions).
func (c *CommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {}

// ClearBuffer clears a buffer region to zero.
func (c *CommandEncoder) ClearBuffer(buffer hal.Buffer, offset, size uint64) {
	if b, ok := buffer.(*Buffer); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Clear to zero
		for i := offset; i < offset+size && i < uint64(len(b.data)); i++ {
			b.data[i] = 0
		}
	}
}

// CopyBufferToBuffer copies data between buffers.
func (c *CommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	srcBuf, srcOK := src.(*Buffer)
	dstBuf, dstOK := dst.(*Buffer)

	if !srcOK || !dstOK {
		return
	}

	for _, region := range regions {
		srcBuf.mu.RLock()
		dstBuf.mu.Lock()

		// Perform copy with bounds checking
		srcEnd := region.SrcOffset + region.Size
		dstEnd := region.DstOffset + region.Size

		if srcEnd <= uint64(len(srcBuf.data)) && dstEnd <= uint64(len(dstBuf.data)) {
			copy(dstBuf.data[region.DstOffset:dstEnd], srcBuf.data[region.SrcOffset:srcEnd])
		}

		dstBuf.mu.Unlock()
		srcBuf.mu.RUnlock()
	}
}

// CopyBufferToTexture copies data from a buffer to a texture.
func (c *CommandEncoder) CopyBufferToTexture(src hal.Buffer, dst hal.Texture, regions []hal.BufferTextureCopy) {
	srcBuf, srcOK := src.(*Buffer)
	dstTex := resolveTexture(dst)
	if !srcOK || dstTex == nil {
		return
	}

	for _, region := range regions {
		r, ok := dstTex.regionOf(region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
		if !ok {
			logRegionOutside("CopyBufferToTexture", dstTex, region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
			continue
		}
		// Row by row: the buffer rows are bytesPerRow apart (a multiple of
		// 256 for multi-row copies) and the region starts at the texture
		// origin of its mip level. Copying one tight block to offset 0
		// sheared padded uploads and ignored the origin.
		srcBuf.mu.RLock()
		dstTex.mu.Lock()
		dstTex.copyLinear(r, srcBuf.data, region.BufferLayout, true)
		dstTex.mu.Unlock()
		srcBuf.mu.RUnlock()
	}
}

// CopyTextureToBuffer copies data from a texture to a buffer: layer by layer
// and row by row from the origin of the region's mip level, into rows
// bytesPerRow apart and images rowsPerImage rows apart. Mirrors
// CopyBufferToTexture above.
func (c *CommandEncoder) CopyTextureToBuffer(src hal.Texture, dst hal.Buffer, regions []hal.BufferTextureCopy) {
	srcTex := resolveTexture(src)
	dstBuf, dstOK := dst.(*Buffer)
	if srcTex == nil || !dstOK {
		return
	}

	for _, region := range regions {
		r, ok := srcTex.regionOf(region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
		if !ok {
			logRegionOutside("CopyTextureToBuffer", srcTex, region.TextureBase.MipLevel, region.TextureBase.Origin, region.Size)
			continue
		}
		srcTex.mu.RLock()
		dstBuf.mu.Lock()
		srcTex.copyLinear(r, dstBuf.data, region.BufferLayout, false)
		dstBuf.mu.Unlock()
		srcTex.mu.RUnlock()
	}
}

// CopyTextureToTexture copies data between textures, including a surface's
// acquired texture on either side.
func (c *CommandEncoder) CopyTextureToTexture(src, dst hal.Texture, regions []hal.TextureCopy) {
	srcTex, dstTex := resolveTexture(src), resolveTexture(dst)
	if srcTex == nil || dstTex == nil {
		return
	}
	for _, region := range regions {
		copyTextureRegion(srcTex, dstTex, region)
	}
}

// copyTextureRegion copies one region as WebGPU copyTextureToTexture does:
// for each array layer or depth slice (Origin.Z) and each row of texel
// blocks, Size.Width texels from the source origin in the source mip level
// to the destination origin in the destination mip level, each side at its
// level's pitch (subresource.go). The formats are copy-compatible (core
// validates that), so a row has the same bytes on both sides.
func copyTextureRegion(src, dst *Texture, region hal.TextureCopy) {
	sr, srcOK := src.regionOf(region.SrcBase.MipLevel, region.SrcBase.Origin, region.Size)
	dr, dstOK := dst.regionOf(region.DstBase.MipLevel, region.DstBase.Origin, region.Size)
	if !srcOK || !dstOK {
		if !srcOK {
			logRegionOutside("CopyTextureToTexture", src, region.SrcBase.MipLevel, region.SrcBase.Origin, region.Size)
		}
		if !dstOK {
			logRegionOutside("CopyTextureToTexture", dst, region.DstBase.MipLevel, region.DstBase.Origin, region.Size)
		}
		return
	}
	if src == dst {
		dst.mu.Lock()
		defer dst.mu.Unlock()
	} else {
		src.mu.RLock()
		defer src.mu.RUnlock()
		dst.mu.Lock()
		defer dst.mu.Unlock()
	}

	rowBytes := min(sr.rowBytes, dr.rowBytes)
	for z := range sr.images {
		for row := range sr.rows {
			srcStart, dstStart := src.rowOffset(sr, z, row), dst.rowOffset(dr, z, row)
			if srcStart+rowBytes <= uint64(len(src.data)) && dstStart+rowBytes <= uint64(len(dst.data)) {
				copy(dst.data[dstStart:dstStart+rowBytes], src.data[srcStart:srcStart+rowBytes])
			}
		}
	}
}

// ResolveQuerySet is a no-op (query sets not supported in software backend).
func (c *CommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {}

// BuildAccelerationStructures builds BVH trees for BLAS entries by extracting
// triangle data from vertex buffers, and stores TLAS instance references.
// This is the CPU equivalent of vkCmdBuildAccelerationStructuresKHR / DXR
// BuildRaytracingAccelerationStructure.
func (c *CommandEncoder) BuildAccelerationStructures(descriptors []hal.BuildAccelerationStructureDescriptor) {
	for i := range descriptors {
		desc := &descriptors[i]
		if desc.Entries == nil || desc.DestinationAccelerationStructure == nil {
			continue
		}

		dstAS, ok := desc.DestinationAccelerationStructure.(*AccelerationStructure)
		if !ok || dstAS == nil {
			continue
		}

		entries := desc.Entries

		switch {
		case len(entries.Triangles) > 0:
			// BLAS build: extract triangles from vertex buffers and build BVH.
			var allTris []Triangle
			for geomIdx, tri := range entries.Triangles {
				buf, bufOK := tri.VertexBuffer.(*Buffer)
				if !bufOK || buf == nil {
					continue
				}
				tris := extractTrianglesFromBuffer(buf, tri.FirstVertex, tri.VertexCount, tri.VertexStride, uint32(geomIdx))
				allTris = append(allTris, tris...)
			}
			dstAS.bvh = BuildBVH(allTris)
			dstAS.format = hal.AccelerationStructureFormatBottomLevel

		case len(entries.AABBs) > 0:
			// BLAS build from AABB primitives.
			var allAABBs []AABBPrimitive
			for geomIdx, aabb := range entries.AABBs {
				buf, bufOK := aabb.Buffer.(*Buffer)
				if !bufOK || buf == nil {
					continue
				}
				prims := extractAABBsFromBuffer(buf, aabb.Offset, aabb.Count, aabb.Stride, uint32(geomIdx))
				allAABBs = append(allAABBs, prims...)
			}
			dstAS.bvh = BuildBVHFromAABBs(allAABBs)
			dstAS.format = hal.AccelerationStructureFormatBottomLevel

		case entries.Instances != nil:
			// TLAS build: store instance references (resolved at traversal time).
			dstAS.format = hal.AccelerationStructureFormatTopLevel
			// Instance data is packed in the buffer — for the software backend,
			// TLAS traversal would need to decode instances. For now, store the
			// count so GetAccelerationStructureBuildSizes returns meaningful values.
			dstAS.instances = nil // Future: decode instance buffer into TLASInstanceData slice.
		}
	}
}

// extractAABBsFromBuffer reads AABB primitives from a buffer.
// Each AABB is 6 x float32 (min.xyz, max.xyz = 24 bytes).
func extractAABBsFromBuffer(buf *Buffer, offset, count uint32, stride uint64, geomIndex uint32) []AABBPrimitive {
	if buf == nil || len(buf.data) == 0 || count == 0 {
		return nil
	}

	buf.mu.RLock()
	defer buf.mu.RUnlock()

	if stride == 0 {
		stride = 24 // tightly packed: 6 x float32
	}

	prims := make([]AABBPrimitive, 0, count)
	for i := uint32(0); i < count; i++ {
		off := uint64(offset)*stride + uint64(i)*stride
		if off+24 > uint64(len(buf.data)) {
			break
		}
		minV := readFloat3(buf.data, off)
		maxV := readFloat3(buf.data, off+12)
		if minV == nil || maxV == nil {
			continue
		}
		prims = append(prims, AABBPrimitive{
			Min:            *minV,
			Max:            *maxV,
			GeometryIndex:  geomIndex,
			PrimitiveIndex: i,
		})
	}
	return prims
}

// PlaceAccelerationStructureBarrier is a no-op on the software backend.
// CPU execution is inherently ordered; no memory barriers are needed.
func (c *CommandEncoder) PlaceAccelerationStructureBarrier(_ hal.AccelerationStructureBarrier) {}

// CopyAccelerationStructure performs a deep copy of the source BVH tree
// into the destination acceleration structure.
func (c *CommandEncoder) CopyAccelerationStructure(src, dst hal.AccelerationStructure, _ gputypes.AccelerationStructureCopyMode) {
	srcAS, ok := src.(*AccelerationStructure)
	if !ok || srcAS == nil {
		return
	}
	dstAS, ok := dst.(*AccelerationStructure)
	if !ok || dstAS == nil {
		return
	}

	dstAS.bvh = deepCopyBVH(srcAS.bvh)
	dstAS.format = srcAS.format
	dstAS.size = srcAS.size

	if len(srcAS.instances) > 0 {
		dstAS.instances = make([]TLASInstanceData, len(srcAS.instances))
		copy(dstAS.instances, srcAS.instances)
	}
}

// ReadAccelerationStructureCompactSize writes the current BVH size estimate
// into the destination buffer as a uint64. This is the CPU equivalent of
// vkCmdWriteAccelerationStructuresPropertiesKHR COMPACTED_SIZE.
func (c *CommandEncoder) ReadAccelerationStructureCompactSize(as hal.AccelerationStructure, buffer hal.Buffer, offset uint64) {
	swAS, ok := as.(*AccelerationStructure)
	if !ok || swAS == nil {
		return
	}
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return
	}

	buf.mu.Lock()
	defer buf.mu.Unlock()

	// Write the stored size as uint64 little-endian.
	if offset+8 <= uint64(len(buf.data)) {
		binary.LittleEndian.PutUint64(buf.data[offset:], swAS.size)
	}
}

// BeginRenderPass begins a render pass and returns an encoder.
// If a depth/stencil attachment is present, a persistent stencil buffer is
// created for the entire pass (matching GPU behavior where the stencil buffer
// is the attachment texture, not recreated per draw call).
func (c *CommandEncoder) BeginRenderPass(desc *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	r := &RenderPassEncoder{
		desc: desc,
	}
	r.resolveColorTargets()

	if hal.Logger().Enabled(context.Background(), slog.LevelDebug) {
		var w, h uint32
		loadOp := gputypes.LoadOpClear
		if len(desc.ColorAttachments) > 0 {
			loadOp = desc.ColorAttachments[0].LoadOp
			if target := r.colorTargets[0]; target != nil {
				w, h = target.width, target.height
			}
		}
		hal.Logger().Debug("software: BeginRenderPass",
			"width", w, "height", h,
			"colorLoadOp", loadOp,
			"attachments", len(desc.ColorAttachments),
		)
	}

	// Create the persistent stencil and depth buffers of the depth/stencil
	// attachment, sized to the view's mip level.
	if tex, level, layer, ok := depthAttachmentImage(desc.DepthStencilAttachment); ok {
		w, h, _ := tex.levelExtent(level)
		if w > 0 && h > 0 {
			r.passStencilBuffer = raster.NewStencilBuffer(int(w), int(h))
			if desc.DepthStencilAttachment.StencilLoadOp == gputypes.LoadOpClear {
				r.passStencilBuffer.Clear(uint8(desc.DepthStencilAttachment.StencilClearValue))
			}
			r.passDepthBuffer = loadDepthAttachment(desc.DepthStencilAttachment, tex, level, layer)
		}
	}

	return r
}

// loadDepthAttachment returns the depth buffer every draw of a pass shares:
// cleared to the clear value with LoadOpClear, otherwise loaded from array
// layer layer of mip level level of the attachment texture (a read-only
// depth aspect always loads). nil when the format has no depth aspect.
func loadDepthAttachment(ds *hal.RenderPassDepthStencilAttachment, tex *Texture, level, layer uint32) *raster.DepthBuffer {
	if !tex.format.HasDepth() {
		return nil
	}
	w, h, _ := tex.levelExtent(level)
	buf := raster.NewDepthBuffer(int(w), int(h))
	if ds.DepthLoadOp == gputypes.LoadOpClear && !ds.DepthReadOnly {
		buf.Clear(ds.DepthClearValue)
		return buf
	}
	tex.mu.RLock()
	defer tex.mu.RUnlock()
	img, ok := tex.image(level, layer)
	if !ok {
		return buf
	}
	for y := range int(img.height) {
		for x := range int(img.width) {
			if v, ok := readDepthTexel(img, tex.format, x, y); ok {
				buf.Set(x, y, v)
			}
		}
	}
	return buf
}

// storeDepthAttachment writes the pass's depth buffer back to the
// attachment view's mip level and array layer with StoreOpStore, unless the
// depth aspect is read-only.
func storeDepthAttachment(ds *hal.RenderPassDepthStencilAttachment, buf *raster.DepthBuffer) {
	if ds == nil || buf == nil || ds.DepthReadOnly || ds.DepthStoreOp != gputypes.StoreOpStore {
		return
	}
	tex, level, layer, ok := depthAttachmentImage(ds)
	if !ok {
		return
	}
	tex.mu.Lock()
	defer tex.mu.Unlock()
	img, ok := tex.image(level, layer)
	if !ok {
		return
	}
	for y := range min(int(img.height), buf.Height()) {
		for x := range min(int(img.width), buf.Width()) {
			writeDepthTexel(img, tex.format, x, y, buf.Get(x, y))
		}
	}
}

// readDepthTexel decodes the depth of texel (x, y) of img, an image of a
// texture of format format: 16-bit unorm for Depth16Unorm, otherwise a
// float32 in the texel's first four bytes (Depth24Plus is opaque in WebGPU,
// so a float32 is a valid encoding). The caller holds the texture lock.
func readDepthTexel(img textureImage, format gputypes.TextureFormat, x, y int) (float32, bool) {
	bpp := int(formatBytesPerPixel(format))
	off := y*int(img.rowPitch) + x*bpp
	if format == gputypes.TextureFormatDepth16Unorm {
		if off+2 > len(img.data) {
			return 0, false
		}
		return float32(binary.LittleEndian.Uint16(img.data[off:])) / 65535, true
	}
	if bpp < 4 || off+4 > len(img.data) {
		return 0, false
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(img.data[off:])), true
}

// writeDepthTexel encodes depth into texel (x, y) of img as readDepthTexel
// decodes it. The caller holds the texture lock.
func writeDepthTexel(img textureImage, format gputypes.TextureFormat, x, y int, depth float32) {
	bpp := int(formatBytesPerPixel(format))
	off := y*int(img.rowPitch) + x*bpp
	if format == gputypes.TextureFormatDepth16Unorm {
		if off+2 <= len(img.data) {
			binary.LittleEndian.PutUint16(img.data[off:], raster.FloatToUnorm16(depth))
		}
		return
	}
	if bpp >= 4 && off+4 <= len(img.data) {
		binary.LittleEndian.PutUint32(img.data[off:], math.Float32bits(depth))
	}
}

// BeginComputePass begins a compute pass and returns an encoder.
// The device reference is passed through so Dispatch can resolve bind group
// buffer data.
func (c *CommandEncoder) BeginComputePass(desc *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &ComputePassEncoder{
		desc:   desc,
		device: c.device,
	}
}

// vertexBufferBinding holds a vertex buffer and its byte offset.
type vertexBufferBinding struct {
	buffer *Buffer
	offset uint64
}

// RenderPassEncoder implements hal.RenderPassEncoder for the software backend.
// It tracks pipeline state set during encoding and executes draw calls
// using the raster/ package for triangle rasterization.
type RenderPassEncoder struct {
	desc *hal.RenderPassDescriptor

	// Pipeline and resource state set during encoding.
	pipeline    *RenderPipeline
	bindGroups  [4]*BindGroup          // max 4 per WebGPU spec
	vertexBufs  [8]vertexBufferBinding // max 8 vertex buffers
	indexBuffer *Buffer
	indexFormat gputypes.IndexFormat
	indexOffset uint64

	// activeIndices, when non-nil, remaps the sequential draw position to a
	// vertex index for the current DrawIndexed call (already including
	// baseVertex). The vertex fetch paths consult it instead of computing
	// firstVertex+i, so indexed and non-indexed draws share one rasterizer.
	activeIndices []uint32

	// Viewport and scissor state.
	viewport    [6]float32 // x, y, w, h, minDepth, maxDepth
	scissorRect [4]uint32  // x, y, w, h
	hasViewport bool
	hasScissor  bool

	// Stencil reference value set by SetStencilReference.
	stencilRef uint32

	// Persistent stencil buffer for the render pass — created once at
	// BeginRenderPass, reused across all Draw() calls. On GPU backends
	// the stencil buffer is the depth/stencil attachment texture; here
	// we emulate that by keeping a single raster.StencilBuffer alive
	// for the entire pass. Without this, stencil writes from pass 1
	// (clip shape) would be lost before pass 2 (content draw).
	passStencilBuffer *raster.StencilBuffer

	// passDepthBuffer is the depth attachment's contents for the pass: loaded
	// or cleared at BeginRenderPass, shared by every draw, written back at End
	// (P1-SW-015). nil without a depth aspect.
	passDepthBuffer *raster.DepthBuffer

	// colorTargets holds the texture each color attachment renders into, by
	// attachment index (resolveColorTargets): the view's texture when it is a
	// single image, the pass-local target of the view's mip level, array
	// layer or depth slice, or nil when the pass writes nothing for it.
	// subTargets are stored back into their subresources at End.
	colorTargets []*Texture
	subTargets   []*subresourceTarget

	// Whether the framebuffer has been cleared this pass.
	// WebGPU spec: LoadOp=Clear happens before the first draw, not at End().
	cleared bool

	// drawCount tracks total Draw/DrawIndexed calls for Stats().
	drawCount uint32
}

// End finishes the render pass.
// If no draw calls were issued and LoadOp is Clear, the clear is applied now.
// MSAA resolve: copies color attachment pixels to resolve target (WebGPU spec).
func (r *RenderPassEncoder) End() {
	hal.Logger().Debug("software: End",
		"draws", r.drawCount,
		"hasScissor", r.hasScissor,
		"scissor", r.scissorRect,
	)

	// If no draw happened, apply pending clears.
	if !r.cleared {
		r.applyClear()
	}

	// MSAA resolve: copy color attachment to resolve target.
	// In WebGPU, if a color attachment has a ResolveTarget, the GPU resolves
	// MSAA samples to the single-sample target at end of render pass.
	// Software backend has no real MSAA — this is a direct pixel copy into
	// the resolve view's mip level and array layer.
	for i, attachment := range r.desc.ColorAttachments {
		if attachment.ResolveTarget != nil {
			resolveInto(r.colorTargets[i], attachment.ResolveTarget)
		}
	}

	r.storeSubresourceTargets()
	storeDepthAttachment(r.desc.DepthStencilAttachment, r.passDepthBuffer)
}

// applyClear clears color attachments that have LoadOp=Clear.
func (r *RenderPassEncoder) applyClear() {
	r.cleared = true
	for i, attachment := range r.desc.ColorAttachments {
		if attachment.LoadOp == gputypes.LoadOpClear && r.colorTargets[i] != nil {
			r.colorTargets[i].Clear(attachment.ClearValue)
		}
	}
}

// SetPipeline stores the render pipeline for subsequent draw calls.
func (r *RenderPassEncoder) SetPipeline(p hal.RenderPipeline) {
	if rp, ok := p.(*RenderPipeline); ok {
		r.pipeline = rp
	}
}

// SetBindGroup stores a bind group at the given index.
func (r *RenderPassEncoder) SetBindGroup(index uint32, bg hal.BindGroup, dynamicOffsets []uint32) {
	if index < 4 {
		if b, ok := bg.(*BindGroup); ok {
			b.dynamicOffsets = dynamicOffsets
			r.bindGroups[index] = b
		}
	}
}

// SetVertexBuffer stores a vertex buffer binding at the given slot.
func (r *RenderPassEncoder) SetVertexBuffer(slot uint32, buf hal.Buffer, offset uint64) {
	if slot < 8 {
		if b, ok := buf.(*Buffer); ok {
			r.vertexBufs[slot] = vertexBufferBinding{buffer: b, offset: offset}
		}
	}
}

// SetIndexBuffer stores the index buffer for indexed draw calls.
func (r *RenderPassEncoder) SetIndexBuffer(buf hal.Buffer, format gputypes.IndexFormat, offset uint64) {
	if b, ok := buf.(*Buffer); ok {
		r.indexBuffer = b
		r.indexFormat = format
		r.indexOffset = offset
	}
}

// SetViewport stores the viewport transformation.
func (r *RenderPassEncoder) SetViewport(vp gputypes.Viewport) {
	r.viewport = [6]float32{vp.X, vp.Y, vp.Width, vp.Height, vp.MinDepth, vp.MaxDepth}
	r.hasViewport = true
}

// SetScissorRect stores the scissor rectangle.
func (r *RenderPassEncoder) SetScissorRect(rect gputypes.ScissorRect) {
	r.scissorRect = [4]uint32{rect.X, rect.Y, rect.Width, rect.Height}
	r.hasScissor = true
	hal.Logger().Debug("software: SetScissorRect", "x", rect.X, "y", rect.Y, "w", rect.Width, "h", rect.Height)
}

// SetBlendConstant is a no-op (blend constants not yet wired to raster pipeline).
func (r *RenderPassEncoder) SetBlendConstant(_ *gputypes.Color) {}

// SetStencilReference stores the stencil reference value for subsequent draw calls.
// The reference value is used by stencil comparison and StencilOpReplace.
func (r *RenderPassEncoder) SetStencilReference(ref uint32) {
	r.stencilRef = ref
}

// Draw executes a non-indexed draw call.
// It performs vertex fetch, viewport transform, and triangle rasterization
// using the raster/ package. If no vertex buffer is bound and a texture is
// available in a bind group, it performs a fullscreen texture blit.
// Supports instanced rendering: instanceCount > 1 draws the same vertices
// multiple times, advancing instance-rate vertex buffers per instance.
func (r *RenderPassEncoder) Draw(args gputypes.DrawArgs) {
	r.drawCount++
	hal.Logger().Debug("software: Draw", "vertices", args.VertexCount, "instances", args.InstanceCount, "drawIndex", r.drawCount)
	r.executeDraw(args.VertexCount, args.InstanceCount, args.FirstVertex, args.FirstInstance)
}

// DrawIndexed executes an indexed draw call. It resolves the index buffer into
// a list of vertex indices (applying baseVertex) and runs the same vertex-fetch
// and rasterization path as Draw, with vertex positions remapped through the
// resolved indices. Previously this was a no-op, so every indexed draw (glyph
// mask text, MSDF text) rendered nothing on the software backend.
func (r *RenderPassEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	r.drawCount++
	if args.IndexCount == 0 || r.indexBuffer == nil {
		return
	}
	indices := r.resolveIndices(args.IndexCount, args.FirstIndex, args.BaseVertex)
	if indices == nil {
		return
	}
	hal.Logger().Debug("software: DrawIndexed", "indices", args.IndexCount, "instances", args.InstanceCount, "drawIndex", r.drawCount)

	r.activeIndices = indices
	defer func() { r.activeIndices = nil }()
	// vertexCount == indexCount; firstVertex is unused because activeIndices
	// supplies the per-position vertex index directly.
	r.executeDraw(args.IndexCount, args.InstanceCount, 0, args.FirstInstance)
}

// resolveIndices reads indexCount entries from the bound index buffer starting
// at firstIndex, honoring the index format (Uint16/Uint32) and the buffer
// offset set by SetIndexBuffer, and adds baseVertex to each. Returns nil if the
// requested range lies outside the buffer.
func (r *RenderPassEncoder) resolveIndices(indexCount, firstIndex uint32, baseVertex int32) []uint32 {
	indexSize := uint64(2)
	if r.indexFormat == gputypes.IndexFormatUint32 {
		indexSize = 4
	}

	r.indexBuffer.mu.RLock()
	data := r.indexBuffer.data
	r.indexBuffer.mu.RUnlock()

	start := r.indexOffset + uint64(firstIndex)*indexSize
	end := start + uint64(indexCount)*indexSize
	if end > uint64(len(data)) {
		return nil
	}

	out := make([]uint32, indexCount)
	for i := uint32(0); i < indexCount; i++ {
		off := start + uint64(i)*indexSize
		var idx uint32
		if indexSize == 4 {
			idx = uint32(data[off]) | uint32(data[off+1])<<8 | uint32(data[off+2])<<16 | uint32(data[off+3])<<24
		} else {
			idx = uint32(data[off]) | uint32(data[off+1])<<8
		}
		out[i] = uint32(int32(idx) + baseVertex)
	}
	return out
}

// drawVertexIndex maps a sequential draw position to the vertex index to fetch.
// For non-indexed draws it is firstVertex+pos; for indexed draws it reads the
// resolved index list set by DrawIndexed.
func (r *RenderPassEncoder) drawVertexIndex(firstVertex, pos uint32) uint32 {
	if r.activeIndices != nil {
		if pos < uint32(len(r.activeIndices)) {
			return r.activeIndices[pos]
		}
		return 0
	}
	return firstVertex + pos
}

// Indirect record sizes in bytes: DrawIndirectArgs (vertexCount,
// instanceCount, firstVertex, firstInstance) and DrawIndexedIndirectArgs
// (indexCount, instanceCount, firstIndex, baseVertex, firstInstance).
const (
	drawIndirectRecordSize        = 16
	drawIndexedIndirectRecordSize = 20
)

// readIndirectArgs reads the first n (at most 5) little-endian u32 words at
// offset in buf. It runs when the command executes, so it sees what earlier
// commands (copies, compute passes, the indirect validation pass) wrote
// there. ok is false when buf is not a software buffer or the range lies
// outside it.
func readIndirectArgs(buf hal.Buffer, offset uint64, n int) (args [5]uint32, ok bool) {
	b, isBuf := buf.(*Buffer)
	if !isBuf || n > len(args) {
		return args, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	end := offset + uint64(n)*4
	if end < offset || end > uint64(len(b.data)) {
		return args, false
	}
	for i := range n {
		args[i] = binary.LittleEndian.Uint32(b.data[offset+uint64(i)*4:])
	}
	return args, true
}

// DrawIndirect draws drawCount consecutive DrawIndirectArgs records starting
// at offset, each as a Draw with the arguments read from the buffer.
func (r *RenderPassEncoder) DrawIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	for i := range uint64(drawCount) {
		args, ok := readIndirectArgs(buffer, offset+i*drawIndirectRecordSize, 4)
		if !ok {
			slog.Warn("software: DrawIndirect record outside the buffer", "offset", offset+i*drawIndirectRecordSize)
			return
		}
		r.Draw(gputypes.DrawArgs{
			VertexCount: args[0], InstanceCount: args[1], FirstVertex: args[2], FirstInstance: args[3],
		})
	}
}

// DrawIndexedIndirect draws drawCount consecutive DrawIndexedIndirectArgs
// records starting at offset, each as a DrawIndexed with the arguments read
// from the buffer. The index buffer and its offset are those set by
// SetIndexBuffer, as for DrawIndexed.
func (r *RenderPassEncoder) DrawIndexedIndirect(buffer hal.Buffer, offset uint64, drawCount uint32) {
	for i := range uint64(drawCount) {
		args, ok := readIndirectArgs(buffer, offset+i*drawIndexedIndirectRecordSize, 5)
		if !ok {
			slog.Warn("software: DrawIndexedIndirect record outside the buffer", "offset", offset+i*drawIndexedIndirectRecordSize)
			return
		}
		r.DrawIndexed(gputypes.DrawIndexedArgs{
			IndexCount: args[0], InstanceCount: args[1], FirstIndex: args[2],
			BaseVertex: int32(args[3]), FirstInstance: args[4],
		})
	}
}

// DrawIndirectCount is a no-op (count-buffer indirect not supported in software backend).
func (r *RenderPassEncoder) DrawIndirectCount(_ hal.Buffer, _ uint64, _ hal.Buffer, _ uint64, _ uint32) {
}

// DrawIndexedIndirectCount is a no-op.
func (r *RenderPassEncoder) DrawIndexedIndirectCount(_ hal.Buffer, _ uint64, _ hal.Buffer, _ uint64, _ uint32) {
}

// ExecuteBundle is a no-op.
func (r *RenderPassEncoder) ExecuteBundle(_ hal.RenderBundle) {}

// Stats returns render pass statistics after End(). Designed for CI e2e
// test assertions — zero overhead (fields already tracked during encoding).
func (r *RenderPassEncoder) Stats() RenderPassStats {
	s := RenderPassStats{
		DrawCount:  r.drawCount,
		HasScissor: r.hasScissor,
	}
	if r.hasScissor {
		s.ScissorRect = image.Rect(
			int(r.scissorRect[0]), int(r.scissorRect[1]),
			int(r.scissorRect[0]+r.scissorRect[2]), int(r.scissorRect[1]+r.scissorRect[3]),
		)
	}
	if r.desc != nil && len(r.desc.ColorAttachments) > 0 {
		s.ColorLoadOp = r.desc.ColorAttachments[0].LoadOp
		if tex := r.getTargetTexture(); tex != nil {
			s.Width = tex.width
			s.Height = tex.height
		}
	}
	return s
}

// ComputePassEncoder implements hal.ComputePassEncoder for the software backend.
// It collects pipeline and bind group state, then executes the SPIR-V interpreter
// on Dispatch. Buffer writes from storage buffer bindings are reflected in-place.
type ComputePassEncoder struct {
	desc   *hal.ComputePassDescriptor
	device *Device

	// Pipeline and resource state set during encoding.
	pipeline   *ComputePipeline
	bindGroups [4]*BindGroup // max 4 per WebGPU spec
}

// End finishes the compute pass. Currently a no-op since all work is done
// synchronously in Dispatch.
func (c *ComputePassEncoder) End() {}

// SetPipeline stores the compute pipeline for subsequent Dispatch calls.
func (c *ComputePassEncoder) SetPipeline(p hal.ComputePipeline) {
	if cp, ok := p.(*ComputePipeline); ok {
		c.pipeline = cp
	}
}

// SetBindGroup stores a bind group at the given index for compute dispatch.
func (c *ComputePassEncoder) SetBindGroup(index uint32, bg hal.BindGroup, dynamicOffsets []uint32) {
	if index < 4 {
		if b, ok := bg.(*BindGroup); ok {
			b.dynamicOffsets = dynamicOffsets
			c.bindGroups[index] = b
		}
	}
}

// Dispatch executes the compute shader for x*y*z workgroups.
//
// The implementation:
//  1. Gets the parsed SPIR-V module from the pipeline's shader module.
//  2. Reads the workgroup size from the entry point's OpExecutionMode LocalSize.
//  3. Builds an ExecutionContext with Buffers populated from bound bind groups.
//  4. Delegates to Module.DispatchCompute which iterates over all workgroups
//     and invocations, calling ExecuteCompute for each.
//  5. Storage buffer writes are reflected in-place through shared []byte slices.
func (c *ComputePassEncoder) Dispatch(x, y, z uint32) {
	if c.pipeline == nil {
		slog.Warn("software: ComputePassEncoder.Dispatch called without a pipeline set")
		return
	}

	parsedModule := c.pipeline.module
	if parsedModule == nil {
		slog.Warn("software: ComputePassEncoder.Dispatch: pipeline has no parsed SPIR-V module")
		return
	}

	entryPoint := c.pipeline.entryPoint

	// Build the execution context with the resources of all bind groups.
	ctx := bindGroupExecutionContext(&c.bindGroups)

	slog.Debug("software: ComputePassEncoder.Dispatch",
		"entryPoint", entryPoint,
		"workgroups", fmt.Sprintf("(%d,%d,%d)", x, y, z),
	)

	if err := parsedModule.DispatchCompute(entryPoint, ctx, x, y, z); err != nil {
		slog.Error("software: compute dispatch failed", "error", err)
	}
}

// DispatchIndirect reads the workgroup counts (x, y, z) from buffer at
// offset when it executes and dispatches them. Core has already replaced
// counts above maxComputeWorkgroupsPerDimension with zeros through its
// indirect validation pass, as on the GPU backends.
func (c *ComputePassEncoder) DispatchIndirect(buffer hal.Buffer, offset uint64) {
	args, ok := readIndirectArgs(buffer, offset, 3)
	if !ok {
		slog.Warn("software: DispatchIndirect arguments outside the buffer", "offset", offset)
		return
	}
	c.Dispatch(args[0], args[1], args[2])
}
