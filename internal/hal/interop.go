//go:build !(js && wasm)

package hal

import publichal "github.com/gogpu/wgpu/hal"

// NativeHandlesProvider is implemented by the HAL objects that can name the
// native objects behind them in the public hal package's terms.
//
// Every backend implements it on its Instance, Adapter, Device and Surface. An
// implementation returns a snapshot: the Kind of its backend and every handle
// it knows, as field copies only. It must not allocate, lock, block or call
// the GPU: the caller already holds the guards that keep the object alive.
type NativeHandlesProvider interface {
	NativeHandles() publichal.Handles
}

// BackendID is the internal identity of the backend that owns a HAL object.
//
// It is the key of the conversion table below and exists because no value the
// backends already publish can serve as one: gputypes.Backend (Backend.Variant)
// is lossy in both directions — the software and no-op backends both report
// BackendEmpty, and a "rust"-tagged build reports the API below wgpu-native —
// so a conversion keyed on it would relabel backends silently.
type BackendID uint8

const (
	// BackendIDUnknown is no identity: an object that cannot name its backend.
	BackendIDUnknown BackendID = 0

	// BackendIDVulkan is the Vulkan backend.
	BackendIDVulkan BackendID = 1

	// BackendIDMetal is the Metal backend.
	BackendIDMetal BackendID = 2

	// BackendIDDX12 is the Direct3D 12 backend.
	BackendIDDX12 BackendID = 3

	// BackendIDGLES is the OpenGL/OpenGL ES backend.
	BackendIDGLES BackendID = 4

	// BackendIDBrowser is the browser's WebGPU backend (js/wasm).
	BackendIDBrowser BackendID = 5

	// BackendIDSoftware is the CPU rasterizer backend.
	BackendIDSoftware BackendID = 6

	// BackendIDNoop is the no-op test backend.
	BackendIDNoop BackendID = 7

	// BackendIDWebGPU is the wgpu-native binding backend (rust tag).
	BackendIDWebGPU BackendID = 8
)

// backendKinds is the one place where the internal backend identity becomes
// the public hal.BackendKind. It is a table keyed on BackendID, never a
// numeric cast: the public values are a frozen ABI and the internal identities
// may be reordered without moving them.
var backendKinds = [...]publichal.BackendKind{
	BackendIDUnknown:  publichal.KindUnknown,
	BackendIDVulkan:   publichal.KindVulkan,
	BackendIDMetal:    publichal.KindMetal,
	BackendIDDX12:     publichal.KindDX12,
	BackendIDGLES:     publichal.KindGLES,
	BackendIDBrowser:  publichal.KindBrowser,
	BackendIDSoftware: publichal.KindSoftware,
	BackendIDNoop:     publichal.KindNoop,
	BackendIDWebGPU:   publichal.KindWebGPU,
}

// PublicKind returns the public hal.BackendKind of an internal backend
// identity. An identity outside the table yields KindUnknown.
func PublicKind(id BackendID) publichal.BackendKind {
	if int(id) >= len(backendKinds) {
		return publichal.KindUnknown
	}
	return backendKinds[id]
}
