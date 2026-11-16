//go:build !rust && !(js && wasm)

package encoder

// The copy-usage scope helpers, relocated from the root package by S4's Move
// (the "staying-side shuttle" file copy_usage_native.go of commit ae8631e).
// They build one copy command's usage scope: the endpoints of a copy are
// validated against each other only, and each usage is applied to the command
// buffer tracker.

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
	buffer *resource.Buffer
	usage  track.BufferUses
}

type preparedCopyBufferUsage struct {
	buffer *resource.Buffer
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
			cb = request.buffer.CoreBuffer()
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
// usage scope: multiple roles of the same texture are merged, and incompatible
// roles (a texture as both copy source and destination) are rejected.
// Whole-texture granularity: a copy between disjoint subresources of one
// texture, which WebGPU allows, is rejected as well until the trackers become
// subresource-aware.
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
