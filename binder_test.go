package wgpu

import (
	"strings"
	"testing"
)

func TestBinderReset(t *testing.T) {
	var b binder

	layout := &BindGroupLayout{}
	b.assign(0, layout)
	b.updateExpectations([]*BindGroupLayout{layout})

	b.reset()

	if b.maxSlots != 0 {
		t.Errorf("maxSlots = %d after reset, want 0", b.maxSlots)
	}
	for i := range b.assigned {
		if b.assigned[i] != nil {
			t.Errorf("assigned[%d] = %v after reset, want nil", i, b.assigned[i])
		}
	}
	for i := range b.expected {
		if b.expected[i] != nil {
			t.Errorf("expected[%d] = %v after reset, want nil", i, b.expected[i])
		}
	}
}

func TestBinderUpdateExpectations(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	if b.maxSlots != 2 {
		t.Errorf("maxSlots = %d, want 2", b.maxSlots)
	}
	if b.expected[0] != l0 {
		t.Error("expected[0] should be l0")
	}
	if b.expected[1] != l1 {
		t.Error("expected[1] should be l1")
	}
	for i := uint32(2); i < MaxBindGroups; i++ {
		if b.expected[i] != nil {
			t.Errorf("expected[%d] = %v, want nil", i, b.expected[i])
		}
	}
}

func TestBinderUpdateExpectationsClearsPrevious(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	// Switch to a pipeline with only 1 bind group.
	l2 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l2})

	if b.maxSlots != 1 {
		t.Errorf("maxSlots = %d, want 1", b.maxSlots)
	}
	if b.expected[0] != l2 {
		t.Error("expected[0] should be l2")
	}
	if b.expected[1] != nil {
		t.Error("expected[1] should be nil after switching to smaller pipeline")
	}
}

func TestBinderAssign(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}

	b.assign(3, l)
	if b.assigned[3] != l {
		t.Error("assigned[3] should be l after assign")
	}
}

func TestBinderAssignOutOfRange(t *testing.T) {
	var b binder
	l := &BindGroupLayout{}

	// Should not panic when index >= MaxBindGroups.
	b.assign(MaxBindGroups, l)
	b.assign(MaxBindGroups+1, l)
}

func TestBinderCheckCompatibilityAllSatisfied(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})
	b.assign(0, l0)
	b.assign(1, l1)

	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityMissingBindGroup(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{l0, l1})
	b.assign(0, l0)
	// Slot 1 is not assigned.

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for missing bind group at index 1")
	}
	if !strings.Contains(err.Error(), "index 1") {
		t.Errorf("error should mention index 1: %v", err)
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("error should mention 'not set': %v", err)
	}
}

func TestBinderCheckCompatibilityIncompatibleLayout(t *testing.T) {
	var b binder

	expected := &BindGroupLayout{}
	wrong := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{expected})
	b.assign(0, wrong)

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error for incompatible layout")
	}
	if !strings.Contains(err.Error(), "incompatible") {
		t.Errorf("error should mention 'incompatible': %v", err)
	}
}

func TestBinderCheckCompatibilityNoPipeline(t *testing.T) {
	var b binder

	// No pipeline set, maxSlots = 0. Should pass (no expectations).
	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with no pipeline = %v, want nil", err)
	}
}

func TestBinderCheckCompatibilityPipelineWithNoBindGroups(t *testing.T) {
	var b binder

	// Pipeline with zero bind group layouts.
	b.updateExpectations(nil)

	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() with empty pipeline = %v, want nil", err)
	}
}

func TestBinderAssignedPreservedAcrossPipelineSwitch(t *testing.T) {
	var b binder

	l0 := &BindGroupLayout{}
	l1 := &BindGroupLayout{}

	// Set bind groups before pipeline.
	b.assign(0, l0)
	b.assign(1, l1)

	// Now set pipeline that expects these layouts.
	b.updateExpectations([]*BindGroupLayout{l0, l1})

	// Assignments should still be valid.
	if err := b.checkCompatibility(); err != nil {
		t.Errorf("checkCompatibility() = %v, want nil (bind groups set before pipeline)", err)
	}
}

func TestBinderCrashScenario(t *testing.T) {
	// Reproduces the crash scenario from the research report:
	// Pipeline has 1 bind group layout (index 0).
	// User calls SetBindGroup(1, ...) — no bind group at index 0.
	var b binder

	expected0 := &BindGroupLayout{}
	b.updateExpectations([]*BindGroupLayout{expected0})

	wrong := &BindGroupLayout{}
	b.assign(1, wrong) // Bind at index 1, but pipeline only expects index 0.

	err := b.checkCompatibility()
	if err == nil {
		t.Fatal("checkCompatibility() = nil, want error (index 0 not satisfied)")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("error should reference index 0 (missing): %v", err)
	}
}

func TestBinderMultipleSlots(t *testing.T) {
	tests := []struct {
		name       string
		expected   int // number of expected layouts
		assigned   []uint32
		wantErr    bool
		errContain string
	}{
		{
			name:     "all 8 slots satisfied",
			expected: 8,
			assigned: []uint32{0, 1, 2, 3, 4, 5, 6, 7},
			wantErr:  false,
		},
		{
			name:       "missing slot 4 of 5",
			expected:   5,
			assigned:   []uint32{0, 1, 2, 3},
			wantErr:    true,
			errContain: "index 4",
		},
		{
			name:       "missing first slot",
			expected:   3,
			assigned:   []uint32{1, 2},
			wantErr:    true,
			errContain: "index 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b binder

			// Create distinct layouts for each expected slot.
			layouts := make([]*BindGroupLayout, tt.expected)
			for i := range layouts {
				layouts[i] = &BindGroupLayout{}
			}
			b.updateExpectations(layouts)

			// Assign the specified slots with the matching layout.
			for _, idx := range tt.assigned {
				if idx < uint32(len(layouts)) {
					b.assign(idx, layouts[idx])
				}
			}

			err := b.checkCompatibility()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkCompatibility() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContain != "" {
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
			}
		})
	}
}
