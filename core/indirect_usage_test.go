//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// usageCaptureDevice records the usage of every HAL buffer it creates.
type usageCaptureDevice struct {
	mockHALDevice
	usages []gputypes.BufferUsage
}

func (d *usageCaptureDevice) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	d.usages = append(d.usages, desc.Usage)
	return mockBuffer{}, nil
}

// TestCreateBufferIndirectAddsInternalStorageUsage checks that an INDIRECT
// buffer is created in the HAL with storage usage, because the indirect
// validation shader binds it as a read-only storage buffer, and that the
// buffer still reports the usage the user asked for. Matches Rust wgpu-core
// device/resource.rs create_buffer.
func TestCreateBufferIndirectAddsInternalStorageUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		usage   gputypes.BufferUsage
		halWant gputypes.BufferUsage
	}{
		{"indirect", gputypes.BufferUsageIndirect | gputypes.BufferUsageCopyDst,
			gputypes.BufferUsageIndirect | gputypes.BufferUsageCopyDst | gputypes.BufferUsageStorage},
		{"indirect-storage", gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage,
			gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage},
		{"vertex", gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
			gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			halDevice := &usageCaptureDevice{}
			device := NewDevice(halDevice, &Adapter{}, 0, gputypes.DefaultLimits(), "usage")
			halDevice.usages = nil // drop internal buffers created by NewDevice
			buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{Label: tc.name, Size: 64, Usage: tc.usage})
			if err != nil {
				t.Fatalf("CreateBuffer: %v", err)
			}
			if len(halDevice.usages) != 1 {
				t.Fatalf("HAL buffers created: %d, want 1", len(halDevice.usages))
			}
			if got := halDevice.usages[0]; got != tc.halWant {
				t.Errorf("HAL usage = %#x, want %#x", uint32(got), uint32(tc.halWant))
			}
			if got := buffer.Usage(); got != tc.usage {
				t.Errorf("Buffer.Usage() = %#x, want the user's %#x", uint32(got), uint32(tc.usage))
			}
		})
	}
}
