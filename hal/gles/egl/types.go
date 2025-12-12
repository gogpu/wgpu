// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package egl

// EGL types based on EGL 1.4/1.5 specification.
type (
	// EGLBoolean represents a boolean value (EGL_TRUE or EGL_FALSE).
	EGLBoolean uint32
	// EGLInt represents a 32-bit signed integer.
	EGLInt int32
	// EGLEnum represents an enumeration value.
	EGLEnum uint32
	// EGLAttrib represents an attribute value (EGL 1.5+).
	EGLAttrib uintptr
	// EGLDisplay represents an EGL display connection.
	EGLDisplay uintptr
	// EGLConfig represents an EGL frame buffer configuration.
	EGLConfig uintptr
	// EGLSurface represents an EGL rendering surface.
	EGLSurface uintptr
	// EGLContext represents an EGL rendering context.
	EGLContext uintptr
	// EGLNativeDisplayType represents a native platform display.
	EGLNativeDisplayType uintptr
	// EGLNativeWindowType represents a native platform window.
	EGLNativeWindowType uintptr
	// EGLNativePixmapType represents a native platform pixmap.
	EGLNativePixmapType uintptr
)

// EGL constants - Boolean values
const (
	False EGLBoolean = 0
	True  EGLBoolean = 1
)

// EGL constants - Special values
const (
	DefaultDisplay  EGLNativeDisplayType = 0
	NoContext       EGLContext           = 0
	NoDisplay       EGLDisplay           = 0
	NoSurface       EGLSurface           = 0
	DontCare        EGLInt               = -1
	Unknown         EGLInt               = -1
	NoNativeDisplay EGLNativeDisplayType = 0
	NoNativeWindow  EGLNativeWindowType  = 0
)

// EGL constants - Errors (returned by eglGetError)
const (
	Success           EGLInt = 0x3000
	NotInitialized    EGLInt = 0x3001
	BadAccess         EGLInt = 0x3002
	BadAlloc          EGLInt = 0x3003
	BadAttribute      EGLInt = 0x3004
	BadConfig         EGLInt = 0x3005
	BadContext        EGLInt = 0x3006
	BadCurrentSurface EGLInt = 0x3007
	BadDisplay        EGLInt = 0x3008
	BadMatch          EGLInt = 0x3009
	BadNativePixmap   EGLInt = 0x300A
	BadNativeWindow   EGLInt = 0x300B
	BadParameter      EGLInt = 0x300C
	BadSurface        EGLInt = 0x300D
	ContextLost       EGLInt = 0x300E
)

// EGL constants - Config attributes
const (
	BufferSize            EGLInt = 0x3020
	AlphaSize             EGLInt = 0x3021
	BlueSize              EGLInt = 0x3022
	GreenSize             EGLInt = 0x3023
	RedSize               EGLInt = 0x3024
	DepthSize             EGLInt = 0x3025
	StencilSize           EGLInt = 0x3026
	ConfigCaveat          EGLInt = 0x3027
	ConfigID              EGLInt = 0x3028
	Level                 EGLInt = 0x3029
	MaxPbufferHeight      EGLInt = 0x302A
	MaxPbufferPixels      EGLInt = 0x302B
	MaxPbufferWidth       EGLInt = 0x302C
	NativeRenderable      EGLInt = 0x302D
	NativeVisualID        EGLInt = 0x302E
	NativeVisualType      EGLInt = 0x302F
	Samples               EGLInt = 0x3031
	SampleBuffers         EGLInt = 0x3032
	SurfaceType           EGLInt = 0x3033
	TransparentType       EGLInt = 0x3034
	TransparentBlueValue  EGLInt = 0x3035
	TransparentGreenValue EGLInt = 0x3036
	TransparentRedValue   EGLInt = 0x3037
	None                  EGLInt = 0x3038
	BindToTextureRGB      EGLInt = 0x3039
	BindToTextureRGBA     EGLInt = 0x303A
	MinSwapInterval       EGLInt = 0x303B
	MaxSwapInterval       EGLInt = 0x303C
	LuminanceSize         EGLInt = 0x303D
	AlphaMaskSize         EGLInt = 0x303E
	ColorBufferType       EGLInt = 0x303F
	RenderableType        EGLInt = 0x3040
	Conformant            EGLInt = 0x3042
)

// EGL constants - Surface attributes
const (
	Height               EGLInt = 0x3056
	Width                EGLInt = 0x3057
	LargestPbuffer       EGLInt = 0x3058
	TextureFormat        EGLInt = 0x3080
	TextureTarget        EGLInt = 0x3081
	MipmapTexture        EGLInt = 0x3082
	MipmapLevel          EGLInt = 0x3083
	RenderBuffer         EGLInt = 0x3086
	VGColorspace         EGLInt = 0x3087
	VGAlphaFormat        EGLInt = 0x3088
	HorizontalResolution EGLInt = 0x3090
	VerticalResolution   EGLInt = 0x3091
	PixelAspectRatio     EGLInt = 0x3092
	SwapBehavior         EGLInt = 0x3093
	MultisampleResolve   EGLInt = 0x3099
)

// EGL constants - Context attributes
const (
	ContextMajorVersion                  EGLInt = 0x3098
	ContextMinorVersion                  EGLInt = 0x30FB
	ContextOpenGLProfileMask             EGLInt = 0x30FD
	ContextOpenGLResetNotification       EGLInt = 0x31BD
	ContextOpenGLCoreProfileBit          EGLInt = 0x00000001
	ContextOpenGLCompatibilityProfileBit EGLInt = 0x00000002
	ContextOpenGLDebug                   EGLInt = 0x31B0
	ContextOpenGLForwardCompatible       EGLInt = 0x31B1
	ContextOpenGLRobustAccess            EGLInt = 0x31B2
	NoResetNotification                  EGLInt = 0x31BE
	LoseContextOnReset                   EGLInt = 0x31BF
)

// EGL constants - Renderable type mask bits
const (
	OpenGLESBit  EGLInt = 0x0001
	OpenVGBit    EGLInt = 0x0002
	OpenGLES2Bit EGLInt = 0x0004
	OpenGLBit    EGLInt = 0x0008
	OpenGLES3Bit EGLInt = 0x0040
)

// EGL constants - Surface type mask bits
const (
	PbufferBit               EGLInt = 0x0001
	PixmapBit                EGLInt = 0x0002
	WindowBit                EGLInt = 0x0004
	VGColorspaceLinearBit    EGLInt = 0x0020
	VGAlphaFormatPreBit      EGLInt = 0x0040
	MultisampleResolveBoxBit EGLInt = 0x0200
	SwapBehaviorPreservedBit EGLInt = 0x0400
)

// EGL constants - API identifiers
const (
	OpenGLESAPI EGLEnum = 0x30A0
	OpenVGAPI   EGLEnum = 0x30A1
	OpenGLAPI   EGLEnum = 0x30A2
)

// EGL constants - QueryString targets
const (
	Vendor     EGLInt = 0x3053
	Version    EGLInt = 0x3054
	Extensions EGLInt = 0x3055
	ClientAPIs EGLInt = 0x308D
)

// EGL constants - Color buffer type
const (
	RGBBuffer       EGLInt = 0x308E
	LuminanceBuffer EGLInt = 0x308F
)

// EGL constants - Platform types (EGL 1.5 and extensions)
const (
	PlatformX11KHR          EGLEnum = 0x31D5
	PlatformWaylandKHR      EGLEnum = 0x31D8
	PlatformSurfacelessMesa EGLEnum = 0x31DD
	PlatformAngleAngle      EGLEnum = 0x3202
)

// EGL constants - Extension-specific
const (
	ContextFlagsKHR                      EGLInt = 0x30FC
	ContextOpenGLDebugBitKHR             EGLInt = 0x0001
	ContextOpenGLRobustAccessExt         EGLInt = 0x30BF
	PlatformAngleNativePlatformTypeAngle EGLInt = 0x348F
	PlatformAngleDebugLayersEnabled      EGLInt = 0x3451
	GLColorspaceKHR                      EGLInt = 0x309D
	GLColorspaceSRGBKHR                  EGLInt = 0x3089
)

// EGL constants - Swap behavior
const (
	BufferPreserved EGLInt = 0x3094
	BufferDestroyed EGLInt = 0x3095
)

// EGL constants - Back buffer
const (
	SingleBuffer EGLInt = 0x3085
	BackBuffer   EGLInt = 0x3084
)

// EGL constants - QueryContext targets
const (
	ContextClientType    EGLInt = 0x3097
	ContextClientVersion EGLInt = 0x3098
)

// WindowKind represents the type of window system.
type WindowKind int

const (
	// WindowKindX11 represents X11 window system.
	WindowKindX11 WindowKind = iota
	// WindowKindWayland represents Wayland window system.
	WindowKindWayland
	// WindowKindSurfaceless represents surfaceless (headless) rendering.
	WindowKindSurfaceless
	// WindowKindUnknown represents unknown window system.
	WindowKindUnknown
)

// String returns the string representation of WindowKind.
func (w WindowKind) String() string {
	switch w {
	case WindowKindX11:
		return "X11"
	case WindowKindWayland:
		return "Wayland"
	case WindowKindSurfaceless:
		return "Surfaceless"
	default:
		return "Unknown"
	}
}
