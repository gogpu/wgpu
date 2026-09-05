//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"testing"
)

func TestRenderPassTimestampDescriptor(t *testing.T) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "timestamps")
	raw := mockQuerySet{}
	active := NewQuerySet(raw, device, hal.QueryTypeTimestamp, 4, "active")
	defer active.Destroy()
	destroyed := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeTimestamp, 4, "destroyed")
	destroyed.Destroy()
	begin, end := uint32(1), uint32(3)
	tests := []struct {
		name   string
		writes *RenderPassTimestampWrites
		want   bool
	}{
		{name: "absent"},
		{name: "nil query set", writes: &RenderPassTimestampWrites{}},
		{name: "destroyed query set", writes: &RenderPassTimestampWrites{QuerySet: destroyed}},
		{name: "both boundaries", writes: &RenderPassTimestampWrites{QuerySet: active, BeginningOfPassWriteIndex: &begin, EndOfPassWriteIndex: &end}, want: true},
		{name: "end only", writes: &RenderPassTimestampWrites{QuerySet: active, EndOfPassWriteIndex: &end}, want: true},
	}
	encoder := &CoreCommandEncoder{device: device}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encoder.convertRenderPassDescriptor(&RenderPassDescriptor{Label: "pass", TimestampWrites: tt.writes})
			if got.Label != "pass" {
				t.Fatalf("Label = %q", got.Label)
			}
			if !tt.want {
				if got.TimestampWrites != nil {
					t.Fatalf("TimestampWrites = %+v, want nil", got.TimestampWrites)
				}
				return
			}
			writes := got.TimestampWrites
			if writes == nil || writes.QuerySet != raw || writes.BeginningOfPassWriteIndex != tt.writes.BeginningOfPassWriteIndex || writes.EndOfPassWriteIndex != tt.writes.EndOfPassWriteIndex {
				t.Fatalf("TimestampWrites = %+v, want %+v", writes, tt.writes)
			}
		})
	}
}
