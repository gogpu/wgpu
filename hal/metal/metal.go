// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	metalLib  unsafe.Pointer
	quartzLib unsafe.Pointer

	// Metal framework functions
	symMTLCreateSystemDefaultDevice unsafe.Pointer
	symMTLCopyAllDevices            unsafe.Pointer

	cifCreateDefaultDevice types.CallInterface
	cifCopyAllDevices      types.CallInterface

	initOnce sync.Once
	initErr  error
)

// Init initializes the Metal framework.
// Must be called before using any other Metal functions.
func Init() error {
	initOnce.Do(func() {
		initErr = doInit()
	})
	return initErr
}

func doInit() error {
	// Initialize Objective-C runtime first
	if err := initObjCRuntime(); err != nil {
		return err
	}

	var err error

	// Load Metal.framework
	metalLib, err = ffi.LoadLibrary("/System/Library/Frameworks/Metal.framework/Metal")
	if err != nil {
		return fmt.Errorf("metal: failed to load Metal.framework: %w", err)
	}

	// Load QuartzCore.framework for CAMetalLayer
	quartzLib, err = ffi.LoadLibrary("/System/Library/Frameworks/QuartzCore.framework/QuartzCore")
	if err != nil {
		return fmt.Errorf("metal: failed to load QuartzCore.framework: %w", err)
	}

	// Load Metal functions
	if symMTLCreateSystemDefaultDevice, err = ffi.GetSymbol(metalLib, "MTLCreateSystemDefaultDevice"); err != nil {
		return fmt.Errorf("metal: MTLCreateSystemDefaultDevice not found: %w", err)
	}
	symMTLCopyAllDevices, _ = ffi.GetSymbol(metalLib, "MTLCopyAllDevices") // Optional

	// Prepare CallInterfaces
	if err = prepareMetalCallInterfaces(); err != nil {
		return err
	}

	// Pre-register common selectors for performance
	preRegisterSelectors()

	return nil
}

func prepareMetalCallInterfaces() error {
	var err error

	// id<MTLDevice> MTLCreateSystemDefaultDevice(void)
	err = ffi.PrepareCallInterface(&cifCreateDefaultDevice, types.DefaultCall,
		types.PointerTypeDescriptor, []*types.TypeDescriptor{})
	if err != nil {
		return fmt.Errorf("metal: failed to prepare MTLCreateSystemDefaultDevice: %w", err)
	}

	// NSArray<id<MTLDevice>>* MTLCopyAllDevices(void)
	if symMTLCopyAllDevices != nil {
		err = ffi.PrepareCallInterface(&cifCopyAllDevices, types.DefaultCall,
			types.PointerTypeDescriptor, []*types.TypeDescriptor{})
		if err != nil {
			return fmt.Errorf("metal: failed to prepare MTLCopyAllDevices: %w", err)
		}
	}

	return nil
}

// preRegisterSelectors pre-registers commonly used selectors.
func preRegisterSelectors() {
	// Common selectors used throughout
	commonSelectors := []string{
		"alloc", "init", "new", "retain", "release", "autorelease",
		"name", "setLabel:", "label",
		"contents", "length", "count",
		"localizedDescription", // NSError
		// MTLDevice
		"newBufferWithLength:options:",
		"newTextureWithDescriptor:",
		"newSamplerStateWithDescriptor:",
		"newCommandQueue",
		"newRenderPipelineStateWithDescriptor:error:",
		"newLibraryWithSource:options:error:",
		"newFunctionWithName:",
		"supportsFamily:",
		// MTLCommandQueue
		"commandBuffer",
		"commandBufferWithUnretainedReferences",
		// MTLCommandBuffer
		"renderCommandEncoderWithDescriptor:",
		"blitCommandEncoder",
		"computeCommandEncoder",
		"commit",
		"waitUntilCompleted",
		"presentDrawable:",
		// MTLRenderCommandEncoder
		"setRenderPipelineState:",
		"setVertexBuffer:offset:atIndex:",
		"setFragmentBuffer:offset:atIndex:",
		"setVertexTexture:atIndex:",
		"setFragmentTexture:atIndex:",
		"setViewport:",
		"setScissorRect:",
		"drawPrimitives:vertexStart:vertexCount:",
		"drawIndexedPrimitives:indexCount:indexType:indexBuffer:indexBufferOffset:",
		"endEncoding",
		// CAMetalLayer
		"setDevice:",
		"setPixelFormat:",
		"setFramebufferOnly:",
		"setDrawableSize:",
		"nextDrawable",
		// MTLTexture
		"width", "height", "depth",
		"pixelFormat", "textureType",
		"newTextureViewWithPixelFormat:",
		// Descriptors
		"setWidth:", "setHeight:", "setDepth:",
		"setPixelFormat:", "setTextureType:", "setUsage:",
		"setStorageMode:", "setMipmapLevelCount:", "setSampleCount:",
		// MTLRenderPipelineDescriptor
		"setVertexFunction:", "setFragmentFunction:",
		"colorAttachments", "objectAtIndexedSubscript:",
		"setWriteMask:", "setBlendingEnabled:",
		"setSourceRGBBlendFactor:", "setDestinationRGBBlendFactor:", "setRgbBlendOperation:",
		"setSourceAlphaBlendFactor:", "setDestinationAlphaBlendFactor:", "setAlphaBlendOperation:",
	}
	for _, sel := range commonSelectors {
		RegisterSelector(sel)
	}
}

// CreateSystemDefaultDevice returns the default Metal device.
func CreateSystemDefaultDevice() ID {
	var result ID
	_ = ffi.CallFunction(&cifCreateDefaultDevice, symMTLCreateSystemDefaultDevice, unsafe.Pointer(&result), nil)
	return result
}

// CopyAllDevices returns all available Metal devices.
// Returns nil if the function is not available (iOS).
func CopyAllDevices() []ID {
	if symMTLCopyAllDevices == nil {
		// On iOS, only default device is available
		device := CreateSystemDefaultDevice()
		if device == 0 {
			return nil
		}
		return []ID{device}
	}

	var nsArray ID
	_ = ffi.CallFunction(&cifCopyAllDevices, symMTLCopyAllDevices, unsafe.Pointer(&nsArray), nil)
	if nsArray == 0 {
		return nil
	}

	count := MsgSendUint(nsArray, Sel("count"))
	if count == 0 {
		Release(nsArray)
		return nil
	}

	devices := make([]ID, count)
	for i := uint(0); i < count; i++ {
		devices[i] = MsgSend(nsArray, Sel("objectAtIndex:"), uintptr(i))
		Retain(devices[i]) // Retain since we will release the array
	}

	Release(nsArray)
	return devices
}

// DeviceName returns the name of a Metal device.
func DeviceName(device ID) string {
	if device == 0 {
		return ""
	}
	name := MsgSend(device, Sel("name"))
	return GoString(name)
}

// DeviceSupportsFamily checks if a device supports a GPU family.
func DeviceSupportsFamily(device ID, family MTLGPUFamily) bool {
	if device == 0 {
		return false
	}
	return MsgSendBool(device, Sel("supportsFamily:"), uintptr(family))
}

// DeviceRegistryID returns the IORegistry ID of the device.
func DeviceRegistryID(device ID) uint64 {
	if device == 0 {
		return 0
	}
	return uint64(MsgSend(device, Sel("registryID")))
}

// DeviceIsLowPower returns true if the device is low-power.
func DeviceIsLowPower(device ID) bool {
	if device == 0 {
		return false
	}
	return MsgSendBool(device, Sel("isLowPower"))
}

// DeviceIsHeadless returns true if the device is headless (no display).
func DeviceIsHeadless(device ID) bool {
	if device == 0 {
		return false
	}
	return MsgSendBool(device, Sel("isHeadless"))
}

// DeviceIsRemovable returns true if the device is removable (eGPU).
func DeviceIsRemovable(device ID) bool {
	if device == 0 {
		return false
	}
	return MsgSendBool(device, Sel("isRemovable"))
}

// DeviceMaxBufferLength returns the maximum buffer length for the device.
func DeviceMaxBufferLength(device ID) uint64 {
	if device == 0 {
		return 0
	}
	return uint64(MsgSend(device, Sel("maxBufferLength")))
}

// DeviceRecommendedMaxWorkingSetSize returns the recommended max working set size.
func DeviceRecommendedMaxWorkingSetSize(device ID) uint64 {
	if device == 0 {
		return 0
	}
	return uint64(MsgSend(device, Sel("recommendedMaxWorkingSetSize")))
}
