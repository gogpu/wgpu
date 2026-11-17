//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/queue"
)

// budgetFailEncoder is the pool's HAL encoder double: BeginEncoding reports
// the command-buffer budget as exhausted, the way the Metal encoder does when
// its queue's budget cannot take another command buffer (P1-MTL-008).
type budgetFailEncoder struct{ hal.CommandEncoder }

func (budgetFailEncoder) BeginEncoding(string) error {
	return fmt.Errorf("metal: command buffer budget exhausted: %w", hal.ErrCommandBufferBudget)
}

// budgetFailDevice hands out the budget-fail encoder from the pool path.
type budgetFailDevice struct{ testHALDevice }

func (budgetFailDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return budgetFailEncoder{}, nil
}

// TestCreateCommandEncoderBudgetErrorReachesScope proves the pool-acquire
// BeginEncoding path does not swallow a command-buffer-budget failure: the
// caller gets the error AND the device's Internal error scope captures it as
// an internal-class GPU error whose errors.Is reaches core.ErrInternal. The
// classification is deliberately not device-lost (P1-MTL-008 / P2-MTL-010).
func TestCreateCommandEncoderBudgetErrorReachesScope(t *testing.T) {
	raw := budgetFailDevice{}
	coreDevice := core.NewDevice(&raw, nil, 0, gputypes.DefaultLimits(), "pool-budget")
	t.Cleanup(coreDevice.Destroy)
	q := queue.New(&testHALQueue{}, nil)
	pool := encoder.NewPool(&raw)
	d := New(coreDevice, q, pool)
	q.SetDevice(d)

	coreDevice.PushErrorScope(core.ErrorFilterInternal)
	_, err := d.CreateCommandEncoder("budget-test")
	gpuErr, popErr := coreDevice.PopErrorScope()

	if err == nil {
		t.Fatal("CreateCommandEncoder returned nil error on an exhausted budget")
	}
	if !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Errorf("CreateCommandEncoder error %v does not wrap hal.ErrCommandBufferBudget", err)
	}
	if popErr != nil {
		t.Fatalf("PopErrorScope: %v", popErr)
	}
	if gpuErr == nil {
		t.Fatal("Internal scope captured nothing: the budget failure never reached the error scope")
	}
	if gpuErr.Type != core.ErrorFilterInternal {
		t.Errorf("captured error type = %v, want %v", gpuErr.Type, core.ErrorFilterInternal)
	}
	if !errors.Is(gpuErr, core.ErrInternal) {
		t.Errorf("captured error does not classify as core.ErrInternal: %v", gpuErr)
	}
}
