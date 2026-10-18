// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"testing"
	"time"

	"github.com/gogpu/wgpu/hal"
)

// TestFencePollingTakesAdapterContext checks that every entry point that
// queries GL sync objects (glGetSynciv, glClientWaitSync) takes the adapter
// context first. Without it the call runs on whatever OS thread the caller's
// goroutine is on, where no GL context is current; on Intel's Windows ICD
// that crashed with an access violation in Device.Poll. Rust wgpu-hal locks
// the context for the same calls (gles/device.rs get_fence_value, wait).
//
// The crash depends on thread scheduling, so the test checks the contract
// instead: while another goroutine holds the context, each call must block.
func TestFencePollingTakesAdapterContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}

	worker := newLockedWorker()
	defer worker.stop()

	var (
		inst     *Instance
		adapters []hal.ExposedAdapter
		err      error
	)
	worker.run(func() {
		var raw hal.Instance
		raw, err = Backend{}.CreateInstance(nil)
		if err != nil {
			return
		}
		inst = raw.(*Instance)
		adapters = inst.EnumerateAdapters(nil)
	})
	if err != nil {
		t.Skipf("GL instance unavailable: %v", err)
	}
	defer worker.run(inst.Destroy)
	if len(adapters) == 0 {
		t.Skip("no GL adapter (GL context could not be created)")
	}

	open, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		t.Skipf("GL device unavailable: %v", err)
	}
	defer open.Device.Destroy()
	device := open.Device.(*Device)
	queue := open.Queue.(*Queue)

	fence, err := device.CreateFence()
	if err != nil {
		t.Fatalf("CreateFence: %v", err)
	}
	defer device.DestroyFence(fence)

	calls := []struct {
		name string
		fn   func()
	}{
		{"Queue.PollCompleted", func() { _ = queue.PollCompleted() }},
		{"Device.GetFenceStatus", func() { _, _ = device.GetFenceStatus(fence) }},
		{"Device.Wait", func() { _, _ = device.Wait(fence, 1, 0) }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			_ = device.ctx.Lock()
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				c.fn()
			}()
			select {
			case <-returned:
				device.ctx.Unlock()
				t.Fatalf("%s returned while another goroutine held the adapter context; it must take the context before calling GL", c.name)
			case <-time.After(200 * time.Millisecond):
			}
			device.ctx.Unlock()
			select {
			case <-returned:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not return after the adapter context was released", c.name)
			}
		})
	}
}
