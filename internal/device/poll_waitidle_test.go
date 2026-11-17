//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/queue"
)

// waitIdleFailDevice is the HAL double whose WaitIdle reports a device-loss
// class failure: Poll(PollWait) must surface it through the error-scope sink
// instead of discarding it (enterprise-gap G4, the swallowed _ = halDev.WaitIdle()).
type waitIdleFailDevice struct {
	testHALDevice
	err error
}

func (d *waitIdleFailDevice) WaitIdle() error { return d.err }

// TestPollWaitIdleErrorReportsInternalScope proves the failure of the wait
// itself reaches a caller-visible sink and that no completion is claimed:
// before the fix Poll discarded the error and drained with the
// all-indices-completed sentinel.
func TestPollWaitIdleErrorReportsInternalScope(t *testing.T) {
	halErr := errors.New("vkDeviceWaitIdle: ERROR_DEVICE_LOST")
	coreDevice := core.NewDevice(
		&waitIdleFailDevice{err: halErr}, nil, 0, gputypes.DefaultLimits(), "poll-waitidle",
	)
	t.Cleanup(coreDevice.Destroy)
	q := queue.New(&testHALQueue{}, nil)
	d := New(coreDevice, q, nil)
	q.SetDevice(d)

	coreDevice.PushErrorScope(core.ErrorFilterInternal)
	got := d.Poll(core.PollWait)
	gpuErr, err := coreDevice.PopErrorScope()

	if got {
		t.Fatalf("Poll(PollWait) = true after a failed WaitIdle, want false: no completion may be claimed")
	}
	if err != nil {
		t.Fatalf("PopErrorScope: %v", err)
	}
	if gpuErr == nil {
		t.Fatal("Poll(PollWait) swallowed the WaitIdle failure: no internal error reached the scope")
	}
	if gpuErr.Type != core.ErrorFilterInternal {
		t.Errorf("captured error type = %v, want %v", gpuErr.Type, core.ErrorFilterInternal)
	}
	if !strings.Contains(gpuErr.Message, "vkDeviceWaitIdle") {
		t.Errorf("captured message %q does not carry the HAL error text", gpuErr.Message)
	}
}
