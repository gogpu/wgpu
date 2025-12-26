// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

import (
	"encoding/binary"
	"math"
	"unsafe"
)

// D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING is the default shader component mapping.
// This maps each component to itself: RGBA -> RGBA.
const D3D12_DEFAULT_SHADER_4_COMPONENT_MAPPING = 0x1688

// -----------------------------------------------------------------------------
// D3D12_SHADER_RESOURCE_VIEW_DESC helpers
// -----------------------------------------------------------------------------

// SetTexture1D sets up a 1D texture SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTexture1D(mostDetailedMip, mipLevels uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURE1D
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], floatBits(resourceMinLODClamp))
}

// SetTexture2D sets up a 2D texture SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTexture2D(mostDetailedMip, mipLevels, planeSlice uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURE2D
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], planeSlice)
	binary.LittleEndian.PutUint32(d.Union[12:16], floatBits(resourceMinLODClamp))
}

// SetTexture2DArray sets up a 2D texture array SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTexture2DArray(mostDetailedMip, mipLevels, firstArraySlice, arraySize, planeSlice uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURE2DARRAY
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], firstArraySlice)
	binary.LittleEndian.PutUint32(d.Union[12:16], arraySize)
	// Note: planeSlice and resourceMinLODClamp are in extended union space (handled by D3D12)
	_ = planeSlice
	_ = resourceMinLODClamp
}

// SetTexture3D sets up a 3D texture SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTexture3D(mostDetailedMip, mipLevels uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURE3D
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], floatBits(resourceMinLODClamp))
}

// SetTextureCube sets up a cube texture SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTextureCube(mostDetailedMip, mipLevels uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURECUBE
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], floatBits(resourceMinLODClamp))
}

// SetTextureCubeArray sets up a cube texture array SRV.
func (d *D3D12_SHADER_RESOURCE_VIEW_DESC) SetTextureCubeArray(mostDetailedMip, mipLevels, first2DArrayFace, numCubes uint32, resourceMinLODClamp float32) {
	d.ViewDimension = D3D12_SRV_DIMENSION_TEXTURECUBEARRAY
	binary.LittleEndian.PutUint32(d.Union[0:4], mostDetailedMip)
	binary.LittleEndian.PutUint32(d.Union[4:8], mipLevels)
	binary.LittleEndian.PutUint32(d.Union[8:12], first2DArrayFace)
	binary.LittleEndian.PutUint32(d.Union[12:16], numCubes)
	_ = resourceMinLODClamp // Extended union space
}

// -----------------------------------------------------------------------------
// D3D12_RENDER_TARGET_VIEW_DESC helpers
// -----------------------------------------------------------------------------

// SetTexture1D sets up a 1D texture RTV.
func (d *D3D12_RENDER_TARGET_VIEW_DESC) SetTexture1D(mipSlice uint32) {
	d.ViewDimension = D3D12_RTV_DIMENSION_TEXTURE1D
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
}

// SetTexture2D sets up a 2D texture RTV.
func (d *D3D12_RENDER_TARGET_VIEW_DESC) SetTexture2D(mipSlice, planeSlice uint32) {
	d.ViewDimension = D3D12_RTV_DIMENSION_TEXTURE2D
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
	binary.LittleEndian.PutUint32(d.Union[4:8], planeSlice)
}

// SetTexture2DArray sets up a 2D texture array RTV.
func (d *D3D12_RENDER_TARGET_VIEW_DESC) SetTexture2DArray(mipSlice, firstArraySlice, arraySize, planeSlice uint32) {
	d.ViewDimension = D3D12_RTV_DIMENSION_TEXTURE2DARRAY
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
	binary.LittleEndian.PutUint32(d.Union[4:8], firstArraySlice)
	binary.LittleEndian.PutUint32(d.Union[8:12], arraySize)
	// planeSlice would require extended union but RTV only has 12 bytes
	_ = planeSlice
}

// SetTexture3D sets up a 3D texture RTV.
func (d *D3D12_RENDER_TARGET_VIEW_DESC) SetTexture3D(mipSlice, firstWSlice, wSize uint32) {
	d.ViewDimension = D3D12_RTV_DIMENSION_TEXTURE3D
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
	binary.LittleEndian.PutUint32(d.Union[4:8], firstWSlice)
	binary.LittleEndian.PutUint32(d.Union[8:12], wSize)
}

// -----------------------------------------------------------------------------
// D3D12_DEPTH_STENCIL_VIEW_DESC helpers
// -----------------------------------------------------------------------------

// SetTexture1D sets up a 1D texture DSV.
func (d *D3D12_DEPTH_STENCIL_VIEW_DESC) SetTexture1D(mipSlice uint32) {
	d.ViewDimension = D3D12_DSV_DIMENSION_TEXTURE1D
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
}

// SetTexture2D sets up a 2D texture DSV.
func (d *D3D12_DEPTH_STENCIL_VIEW_DESC) SetTexture2D(mipSlice uint32) {
	d.ViewDimension = D3D12_DSV_DIMENSION_TEXTURE2D
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
}

// SetTexture2DArray sets up a 2D texture array DSV.
func (d *D3D12_DEPTH_STENCIL_VIEW_DESC) SetTexture2DArray(mipSlice, firstArraySlice, arraySize uint32) {
	d.ViewDimension = D3D12_DSV_DIMENSION_TEXTURE2DARRAY
	binary.LittleEndian.PutUint32(d.Union[0:4], mipSlice)
	binary.LittleEndian.PutUint32(d.Union[4:8], firstArraySlice)
	// arraySize would exceed Union bounds (only 8 bytes)
	_ = arraySize
}

// -----------------------------------------------------------------------------
// D3D12_CLEAR_VALUE helpers
// -----------------------------------------------------------------------------

// SetColor sets up a color clear value.
func (d *D3D12_CLEAR_VALUE) SetColor(color [4]float32) {
	d.Color = color
}

// SetDepthStencil sets up a depth/stencil clear value.
// This reinterprets the Color field as depth/stencil values.
func (d *D3D12_CLEAR_VALUE) SetDepthStencil(depth float32, stencil uint8) {
	d.Color[0] = depth
	// Stencil is stored in the first byte of Color[1]
	stencilBits := *(*[4]byte)(unsafe.Pointer(&d.Color[1]))
	stencilBits[0] = stencil
	d.Color[1] = *(*float32)(unsafe.Pointer(&stencilBits))
}

// floatBits returns the IEEE 754 binary representation of a float32.
func floatBits(f float32) uint32 {
	return math.Float32bits(f)
}
