//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// ffiPointerDepths is how many stack depths TestMappableBufferCreationAtEveryStackDepth sweeps.
// Each step adds one frame with 32 bytes of padding, so the sweep moves the end of a fresh goroutine's stack
// across the whole call path in small steps.
//
// Where the stack ends also depends on the size of a new goroutine's first stack, which the runtime
// adapts per process, so this test is probabilistic: on the code before the fix it failed in 5 of 8
// runs (Intel Iris Xe). TestFFIPointersArePinned in the root package is the deterministic guard of
// the pattern; this test proves the runtime symptom is gone.
const ffiPointerDepths = 1024

// withStackDepth calls f below depth extra frames with 32 bytes of padding each.
//
//go:noinline
func withStackDepth(depth int, f func()) {
	var pad [32]byte
	pad[depth%len(pad)] = byte(depth)
	if depth > 0 {
		withStackDepth(depth-1, f)
	} else {
		f()
	}
	if pad[depth%len(pad)] != byte(depth) {
		panic("unreachable: keeps pad on the stack")
	}
}

// TestMappableBufferCreationAtEveryStackDepth is the regression test for P1-VK-012. The Vulkan
// backend passed the address of a stack variable to vkMapMemory as a uintptr. When the goroutine
// stack was copied during the call, the driver wrote the mapped pointer into the old stack: the
// map came back as NULL with VK_SUCCESS, and the next map of the same memory failed validation
// with VUID-vkMapMemory-memory-00678.
//
// Each buffer is above the Vulkan allocator's dedicated threshold (32 MiB), so every creation maps
// a fresh VkDeviceMemory. Each creation runs on a new goroutine below a different number of padding
// frames, so for some depths the stack has to grow inside the FFI call itself. The test runs on
// Vulkan only: the dedicated-allocation map path is the Vulkan allocator's, and the other backends
// would only allocate 1024 large buffers (on the software backend in the Go heap).
func TestMappableBufferCreationAtEveryStackDepth(t *testing.T) {
	vulkan := overrideBackends[0]
	if vulkan.name != backendVulkan {
		t.Fatalf("overrideBackends[0] = %q, want vulkan", vulkan.name)
	}
	t.Run(string(vulkan.name), func(t *testing.T) {
		device, _ := openBackendDevice(t, vulkan, 0, gputypes.PowerPreferenceLowPower)
		const size = 33 << 20
		for depth := 0; depth < ffiPointerDepths; depth++ {
			var (
				wg  sync.WaitGroup
				buf *wgpu.Buffer
				err error
			)
			wg.Add(1)
			go func() {
				defer wg.Done()
				withStackDepth(depth, func() {
					buf, err = device.CreateBuffer(&wgpu.BufferDescriptor{
						Label: "ffi-pointer",
						Size:  size,
						Usage: gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc,
					})
				})
			}()
			wg.Wait()
			if err != nil {
				t.Fatalf("depth %d: CreateBuffer: %v", depth, err)
			}
			buf.Release()
		}
	})
}
