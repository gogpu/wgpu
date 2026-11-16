//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// The dynamic offset validation moved here with the binder (S5's Move), its
// only production caller. The test's fixture group carries the dynamic
// bindings the root test's group was built with: binding 1, a 1024-byte
// buffer bound at [256, 288) as a dynamic uniform; binding 3, a 512-byte
// buffer bound at [0, 64) as dynamic read-only storage. The validation reads
// only the dynamic bindings, so the group needs no HAL objects.

func dynamicOffsetsTestGroup() *resource.BindGroup {
	return resource.NewBindGroup(&resource.BindGroupInit{DynamicBindings: []core.DynamicBinding{
		{Binding: 1, BindingType: gputypes.BufferBindingTypeUniform, BufferSize: 1024, BindingEnd: 288, MaxDynamicOffset: 736},
		{Binding: 3, BindingType: gputypes.BufferBindingTypeReadOnlyStorage, BufferSize: 512, BindingEnd: 64, MaxDynamicOffset: 448},
	}})
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

	plain := resource.NewBindGroup(&resource.BindGroupInit{})
	if err := validateDynamicOffsets(0, plain, []uint32{256}, &limits); !errors.Is(err, errDynamicOffsetCount) {
		t.Fatalf("offsets for a group without dynamic bindings: error = %v, want %v", err, errDynamicOffsetCount)
	}
}
