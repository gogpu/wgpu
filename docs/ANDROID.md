# Android Vulkan preview

Android/arm64 support is an unreleased implementation preview in
[gogpu/wgpu#268](https://github.com/gogpu/wgpu/pull/268), not a released support
claim. Its current scope is Vulkan, arm64, Android API 29 or newer, and both
`CGO_ENABLED=0` and `CGO_ENABLED=1`. GLES, software fallback, 32-bit Android,
debug callbacks, and API 28 or older are out of scope.

The preview depends on the unreleased canonical goffi work in
[go-webgpu/goffi#62](https://github.com/go-webgpu/goffi/pull/62), exactly at
`8ccaae72d877a7af0af4b628bf86e92536e27d88`. Neither module uses a forked
module path, a `replace` directive, or a committed `go.work`. Draft CI checks
out that exact canonical commit and creates an ephemeral workspace only for
integration proof.

## Host contract

`Instance.CreateSurface` keeps the existing two-`uintptr` API on Android:

- `displayHandle` is ignored, matching Rust wgpu v29.
- `windowHandle` is the raw, non-null `ANativeWindow*` value.

The host owns the Activity/native-window lifecycle and retains its application
reference. A successfully created `VkSurfaceKHR` owns Vulkan's separate window
reference until `vkDestroySurfaceKHR`; WGPU therefore does not call
`ANativeWindow_acquire` or `ANativeWindow_release`.

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

## Exact draft stack

The draft is stacked only until its prerequisites merge. Their original exact
heads remain separately reviewable:

| Prerequisite | Exact head |
|--------------|------------|
| [wgpu#264](https://github.com/gogpu/wgpu/pull/264), drain before device teardown | `0ed17064f8c977f35d9b49b5cde0d0c69e867ecf` |
| [wgpu#265](https://github.com/gogpu/wgpu/pull/265), fail-closed swapchain negotiation | `a8ff52e340f0a06c8e1b6599a03856d7fc74d1a2` |
| [wgpu#266](https://github.com/gogpu/wgpu/pull/266), explicit mock construction | `e97e4901ee76d9e5f587569c073b38114221c4e6` |
| [wgpu#267](https://github.com/gogpu/wgpu/pull/267), surface-qualified present queue | `a3e839f94a12edce98e2496d96e5bd8d3cdd2fc3` |
| [wgpu#269](https://github.com/gogpu/wgpu/pull/269), surface lifetime ownership | `85eeeb02a278bf4caebf66310b6baad8c328dd87` |
| [goffi#62](https://github.com/go-webgpu/goffi/pull/62), Android/Bionic runtime | `8ccaae72d877a7af0af4b628bf86e92536e27d88` |

## Reproducing deterministic proof

Use Android NDK r29 and clean checkouts of this branch and the exact goffi
candidate:

```bash
GOFFI_DIR=/path/to/goffi \
GOFFI_EXPECTED_HEAD=8ccaae72d877a7af0af4b628bf86e92536e27d88 \
ANDROID_NDK_HOME=/path/to/android-ndk-r29 \
GOTOOLCHAIN=go1.26.5 \
./scripts/check-android-arm64-preview.sh
```

The script verifies Android-only source selection, compiles every package and
test in cgo0 and cgo1 modes, builds the headless example, checks the generated
Go and NDK C ABI layouts, and audits ELF dependencies. It requires Bionic
`libc.so`/`libdl.so`, confirms `libvulkan.so`, and rejects glibc sonames,
standalone `libpthread`, desktop WSI, GLES, and software fallback. CI repeats
the proof with Go 1.25.12 and Go 1.26.5.

These are deterministic cross-build and binary-shape checks. They do not prove
process startup, adapter enumeration, surface creation, rendering,
presentation, rotation, or lifecycle recovery on a physical device.

Before the preview can become merge-ready, goffi#62 and the WGPU prerequisites
must merge and ship through canonical refs, #268 must rebase to Android-only
commits, and API 29 plus API 30-or-newer arm64 devices must pass cgo0/cgo1
startup, known-color presentation, rotation, Activity recreation, and repeated
native-window replacement without crashes, hangs, stale frames, or validation
errors.
