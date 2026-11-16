//go:build !rust && !(js && wasm)

package queue

import (
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// TestCopyUsageTransitionDrivesBarriers is the queue-internal half of the root
// package's TestCopyCommandTextureScopeDrivesBarriers, ported with the queue
// (ADR-070 S9): a copy's usage in a command buffer's scope must merge into the
// device tracker as one transition and inject a barrier command buffer. The
// root test keeps the scope-shape half (the copy commands populate the scope).
func TestCopyUsageTransitionDrivesBarriers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		before    track.TextureUses
		copyUsage track.TextureUses
	}{
		{
			name:      "sampled to copy source",
			before:    track.TextureUsesResource,
			copyUsage: track.TextureUsesCopySrc,
		},
		{
			name:      "render target to copy destination",
			before:    track.TextureUsesColorTarget,
			copyUsage: track.TextureUsesCopyDst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, device := newTestHost(t, true)
			q := newTestQueue(host)

			tex := newTestTrackedTexture(t, device, host, tt.name)
			idx := tex.CoreTexture().TrackingData().Index()
			device.Tracker().InsertTexture(idx, tt.before)

			coreEnc, err := device.CreateCommandEncoder("test")
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			coreCB, err := coreEnc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			coreCB.TextureScope().AppendSingle(nil, idx, track.TextureSelector{}, tt.copyUsage)

			cb := encoder.NewTestCommandBuffer(coreCB, host)
			cb.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)

			probeTracker := core.NewDeviceTracker()
			probeTracker.InsertTexture(idx, tt.before)
			transitions := mergeTextureScopes(probeTracker, []*encoder.CommandBuffer{cb})
			if len(transitions) != 1 {
				t.Fatalf("copy scope produced %d transitions, want 1", len(transitions))
			}
			transition := transitions[0]
			if transition.Index != idx || transition.Usage.From != tt.before || transition.Usage.To != tt.copyUsage {
				t.Fatalf("transition = %+v, want index %d %v -> %v", transition, idx, tt.before, tt.copyUsage)
			}

			barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
			if err != nil {
				t.Fatalf("injectBarriers: %v", err)
			}
			if barrierCB == nil {
				t.Fatal("expected copy usage transition to inject a barrier command buffer")
			}
			if got := device.Tracker().TextureUsage(idx); got != tt.copyUsage {
				t.Errorf("tracked usage after injection = %v, want %v", got, tt.copyUsage)
			}
		})
	}
}
