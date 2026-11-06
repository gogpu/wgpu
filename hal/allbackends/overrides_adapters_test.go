// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestOverridesAllAdapters runs the override cases of this package on every
// adapter that every HAL backend exposes, not only on the adapter that
// RequestAdapter picks: on a hybrid machine the integrated and the discrete
// GPU must both pass. The public API cannot select an adapter (it has no
// EnumerateAdapters, wgpu#360), so this test drives the HAL directly: it
// covers the HAL backends, where pipeline overrides are applied. The public
// tests in this package cover the path from wgpu.ComputePipelineDescriptor to
// the HAL.
//
// Subtests are named backend/adapter/case, so one adapter can be selected
// with -run (for example to give each Vulkan adapter its own validation log).
func TestOverridesAllAdapters(t *testing.T) {
	for _, cfg := range []struct {
		name    string
		variant gputypes.Backend
		mask    gputypes.Backends
		dxil    bool
	}{
		{name: "vulkan", variant: gputypes.BackendVulkan, mask: gputypes.BackendsVulkan},
		{name: "dx12-fxc", variant: gputypes.BackendDX12, mask: gputypes.BackendsDX12},
		{name: "dx12-dxil", variant: gputypes.BackendDX12, mask: gputypes.BackendsDX12, dxil: true},
		{name: "gles", variant: gputypes.BackendGL, mask: gputypes.BackendsGL},
		{name: "metal", variant: gputypes.BackendMetal, mask: gputypes.BackendsMetal},
		{name: "software", variant: gputypes.BackendEmpty, mask: gputypes.BackendsAll},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			// DX12 reads the variable when a device is opened.
			if cfg.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			backend, ok := hal.GetBackend(cfg.variant)
			if !ok {
				t.Skipf("%s backend not registered on this platform", cfg.name)
			}
			instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Backends: cfg.mask})
			if err != nil {
				t.Skipf("CreateInstance: %v", err)
			}
			defer instance.Destroy()
			adapters := instance.EnumerateAdapters(nil)
			if len(adapters) == 0 {
				t.Skip("no adapters")
			}
			for i := range adapters {
				a := &adapters[i]
				t.Logf("adapter %d: %q (%v, %v)", i, a.Info.Name, a.Info.DeviceType, a.Info.Backend)
			}
			for i := range adapters {
				a := &adapters[i]
				t.Run(subtestName(a.Info.Name), func(t *testing.T) {
					runOverrideCasesOnAdapter(t, a)
				})
			}
		})
	}
}

func subtestName(adapter string) string {
	return strings.NewReplacer(" ", "_", "(", "", ")", "", "/", "_").Replace(adapter)
}

func runOverrideCasesOnAdapter(t *testing.T, a *hal.ExposedAdapter) {
	t.Helper()
	// Every case is a compute pipeline; skip adapters without compute, as
	// Rust wgpu's harness does with TestParameters::downlevel_flags.
	if a.Capabilities.DownlevelCapabilities.Flags&gputypes.DownlevelFlagsComputeShaders == 0 {
		t.Skipf("%s has no compute shaders (DownlevelFlagsComputeShaders unset)", a.Info.Name)
	}
	open, err := a.Adapter.Open(0, a.Capabilities.Limits)
	if err != nil {
		t.Fatalf("Open %q: %v", a.Info.Name, err)
	}
	defer func() {
		_ = open.Device.WaitIdle()
		open.Device.Destroy()
	}()
	t.Logf("adapter %q (%v)", a.Info.Name, a.Info.DeviceType)
	r := &halRunner{device: open.Device, queue: open.Queue}
	for _, c := range overrideCases() {
		t.Run(c.name, func(t *testing.T) {
			out, err := r.run(c.wgsl, c.constants, c.inputs)
			if err != nil {
				t.Fatalf("%v", err)
			}
			checkCase(t, &c, out, func(wgsl string) ([][]uint32, error) {
				return r.run(wgsl, nil, c.inputs)
			})
		})
	}
}

// halRunner runs one compute dispatch through the HAL.
type halRunner struct {
	device hal.Device
	queue  hal.Queue
}

// run creates the pipeline (its error is returned), uploads inputs, runs one
// workgroup and returns the contents of every binding.
func (r *halRunner) run(wgsl string, constants map[string]float64, inputs [][]uint32) ([][]uint32, error) {
	d := r.device
	var cleanup []func()
	defer func() {
		_ = d.WaitIdle()
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}()

	shader, err := d.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "overrides", Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		return nil, fmt.Errorf("CreateShaderModule: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyShaderModule(shader) })

	entries := make([]gputypes.BindGroupLayoutEntry, len(inputs))
	for i := range inputs {
		entries[i] = gputypes.BindGroupLayoutEntry{
			Binding: uint32(i), Visibility: gputypes.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		}
	}
	bgl, err := d.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Label: "overrides-bgl", Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("CreateBindGroupLayout: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyBindGroupLayout(bgl) })
	pl, err := d.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{Label: "overrides-pl", BindGroupLayouts: []hal.BindGroupLayout{bgl}})
	if err != nil {
		return nil, fmt.Errorf("CreatePipelineLayout: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyPipelineLayout(pl) })
	pipeline, err := d.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:   "overrides",
		Layout:  pl,
		Compute: hal.ComputeState{Module: shader, EntryPoint: "main", Constants: constants},
	})
	if err != nil {
		return nil, fmt.Errorf("CreateComputePipeline: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyComputePipeline(pipeline) })

	newBuffer := func(label string, size uint64, usage gputypes.BufferUsage) (hal.Buffer, error) {
		b, err := d.CreateBuffer(&hal.BufferDescriptor{Label: label, Size: size, Usage: usage})
		if err != nil {
			return nil, fmt.Errorf("CreateBuffer %s: %w", label, err)
		}
		cleanup = append(cleanup, func() { d.DestroyBuffer(b) })
		return b, nil
	}
	storage := make([]hal.Buffer, len(inputs))
	upload := make([]hal.Buffer, len(inputs))
	readback := make([]hal.Buffer, len(inputs))
	bgEntries := make([]gputypes.BindGroupEntry, len(inputs))
	for i, in := range inputs {
		size := uint64(len(in) * 4)
		if storage[i], err = newBuffer("storage", size, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst); err != nil {
			return nil, err
		}
		if upload[i], err = newBuffer("upload", size, gputypes.BufferUsageMapWrite|gputypes.BufferUsageCopySrc); err != nil {
			return nil, err
		}
		if readback[i], err = newBuffer("readback", size, gputypes.BufferUsageMapRead|gputypes.BufferUsageCopyDst); err != nil {
			return nil, err
		}
		data := make([]byte, size)
		for j, w := range in {
			binary.LittleEndian.PutUint32(data[j*4:], w)
		}
		if err := r.queue.WriteBuffer(upload[i], 0, data); err != nil {
			return nil, fmt.Errorf("WriteBuffer: %w", err)
		}
		bgEntries[i] = gputypes.BindGroupEntry{
			Binding:  uint32(i),
			Resource: gputypes.BufferBinding{Buffer: storage[i].NativeHandle(), Size: size},
		}
	}
	bg, err := d.CreateBindGroup(&hal.BindGroupDescriptor{Label: "overrides-bg", Layout: bgl, Entries: bgEntries})
	if err != nil {
		return nil, fmt.Errorf("CreateBindGroup: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyBindGroup(bg) })

	if err := r.encodeAndSubmit(pipeline, bg, inputs, storage, upload, readback); err != nil {
		return nil, err
	}

	out := make([][]uint32, len(inputs))
	for i, in := range inputs {
		size := uint64(len(in) * 4)
		m, err := d.MapBuffer(readback[i], 0, size)
		if err != nil {
			return nil, fmt.Errorf("MapBuffer: %w", err)
		}
		data := unsafe.Slice((*byte)(m.Ptr), size)
		out[i] = make([]uint32, len(in))
		for j := range out[i] {
			out[i][j] = binary.LittleEndian.Uint32(data[j*4:])
		}
		_ = d.UnmapBuffer(readback[i])
	}
	return out, nil
}

// encodeAndSubmit copies the uploads into the storage buffers, dispatches
// one workgroup, copies the storage buffers to the readback buffers and
// waits for the GPU. The HAL needs explicit barriers between the steps.
func (r *halRunner) encodeAndSubmit(pipeline hal.ComputePipeline, bg hal.BindGroup, inputs [][]uint32, storage, upload, readback []hal.Buffer) error {
	enc, err := r.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "overrides"})
	if err != nil {
		return fmt.Errorf("CreateCommandEncoder: %w", err)
	}
	if err := enc.BeginEncoding("overrides"); err != nil {
		return fmt.Errorf("BeginEncoding: %w", err)
	}
	barriers := func(from, to gputypes.BufferUsage) {
		b := make([]hal.BufferBarrier, len(storage))
		for i := range storage {
			b[i] = hal.BufferBarrier{Buffer: storage[i], Usage: hal.BufferUsageTransition{OldUsage: from, NewUsage: to}}
		}
		enc.TransitionBuffers(b)
	}
	barriers(0, gputypes.BufferUsageCopyDst)
	for i, in := range inputs {
		enc.CopyBufferToBuffer(upload[i], storage[i], []hal.BufferCopy{{Size: uint64(len(in) * 4)}})
	}
	barriers(gputypes.BufferUsageCopyDst, gputypes.BufferUsageStorage)
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: "overrides"})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	pass.End()
	barriers(gputypes.BufferUsageStorage, gputypes.BufferUsageCopySrc)
	for i, in := range inputs {
		enc.CopyBufferToBuffer(storage[i], readback[i], []hal.BufferCopy{{Size: uint64(len(in) * 4)}})
	}
	cmd, err := enc.EndEncoding()
	if err != nil {
		return fmt.Errorf("EndEncoding: %w", err)
	}
	defer r.device.FreeCommandBuffer(cmd)
	if _, err := r.queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
		return fmt.Errorf("Submit: %w", err)
	}
	if err := r.device.WaitIdle(); err != nil {
		return fmt.Errorf("WaitIdle: %w", err)
	}
	return nil
}
