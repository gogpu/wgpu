//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// dynamicOffsetsTestGroup returns a bind group with, listed out of binding
// order: binding 3, a 512-byte buffer bound at [0, 64) as dynamic read-only
// storage; binding 1, a 1024-byte buffer bound at [256, 288) as a dynamic
// uniform; and binding 0, a non-dynamic uniform.
func dynamicOffsetsTestGroup() *BindGroup {
	layout := []gputypes.BindGroupLayoutEntry{
		{Binding: 3, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage, HasDynamicOffset: true}},
		{Binding: 0, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		{Binding: 1, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, HasDynamicOffset: true}},
	}
	entries := []BindGroupEntry{
		{Binding: 0, Buffer: NewTestBuffer(64, gputypes.BufferUsageUniform, "plain")},
		{Binding: 1, Buffer: NewTestBuffer(1024, gputypes.BufferUsageUniform, "uniform"), Offset: 256, Size: 32},
		{Binding: 3, Buffer: NewTestBuffer(512, gputypes.BufferUsageStorage, "storage"), Size: 64},
	}
	return &BindGroup{dynamicBindings: buildDynamicBindings(layout, entries)}
}

func TestBuildDynamicBindings(t *testing.T) {
	got := dynamicOffsetsTestGroup().dynamicBindings
	want := []dynamicBinding{
		{binding: 1, bindingType: gputypes.BufferBindingTypeUniform, bufferSize: 1024, bindingEnd: 288, maxDynamicOffset: 736},
		{binding: 3, bindingType: gputypes.BufferBindingTypeReadOnlyStorage, bufferSize: 512, bindingEnd: 64, maxDynamicOffset: 448},
	}
	if len(got) != len(want) {
		t.Fatalf("dynamic bindings = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dynamic binding %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestValidateDynamicOffsets covers Rust wgpu-core's
// BindGroup::validate_dynamic_bindings: the count, the alignment by binding
// type, and the range. The limits give uniform and storage different
// alignments, so an offset aligned for one type only tells them apart.
func TestValidateDynamicOffsets(t *testing.T) {
	limits := gputypes.DefaultLimits()
	limits.MinUniformBufferOffsetAlignment = 256
	limits.MinStorageBufferOffsetAlignment = 64
	group := dynamicOffsetsTestGroup()

	tests := []struct {
		name    string
		offsets []uint32
		want    error
	}{
		{name: "valid", offsets: []uint32{256, 64}},
		{name: "valid at the maximum", offsets: []uint32{512, 448}},
		{name: "zero offsets", offsets: []uint32{0, 0}},
		{name: "missing offsets", offsets: nil, want: errDynamicOffsetCount},
		{name: "too few", offsets: []uint32{0}, want: errDynamicOffsetCount},
		{name: "too many", offsets: []uint32{0, 0, 0}, want: errDynamicOffsetCount},
		{name: "uniform needs its own alignment", offsets: []uint32{64, 0}, want: errDynamicOffsetUnaligned},
		{name: "storage alignment is not 256", offsets: []uint32{0, 192}, want: nil},
		{name: "storage unaligned", offsets: []uint32{0, 32}, want: errDynamicOffsetUnaligned},
		{name: "uniform past the buffer", offsets: []uint32{768, 0}, want: errDynamicOffsetOutOfBounds},
		{name: "storage past the buffer", offsets: []uint32{0, 512}, want: errDynamicOffsetOutOfBounds},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDynamicOffsets(2, group, tt.offsets, &limits)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}

	plain := &BindGroup{}
	if err := validateDynamicOffsets(0, plain, []uint32{256}, &limits); !errors.Is(err, errDynamicOffsetCount) {
		t.Fatalf("offsets for a group without dynamic bindings: error = %v, want %v", err, errDynamicOffsetCount)
	}
}
