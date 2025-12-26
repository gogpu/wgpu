// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dxgi

// DXGI_FORMAT specifies the data format for resources.
// Note: This is also defined in d3d12 package. We define it here for
// DXGI-specific contexts where importing d3d12 is not desirable.
type DXGI_FORMAT uint32

// DXGI format constants.
const (
	DXGI_FORMAT_UNKNOWN                                 DXGI_FORMAT = 0
	DXGI_FORMAT_R32G32B32A32_TYPELESS                   DXGI_FORMAT = 1
	DXGI_FORMAT_R32G32B32A32_FLOAT                      DXGI_FORMAT = 2
	DXGI_FORMAT_R32G32B32A32_UINT                       DXGI_FORMAT = 3
	DXGI_FORMAT_R32G32B32A32_SINT                       DXGI_FORMAT = 4
	DXGI_FORMAT_R32G32B32_TYPELESS                      DXGI_FORMAT = 5
	DXGI_FORMAT_R32G32B32_FLOAT                         DXGI_FORMAT = 6
	DXGI_FORMAT_R32G32B32_UINT                          DXGI_FORMAT = 7
	DXGI_FORMAT_R32G32B32_SINT                          DXGI_FORMAT = 8
	DXGI_FORMAT_R16G16B16A16_TYPELESS                   DXGI_FORMAT = 9
	DXGI_FORMAT_R16G16B16A16_FLOAT                      DXGI_FORMAT = 10
	DXGI_FORMAT_R16G16B16A16_UNORM                      DXGI_FORMAT = 11
	DXGI_FORMAT_R16G16B16A16_UINT                       DXGI_FORMAT = 12
	DXGI_FORMAT_R16G16B16A16_SNORM                      DXGI_FORMAT = 13
	DXGI_FORMAT_R16G16B16A16_SINT                       DXGI_FORMAT = 14
	DXGI_FORMAT_R32G32_TYPELESS                         DXGI_FORMAT = 15
	DXGI_FORMAT_R32G32_FLOAT                            DXGI_FORMAT = 16
	DXGI_FORMAT_R32G32_UINT                             DXGI_FORMAT = 17
	DXGI_FORMAT_R32G32_SINT                             DXGI_FORMAT = 18
	DXGI_FORMAT_R32G8X24_TYPELESS                       DXGI_FORMAT = 19
	DXGI_FORMAT_D32_FLOAT_S8X24_UINT                    DXGI_FORMAT = 20
	DXGI_FORMAT_R32_FLOAT_X8X24_TYPELESS                DXGI_FORMAT = 21
	DXGI_FORMAT_X32_TYPELESS_G8X24_UINT                 DXGI_FORMAT = 22
	DXGI_FORMAT_R10G10B10A2_TYPELESS                    DXGI_FORMAT = 23
	DXGI_FORMAT_R10G10B10A2_UNORM                       DXGI_FORMAT = 24
	DXGI_FORMAT_R10G10B10A2_UINT                        DXGI_FORMAT = 25
	DXGI_FORMAT_R11G11B10_FLOAT                         DXGI_FORMAT = 26
	DXGI_FORMAT_R8G8B8A8_TYPELESS                       DXGI_FORMAT = 27
	DXGI_FORMAT_R8G8B8A8_UNORM                          DXGI_FORMAT = 28
	DXGI_FORMAT_R8G8B8A8_UNORM_SRGB                     DXGI_FORMAT = 29
	DXGI_FORMAT_R8G8B8A8_UINT                           DXGI_FORMAT = 30
	DXGI_FORMAT_R8G8B8A8_SNORM                          DXGI_FORMAT = 31
	DXGI_FORMAT_R8G8B8A8_SINT                           DXGI_FORMAT = 32
	DXGI_FORMAT_R16G16_TYPELESS                         DXGI_FORMAT = 33
	DXGI_FORMAT_R16G16_FLOAT                            DXGI_FORMAT = 34
	DXGI_FORMAT_R16G16_UNORM                            DXGI_FORMAT = 35
	DXGI_FORMAT_R16G16_UINT                             DXGI_FORMAT = 36
	DXGI_FORMAT_R16G16_SNORM                            DXGI_FORMAT = 37
	DXGI_FORMAT_R16G16_SINT                             DXGI_FORMAT = 38
	DXGI_FORMAT_R32_TYPELESS                            DXGI_FORMAT = 39
	DXGI_FORMAT_D32_FLOAT                               DXGI_FORMAT = 40
	DXGI_FORMAT_R32_FLOAT                               DXGI_FORMAT = 41
	DXGI_FORMAT_R32_UINT                                DXGI_FORMAT = 42
	DXGI_FORMAT_R32_SINT                                DXGI_FORMAT = 43
	DXGI_FORMAT_R24G8_TYPELESS                          DXGI_FORMAT = 44
	DXGI_FORMAT_D24_UNORM_S8_UINT                       DXGI_FORMAT = 45
	DXGI_FORMAT_R24_UNORM_X8_TYPELESS                   DXGI_FORMAT = 46
	DXGI_FORMAT_X24_TYPELESS_G8_UINT                    DXGI_FORMAT = 47
	DXGI_FORMAT_R8G8_TYPELESS                           DXGI_FORMAT = 48
	DXGI_FORMAT_R8G8_UNORM                              DXGI_FORMAT = 49
	DXGI_FORMAT_R8G8_UINT                               DXGI_FORMAT = 50
	DXGI_FORMAT_R8G8_SNORM                              DXGI_FORMAT = 51
	DXGI_FORMAT_R8G8_SINT                               DXGI_FORMAT = 52
	DXGI_FORMAT_R16_TYPELESS                            DXGI_FORMAT = 53
	DXGI_FORMAT_R16_FLOAT                               DXGI_FORMAT = 54
	DXGI_FORMAT_D16_UNORM                               DXGI_FORMAT = 55
	DXGI_FORMAT_R16_UNORM                               DXGI_FORMAT = 56
	DXGI_FORMAT_R16_UINT                                DXGI_FORMAT = 57
	DXGI_FORMAT_R16_SNORM                               DXGI_FORMAT = 58
	DXGI_FORMAT_R16_SINT                                DXGI_FORMAT = 59
	DXGI_FORMAT_R8_TYPELESS                             DXGI_FORMAT = 60
	DXGI_FORMAT_R8_UNORM                                DXGI_FORMAT = 61
	DXGI_FORMAT_R8_UINT                                 DXGI_FORMAT = 62
	DXGI_FORMAT_R8_SNORM                                DXGI_FORMAT = 63
	DXGI_FORMAT_R8_SINT                                 DXGI_FORMAT = 64
	DXGI_FORMAT_A8_UNORM                                DXGI_FORMAT = 65
	DXGI_FORMAT_R1_UNORM                                DXGI_FORMAT = 66
	DXGI_FORMAT_R9G9B9E5_SHAREDEXP                      DXGI_FORMAT = 67
	DXGI_FORMAT_R8G8_B8G8_UNORM                         DXGI_FORMAT = 68
	DXGI_FORMAT_G8R8_G8B8_UNORM                         DXGI_FORMAT = 69
	DXGI_FORMAT_BC1_TYPELESS                            DXGI_FORMAT = 70
	DXGI_FORMAT_BC1_UNORM                               DXGI_FORMAT = 71
	DXGI_FORMAT_BC1_UNORM_SRGB                          DXGI_FORMAT = 72
	DXGI_FORMAT_BC2_TYPELESS                            DXGI_FORMAT = 73
	DXGI_FORMAT_BC2_UNORM                               DXGI_FORMAT = 74
	DXGI_FORMAT_BC2_UNORM_SRGB                          DXGI_FORMAT = 75
	DXGI_FORMAT_BC3_TYPELESS                            DXGI_FORMAT = 76
	DXGI_FORMAT_BC3_UNORM                               DXGI_FORMAT = 77
	DXGI_FORMAT_BC3_UNORM_SRGB                          DXGI_FORMAT = 78
	DXGI_FORMAT_BC4_TYPELESS                            DXGI_FORMAT = 79
	DXGI_FORMAT_BC4_UNORM                               DXGI_FORMAT = 80
	DXGI_FORMAT_BC4_SNORM                               DXGI_FORMAT = 81
	DXGI_FORMAT_BC5_TYPELESS                            DXGI_FORMAT = 82
	DXGI_FORMAT_BC5_UNORM                               DXGI_FORMAT = 83
	DXGI_FORMAT_BC5_SNORM                               DXGI_FORMAT = 84
	DXGI_FORMAT_B5G6R5_UNORM                            DXGI_FORMAT = 85
	DXGI_FORMAT_B5G5R5A1_UNORM                          DXGI_FORMAT = 86
	DXGI_FORMAT_B8G8R8A8_UNORM                          DXGI_FORMAT = 87
	DXGI_FORMAT_B8G8R8X8_UNORM                          DXGI_FORMAT = 88
	DXGI_FORMAT_R10G10B10_XR_BIAS_A2_UNORM              DXGI_FORMAT = 89
	DXGI_FORMAT_B8G8R8A8_TYPELESS                       DXGI_FORMAT = 90
	DXGI_FORMAT_B8G8R8A8_UNORM_SRGB                     DXGI_FORMAT = 91
	DXGI_FORMAT_B8G8R8X8_TYPELESS                       DXGI_FORMAT = 92
	DXGI_FORMAT_B8G8R8X8_UNORM_SRGB                     DXGI_FORMAT = 93
	DXGI_FORMAT_BC6H_TYPELESS                           DXGI_FORMAT = 94
	DXGI_FORMAT_BC6H_UF16                               DXGI_FORMAT = 95
	DXGI_FORMAT_BC6H_SF16                               DXGI_FORMAT = 96
	DXGI_FORMAT_BC7_TYPELESS                            DXGI_FORMAT = 97
	DXGI_FORMAT_BC7_UNORM                               DXGI_FORMAT = 98
	DXGI_FORMAT_BC7_UNORM_SRGB                          DXGI_FORMAT = 99
	DXGI_FORMAT_AYUV                                    DXGI_FORMAT = 100
	DXGI_FORMAT_Y410                                    DXGI_FORMAT = 101
	DXGI_FORMAT_Y416                                    DXGI_FORMAT = 102
	DXGI_FORMAT_NV12                                    DXGI_FORMAT = 103
	DXGI_FORMAT_P010                                    DXGI_FORMAT = 104
	DXGI_FORMAT_P016                                    DXGI_FORMAT = 105
	DXGI_FORMAT_420_OPAQUE                              DXGI_FORMAT = 106
	DXGI_FORMAT_YUY2                                    DXGI_FORMAT = 107
	DXGI_FORMAT_Y210                                    DXGI_FORMAT = 108
	DXGI_FORMAT_Y216                                    DXGI_FORMAT = 109
	DXGI_FORMAT_NV11                                    DXGI_FORMAT = 110
	DXGI_FORMAT_AI44                                    DXGI_FORMAT = 111
	DXGI_FORMAT_IA44                                    DXGI_FORMAT = 112
	DXGI_FORMAT_P8                                      DXGI_FORMAT = 113
	DXGI_FORMAT_A8P8                                    DXGI_FORMAT = 114
	DXGI_FORMAT_B4G4R4A4_UNORM                          DXGI_FORMAT = 115
	DXGI_FORMAT_P208                                    DXGI_FORMAT = 130
	DXGI_FORMAT_V208                                    DXGI_FORMAT = 131
	DXGI_FORMAT_V408                                    DXGI_FORMAT = 132
	DXGI_FORMAT_SAMPLER_FEEDBACK_MIN_MIP_OPAQUE         DXGI_FORMAT = 189
	DXGI_FORMAT_SAMPLER_FEEDBACK_MIP_REGION_USED_OPAQUE DXGI_FORMAT = 190
)

// DXGI_SWAP_EFFECT specifies options for swap chain presentation behavior.
type DXGI_SWAP_EFFECT uint32

// Swap effect constants.
const (
	DXGI_SWAP_EFFECT_DISCARD         DXGI_SWAP_EFFECT = 0
	DXGI_SWAP_EFFECT_SEQUENTIAL      DXGI_SWAP_EFFECT = 1
	DXGI_SWAP_EFFECT_FLIP_SEQUENTIAL DXGI_SWAP_EFFECT = 3
	DXGI_SWAP_EFFECT_FLIP_DISCARD    DXGI_SWAP_EFFECT = 4
)

// DXGI_SCALING specifies scaling behavior when the back buffer is presented.
type DXGI_SCALING uint32

// Scaling constants.
const (
	DXGI_SCALING_STRETCH              DXGI_SCALING = 0
	DXGI_SCALING_NONE                 DXGI_SCALING = 1
	DXGI_SCALING_ASPECT_RATIO_STRETCH DXGI_SCALING = 2
)

// DXGI_ALPHA_MODE specifies alpha blending behavior.
type DXGI_ALPHA_MODE uint32

// Alpha mode constants.
const (
	DXGI_ALPHA_MODE_UNSPECIFIED   DXGI_ALPHA_MODE = 0
	DXGI_ALPHA_MODE_PREMULTIPLIED DXGI_ALPHA_MODE = 1
	DXGI_ALPHA_MODE_STRAIGHT      DXGI_ALPHA_MODE = 2
	DXGI_ALPHA_MODE_IGNORE        DXGI_ALPHA_MODE = 3
)

// DXGI_USAGE specifies how a surface or resource is intended to be used.
type DXGI_USAGE uint32

// Usage constants.
const (
	DXGI_USAGE_SHADER_INPUT         DXGI_USAGE = 0x00000010
	DXGI_USAGE_RENDER_TARGET_OUTPUT DXGI_USAGE = 0x00000020
	DXGI_USAGE_BACK_BUFFER          DXGI_USAGE = 0x00000040
	DXGI_USAGE_SHARED               DXGI_USAGE = 0x00000080
	DXGI_USAGE_READ_ONLY            DXGI_USAGE = 0x00000100
	DXGI_USAGE_DISCARD_ON_PRESENT   DXGI_USAGE = 0x00000200
	DXGI_USAGE_UNORDERED_ACCESS     DXGI_USAGE = 0x00000400
)

// DXGI_GPU_PREFERENCE specifies which GPU to prefer.
type DXGI_GPU_PREFERENCE uint32

// GPU preference constants.
const (
	DXGI_GPU_PREFERENCE_UNSPECIFIED      DXGI_GPU_PREFERENCE = 0
	DXGI_GPU_PREFERENCE_MINIMUM_POWER    DXGI_GPU_PREFERENCE = 1
	DXGI_GPU_PREFERENCE_HIGH_PERFORMANCE DXGI_GPU_PREFERENCE = 2
)

// DXGI_MODE_SCANLINE_ORDER specifies the order of scanlines.
type DXGI_MODE_SCANLINE_ORDER uint32

// Scanline order constants.
const (
	DXGI_MODE_SCANLINE_ORDER_UNSPECIFIED       DXGI_MODE_SCANLINE_ORDER = 0
	DXGI_MODE_SCANLINE_ORDER_PROGRESSIVE       DXGI_MODE_SCANLINE_ORDER = 1
	DXGI_MODE_SCANLINE_ORDER_UPPER_FIELD_FIRST DXGI_MODE_SCANLINE_ORDER = 2
	DXGI_MODE_SCANLINE_ORDER_LOWER_FIELD_FIRST DXGI_MODE_SCANLINE_ORDER = 3
)

// DXGI_MODE_SCALING specifies how images are stretched.
type DXGI_MODE_SCALING uint32

// Mode scaling constants.
const (
	DXGI_MODE_SCALING_UNSPECIFIED DXGI_MODE_SCALING = 0
	DXGI_MODE_SCALING_CENTERED    DXGI_MODE_SCALING = 1
	DXGI_MODE_SCALING_STRETCHED   DXGI_MODE_SCALING = 2
)

// DXGI_MODE_ROTATION specifies display rotation.
type DXGI_MODE_ROTATION uint32

// Mode rotation constants.
const (
	DXGI_MODE_ROTATION_UNSPECIFIED DXGI_MODE_ROTATION = 0
	DXGI_MODE_ROTATION_IDENTITY    DXGI_MODE_ROTATION = 1
	DXGI_MODE_ROTATION_ROTATE90    DXGI_MODE_ROTATION = 2
	DXGI_MODE_ROTATION_ROTATE180   DXGI_MODE_ROTATION = 3
	DXGI_MODE_ROTATION_ROTATE270   DXGI_MODE_ROTATION = 4
)

// DXGI_ADAPTER_FLAG specifies adapter flags.
type DXGI_ADAPTER_FLAG uint32

// Adapter flag constants.
const (
	DXGI_ADAPTER_FLAG_NONE     DXGI_ADAPTER_FLAG = 0
	DXGI_ADAPTER_FLAG_REMOTE   DXGI_ADAPTER_FLAG = 1
	DXGI_ADAPTER_FLAG_SOFTWARE DXGI_ADAPTER_FLAG = 2
)

// DXGI_SWAP_CHAIN_FLAG specifies swap chain options.
type DXGI_SWAP_CHAIN_FLAG uint32

// Swap chain flag constants.
const (
	DXGI_SWAP_CHAIN_FLAG_NONPREROTATED                          DXGI_SWAP_CHAIN_FLAG = 1
	DXGI_SWAP_CHAIN_FLAG_ALLOW_MODE_SWITCH                      DXGI_SWAP_CHAIN_FLAG = 2
	DXGI_SWAP_CHAIN_FLAG_GDI_COMPATIBLE                         DXGI_SWAP_CHAIN_FLAG = 4
	DXGI_SWAP_CHAIN_FLAG_RESTRICTED_CONTENT                     DXGI_SWAP_CHAIN_FLAG = 8
	DXGI_SWAP_CHAIN_FLAG_RESTRICT_SHARED_RESOURCE_DRIVER        DXGI_SWAP_CHAIN_FLAG = 16
	DXGI_SWAP_CHAIN_FLAG_DISPLAY_ONLY                           DXGI_SWAP_CHAIN_FLAG = 32
	DXGI_SWAP_CHAIN_FLAG_FRAME_LATENCY_WAITABLE_OBJECT          DXGI_SWAP_CHAIN_FLAG = 64
	DXGI_SWAP_CHAIN_FLAG_FOREGROUND_LAYER                       DXGI_SWAP_CHAIN_FLAG = 128
	DXGI_SWAP_CHAIN_FLAG_FULLSCREEN_VIDEO                       DXGI_SWAP_CHAIN_FLAG = 256
	DXGI_SWAP_CHAIN_FLAG_YUV_VIDEO                              DXGI_SWAP_CHAIN_FLAG = 512
	DXGI_SWAP_CHAIN_FLAG_HW_PROTECTED                           DXGI_SWAP_CHAIN_FLAG = 1024
	DXGI_SWAP_CHAIN_FLAG_ALLOW_TEARING                          DXGI_SWAP_CHAIN_FLAG = 2048
	DXGI_SWAP_CHAIN_FLAG_RESTRICTED_TO_ALL_HOLOGRAPHIC_DISPLAYS DXGI_SWAP_CHAIN_FLAG = 4096
)

// DXGI_PRESENT specifies present options.
type DXGI_PRESENT uint32

// Present flag constants.
const (
	DXGI_PRESENT_TEST                  DXGI_PRESENT = 0x00000001
	DXGI_PRESENT_DO_NOT_SEQUENCE       DXGI_PRESENT = 0x00000002
	DXGI_PRESENT_RESTART               DXGI_PRESENT = 0x00000004
	DXGI_PRESENT_DO_NOT_WAIT           DXGI_PRESENT = 0x00000008
	DXGI_PRESENT_STEREO_PREFER_RIGHT   DXGI_PRESENT = 0x00000010
	DXGI_PRESENT_STEREO_TEMPORARY_MONO DXGI_PRESENT = 0x00000020
	DXGI_PRESENT_RESTRICT_TO_OUTPUT    DXGI_PRESENT = 0x00000040
	DXGI_PRESENT_USE_DURATION          DXGI_PRESENT = 0x00000100
	DXGI_PRESENT_ALLOW_TEARING         DXGI_PRESENT = 0x00000200
)

// DXGI_MWA specifies window association flags.
type DXGI_MWA uint32

// Window association flag constants.
const (
	DXGI_MWA_NO_WINDOW_CHANGES DXGI_MWA = 1
	DXGI_MWA_NO_ALT_ENTER      DXGI_MWA = 2
	DXGI_MWA_NO_PRINT_SCREEN   DXGI_MWA = 4
	DXGI_MWA_VALID             DXGI_MWA = 7
)

// Factory creation flags for CreateDXGIFactory2.
const (
	DXGI_CREATE_FACTORY_DEBUG uint32 = 0x01
)

// DXGI_FEATURE specifies DXGI features.
type DXGI_FEATURE uint32

// Feature constants.
const (
	DXGI_FEATURE_PRESENT_ALLOW_TEARING DXGI_FEATURE = 0
)
