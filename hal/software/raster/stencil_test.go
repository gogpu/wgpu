package raster

import (
	"testing"
)

// =============================================================================
// StencilBuffer Basic Tests
// =============================================================================

func TestStencilBufferBasic(t *testing.T) {
	sb := NewStencilBuffer(100, 100)

	if sb.Width() != 100 {
		t.Errorf("Width() = %d, want 100", sb.Width())
	}
	if sb.Height() != 100 {
		t.Errorf("Height() = %d, want 100", sb.Height())
	}

	// All values should be initialized to 0
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := sb.Get(x, y); got != 0 {
				t.Errorf("Initial value at (%d, %d) = %v, want 0", x, y, got)
				return
			}
		}
	}
}

func TestStencilBufferClear(t *testing.T) {
	sb := NewStencilBuffer(10, 10)

	sb.Clear(128)

	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if got := sb.Get(x, y); got != 128 {
				t.Errorf("After Clear(128), value at (%d, %d) = %v, want 128", x, y, got)
			}
		}
	}
}

func TestStencilBufferGetSet(t *testing.T) {
	sb := NewStencilBuffer(10, 10)

	sb.Set(5, 5, 42)
	if got := sb.Get(5, 5); got != 42 {
		t.Errorf("Get(5, 5) = %v, want 42", got)
	}

	// Out of bounds should return 0
	if got := sb.Get(-1, 0); got != 0 {
		t.Errorf("Get(-1, 0) = %v, want 0 for out of bounds", got)
	}
	if got := sb.Get(100, 100); got != 0 {
		t.Errorf("Get(100, 100) = %v, want 0 for out of bounds", got)
	}
}

func TestStencilBufferResize(t *testing.T) {
	sb := NewStencilBuffer(100, 100)
	sb.Set(50, 50, 42)

	sb.Resize(50, 50)

	if sb.Width() != 50 || sb.Height() != 50 {
		t.Errorf("After Resize, dimensions = (%d, %d), want (50, 50)", sb.Width(), sb.Height())
	}

	// Value should be cleared
	if got := sb.Get(25, 25); got != 0 {
		t.Errorf("After Resize, value should be 0, got %v", got)
	}
}

// =============================================================================
// StencilOp Tests
// =============================================================================

func TestStencilOpKeep(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 42)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
		Reference: 100,
	}

	sb.Apply(5, 5, StencilOpKeep, state)

	if got := sb.Get(5, 5); got != 42 {
		t.Errorf("After StencilOpKeep, value = %v, want 42", got)
	}
}

func TestStencilOpZero(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 42)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
		Reference: 100,
	}

	sb.Apply(5, 5, StencilOpZero, state)

	if got := sb.Get(5, 5); got != 0 {
		t.Errorf("After StencilOpZero, value = %v, want 0", got)
	}
}

func TestStencilOpReplace(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 42)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
		Reference: 100,
	}

	sb.Apply(5, 5, StencilOpReplace, state)

	if got := sb.Get(5, 5); got != 100 {
		t.Errorf("After StencilOpReplace, value = %v, want 100", got)
	}
}

func TestStencilOpIncrementClamp(t *testing.T) {
	sb := NewStencilBuffer(10, 10)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}

	// Test normal increment
	sb.Set(5, 5, 42)
	sb.Apply(5, 5, StencilOpIncrementClamp, state)
	if got := sb.Get(5, 5); got != 43 {
		t.Errorf("After StencilOpIncrementClamp from 42, value = %v, want 43", got)
	}

	// Test clamping at 255
	sb.Set(5, 5, 255)
	sb.Apply(5, 5, StencilOpIncrementClamp, state)
	if got := sb.Get(5, 5); got != 255 {
		t.Errorf("After StencilOpIncrementClamp from 255, value = %v, want 255", got)
	}
}

func TestStencilOpDecrementClamp(t *testing.T) {
	sb := NewStencilBuffer(10, 10)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}

	// Test normal decrement
	sb.Set(5, 5, 42)
	sb.Apply(5, 5, StencilOpDecrementClamp, state)
	if got := sb.Get(5, 5); got != 41 {
		t.Errorf("After StencilOpDecrementClamp from 42, value = %v, want 41", got)
	}

	// Test clamping at 0
	sb.Set(5, 5, 0)
	sb.Apply(5, 5, StencilOpDecrementClamp, state)
	if got := sb.Get(5, 5); got != 0 {
		t.Errorf("After StencilOpDecrementClamp from 0, value = %v, want 0", got)
	}
}

func TestStencilOpInvert(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 0xAA) // Binary: 10101010

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}

	sb.Apply(5, 5, StencilOpInvert, state)

	if got := sb.Get(5, 5); got != 0x55 { // Binary: 01010101
		t.Errorf("After StencilOpInvert from 0xAA, value = 0x%X, want 0x55", got)
	}
}

func TestStencilOpIncrementWrap(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 255)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}

	sb.Apply(5, 5, StencilOpIncrementWrap, state)

	if got := sb.Get(5, 5); got != 0 {
		t.Errorf("After StencilOpIncrementWrap from 255, value = %v, want 0", got)
	}
}

func TestStencilOpDecrementWrap(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 0)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}

	sb.Apply(5, 5, StencilOpDecrementWrap, state)

	if got := sb.Get(5, 5); got != 255 {
		t.Errorf("After StencilOpDecrementWrap from 0, value = %v, want 255", got)
	}
}

// =============================================================================
// Stencil Test (Compare) Tests
// =============================================================================

func TestStencilCompareFunctions(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 50)

	tests := []struct {
		name      string
		compare   CompareFunc
		reference uint8
		want      bool
	}{
		{"never", CompareNever, 50, false},
		{"always", CompareAlways, 0, true},
		{"less_pass", CompareLess, 25, true},  // 25 < 50
		{"less_fail", CompareLess, 75, false}, // 75 !< 50
		{"less_equal_pass", CompareLessEqual, 50, true},
		{"less_equal_fail", CompareLessEqual, 75, false},
		{"greater_pass", CompareGreater, 75, true},  // 75 > 50
		{"greater_fail", CompareGreater, 25, false}, // 25 !> 50
		{"greater_equal_pass", CompareGreaterEqual, 50, true},
		{"greater_equal_fail", CompareGreaterEqual, 25, false},
		{"equal_pass", CompareEqual, 50, true},
		{"equal_fail", CompareEqual, 51, false},
		{"not_equal_pass", CompareNotEqual, 51, true},
		{"not_equal_fail", CompareNotEqual, 50, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := StencilState{
				Enabled:   true,
				ReadMask:  0xFF,
				WriteMask: 0xFF,
				Compare:   tt.compare,
				Reference: tt.reference,
			}

			got := sb.Test(5, 5, state)
			if got != tt.want {
				t.Errorf("Test() with compare=%v, ref=%d = %v, want %v",
					tt.compare, tt.reference, got, tt.want)
			}
		})
	}
}

// =============================================================================
// TestAndApply Tests
// =============================================================================

func TestStencilTestAndApply(t *testing.T) {
	tests := []struct {
		name        string
		initial     uint8
		reference   uint8
		compare     CompareFunc
		depthPassed bool
		failOp      StencilOp
		depthFailOp StencilOp
		passOp      StencilOp
		wantPass    bool
		wantValue   uint8
	}{
		{
			name:        "stencil_pass_depth_pass",
			initial:     50,
			reference:   50,
			compare:     CompareEqual,
			depthPassed: true,
			failOp:      StencilOpKeep,
			depthFailOp: StencilOpZero,
			passOp:      StencilOpReplace,
			wantPass:    true,
			wantValue:   50, // PassOp = Replace with ref=50
		},
		{
			name:        "stencil_pass_depth_fail",
			initial:     50,
			reference:   50,
			compare:     CompareEqual,
			depthPassed: false,
			failOp:      StencilOpKeep,
			depthFailOp: StencilOpZero,
			passOp:      StencilOpReplace,
			wantPass:    true,
			wantValue:   0, // DepthFailOp = Zero
		},
		{
			name:        "stencil_fail",
			initial:     50,
			reference:   25,
			compare:     CompareEqual, // 25 != 50, so fail
			depthPassed: true,
			failOp:      StencilOpIncrementClamp,
			depthFailOp: StencilOpZero,
			passOp:      StencilOpReplace,
			wantPass:    false,
			wantValue:   51, // FailOp = IncrementClamp
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := NewStencilBuffer(10, 10)
			sb.Set(5, 5, tt.initial)

			state := StencilState{
				Enabled:     true,
				ReadMask:    0xFF,
				WriteMask:   0xFF,
				Compare:     tt.compare,
				Reference:   tt.reference,
				FailOp:      tt.failOp,
				DepthFailOp: tt.depthFailOp,
				PassOp:      tt.passOp,
			}

			gotPass := sb.TestAndApply(5, 5, tt.depthPassed, state)
			gotValue := sb.Get(5, 5)

			if gotPass != tt.wantPass {
				t.Errorf("TestAndApply() pass = %v, want %v", gotPass, tt.wantPass)
			}
			if gotValue != tt.wantValue {
				t.Errorf("TestAndApply() value = %v, want %v", gotValue, tt.wantValue)
			}
		})
	}
}

// =============================================================================
// Write Mask Tests
// =============================================================================

func TestStencilWriteMask(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 0xF0) // Binary: 11110000

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0x0F, // Only write low 4 bits
		Reference: 0x05,
	}

	// Replace should only affect low 4 bits
	sb.Apply(5, 5, StencilOpReplace, state)

	// Expected: high 4 bits preserved (0xF0 & 0xF0 = 0xF0) + low 4 bits from ref (0x05 & 0x0F = 0x05)
	// Result: 0xF5
	if got := sb.Get(5, 5); got != 0xF5 {
		t.Errorf("After StencilOpReplace with mask 0x0F, value = 0x%X, want 0xF5", got)
	}
}

func TestStencilReadMask(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 0xF5) // Binary: 11110101

	// Test with read mask that only checks low 4 bits
	state := StencilState{
		Enabled:   true,
		ReadMask:  0x0F, // Only read low 4 bits
		WriteMask: 0xFF,
		Compare:   CompareEqual,
		Reference: 0x05, // Low 4 bits match (0xF5 & 0x0F = 0x05)
	}

	if !sb.Test(5, 5, state) {
		t.Error("Test should pass when comparing masked values (0x05 == 0x05)")
	}

	// Now test with full reference that doesn't match
	state.Reference = 0xF5
	// With read mask 0x0F: stored = 0x05, ref = 0x05 -> should still pass
	if !sb.Test(5, 5, state) {
		t.Error("Test should pass when masked values match")
	}
}

// =============================================================================
// Disabled Stencil Tests
// =============================================================================

func TestStencilDisabled(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(5, 5, 50)

	state := StencilState{
		Enabled: false,
	}

	// Test should always pass when disabled
	if !sb.Test(5, 5, state) {
		t.Error("Test should always pass when stencil is disabled")
	}

	// TestAndApply should pass and not modify buffer
	if !sb.TestAndApply(5, 5, true, state) {
		t.Error("TestAndApply should pass when stencil is disabled")
	}
}

// =============================================================================
// DefaultStencilState Tests
// =============================================================================

func TestDefaultStencilState(t *testing.T) {
	state := DefaultStencilState()

	if state.Enabled {
		t.Error("Default state should have Enabled = false")
	}
	if state.ReadMask != 0xFF {
		t.Errorf("Default ReadMask = 0x%X, want 0xFF", state.ReadMask)
	}
	if state.WriteMask != 0xFF {
		t.Errorf("Default WriteMask = 0x%X, want 0xFF", state.WriteMask)
	}
	if state.Compare != CompareAlways {
		t.Errorf("Default Compare = %v, want CompareAlways", state.Compare)
	}
	if state.FailOp != StencilOpKeep {
		t.Errorf("Default FailOp = %v, want StencilOpKeep", state.FailOp)
	}
	if state.DepthFailOp != StencilOpKeep {
		t.Errorf("Default DepthFailOp = %v, want StencilOpKeep", state.DepthFailOp)
	}
	if state.PassOp != StencilOpKeep {
		t.Errorf("Default PassOp = %v, want StencilOpKeep", state.PassOp)
	}
	if state.Reference != 0 {
		t.Errorf("Default Reference = %v, want 0", state.Reference)
	}
}

// =============================================================================
// GetData Tests
// =============================================================================

func TestStencilBufferGetData(t *testing.T) {
	sb := NewStencilBuffer(10, 10)
	sb.Set(0, 0, 1)
	sb.Set(5, 5, 2)
	sb.Set(9, 9, 3)

	data := sb.GetData()

	if len(data) != 100 {
		t.Errorf("GetData() len = %d, want 100", len(data))
	}

	if data[0] != 1 {
		t.Errorf("GetData()[0] = %v, want 1", data[0])
	}
	if data[55] != 2 { // 5*10 + 5 = 55
		t.Errorf("GetData()[55] = %v, want 2", data[55])
	}
	if data[99] != 3 { // 9*10 + 9 = 99
		t.Errorf("GetData()[99] = %v, want 3", data[99])
	}

	// Ensure it's a copy
	data[0] = 255
	if sb.Get(0, 0) != 1 {
		t.Error("GetData should return a copy, not the original buffer")
	}
}

// =============================================================================
// Pipeline Integration Tests
// =============================================================================

func TestPipelineStencilBasic(t *testing.T) {
	p := NewPipeline(100, 100)
	p.Clear(0, 0, 0, 1)

	// Create and set stencil buffer
	stencilBuf := NewStencilBuffer(100, 100)
	p.SetStencilBuffer(stencilBuf)

	// Get it back
	got := p.GetStencilBuffer()
	if got != stencilBuf {
		t.Error("GetStencilBuffer() didn't return the set buffer")
	}

	// Set stencil state
	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
		Compare:   CompareAlways,
		PassOp:    StencilOpReplace,
		Reference: 1,
	}
	p.SetStencilState(state)

	// Get it back
	gotState := p.GetStencilState()
	if gotState.Enabled != state.Enabled || gotState.Reference != state.Reference {
		t.Error("GetStencilState() didn't return the set state")
	}

	// Clear stencil
	p.ClearStencil(0)
	if stencilBuf.Get(50, 50) != 0 {
		t.Error("ClearStencil() didn't clear the buffer")
	}
}

func TestPipelineScissor(t *testing.T) {
	p := NewPipeline(100, 100)

	// Initially no scissor
	if p.GetScissor() != nil {
		t.Error("Initially scissor should be nil")
	}

	// Set scissor
	rect := &Rect{X: 10, Y: 10, Width: 50, Height: 50}
	p.SetScissor(rect)

	got := p.GetScissor()
	if got == nil {
		t.Error("GetScissor() should not be nil after SetScissor")
		return
	}
	if got.X != 10 || got.Y != 10 || got.Width != 50 || got.Height != 50 {
		t.Errorf("GetScissor() = %+v, want (10, 10, 50, 50)", got)
	}

	// Ensure it's a copy
	rect.X = 999
	got2 := p.GetScissor()
	if got2.X == 999 {
		t.Error("SetScissor should copy the rect")
	}

	// Disable scissor
	p.SetScissor(nil)
	if p.GetScissor() != nil {
		t.Error("GetScissor() should be nil after SetScissor(nil)")
	}
}

func TestPipelineClipping(t *testing.T) {
	p := NewPipeline(100, 100)

	// Initially disabled
	if p.IsClippingEnabled() {
		t.Error("Initially clipping should be disabled")
	}

	// Enable clipping
	p.SetClipping(true)
	if !p.IsClippingEnabled() {
		t.Error("Clipping should be enabled after SetClipping(true)")
	}

	// Disable clipping
	p.SetClipping(false)
	if p.IsClippingEnabled() {
		t.Error("Clipping should be disabled after SetClipping(false)")
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkStencilBufferTest(b *testing.B) {
	sb := NewStencilBuffer(800, 600)
	sb.Clear(128)

	state := StencilState{
		Enabled:   true,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
		Compare:   CompareLess,
		Reference: 64,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := i % 800
		y := (i / 800) % 600
		sb.Test(x, y, state)
	}
}

func BenchmarkStencilBufferTestAndApply(b *testing.B) {
	sb := NewStencilBuffer(800, 600)

	state := StencilState{
		Enabled:     true,
		ReadMask:    0xFF,
		WriteMask:   0xFF,
		Compare:     CompareLess,
		Reference:   64,
		FailOp:      StencilOpKeep,
		DepthFailOp: StencilOpZero,
		PassOp:      StencilOpIncrementClamp,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := i % 800
		y := (i / 800) % 600
		sb.TestAndApply(x, y, true, state)
	}
}

func BenchmarkStencilOpApply(b *testing.B) {
	sb := NewStencilBuffer(800, 600)

	state := StencilState{
		Enabled:   true,
		WriteMask: 0xFF,
		Reference: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := i % 800
		y := (i / 800) % 600
		sb.Apply(x, y, StencilOpIncrementClamp, state)
	}
}
