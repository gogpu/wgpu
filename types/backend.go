package types

import "fmt"

// Backend identifies the GPU backend.
type Backend uint8

const (
	// BackendEmpty represents no backend (invalid state).
	BackendEmpty Backend = iota
	// BackendVulkan uses the Vulkan API (cross-platform).
	BackendVulkan
	// BackendMetal uses Apple's Metal API (macOS, iOS).
	BackendMetal
	// BackendDX12 uses Microsoft DirectX 12 (Windows).
	BackendDX12
	// BackendGL uses OpenGL/OpenGL ES (legacy fallback).
	BackendGL
	// BackendBrowserWebGPU uses the browser's native WebGPU (WASM).
	BackendBrowserWebGPU
)

// String returns the backend name.
func (b Backend) String() string {
	switch b {
	case BackendEmpty:
		return "Empty"
	case BackendVulkan:
		return "Vulkan"
	case BackendMetal:
		return "Metal"
	case BackendDX12:
		return "DX12"
	case BackendGL:
		return "GL"
	case BackendBrowserWebGPU:
		return "BrowserWebGPU"
	default:
		return fmt.Sprintf("Backend(%d)", b)
	}
}

// Backends is a set of backends.
type Backends uint8

const (
	// BackendsVulkan includes Vulkan.
	BackendsVulkan Backends = 1 << BackendVulkan
	// BackendsMetal includes Metal.
	BackendsMetal Backends = 1 << BackendMetal
	// BackendsDX12 includes DirectX 12.
	BackendsDX12 Backends = 1 << BackendDX12
	// BackendsGL includes OpenGL.
	BackendsGL Backends = 1 << BackendGL
	// BackendsBrowserWebGPU includes browser WebGPU.
	BackendsBrowserWebGPU Backends = 1 << BackendBrowserWebGPU

	// BackendsPrimary includes Vulkan, Metal, DX12, and browser WebGPU.
	BackendsPrimary = BackendsVulkan | BackendsMetal | BackendsDX12 | BackendsBrowserWebGPU
	// BackendsSecondary includes only GL (fallback).
	BackendsSecondary = BackendsGL
	// BackendsAll includes all backends.
	BackendsAll = BackendsPrimary | BackendsSecondary
)

// Contains checks if the backend set contains a specific backend.
func (b Backends) Contains(backend Backend) bool {
	if backend == BackendEmpty {
		return false
	}
	return b&(1<<backend) != 0
}

// Dx12ShaderCompiler specifies the shader compiler for DX12.
type Dx12ShaderCompiler uint8

const (
	// Dx12ShaderCompilerFxc uses the legacy FXC compiler.
	Dx12ShaderCompilerFxc Dx12ShaderCompiler = iota
	// Dx12ShaderCompilerDxc uses the modern DXC compiler.
	Dx12ShaderCompilerDxc
)

// GlBackend specifies the OpenGL backend flavor.
type GlBackend uint8

const (
	// GlBackendGL uses desktop OpenGL.
	GlBackendGL GlBackend = iota
	// GlBackendGLES uses OpenGL ES.
	GlBackendGLES
)

// InstanceFlags controls instance behavior.
type InstanceFlags uint8

const (
	// InstanceFlagsDebug enables debug layers when available.
	InstanceFlagsDebug InstanceFlags = 1 << iota
	// InstanceFlagsValidation enables validation layers.
	InstanceFlagsValidation
	// InstanceFlagsGPUBasedValidation enables GPU-based validation (slower).
	InstanceFlagsGPUBasedValidation
	// InstanceFlagsDiscardHalLabels discards HAL labels.
	InstanceFlagsDiscardHalLabels
)

// InstanceDescriptor describes how to create a GPU instance.
type InstanceDescriptor struct {
	// Backends specifies which backends to enable.
	Backends Backends
	// Flags controls instance behavior.
	Flags InstanceFlags
	// Dx12ShaderCompiler specifies the DX12 shader compiler.
	Dx12ShaderCompiler Dx12ShaderCompiler
	// GlBackend specifies the OpenGL backend flavor.
	GlBackend GlBackend
}

// DefaultInstanceDescriptor returns the default instance descriptor.
func DefaultInstanceDescriptor() InstanceDescriptor {
	return InstanceDescriptor{
		Backends:           BackendsPrimary,
		Flags:              0,
		Dx12ShaderCompiler: Dx12ShaderCompilerDxc,
		GlBackend:          GlBackendGL,
	}
}
