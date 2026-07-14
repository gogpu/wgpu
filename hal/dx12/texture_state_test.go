//go:build windows && !(js && wasm)

package dx12

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

func TestTransitionTextureIfNeededEmitsCopySourceBarrier(t *testing.T) {
	texture := &Texture{raw: &d3d12.ID3D12Resource{}, currentState: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET}
	var got d3d12.D3D12_RESOURCE_BARRIER
	old := transitionTextureResourceBarrier
	transitionTextureResourceBarrier = func(_ *d3d12.ID3D12GraphicsCommandList, barrier *d3d12.D3D12_RESOURCE_BARRIER) { got = *barrier }
	defer func() { transitionTextureResourceBarrier = old }()

	(&CommandEncoder{}).transitionTextureIfNeeded(texture, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE)
	if got.Type != d3d12.D3D12_RESOURCE_BARRIER_TYPE_TRANSITION {
		t.Fatalf("barrier type = %d, want transition", got.Type)
	}
	transition := (*d3d12.D3D12_RESOURCE_TRANSITION_BARRIER)(unsafe.Pointer(&got.Union[0]))
	if transition.StateBefore != d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET || transition.StateAfter != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Fatalf("barrier states = %d -> %d, want RENDER_TARGET -> COPY_SOURCE", transition.StateBefore, transition.StateAfter)
	}
	if texture.currentState != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Fatalf("tracked texture state = %d, want COPY_SOURCE", texture.currentState)
	}
}

func TestTransitionTextureIfNeededEmitsRenderTargetBarrierFromCommon(t *testing.T) {
	texture := &Texture{raw: &d3d12.ID3D12Resource{}, currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}
	var calls int
	var got d3d12.D3D12_RESOURCE_BARRIER
	old := transitionTextureResourceBarrier
	transitionTextureResourceBarrier = func(_ *d3d12.ID3D12GraphicsCommandList, barrier *d3d12.D3D12_RESOURCE_BARRIER) {
		calls++
		got = *barrier
	}
	defer func() { transitionTextureResourceBarrier = old }()

	(&CommandEncoder{}).transitionTextureIfNeeded(texture, d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET)
	if calls != 1 {
		t.Fatalf("ResourceBarrier calls = %d, want 1", calls)
	}
	transition := (*d3d12.D3D12_RESOURCE_TRANSITION_BARRIER)(unsafe.Pointer(&got.Union[0]))
	if transition.StateBefore != d3d12.D3D12_RESOURCE_STATE_COMMON || transition.StateAfter != d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET {
		t.Fatalf("barrier states = %d -> %d, want COMMON -> RENDER_TARGET", transition.StateBefore, transition.StateAfter)
	}
	if texture.currentState != d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET {
		t.Fatalf("tracked texture state = %d, want RENDER_TARGET", texture.currentState)
	}
}

func TestNeedsExplicitTextureBarrier(t *testing.T) {
	tests := []struct {
		name            string
		current, target d3d12.D3D12_RESOURCE_STATES
		want            bool
	}{
		{name: "same state", current: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE, target: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE},
		{name: "common to render target", current: d3d12.D3D12_RESOURCE_STATE_COMMON, target: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET, want: true},
		{name: "render target to copy source", current: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET, target: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE, want: true},
		{name: "common to copy source", current: d3d12.D3D12_RESOURCE_STATE_COMMON, target: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsExplicitTextureBarrier(tt.current, tt.target); got != tt.want {
				t.Fatalf("needsExplicitTextureBarrier(%d, %d) = %t, want %t", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestTextureResolveUsesAndCommitsTrackedStates(t *testing.T) {
	source := &Texture{raw: &d3d12.ID3D12Resource{}, currentState: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET}
	destination := &Texture{raw: &d3d12.ID3D12Resource{}, currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}
	var transitions [][2]d3d12.D3D12_RESOURCE_STATES
	old := transitionTextureResourceBarrier
	transitionTextureResourceBarrier = func(_ *d3d12.ID3D12GraphicsCommandList, barrier *d3d12.D3D12_RESOURCE_BARRIER) {
		transition := (*d3d12.D3D12_RESOURCE_TRANSITION_BARRIER)(unsafe.Pointer(&barrier.Union[0]))
		transitions = append(transitions, [2]d3d12.D3D12_RESOURCE_STATES{transition.StateBefore, transition.StateAfter})
	}
	defer func() { transitionTextureResourceBarrier = old }()

	encoder := &CommandEncoder{}
	encoder.prepareTextureResolve(source, destination)
	encoder.finishTextureResolve(source, destination, d3d12.D3D12_RESOURCE_STATE_PRESENT)
	want := [][2]d3d12.D3D12_RESOURCE_STATES{
		{d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET, d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE},
		{d3d12.D3D12_RESOURCE_STATE_COMMON, d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST},
		{d3d12.D3D12_RESOURCE_STATE_RESOLVE_SOURCE, d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET},
		{d3d12.D3D12_RESOURCE_STATE_RESOLVE_DEST, d3d12.D3D12_RESOURCE_STATE_PRESENT},
	}
	if len(transitions) != len(want) {
		t.Fatalf("transition count = %d, want %d", len(transitions), len(want))
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transition %d = %v, want %v", i, transitions[i], want[i])
		}
	}
	if source.currentState != d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET || destination.currentState != d3d12.D3D12_RESOURCE_STATE_PRESENT {
		t.Fatalf("final states = %d/%d, want RENDER_TARGET/PRESENT", source.currentState, destination.currentState)
	}
}

func TestDepthStencilAttachmentState(t *testing.T) {
	tests := []struct {
		name                           string
		format                         gputypes.TextureFormat
		depthReadOnly, stencilReadOnly bool
		want                           d3d12.D3D12_RESOURCE_STATES
	}{
		{name: "depth writable", format: gputypes.TextureFormatDepth32Float, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE},
		{name: "depth read only", format: gputypes.TextureFormatDepth32Float, depthReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ},
		{name: "stencil read only", format: gputypes.TextureFormatStencil8, stencilReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ},
		{name: "combined read only", format: gputypes.TextureFormatDepth24PlusStencil8, depthReadOnly: true, stencilReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_READ},
		{name: "combined depth writable", format: gputypes.TextureFormatDepth24PlusStencil8, stencilReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE},
		{name: "combined stencil writable", format: gputypes.TextureFormatDepth24PlusStencil8, depthReadOnly: true, want: d3d12.D3D12_RESOURCE_STATE_DEPTH_WRITE},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := depthStencilAttachmentState(tt.format, tt.depthReadOnly, tt.stencilReadOnly); got != tt.want {
				t.Fatalf("state = %d, want %d", got, tt.want)
			}
		})
	}
}
