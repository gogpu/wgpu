package raster

import (
	"sync"
)

// StencilOp specifies the operation to perform on the stencil buffer.
type StencilOp uint8

const (
	// StencilOpKeep keeps the current stencil value.
	StencilOpKeep StencilOp = iota

	// StencilOpZero sets the stencil value to zero.
	StencilOpZero

	// StencilOpReplace sets the stencil value to the reference value.
	StencilOpReplace

	// StencilOpIncrementClamp increments the stencil value, clamping to max (255).
	StencilOpIncrementClamp

	// StencilOpDecrementClamp decrements the stencil value, clamping to 0.
	StencilOpDecrementClamp

	// StencilOpInvert bitwise inverts the stencil value.
	StencilOpInvert

	// StencilOpIncrementWrap increments the stencil value, wrapping to 0 on overflow.
	StencilOpIncrementWrap

	// StencilOpDecrementWrap decrements the stencil value, wrapping to 255 on underflow.
	StencilOpDecrementWrap
)

// StencilState configures stencil testing for a render pass.
type StencilState struct {
	// Enabled indicates whether stencil testing is active.
	Enabled bool

	// ReadMask is AND-ed with the stencil value before comparison.
	ReadMask uint8

	// WriteMask is AND-ed with the value before writing to the buffer.
	WriteMask uint8

	// Compare is the comparison function for the stencil test.
	Compare CompareFunc

	// FailOp is applied when the stencil test fails.
	FailOp StencilOp

	// DepthFailOp is applied when stencil passes but depth test fails.
	DepthFailOp StencilOp

	// PassOp is applied when both stencil and depth tests pass.
	PassOp StencilOp

	// Reference is the reference value for stencil comparison and StencilOpReplace.
	Reference uint8
}

// DefaultStencilState returns a default stencil state with testing disabled.
func DefaultStencilState() StencilState {
	return StencilState{
		Enabled:     false,
		ReadMask:    0xFF,
		WriteMask:   0xFF,
		Compare:     CompareAlways,
		FailOp:      StencilOpKeep,
		DepthFailOp: StencilOpKeep,
		PassOp:      StencilOpKeep,
		Reference:   0,
	}
}

// StencilBuffer stores stencil values for the software rasterizer.
// Stencil values are uint8 in the range [0, 255].
// The buffer is thread-safe for concurrent access.
type StencilBuffer struct {
	mu     sync.RWMutex
	data   []uint8
	width  int
	height int
}

// NewStencilBuffer creates a new stencil buffer with the given dimensions.
// All values are initialized to 0.
func NewStencilBuffer(width, height int) *StencilBuffer {
	return &StencilBuffer{
		data:   make([]uint8, width*height),
		width:  width,
		height: height,
	}
}

// Width returns the width of the stencil buffer.
func (s *StencilBuffer) Width() int {
	return s.width
}

// Height returns the height of the stencil buffer.
func (s *StencilBuffer) Height() int {
	return s.height
}

// Clear fills the entire stencil buffer with the given value.
func (s *StencilBuffer) Clear(value uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data {
		s.data[i] = value
	}
}

// Get returns the stencil value at pixel (x, y).
// Returns 0 if coordinates are out of bounds.
func (s *StencilBuffer) Get(x, y int) uint8 {
	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.data[y*s.width+x]
}

// Set writes a stencil value at pixel (x, y).
// Out-of-bounds coordinates are silently ignored.
func (s *StencilBuffer) Set(x, y int, value uint8) {
	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[y*s.width+x] = value
}

// Test performs a stencil test at pixel (x, y).
// Returns true if the test passes (fragment should proceed to depth test).
// This does NOT modify the stencil buffer.
func (s *StencilBuffer) Test(x, y int, state StencilState) bool {
	if !state.Enabled {
		return true
	}

	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return false
	}

	s.mu.RLock()
	storedValue := s.data[y*s.width+x]
	s.mu.RUnlock()

	// Apply read mask
	maskedStored := storedValue & state.ReadMask
	maskedRef := state.Reference & state.ReadMask

	return compareStencil(maskedRef, maskedStored, state.Compare)
}

// Apply applies a stencil operation to the buffer at pixel (x, y).
// This modifies the stencil buffer based on the operation.
func (s *StencilBuffer) Apply(x, y int, op StencilOp, state StencilState) {
	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := y*s.width + x
	current := s.data[idx]
	newValue := applyStencilOp(current, op, state.Reference, state.WriteMask)
	s.data[idx] = newValue
}

// TestAndApply performs the stencil test and applies the appropriate operation.
// Returns true if the stencil test passed.
//
// Operations applied based on test results:
//   - Stencil test fails: FailOp
//   - Stencil passes, depth fails: DepthFailOp
//   - Both pass: PassOp
func (s *StencilBuffer) TestAndApply(x, y int, depthPassed bool, state StencilState) bool {
	if !state.Enabled {
		return true
	}

	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := y*s.width + x
	storedValue := s.data[idx]

	// Apply read mask for comparison
	maskedStored := storedValue & state.ReadMask
	maskedRef := state.Reference & state.ReadMask

	stencilPassed := compareStencil(maskedRef, maskedStored, state.Compare)

	// Determine which operation to apply
	var op StencilOp
	switch {
	case !stencilPassed:
		op = state.FailOp
	case !depthPassed:
		op = state.DepthFailOp
	default:
		op = state.PassOp
	}

	// Apply the operation
	newValue := applyStencilOp(storedValue, op, state.Reference, state.WriteMask)
	s.data[idx] = newValue

	return stencilPassed
}

// Resize resizes the stencil buffer and clears all contents.
func (s *StencilBuffer) Resize(width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.width = width
	s.height = height
	s.data = make([]uint8, width*height)
}

// GetData returns a copy of the raw stencil buffer data.
// The data is in row-major order (y * width + x).
func (s *StencilBuffer) GetData() []uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]uint8, len(s.data))
	copy(result, s.data)
	return result
}

// applyStencilOp applies a single stencil operation to a value.
func applyStencilOp(current uint8, op StencilOp, ref, writeMask uint8) uint8 {
	var newValue uint8

	switch op {
	case StencilOpKeep:
		return current

	case StencilOpZero:
		newValue = 0

	case StencilOpReplace:
		newValue = ref

	case StencilOpIncrementClamp:
		if current < 255 {
			newValue = current + 1
		} else {
			newValue = 255
		}

	case StencilOpDecrementClamp:
		if current > 0 {
			newValue = current - 1
		} else {
			newValue = 0
		}

	case StencilOpInvert:
		newValue = ^current

	case StencilOpIncrementWrap:
		newValue = current + 1 // Wraps naturally due to uint8

	case StencilOpDecrementWrap:
		newValue = current - 1 // Wraps naturally due to uint8

	default:
		return current
	}

	// Apply write mask: preserve bits where mask is 0, write where mask is 1
	return (current &^ writeMask) | (newValue & writeMask)
}

// compareStencil compares reference value against stored value using the specified function.
func compareStencil(ref, stored uint8, compare CompareFunc) bool {
	switch compare {
	case CompareNever:
		return false
	case CompareLess:
		return ref < stored
	case CompareEqual:
		return ref == stored
	case CompareLessEqual:
		return ref <= stored
	case CompareGreater:
		return ref > stored
	case CompareNotEqual:
		return ref != stored
	case CompareGreaterEqual:
		return ref >= stored
	case CompareAlways:
		return true
	default:
		return false
	}
}
