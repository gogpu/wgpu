// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/indirect"
)

const (
	indexedICBMinCommands     uint32 = 1024
	indexedICBMaxCommands     uint32 = 52428 // floor(1 MiB / 20-byte records)
	indexedICBMaxRetainedIDs         = 128
	indexedICBThreadsPerGroup uint32 = 64
)

type indexedICBTranslator struct {
	library  ID
	pipeline ID
}

type indexedICBParams struct {
	commandBase uint32
	count       uint32
}

type indexedICBArena struct {
	icb            ID
	argumentBuffer ID
	capacity       uint32
}

type indexedICBCommands struct {
	icb   ID
	count uint32
	owner *indexedICBOwnership
}

// indexedICBOwnership is the single terminal-path owner for native objects and
// explicitly retained buffers referenced by one ICB execution. The fixed array
// makes residency bounded before any optimized work is encoded.
type indexedICBOwnership struct {
	once          sync.Once
	objects       [indexedICBMaxRetainedIDs]ID
	objectCount   int
	releaseObject func(ID)
}

// retainOwned adopts an existing +1 Objective-C reference.
func (o *indexedICBOwnership) retainOwned(id ID) bool {
	if o == nil || id == 0 || o.objectCount >= len(o.objects) {
		return false
	}
	o.objects[o.objectCount] = id
	o.objectCount++
	return true
}

func (o *indexedICBOwnership) retainReference(id ID) bool {
	if o == nil || id == 0 || o.objectCount >= len(o.objects) {
		return false
	}
	Retain(id)
	return o.retainOwned(id)
}

func (o *indexedICBOwnership) release() {
	if o == nil {
		return
	}
	o.once.Do(func() {
		release := o.releaseObject
		if release == nil {
			release = Release
		}
		for i := o.objectCount - 1; i >= 0; i-- {
			release(o.objects[i])
			o.objects[i] = 0
		}
		o.objectCount = 0
	})
}

func indexedICBCountEligible(count uint32) bool {
	return count >= indexedICBMinCommands && count <= indexedICBMaxCommands
}

func indexedICBArenaCapacity(count, max uint32) uint32 {
	if count == 0 || max == 0 || count > max {
		return 0
	}
	capacity := indexedICBMinCommands
	if capacity > max {
		capacity = max
	}
	for capacity < count {
		next := capacity * 2
		if next < capacity || next > max {
			capacity = max
			break
		}
		capacity = next
	}
	if capacity < count {
		return 0
	}
	return capacity
}

func indexedICBDispatchGroups(count uint32) uint32 {
	if count == 0 {
		return 0
	}
	return (count + indexedICBThreadsPerGroup - 1) / indexedICBThreadsPerGroup
}

func (d *Device) indexedICBTranslator(format gputypes.IndexFormat) (indexedICBTranslator, error) {
	d.icbTranslatorMu.Lock()
	defer d.icbTranslatorMu.Unlock()
	if d.icbTranslators == nil {
		d.icbTranslators = make(map[gputypes.IndexFormat]indexedICBTranslator, 2)
	}
	if translator, ok := d.icbTranslators[format]; ok {
		return translator, nil
	}
	translator, err := compileIndexedICBTranslator(d, format)
	if err != nil {
		return indexedICBTranslator{}, err
	}
	d.icbTranslators[format] = translator
	return translator, nil
}

func (d *Device) releaseIndexedICBTranslators() {
	d.icbTranslatorMu.Lock()
	defer d.icbTranslatorMu.Unlock()
	for format, translator := range d.icbTranslators {
		if translator.pipeline != 0 {
			Release(translator.pipeline)
		}
		if translator.library != 0 {
			Release(translator.library)
		}
		delete(d.icbTranslators, format)
	}
}

func compileIndexedICBTranslator(d *Device, format gputypes.IndexFormat) (indexedICBTranslator, error) {
	indexType := "uint"
	switch format {
	case gputypes.IndexFormatUint16:
		indexType = "ushort"
	case gputypes.IndexFormatUint32:
	default:
		return indexedICBTranslator{}, fmt.Errorf("metal: unsupported ICB index format %d", format)
	}
	source := fmt.Sprintf(`
#include <metal_stdlib>
#include <metal_command_buffer>
using namespace metal;
struct DrawIndexedArgs { uint indexCount; uint instanceCount; uint firstIndex; int baseVertex; uint firstInstance; };
struct ICBContainer { command_buffer commandBuffer [[id(0)]]; };
struct IndexedICBParams { uint commandBase; uint count; };
kernel void wgpu_translate_indexed_icb(device const DrawIndexedArgs* args [[buffer(0)]],
    device ICBContainer* container [[buffer(1)]], device const %s* indices [[buffer(2)]],
    constant IndexedICBParams& params [[buffer(3)]], uint id [[thread_position_in_grid]]) {
    if (id >= params.count) return;
    DrawIndexedArgs a = args[id];
    render_command command(container->commandBuffer, params.commandBase + id);
    command.draw_indexed_primitives(primitive_type::triangle, a.indexCount,
        indices + a.firstIndex, a.instanceCount, a.baseVertex, a.firstInstance);
}
`, indexType)

	str := NSString(source)
	defer Release(str)
	var errorPtr ID
	library := MsgSend(d.raw, Sel("newLibraryWithSource:options:error:"), uintptr(str), 0, uintptr(unsafe.Pointer(&errorPtr)))
	if library == 0 {
		return indexedICBTranslator{}, fmt.Errorf("metal: indexed ICB translator compilation failed: %s", formatNSError(errorPtr))
	}
	name := NSString("wgpu_translate_indexed_icb")
	function := MsgSend(library, Sel("newFunctionWithName:"), uintptr(name))
	Release(name)
	if function == 0 {
		Release(library)
		return indexedICBTranslator{}, fmt.Errorf("metal: indexed ICB translator function missing")
	}
	pipeline := MsgSend(d.raw, Sel("newComputePipelineStateWithFunction:error:"), uintptr(function), uintptr(unsafe.Pointer(&errorPtr)))
	Release(function)
	if pipeline == 0 {
		Release(library)
		return indexedICBTranslator{}, fmt.Errorf("metal: indexed ICB translator pipeline creation failed: %s", formatNSError(errorPtr))
	}
	return indexedICBTranslator{library: library, pipeline: pipeline}, nil
}

func (d *Device) newIndexedICBArena(translator indexedICBTranslator, count uint32) (*indexedICBArena, error) {
	capacity := indexedICBArenaCapacity(count, indexedICBMaxCommands)
	if capacity == 0 {
		return nil, fmt.Errorf("metal: indexed ICB command count out of range")
	}
	descriptor := MsgSend(ID(GetClass("MTLIndirectCommandBufferDescriptor")), Sel("new"))
	if descriptor == 0 {
		return nil, fmt.Errorf("metal: failed to create ICB descriptor")
	}
	defer Release(descriptor)
	_ = MsgSend(descriptor, Sel("setCommandTypes:"), uintptr(MTLIndirectCommandTypeDrawIndexed))
	_ = MsgSend(descriptor, Sel("setInheritPipelineState:"), uintptr(YES))
	_ = MsgSend(descriptor, Sel("setInheritBuffers:"), uintptr(YES))
	icb := MsgSend(d.raw, Sel("newIndirectCommandBufferWithDescriptor:maxCommandCount:options:"),
		uintptr(descriptor), uintptr(capacity), 0)
	if icb == 0 {
		return nil, fmt.Errorf("metal: failed to create indexed ICB")
	}

	name := NSString("wgpu_translate_indexed_icb")
	function := MsgSend(translator.library, Sel("newFunctionWithName:"), uintptr(name))
	Release(name)
	if function == 0 {
		Release(icb)
		return nil, fmt.Errorf("metal: indexed ICB translator function missing")
	}
	defer Release(function)
	argumentEncoder := MsgSend(function, Sel("newArgumentEncoderWithBufferIndex:"), uintptr(1))
	if argumentEncoder == 0 {
		Release(icb)
		return nil, fmt.Errorf("metal: failed to create ICB argument encoder")
	}
	defer Release(argumentEncoder)
	encodedLength := MsgSendUint(argumentEncoder, Sel("encodedLength"))
	argumentBuffer := MsgSend(d.raw, Sel("newBufferWithLength:options:"), uintptr(encodedLength), uintptr(MTLResourceStorageModePrivate))
	if argumentBuffer == 0 {
		Release(icb)
		return nil, fmt.Errorf("metal: failed to create ICB argument buffer")
	}
	_ = MsgSend(argumentEncoder, Sel("setArgumentBuffer:offset:"), uintptr(argumentBuffer), 0)
	_ = MsgSend(argumentEncoder, Sel("setIndirectCommandBuffer:atIndex:"), uintptr(icb), 0)
	return &indexedICBArena{icb: icb, argumentBuffer: argumentBuffer, capacity: capacity}, nil
}

func (d *Device) indexedICBSelectorsAvailable() bool {
	if d == nil || d.raw == 0 || !DeviceSupportsFamily(d.raw, MTLGPUFamilyApple7) {
		return false
	}
	for _, name := range []string{
		"newIndirectCommandBufferWithDescriptor:maxCommandCount:options:",
		"newLibraryWithSource:options:error:",
		"newComputePipelineStateWithFunction:error:",
		"newBufferWithLength:options:",
	} {
		selector := Sel(name)
		if selector == 0 || !MsgSendBool(d.raw, Sel("respondsToSelector:"), uintptr(selector)) {
			return false
		}
	}
	class := GetClass("MTLIndirectCommandBufferDescriptor")
	if class == 0 {
		return false
	}
	for _, name := range []string{"setCommandTypes:", "setInheritPipelineState:", "setInheritBuffers:"} {
		selector := Sel(name)
		if selector == 0 || !MsgSendBool(ID(class), Sel("instancesRespondToSelector:"), uintptr(selector)) {
			return false
		}
	}
	for _, name := range []string{
		"resetWithRange:", "executeCommandsInBuffer:withRange:",
		"newArgumentEncoderWithBufferIndex:", "setArgumentBuffer:offset:",
		"setIndirectCommandBuffer:atIndex:",
	} {
		if Sel(name) == 0 {
			return false
		}
	}
	return true
}

func (e *RenderPassEncoder) prepareIndexedICB(arguments *Buffer, offset uint64, count uint32) (*indexedICBCommands, bool) {
	if e == nil || e.commandEncoder == nil || e.commandEncoder.cmdBuffer == 0 || e.device == nil ||
		e.raw != 0 || e.pipeline == nil || !e.pipeline.icbCompatible || e.indexBuffer == nil ||
		arguments == nil || arguments.raw == 0 || e.indexBuffer.raw == 0 || !indexedICBCountEligible(count) ||
		!e.device.indexedICBSelectorsAvailable() {
		return nil, false
	}
	if e.indexFormat != gputypes.IndexFormatUint16 && e.indexFormat != gputypes.IndexFormatUint32 {
		return nil, false
	}
	if !indirect.RangeFits(arguments.size, offset, 20, count) {
		return nil, false
	}

	translator, err := e.device.indexedICBTranslator(e.indexFormat)
	if err != nil {
		return nil, false
	}
	arena, err := e.device.newIndexedICBArena(translator, count)
	if err != nil {
		return nil, false
	}
	owner := &indexedICBOwnership{}
	if !owner.retainOwned(arena.icb) || !owner.retainOwned(arena.argumentBuffer) {
		owner.release()
		return nil, false
	}
	if !e.retainIndexedICBResources(owner, arguments) {
		owner.release()
		return nil, false
	}

	rng := NSRange{Length: NSUInteger(count)}
	msgSendVoid(arena.icb, Sel("resetWithRange:"), argStruct(rng, nsRangeType))
	pool := NewAutoreleasePool()
	compute := MsgSend(e.commandEncoder.cmdBuffer, Sel("computeCommandEncoder"))
	if compute == 0 {
		pool.Drain()
		owner.release()
		return nil, false
	}
	Retain(compute)
	pool.Drain()
	_ = MsgSend(compute, Sel("setComputePipelineState:"), uintptr(translator.pipeline))
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(arguments.raw), uintptr(offset), 0)
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(arena.argumentBuffer), 0, 1)
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(e.indexBuffer.raw), uintptr(e.indexOffset), 2)
	params := indexedICBParams{count: count}
	_ = MsgSend(compute, Sel("setBytes:length:atIndex:"), uintptr(unsafe.Pointer(&params)), uintptr(unsafe.Sizeof(params)), 3)
	_ = MsgSend(compute, Sel("useResource:usage:"), uintptr(arguments.raw), 1)
	_ = MsgSend(compute, Sel("useResource:usage:"), uintptr(arena.argumentBuffer), 1)
	_ = MsgSend(compute, Sel("useResource:usage:"), uintptr(e.indexBuffer.raw), 1)
	_ = MsgSend(compute, Sel("useResource:usage:"), uintptr(arena.icb), 2)
	groups := MTLSize{Width: NSUInteger(indexedICBDispatchGroups(count)), Height: 1, Depth: 1}
	threads := MTLSize{Width: NSUInteger(indexedICBThreadsPerGroup), Height: 1, Depth: 1}
	msgSendVoid(compute, Sel("dispatchThreadgroups:threadsPerThreadgroup:"),
		argStruct(groups, mtlSizeType), argStruct(threads, mtlSizeType))
	_ = MsgSend(compute, Sel("endEncoding"))
	Release(compute)

	e.commandEncoder.icbOwners = append(e.commandEncoder.icbOwners, owner)
	return &indexedICBCommands{icb: arena.icb, count: count, owner: owner}, true
}

func (e *RenderPassEncoder) executeIndexedICB(commands *indexedICBCommands, arguments *Buffer) bool {
	if commands == nil || commands.icb == 0 || commands.count == 0 || e.raw == 0 {
		return false
	}
	e.declareIndexedICBResources(arguments)
	rng := NSRange{Length: NSUInteger(commands.count)}
	msgSendVoid(e.raw, Sel("executeCommandsInBuffer:withRange:"),
		argPointer(uintptr(commands.icb)), argStruct(rng, nsRangeType))
	return true
}

func (e *RenderPassEncoder) declareIndexedICBResources(arguments *Buffer) {
	declare := func(id ID) {
		if id != 0 {
			_ = MsgSend(e.raw, Sel("useResource:usage:"), uintptr(id), 1)
		}
	}
	if arguments != nil {
		declare(arguments.raw)
	}
	if e.indexBuffer != nil {
		declare(e.indexBuffer.raw)
	}
	if e.pending == nil {
		return
	}
	for i := range e.pending.vertexBuffers {
		state := &e.pending.vertexBuffers[i]
		if state.set && state.buffer != nil {
			declare(state.buffer.raw)
		}
	}
	for i := range e.pending.bindGroups {
		state := &e.pending.bindGroups[i]
		if !state.set || state.group == nil {
			continue
		}
		for _, entry := range state.group.entries {
			if binding, ok := entry.Resource.(gputypes.BufferBinding); ok {
				declare(ID(binding.Buffer))
			}
		}
	}
}

func (e *RenderPassEncoder) retainIndexedICBResources(owner *indexedICBOwnership, arguments *Buffer) bool {
	if !owner.retainReference(arguments.raw) || !owner.retainReference(e.indexBuffer.raw) {
		return false
	}
	if e.pending == nil {
		return true
	}
	for i := range e.pending.vertexBuffers {
		state := &e.pending.vertexBuffers[i]
		if state.set && state.buffer != nil && !owner.retainReference(state.buffer.raw) {
			return false
		}
	}
	for i := range e.pending.bindGroups {
		state := &e.pending.bindGroups[i]
		if !state.set || state.group == nil {
			continue
		}
		for _, entry := range state.group.entries {
			binding, ok := entry.Resource.(gputypes.BufferBinding)
			if !ok || binding.Buffer == 0 || !owner.retainReference(ID(binding.Buffer)) {
				return false
			}
		}
	}
	return true
}
