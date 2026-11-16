//go:build js && wasm

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// The browser build's public ErrReleased identity (ADR-070 §15.1). The sentinel
// is internal/core's on every build; this build used to bind the resource
// package's own value instead, which kept errors.Is working only for the
// resource children. The S12 rebind makes one value serve the public API and
// every browser extraction child, and these tests pin both halves.
//
// The file is executed by the wasm-test gate (the root suite under node), not
// only compiled.

// TestBrowserInstanceReleasedAnswersErrReleased pins the public error of a
// released browser instance: RequestAdapter answers the exported ErrReleased and
// errors.Is matches it. The instance is released through the public method
// rather than built with its flag set, so the test drives the lifecycle a caller
// drives; CreateInstance cannot be used here because the browser build needs
// navigator.gpu, which node does not provide.
func TestBrowserInstanceReleasedAnswersErrReleased(t *testing.T) {
	i := &Instance{}
	i.Release()

	if _, err := i.RequestAdapter(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("RequestAdapter on a released instance = %v, want ErrReleased", err)
	}
}

// TestBrowserErrReleasedIsTheSharedSentinel proves the contract error.go states:
// the public sentinel and the value the browser extraction children return are
// the same. It is the regression the rebind exists for — before it the public
// value was internal/resource's, so a value returned by another child (the
// browser device, queue or encoder) did not match it.
func TestBrowserErrReleasedIsTheSharedSentinel(t *testing.T) {
	if !errors.Is(core.ErrReleased, ErrReleased) {
		t.Error("internal/core.ErrReleased and wgpu.ErrReleased are different values: errors.Is is broken across the browser boundary")
	}
	if !errors.Is(resource.ErrReleased, ErrReleased) {
		t.Error("internal/resource.ErrReleased and wgpu.ErrReleased are different values: errors.Is is broken across the browser boundary")
	}
}
