// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"errors"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

func TestCommandStateTrackerRetainsFirstUseAndEmitsInlineTransitions(t *testing.T) {
	const resource = "texture"
	tracker := newCommandStateTracker()

	if before, barrier := tracker.transition(resource, 3, d3d12.D3D12_RESOURCE_STATE_COMMON, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); before != d3d12.D3D12_RESOURCE_STATE_COMMON || !barrier {
		t.Fatalf("first use = (%d, %v), want (COMMON, true)", before, barrier)
	}
	if before, barrier := tracker.transition(resource, 3, d3d12.D3D12_RESOURCE_STATE_COMMON, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); before != d3d12.D3D12_RESOURCE_STATE_COPY_DEST || barrier {
		t.Fatalf("same local state = (%d, %v), want (COPY_DEST, false)", before, barrier)
	}
	if before, barrier := tracker.transition(resource, 3, d3d12.D3D12_RESOURCE_STATE_COMMON, d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE); before != d3d12.D3D12_RESOURCE_STATE_COPY_DEST || !barrier {
		t.Fatalf("inline transition = (%d, %v), want (COPY_DEST, true)", before, barrier)
	}

	summary := tracker.summary()
	if len(summary) != 1 || len(summary[0].states) != 1 {
		t.Fatalf("summary = %#v, want one resource/subresource", summary)
	}
	state := summary[0].states[0]
	if state.subresource != 3 || state.initial != d3d12.D3D12_RESOURCE_STATE_COMMON || state.current != d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE {
		t.Fatalf("state summary = %#v, want initial COMMON/final PIXEL_SHADER_RESOURCE", state)
	}
}

func TestMergeCompatibleReadStates(t *testing.T) {
	shaderRead := d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE | d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE
	tests := []struct {
		name      string
		current   d3d12.D3D12_RESOURCE_STATES
		requested d3d12.D3D12_RESOURCE_STATES
		want      d3d12.D3D12_RESOURCE_STATES
	}{
		{
			name:      "uniform and storage reads coexist",
			current:   d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER,
			requested: shaderRead,
			want:      d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER | shaderRead,
		},
		{
			name:      "read-only depth remains attached while sampled",
			current:   d3d12.D3D12_RESOURCE_STATE_DEPTH_READ,
			requested: shaderRead,
			want:      d3d12.D3D12_RESOURCE_STATE_DEPTH_READ | shaderRead,
		},
		{
			name:      "write state is replaced by read state",
			current:   d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			requested: shaderRead,
			want:      shaderRead,
		},
		{
			name:      "read state is replaced by write state",
			current:   shaderRead,
			requested: d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			want:      d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mergeCompatibleReadStates(test.current, test.requested); got != test.want {
				t.Fatalf("merged state = %d, want %d", got, test.want)
			}
		})
	}
}

func TestTextureSubresourceCountTreats3DDepthAsOneSubresource(t *testing.T) {
	array := &Texture{dimension: gputypes.TextureDimension2D, mipLevels: 4, size: hal.Extent3D{DepthOrArrayLayers: 3}}
	_3d := &Texture{dimension: gputypes.TextureDimension3D, mipLevels: 4, size: hal.Extent3D{DepthOrArrayLayers: 16}}

	if got := array.subresourceCount(); got != 12 {
		t.Fatalf("array subresource count = %d, want 12", got)
	}
	if got := _3d.subresourceCount(); got != 4 {
		t.Fatalf("3D subresource count = %d, want 4", got)
	}
}

func TestDepthStencilSubresourcePlanes(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatDepth24PlusStencil8,
		dimension: gputypes.TextureDimension2D,
		mipLevels: 2,
		size:      hal.Extent3D{DepthOrArrayLayers: 2},
	}
	if got := texture.subresourceCount(); got != 8 {
		t.Fatalf("depth/stencil subresource count = %d, want 8", got)
	}

	depth := textureRangeSubresources(texture, hal.TextureRange{
		Aspect:          gputypes.TextureAspectDepthOnly,
		MipLevelCount:   2,
		ArrayLayerCount: 2,
	})
	stencil := textureRangeSubresources(texture, hal.TextureRange{
		Aspect:          gputypes.TextureAspectStencilOnly,
		MipLevelCount:   2,
		ArrayLayerCount: 2,
	})
	all := textureRangeSubresources(texture, hal.TextureRange{
		Aspect:          gputypes.TextureAspectAll,
		MipLevelCount:   2,
		ArrayLayerCount: 2,
	})
	if want := []uint32{0, 1, 2, 3}; !equalUint32s(depth, want) {
		t.Fatalf("depth plane indexes = %v, want %v", depth, want)
	}
	if want := []uint32{4, 5, 6, 7}; !equalUint32s(stencil, want) {
		t.Fatalf("stencil plane indexes = %v, want %v", stencil, want)
	}
	if len(all) != 8 {
		t.Fatalf("all-aspect indexes = %v, want 8 entries", all)
	}

	stencilOnlyTexture := &Texture{
		format:    gputypes.TextureFormatStencil8,
		dimension: gputypes.TextureDimension2D,
		mipLevels: 1,
		size:      hal.Extent3D{DepthOrArrayLayers: 2},
	}
	standaloneStencil := textureRangeSubresources(stencilOnlyTexture, hal.TextureRange{
		Aspect:          gputypes.TextureAspectAll,
		MipLevelCount:   1,
		ArrayLayerCount: 2,
	})
	if want := []uint32{2, 3}; !equalUint32s(standaloneStencil, want) {
		t.Fatalf("standalone stencil indexes = %v, want physical plane-1 indexes %v", standaloneStencil, want)
	}
}

func TestDepthStencilAttachmentIncludesPackedCompanionPlane(t *testing.T) {
	view := &TextureView{
		texture: &Texture{
			format:    gputypes.TextureFormatStencil8,
			dimension: gputypes.TextureDimension2D,
			mipLevels: 1,
			size:      hal.Extent3D{DepthOrArrayLayers: 2},
		},
		aspect:     gputypes.TextureAspectAll,
		mipCount:   1,
		layerCount: 2,
	}
	selective := textureViewSubresourcePlanes(view)
	physical := textureViewPhysicalSubresourcePlanes(view)
	if want := []uint32{2, 3}; !equalSubresourceIndexes(selective, want) {
		t.Fatalf("selective stencil subresources = %#v, want %v", selective, want)
	}
	if want := []uint32{0, 1, 2, 3}; !equalSubresourceIndexes(physical, want) {
		t.Fatalf("physical DSV subresources = %#v, want %v", physical, want)
	}
}

func TestDepthStencilPlaneStates(t *testing.T) {
	shaderRead := d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE | d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE
	tests := []struct {
		name            string
		plane           uint32
		depthReadOnly   bool
		stencilReadOnly bool
		shaderReadable  bool
		want            d3d12.D3D12_RESOURCE_STATES
	}{
		{name: "depth read stencil write", plane: 0, depthReadOnly: true, shaderReadable: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ | shaderRead},
		{name: "depth write stencil read", plane: 1, stencilReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ},
		{name: "depth write stencil write", plane: 0, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE},
		{name: "stencil write", plane: 1, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE},
		{name: "both read", plane: 0, depthReadOnly: true, stencilReadOnly: true, shaderReadable: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ | shaderRead},
		{name: "sampled read-only stencil plane", plane: 1, depthReadOnly: true, stencilReadOnly: true, shaderReadable: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ | shaderRead},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := depthStencilPlaneState(test.plane, test.depthReadOnly, test.stencilReadOnly, test.shaderReadable); got != test.want {
				t.Fatalf("plane state = %d, want %d", got, test.want)
			}
		})
	}
}

func TestDepthStencilReadOnlyFlagsProtectUnexposedPlanes(t *testing.T) {
	tests := []struct {
		name        string
		format      gputypes.TextureFormat
		aspect      gputypes.TextureAspect
		depth       bool
		stencil     bool
		wantDepth   bool
		wantStencil bool
	}{
		{name: "standalone stencil protects backing depth", format: gputypes.TextureFormatStencil8, aspect: gputypes.TextureAspectAll, wantDepth: true},
		{name: "depth24plus protects backing stencil", format: gputypes.TextureFormatDepth24Plus, aspect: gputypes.TextureAspectAll, wantStencil: true},
		{name: "stencil aspect protects depth", format: gputypes.TextureFormatDepth24PlusStencil8, aspect: gputypes.TextureAspectStencilOnly, wantDepth: true},
		{name: "depth aspect protects stencil", format: gputypes.TextureFormatDepth32FloatStencil8, aspect: gputypes.TextureAspectDepthOnly, wantStencil: true},
		{name: "single-plane depth drops invalid stencil flag", format: gputypes.TextureFormatDepth32Float, aspect: gputypes.TextureAspectDepthOnly, stencil: true},
		{name: "explicit flags remain set", format: gputypes.TextureFormatDepth24PlusStencil8, aspect: gputypes.TextureAspectAll, depth: true, stencil: true, wantDepth: true, wantStencil: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			depth, stencil := depthStencilReadOnlyFlags(test.format, test.aspect, test.depth, test.stencil)
			if depth != test.wantDepth || stencil != test.wantStencil {
				t.Fatalf("read-only flags = (%v, %v), want (%v, %v)", depth, stencil, test.wantDepth, test.wantStencil)
			}
		})
	}
}

func TestDSVReadOnlyVariantsMatchPhysicalFormat(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		want   []uint32
	}{
		{name: "depth16", format: gputypes.TextureFormatDepth16Unorm, want: []uint32{1}},
		{name: "depth32", format: gputypes.TextureFormatDepth32Float, want: []uint32{1}},
		{name: "depth24 backing stencil", format: gputypes.TextureFormatDepth24Plus, want: []uint32{1, 2, 3}},
		{name: "stencil8 backing depth", format: gputypes.TextureFormatStencil8, want: []uint32{1, 2, 3}},
		{name: "depth24 stencil8", format: gputypes.TextureFormatDepth24PlusStencil8, want: []uint32{1, 2, 3}},
		{name: "depth32 stencil8", format: gputypes.TextureFormatDepth32FloatStencil8, want: []uint32{1, 2, 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dsvReadOnlyVariantMasks(test.format); !equalUint32s(got, test.want) {
				t.Fatalf("DSV read-only masks = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDSVHandleSelectionCoversLegalAndHiddenCompanionFlags(t *testing.T) {
	packed := &TextureView{
		dsvHandle:      d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{Ptr: 100},
		dsvHandles:     [4]d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{{Ptr: 100}, {Ptr: 101}, {Ptr: 102}, {Ptr: 103}},
		hasDSVVariants: [4]bool{true, true, true, true},
	}
	depth, stencil := depthStencilReadOnlyFlags(gputypes.TextureFormatStencil8, gputypes.TextureAspectAll, false, false)
	if got := packed.dsvHandleForFlags(dsvFlags(depth, stencil)); got.Ptr != 101 {
		t.Fatalf("Stencil8 hidden-depth descriptor = %d, want 101", got.Ptr)
	}
	depth, stencil = depthStencilReadOnlyFlags(gputypes.TextureFormatDepth24Plus, gputypes.TextureAspectAll, false, false)
	if got := packed.dsvHandleForFlags(dsvFlags(depth, stencil)); got.Ptr != 102 {
		t.Fatalf("Depth24Plus hidden-stencil descriptor = %d, want 102", got.Ptr)
	}
	if got := packed.dsvHandleForFlags(d3d12.D3D12_DSV_FLAG_READ_ONLY_DEPTH | d3d12.D3D12_DSV_FLAG_READ_ONLY_STENCIL); got.Ptr != 103 {
		t.Fatalf("fully read-only descriptor = %d, want 103", got.Ptr)
	}

	singlePlane := &TextureView{
		dsvHandle:      d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{Ptr: 200},
		dsvHandles:     [4]d3d12.D3D12_CPU_DESCRIPTOR_HANDLE{{Ptr: 200}, {Ptr: 201}},
		hasDSVVariants: [4]bool{true, true},
	}
	depth, stencil = depthStencilReadOnlyFlags(gputypes.TextureFormatDepth32Float, gputypes.TextureAspectDepthOnly, true, true)
	if stencil {
		t.Fatal("single-plane format retained an invalid stencil flag")
	}
	if got := singlePlane.dsvHandleForFlags(dsvFlags(depth, stencil)); got.Ptr != 201 {
		t.Fatalf("single-plane read-only descriptor = %d, want 201", got.Ptr)
	}
}

func dsvFlags(depthReadOnly, stencilReadOnly bool) d3d12.D3D12_DSV_FLAGS {
	flags := d3d12.D3D12_DSV_FLAG_NONE
	if depthReadOnly {
		flags |= d3d12.D3D12_DSV_FLAG_READ_ONLY_DEPTH
	}
	if stencilReadOnly {
		flags |= d3d12.D3D12_DSV_FLAG_READ_ONLY_STENCIL
	}
	return flags
}

func TestPlanBufferTextureCopiesSplitsArrayLayers(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatRGBA8Unorm,
		dimension: gputypes.TextureDimension2D,
		mipLevels: 2,
		size:      hal.Extent3D{DepthOrArrayLayers: 6},
	}
	plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{
		MipLevel: 1,
		Origin:   hal.Origin3D{Z: 2},
		Aspect:   gputypes.TextureAspectAll,
	}, hal.ImageDataLayout{
		Offset:       1024,
		BytesPerRow:  256,
		RowsPerImage: 4,
	}, hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2})

	want := []bufferTextureCopyPlan{
		{subresource: 5, bufferOffset: 1024, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 0},
		{subresource: 7, bufferOffset: 2048, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 0},
	}
	if !equalBufferTextureCopyPlans(plans, want) {
		t.Fatalf("array copy plans = %#v, want %#v", plans, want)
	}
}

func TestPlanBufferTextureCopiesAlignsPlacedFootprintsByWholeRows(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatRGBA8Unorm,
		dimension: gputypes.TextureDimension2D,
		mipLevels: 1,
		size:      hal.Extent3D{DepthOrArrayLayers: 2},
	}
	plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{
		Aspect: gputypes.TextureAspectAll,
	}, hal.ImageDataLayout{
		Offset:       512,
		BytesPerRow:  768,
		RowsPerImage: 3,
	}, hal.Extent3D{Width: 8, Height: 2, DepthOrArrayLayers: 2})

	want := []bufferTextureCopyPlan{
		{subresource: 0, bufferOffset: 512, footprintHeight: 2, footprintDepth: 1},
		{subresource: 1, bufferOffset: 2048, bufferOriginY: 1, footprintHeight: 3, footprintDepth: 1},
	}
	if !equalBufferTextureCopyPlans(plans, want) {
		t.Fatalf("array copy plans = %#v, want %#v", plans, want)
	}
	if got := plans[1].bufferOffset + uint64(plans[1].bufferOriginY)*768; got != 2816 {
		t.Fatalf("second logical layer offset = %d, want 2816", got)
	}
}

func TestPlanBufferTextureCopiesUsesBlockRowsForCompressedLayers(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatBC1RGBAUnorm,
		dimension: gputypes.TextureDimension2D,
		mipLevels: 1,
		size:      hal.Extent3D{DepthOrArrayLayers: 2},
	}
	plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{
		Aspect: gputypes.TextureAspectAll,
	}, hal.ImageDataLayout{
		Offset:       512,
		BytesPerRow:  256,
		RowsPerImage: 4,
	}, hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 2})

	want := []bufferTextureCopyPlan{
		{subresource: 0, bufferOffset: 512, footprintHeight: 4, footprintDepth: 1},
		{subresource: 1, bufferOffset: 512, bufferOriginY: 4, footprintHeight: 8, footprintDepth: 1},
	}
	if !equalBufferTextureCopyPlans(plans, want) {
		t.Fatalf("compressed array copy plans = %#v, want %#v", plans, want)
	}
}

func TestPlanBufferTextureCopiesKeeps3DDepthInOneSubresource(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatRGBA8Unorm,
		dimension: gputypes.TextureDimension3D,
		mipLevels: 3,
		size:      hal.Extent3D{DepthOrArrayLayers: 16},
	}
	plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{
		MipLevel: 2,
		Origin:   hal.Origin3D{Z: 4},
		Aspect:   gputypes.TextureAspectAll,
	}, hal.ImageDataLayout{Offset: 1024, BytesPerRow: 256, RowsPerImage: 4}, hal.Extent3D{
		Width: 8, Height: 4, DepthOrArrayLayers: 3,
	})

	want := []bufferTextureCopyPlan{
		{subresource: 2, bufferOffset: 1024, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 4},
		{subresource: 2, bufferOffset: 2048, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 5},
		{subresource: 2, bufferOffset: 3072, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 6},
	}
	if !equalBufferTextureCopyPlans(plans, want) {
		t.Fatalf("3D copy plans = %#v, want %#v", plans, want)
	}
}

func TestPlanBufferTextureCopiesPreserves3DRowsPerImageAsSliceStride(t *testing.T) {
	texture := &Texture{
		format:    gputypes.TextureFormatRGBA8Unorm,
		dimension: gputypes.TextureDimension3D,
		mipLevels: 1,
		size:      hal.Extent3D{DepthOrArrayLayers: 8},
	}
	plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{
		Aspect: gputypes.TextureAspectAll,
	}, hal.ImageDataLayout{Offset: 512, BytesPerRow: 256, RowsPerImage: 8}, hal.Extent3D{
		Width: 8, Height: 4, DepthOrArrayLayers: 3,
	})
	want := []bufferTextureCopyPlan{
		{subresource: 0, bufferOffset: 512, footprintHeight: 4, footprintDepth: 1},
		{subresource: 0, bufferOffset: 2560, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 1},
		{subresource: 0, bufferOffset: 4608, footprintHeight: 4, footprintDepth: 1, textureOriginZ: 2},
	}
	if !equalBufferTextureCopyPlans(plans, want) {
		t.Fatalf("3D padded-row copy plans = %#v, want %#v", plans, want)
	}
}

func TestPlanBufferTextureCopiesKeepsMinimum3DBufferBounds(t *testing.T) {
	tests := []struct {
		name         string
		format       gputypes.TextureFormat
		width        uint32
		height       uint32
		rowsPerImage uint32
		rowBytes     uint64
		wantEnd      uint64
	}{
		{name: "rgba8", format: gputypes.TextureFormatRGBA8Unorm, width: 1, height: 1, rowsPerImage: 2, rowBytes: 4, wantEnd: 1028},
		{name: "bc1", format: gputypes.TextureFormatBC1RGBAUnorm, width: 4, height: 4, rowsPerImage: 8, rowBytes: 8, wantEnd: 1032},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			texture := &Texture{format: test.format, dimension: gputypes.TextureDimension3D, mipLevels: 1}
			plans := planBufferTextureCopies(texture, hal.ImageCopyTexture{Aspect: gputypes.TextureAspectAll}, hal.ImageDataLayout{
				Offset: 512, BytesPerRow: 256, RowsPerImage: test.rowsPerImage,
			}, hal.Extent3D{Width: test.width, Height: test.height, DepthOrArrayLayers: 2})
			if len(plans) != 2 {
				t.Fatalf("3D slice plans = %#v, want 2", plans)
			}
			last := plans[1]
			if last.footprintDepth != 1 {
				t.Fatalf("last footprint depth = %d, want 1", last.footprintDepth)
			}
			logicalStart := last.bufferOffset + uint64(last.bufferOriginY/textureFormatBlockHeight(test.format))*256
			if got := logicalStart + test.rowBytes; got != test.wantEnd {
				t.Fatalf("last copied byte end = %d, want minimum buffer end %d", got, test.wantEnd)
			}
		})
	}
}

func TestPlanTextureCopiesSplitsArraysAndUsesAbsolute3DBack(t *testing.T) {
	arraySrc := &Texture{format: gputypes.TextureFormatRGBA8Unorm, dimension: gputypes.TextureDimension2D, mipLevels: 2, size: hal.Extent3D{DepthOrArrayLayers: 6}}
	arrayDst := &Texture{format: gputypes.TextureFormatRGBA8Unorm, dimension: gputypes.TextureDimension2D, mipLevels: 2, size: hal.Extent3D{DepthOrArrayLayers: 8}}
	arrayPlans := planTextureTextureCopies(arraySrc, arrayDst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 1}, Aspect: gputypes.TextureAspectAll},
		DstBase: hal.ImageCopyTexture{Origin: hal.Origin3D{Z: 3}, Aspect: gputypes.TextureAspectAll},
		Size:    hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 2},
	})
	arrayWant := []textureTextureCopyPlan{
		{srcSubresource: 2, dstSubresource: 6, srcFront: 0, srcBack: 1, dstZ: 0},
		{srcSubresource: 4, dstSubresource: 8, srcFront: 0, srcBack: 1, dstZ: 0},
	}
	if !equalTextureTextureCopyPlans(arrayPlans, arrayWant) {
		t.Fatalf("array texture plans = %#v, want %#v", arrayPlans, arrayWant)
	}

	volumeSrc := &Texture{format: gputypes.TextureFormatRGBA8Unorm, dimension: gputypes.TextureDimension3D, mipLevels: 3}
	volumeDst := &Texture{format: gputypes.TextureFormatRGBA8Unorm, dimension: gputypes.TextureDimension3D, mipLevels: 3}
	volumePlans := planTextureTextureCopies(volumeSrc, volumeDst, hal.TextureCopy{
		SrcBase: hal.ImageCopyTexture{MipLevel: 1, Origin: hal.Origin3D{Z: 4}, Aspect: gputypes.TextureAspectAll},
		DstBase: hal.ImageCopyTexture{MipLevel: 2, Origin: hal.Origin3D{Z: 7}, Aspect: gputypes.TextureAspectAll},
		Size:    hal.Extent3D{Width: 8, Height: 4, DepthOrArrayLayers: 3},
	})
	volumeWant := []textureTextureCopyPlan{{srcSubresource: 1, dstSubresource: 2, srcFront: 4, srcBack: 7, dstZ: 7}}
	if !equalTextureTextureCopyPlans(volumePlans, volumeWant) {
		t.Fatalf("3D texture plans = %#v, want %#v", volumePlans, volumeWant)
	}
}

func equalUint32s(got, want []uint32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func equalSubresourceIndexes(got []textureSubresource, want []uint32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].subresource != want[i] {
			return false
		}
	}
	return true
}

func equalBufferTextureCopyPlans(got, want []bufferTextureCopyPlan) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func equalTextureTextureCopyPlans(got, want []textureTextureCopyPlan) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSubmissionPlannerReconcilesActualOrderWithoutMutatingInput(t *testing.T) {
	const resource = "texture"
	common := d3d12.D3D12_RESOURCE_STATE_COMMON
	read := d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE
	write := d3d12.D3D12_RESOURCE_STATE_COPY_DEST

	first := commandStateSummary{commandIndex: 0, resource: resource, states: []subresourceState{{subresource: 0, initial: common, current: read}}}
	second := commandStateSummary{commandIndex: 1, resource: resource, states: []subresourceState{{subresource: 0, initial: common, current: write}}}
	scheduled := map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES{resource: {0: common}}

	preambles, final := planSubmissionState(scheduled, []commandStateSummary{first, second})
	if len(preambles) != 2 || len(preambles[0]) != 0 || len(preambles[1]) != 1 {
		t.Fatalf("preambles = %#v, want [none, one transition]", preambles)
	}
	transition := preambles[1][0]
	if transition.before != read || transition.after != common {
		t.Fatalf("second preamble = %#v, want READ -> COMMON", transition)
	}
	if got := final[resource][0]; got != write {
		t.Fatalf("final scheduled state = %d, want COPY_DEST", got)
	}
	if got := scheduled[resource][0]; got != common {
		t.Fatalf("input scheduled state mutated to %d, want COMMON", got)
	}
}

func TestSubmissionPlannerDecaysBuffersOnlyAfterExecuteBoundary(t *testing.T) {
	buffer := &Buffer{}
	common := d3d12.D3D12_RESOURCE_STATE_COMMON
	write := d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	read := d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE
	commands := []commandStateSummary{
		{commandIndex: 0, resource: buffer, states: []subresourceState{{subresource: 0, initial: common, current: write}}},
		{commandIndex: 1, resource: buffer, states: []subresourceState{{subresource: 0, initial: common, current: read}}},
	}

	preambles, final := planSubmissionState(
		map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES{buffer: {0: common}},
		commands,
	)
	if len(preambles) != 2 || len(preambles[0]) != 0 || len(preambles[1]) != 1 {
		t.Fatalf("preambles = %#v, want second-list reconciliation only", preambles)
	}
	if got := preambles[1][0]; got.before != write || got.after != common {
		t.Fatalf("second-list preamble = %#v, want COPY_DEST -> COMMON", got)
	}
	if got := final[buffer][0]; got != common {
		t.Fatalf("post-execute buffer state = %d, want COMMON", got)
	}

	next, _ := planSubmissionState(final, []commandStateSummary{{
		commandIndex: 0,
		resource:     buffer,
		states:       []subresourceState{{subresource: 0, initial: common, current: read}},
	}})
	if len(next) != 1 || len(next[0]) != 0 {
		t.Fatalf("next execute preambles = %#v, want no transition from decayed COMMON", next)
	}
}

func TestResourceStateSnapshotsAndCommitsAreConcurrentSafe(t *testing.T) {
	buffer := &Buffer{currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}
	texture := &Texture{currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}
	states := []d3d12.D3D12_RESOURCE_STATES{
		d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
		d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
		d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE,
	}

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				state := states[(i+offset)%len(states)]
				buffer.commitScheduledState(state)
				_ = buffer.scheduledStateSnapshot()
				texture.commitScheduledStates([]d3d12.D3D12_RESOURCE_STATES{state})
				_ = texture.scheduledStateSnapshot(0)
				_ = texture.scheduledStateSnapshotAll()
			}
		}(worker)
	}
	wg.Wait()
}

type fakePreambleEvent struct {
	op string
	id uint64
}

type fakePreambleNativeOps struct {
	nextID                 uint64
	createFailures         int
	resetAllocatorFailures int
	resetListFailures      int
	closeFailures          int
	events                 []fakePreambleEvent
	releases               map[uint64]int
}

func (f *fakePreambleNativeOps) create(*Device) (preambleInFlight, error) {
	f.nextID++
	f.events = append(f.events, fakePreambleEvent{op: "create", id: f.nextID})
	if f.createFailures > 0 {
		f.createFailures--
		return preambleInFlight{}, errors.New("create pair")
	}
	return preambleInFlight{testID: f.nextID}, nil
}

func (f *fakePreambleNativeOps) resetAllocator(preamble preambleInFlight) error {
	f.events = append(f.events, fakePreambleEvent{op: "reset allocator", id: preamble.testID})
	if f.resetAllocatorFailures > 0 {
		f.resetAllocatorFailures--
		return errors.New("reset allocator")
	}
	return nil
}

func (f *fakePreambleNativeOps) resetCommandList(preamble preambleInFlight) error {
	f.events = append(f.events, fakePreambleEvent{op: "reset list", id: preamble.testID})
	if f.resetListFailures > 0 {
		f.resetListFailures--
		return errors.New("reset list")
	}
	return nil
}

func (f *fakePreambleNativeOps) recordAndClose(preamble preambleInFlight, _ []stateBarrierPlan) error {
	f.events = append(f.events, fakePreambleEvent{op: "close", id: preamble.testID})
	if f.closeFailures > 0 {
		f.closeFailures--
		return errors.New("close list")
	}
	return nil
}

func (f *fakePreambleNativeOps) release(preamble preambleInFlight) {
	f.events = append(f.events, fakePreambleEvent{op: "release", id: preamble.testID})
	if f.releases == nil {
		f.releases = make(map[uint64]int)
	}
	f.releases[preamble.testID]++
}

func TestPreamblePoolWaitsForFenceThenReusesPair(t *testing.T) {
	ops := &fakePreambleNativeOps{}
	queue := &Queue{state: &queueState{preambleOps: ops}}

	first, err := queue.buildPreamble(nil)
	if err != nil || first.testID != 1 {
		t.Fatalf("first preamble = (%+v, %v), want fresh pair 1", first, err)
	}
	first.submission = 4
	queue.state.preambleInFlight = append(queue.state.preambleInFlight, first)
	queue.releaseCompletedPreambles(3)
	if len(queue.state.preambleInFlight) != 1 || len(queue.state.preambleIdle) != 0 {
		t.Fatal("preamble became reusable before its fence completed")
	}
	queue.releaseCompletedPreambles(4)
	queue.releaseCompletedPreambles(4) // polling is idempotent
	if len(queue.state.preambleInFlight) != 0 || len(queue.state.preambleIdle) != 1 || len(ops.releases) != 0 {
		t.Fatalf("completed pool state = in-flight %d idle %d releases %v", len(queue.state.preambleInFlight), len(queue.state.preambleIdle), ops.releases)
	}

	second, err := queue.buildPreamble(nil)
	if err != nil || second.testID != first.testID {
		t.Fatalf("reused preamble = (%+v, %v), want pair %d", second, err, first.testID)
	}
	want := []fakePreambleEvent{
		{op: "create", id: 1},
		{op: "close", id: 1},
		{op: "reset allocator", id: 1},
		{op: "reset list", id: 1},
		{op: "close", id: 1},
	}
	if !equalPreambleEvents(ops.events, want) {
		t.Fatalf("native events = %#v, want %#v", ops.events, want)
	}
}

func TestPreamblePoolDiscardsFailedReusablePairAndFallsBackFresh(t *testing.T) {
	tests := []struct {
		name           string
		resetAllocator int
		resetList      int
		close          int
		wantOps        []string
	}{
		{name: "allocator reset", resetAllocator: 1, wantOps: []string{"reset allocator", "release", "create", "close"}},
		{name: "list reset", resetList: 1, wantOps: []string{"reset allocator", "reset list", "release", "create", "close"}},
		{name: "close", close: 1, wantOps: []string{"reset allocator", "reset list", "close", "release", "create", "close"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ops := &fakePreambleNativeOps{
				nextID:                 10,
				resetAllocatorFailures: test.resetAllocator,
				resetListFailures:      test.resetList,
				closeFailures:          test.close,
			}
			queue := &Queue{state: &queueState{
				preambleOps:  ops,
				preambleIdle: []preambleInFlight{{testID: 10}},
			}}
			got, err := queue.buildPreamble(nil)
			if err != nil || got.testID != 11 {
				t.Fatalf("fallback preamble = (%+v, %v), want fresh pair 11", got, err)
			}
			if ops.releases[10] != 1 {
				t.Fatalf("failed pair releases = %v, want pair 10 exactly once", ops.releases)
			}
			if gotOps := preambleEventOps(ops.events); !equalStrings(gotOps, test.wantOps) {
				t.Fatalf("native operations = %v, want %v", gotOps, test.wantOps)
			}
		})
	}
}

func TestPreambleBuildFailuresReleaseEveryCreatedPairOnce(t *testing.T) {
	t.Run("fresh create", func(t *testing.T) {
		ops := &fakePreambleNativeOps{createFailures: 1}
		queue := &Queue{state: &queueState{preambleOps: ops}}
		if _, err := queue.buildPreamble(nil); err == nil {
			t.Fatal("fresh create failure succeeded")
		}
		if len(ops.releases) != 0 {
			t.Fatalf("create failure releases = %v, want none", ops.releases)
		}
	})

	t.Run("fresh close", func(t *testing.T) {
		ops := &fakePreambleNativeOps{closeFailures: 1}
		queue := &Queue{state: &queueState{preambleOps: ops}}
		if _, err := queue.buildPreamble(nil); err == nil {
			t.Fatal("fresh close failure succeeded")
		}
		if ops.releases[1] != 1 {
			t.Fatalf("fresh close releases = %v, want pair 1 once", ops.releases)
		}
	})

	t.Run("later preamble", func(t *testing.T) {
		ops := &fakePreambleNativeOps{}
		queue := &Queue{state: &queueState{preambleOps: ops}}
		first, err := queue.buildPreamble(nil)
		if err != nil {
			t.Fatalf("first preamble: %v", err)
		}
		ops.createFailures = 1
		if _, err := queue.buildPreamble(nil); err == nil {
			t.Fatal("second preamble create failure succeeded")
		}
		queue.releaseBuiltPreambles([]preambleInFlight{first})
		if ops.releases[first.testID] != 1 {
			t.Fatalf("partial unwind releases = %v, want first pair once", ops.releases)
		}
	})
}

func TestPreamblePoolCapsIdlePairsAndRetainsUnfencedPairs(t *testing.T) {
	ops := &fakePreambleNativeOps{}
	queue := &Queue{state: &queueState{preambleOps: ops}}
	for id := uint64(1); id <= maxFramesInFlight+2; id++ {
		queue.state.preambleInFlight = append(queue.state.preambleInFlight, preambleInFlight{submission: 7, testID: id})
	}
	queue.retainPreamblesWithoutFence([]preambleInFlight{{testID: 99}})
	queue.releaseCompletedPreambles(7)
	queue.releaseCompletedPreambles(7)

	if len(queue.state.preambleIdle) != maxFramesInFlight {
		t.Fatalf("idle pool size = %d, want cap %d", len(queue.state.preambleIdle), maxFramesInFlight)
	}
	if got := queue.state.preambleInFlight; len(got) != 1 || got[0].submission != 0 || got[0].testID != 99 {
		t.Fatalf("retained unfenced pairs = %+v, want pair 99 at submission 0", got)
	}
	if len(ops.releases) != 2 {
		t.Fatalf("excess releases = %v, want two pairs", ops.releases)
	}

	queue.state.releaseAllOwnedLocked()
	if len(queue.state.preambleIdle) != 0 || len(queue.state.preambleInFlight) != 0 {
		t.Fatal("pool destruction retained owned pairs")
	}
	for id, count := range ops.releases {
		if count != 1 {
			t.Fatalf("pair %d release count = %d, want 1", id, count)
		}
	}
}

func TestTerminalPreambleReleaseKeepsOnlyAmbiguousInFlightPairs(t *testing.T) {
	tests := []struct {
		name          string
		waitErr       error
		deviceRemoved bool
		wantInFlight  int
		wantIdle      int
	}{
		{name: "successful wait"},
		{name: "confirmed removal", waitErr: errors.New("device removed"), deviceRemoved: true},
		{name: "ambiguous wait", waitErr: errors.New("event wait failed"), wantInFlight: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ops := &fakePreambleNativeOps{}
			state := &queueState{
				preambleOps:      ops,
				preambleIdle:     []preambleInFlight{{testID: 1}},
				preambleInFlight: []preambleInFlight{{submission: 2, testID: 2}},
			}
			state.releaseTerminalOwnedLocked(test.waitErr, test.deviceRemoved)
			if len(state.preambleInFlight) != test.wantInFlight || len(state.preambleIdle) != test.wantIdle {
				t.Fatalf("terminal state = in-flight %d idle %d, want %d/%d", len(state.preambleInFlight), len(state.preambleIdle), test.wantInFlight, test.wantIdle)
			}
			if ops.releases[1] != 1 {
				t.Fatalf("idle pair release count = %d, want 1", ops.releases[1])
			}
			if test.wantInFlight == 0 && ops.releases[2] != 1 {
				t.Fatalf("safe in-flight pair release count = %d, want 1", ops.releases[2])
			}
			if test.wantInFlight == 1 && ops.releases[2] != 0 {
				t.Fatal("ambiguous in-flight pair was released")
			}
		})
	}
}

func equalPreambleEvents(got, want []fakePreambleEvent) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func preambleEventOps(events []fakePreambleEvent) []string {
	result := make([]string, len(events))
	for i, event := range events {
		result[i] = event.op
	}
	return result
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestQueueStateReleasesTrackedAndUnfencedOwnedObjects(t *testing.T) {
	device := &Device{}
	queue := newQueue(device)
	if device.queueState == nil || device.queueState != queue.state {
		t.Fatal("device and queue do not share their noncyclic lifetime state")
	}
	queue.state.preambleInFlight = []preambleInFlight{
		{submission: 7},
		{submission: 0},
	}
	queue.state.oneShotsInFlight = []oneShotInFlight{
		{submission: 7},
		{submission: 0},
	}

	queue.state.releaseAllOwnedLocked()
	if len(queue.state.preambleInFlight) != 0 {
		t.Fatalf("preambles after drain = %d, want 0", len(queue.state.preambleInFlight))
	}
	if len(queue.state.oneShotsInFlight) != 0 {
		t.Fatalf("one-shots after drain = %d, want 0", len(queue.state.oneShotsInFlight))
	}
}

func TestCompletedOneShotsReleaseOnlyFenceTrackedOwnership(t *testing.T) {
	queue := &Queue{state: &queueState{oneShotsInFlight: []oneShotInFlight{
		{submission: 4},
		{submission: 0},
		{submission: 6},
	}}}

	queue.releaseCompletedOneShots(4)
	if got := queue.state.oneShotsInFlight; len(got) != 2 || got[0].submission != 0 || got[1].submission != 6 {
		t.Fatalf("retained one-shots = %+v, want submissions [0 6]", got)
	}
}

func TestOneShotDetachTransfersNativeOwnershipWithoutDeviceBackReferences(t *testing.T) {
	stagingRaw := &d3d12.ID3D12Resource{}
	destinationRaw := &d3d12.ID3D12Resource{}
	allocatorRaw := &d3d12.ID3D12CommandAllocator{}
	cmdListRaw := &d3d12.ID3D12GraphicsCommandList{}
	destination := &Buffer{raw: destinationRaw}
	owner := &oneShotWriteOwner{
		staging:       &Buffer{raw: stagingRaw},
		destination:   destinationRaw,
		encoder:       &CommandEncoder{allocator: allocatorRaw},
		commandBuffer: &CommandBuffer{cmdList: cmdListRaw},
	}

	retained := owner.detach(11)
	if retained.submission != 11 || retained.staging != stagingRaw || retained.destination != destinationRaw || retained.allocator != allocatorRaw || retained.cmdList != cmdListRaw {
		t.Fatalf("detached ownership = %+v, want all native objects at submission 11", retained)
	}
	if owner.staging.raw != nil || owner.destination != nil || owner.encoder.allocator != nil || owner.commandBuffer.cmdList != nil {
		t.Fatal("wrapper ownership remained after detach")
	}
	// Deferred local cleanup runs after detach in the error paths. It must be a
	// no-op for every transferred reference rather than double-releasing it.
	owner.release()
	if destination.raw != destinationRaw {
		t.Fatal("detach stole the caller's destination wrapper reference")
	}
}

func TestClosedQueueOperationsRejectBeforeTouchingDevice(t *testing.T) {
	queue := &Queue{state: &queueState{closed: true}}
	if _, err := queue.Submit(nil); err == nil {
		t.Fatal("Submit on closed queue succeeded")
	}
	if err := queue.WriteBuffer(nil, 0, nil); err == nil {
		t.Fatal("WriteBuffer on closed queue succeeded")
	}
	if err := queue.WriteTexture(nil, nil, nil, nil); err == nil {
		t.Fatal("WriteTexture on closed queue succeeded")
	}
	if err := queue.Present(nil, nil, nil); err == nil {
		t.Fatal("Present on closed queue succeeded")
	}
	if got := queue.PollCompleted(); got != 0 {
		t.Fatalf("PollCompleted on closed queue = %d, want 0", got)
	}
	if got := queue.GetTimestampPeriod(); got != 1.0 {
		t.Fatalf("GetTimestampPeriod on closed queue = %f, want fallback 1.0", got)
	}
}

func TestClosedDeviceWaitIdleRejectsBeforeTouchingNativeDevice(t *testing.T) {
	device := &Device{queueState: &queueState{closed: true}}
	if err := device.WaitIdle(); err == nil {
		t.Fatal("WaitIdle on closed device succeeded")
	}
}

func TestQueueCloseExcludesConcurrentOperation(t *testing.T) {
	state := &queueState{}
	queue := &Queue{state: state}
	started := make(chan struct{})
	result := make(chan error, 1)

	state.submitMu.Lock()
	go func() {
		close(started)
		result <- queue.lockOpen()
	}()
	<-started
	state.closed = true
	state.submitMu.Unlock()

	if err := <-result; err == nil {
		// lockOpen retains the mutex on success; avoid poisoning the test
		// process if the exclusion contract regresses.
		state.submitMu.Unlock()
		t.Fatal("operation entered after queue close")
	}
}

func TestTerminalOwnedObjectReleaseRequiresProofOfCompletion(t *testing.T) {
	waitErr := errors.New("event wait failed")
	if !shouldReleaseTerminalOwnedObjects(nil, false) {
		t.Fatal("successful idle wait did not release owned objects")
	}
	if !shouldReleaseTerminalOwnedObjects(waitErr, true) {
		t.Fatal("confirmed device removal did not release owned objects")
	}
	if shouldReleaseTerminalOwnedObjects(waitErr, false) {
		t.Fatal("ambiguous idle failure released owned objects")
	}
}

func TestFailTextureViewCreationRecyclesAllocatedDescriptors(t *testing.T) {
	device := &Device{
		stagingViewHeap: &DescriptorHeap{},
		rtvHeap:         &DescriptorHeap{},
		dsvHeap:         &DescriptorHeap{},
	}
	view := &TextureView{
		texture:        &Texture{},
		device:         device,
		hasSRV:         true,
		hasRTV:         true,
		hasDSV:         true,
		hasDSVVariants: [4]bool{true, true, true, true},
		srvHeapIndex:   11,
		rtvHeapIndex:   12,
		dsvHeapIndex:   [4]uint32{20, 21, 22, 23},
	}
	wantErr := errors.New("descriptor allocation failed")
	gotView, gotErr := failTextureViewCreation(view, wantErr)
	if gotView != nil || !errors.Is(gotErr, wantErr) {
		t.Fatalf("failure result = (%v, %v), want (nil, %v)", gotView, gotErr, wantErr)
	}
	if want := []uint32{11}; !equalUint32s(device.stagingViewHeap.freeList, want) {
		t.Fatalf("freed SRVs = %v, want %v", device.stagingViewHeap.freeList, want)
	}
	if want := []uint32{12}; !equalUint32s(device.rtvHeap.freeList, want) {
		t.Fatalf("freed RTVs = %v, want %v", device.rtvHeap.freeList, want)
	}
	if want := []uint32{20, 21, 22, 23}; !equalUint32s(device.dsvHeap.freeList, want) {
		t.Fatalf("freed DSVs = %v, want %v", device.dsvHeap.freeList, want)
	}
}

func TestTextureViewDestroyRecyclesOnlyAllocatedDSVVariantsOnce(t *testing.T) {
	device := &Device{dsvHeap: &DescriptorHeap{}}
	view := &TextureView{
		texture:        &Texture{},
		device:         device,
		hasDSV:         true,
		hasDSVVariants: [4]bool{true, true},
		dsvHeapIndex:   [4]uint32{20, 21},
	}
	view.Destroy()
	view.Destroy()
	if want := []uint32{20, 21}; !equalUint32s(device.dsvHeap.freeList, want) {
		t.Fatalf("freed partial DSV variants = %v, want %v", device.dsvHeap.freeList, want)
	}
}
