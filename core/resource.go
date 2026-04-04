package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Resource placeholder types - will be properly defined later.
// These types represent the actual WebGPU resources managed by the hub.

// Adapter represents a physical GPU adapter.
type Adapter struct {
	// Info contains information about the adapter.
	Info gputypes.AdapterInfo
	// Features contains the features supported by the adapter.
	Features gputypes.Features
	// Limits contains the resource limits of the adapter.
	Limits gputypes.Limits
	// Backend identifies which graphics backend this adapter uses.
	Backend gputypes.Backend

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

	// destroyQueue defers HAL resource destruction until the GPU completes
	// the submission that was active when the resource was released.
	// This prevents use-after-free on DX12/Vulkan when a resource is
	// released while the GPU is still referencing it (BUG-DX12-TDR).
	// Matches Rust wgpu-core's LifetimeTracker pattern.
	destroyQueue *DestroyQueue

	// === Common fields ===

	// Label is a debug label for the device.
	Label string
	// Features contains the features enabled on this device.
	Features gputypes.Features
	// Limits contains the resource limits of this device.
	Limits gputypes.Limits

	// valid indicates whether the device is still valid for use.
	// Once a device is destroyed, this becomes false.
	valid *atomic.Bool

	// errorScopeManager manages the error scope stack for this device.
	// Initialized lazily on first use. This is a plain pointer because
	// Device is passed by value in the legacy ID-based API, which
	// prohibits noCopy types (sync.Once, atomic.Pointer, etc.).
	// Thread-safety is provided by ErrorScopeManager's internal mutex.
	errorScopeManager *ErrorScopeManager
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
	features gputypes.Features,
	limits gputypes.Limits,
	label string,
) *Device {
	d := &Device{
		raw:            NewSnatchable(halDevice),
		adapter:        adapter,
		snatchLock:     NewSnatchLock(),
		trackerIndices: NewTrackerIndexAllocators(),
		destroyQueue:   NewDestroyQueue(),
		Label:          label,
		Features:       features,
		Limits:         limits,
	}
	valid := &atomic.Bool{}
	valid.Store(true)
	d.valid = valid
	trackResource(uintptr(unsafe.Pointer(d)), "Device") //nolint:gosec // debug tracking uses pointer as unique ID
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

	untrackResource(uintptr(unsafe.Pointer(d))) //nolint:gosec // debug tracking uses pointer as unique ID

	// Flush all deferred resource destructions before destroying the device.
	// At this point the GPU should be idle (caller should have called WaitIdle
	// or equivalent), so all deferred destroys are safe to execute.
	if d.destroyQueue != nil {
		d.destroyQueue.FlushAll()
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

// DestroyQueueRef returns the device's deferred destruction queue.
// Resources scheduled via Defer() will be destroyed when Triage() confirms
// their associated GPU submission has completed.
//
// Returns nil if the device has no HAL integration.
func (d *Device) DestroyQueueRef() *DestroyQueue {
	return d.destroyQueue
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
func (d *Device) CreateBuffer(desc *gputypes.BufferDescriptor) (*Buffer, error) {
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
	hasMapRead := desc.Usage.Contains(gputypes.BufferUsageMapRead)
	hasMapWrite := desc.Usage.Contains(gputypes.BufferUsageMapWrite)
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
	usage gputypes.BufferUsage

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

	// Ref is the GPU-aware reference counter for this buffer (Phase 2).
	// When a command encoder uses this buffer, it Clone()'s the Ref.
	// When the GPU completes the submission, the clone is Drop()'d.
	// This keeps the resource alive exactly as long as needed.
	Ref *ResourceRef
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
	usage gputypes.BufferUsage,
	size uint64,
	label string,
) *Buffer {
	b := &Buffer{
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
	trackResource(uintptr(unsafe.Pointer(b)), "Buffer") //nolint:gosec // debug tracking uses pointer as unique ID
	return b
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
func (b *Buffer) Usage() gputypes.BufferUsage {
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
	untrackResource(uintptr(unsafe.Pointer(b))) //nolint:gosec // debug tracking uses pointer as unique ID

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

// Texture represents a GPU texture with HAL integration.
//
// Texture wraps a HAL texture handle and stores WebGPU texture properties.
// The HAL texture is wrapped in a Snatchable to enable safe deferred destruction.
type Texture struct {
	// raw is the HAL texture handle wrapped for safe destruction.
	raw *Snatchable[hal.Texture]

	// device is a pointer to the parent Device.
	device *Device

	// format is the texture pixel format.
	format gputypes.TextureFormat

	// dimension is the texture dimension (1D, 2D, 3D).
	dimension gputypes.TextureDimension

	// usage is the texture usage flags.
	usage gputypes.TextureUsage

	// size is the texture dimensions.
	size gputypes.Extent3D

	// mipLevelCount is the number of mip levels.
	mipLevelCount uint32

	// sampleCount is the number of samples per pixel.
	sampleCount uint32

	// label is a debug label for the texture.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this texture (Phase 2).
	Ref *ResourceRef
}

// NewTexture creates a core Texture wrapping a HAL texture.
//
// Parameters:
//   - halTexture: The HAL texture to wrap (ownership transferred)
//   - device: The parent device
//   - format: Texture pixel format
//   - dimension: Texture dimension (1D, 2D, 3D)
//   - usage: Texture usage flags
//   - size: Texture dimensions
//   - mipLevelCount: Number of mip levels
//   - sampleCount: Number of samples per pixel
//   - label: Debug label for the texture
func NewTexture(
	halTexture hal.Texture,
	device *Device,
	format gputypes.TextureFormat,
	dimension gputypes.TextureDimension,
	usage gputypes.TextureUsage,
	size gputypes.Extent3D,
	mipLevelCount uint32,
	sampleCount uint32,
	label string,
) *Texture {
	t := &Texture{
		raw:           NewSnatchable(halTexture),
		device:        device,
		format:        format,
		dimension:     dimension,
		usage:         usage,
		size:          size,
		mipLevelCount: mipLevelCount,
		sampleCount:   sampleCount,
		label:         label,
		trackingData:  NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(t)), "Texture") //nolint:gosec // debug tracking uses pointer as unique ID
	return t
}

// Raw returns the underlying HAL texture if it hasn't been snatched.
func (t *Texture) Raw(guard *SnatchGuard) hal.Texture {
	if t.raw == nil {
		return nil
	}
	p := t.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// TextureView represents a view into a texture.
type TextureView struct {
	// HAL is the underlying HAL texture view handle.
	// Set by the public API layer when creating texture views with real HAL backends.
	HAL hal.TextureView
}

// Sampler represents a texture sampler with HAL integration.
//
// Sampler wraps a HAL sampler handle. Samplers are immutable after creation
// and have no mutable state beyond their HAL handle.
type Sampler struct {
	// raw is the HAL sampler handle wrapped for safe destruction.
	raw *Snatchable[hal.Sampler]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the sampler.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this sampler (Phase 2).
	Ref *ResourceRef
}

// NewSampler creates a core Sampler wrapping a HAL sampler.
//
// Parameters:
//   - halSampler: The HAL sampler to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the sampler
func NewSampler(
	halSampler hal.Sampler,
	device *Device,
	label string,
) *Sampler {
	s := &Sampler{
		raw:          NewSnatchable(halSampler),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(s)), "Sampler") //nolint:gosec // debug tracking uses pointer as unique ID
	return s
}

// Raw returns the underlying HAL sampler if it hasn't been snatched.
func (s *Sampler) Raw(guard *SnatchGuard) hal.Sampler {
	if s.raw == nil {
		return nil
	}
	p := s.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// BindGroupLayout represents the layout of a bind group with HAL integration.
//
// BindGroupLayout wraps a HAL bind group layout handle and stores the layout entries.
type BindGroupLayout struct {
	// raw is the HAL bind group layout handle wrapped for safe destruction.
	raw *Snatchable[hal.BindGroupLayout]

	// device is a pointer to the parent Device.
	device *Device

	// entries are the binding entries in this layout.
	entries []gputypes.BindGroupLayoutEntry

	// label is a debug label for the bind group layout.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this bind group layout (Phase 2).
	Ref *ResourceRef
}

// NewBindGroupLayout creates a core BindGroupLayout wrapping a HAL bind group layout.
//
// Parameters:
//   - halLayout: The HAL bind group layout to wrap (ownership transferred)
//   - device: The parent device
//   - entries: The binding entries in this layout
//   - label: Debug label for the bind group layout
func NewBindGroupLayout(
	halLayout hal.BindGroupLayout,
	device *Device,
	entries []gputypes.BindGroupLayoutEntry,
	label string,
) *BindGroupLayout {
	bgl := &BindGroupLayout{
		raw:          NewSnatchable(halLayout),
		device:       device,
		entries:      entries,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(bgl)), "BindGroupLayout") //nolint:gosec // debug tracking uses pointer as unique ID
	return bgl
}

// Raw returns the underlying HAL bind group layout if it hasn't been snatched.
func (bgl *BindGroupLayout) Raw(guard *SnatchGuard) hal.BindGroupLayout {
	if bgl.raw == nil {
		return nil
	}
	p := bgl.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// PipelineLayout represents the layout of a pipeline with HAL integration.
//
// PipelineLayout wraps a HAL pipeline layout handle and stores the bind group
// layout count. It does not store pointers to BindGroupLayout to avoid
// circular references.
type PipelineLayout struct {
	// raw is the HAL pipeline layout handle wrapped for safe destruction.
	raw *Snatchable[hal.PipelineLayout]

	// device is a pointer to the parent Device.
	device *Device

	// bindGroupLayoutCount is the number of bind group layouts in this pipeline layout.
	bindGroupLayoutCount int

	// label is a debug label for the pipeline layout.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this pipeline layout (Phase 2).
	Ref *ResourceRef
}

// NewPipelineLayout creates a core PipelineLayout wrapping a HAL pipeline layout.
//
// Parameters:
//   - halLayout: The HAL pipeline layout to wrap (ownership transferred)
//   - device: The parent device
//   - bindGroupLayoutCount: Number of bind group layouts
//   - label: Debug label for the pipeline layout
func NewPipelineLayout(
	halLayout hal.PipelineLayout,
	device *Device,
	bindGroupLayoutCount int,
	label string,
) *PipelineLayout {
	pl := &PipelineLayout{
		raw:                  NewSnatchable(halLayout),
		device:               device,
		bindGroupLayoutCount: bindGroupLayoutCount,
		label:                label,
		trackingData:         NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(pl)), "PipelineLayout") //nolint:gosec // debug tracking uses pointer as unique ID
	return pl
}

// Raw returns the underlying HAL pipeline layout if it hasn't been snatched.
func (pl *PipelineLayout) Raw(guard *SnatchGuard) hal.PipelineLayout {
	if pl.raw == nil {
		return nil
	}
	p := pl.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// BindGroup represents a collection of resources bound together with HAL integration.
//
// BindGroup wraps a HAL bind group handle. Resource references are not stored
// yet to keep the implementation simple — that is future work.
type BindGroup struct {
	// raw is the HAL bind group handle wrapped for safe destruction.
	raw *Snatchable[hal.BindGroup]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the bind group.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this bind group (Phase 2).
	Ref *ResourceRef
}

// NewBindGroup creates a core BindGroup wrapping a HAL bind group.
//
// Parameters:
//   - halGroup: The HAL bind group to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the bind group
func NewBindGroup(
	halGroup hal.BindGroup,
	device *Device,
	label string,
) *BindGroup {
	bg := &BindGroup{
		raw:          NewSnatchable(halGroup),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(bg)), "BindGroup") //nolint:gosec // debug tracking uses pointer as unique ID
	return bg
}

// Raw returns the underlying HAL bind group if it hasn't been snatched.
func (bg *BindGroup) Raw(guard *SnatchGuard) hal.BindGroup {
	if bg.raw == nil {
		return nil
	}
	p := bg.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// ShaderModule represents a compiled shader module with HAL integration.
//
// ShaderModule wraps a HAL shader module handle.
type ShaderModule struct {
	// raw is the HAL shader module handle wrapped for safe destruction.
	raw *Snatchable[hal.ShaderModule]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the shader module.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this shader module (Phase 2).
	Ref *ResourceRef
}

// NewShaderModule creates a core ShaderModule wrapping a HAL shader module.
//
// Parameters:
//   - halModule: The HAL shader module to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the shader module
func NewShaderModule(
	halModule hal.ShaderModule,
	device *Device,
	label string,
) *ShaderModule {
	sm := &ShaderModule{
		raw:          NewSnatchable(halModule),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(sm)), "ShaderModule") //nolint:gosec // debug tracking uses pointer as unique ID
	return sm
}

// Raw returns the underlying HAL shader module if it hasn't been snatched.
func (sm *ShaderModule) Raw(guard *SnatchGuard) hal.ShaderModule {
	if sm.raw == nil {
		return nil
	}
	p := sm.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// RenderPipeline represents a render pipeline with HAL integration.
//
// RenderPipeline wraps a HAL render pipeline handle.
type RenderPipeline struct {
	// raw is the HAL render pipeline handle wrapped for safe destruction.
	raw *Snatchable[hal.RenderPipeline]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the render pipeline.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this render pipeline (Phase 2).
	Ref *ResourceRef
}

// NewRenderPipeline creates a core RenderPipeline wrapping a HAL render pipeline.
//
// Parameters:
//   - halPipeline: The HAL render pipeline to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the render pipeline
func NewRenderPipeline(
	halPipeline hal.RenderPipeline,
	device *Device,
	label string,
) *RenderPipeline {
	rp := &RenderPipeline{
		raw:          NewSnatchable(halPipeline),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(rp)), "RenderPipeline") //nolint:gosec // debug tracking uses pointer as unique ID
	return rp
}

// Raw returns the underlying HAL render pipeline if it hasn't been snatched.
func (rp *RenderPipeline) Raw(guard *SnatchGuard) hal.RenderPipeline {
	if rp.raw == nil {
		return nil
	}
	p := rp.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// ComputePipeline represents a compute pipeline with HAL integration.
//
// ComputePipeline wraps a HAL compute pipeline handle.
type ComputePipeline struct {
	// raw is the HAL compute pipeline handle wrapped for safe destruction.
	raw *Snatchable[hal.ComputePipeline]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the compute pipeline.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this compute pipeline (Phase 2).
	Ref *ResourceRef
}

// NewComputePipeline creates a core ComputePipeline wrapping a HAL compute pipeline.
//
// Parameters:
//   - halPipeline: The HAL compute pipeline to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the compute pipeline
func NewComputePipeline(
	halPipeline hal.ComputePipeline,
	device *Device,
	label string,
) *ComputePipeline {
	cp := &ComputePipeline{
		raw:          NewSnatchable(halPipeline),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(cp)), "ComputePipeline") //nolint:gosec // debug tracking uses pointer as unique ID
	return cp
}

// Raw returns the underlying HAL compute pipeline if it hasn't been snatched.
func (cp *ComputePipeline) Raw(guard *SnatchGuard) hal.ComputePipeline {
	if cp.raw == nil {
		return nil
	}
	p := cp.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// CommandEncoderPassState represents the current state of a command encoder
// with respect to pass lifecycle.
type CommandEncoderPassState int

const (
	// CommandEncoderPassStateRecording means the encoder is recording commands
	// outside of any pass.
	CommandEncoderPassStateRecording CommandEncoderPassState = iota

	// CommandEncoderPassStateInRenderPass means the encoder is inside a render pass.
	CommandEncoderPassStateInRenderPass

	// CommandEncoderPassStateInComputePass means the encoder is inside a compute pass.
	CommandEncoderPassStateInComputePass

	// CommandEncoderPassStateFinished means encoding is complete.
	CommandEncoderPassStateFinished

	// CommandEncoderPassStateError means the encoder encountered an error.
	CommandEncoderPassStateError
)

// String returns a human-readable representation of the pass state.
func (s CommandEncoderPassState) String() string {
	switch s {
	case CommandEncoderPassStateRecording:
		return "Recording"
	case CommandEncoderPassStateInRenderPass:
		return "InRenderPass"
	case CommandEncoderPassStateInComputePass:
		return "InComputePass"
	case CommandEncoderPassStateFinished:
		return "Finished"
	case CommandEncoderPassStateError:
		return "Error"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// CommandEncoder represents a command encoder with HAL integration.
//
// CommandEncoder wraps a HAL command encoder handle and tracks the encoder's
// lifecycle state. The state machine ensures commands are recorded in the
// correct order: passes must be opened and closed properly, and encoding
// must be finished before the resulting command buffer can be submitted.
//
// State transitions:
//
//	Recording     -> BeginRenderPass()  -> InRenderPass
//	Recording     -> BeginComputePass() -> InComputePass
//	InRenderPass  -> EndRenderPass()    -> Recording
//	InComputePass -> EndComputePass()   -> Recording
//	Recording     -> Finish()           -> Finished
//	Any           -> RecordError()      -> Error
type CommandEncoder struct {
	// raw is the HAL command encoder handle.
	// Not wrapped in Snatchable — state machine lifecycle is managed by CORE-003.
	raw hal.CommandEncoder

	// device is a pointer to the parent Device.
	device *Device

	// passState is the current pass lifecycle state.
	passState CommandEncoderPassState

	// label is a debug label for the command encoder.
	label string

	// passDepth tracks nesting depth (should never exceed 1).
	passDepth int

	// errorMessage holds the first error encountered.
	errorMessage string
}

// NewCommandEncoder creates a core CommandEncoder wrapping a HAL command encoder.
//
// The encoder starts in the Recording state, ready to record commands.
//
// Parameters:
//   - halEncoder: The HAL command encoder to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the command encoder
func NewCommandEncoder(
	halEncoder hal.CommandEncoder,
	device *Device,
	label string,
) *CommandEncoder {
	ce := &CommandEncoder{
		raw:       halEncoder,
		device:    device,
		passState: CommandEncoderPassStateRecording,
		label:     label,
	}
	trackResource(uintptr(unsafe.Pointer(ce)), "CommandEncoder") //nolint:gosec // debug tracking uses pointer as unique ID
	return ce
}

// RawEncoder returns the underlying HAL command encoder.
func (ce *CommandEncoder) RawEncoder() hal.CommandEncoder {
	return ce.raw
}

// CommandBufferSubmitState represents the submission state of a command buffer.
type CommandBufferSubmitState int

const (
	// CommandBufferSubmitStateAvailable means the buffer is ready for submission.
	CommandBufferSubmitStateAvailable CommandBufferSubmitState = iota

	// CommandBufferSubmitStateSubmitted means the buffer has been submitted to a queue.
	CommandBufferSubmitStateSubmitted
)

// CommandBuffer represents a recorded command buffer with HAL integration.
//
// CommandBuffer wraps a HAL command buffer handle. Command buffers are
// immutable after encoding and can be submitted to a queue exactly once.
type CommandBuffer struct {
	// raw is the HAL command buffer handle wrapped for safe destruction.
	raw *Snatchable[hal.CommandBuffer]

	// device is a pointer to the parent Device.
	device *Device

	// label is a debug label for the command buffer.
	label string

	// submitState tracks whether the buffer has been submitted.
	submitState CommandBufferSubmitState

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData
}

// NewCommandBuffer creates a core CommandBuffer wrapping a HAL command buffer.
//
// Parameters:
//   - halBuffer: The HAL command buffer to wrap (ownership transferred)
//   - device: The parent device
//   - label: Debug label for the command buffer
func NewCommandBuffer(
	halBuffer hal.CommandBuffer,
	device *Device,
	label string,
) *CommandBuffer {
	cb := &CommandBuffer{
		raw:          NewSnatchable(halBuffer),
		device:       device,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(cb)), "CommandBuffer") //nolint:gosec // debug tracking uses pointer as unique ID
	return cb
}

// Raw returns the underlying HAL command buffer if it hasn't been snatched.
func (cb *CommandBuffer) Raw(guard *SnatchGuard) hal.CommandBuffer {
	if cb.raw == nil {
		return nil
	}
	p := cb.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// QuerySet represents a set of queries with HAL integration.
//
// QuerySet wraps a HAL query set handle and stores query set properties.
type QuerySet struct {
	// raw is the HAL query set handle wrapped for safe destruction.
	raw *Snatchable[hal.QuerySet]

	// device is a pointer to the parent Device.
	device *Device

	// queryType is the type of queries in this set.
	queryType hal.QueryType

	// count is the number of queries in the set.
	count uint32

	// label is a debug label for the query set.
	label string

	// trackingData holds per-resource tracking information.
	trackingData *TrackingData

	// Ref is the GPU-aware reference counter for this query set (Phase 2).
	Ref *ResourceRef
}

// NewQuerySet creates a core QuerySet wrapping a HAL query set.
//
// Parameters:
//   - halQuerySet: The HAL query set to wrap (ownership transferred)
//   - device: The parent device
//   - queryType: The type of queries in this set
//   - count: Number of queries in the set
//   - label: Debug label for the query set
func NewQuerySet(
	halQuerySet hal.QuerySet,
	device *Device,
	queryType hal.QueryType,
	count uint32,
	label string,
) *QuerySet {
	qs := &QuerySet{
		raw:          NewSnatchable(halQuerySet),
		device:       device,
		queryType:    queryType,
		count:        count,
		label:        label,
		trackingData: NewTrackingData(device.TrackerIndices()),
	}
	trackResource(uintptr(unsafe.Pointer(qs)), "QuerySet") //nolint:gosec // debug tracking uses pointer as unique ID
	return qs
}

// Raw returns the underlying HAL query set if it hasn't been snatched.
func (qs *QuerySet) Raw(guard *SnatchGuard) hal.QuerySet {
	if qs.raw == nil {
		return nil
	}
	p := qs.raw.Get(guard)
	if p == nil {
		return nil
	}
	return *p
}

// SurfaceState represents the lifecycle state of a surface.
type SurfaceState int

const (
	// SurfaceStateUnconfigured indicates the surface has not been configured.
	SurfaceStateUnconfigured SurfaceState = iota

	// SurfaceStateConfigured indicates the surface is configured and ready to acquire textures.
	SurfaceStateConfigured

	// SurfaceStateAcquired indicates a texture has been acquired and not yet presented or discarded.
	SurfaceStateAcquired
)

// PrepareFrameFunc is a platform hook called before acquiring a surface texture.
// It returns the current surface dimensions and whether they changed since the last call.
// If changed is true, the surface will be reconfigured with the new dimensions before acquiring.
type PrepareFrameFunc func() (width, height uint32, changed bool)

// Surface represents a rendering surface with HAL integration.
//
// Surface wraps a HAL surface handle. Unlike other resources, surfaces are
// owned by the Instance (not Device) and outlive devices, so the HAL handle
// is stored directly rather than in a Snatchable.
//
// Surface manages a state machine: Unconfigured -> Configured -> Acquired -> Configured.
// All state transitions are protected by a mutex.
type Surface struct {
	// raw is the HAL surface handle.
	// Not wrapped in Snatchable — surfaces are owned by Instance, not Device.
	raw hal.Surface

	// label is a debug label for the surface.
	label string

	// device is the configured device (nil when unconfigured).
	device *Device

	// config is the current surface configuration (nil when unconfigured).
	config *hal.SurfaceConfiguration

	// state is the current lifecycle state.
	state SurfaceState

	// acquiredTex is the currently acquired surface texture (nil when not acquired).
	acquiredTex hal.SurfaceTexture

	// prepareFrame is an optional platform hook called before acquiring a texture.
	prepareFrame PrepareFrameFunc

	// mu protects state transitions.
	mu sync.Mutex
}

// NewSurface creates a core Surface wrapping a HAL surface.
//
// The surface starts in the Unconfigured state. Call Configure() before
// acquiring textures.
//
// Parameters:
//   - halSurface: The HAL surface to wrap (ownership transferred)
//   - label: Debug label for the surface
func NewSurface(
	halSurface hal.Surface,
	label string,
) *Surface {
	s := &Surface{
		raw:   halSurface,
		label: label,
		state: SurfaceStateUnconfigured,
	}
	trackResource(uintptr(unsafe.Pointer(s)), "Surface") //nolint:gosec // debug tracking uses pointer as unique ID
	return s
}

// RawSurface returns the underlying HAL surface.
func (s *Surface) RawSurface() hal.Surface {
	return s.raw
}
