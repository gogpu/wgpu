package core

import (
	"strings"
	"testing"
)

func TestLeakDetection(t *testing.T) {
	// Setup
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	// Simulate resource creation
	trackResource(0x1000, "Buffer")
	trackResource(0x2000, "Texture")

	// Verify leaks are reported
	report := ReportLeaks()
	if report == nil {
		t.Fatal("expected leak report, got nil")
	}
	if report.Count != 2 {
		t.Errorf("expected 2 leaks, got %d", report.Count)
	}
	if report.Types["Buffer"] != 1 {
		t.Errorf("expected 1 Buffer leak, got %d", report.Types["Buffer"])
	}
	if report.Types["Texture"] != 1 {
		t.Errorf("expected 1 Texture leak, got %d", report.Types["Texture"])
	}

	// Destroy one resource
	untrackResource(0x1000)

	report = ReportLeaks()
	if report == nil {
		t.Fatal("expected leak report after partial cleanup, got nil")
	}
	if report.Count != 1 {
		t.Errorf("expected 1 leak after partial cleanup, got %d", report.Count)
	}
	if report.Types["Texture"] != 1 {
		t.Errorf("expected 1 Texture leak, got %d", report.Types["Texture"])
	}

	// Destroy remaining resource
	untrackResource(0x2000)

	report = ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report after full cleanup, got %v", report)
	}
}

func TestLeakDetectionDisabled(t *testing.T) {
	// Ensure debug mode is off
	SetDebugMode(false)
	defer ResetLeakTracker()
	ResetLeakTracker()

	// Track resources while disabled
	trackResource(0x3000, "Buffer")
	trackResource(0x4000, "Device")

	// No report when disabled
	report := ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report when debug disabled, got %v", report)
	}

	// Enable debug and verify nothing was tracked
	SetDebugMode(true)
	defer SetDebugMode(false)

	report = ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report (nothing tracked while disabled), got %v", report)
	}
}

func TestLeakReportString(t *testing.T) {
	tests := []struct {
		name   string
		report LeakReport
		want   string
	}{
		{
			name: "no leaks",
			report: LeakReport{
				Count: 0,
				Types: map[string]int{},
			},
			want: "no resource leaks detected",
		},
		{
			name: "single type",
			report: LeakReport{
				Count: 2,
				Types: map[string]int{"Buffer": 2},
			},
			want: "2 unreleased GPU resource(s): Buffer=2",
		},
		{
			name: "multiple types sorted",
			report: LeakReport{
				Count: 3,
				Types: map[string]int{"Texture": 1, "Buffer": 2},
			},
			want: "3 unreleased GPU resource(s): Buffer=2 Texture=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.report.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLeakReportStringContainsAllTypes(t *testing.T) {
	report := LeakReport{
		Count: 5,
		Types: map[string]int{
			"Device":  1,
			"Buffer":  2,
			"Texture": 2,
		},
	}

	s := report.String()
	if !strings.Contains(s, "5 unreleased GPU resource(s):") {
		t.Errorf("expected count in string, got %q", s)
	}
	if !strings.Contains(s, "Device=1") {
		t.Errorf("expected Device=1 in string, got %q", s)
	}
	if !strings.Contains(s, "Buffer=2") {
		t.Errorf("expected Buffer=2 in string, got %q", s)
	}
	if !strings.Contains(s, "Texture=2") {
		t.Errorf("expected Texture=2 in string, got %q", s)
	}
}

func TestResetLeakTracker(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()

	// Track some resources
	trackResource(0x5000, "Buffer")
	trackResource(0x6000, "Device")
	trackResource(0x7000, "Instance")

	report := ReportLeaks()
	if report == nil {
		t.Fatal("expected leak report before reset, got nil")
	}
	if report.Count != 3 {
		t.Errorf("expected 3 leaks before reset, got %d", report.Count)
	}

	// Reset
	ResetLeakTracker()

	report = ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report after reset, got %v", report)
	}
}

func TestTrackResourceZeroHandle(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	// Zero handle should be ignored
	trackResource(0, "Buffer")

	report := ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report for zero handle, got %v", report)
	}
}

func TestUntrackResourceZeroHandle(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	// Should not panic on zero handle
	untrackResource(0)

	report := ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report, got %v", report)
	}
}

func TestDebugModeToggle(t *testing.T) {
	// Initially off
	SetDebugMode(false)
	if DebugMode() {
		t.Error("expected debug mode to be off initially")
	}

	// Turn on
	SetDebugMode(true)
	if !DebugMode() {
		t.Error("expected debug mode to be on after SetDebugMode(true)")
	}

	// Turn off
	SetDebugMode(false)
	if DebugMode() {
		t.Error("expected debug mode to be off after SetDebugMode(false)")
	}
}
