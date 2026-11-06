// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

const viewDescTestFormat = d3d12.DXGI_FORMAT_R8G8B8A8_UNORM

// view2D is a one-layer, one-mip view of layer `layer` at mip `mip`.
func view2D(dim gputypes.TextureViewDimension, mip, layer uint32, ms bool) viewRange {
	return viewRange{dim: dim, baseMip: mip, mipCount: 1, baseLayer: layer, layerCount: 1, multisampled: ms}
}

// TestRTVDescLayer checks the render target view descriptor of 2D and
// 2D-array views. The plain TEXTURE2D and TEXTURE2DMS forms address layer
// 0 only, so a 2D view of any other layer must be a one-layer array view
// starting at that layer (Rust wgpu-hal dx12/view.rs to_rtv).
func TestRTVDescLayer(t *testing.T) {
	rtv := func(set func(*d3d12.D3D12_RENDER_TARGET_VIEW_DESC)) d3d12.D3D12_RENDER_TARGET_VIEW_DESC {
		d := d3d12.D3D12_RENDER_TARGET_VIEW_DESC{Format: viewDescTestFormat}
		set(&d)
		return d
	}
	tests := []struct {
		name string
		r    viewRange
		want d3d12.D3D12_RENDER_TARGET_VIEW_DESC
	}{
		{"2d layer 0", view2D(gputypes.TextureViewDimension2D, 1, 0, false),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture2D(1, 0) })},
		{"2d layer 3", view2D(gputypes.TextureViewDimension2D, 1, 3, false),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture2DArray(1, 3, 1, 0) })},
		{"2d-array layer 3", view2D(gputypes.TextureViewDimension2DArray, 0, 3, false),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture2DArray(0, 3, 1, 0) })},
		{"2d ms layer 0", view2D(gputypes.TextureViewDimension2D, 0, 0, true),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture2DMS() })},
		{"2d ms layer 2", view2D(gputypes.TextureViewDimension2D, 0, 2, true),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture2DMSArray(2, 1) })},
		{"1d", view2D(gputypes.TextureViewDimension1D, 2, 0, false),
			rtv(func(d *d3d12.D3D12_RENDER_TARGET_VIEW_DESC) { d.SetTexture1D(2) })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rtvDesc(viewDescTestFormat, tt.r); got != tt.want {
				t.Errorf("rtvDesc = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestDSVDescLayer is TestRTVDescLayer for depth/stencil views (to_dsv).
func TestDSVDescLayer(t *testing.T) {
	const format = d3d12.DXGI_FORMAT_D32_FLOAT
	dsv := func(set func(*d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC)) d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC {
		d := d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC{Format: format}
		set(&d)
		return d
	}
	tests := []struct {
		name string
		r    viewRange
		want d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC
	}{
		{"2d layer 0", view2D(gputypes.TextureViewDimension2D, 1, 0, false),
			dsv(func(d *d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC) { d.SetTexture2D(1) })},
		{"2d layer 3", view2D(gputypes.TextureViewDimension2D, 1, 3, false),
			dsv(func(d *d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC) { d.SetTexture2DArray(1, 3, 1) })},
		{"2d ms layer 0", view2D(gputypes.TextureViewDimension2D, 0, 0, true),
			dsv(func(d *d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC) { d.SetTexture2DMS() })},
		{"2d ms layer 2", view2D(gputypes.TextureViewDimension2D, 0, 2, true),
			dsv(func(d *d3d12.D3D12_DEPTH_STENCIL_VIEW_DESC) { d.SetTexture2DMSArray(2, 1) })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dsvDesc(format, tt.r); got != tt.want {
				t.Errorf("dsvDesc = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestSRVDescLayer checks the shader resource view descriptor: 2D views
// follow the layer rule of TestRTVDescLayer, and a cube view at base layer
// b > 0 is a one-cube TEXTURECUBEARRAY whose First2DArrayFace is b, a face
// (array layer) index, not b/6 (to_srv).
func TestSRVDescLayer(t *testing.T) {
	srv := func(set func(*d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC)) d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC {
		d := d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC{
			Format:                  viewDescTestFormat,
			Shader4ComponentMapping: d3d12.D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING,
		}
		set(&d)
		return d
	}
	cube := func(base, count uint32) viewRange {
		return viewRange{dim: gputypes.TextureViewDimensionCube, mipCount: 1, baseLayer: base, layerCount: count}
	}
	cubeArray := func(base, count uint32) viewRange {
		r := cube(base, count)
		r.dim = gputypes.TextureViewDimensionCubeArray
		return r
	}
	tests := []struct {
		name string
		r    viewRange
		want d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC
	}{
		{"2d layer 0", view2D(gputypes.TextureViewDimension2D, 1, 0, false),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTexture2D(1, 1, 0, 0) })},
		{"2d layer 3", view2D(gputypes.TextureViewDimension2D, 1, 3, false),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTexture2DArray(1, 1, 3, 1, 0, 0) })},
		{"2d-array layer 0", view2D(gputypes.TextureViewDimension2DArray, 0, 0, false),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTexture2DArray(0, 1, 0, 1, 0, 0) })},
		{"2d ms layer 0", view2D(gputypes.TextureViewDimension2D, 0, 0, true),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTexture2DMS() })},
		{"2d ms layer 2", view2D(gputypes.TextureViewDimension2D, 0, 2, true),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTexture2DMSArray(2, 1) })},
		{"cube base 0", cube(0, 6),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTextureCube(0, 1, 0) })},
		{"cube base 6", cube(6, 6),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTextureCubeArray(0, 1, 6, 1, 0) })},
		{"cube-array base 6", cubeArray(6, 12),
			srv(func(d *d3d12.D3D12_SHADER_RESOURCE_VIEW_DESC) { d.SetTextureCubeArray(0, 1, 6, 2, 0) })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := srvDesc(viewDescTestFormat, 0, tt.r); got != tt.want {
				t.Errorf("srvDesc = %+v, want %+v", got, tt.want)
			}
		})
	}
}
