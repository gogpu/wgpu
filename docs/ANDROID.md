# Android Vulkan preview

Android/arm64 support is an unreleased implementation preview in
[gogpu/wgpu#268](https://github.com/gogpu/wgpu/pull/268), not a released support
claim. Its current scope is Vulkan, arm64, Android API 29 or newer, and both
`CGO_ENABLED=0` and `CGO_ENABLED=1`. GLES, software fallback, 32-bit Android,
debug callbacks, and API 28 or older are out of scope.

The default backend consumes canonical
[goffi v0.6.1](https://github.com/go-webgpu/goffi/releases/tag/v0.6.1), released
from [go-webgpu/goffi#62](https://github.com/go-webgpu/goffi/pull/62). The
`rust` build-tag path also depends on canonical
[go-webgpu/webgpu#24](https://github.com/go-webgpu/webgpu/pull/24), exactly at
`08592c9f5916b64dfc70aba9e67a74a764bb3ef5`. goffi is declared directly in
`go.mod` and verified by `go.sum`; only the unreleased WebGPU candidate is
injected through an ephemeral workspace. There is no forked module path,
`replace` directive, or committed `go.work`.

API 29 is the native symbol and application install floor. API 36 is the
downstream Java compile/Play target and the newest runtime-observer endpoint;
it is not a distinct WGPU ABI. NDK r29 publishes native platform wrappers
through API 35, so the native cross-build deliberately targets the supported
API 29 floor instead of inventing an API 36 clang target. Policy tests cover
the API 29 branch and the shared API 30-through-36 branch.

## Host contract

New Android hosts should use the explicit raw-target path:

```go
surface, err := instance.CreateSurfaceUnsafe(
	wgpu.SurfaceTargetFromAndroidNativeWindow(nativeWindow),
)
```

`nativeWindow` is the raw, non-null `ANativeWindow*` value. Android has no
display handle for this operation. The existing
`Instance.CreateSurface(displayHandle, windowHandle)` method remains a
source-compatible adapter and ignores `displayHandle`, matching Rust `wgpu`
v29.

Hosts that represent native-window ownership with a Go object can instead
implement `SurfaceTarget` and call `CreateSurfaceFromTarget`. WGPU samples the
provider once and retains it until backend surface destruction completes.
See [Surface targets](SURFACE-TARGETS.md) for the complete safe/unsafe mapping.

The host owns the Activity/native-window lifecycle. For the unsafe path it must
keep its application reference valid until `Surface.Release` returns. A
successfully created `VkSurfaceKHR` owns Vulkan's separate window reference
until `vkDestroySurfaceKHR`; WGPU therefore does not call
`ANativeWindow_acquire` or `ANativeWindow_release` itself.

Surfaces are independent. Creating a second surface does not invalidate the
first. When Android replaces a native window, create a surface for the new raw
window and release the old surface after its configured device has drained.
There is deliberately no display-generation protocol: a counter cannot prove
pointer lifetime and would reject legitimate simultaneous surfaces.

## Rust wgpu v29 mapping

The semantic oracle is gfx-rs/wgpu v29.0.3 commit
`4cbe6232b2d7c289b6e1a38416a6ae1461a22e81`.

| Behavior | Rust v29 location | Go implementation and proof |
|----------|-------------------|-----------------------------|
| Safe provider is retained for the surface lifetime | `wgpu/src/api/{instance,surface}.rs` | `SurfaceTarget`, `CreateSurfaceFromTarget`, release-order test |
| Unsafe raw target retains no ownership source | `SurfaceTargetUnsafe::RawHandle`, `create_surface_unsafe` | opaque constructors and `CreateSurfaceUnsafe` validation tests |
| Target kind remains explicit below the public API | `raw-window-handle` enum matching | typed `hal.SurfaceTarget`; backend kind-routing tests |
| Create for every enabled backend; succeed if any succeeds | `wgpu-core/src/instance.rs::create_surface` | per-backend HAL surface map and matching adapter-qualification tests |
| Ignore Android display; use only raw `a_native_window` | `wgpu-hal/src/vulkan/instance.rs` | `api_android.go`, `android_surface_policy_test.go` |
| Load `libvulkan.so`; require Android WSI extension/commands | Vulkan instance loader | `vk/loader.go`, `vk/commands.go`, source-selection audit |
| API 29 uses infinite acquire timeout; API 30+ preserves one second | `swapchain/native.rs::acquire` | `swapchainPlatformPolicy.acquireTimeout` |
| Fence wait uses `waitAll=true` | `swapchain/native.rs::acquire` | checked acquire path in `swapchain.go` |
| Identity pre-transform; ignore orientation-only `SUBOPTIMAL` | `swapchain/native.rs::{create_swapchain,acquire,present}` | platform policy and host tests |
| Fixed `currentExtent` is authoritative; variable extent is clamped | `swapchain/native.rs::surface_capabilities` | `selectSwapchainExtent` tests |
| `Rgba16Float` pairs with extended-linear sRGB | `swapchain/native.rs::create_swapchain`, `conv.rs` | exact pair-selection test |
| Device drains configured swapchains before native teardown | native swapchain ownership | [wgpu#269](https://github.com/gogpu/wgpu/pull/269) lifecycle tests |

This change adds no Android-driven global Vulkan version check or adapter
filter. The canonical backend's existing application-version request remains
unchanged; actual extensions, commands, features, and device evidence decide
whether a Vulkan implementation works.

## Proposed review order

The Android Vulkan work remains in
[wgpu#268](https://github.com/gogpu/wgpu/pull/268). This typed-surface change is
best reviewed as a follow-up on that exact head, not folded into unrelated
prerequisites. After predecessors merge, #268 can be rebased to its Android-only
commits and this follow-up can be replayed without changing its public design.

The independently reviewable prerequisites and their current exact heads are:

| Prerequisite | Exact head |
|--------------|------------|
| [wgpu#264](https://github.com/gogpu/wgpu/pull/264), drain before device teardown | `6acfb1ea1cca1dc14d7ab0902931c678dc13e0a6` |
| [wgpu#265](https://github.com/gogpu/wgpu/pull/265), fail-closed swapchain negotiation | `eb48db0e7eec4c12eaa87cc998f6ea9e8f348fdd` |
| [wgpu#266](https://github.com/gogpu/wgpu/pull/266), explicit mock construction | `e65bec3c74c56cd07265c055ce6a7ec7fbedc680` |
| [wgpu#267](https://github.com/gogpu/wgpu/pull/267), surface-qualified present queue | `63efce782d5cf41dacab761d202da0e3d68ed2da` |
| [wgpu#269](https://github.com/gogpu/wgpu/pull/269), surface lifetime ownership | `2a069cb1fde942339813efd7199ed305a3a2ca84` |
| [webgpu#24](https://github.com/go-webgpu/webgpu/pull/24), Android Rust-wrapper surface source | `08592c9f5916b64dfc70aba9e67a74a764bb3ef5` |

The current stacked [wgpu#268](https://github.com/gogpu/wgpu/pull/268) head is
`13b59db954dafd2653e6cb5ce3ea14c022782ad2`. Once the five WGPU prerequisites
merge, #268 drops their replayed commits and retains its four Android-owned
commits. This typed-target follow-up is then replayed on that reduced head.

[wgpu#253](https://github.com/gogpu/wgpu/pull/253) is the design precedent for
matching Rust's typed public/HAL seam; it is not a code dependency of Android.
The default Vulkan path needs #268 and goffi v0.6.1. The `rust` build-tag path
needs the canonical `go-webgpu/webgpu` helper as well. No WGPU-local copy of
that helper should be merged.

## Reproducing deterministic proof

Use Android NDK r29 and clean checkouts of this branch and the canonical
`go-webgpu/webgpu` helper candidate:

```bash
WEBGPU_DIR=/path/to/go-webgpu-webgpu \
WEBGPU_EXPECTED_HEAD=08592c9f5916b64dfc70aba9e67a74a764bb3ef5 \
WEBGPU_EXPECTED_PATCH=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 \
ANDROID_NDK_HOME=/path/to/android-ndk-r29 \
GOTOOLCHAIN=go1.26.5 \
./scripts/check-android-arm64-preview.sh
```

The script first asserts that the module graph selects canonical goffi v0.6.1.
It then verifies Android-only source selection; compiles every package and test
for both the default and `rust` implementations in cgo0 and cgo1 modes; builds
each headless example; checks the generated Go and NDK C ABI layouts; and
audits both ELF dependency sets. It requires Bionic `libc.so`/`libdl.so`,
confirms `libvulkan.so` for the default backend and `libwgpu_native.so` for the
Rust backend, and rejects glibc sonames, standalone `libpthread`, desktop WSI,
GLES, and software fallback. Current CI repeats both lanes with Go 1.25.12 and
Go 1.26.5 against canonical goffi v0.6.1 and the exact clean webgpu#24 head.

These are deterministic cross-build and binary-shape checks. They do not prove
process startup, adapter enumeration, surface creation, rendering,
presentation, rotation, or lifecycle recovery on a physical device.

Before the preview can become a released support claim, the canonical
`go-webgpu/webgpu` helper and the WGPU prerequisites must merge and ship through
canonical refs; #268 must rebase to Android-only commits; and API 29 plus API
36 arm64 devices must pass startup, known-color presentation, rotation,
Activity recreation, and repeated native-window replacement without crashes,
hangs, stale frames, or validation errors.
