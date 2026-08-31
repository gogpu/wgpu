//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
)

func TestCopyUsageHelpersSkipUntrackedResources(t *testing.T) {
	t.Parallel()

	encoder := &CommandEncoder{}
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{{usage: track.BufferUsesCopySrc}}) {
		t.Fatal("nil buffer should be ignored")
	}
	if !encoder.recordCopyUsages(nil, nil, track.BufferUsesNone) {
		t.Fatal("encoder without core state should ignore copy usage")
	}

	invalidCoreTexture := core.NewTexture(
		nil, nil,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageCopySrc,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		1, 1, "invalid-tracking",
	)
	t.Cleanup(invalidCoreTexture.Destroy)

	requests := []copyTextureUsage{
		{texture: nil, usage: track.TextureUsesCopySrc},
		{texture: &Texture{}, usage: track.TextureUsesCopySrc},
		{texture: &Texture{coreTexture: &core.Texture{}}, usage: track.TextureUsesCopySrc},
		{texture: &Texture{coreTexture: invalidCoreTexture}, usage: track.TextureUsesCopySrc},
	}
	prepared, err := prepareCopyTextureUsages(requests)
	if err != nil {
		t.Fatalf("prepareCopyTextureUsages: %v", err)
	}
	if len(prepared) != 0 {
		t.Fatalf("prepared %d untracked resources, want 0", len(prepared))
	}
}

func TestCopyUsagePreflightGuardBranches(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	texture := createCopyScopeTexture(t, device, "preflight-guards", TextureUsageCopySrc|TextureUsageCopyDst)
	defer texture.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	defer encoder.DiscardEncoding()
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{{usage: track.BufferUsesCopySrc}}) {
		t.Fatal("nil buffer with live core encoder should be ignored")
	}
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{{buffer: &core.Buffer{}, usage: track.BufferUsesCopySrc}}) {
		t.Fatal("buffer without tracking data should be ignored")
	}

	textureIndex := texture.coreTexture.TrackingData().Index()
	if err := encoder.core.Mutable().TextureScope().SetUsage(textureIndex, track.TextureUsesColorTarget); err != nil {
		t.Fatalf("seed texture usage: %v", err)
	}

	request := preparedCopyTextureUsage{
		texture: texture,
		index:   textureIndex,
		usage:   track.TextureUsesCopySrc,
	}
	prepared, err := prepareCopyTextureUsages([]copyTextureUsage{
		{texture: texture, usage: track.TextureUsesCopySrc},
		{texture: texture, usage: track.TextureUsesResource},
	})
	if err != nil {
		t.Fatalf("prepare compatible duplicate texture usages: %v", err)
	}
	if len(prepared) != 1 || prepared[0].usage != track.TextureUsesCopySrc|track.TextureUsesResource {
		t.Fatalf("prepared duplicate usages = %+v, want one CopySrc|Resource request", prepared)
	}

	encoder.core.Mutable().TextureScope().ReplaceUsage(textureIndex, track.TextureUsesResource)
	compatibleRequests := []preparedCopyTextureUsage{request}
	if err := encoder.preflightCopyTextureUsages(compatibleRequests); err != nil {
		t.Fatalf("compatible texture usage rejected: %v", err)
	}
	if got := compatibleRequests[0].usage; got != track.TextureUsesResource|track.TextureUsesCopySrc {
		t.Fatalf("merged compatible texture usage = %v, want Resource|CopySrc", got)
	}
	encoder.core.Mutable().TextureScope().ReplaceUsage(textureIndex, track.TextureUsesColorTarget)

	encoder.explicitTextureTransitions = map[*Texture]gputypes.TextureUsage{
		texture: TextureUsageCopyDst,
	}
	if err := encoder.preflightCopyTextureUsages([]preparedCopyTextureUsage{request}); err == nil {
		t.Fatal("mismatched explicit transition accepted incompatible usage")
	}

	encoder.explicitTextureTransitions[texture] = TextureUsageCopySrc
	requests := []preparedCopyTextureUsage{request}
	if err := encoder.preflightCopyTextureUsages(requests); err != nil {
		t.Fatalf("matching explicit transition rejected: %v", err)
	}
	if got := requests[0].usage; got != track.TextureUsesCopySrc {
		t.Fatalf("usage after explicit transition = %v, want CopySrc", got)
	}

	if _, _, tracked, err := encoder.preflightCopyBufferUsage(&core.Buffer{}, track.BufferUsesCopySrc); err != nil || tracked {
		t.Fatalf("buffer without tracking data = (tracked %v, err %v), want ignored", tracked, err)
	}
	invalidBuffer := core.NewBuffer(nil, nil, gputypes.BufferUsageCopySrc, 4, "invalid-tracking")
	defer invalidBuffer.Destroy()
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{{buffer: invalidBuffer, usage: track.BufferUsesCopySrc}}) {
		t.Fatal("buffer with invalid tracker index should be ignored")
	}
	if _, _, tracked, err := encoder.preflightCopyBufferUsage(invalidBuffer, track.BufferUsesCopySrc); err != nil || tracked {
		t.Fatalf("buffer with invalid tracker index = (tracked %v, err %v), want ignored", tracked, err)
	}

	buffer, err := device.CreateBuffer(&BufferDescriptor{
		Size:  4,
		Usage: BufferUsageCopySrc | BufferUsageStorage,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()
	bufferIndex := buffer.core.TrackingData().Index()
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{
		{buffer: buffer.core, usage: track.BufferUsesCopySrc},
		{buffer: buffer.core, usage: track.BufferUsesUniform},
	}) {
		t.Fatal("duplicate buffer endpoint rejected compatible usages")
	}
	if got := encoder.core.Mutable().BufferScope().GetUsage(bufferIndex); got != track.BufferUsesCopySrc|track.BufferUsesUniform {
		t.Fatalf("combined duplicate buffer usage = %v, want CopySrc|Uniform", got)
	}
	encoder.core.Mutable().BufferScope().ReplaceUsage(bufferIndex, track.BufferUsesNone)

	if encoder.recordCopyBufferUsages([]copyBufferUsage{
		{buffer: buffer.core, usage: track.BufferUsesCopySrc},
		{buffer: buffer.core, usage: track.BufferUsesCopyDst},
	}) {
		t.Fatal("duplicate buffer endpoint accepted incompatible usages")
	}

	encoder.core.Mutable().BufferScope().ReplaceUsage(bufferIndex, track.BufferUsesVertex)
	if !encoder.recordCopyBufferUsages([]copyBufferUsage{{buffer: buffer.core, usage: track.BufferUsesUniform}}) {
		t.Fatal("compatible existing buffer usage was rejected")
	}
	if got := encoder.core.Mutable().BufferScope().GetUsage(bufferIndex); got != track.BufferUsesVertex|track.BufferUsesUniform {
		t.Fatalf("merged compatible buffer usage = %v, want Vertex|Uniform", got)
	}
	if _, usage, tracked, err := encoder.preflightCopyBufferUsage(buffer.core, track.BufferUsesUniform); err != nil || !tracked || usage != track.BufferUsesVertex|track.BufferUsesUniform {
		t.Fatalf("compatible buffer preflight = (usage %v, tracked %v, err %v), want Vertex|Uniform", usage, tracked, err)
	}
	encoder.core.Mutable().BufferScope().ReplaceUsage(bufferIndex, track.BufferUsesStorageWrite)
	if encoder.recordCopyBufferUsages([]copyBufferUsage{{buffer: buffer.core, usage: track.BufferUsesCopySrc}}) {
		t.Fatal("recordCopyBufferUsages accepted incompatible usage")
	}
}

func TestCopyBufferToBufferRejectsReleasedEndpoint(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	src := createCopyScopeBuffer(t, device, "released-copy-src")
	dst := createCopyScopeBuffer(t, device, "released-copy-dst")
	defer dst.Release()
	src.Release()

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	encoder.CopyBufferToBuffer(src, 0, dst, 0, 4)
	if _, err := encoder.Finish(); err == nil {
		t.Fatal("Finish succeeded after copying from a released buffer")
	}
}

func TestTransitionTexturesReusesExplicitTransitionMap(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "explicit-transition-map", TextureUsageCopySrc|TextureUsageCopyDst)
	defer texture.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Usage: TextureUsageTransition{
			OldUsage: TextureUsageTextureBinding,
			NewUsage: TextureUsageCopySrc,
		},
	}})
	encoder.TransitionTextures([]TextureBarrier{{
		Texture: texture,
		Usage: TextureUsageTransition{
			OldUsage: TextureUsageCopySrc,
			NewUsage: TextureUsageCopyDst,
		},
	}})
	if got := encoder.explicitTextureTransitions[texture]; got != TextureUsageCopyDst {
		t.Fatalf("explicit transition state = %v, want CopyDst", got)
	}
	commandBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	commandBuffer.Release()
}
