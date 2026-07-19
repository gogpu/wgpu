# Android Vulkan preview

Android/arm64 support is an unreleased implementation preview in
[gogpu/wgpu#268](https://github.com/gogpu/wgpu/pull/268), not a released support
claim. Its current scope is Vulkan, arm64, Android API 29 or newer, and both
`CGO_ENABLED=0` and `CGO_ENABLED=1`. GLES, software fallback, 32-bit Android,
debug callbacks, and API 28 or older are out of scope.

The default backend depends on the unreleased canonical goffi work in
[go-webgpu/goffi#62](https://github.com/go-webgpu/goffi/pull/62), exactly at
`8ccaae72d877a7af0af4b628bf86e92536e27d88`. The `rust` build-tag path also
needs the Android surface-source helper proposed in canonical
[`go-webgpu/webgpu`](https://github.com/go-webgpu/webgpu). Neither integration
uses a forked module path, a `replace` directive, or a committed `go.work`.
The proof script creates an ephemeral workspace and verifies the exact heads
and working-tree fingerprints supplied by the caller.

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

The independent prerequisites and their original exact heads are:

| Prerequisite | Exact head |
|--------------|------------|
| [wgpu#264](https://github.com/gogpu/wgpu/pull/264), drain before device teardown | `0ed17064f8c977f35d9b49b5cde0d0c69e867ecf` |
| [wgpu#265](https://github.com/gogpu/wgpu/pull/265), fail-closed swapchain negotiation | `a8ff52e340f0a06c8e1b6599a03856d7fc74d1a2` |
| [wgpu#266](https://github.com/gogpu/wgpu/pull/266), explicit mock construction | `e97e4901ee76d9e5f587569c073b38114221c4e6` |
| [wgpu#267](https://github.com/gogpu/wgpu/pull/267), surface-qualified present queue | `a3e839f94a12edce98e2496d96e5bd8d3cdd2fc3` |
| [wgpu#269](https://github.com/gogpu/wgpu/pull/269), surface lifetime ownership | `85eeeb02a278bf4caebf66310b6baad8c328dd87` |
| [goffi#62](https://github.com/go-webgpu/goffi/pull/62), Android/Bionic runtime | `8ccaae72d877a7af0af4b628bf86e92536e27d88` |
| `go-webgpu/webgpu` Android surface-source helper | base `351770c3f88ab91014abf9f8a512e58684018917`; separate helper PR required |

[wgpu#253](https://github.com/gogpu/wgpu/pull/253) is the design precedent for
matching Rust's typed public/HAL seam; it is not a code dependency of Android.
The default Vulkan path needs #268 and goffi#62. The `rust` build-tag path needs
the canonical `go-webgpu/webgpu` helper as well. No WGPU-local copy of that
helper should be merged.

## Reproducing deterministic proof

Use Android NDK r29 and checkouts of this branch, the exact clean goffi
candidate, and the canonical `go-webgpu/webgpu` helper candidate:

```bash
GOFFI_DIR=/path/to/goffi \
GOFFI_EXPECTED_HEAD=8ccaae72d877a7af0af4b628bf86e92536e27d88 \
GOFFI_EXPECTED_PATCH=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 \
WEBGPU_DIR=/path/to/go-webgpu-webgpu \
WEBGPU_EXPECTED_HEAD=351770c3f88ab91014abf9f8a512e58684018917 \
WEBGPU_EXPECTED_PATCH=2f4d4b42953d17e843684e99c7b7590214a1fcaa02bf19a80c8503d8b0e26cb2 \
ANDROID_NDK_HOME=/path/to/android-ndk-r29 \
GOTOOLCHAIN=go1.26.5 \
./scripts/check-android-arm64-preview.sh
```

The helper fingerprint above identifies the four-file helper plus its changelog
entry on that canonical base. Once its helper PR exists, replace the
base/fingerprint pair with the immutable PR head and the clean-tree fingerprint
(`e3b0c442...`).

The script verifies Android-only source selection; compiles every package and
test for both the default and `rust` implementations in cgo0 and cgo1 modes;
builds each headless example; checks the generated Go and NDK C ABI layouts;
and audits both ELF dependency sets. It requires Bionic `libc.so`/`libdl.so`,
confirms `libvulkan.so` for the default backend and `libwgpu_native.so` for the
Rust backend, and rejects glibc sonames, standalone `libpthread`, desktop WSI,
GLES, and software fallback. Current CI repeats the default lane with Go
1.25.12 and Go 1.26.5. Supplying `WEBGPU_DIR` enables the Rust lane locally;
that lane can become mandatory in CI as soon as the helper has a canonical
immutable ref.

These are deterministic cross-build and binary-shape checks. They do not prove
process startup, adapter enumeration, surface creation, rendering,
presentation, rotation, or lifecycle recovery on a physical device.

Before the preview can become a released support claim, goffi#62, the canonical
`go-webgpu/webgpu` helper, and the WGPU prerequisites must merge and ship
through canonical refs; #268 must rebase to Android-only commits; and API 29
plus API 30-or-newer arm64 devices must pass cgo0/cgo1 startup, known-color
presentation, rotation, Activity recreation, and repeated native-window
replacement without crashes, hangs, stale frames, or validation errors.
