//go:build !(js && wasm)

package core

import (
	"encoding/binary"
	"testing"
	"unsafe"

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

// ivTestBindGroup is a distinct bind group per creation, carrying the
// binding of entry 0 (buffer binding offset) and of entry 1 if any.
type ivTestBindGroup struct {
	id      int
	offsets []uint64
}

func (*ivTestBindGroup) Destroy() {}

// ivTestDevice is a mock HAL device that maps buffers to host memory and
// records bind group creation and destruction, enough to build
// IndirectValidation without a GPU.
type ivTestDevice struct {
	mockHALDevice
	memory    map[uint64][]byte // by buffer size; the test creates one buffer per size
	created   []*ivTestBindGroup
	destroyed []hal.BindGroup
}

func (d *ivTestDevice) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if d.memory == nil {
		d.memory = make(map[uint64][]byte)
	}
	d.memory[desc.Size] = make([]byte, desc.Size)
	return mockBuffer{}, nil
}

func (d *ivTestDevice) MapBuffer(_ hal.Buffer, _, size uint64) (hal.BufferMapping, error) {
	mem := d.memory[size]
	return hal.BufferMapping{Ptr: unsafe.Pointer(&mem[0]), IsCoherent: true}, nil
}

func (d *ivTestDevice) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	g := &ivTestBindGroup{id: len(d.created)}
	for _, e := range desc.Entries {
		if b, ok := e.Resource.(gputypes.BufferBinding); ok {
			g.offsets = append(g.offsets, b.Offset)
		}
	}
	d.created = append(d.created, g)
	return g, nil
}

func (d *ivTestDevice) DestroyBindGroup(g hal.BindGroup) { d.destroyed = append(d.destroyed, g) }

// TestIndirectDispatchParamsSelectPerDispatchInputs checks that every
// DispatchIndirect offset gets bind groups that select its own arguments —
// the source bind group at the aligned offset, cached per buffer, and the
// destination bind group whose index table entry holds the remainder — and
// that the source bind groups are destroyed with the buffer, not before.
func TestIndirectDispatchParamsSelectPerDispatchInputs(t *testing.T) {
	halDevice := &ivTestDevice{}
	limits := gputypes.DefaultLimits()
	limits.MinStorageBufferOffsetAlignment = 256
	limits.MinUniformBufferOffsetAlignment = 64
	device := NewDevice(halDevice, &Adapter{}, 0, limits, "iv")
	iv := device.IndirectValidation()
	if iv == nil {
		t.Fatal("IndirectValidation not created")
	}
	// The index table: entry i at i*64 holds i, for i < 256/4.
	table := halDevice.memory[64*64]
	for _, i := range []uint32{0, 3, 63} {
		if got := binary.LittleEndian.Uint32(table[i*64:]); got != i {
			t.Fatalf("index table entry %d = %d", i, got)
		}
	}

	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{Label: "args", Size: 1024, Usage: gputypes.BufferUsageIndirect})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	before := len(halDevice.created)
	for _, tc := range []struct {
		offset        uint64
		srcOffset     uint64
		indexEntryOff uint64
	}{
		{0, 0, 0},
		{12, 0, 3 * 64},
		{268, 256, 3 * 64},
		{1012, 768, 61 * 64},
		{256, 256, 0},
	} {
		p, err := iv.DispatchParams(buffer, tc.offset)
		if err != nil {
			t.Fatalf("DispatchParams(%d): %v", tc.offset, err)
		}
		src := p.SrcBindGroup.(*ivTestBindGroup)
		dst := p.DstBindGroup.(*ivTestBindGroup)
		if src.offsets[0] != tc.srcOffset {
			t.Errorf("offset %d: source binding offset %d, want %d", tc.offset, src.offsets[0], tc.srcOffset)
		}
		if dst.offsets[1] != tc.indexEntryOff {
			t.Errorf("offset %d: index table entry at %d, want %d", tc.offset, dst.offsets[1], tc.indexEntryOff)
		}
	}
	if got := len(halDevice.created) - before; got != 3 {
		t.Errorf("source bind groups created: %d, want 3 (aligned offsets 0, 256, 768)", got)
	}
	if len(halDevice.destroyed) != 0 {
		t.Fatalf("bind groups destroyed while the buffer lives: %d", len(halDevice.destroyed))
	}
	buffer.Destroy()
	if got := len(halDevice.destroyed); got != 3 {
		t.Errorf("bind groups destroyed with the buffer: %d, want 3", got)
	}

	plain, err := device.CreateBuffer(&gputypes.BufferDescriptor{Label: "storage", Size: 64, Usage: gputypes.BufferUsageStorage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	if _, err := iv.DispatchParams(plain, 0); err == nil {
		t.Error("DispatchParams on a buffer without INDIRECT usage: no error")
	}
}
