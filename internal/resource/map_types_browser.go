//go:build js && wasm

package resource

import "errors"

// MapMode selects the type of access requested for a buffer mapping.
type MapMode uint32

const (
	MapModeRead  MapMode = 1
	MapModeWrite MapMode = 2
)

// MapState reports the current mapping state of a buffer.
type MapState uint8

const (
	MapStateUnmapped MapState = iota
	MapStatePending
	MapStateMapped
	MapStateDestroyed
)

// PollType, PollPoll and PollWait live in poll_type.go, one definition for
// every build.

// Typed buffer mapping errors.
var (
	// ErrReleased is returned when operating on a released resource. This build
	// has no internal/core, so the sentinel is declared with the buffer code it
	// belongs to and the root re-exports the same value after the move, keeping
	// errors.Is working through the public API.
	ErrReleased = errors.New("wgpu: resource already released")

	ErrMapAlreadyPending = errors.New("wgpu: buffer map already pending")
	ErrMapAlreadyMapped  = errors.New("wgpu: buffer is already mapped")
	ErrMapNotMapped      = errors.New("wgpu: buffer is not mapped")
	ErrMapRangeOverlap   = errors.New("wgpu: mapped range overlaps existing")
	ErrMapRangeDetached  = errors.New("wgpu: mapped range detached (buffer unmapped)")
	ErrMapAlignment      = errors.New("wgpu: map offset/size not aligned")
	ErrMapCanceled       = errors.New("wgpu: map canceled")
	ErrBufferDestroyed   = errors.New("wgpu: buffer destroyed")
	ErrMapDeviceLost     = errors.New("wgpu: device lost during map")
	ErrMapInvalidMode    = errors.New("wgpu: buffer not created with required MAP_READ/MAP_WRITE usage")
	ErrMapRangeOverflow  = errors.New("wgpu: map range exceeds buffer size")
)
