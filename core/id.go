package core

import (
	"fmt"
)

// Index is the index component of a resource ID.
// It identifies the slot in the storage array.
type Index = uint32

// Epoch is the generation component of a resource ID.
// It prevents use-after-free by invalidating old IDs.
type Epoch = uint32

// RawID is the underlying 64-bit representation of a resource identifier.
// Layout: lower 32 bits = index, upper 32 bits = epoch.
type RawID uint64

// Zip combines an index and epoch into a RawID.
func Zip(index Index, epoch Epoch) RawID {
	return RawID(index) | (RawID(epoch) << 32)
}

// Unzip extracts the index and epoch from a RawID.
func (id RawID) Unzip() (Index, Epoch) {
	//nolint:gosec // G115: Safe conversion - masked to 32 bits
	return Index(id & 0xFFFFFFFF), Epoch(id >> 32)
}

// Index returns the index component of the RawID.
func (id RawID) Index() Index {
	//nolint:gosec // G115: Safe conversion - masked to 32 bits
	return Index(id & 0xFFFFFFFF)
}

// Epoch returns the epoch component of the RawID.
func (id RawID) Epoch() Epoch {
	//nolint:gosec // G115: Safe conversion - shifted down from upper 32 bits
	return Epoch(id >> 32)
}

// IsZero returns true if both index and epoch are zero.
func (id RawID) IsZero() bool {
	return id == 0
}

// String returns a string representation of the RawID.
func (id RawID) String() string {
	index, epoch := id.Unzip()
	return fmt.Sprintf("RawID(%d,%d)", index, epoch)
}

// Marker is a constraint for marker types used to distinguish ID types.
// Marker types are empty structs that provide compile-time type safety.
type Marker interface {
	marker() // unexported method prevents external implementation
}

// ID is a type-safe resource identifier parameterized by a marker type.
// Different resource types (Device, Buffer, Texture, etc.) have different
// marker types, preventing accidental misuse of IDs.
type ID[T Marker] struct {
	raw RawID
}

// NewID creates a new ID from index and epoch components.
func NewID[T Marker](index Index, epoch Epoch) ID[T] {
	return ID[T]{raw: Zip(index, epoch)}
}

// FromRaw creates an ID from a raw representation.
// Use with caution - the caller must ensure type safety.
func FromRaw[T Marker](raw RawID) ID[T] {
	return ID[T]{raw: raw}
}

// Raw returns the underlying RawID.
func (id ID[T]) Raw() RawID {
	return id.raw
}

// Unzip extracts the index and epoch from the ID.
func (id ID[T]) Unzip() (Index, Epoch) {
	return id.raw.Unzip()
}

// Index returns the index component of the ID.
func (id ID[T]) Index() Index {
	return id.raw.Index()
}

// Epoch returns the epoch component of the ID.
func (id ID[T]) Epoch() Epoch {
	return id.raw.Epoch()
}

// IsZero returns true if the ID is zero (invalid).
func (id ID[T]) IsZero() bool {
	return id.raw.IsZero()
}

// String returns a string representation of the ID.
func (id ID[T]) String() string {
	index, epoch := id.Unzip()
	return fmt.Sprintf("ID(%d,%d)", index, epoch)
}

// Marker types for each resource kind.
// These are empty structs that implement the Marker interface.

type adapterMarker struct{}

func (adapterMarker) marker() {}

type surfaceMarker struct{}

func (surfaceMarker) marker() {}

type deviceMarker struct{}

func (deviceMarker) marker() {}

type queueMarker struct{}

func (queueMarker) marker() {}

type bufferMarker struct{}

func (bufferMarker) marker() {}

type textureMarker struct{}

func (textureMarker) marker() {}

type textureViewMarker struct{}

func (textureViewMarker) marker() {}

type samplerMarker struct{}

func (samplerMarker) marker() {}

type bindGroupLayoutMarker struct{}

func (bindGroupLayoutMarker) marker() {}

type pipelineLayoutMarker struct{}

func (pipelineLayoutMarker) marker() {}

type bindGroupMarker struct{}

func (bindGroupMarker) marker() {}

type shaderModuleMarker struct{}

func (shaderModuleMarker) marker() {}

type renderPipelineMarker struct{}

func (renderPipelineMarker) marker() {}

type computePipelineMarker struct{}

func (computePipelineMarker) marker() {}

type commandEncoderMarker struct{}

func (commandEncoderMarker) marker() {}

type commandBufferMarker struct{}

func (commandBufferMarker) marker() {}

type querySetMarker struct{}

func (querySetMarker) marker() {}

// Type aliases for resource IDs.
// These provide convenient, readable type names.

// AdapterID identifies an Adapter resource.
type AdapterID = ID[adapterMarker]

// SurfaceID identifies a Surface resource.
type SurfaceID = ID[surfaceMarker]

// DeviceID identifies a Device resource.
type DeviceID = ID[deviceMarker]

// QueueID identifies a Queue resource.
type QueueID = ID[queueMarker]

// BufferID identifies a Buffer resource.
type BufferID = ID[bufferMarker]

// TextureID identifies a Texture resource.
type TextureID = ID[textureMarker]

// TextureViewID identifies a TextureView resource.
type TextureViewID = ID[textureViewMarker]

// SamplerID identifies a Sampler resource.
type SamplerID = ID[samplerMarker]

// BindGroupLayoutID identifies a BindGroupLayout resource.
type BindGroupLayoutID = ID[bindGroupLayoutMarker]

// PipelineLayoutID identifies a PipelineLayout resource.
type PipelineLayoutID = ID[pipelineLayoutMarker]

// BindGroupID identifies a BindGroup resource.
type BindGroupID = ID[bindGroupMarker]

// ShaderModuleID identifies a ShaderModule resource.
type ShaderModuleID = ID[shaderModuleMarker]

// RenderPipelineID identifies a RenderPipeline resource.
type RenderPipelineID = ID[renderPipelineMarker]

// ComputePipelineID identifies a ComputePipeline resource.
type ComputePipelineID = ID[computePipelineMarker]

// CommandEncoderID identifies a CommandEncoder resource.
type CommandEncoderID = ID[commandEncoderMarker]

// CommandBufferID identifies a CommandBuffer resource.
type CommandBufferID = ID[commandBufferMarker]

// QuerySetID identifies a QuerySet resource.
type QuerySetID = ID[querySetMarker]
