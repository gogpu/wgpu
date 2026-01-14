package core

import (
	"sync"
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// Resource placeholder types - will be properly defined later.
// These types represent the actual WebGPU resources managed by the hub.

// Adapter represents a physical GPU adapter.
type Adapter struct {
	// Info contains information about the adapter.
	Info types.AdapterInfo
	// Features contains the features supported by the adapter.
	Features types.Features
	// Limits contains the resource limits of the adapter.
	Limits types.Limits
	// Backend identifies which graphics backend this adapter uses.
	Backend types.Backend

	// === HAL integration fields ===

	// halAdapter is the underlying HAL adapter handle.
	// This is nil for mock adapters created without HAL integration.
	halAdapter hal.Adapter

	// halCapabilities contains the adapter's full capability information.
	// This is nil for mock adapters.
	halCapabilities *hal.Capabilities
}

// HALAdapter returns the underlying HAL adapter, if available.
// Returns nil for mock adapters created without HAL integration.
func (a *Adapter) HALAdapter() hal.Adapter {
	return a.halAdapter
}

// HasHAL returns true if the adapter has HAL integration.
func (a *Adapter) HasHAL() bool {
	return a.halAdapter != nil
}

// Capabilities returns the adapter's full capability information.
// Returns nil for mock adapters.
func (a *Adapter) Capabilities() *hal.Capabilities {
	return a.halCapabilities
}

// Device represents a logical GPU device.
//
// Device wraps a HAL device handle and provides safe access to GPU resources.
// The HAL device is wrapped in a Snatchable to enable safe deferred destruction.
//
// The Device maintains backward compatibility with the ID-based API while
// adding HAL integration for actual GPU operations.
type Device struct {
	// === ID-based API fields (backward compatibility) ===

	// Adapter is the adapter this device was created from (ID-based API).
	Adapter AdapterID
	// Queue is the device's default queue (ID-based API).
	Queue QueueID

	// === HAL integration fields ===

	// raw is the HAL device handle wrapped for safe destruction.
	// This is nil for devices created via the ID-based API without HAL.
	raw *Snatchable[hal.Device]

	// adapter is a pointer to the parent Adapter struct.
	// This is nil for devices created via the ID-based API without HAL.
	adapter *Adapter

	// queue is a pointer to the associated Queue struct.
	// This is nil for devices created via the ID-based API without HAL.
	queue *Queue

	// snatchLock provides device-global coordination for resource destruction.
	// This is nil for devices created via the ID-based API without HAL.
	snatchLock *SnatchLock

	// trackerIndices manages tracker indices per resource type.
	// This is nil for devices created via the ID-based API without HAL.
	trackerIndices *TrackerIndexAllocators

	// === Common fields ===

	// Label is a debug label for the device.
	Label string
	// Features contains the features enabled on this device.
	Features types.Features
	// Limits contains the resource limits of this device.
	Limits types.Limits

	// valid indicates whether the device is still valid for use.
	// Once a device is destroyed, this becomes false.
	valid *atomic.Bool
}

// NewDevice creates a new Device wrapping a HAL device.
//
// This is the constructor for devices with full HAL integration.
// The device takes ownership of the HAL device and will destroy it
// when the Device is destroyed.
//
// Parameters:
//   - halDevice: The HAL device to wrap (ownership transferred)
//   - adapter: The parent adapter struct
//   - features: Enabled features for this device
//   - limits: Resource limits for this device
//   - label: Debug label for the device
//
// Returns a new Device ready for use.
func NewDevice(
	halDevice hal.Device,
	adapter *Adapter,
	features types.Features,
	limits types.Limits,
	label string,
) *Device {
	d := &Device{
		raw:            NewSnatchable(halDevice),
		adapter:        adapter,
		snatchLock:     NewSnatchLock(),
		trackerIndices: NewTrackerIndexAllocators(),
		Label:          label,
		Features:       features,
		Limits:         limits,
	}
	valid := &atomic.Bool{}
	valid.Store(true)
	d.valid = valid
	return d
}

// Raw returns the underlying HAL device if it hasn't been snatched.
//
// The caller must hold a SnatchGuard obtained from the device's SnatchLock.
// This ensures the device won't be destroyed during access.
//
// Returns nil if:
//   - The device has no HAL integration (ID-based API only)
//   - The HAL device has been snatched (device destroyed)
func (d *Device) Raw(guard *SnatchGuard) hal.Device {
	if d.raw == nil {
		return nil
	}
	ptr := d.raw.Get(guard)
	if ptr == nil {
		return nil
	}
	return *ptr
}

// IsValid returns true if the device is still valid for use.
//
// A device becomes invalid after Destroy() is called.
func (d *Device) IsValid() bool {
	if d.valid == nil {
		return false
	}
	return d.valid.Load()
}

// SnatchLock returns the device's snatch lock for resource coordination.
//
// The snatch lock must be held when accessing the raw HAL device or
// when destroying resources associated with this device.
//
// Returns nil if the device has no HAL integration.
func (d *Device) SnatchLock() *SnatchLock {
	return d.snatchLock
}

// Destroy releases the HAL device and marks the device as invalid.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), IsValid() returns false and Raw() returns nil.
//
// If the device has no HAL integration (ID-based API only), this only
// marks the device as invalid.
func (d *Device) Destroy() {
	// Mark as invalid first to prevent new operations
	if d.valid != nil {
		d.valid.Store(false)
	}

	if d.snatchLock == nil || d.raw == nil {
		return
	}

	// Acquire exclusive lock for destruction
	guard := d.snatchLock.Write()
	defer guard.Release()

	// Snatch the HAL device
	halDevice := d.raw.Snatch(guard)
	if halDevice == nil {
		// Already destroyed
		return
	}

	// Destroy the HAL device
	(*halDevice).Destroy()
}

// checkValid returns an error if the device is not valid.
func (d *Device) checkValid() error {
	if d.valid == nil || !d.valid.Load() {
		return ErrDeviceDestroyed
	}
	return nil
}

// HasHAL returns true if the device has HAL integration.
//
// Devices created via NewDevice have HAL integration.
// Devices created via the ID-based API (CreateDevice) do not.
func (d *Device) HasHAL() bool {
	return d.raw != nil
}

// TrackerIndices returns the tracker index allocators for this device.
//
// Returns nil if the device has no HAL integration.
func (d *Device) TrackerIndices() *TrackerIndexAllocators {
	return d.trackerIndices
}

// ParentAdapter returns the parent adapter for this device.
//
// Returns nil if the device has no HAL integration.
func (d *Device) ParentAdapter() *Adapter {
	return d.adapter
}

// AssociatedQueue returns the associated queue for this device.
//
// Returns nil if the queue has not been set.
func (d *Device) AssociatedQueue() *Queue {
	return d.queue
}

// SetAssociatedQueue sets the associated queue for this device.
//
// This is called internally when creating a device to link it with its queue.
func (d *Device) SetAssociatedQueue(queue *Queue) {
	d.queue = queue
}

// CreateBuffer creates a new buffer on this device.
//
// Validation performed:
//   - Device must be valid (not destroyed)
//   - Size must be > 0
//   - Size must not exceed MaxBufferSize device limit
//   - Usage must not be empty
//   - Usage must not contain unknown bits
//   - MAP_READ and MAP_WRITE are mutually exclusive
//
// Size is automatically aligned to COPY_BUFFER_ALIGNMENT (4 bytes).
//
// Returns the buffer and nil on success.
// Returns nil and an error if validation fails or HAL creation fails.
func (d *Device) CreateBuffer(desc *types.BufferDescriptor) (*Buffer, error) {
	// 1. Check device validity
	if err := d.checkValid(); err != nil {
		return nil, err
	}

	// 2. Validate descriptor
	if desc == nil {
		return nil, &CreateBufferError{
			Kind:  CreateBufferErrorEmptyUsage,
			Label: "",
		}
	}

	// 3. Validate size
	if desc.Size == 0 {
		return nil, &CreateBufferError{
			Kind:  CreateBufferErrorZeroSize,
			Label: desc.Label,
		}
	}
	if desc.Size > d.Limits.MaxBufferSize {
		return nil, &CreateBufferError{
			Kind:          CreateBufferErrorMaxBufferSize,
			Label:         desc.Label,
			RequestedSize: desc.Size,
			MaxSize:       d.Limits.MaxBufferSize,
		}
	}

	// 4. Validate usage
	if desc.Usage == 0 {
		return nil, &CreateBufferError{
			Kind:  CreateBufferErrorEmptyUsage,
			Label: desc.Label,
		}
	}
	if desc.Usage.ContainsUnknownBits() {
		return nil, &CreateBufferError{
			Kind:  CreateBufferErrorInvalidUsage,
			Label: desc.Label,
		}
	}

	// 5. Validate MAP_READ/MAP_WRITE exclusivity
	hasMapRead := desc.Usage.Contains(types.BufferUsageMapRead)
	hasMapWrite := desc.Usage.Contains(types.BufferUsageMapWrite)
	if hasMapRead && hasMapWrite {
		return nil, &CreateBufferError{
			Kind:  CreateBufferErrorMapReadWriteExclusive,
			Label: desc.Label,
		}
	}

	// 6. Calculate aligned size (align to COPY_BUFFER_ALIGNMENT = 4)
	const copyBufferAlignment uint64 = 4
	alignedSize := (desc.Size + copyBufferAlignment - 1) &^ (copyBufferAlignment - 1)

	// 7. Build HAL descriptor
	halDesc := &hal.BufferDescriptor{
		Label:            desc.Label,
		Size:             alignedSize,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	}

	// 8. Acquire snatch guard for HAL access
	guard := d.snatchLock.Read()
	defer guard.Release()

	halDevice := d.raw.Get(guard)
	if halDevice == nil {
		return nil, ErrDeviceDestroyed
	}

	// 9. Create HAL buffer
	halBuffer, err := (*halDevice).CreateBuffer(halDesc)
	if err != nil {
		return nil, &CreateBufferError{
			Kind:     CreateBufferErrorHAL,
			Label:    desc.Label,
			HALError: err,
		}
	}

	// 10. Wrap in core Buffer
	buffer := NewBuffer(halBuffer, d, desc.Usage, desc.Size, desc.Label)

	// 11. Handle MappedAtCreation
	if desc.MappedAtCreation {
		buffer.SetMapState(BufferMapStateMapped)
		// Mark entire buffer as initialized when mapped at creation
		buffer.MarkInitialized(0, desc.Size)
	}

	return buffer, nil
}

// Queue represents a command queue for a device.
type Queue struct {
	// Device is the device this queue belongs to.
	Device DeviceID
	// Label is a debug label for the queue.
	Label string
}

// Buffer represents a GPU buffer with HAL integration.
//
// Buffer wraps a HAL buffer handle and provides safe access to GPU memory.
// The HAL buffer is wrapped in a Snatchable to enable safe deferred destruction.
//
// Buffer maintains backward compatibility with the ID-based API while
// adding HAL integration for actual GPU operations.
type Buffer struct {
	// === HAL integration fields ===

	// raw is the HAL buffer handle wrapped for safe destruction.
	// This is nil for buffers created via the ID-based API without HAL.
	raw *Snatchable[hal.Buffer]

	// device is a pointer to the parent Device.
	// This is nil for buffers created via the ID-based API without HAL.
	device *Device

	// === WebGPU properties ===

	// usage is the buffer's usage flags.
	usage types.BufferUsage

	// size is the buffer size in bytes.
	size uint64

	// label is a debug label for the buffer.
	label string

	// === State tracking ===

	// initTracker tracks which regions have been initialized.
	initTracker *BufferInitTracker

	// mapState tracks the current mapping state.
	// Protected by the device's snatch lock for modification.
	mapState BufferMapState

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData
}

// BufferMapState represents the current mapping state of a buffer.
type BufferMapState int

const (
	// BufferMapStateIdle indicates the buffer is not mapped.
	BufferMapStateIdle BufferMapState = iota
	// BufferMapStatePending indicates a mapping operation is in progress.
	BufferMapStatePending
	// BufferMapStateMapped indicates the buffer is currently mapped.
	BufferMapStateMapped
)

// BufferInitTracker tracks which parts of a buffer have been initialized.
//
// This is used for validation to ensure uninitialized memory is not read.
type BufferInitTracker struct {
	mu          sync.RWMutex
	initialized []bool // Per-chunk initialization status
	chunkSize   uint64
}

// TrackingData holds per-resource tracking information.
//
// Each resource that needs state tracking during command encoding
// embeds a TrackingData struct to hold its tracker index.
//
// This is a stub - full implementation in CORE-006.
type TrackingData struct {
	index TrackerIndex
}

// TrackerIndex is a dense index for efficient resource state tracking.
//
// Unlike resource IDs (which use epochs and may be sparse), tracker indices
// are always dense (0, 1, 2, ...) for efficient array access.
//
// This is a stub - full implementation in CORE-006.
type TrackerIndex uint32

// InvalidTrackerIndex represents an unassigned tracker index.
const InvalidTrackerIndex TrackerIndex = ^TrackerIndex(0)

// NewBuffer creates a core Buffer wrapping a HAL buffer.
//
// This is the constructor for buffers with full HAL integration.
// The buffer takes ownership of the HAL buffer and will destroy it
// when the Buffer is destroyed.
//
// Parameters:
//   - halBuffer: The HAL buffer to wrap (ownership transferred)
//   - device: The parent device
//   - usage: Buffer usage flags
//   - size: Buffer size in bytes
//   - label: Debug label for the buffer
//
// Returns a new Buffer ready for use.
func NewBuffer(
	halBuffer hal.Buffer,
	device *Device,
	usage types.BufferUsage,
	size uint64,
	label string,
) *Buffer {
	return &Buffer{
		raw:         NewSnatchable(halBuffer),
		device:      device,
		usage:       usage,
		size:        size,
		label:       label,
		initTracker: NewBufferInitTracker(size),
		trackingData: NewTrackingData(
			device.TrackerIndices(),
		),
		mapState: BufferMapStateIdle,
	}
}

// NewBufferInitTracker creates a new initialization tracker for a buffer.
//
// The tracker divides the buffer into chunks and tracks which chunks
// have been initialized (written to).
func NewBufferInitTracker(size uint64) *BufferInitTracker {
	const chunkSize uint64 = 4096 // 4KB chunks
	if size == 0 {
		return &BufferInitTracker{
			initialized: nil,
			chunkSize:   chunkSize,
		}
	}
	numChunks := (size + chunkSize - 1) / chunkSize
	return &BufferInitTracker{
		initialized: make([]bool, numChunks),
		chunkSize:   chunkSize,
	}
}

// NewTrackingData creates tracking data for a resource.
//
// This is a stub - full implementation in CORE-006.
func NewTrackingData(_ *TrackerIndexAllocators) *TrackingData {
	return &TrackingData{
		index: InvalidTrackerIndex,
	}
}

// Index returns the tracker index for this resource.
func (t *TrackingData) Index() TrackerIndex {
	return t.index
}

// Raw returns the underlying HAL buffer if it hasn't been snatched.
//
// The caller must hold a SnatchGuard obtained from the device's SnatchLock.
// This ensures the buffer won't be destroyed during access.
//
// Returns nil if:
//   - The buffer has no HAL integration (ID-based API only)
//   - The HAL buffer has been snatched (buffer destroyed)
func (b *Buffer) Raw(guard *SnatchGuard) hal.Buffer {
	if b.raw == nil {
		return nil
	}
	p := b.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// Device returns the parent device for this buffer.
//
// Returns nil if the buffer has no HAL integration.
func (b *Buffer) Device() *Device {
	return b.device
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() types.BufferUsage {
	return b.usage
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// Label returns the buffer's debug label.
func (b *Buffer) Label() string {
	return b.label
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() BufferMapState {
	return b.mapState
}

// SetMapState updates the mapping state of the buffer.
// Caller must hold appropriate synchronization (device snatch lock).
func (b *Buffer) SetMapState(state BufferMapState) {
	b.mapState = state
}

// TrackingData returns the tracking data for this buffer.
func (b *Buffer) TrackingData() *TrackingData {
	return b.trackingData
}

// Destroy releases the HAL buffer.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (b *Buffer) Destroy() {
	if b.device == nil || b.device.SnatchLock() == nil || b.raw == nil {
		return
	}

	// First, get the HAL device reference while holding a read lock.
	// This must be done before acquiring the exclusive lock.
	readGuard := b.device.SnatchLock().Read()
	halDevice := b.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		// Device already destroyed, can't destroy buffer properly
		return
	}

	// Now acquire exclusive lock for the actual destruction
	exclusiveGuard := b.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	// Snatch the HAL buffer
	halBuffer := b.raw.Snatch(exclusiveGuard)
	if halBuffer == nil {
		// Already destroyed
		return
	}

	// Destroy the HAL buffer
	halDevice.DestroyBuffer(*halBuffer)
}

// IsDestroyed returns true if the buffer has been destroyed.
func (b *Buffer) IsDestroyed() bool {
	if b.raw == nil {
		return true
	}
	return b.raw.IsSnatched()
}

// HasHAL returns true if the buffer has HAL integration.
func (b *Buffer) HasHAL() bool {
	return b.raw != nil
}

// MarkInitialized marks a region of the buffer as initialized.
func (b *Buffer) MarkInitialized(offset, size uint64) {
	if b.initTracker == nil {
		return
	}
	b.initTracker.MarkInitialized(offset, size)
}

// IsInitialized returns true if a region of the buffer is initialized.
func (b *Buffer) IsInitialized(offset, size uint64) bool {
	if b.initTracker == nil {
		return true // No tracker means assume initialized
	}
	return b.initTracker.IsInitialized(offset, size)
}

// MarkInitialized marks a region as initialized in the tracker.
func (t *BufferInitTracker) MarkInitialized(offset, size uint64) {
	if t == nil || len(t.initialized) == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	startChunk := offset / t.chunkSize
	endChunk := (offset + size + t.chunkSize - 1) / t.chunkSize

	for i := startChunk; i < endChunk && i < uint64(len(t.initialized)); i++ {
		t.initialized[i] = true
	}
}

// IsInitialized returns true if a region is fully initialized.
func (t *BufferInitTracker) IsInitialized(offset, size uint64) bool {
	if t == nil || len(t.initialized) == 0 {
		return true // No tracker means assume initialized
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	startChunk := offset / t.chunkSize
	endChunk := (offset + size + t.chunkSize - 1) / t.chunkSize

	for i := startChunk; i < endChunk && i < uint64(len(t.initialized)); i++ {
		if !t.initialized[i] {
			return false
		}
	}
	return true
}

// Texture represents a GPU texture.
type Texture struct{}

// TextureView represents a view into a texture.
type TextureView struct{}

// Sampler represents a texture sampler.
type Sampler struct{}

// BindGroupLayout represents the layout of a bind group.
type BindGroupLayout struct{}

// PipelineLayout represents the layout of a pipeline.
type PipelineLayout struct{}

// BindGroup represents a collection of resources bound together.
type BindGroup struct{}

// ShaderModule represents a compiled shader module.
type ShaderModule struct{}

// RenderPipeline represents a render pipeline.
type RenderPipeline struct{}

// ComputePipeline represents a compute pipeline.
type ComputePipeline struct{}

// CommandEncoder represents a command encoder.
type CommandEncoder struct{}

// CommandBuffer represents a recorded command buffer.
type CommandBuffer struct{}

// QuerySet represents a set of queries.
type QuerySet struct{}

// Surface represents a rendering surface.
type Surface struct{}
