// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

func graphicsPSOCacheKey(
	desc *hal.RenderPipelineDescriptor,
	psoDesc *d3d12.D3D12_GRAPHICS_PIPELINE_STATE_DESC,
	rootSignatureHash [32]byte,
) string {
	h := sha256New()
	writeBytes(h, rootSignatureHash[:])
	writeShaderBytecode(h, psoDesc.VS)
	writeShaderBytecode(h, psoDesc.PS)
	writeInputLayout(h, psoDesc.InputLayout)
	writeGraphicsFixedState(h, desc, psoDesc)
	return hex.EncodeToString(digestBytes(h))
}

func computePSOCacheKey(
	desc *hal.ComputePipelineDescriptor,
	psoDesc *d3d12.D3D12_COMPUTE_PIPELINE_STATE_DESC,
	rootSignatureHash [32]byte,
) string {
	h := sha256New()
	writeBytes(h, rootSignatureHash[:])
	writeShaderBytecode(h, psoDesc.CS)
	_ = desc // reserved for future specialization constants
	return hex.EncodeToString(digestBytes(h))
}

func rootSignatureHashForLayout(layout *PipelineLayout, emptyHash *[32]byte) [32]byte {
	if layout != nil {
		return layout.rootSignatureHash
	}
	if emptyHash != nil {
		return *emptyHash
	}
	return [32]byte{}
}

func sha256New() hash.Hash {
	return sha256.New()
}

func digestBytes(h hash.Hash) []byte {
	return h.Sum(nil)
}

func writeBytes(h hash.Hash, data []byte) {
	if len(data) == 0 {
		return
	}
	_, _ = h.Write(data)
}

func writeShaderBytecode(h hash.Hash, bc d3d12.D3D12_SHADER_BYTECODE) {
	if bc.ShaderBytecode == nil || bc.BytecodeLength == 0 {
		return
	}
	slice := unsafe.Slice((*byte)(bc.ShaderBytecode), bc.BytecodeLength)
	_, _ = h.Write(slice)
}

func writeInputLayout(h hash.Hash, layout d3d12.D3D12_INPUT_LAYOUT_DESC) {
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], layout.NumElements)
	_, _ = h.Write(count[:])
	if layout.NumElements == 0 || layout.InputElementDescs == nil {
		return
	}
	elements := unsafe.Slice(layout.InputElementDescs, layout.NumElements)
	for i := range elements {
		writeInputElement(h, &elements[i])
	}
}

func writeInputElement(h hash.Hash, el *d3d12.D3D12_INPUT_ELEMENT_DESC) {
	var header [12]byte
	binary.LittleEndian.PutUint32(header[0:4], el.InputSlot)
	binary.LittleEndian.PutUint32(header[4:8], el.AlignedByteOffset)
	binary.LittleEndian.PutUint32(header[8:12], uint32(el.Format))
	_, _ = h.Write(header[:])
	var classStep [5]byte
	classStep[0] = byte(el.InputSlotClass)
	binary.LittleEndian.PutUint32(classStep[1:5], el.InstanceDataStepRate)
	_, _ = h.Write(classStep[:])
	if el.SemanticName != nil {
		name := unsafe.String(el.SemanticName, findNull(el.SemanticName))
		_, _ = h.Write([]byte(name))
	}
	var index [4]byte
	binary.LittleEndian.PutUint32(index[:], el.SemanticIndex)
	_, _ = h.Write(index[:])
}

func findNull(p *byte) int {
	if p == nil {
		return 0
	}
	n := 0
	for {
		if *p == 0 {
			return n
		}
		n++
		p = (*byte)(unsafe.Add(unsafe.Pointer(p), 1))
	}
}

func writeGraphicsFixedState(
	h hash.Hash,
	desc *hal.RenderPipelineDescriptor,
	psoDesc *d3d12.D3D12_GRAPHICS_PIPELINE_STATE_DESC,
) {
	var buf [64]byte
	binary.LittleEndian.PutUint32(buf[0:4], uint32(psoDesc.PrimitiveTopologyType))
	binary.LittleEndian.PutUint32(buf[4:8], psoDesc.NumRenderTargets)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(psoDesc.DSVFormat))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(psoDesc.SampleDesc.Count))
	binary.LittleEndian.PutUint32(buf[16:20], psoDesc.SampleDesc.Quality)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(psoDesc.IBStripCutValue))
	_, _ = h.Write(buf[:24])

	writeRasterizer(h, &psoDesc.RasterizerState)
	writeDepthStencil(h, &psoDesc.DepthStencilState)
	writeBlend(h, &psoDesc.BlendState)

	if desc != nil && desc.Fragment != nil {
		for _, target := range desc.Fragment.Targets {
			writeColorTarget(h, &target)
		}
	}
}

func writeRasterizer(h hash.Hash, rs *d3d12.D3D12_RASTERIZER_DESC) {
	var buf [32]byte
	buf[0] = byte(rs.FillMode)
	buf[1] = byte(rs.CullMode)
	buf[2] = boolByte(rs.FrontCounterClockwise)
	buf[3] = boolByte(rs.DepthClipEnable)
	buf[4] = boolByte(rs.MultisampleEnable)
	buf[5] = boolByte(rs.AntialiasedLineEnable)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(rs.DepthBias))
	binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(rs.DepthBiasClamp))
	binary.LittleEndian.PutUint32(buf[16:20], math.Float32bits(rs.SlopeScaledDepthBias))
	binary.LittleEndian.PutUint32(buf[20:24], rs.ForcedSampleCount)
	buf[24] = byte(rs.ConservativeRaster)
	_, _ = h.Write(buf[:25])
}

func writeDepthStencil(h hash.Hash, ds *d3d12.D3D12_DEPTH_STENCIL_DESC) {
	var buf [6]byte
	buf[0] = boolByte(ds.DepthEnable)
	buf[1] = byte(ds.DepthWriteMask)
	buf[2] = byte(ds.DepthFunc)
	buf[3] = boolByte(ds.StencilEnable)
	buf[4] = ds.StencilReadMask
	buf[5] = ds.StencilWriteMask
	_, _ = h.Write(buf[:])
	writeStencilOp(h, &ds.FrontFace)
	writeStencilOp(h, &ds.BackFace)
}

func writeStencilOp(h hash.Hash, op *d3d12.D3D12_DEPTH_STENCILOP_DESC) {
	var buf [4]byte
	buf[0] = byte(op.StencilFailOp)
	buf[1] = byte(op.StencilDepthFailOp)
	buf[2] = byte(op.StencilPassOp)
	buf[3] = byte(op.StencilFunc)
	_, _ = h.Write(buf[:])
}

func writeBlend(h hash.Hash, blend *d3d12.D3D12_BLEND_DESC) {
	var buf [8]byte
	buf[0] = boolByte(blend.AlphaToCoverageEnable)
	buf[1] = boolByte(blend.IndependentBlendEnable)
	_, _ = h.Write(buf[:2])
	for i := range blend.RenderTarget {
		writeRenderTargetBlend(h, &blend.RenderTarget[i])
	}
}

func writeRenderTargetBlend(h hash.Hash, rt *d3d12.D3D12_RENDER_TARGET_BLEND_DESC) {
	var buf [16]byte
	buf[0] = boolByte(rt.BlendEnable)
	buf[1] = boolByte(rt.LogicOpEnable)
	buf[2] = byte(rt.SrcBlend)
	buf[3] = byte(rt.DestBlend)
	buf[4] = byte(rt.BlendOp)
	buf[5] = byte(rt.SrcBlendAlpha)
	buf[6] = byte(rt.DestBlendAlpha)
	buf[7] = byte(rt.BlendOpAlpha)
	buf[8] = byte(rt.LogicOp)
	buf[9] = rt.RenderTargetWriteMask
	_, _ = h.Write(buf[:10])
}

func writeColorTarget(h hash.Hash, target *gputypes.ColorTargetState) {
	var buf [12]byte
	binary.LittleEndian.PutUint32(buf[0:4], uint32(target.Format))
	buf[4] = flagByte(target.Blend != nil)
	buf[5] = flagByte(target.WriteMask != 0)
	binary.LittleEndian.PutUint32(buf[8:12], uint32(target.WriteMask))
	_, _ = h.Write(buf[:12])
	if target.Blend != nil {
		writeBlendComponent(h, &target.Blend.Color)
		writeBlendComponent(h, &target.Blend.Alpha)
	}
}

func writeBlendComponent(h hash.Hash, bc *gputypes.BlendComponent) {
	var buf [4]byte
	buf[0] = byte(bc.SrcFactor)
	buf[1] = byte(bc.DstFactor)
	buf[2] = byte(bc.Operation)
	_, _ = h.Write(buf[:3])
}

func boolByte(v int32) byte {
	if v != 0 {
		return 1
	}
	return 0
}

func flagByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}
