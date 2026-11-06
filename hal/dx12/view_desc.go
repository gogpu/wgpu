// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// viewRange is the subresource range of a texture view that its D3D12 view
// descriptors cover.
type viewRange struct {
	dim          gputypes.TextureViewDimension
	baseMip      uint32
	mipCount     uint32
	baseLayer    uint32
	layerCount   uint32
	multisampled bool
}

// rtvDesc builds the render target view descriptor of a 1D, 2D or 2D-array
// view. A 2D view uses the plain TEXTURE2D (or TEXTURE2DMS) form only when
// its base layer is 0; a 2D view of any other layer is a one-layer
// TEXTURE2DARRAY (or TEXTURE2DMSARRAY) starting at that layer, since the
// plain forms always address layer 0.
//
// Reference: Rust wgpu-hal dx12/view.rs ViewDescriptor::to_rtv.
func rtvDesc(format d3d12.DXGI_FORMAT, r viewRange) d3d12.D3D12_RENDER_TARGET_VIEW_DESC {
	desc := d3d12.D3D12_RENDER_TARGET_VIEW_DESC{Format: format}
	switch {
	case r.dim == gputypes.TextureViewDimension1D:
		desc.SetTexture1D(r.baseMip)
	case r.dim == gputypes.TextureViewDimension2D && r.baseLayer == 0 && r.multisampled:
		desc.SetTexture2DMS()
	case r.dim == gputypes.TextureViewDimension2D && r.baseLayer == 0:
		desc.SetTexture2D(r.baseMip, 0)
	case r.multisampled:
		desc.SetTexture2DMSArray(r.baseLayer, r.layerCount)
	default:
		desc.SetTexture2DArray(r.baseMip, r.baseLayer, r.layerCount, 0)
	}
	return desc
}

// dsvDesc builds the depth/stencil view descriptor of a 1D, 2D or 2D-array
// view, with the same layer rule as rtvDesc.
//
// Reference: Rust wgpu-hal dx12/view.rs ViewDescriptor::to_dsv.
func dsvDesc(format d3d12.DXGI_FORMAT, r viewRange) d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC {
	desc := d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC{Format: format}
	switch {
	case r.dim == gputypes.TextureViewDimension1D:
		desc.SetTexture1D(r.baseMip)
	case r.dim == gputypes.TextureViewDimension2D && r.baseLayer == 0 && r.multisampled:
		desc.SetTexture2DMS()
	case r.dim == gputypes.TextureViewDimension2D && r.baseLayer == 0:
		desc.SetTexture2D(r.baseMip)
	case r.multisampled:
		desc.SetTexture2DMSArray(r.baseLayer, r.layerCount)
	default:
		desc.SetTexture2DArray(r.baseMip, r.baseLayer, r.layerCount)
	}
	return desc
}

// srvDesc builds the shader resource view descriptor of a view. A 2D view
// follows the layer rule of rtvDesc. A cube view uses TEXTURECUBE only when
// its base layer is 0; otherwise it is a one-cube TEXTURECUBEARRAY whose
// First2DArrayFace is the base layer. First2DArrayFace counts faces (array
// layers), not cubes.
//
// Reference: Rust wgpu-hal dx12/view.rs ViewDescriptor::to_srv.
func srvDesc(format d3d12.DXGI_FORMAT, plane uint32, r viewRange) d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC {
	desc := d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC{
		Format:                  format,
		Shader4ComponentMapping: srvComponentMapping(format),
	}
	switch r.dim {
	case gputypes.TextureViewDimension1D:
		desc.SetTexture1D(r.baseMip, r.mipCount, 0)
	case gputypes.TextureViewDimension2D, gputypes.TextureViewDimension2DArray:
		srv2D(&desc, plane, r)
	case gputypes.TextureViewDimension3D:
		desc.SetTexture3D(r.baseMip, r.mipCount, 0)
	case gputypes.TextureViewDimensionCube, gputypes.TextureViewDimensionCubeArray:
		if r.dim == gputypes.TextureViewDimensionCube && r.baseLayer == 0 {
			desc.SetTextureCube(r.baseMip, r.mipCount, 0)
		} else {
			desc.SetTextureCubeArray(r.baseMip, r.mipCount, r.baseLayer, r.layerCount/6, 0)
		}
	default:
		srv2D(&desc, plane, r)
	}
	return desc
}

// stencilSRVComponentMapping maps (G, 0, 0, 1): red reads memory component 1,
// green and blue are forced to 0, alpha to 1, plus the always-set bit
// (D3D12_ENCODE_SHADER_4_COMPONENT_MAPPING(1, 4, 4, 5)).
const stencilSRVComponentMapping = 1 | 4<<3 | 4<<6 | 5<<9 | 1<<12

// srvComponentMapping returns the SRV component mapping of format. The stencil
// SRV formats X24_TYPELESS_G8_UINT and X32_TYPELESS_G8X24_UINT hold the
// stencil value in green, while WebGPU reads it from red with (0, 0, 1) in the
// other channels, so their views swizzle (G, 0, 0, 1). Without it a
// stencil-only view samples 0.
//
// Reference: Rust wgpu-hal dx12/view.rs ViewDescriptor::to_srv, which swizzles
// the same way for the STENCIL aspect.
func srvComponentMapping(format d3d12.DXGI_FORMAT) uint32 {
	switch format {
	case d3d12.DXGI_FORMAT_X24_TYPELESS_G8_UINT, d3d12.DXGI_FORMAT_X32_TYPELESS_G8X24_UINT:
		return stencilSRVComponentMapping
	}
	return d3d12.D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING
}

// srv2D fills desc for a 2D or 2D-array view (see srvDesc).
func srv2D(desc *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC, plane uint32, r viewRange) {
	single := r.dim != gputypes.TextureViewDimension2DArray && r.baseLayer == 0
	switch {
	case single && r.multisampled:
		desc.SetTexture2DMS()
	case single:
		desc.SetTexture2D(r.baseMip, r.mipCount, plane, 0)
	case r.multisampled:
		desc.SetTexture2DMSArray(r.baseLayer, r.layerCount)
	default:
		desc.SetTexture2DArray(r.baseMip, r.mipCount, r.baseLayer, r.layerCount, plane, 0)
	}
}
