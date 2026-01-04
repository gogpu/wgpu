package track

import (
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"
)

// BufferUses represents internal buffer usage states for tracking.
// These are more granular than types.BufferUsage for precise barrier insertion.
type BufferUses uint32

// Buffer usage flags for state tracking.
const (
	BufferUsesNone         BufferUses = 0
	BufferUsesCopySrc      BufferUses = 1 << 0  // Being read by copy operation
	BufferUsesCopyDst      BufferUses = 1 << 1  // Being written by copy operation
	BufferUsesIndex        BufferUses = 1 << 2  // Bound as index buffer
	BufferUsesVertex       BufferUses = 1 << 3  // Bound as vertex buffer
	BufferUsesUniform      BufferUses = 1 << 4  // Bound in bind group for reading
	BufferUsesStorageRead  BufferUses = 1 << 5  // Storage buffer read-only
	BufferUsesStorageWrite BufferUses = 1 << 6  // Storage buffer read-write
	BufferUsesIndirect     BufferUses = 1 << 7  // Indirect command buffer
	BufferUsesMapRead      BufferUses = 1 << 8  // Mapped for CPU read
	BufferUsesMapWrite     BufferUses = 1 << 9  // Mapped for CPU write
	BufferUsesQueryResolve BufferUses = 1 << 10 // Query result destination
)

// IsReadOnly returns true if the usage contains only read-only operations.
func (u BufferUses) IsReadOnly() bool {
	writeUsages := BufferUsesCopyDst | BufferUsesStorageWrite | BufferUsesMapWrite | BufferUsesQueryResolve
	return u&writeUsages == 0
}

// IsEmpty returns true if no usage flags are set.
func (u BufferUses) IsEmpty() bool {
	return u == BufferUsesNone
}

// Contains returns true if all flags in other are present in u.
func (u BufferUses) Contains(other BufferUses) bool {
	return u&other == other
}

// IsCompatible returns true if two usages can coexist without a barrier.
// Read-only usages are compatible with each other.
// Write usages require exclusive access.
func (u BufferUses) IsCompatible(other BufferUses) bool {
	// Empty is compatible with everything
	if u.IsEmpty() || other.IsEmpty() {
		return true
	}
	// Read-only usages are always compatible with each other
	if u.IsReadOnly() && other.IsReadOnly() {
		return true
	}
	// If either has write, they're only compatible if identical
	return u == other
}

// ToBufferUsage converts internal uses to types.BufferUsage for HAL.
func (u BufferUses) ToBufferUsage() types.BufferUsage {
	var result types.BufferUsage

	if u&BufferUsesCopySrc != 0 {
		result |= types.BufferUsageCopySrc
	}
	if u&BufferUsesCopyDst != 0 {
		result |= types.BufferUsageCopyDst
	}
	if u&BufferUsesIndex != 0 {
		result |= types.BufferUsageIndex
	}
	if u&BufferUsesVertex != 0 {
		result |= types.BufferUsageVertex
	}
	if u&BufferUsesUniform != 0 {
		result |= types.BufferUsageUniform
	}
	if u&(BufferUsesStorageRead|BufferUsesStorageWrite) != 0 {
		result |= types.BufferUsageStorage
	}
	if u&BufferUsesIndirect != 0 {
		result |= types.BufferUsageIndirect
	}
	if u&BufferUsesMapRead != 0 {
		result |= types.BufferUsageMapRead
	}
	if u&BufferUsesMapWrite != 0 {
		result |= types.BufferUsageMapWrite
	}
	if u&BufferUsesQueryResolve != 0 {
		result |= types.BufferUsageQueryResolve
	}

	return result
}

// BufferState holds the tracked state for a single buffer.
type BufferState struct {
	usage BufferUses
}

// Usage returns the current usage.
func (s BufferState) Usage() BufferUses {
	return s.usage
}

// BufferTracker tracks buffer usage states for a device.
// Used to validate usage transitions and generate barriers.
type BufferTracker struct {
	states   []BufferState    // States indexed by TrackerIndex
	metadata ResourceMetadata // Tracks which indices are valid
}

// NewBufferTracker creates a new buffer tracker.
func NewBufferTracker() *BufferTracker {
	return &BufferTracker{
		states:   make([]BufferState, 0, 64),
		metadata: NewResourceMetadata(),
	}
}

// InsertSingle tracks a new buffer with initial usage.
func (t *BufferTracker) InsertSingle(index TrackerIndex, usage BufferUses) {
	t.ensureSize(int(index) + 1)
	t.states[index] = BufferState{usage: usage}
	t.metadata.SetOwned(index, true)
}

// Remove stops tracking a buffer.
func (t *BufferTracker) Remove(index TrackerIndex) {
	if int(index) < len(t.states) {
		t.states[index] = BufferState{}
		t.metadata.SetOwned(index, false)
	}
}

// GetUsage returns the current usage of a buffer.
func (t *BufferTracker) GetUsage(index TrackerIndex) BufferUses {
	if int(index) < len(t.states) && t.metadata.IsOwned(index) {
		return t.states[index].usage
	}
	return BufferUsesNone
}

// SetUsage updates the usage of a tracked buffer.
func (t *BufferTracker) SetUsage(index TrackerIndex, usage BufferUses) {
	if int(index) < len(t.states) && t.metadata.IsOwned(index) {
		t.states[index].usage = usage
	}
}

// IsTracked returns true if the buffer is being tracked.
func (t *BufferTracker) IsTracked(index TrackerIndex) bool {
	return int(index) < len(t.states) && t.metadata.IsOwned(index)
}

// Size returns the number of tracked buffers.
func (t *BufferTracker) Size() int {
	return t.metadata.Count()
}

// ensureSize grows the state vector if needed.
func (t *BufferTracker) ensureSize(size int) {
	for len(t.states) < size {
		t.states = append(t.states, BufferState{})
	}
}

// Merge merges usage from scope into tracker, returning needed transitions.
// This is called during queue submit to synchronize command buffer state
// with device state.
func (t *BufferTracker) Merge(scope *BufferUsageScope) []PendingTransition {
	var transitions []PendingTransition

	for i := range scope.states {
		if i < 0 || i > int(^TrackerIndex(0)-1) {
			continue // Skip if index would overflow TrackerIndex
		}
		index := TrackerIndex(i)
		if !scope.metadata.IsOwned(index) {
			continue
		}

		newUsage := scope.states[i].usage
		oldUsage := t.GetUsage(index)

		// If buffer not tracked in device, add it
		if !t.IsTracked(index) {
			t.InsertSingle(index, newUsage)
			// No transition needed for new buffer
			continue
		}

		// Check if transition is needed
		if !oldUsage.IsCompatible(newUsage) || oldUsage != newUsage {
			transitions = append(transitions, PendingTransition{
				Index: index,
				Usage: StateTransition{
					From: oldUsage,
					To:   newUsage,
				},
			})
			t.states[index].usage = newUsage
		}
	}

	return transitions
}

// BufferUsageScope tracks buffer usage within a command buffer or pass.
// Each command buffer has its own scope that gets merged into the device
// tracker on submit.
type BufferUsageScope struct {
	states   []BufferState
	metadata ResourceMetadata
}

// NewBufferUsageScope creates a new usage scope.
func NewBufferUsageScope() *BufferUsageScope {
	return &BufferUsageScope{
		states:   make([]BufferState, 0, 32),
		metadata: NewResourceMetadata(),
	}
}

// SetUsage sets the usage for a buffer in this scope.
// Returns error if the buffer already has an incompatible usage.
func (s *BufferUsageScope) SetUsage(index TrackerIndex, usage BufferUses) error {
	s.ensureSize(int(index) + 1)

	if s.metadata.IsOwned(index) {
		existing := s.states[index].usage
		if !existing.IsCompatible(usage) {
			return &UsageConflictError{
				Index:    index,
				Existing: existing,
				New:      usage,
			}
		}
		// Merge usages if compatible
		s.states[index].usage = existing | usage
	} else {
		s.states[index] = BufferState{usage: usage}
		s.metadata.SetOwned(index, true)
	}

	return nil
}

// GetUsage returns the current usage in this scope.
func (s *BufferUsageScope) GetUsage(index TrackerIndex) BufferUses {
	if int(index) < len(s.states) && s.metadata.IsOwned(index) {
		return s.states[index].usage
	}
	return BufferUsesNone
}

// IsUsed returns true if the buffer is used in this scope.
func (s *BufferUsageScope) IsUsed(index TrackerIndex) bool {
	return int(index) < len(s.states) && s.metadata.IsOwned(index)
}

// Clear resets the scope for reuse.
func (s *BufferUsageScope) Clear() {
	s.states = s.states[:0]
	s.metadata.Clear()
}

// ensureSize grows the state vector if needed.
func (s *BufferUsageScope) ensureSize(size int) {
	for len(s.states) < size {
		s.states = append(s.states, BufferState{})
	}
}

// PendingTransition represents a state transition that needs a barrier.
type PendingTransition struct {
	Index TrackerIndex
	Usage StateTransition
}

// StateTransition represents a from→to state change.
type StateTransition struct {
	From BufferUses
	To   BufferUses
}

// NeedsBarrier returns true if this transition requires a barrier.
func (t StateTransition) NeedsBarrier() bool {
	// No barrier needed if transitioning to same state
	if t.From == t.To {
		return false
	}
	// No barrier needed if both are read-only
	if t.From.IsReadOnly() && t.To.IsReadOnly() {
		return false
	}
	return true
}

// IntoHAL converts a pending transition to a HAL buffer barrier.
func (p PendingTransition) IntoHAL(buffer hal.Buffer) hal.BufferBarrier {
	return hal.BufferBarrier{
		Buffer: buffer,
		Usage: hal.BufferUsageTransition{
			OldUsage: p.Usage.From.ToBufferUsage(),
			NewUsage: p.Usage.To.ToBufferUsage(),
		},
	}
}

// UsageConflictError is returned when incompatible usages are detected.
type UsageConflictError struct {
	Index    TrackerIndex
	Existing BufferUses
	New      BufferUses
}

// Error implements the error interface.
func (e *UsageConflictError) Error() string {
	return "buffer usage conflict: incompatible usages in same scope"
}

// ResourceMetadata tracks which resources are owned/present.
type ResourceMetadata struct {
	owned []bool
	count int
}

// NewResourceMetadata creates new metadata.
func NewResourceMetadata() ResourceMetadata {
	return ResourceMetadata{
		owned: make([]bool, 0, 64),
		count: 0,
	}
}

// SetOwned marks a resource as owned/not owned.
func (m *ResourceMetadata) SetOwned(index TrackerIndex, owned bool) {
	for int(index) >= len(m.owned) {
		m.owned = append(m.owned, false)
	}

	wasOwned := m.owned[index]
	m.owned[index] = owned

	// Update count
	if owned && !wasOwned {
		m.count++
	} else if !owned && wasOwned {
		m.count--
	}
}

// IsOwned returns true if the resource is owned.
func (m *ResourceMetadata) IsOwned(index TrackerIndex) bool {
	if int(index) >= len(m.owned) {
		return false
	}
	return m.owned[index]
}

// Count returns the number of owned resources.
func (m *ResourceMetadata) Count() int {
	return m.count
}

// Clear resets the metadata.
func (m *ResourceMetadata) Clear() {
	for i := range m.owned {
		m.owned[i] = false
	}
	m.count = 0
}
