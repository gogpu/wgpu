package core

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

// debugMode controls whether resource tracking is enabled.
// When enabled, all resource allocations/releases are tracked.
// Zero overhead when disabled (~1ns atomic load per call).
var debugMode atomic.Bool

// resourceTracker tracks live GPU resources for leak detection.
var resourceTracker struct {
	mu        sync.Mutex
	resources map[uintptr]resourceInfo
}

type resourceInfo struct {
	Type string // "Buffer", "Texture", "Device", etc.
}

func init() {
	resourceTracker.resources = make(map[uintptr]resourceInfo)
}

// SetDebugMode enables or disables resource tracking.
// When enabled, all GPU resource allocations and releases are tracked,
// and ReportLeaks can be used to find unreleased resources.
// Should be called before any GPU operations.
func SetDebugMode(enabled bool) {
	debugMode.Store(enabled)
}

// DebugMode returns whether debug mode is currently enabled.
func DebugMode() bool {
	return debugMode.Load()
}

// trackResource records a resource allocation (debug mode only).
func trackResource(handle uintptr, typeName string) {
	if !debugMode.Load() || handle == 0 {
		return
	}
	resourceTracker.mu.Lock()
	resourceTracker.resources[handle] = resourceInfo{Type: typeName}
	resourceTracker.mu.Unlock()
}

// untrackResource records a resource release (debug mode only).
func untrackResource(handle uintptr) {
	if !debugMode.Load() || handle == 0 {
		return
	}
	resourceTracker.mu.Lock()
	delete(resourceTracker.resources, handle)
	resourceTracker.mu.Unlock()
}

// LeakReport contains information about unreleased GPU resources.
type LeakReport struct {
	// Count is the total number of unreleased resources.
	Count int
	// Types maps resource type names to their counts.
	Types map[string]int
}

// String returns a human-readable summary of the leak report.
func (r *LeakReport) String() string {
	if r.Count == 0 {
		return "no resource leaks detected"
	}
	s := fmt.Sprintf("%d unreleased GPU resource(s):", r.Count)

	// Sort type names for deterministic output
	names := make([]string, 0, len(r.Types))
	for name := range r.Types {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		s += fmt.Sprintf(" %s=%d", name, r.Types[name])
	}
	return s
}

// ReportLeaks returns information about unreleased GPU resources.
// Only meaningful when debug mode is enabled via SetDebugMode(true).
// Returns nil if no leaks are detected.
func ReportLeaks() *LeakReport {
	if !debugMode.Load() {
		return nil
	}
	resourceTracker.mu.Lock()
	defer resourceTracker.mu.Unlock()

	count := len(resourceTracker.resources)
	if count == 0 {
		return nil
	}

	types := make(map[string]int)
	for _, info := range resourceTracker.resources {
		types[info.Type]++
	}

	return &LeakReport{
		Count: count,
		Types: types,
	}
}

// ResetLeakTracker clears the resource tracker. Useful for test cleanup.
func ResetLeakTracker() {
	resourceTracker.mu.Lock()
	resourceTracker.resources = make(map[uintptr]resourceInfo)
	resourceTracker.mu.Unlock()
}
