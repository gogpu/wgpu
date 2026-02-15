// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"context"
	"log/slog"
	"runtime"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// debugCallbackPtr holds the callback function pointer to prevent GC collection.
// Once created, the callback lives for the process lifetime (Vulkan requirement).
var debugCallbackPtr uintptr

// vulkanDebugCallback is the Go function registered as the Vulkan debug messenger callback.
// The Vulkan spec defines the callback signature as:
//
//	VkBool32 callback(
//	    VkDebugUtilsMessageSeverityFlagBitsEXT severity,
//	    VkDebugUtilsMessageTypeFlagsEXT types,
//	    const VkDebugUtilsMessengerCallbackDataEXT* callbackData,
//	    void* userData)
//
// All parameters are uintptr-sized for compatibility with ffi.NewCallback.
func vulkanDebugCallback(severity, types, callbackData, userData uintptr) uintptr {
	if callbackData == 0 {
		return vk.False
	}

	// Read callback data struct to extract the message.
	// The pointer arrives as a uintptr from the Vulkan driver (not GC-managed).
	// Use double-indirection pattern to satisfy go vet (same as ptrFromUintptr).
	data := *(**vk.DebugUtilsMessengerCallbackDataEXT)(unsafe.Pointer(&callbackData))

	// Extract the null-terminated C string from PMessage.
	msg := "(no message)"
	if data.PMessage != 0 {
		msg = cStringFromPtr(data.PMessage)
	}

	// Extract message ID name (e.g., "VUID-vkCmdDraw-None-02699").
	msgID := ""
	if data.PMessageIdName != 0 {
		msgID = cStringFromPtr(data.PMessageIdName)
	}

	// Map Vulkan severity to slog level.
	severityBits := vk.DebugUtilsMessageSeverityFlagBitsEXT(severity)
	var level slog.Level
	switch {
	case severityBits&vk.DebugUtilsMessageSeverityErrorBitExt != 0:
		level = slog.LevelError
	case severityBits&vk.DebugUtilsMessageSeverityWarningBitExt != 0:
		level = slog.LevelWarn
	case severityBits&vk.DebugUtilsMessageSeverityInfoBitExt != 0:
		level = slog.LevelInfo
	default:
		level = slog.LevelDebug
	}

	// Determine message type.
	typeBits := vk.DebugUtilsMessageTypeFlagBitsEXT(types)
	var typeStr string
	switch {
	case typeBits&vk.DebugUtilsMessageTypeValidationBitExt != 0:
		typeStr = "Validation"
	case typeBits&vk.DebugUtilsMessageTypePerformanceBitExt != 0:
		typeStr = "Performance"
	default:
		typeStr = "General"
	}

	attrs := []slog.Attr{
		slog.String("type", typeStr),
	}
	if msgID != "" {
		attrs = append(attrs, slog.String("id", msgID))
	}
	hal.Logger().LogAttrs(context.Background(), level, "vulkan: "+msg, attrs...)

	// Returning VK_FALSE (0) means the Vulkan call that triggered the callback
	// should NOT be aborted. Returning VK_TRUE would abort the call.
	return vk.False
}

// cStringFromPtr reads a null-terminated C string from a uintptr.
func cStringFromPtr(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	// Use ptrFromUintptr (defined in unsafe.go) to convert safely.
	p := ptrFromUintptr(ptr)
	// Read bytes until null terminator, with a reasonable limit.
	const maxLen = 4096
	buf := unsafe.Slice(p, maxLen)
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// createDebugMessenger creates a Vulkan debug utils messenger for the instance.
// It registers vulkanDebugCallback to receive validation layer messages.
// Returns the messenger handle, or 0 if creation fails (non-fatal).
func createDebugMessenger(instance *Instance) vk.DebugUtilsMessengerEXT {
	// Create callback pointer once (safe for concurrent use after initialization).
	if debugCallbackPtr == 0 {
		debugCallbackPtr = ffi.NewCallback(vulkanDebugCallback)
	}

	severityFlags := vk.DebugUtilsMessageSeverityFlagsEXT(
		vk.DebugUtilsMessageSeverityWarningBitExt |
			vk.DebugUtilsMessageSeverityErrorBitExt,
	)

	typeFlags := vk.DebugUtilsMessageTypeFlagsEXT(
		vk.DebugUtilsMessageTypeGeneralBitExt |
			vk.DebugUtilsMessageTypeValidationBitExt |
			vk.DebugUtilsMessageTypePerformanceBitExt,
	)

	createInfo := vk.DebugUtilsMessengerCreateInfoEXT{
		SType:           vk.StructureTypeDebugUtilsMessengerCreateInfoExt,
		MessageSeverity: severityFlags,
		MessageType:     typeFlags,
		PfnUserCallback: debugCallbackPtr,
	}

	var messenger vk.DebugUtilsMessengerEXT
	result := instance.cmds.CreateDebugUtilsMessengerEXT(
		instance.handle, &createInfo, nil, &messenger,
	)
	if result != vk.Success {
		hal.Logger().Warn("vulkan: failed to create debug messenger", "result", result)
		return 0
	}

	// Keep the callback function alive for the lifetime of the messenger.
	runtime.KeepAlive(debugCallbackPtr)

	return messenger
}

// destroyDebugMessenger destroys the debug utils messenger.
func destroyDebugMessenger(instance *Instance, messenger vk.DebugUtilsMessengerEXT) {
	if messenger != 0 {
		instance.cmds.DestroyDebugUtilsMessengerEXT(instance.handle, messenger, nil)
	}
}
