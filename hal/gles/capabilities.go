// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

// Adapter capability detection for the GLES backend.
//
// This file implements GL version parsing, extension probing, feature detection,
// limits querying, and device type inference -- following the patterns established
// in Rust wgpu-hal/src/gles/adapter.rs (1325 LOC). The logic is platform-independent
// and shared between Windows (WGL) and Linux (EGL) adapters.

import (
	"strings"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// AdapterCapabilities holds parsed adapter information queried from GL at
// adapter enumeration time. Populated by queryAdapterCapabilities and consumed
// by GetAdapterInfo in the platform-specific resource files.
type AdapterCapabilities struct {
	// Vendor/renderer/version strings as returned by GL.
	Vendor   string
	Renderer string
	Version  string

	// Parsed GL version numbers (e.g., [4, 6] for OpenGL 4.6).
	GLMajor int
	GLMinor int

	// True if this is an OpenGL ES context (as opposed to desktop GL).
	IsES bool

	// GLSL version number (e.g., 460 for GLSL 4.60).
	GLSLVersion int

	// Set of supported GL extensions.
	Extensions map[string]bool

	// Maximum MSAA sample count (from GL_MAX_SAMPLES).
	MaxMSAASamples int32

	// Detected features.
	Features gputypes.Features

	// Queried limits.
	Limits gputypes.Limits

	// Downlevel capability flags.
	DownlevelFlags hal.DownlevelFlags

	// Inferred device type.
	DeviceType gputypes.DeviceType

	// Inferred vendor ID.
	VendorID uint32
}

// queryAdapterCapabilities queries all adapter capabilities from the GL context.
// This is the single entry point for adapter detection; callers pass the result
// to buildExposedAdapter to construct the ExposedAdapter.
func queryAdapterCapabilities(glCtx *gl.Context) AdapterCapabilities {
	caps := AdapterCapabilities{
		Extensions: make(map[string]bool),
	}

	// --- 1. Version and vendor strings ---
	caps.Vendor = glCtx.GetString(gl.VENDOR)
	caps.Renderer = glCtx.GetString(gl.RENDERER)
	caps.Version = glCtx.GetString(gl.VERSION)

	caps.GLMajor, caps.GLMinor, caps.IsES = parseGLVersion(caps.Version)
	caps.GLSLVersion = parseGLSLVersion(glCtx.GetString(gl.SHADING_LANGUAGE_VERSION), caps.IsES)

	// --- 2. Extension set ---
	caps.Extensions = queryExtensions(glCtx)

	// --- 3. Max MSAA samples ---
	var maxSamples int32
	glCtx.GetIntegerv(gl.MAX_SAMPLES, &maxSamples)
	caps.MaxMSAASamples = maxSamples

	// --- 4. Detect features ---
	caps.Features = queryFeatures(caps.Extensions, caps.GLMajor, caps.GLMinor, caps.IsES, glCtx)

	// --- 5. Query limits ---
	caps.Limits = queryLimits(glCtx, caps.GLMajor, caps.GLMinor, caps.IsES, caps.Extensions)

	// --- 6. Downlevel flags ---
	caps.DownlevelFlags = queryDownlevelFlags(glCtx, caps.Extensions, caps.GLMajor, caps.GLMinor, caps.IsES)

	// --- 7. Device type and vendor ID ---
	caps.DeviceType = inferDeviceType(caps.Vendor, caps.Renderer)
	caps.VendorID = inferVendorID(caps.Vendor)

	hal.Logger().Info("gles: adapter capabilities detected",
		"vendor", caps.Vendor,
		"renderer", caps.Renderer,
		"version", caps.Version,
		"gl", [2]int{caps.GLMajor, caps.GLMinor},
		"es", caps.IsES,
		"glsl", caps.GLSLVersion,
		"extensions", len(caps.Extensions),
		"features", caps.Features,
		"maxMSAA", caps.MaxMSAASamples,
	)

	return caps
}

// ---------------------------------------------------------------------------
// GL version parsing
// ---------------------------------------------------------------------------

// parseGLVersion extracts the major and minor version from the GL_VERSION string.
// Returns (major, minor, isES). Handles formats:
//   - "4.6.0 NVIDIA 536.23"           -> (4, 6, false)
//   - "OpenGL ES 3.2 V@490.0"         -> (3, 2, true)
//   - "3.3 Mesa 23.0.0"               -> (3, 3, false)
//
// Returns (3, 3, false) as a safe fallback if parsing fails (our minimum is GL 3.3).
// Adapted from Rust wgpu-hal adapter.rs parse_version/parse_full_version.
func parseGLVersion(version string) (major, minor int, isES bool) {
	if version == "" {
		return 3, 3, false
	}

	src := version

	// Detect ES
	if idx := strings.Index(src, " ES "); idx >= 0 {
		isES = true
		src = src[idx+4:]
	}

	// Strip leading whitespace
	src = strings.TrimSpace(src)

	// Extract "major.minor" from the beginning
	major, minor = parseVersionNumbers(src)
	if major == 0 {
		// Fallback: GL 3.3 minimum
		return 3, 3, isES
	}
	return major, minor, isES
}

// parseVersionNumbers extracts "major.minor" from the beginning of a version string.
// e.g., "4.6.0 NVIDIA" -> (4, 6), "3.2 V@490" -> (3, 2)
func parseVersionNumbers(src string) (major, minor int) {
	// Find the dot
	dotIdx := strings.IndexByte(src, '.')
	if dotIdx < 0 || dotIdx == 0 {
		return 0, 0
	}

	// Parse major (single digit or multi-digit before dot)
	majorStr := src[:dotIdx]
	for _, c := range majorStr {
		if c >= '0' && c <= '9' {
			major = major*10 + int(c-'0')
		} else {
			return 0, 0
		}
	}

	// Parse minor (one or more digits after dot, stop at non-digit)
	rest := src[dotIdx+1:]
	for _, c := range rest {
		if c >= '0' && c <= '9' {
			minor = minor*10 + int(c-'0')
		} else {
			break
		}
	}

	return major, minor
}

// parseGLSLVersion extracts the GLSL version number from GL_SHADING_LANGUAGE_VERSION.
// Returns a value like 330 (for "3.30") or 300 (for "OpenGL ES GLSL ES 3.00").
// GLSL versions use "major.minor" where the integer form is major*100+minor.
// For example, "4.50" -> 450, "3.30" -> 330, "3.00" -> 300.
func parseGLSLVersion(slVersion string, isES bool) int {
	if slVersion == "" {
		if isES {
			return 300
		}
		return 330
	}

	src := slVersion

	// Strip "OpenGL ES GLSL ES " or "GLSL ES " prefix
	if idx := strings.Index(src, "GLSL ES "); idx >= 0 {
		src = src[idx+8:]
	}

	src = strings.TrimSpace(src)
	major, minor := parseVersionNumbers(src)
	if major == 0 {
		if isES {
			return 300
		}
		return 330
	}

	// GLSL version encoding: "3.30" -> 330, "4.50" -> 450, "3.00" -> 300.
	// The minor part in the string is already scaled (30, 50, 0), so we
	// just concatenate: major*100 + minor.
	version := major*100 + minor
	// Cap at 450 like Rust wgpu (naga doesn't support GL 460+)
	if !isES && version > 450 {
		version = 450
	}
	return version
}

// ---------------------------------------------------------------------------
// Extension probing
// ---------------------------------------------------------------------------

// queryExtensions builds a set of all supported GL extensions.
// Uses the modern glGetStringi (GL 3.0+) path which is available on both
// OpenGL 3.3+ and OpenGL ES 3.0+ -- our minimum requirements.
func queryExtensions(glCtx *gl.Context) map[string]bool {
	exts := make(map[string]bool)

	var numExts int32
	glCtx.GetIntegerv(gl.NUM_EXTENSIONS, &numExts)
	if numExts <= 0 {
		return exts
	}

	for i := int32(0); i < numExts; i++ {
		ext := glCtx.GetStringi(gl.EXTENSIONS, uint32(i))
		if ext != "" {
			exts[ext] = true
		}
	}

	return exts
}

// hasExtension checks if any of the given extension names are present.
func hasExtension(exts map[string]bool, names ...string) bool {
	for _, n := range names {
		if exts[n] {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Feature detection
// ---------------------------------------------------------------------------

// queryFeatures maps GL extensions to WebGPU feature flags.
// Follows Rust wgpu-hal adapter.rs feature detection logic.
func queryFeatures(exts map[string]bool, glMajor, glMinor int, isES bool, glCtx *gl.Context) gputypes.Features {
	var features gputypes.Features

	// Depth32FloatStencil8 -- always available on GL 3.3+ / ES 3.0+
	features.Insert(gputypes.FeatureDepth32FloatStencil8)

	// TextureAdapterSpecificFormatFeatures -- always reported
	features.Insert(gputypes.FeatureTextureAdapterSpecificFormatFeatures)

	// Depth clip control: GL_EXT_depth_clamp or GL_ARB_depth_clamp
	if hasExtension(exts, "GL_EXT_depth_clamp", "GL_ARB_depth_clamp") {
		features.Insert(gputypes.FeatureDepthClipControl)
	}

	// BC texture compression -- requires S3TC + RGTC + BPTC
	if isES {
		// GLES needs srgb variant too
		if hasExtension(exts, "GL_EXT_texture_compression_s3tc") &&
			hasExtension(exts, "GL_EXT_texture_compression_s3tc_srgb") &&
			hasExtension(exts, "GL_EXT_texture_compression_rgtc") &&
			hasExtension(exts, "GL_EXT_texture_compression_bptc") {
			features.Insert(gputypes.FeatureTextureCompressionBC)
		}
	} else {
		if hasExtension(exts, "GL_EXT_texture_compression_s3tc") &&
			hasExtension(exts, "GL_EXT_texture_compression_rgtc") &&
			hasExtension(exts, "GL_ARB_texture_compression_bptc") {
			features.Insert(gputypes.FeatureTextureCompressionBC)
		}
	}

	// ETC2 -- built into ES 3.0+, needs GL_ARB_ES3_compatibility on desktop
	if isES || hasExtension(exts, "GL_ARB_ES3_compatibility") {
		features.Insert(gputypes.FeatureTextureCompressionETC2)
	}

	// ASTC
	if hasExtension(exts, "GL_OES_texture_compression_astc",
		"GL_KHR_texture_compression_astc_ldr") {
		features.Insert(gputypes.FeatureTextureCompressionASTC)
	}

	// Float32 filterable (linear filtering for float textures)
	if hasExtension(exts, "GL_ARB_color_buffer_float",
		"GL_EXT_color_buffer_float",
		"OES_texture_float_linear") {
		features.Insert(gputypes.FeatureFloat32Filterable)
	}

	// Timestamp queries (GL_ARB_timer_query, desktop GL 3.3+)
	if hasExtension(exts, "GL_ARB_timer_query") || glCtx.SupportsTimestampQueries() {
		features.Insert(gputypes.FeatureTimestampQuery)
	}

	// RG11B10Ufloat renderable -- available when EXT_color_buffer_float present
	if hasExtension(exts, "GL_EXT_color_buffer_float", "GL_ARB_color_buffer_float") {
		features.Insert(gputypes.FeatureRG11B10UfloatRenderable)
	}

	// IndirectFirstInstance -- desktop GL 4.2+ with ARB_shader_draw_parameters
	if !isES && glMajor >= 4 && glMinor >= 2 &&
		hasExtension(exts, "GL_ARB_shader_draw_parameters") {
		features.Insert(gputypes.FeatureIndirectFirstInstance)
	}

	return features
}

// ---------------------------------------------------------------------------
// Limits querying
// ---------------------------------------------------------------------------

// getGLInt queries a single GL integer parameter. Returns the fallback value
// if the result is <= 0.
func getGLInt(glCtx *gl.Context, pname uint32, fallback int32) int32 {
	var val int32
	glCtx.GetIntegerv(pname, &val)
	if val <= 0 {
		return fallback
	}
	return val
}

// glVersionAtLeast returns true if the GL version is >= (reqMajor, reqMinor)
// for ES or (reqFullMajor, reqFullMinor) for desktop GL.
func glVersionAtLeast(glMajor, glMinor int, isES bool, reqES, reqFull [2]int) bool {
	if isES {
		return glMajor > reqES[0] || (glMajor == reqES[0] && glMinor >= reqES[1])
	}
	return glMajor > reqFull[0] || (glMajor == reqFull[0] && glMinor >= reqFull[1])
}

// queryLimits queries GL limits and returns a populated Limits struct.
// Follows Rust wgpu-hal adapter.rs limits construction.
func queryLimits(glCtx *gl.Context, glMajor, glMinor int, isES bool, exts map[string]bool) gputypes.Limits {
	supportsStorage := glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 3}) ||
		hasExtension(exts, "GL_ARB_shader_storage_buffer_object")
	supportsCompute := glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 3}) ||
		hasExtension(exts, "GL_ARB_compute_shader")

	maxTextureSize := getGLInt(glCtx, gl.MAX_TEXTURE_SIZE, 2048)
	maxTexture3DSize := getGLInt(glCtx, gl.MAX_3D_TEXTURE_SIZE, 256)
	maxArrayLayers := getGLInt(glCtx, gl.MAX_ARRAY_TEXTURE_LAYERS, 256)
	maxVertexAttribs := getGLInt(glCtx, gl.MAX_VERTEX_ATTRIBS, 16)

	// Uniform buffer limits
	maxVertexUBOs := getGLInt(glCtx, gl.MAX_VERTEX_UNIFORM_BLOCKS, 12)
	maxFragmentUBOs := getGLInt(glCtx, gl.MAX_FRAGMENT_UNIFORM_BLOCKS, 12)
	maxUBOsPerStage := minI32(maxVertexUBOs, maxFragmentUBOs)
	maxUBOSize := getGLInt(glCtx, gl.MAX_UNIFORM_BLOCK_SIZE, 16384)
	uboAlignment := getGLInt(glCtx, gl.UNIFORM_BUFFER_OFFSET_ALIGNMENT, 256)

	// Storage buffer limits (ES 3.1+ / GL 4.3+)
	var maxSSBOsPerStage int32
	var maxSSBOSize int32
	var ssboAlignment int32
	var maxStorageTextures int32
	if supportsStorage {
		maxSSBOsPerStage = queryMinPerStage(glCtx,
			gl.MAX_VERTEX_SHADER_STORAGE_BLOCKS, gl.MAX_FRAGMENT_SHADER_STORAGE_BLOCKS)
		maxSSBOSize = getGLInt(glCtx, gl.MAX_SHADER_STORAGE_BLOCK_SIZE, 0)
		ssboAlignment = getGLInt(glCtx, gl.SHADER_STORAGE_BUFFER_OFFSET_ALIGNMENT, 256)
		maxStorageTextures = queryMinPerStage(glCtx,
			gl.MAX_VERTEX_IMAGE_UNIFORMS, gl.MAX_FRAGMENT_IMAGE_UNIFORMS)
	} else {
		ssboAlignment = 256
	}

	// Color attachments
	maxColorAttachments := getGLInt(glCtx, gl.MAX_COLOR_ATTACHMENTS, 4)
	maxDrawBuffers := getGLInt(glCtx, gl.MAX_DRAW_BUFFERS, 4)
	if maxDrawBuffers < maxColorAttachments {
		maxColorAttachments = maxDrawBuffers
	}
	if maxColorAttachments > 8 {
		maxColorAttachments = 8 // WebGPU max
	}

	// Texture units / samplers
	maxTextureUnits := getGLInt(glCtx, gl.MAX_TEXTURE_IMAGE_UNITS, 16)

	// Vertex buffers
	var maxVertexBuffers int32
	var maxVertexStride int32
	if hasExtension(exts, "GL_ARB_vertex_attrib_binding") ||
		glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 3}) {
		maxVertexBuffers = getGLInt(glCtx, gl.MAX_VERTEX_ATTRIB_BINDINGS, 16)
		maxVertexStride = getGLInt(glCtx, gl.MAX_VERTEX_ATTRIB_STRIDE, 2048)
		if maxVertexStride == 0 {
			maxVertexStride = 2048
		}
	} else {
		maxVertexBuffers = 16
		maxVertexStride = 2048
	}

	// Varying components (inter-stage shader variables)
	maxVaryingComponents := getGLInt(glCtx, gl.MAX_VARYING_COMPONENTS, 60)
	if maxVaryingComponents == 0 {
		// MAX_VARYING_COMPONENTS is deprecated in OpenGL 3.2+ core profile;
		// some drivers return 0. Use the WebGPU default.
		maxVaryingComponents = 60
	}

	// Compute limits (ES 3.1+ / GL 4.3+)
	var computeSharedMem int32
	var computeInvocations int32
	var computeWorkgroupsPerDim int32
	// NOTE: compute workgroup size per-axis requires glGetIntegeri_v which we
	// don't expose yet. Use conservative defaults matching the GL spec minimums.
	var computeWGSizeX, computeWGSizeY, computeWGSizeZ int32
	if supportsCompute {
		computeSharedMem = getGLInt(glCtx, gl.MAX_COMPUTE_SHARED_MEMORY_SIZE, 16384)
		computeInvocations = getGLInt(glCtx, gl.MAX_COMPUTE_WORK_GROUP_INVOCATIONS, 256)
		// Without glGetIntegeri_v, use the guaranteed minimums from the GL spec.
		// ES 3.1 / GL 4.3 guarantee at least 1024 for X/Y and 64 for Z.
		computeWGSizeX = 1024
		computeWGSizeY = 1024
		computeWGSizeZ = 64
		// Workgroups per dimension -- also indexed but we can approximate.
		// GL spec guarantees at least 65535 per dimension.
		computeWorkgroupsPerDim = 65535
	}

	return gputypes.Limits{
		MaxTextureDimension1D:                     uint32(maxTextureSize),
		MaxTextureDimension2D:                     uint32(maxTextureSize),
		MaxTextureDimension3D:                     uint32(maxTexture3DSize),
		MaxTextureArrayLayers:                     uint32(maxArrayLayers),
		MaxBindGroups:                             4,    // GLES fixed limit
		MaxBindGroupsPlusVertexBuffers:            24,   // WebGPU default
		MaxBindingsPerBindGroup:                   1000, // WebGPU default
		MaxDynamicUniformBuffersPerPipelineLayout: uint32(maxUBOsPerStage),
		MaxDynamicStorageBuffersPerPipelineLayout: uint32(maxSSBOsPerStage),
		MaxSampledTexturesPerShaderStage:          uint32(maxTextureUnits),
		MaxSamplersPerShaderStage:                 16, // WebGPU default, safe for GL
		MaxStorageBuffersPerShaderStage:           uint32(maxSSBOsPerStage),
		MaxStorageTexturesPerShaderStage:          uint32(maxStorageTextures),
		MaxUniformBuffersPerShaderStage:           uint32(maxUBOsPerStage),
		MaxUniformBufferBindingSize:               uint64(maxUBOSize),
		MaxStorageBufferBindingSize:               uint64(maxSSBOSize),
		MinUniformBufferOffsetAlignment:           uint32(uboAlignment),
		MinStorageBufferOffsetAlignment:           uint32(ssboAlignment),
		MaxVertexBuffers:                          uint32(maxVertexBuffers),
		MaxBufferSize:                             1<<31 - 1, // i32::MAX
		MaxVertexAttributes:                       uint32(maxVertexAttribs),
		MaxVertexBufferArrayStride:                uint32(maxVertexStride),
		MaxInterStageShaderVariables:              uint32(maxVaryingComponents),
		MaxColorAttachments:                       uint32(maxColorAttachments),
		MaxColorAttachmentBytesPerSample:          uint32(maxColorAttachments) * 16, // 16 bytes max per attachment
		MaxComputeWorkgroupStorageSize:            uint32(computeSharedMem),
		MaxComputeInvocationsPerWorkgroup:         uint32(computeInvocations),
		MaxComputeWorkgroupSizeX:                  uint32(computeWGSizeX),
		MaxComputeWorkgroupSizeY:                  uint32(computeWGSizeY),
		MaxComputeWorkgroupSizeZ:                  uint32(computeWGSizeZ),
		MaxComputeWorkgroupsPerDimension:          uint32(computeWorkgroupsPerDim),
		MaxPushConstantSize:                       0,       // Not in WebGPU spec
		MaxNonSamplerBindings:                     1000000, // WebGPU default
	}
}

// ---------------------------------------------------------------------------
// Downlevel capability flags
// ---------------------------------------------------------------------------

// queryDownlevelFlags computes the downlevel capability flags from the GL context.
// Follows Rust wgpu-hal adapter.rs downlevel_flags logic.
func queryDownlevelFlags(glCtx *gl.Context, exts map[string]bool, glMajor, glMinor int, isES bool) hal.DownlevelFlags {
	var flags hal.DownlevelFlags

	supportsCompute := glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 3}) ||
		hasExtension(exts, "GL_ARB_compute_shader")
	supportsStorage := glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 1}, [2]int{4, 3}) ||
		hasExtension(exts, "GL_ARB_shader_storage_buffer_object")

	if supportsCompute {
		flags |= hal.DownlevelFlagsComputeShaders
	}

	if supportsStorage {
		var maxSSBOSize int32
		glCtx.GetIntegerv(gl.MAX_SHADER_STORAGE_BLOCK_SIZE, &maxSSBOSize)
		if maxSSBOSize > 0 {
			flags |= hal.DownlevelFlagsFragmentWritableStorage
		}
	}

	// Base vertex support: ES 3.2+ / GL 3.2+
	if glVersionAtLeast(glMajor, glMinor, isES, [2]int{3, 2}, [2]int{3, 2}) {
		flags |= hal.DownlevelFlagsBaseVertexBaseInstance
	}

	// Anisotropic filtering
	if hasExtension(exts, "EXT_texture_filter_anisotropic", "GL_EXT_texture_filter_anisotropic") {
		var maxAniso int32
		glCtx.GetIntegerv(gl.MAX_TEXTURE_MAX_ANISOTROPY, &maxAniso)
		if maxAniso >= 16 {
			flags |= hal.DownlevelFlagsAnisotropicFiltering
		}
	}

	return flags
}

// ---------------------------------------------------------------------------
// Device type inference
// ---------------------------------------------------------------------------

// inferDeviceType infers the device type from vendor and renderer strings.
// Adapted from Rust wgpu-hal adapter.rs make_info.
func inferDeviceType(vendor, renderer string) gputypes.DeviceType {
	v := strings.ToLower(vendor)
	r := strings.ToLower(renderer)

	// CPU renderers
	cpuStrings := []string{"mesa offscreen", "swiftshader", "llvmpipe"}
	for _, s := range cpuStrings {
		if strings.Contains(r, s) {
			return gputypes.DeviceTypeCPU
		}
	}

	// Integrated GPU indicators
	integratedStrings := []string{
		" xpress", // space intentional -- don't match "express"
		"amd renoir",
		"radeon hd 4200", "radeon hd 4250", "radeon hd 4290",
		"radeon hd 4270", "radeon hd 4225",
		"radeon hd 3100", "radeon hd 3200", "radeon hd 3000", "radeon hd 3300",
		"radeon(tm) r4 graphics", "radeon(tm) r5 graphics",
		"radeon(tm) r6 graphics", "radeon(tm) r7 graphics",
		"radeon r7 graphics",
		"nforce", "tegra", "shield", // NVIDIA integrated
		"igp", "mali",
		"intel",
		"v3d",
		"apple m",
	}

	if strings.Contains(v, "qualcomm") || strings.Contains(v, "intel") {
		return gputypes.DeviceTypeIntegratedGPU
	}
	for _, s := range integratedStrings {
		if strings.Contains(r, s) {
			return gputypes.DeviceTypeIntegratedGPU
		}
	}

	// Default to Other (not DiscreteGPU) to avoid incorrect assumptions,
	// matching the Rust wgpu-hal approach.
	return gputypes.DeviceTypeOther
}

// Known PCI vendor IDs (matches Rust wgpu auxil::db constants).
const (
	vendorIDAMD      uint32 = 0x1002
	vendorIDImgTec   uint32 = 0x1010
	vendorIDNVIDIA   uint32 = 0x10DE
	vendorIDARM      uint32 = 0x13B5
	vendorIDQualcomm uint32 = 0x5143
	vendorIDIntel    uint32 = 0x8086
	vendorIDBroadcom uint32 = 0x14E4
	vendorIDMesa     uint32 = 0x10005
	vendorIDApple    uint32 = 0x106B
)

// inferVendorID returns a PCI vendor ID based on the GL_VENDOR string.
// Order matters: "nvidia corporation" contains "ati" as a substring, so
// NVIDIA must be checked before ATI/AMD.
func inferVendorID(vendor string) uint32 {
	v := strings.ToLower(vendor)
	switch {
	case strings.Contains(v, "nvidia"):
		return vendorIDNVIDIA
	case strings.Contains(v, "amd") || strings.Contains(v, "ati"):
		return vendorIDAMD
	case strings.Contains(v, "imgtec"):
		return vendorIDImgTec
	case strings.Contains(v, "qualcomm"):
		return vendorIDQualcomm
	case strings.Contains(v, "intel"):
		return vendorIDIntel
	case strings.Contains(v, "arm"):
		return vendorIDARM
	case strings.Contains(v, "broadcom"):
		return vendorIDBroadcom
	case strings.Contains(v, "mesa"):
		return vendorIDMesa
	case strings.Contains(v, "apple"):
		return vendorIDApple
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// Texture format capabilities (shared, extension-aware)
// ---------------------------------------------------------------------------

// queryTextureFormatCapabilities returns per-format capability flags based on
// the adapter's probed capabilities. This replaces the hardcoded switch in the
// platform-specific adapter files.
//
// Follows Rust wgpu-hal adapter.rs texture_format_capabilities, matching the
// OpenGL ES 3.0 spec table 3.8 (base types) and table 8.26 (image stores).
func queryTextureFormatCapabilities(
	format gputypes.TextureFormat,
	features gputypes.Features,
	maxMSAA int32,
	exts map[string]bool,
) hal.TextureFormatCapabilities {
	// MSAA capability tier based on GL_MAX_SAMPLES.
	// GL ES 3.0 guarantees at least 4x. Drivers may report 0 (e.g., iOS Safari),
	// in which case we still advertise 4x as a safe baseline (Rust wgpu-hal parity).
	var msaa hal.TextureFormatCapabilityFlags
	if maxMSAA >= 4 || maxMSAA == 0 {
		msaa = hal.TextureFormatCapabilityMultisample | hal.TextureFormatCapabilityMultisampleResolve
	}

	base := hal.TextureFormatCapabilitySampled
	depth := base | hal.TextureFormatCapabilityRenderAttachment | msaa
	renderable := base | hal.TextureFormatCapabilityRenderAttachment | msaa |
		hal.TextureFormatCapabilityMultisampleResolve
	filterableRenderable := renderable | hal.TextureFormatCapabilityBlendable
	storage := base | hal.TextureFormatCapabilityStorage | hal.TextureFormatCapabilityStorageReadWrite

	// Helper for optional features gating compressed format capabilities.
	compressionCaps := func(feature gputypes.Feature) hal.TextureFormatCapabilityFlags {
		if features.Contains(feature) {
			return base
		}
		return 0
	}

	// Half-float renderability: GL_EXT_color_buffer_half_float or GL_EXT_color_buffer_float
	hasHalfFloatRender := hasExtension(exts, "GL_EXT_color_buffer_half_float",
		"GL_EXT_color_buffer_float", "GL_ARB_color_buffer_float",
		"GL_ARB_half_float_pixel")
	halfFloatRender := hal.TextureFormatCapabilityFlags(0)
	if hasHalfFloatRender {
		halfFloatRender = hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable | msaa
	}

	// Float renderability: GL_EXT_color_buffer_float
	hasFloatRender := hasExtension(exts, "GL_EXT_color_buffer_float", "GL_ARB_color_buffer_float")
	floatRender := hal.TextureFormatCapabilityFlags(0)
	if hasFloatRender {
		floatRender = hal.TextureFormatCapabilityRenderAttachment |
			hal.TextureFormatCapabilityBlendable | msaa
	}

	var flags hal.TextureFormatCapabilityFlags
	switch format {
	// --- 8-bit single/dual channel ---
	case gputypes.TextureFormatR8Unorm, gputypes.TextureFormatRG8Unorm:
		flags = filterableRenderable
	case gputypes.TextureFormatR8Snorm, gputypes.TextureFormatRG8Snorm:
		flags = base // filterable but not renderable
	case gputypes.TextureFormatR8Uint, gputypes.TextureFormatR8Sint,
		gputypes.TextureFormatRG8Uint, gputypes.TextureFormatRG8Sint:
		flags = renderable

	// --- 16-bit ---
	case gputypes.TextureFormatR16Uint, gputypes.TextureFormatR16Sint,
		gputypes.TextureFormatRG16Uint, gputypes.TextureFormatRG16Sint:
		flags = renderable
	case gputypes.TextureFormatR16Float, gputypes.TextureFormatRG16Float:
		flags = base | halfFloatRender

	// --- 32-bit ---
	case gputypes.TextureFormatR32Uint, gputypes.TextureFormatR32Sint:
		flags = renderable | storage
	case gputypes.TextureFormatR32Float:
		flags = base | storage | floatRender

	// --- RGBA 8-bit ---
	case gputypes.TextureFormatRGBA8Unorm:
		flags = filterableRenderable | storage
	case gputypes.TextureFormatRGBA8UnormSrgb:
		flags = filterableRenderable
	case gputypes.TextureFormatRGBA8Snorm:
		flags = base | storage // filterable but not renderable
	case gputypes.TextureFormatRGBA8Uint, gputypes.TextureFormatRGBA8Sint:
		flags = renderable | storage
	case gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb:
		flags = filterableRenderable

	// --- Packed ---
	case gputypes.TextureFormatRGB10A2Uint:
		flags = renderable
	case gputypes.TextureFormatRGB10A2Unorm:
		flags = filterableRenderable
	case gputypes.TextureFormatRG11B10Ufloat:
		flags = base | floatRender
	case gputypes.TextureFormatRGB9E5Ufloat:
		flags = base // filterable only

	// --- 64-bit ---
	case gputypes.TextureFormatRG32Uint, gputypes.TextureFormatRG32Sint:
		flags = renderable
	case gputypes.TextureFormatRG32Float:
		flags = base | floatRender

	// --- 128-bit ---
	case gputypes.TextureFormatRGBA16Uint, gputypes.TextureFormatRGBA16Sint:
		flags = renderable | storage
	case gputypes.TextureFormatRGBA16Float:
		flags = base | storage | halfFloatRender
	case gputypes.TextureFormatRGBA32Uint, gputypes.TextureFormatRGBA32Sint:
		flags = renderable | storage
	case gputypes.TextureFormatRGBA32Float:
		flags = base | storage | floatRender

	// --- Depth/stencil ---
	case gputypes.TextureFormatStencil8,
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
		flags = depth

	// --- BC compressed ---
	case gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb,
		gputypes.TextureFormatBC2RGBAUnorm, gputypes.TextureFormatBC2RGBAUnormSrgb,
		gputypes.TextureFormatBC3RGBAUnorm, gputypes.TextureFormatBC3RGBAUnormSrgb,
		gputypes.TextureFormatBC4RUnorm, gputypes.TextureFormatBC4RSnorm,
		gputypes.TextureFormatBC5RGUnorm, gputypes.TextureFormatBC5RGSnorm,
		gputypes.TextureFormatBC6HRGBUfloat, gputypes.TextureFormatBC6HRGBFloat,
		gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb:
		flags = compressionCaps(gputypes.FeatureTextureCompressionBC)

	// --- ETC2 compressed ---
	case gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb,
		gputypes.TextureFormatETC2RGB8A1Unorm, gputypes.TextureFormatETC2RGB8A1UnormSrgb,
		gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb,
		gputypes.TextureFormatEACR11Unorm, gputypes.TextureFormatEACR11Snorm,
		gputypes.TextureFormatEACRG11Unorm, gputypes.TextureFormatEACRG11Snorm:
		flags = compressionCaps(gputypes.FeatureTextureCompressionETC2)

	// --- ASTC compressed ---
	case gputypes.TextureFormatASTC4x4Unorm, gputypes.TextureFormatASTC4x4UnormSrgb,
		gputypes.TextureFormatASTC5x4Unorm, gputypes.TextureFormatASTC5x4UnormSrgb,
		gputypes.TextureFormatASTC5x5Unorm, gputypes.TextureFormatASTC5x5UnormSrgb,
		gputypes.TextureFormatASTC6x5Unorm, gputypes.TextureFormatASTC6x5UnormSrgb,
		gputypes.TextureFormatASTC6x6Unorm, gputypes.TextureFormatASTC6x6UnormSrgb,
		gputypes.TextureFormatASTC8x5Unorm, gputypes.TextureFormatASTC8x5UnormSrgb,
		gputypes.TextureFormatASTC8x6Unorm, gputypes.TextureFormatASTC8x6UnormSrgb,
		gputypes.TextureFormatASTC8x8Unorm, gputypes.TextureFormatASTC8x8UnormSrgb,
		gputypes.TextureFormatASTC10x5Unorm, gputypes.TextureFormatASTC10x5UnormSrgb,
		gputypes.TextureFormatASTC10x6Unorm, gputypes.TextureFormatASTC10x6UnormSrgb,
		gputypes.TextureFormatASTC10x8Unorm, gputypes.TextureFormatASTC10x8UnormSrgb,
		gputypes.TextureFormatASTC10x10Unorm, gputypes.TextureFormatASTC10x10UnormSrgb,
		gputypes.TextureFormatASTC12x10Unorm, gputypes.TextureFormatASTC12x10UnormSrgb,
		gputypes.TextureFormatASTC12x12Unorm, gputypes.TextureFormatASTC12x12UnormSrgb:
		flags = compressionCaps(gputypes.FeatureTextureCompressionASTC)

	default:
		flags = 0
	}

	return hal.TextureFormatCapabilities{Flags: flags}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func minI32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// queryMinPerStage queries two per-stage GL parameters (vertex and fragment)
// and returns the minimum of the two. If the vertex value is zero (some drivers
// report 0 for vertex SSBOs), the fragment value is used alone.
// Adapted from Rust wgpu-hal adapter.rs vertex_ssbo_false_zero logic.
func queryMinPerStage(glCtx *gl.Context, vertexParam, fragmentParam uint32) int32 {
	vertex := getGLInt(glCtx, vertexParam, 0)
	fragment := getGLInt(glCtx, fragmentParam, 0)
	if vertex == 0 {
		return fragment
	}
	return minI32(vertex, fragment)
}
