//go:build js && wasm

package wgpu

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

// PollType selects the blocking behavior of Device.Poll.
type PollType uint8

const (
	PollPoll PollType = iota
	PollWait
)

// Typed buffer mapping errors.
var (
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
