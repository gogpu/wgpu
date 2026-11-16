//go:build !rust && !(js && wasm)

package wgpu

// Staying-side shuttle (Prepare 2026-11-16). These declarations used to live
// in encoder_native.go, one of the three encoder context files that S4's Move
// relocates into internal/encoder. The staying white-box test
// copy_usage_coverage_test.go names them, which is the planner's
// `unexported-boundary` refusal class: a staying file naming an unexported
// declaration that moves.
//
// With the declarations on the staying side the remaining direction is the
// tolerated one: the moving encoder files reference them, and the Move defers
// that through handRewrite. The Move relocates this file, its declarations and
// copy_usage_coverage_test.go into internal/encoder.

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/resource"
)

// copyTextureUsage describes one texture endpoint of a copy command.
type copyTextureUsage struct {
	texture *resource.Texture
	usage   track.TextureUses
}

type preparedCopyTextureUsage struct {
	texture *resource.Texture
	index   track.TrackerIndex
	usage   track.TextureUses
}

// copyBufferUsage describes one buffer endpoint of a copy command.
type copyBufferUsage struct {
	buffer *Buffer
	usage  track.BufferUses
}

type preparedCopyBufferUsage struct {
	buffer *Buffer
	index  track.TrackerIndex
	usage  track.BufferUses
}

// prepareCopyBufferUsages builds the buffer part of one copy command's usage
// scope: multiple roles of the same buffer are merged, and incompatible roles
// (a buffer as both copy source and destination) are rejected.
//
// Reference: wgpu-core command/transfer.rs SameSourceDestinationBuffer
func prepareCopyBufferUsages(requests []copyBufferUsage) ([]preparedCopyBufferUsage, error) {
	prepared := make([]preparedCopyBufferUsage, 0, len(requests))
	positions := make(map[track.TrackerIndex]int, len(requests))
	for _, request := range requests {
		var cb *core.Buffer
		if request.buffer != nil {
			cb = request.buffer.impl.CoreBuffer()
		}
		if cb == nil {
			continue
		}
		td := cb.TrackingData()
		if td == nil || !td.Index().IsValid() {
			continue
		}
		index := td.Index()
		position, exists := positions[index]
		if !exists {
			positions[index] = len(prepared)
			prepared = append(prepared, preparedCopyBufferUsage{
				buffer: request.buffer, index: index, usage: request.usage,
			})
			continue
		}
		existing := prepared[position].usage
		if !existing.IsCompatible(request.usage) {
			return nil, &track.UsageConflictError{Index: index, Existing: existing, New: request.usage}
		}
		prepared[position].usage = existing | request.usage
	}
	return prepared, nil
}

// prepareCopyTextureUsages builds the texture part of one copy command's
// usage scope: multiple roles of the same texture are merged, and
// incompatible roles (a texture as both copy source and destination) are
// rejected. Whole-texture granularity: a copy between disjoint subresources
// of one texture, which WebGPU allows, is rejected as well until the
// trackers become subresource-aware.
//
// Reference: wgpu-core command/transfer.rs validate_copy_within_same_texture
func prepareCopyTextureUsages(requests []copyTextureUsage) ([]preparedCopyTextureUsage, error) {
	prepared := make([]preparedCopyTextureUsage, 0, len(requests))
	positions := make(map[track.TrackerIndex]int, len(requests))
	for _, request := range requests {
		if request.texture == nil || request.texture.CoreTexture() == nil {
			continue
		}
		td := request.texture.CoreTexture().TrackingData()
		if td == nil || !td.Index().IsValid() {
			continue
		}
		index := td.Index()
		position, exists := positions[index]
		if !exists {
			positions[index] = len(prepared)
			prepared = append(prepared, preparedCopyTextureUsage{
				texture: request.texture, index: index, usage: request.usage,
			})
			continue
		}
		existing := prepared[position].usage
		combined := existing | request.usage
		if !combined.IsCompatible(combined) {
			return nil, &track.TextureUsageConflictError{
				Index: index, Existing: existing, New: request.usage,
			}
		}
		prepared[position].usage = combined
	}
	return prepared, nil
}
