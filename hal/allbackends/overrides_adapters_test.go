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
	forEachHALAdapter(t, func(t *testing.T, _ backendName, a *hal.ExposedAdapter, _ *vvlCapture) {
		runOverrideCasesOnAdapter(t, a)
	})
}

// halBackendConfig is one HAL backend variant that the all-adapter tests run.
type halBackendConfig struct {
	name    backendName
	variant gputypes.Backend
	mask    gputypes.Backends
	dxil    bool // DX12 with naga's DXIL backend instead of HLSL+FXC
}

var halBackendConfigs = []halBackendConfig{
	{name: backendVulkan, variant: gputypes.BackendVulkan, mask: gputypes.BackendsVulkan},
	{name: backendDX12FXC, variant: gputypes.BackendDX12, mask: gputypes.BackendsDX12},
	{name: backendDX12DXIL, variant: gputypes.BackendDX12, mask: gputypes.BackendsDX12, dxil: true},
	{name: backendGLES, variant: gputypes.BackendGL, mask: gputypes.BackendsGL},
	{name: backendMetal, variant: gputypes.BackendMetal, mask: gputypes.BackendsMetal},
	{name: backendSoftware, variant: gputypes.BackendEmpty, mask: gputypes.BackendsAll},
}

// forEachHALAdapter runs fn in a subtest backend/adapter for every adapter of
// every registered HAL backend variant. The instance lives for the backend's
// subtest; a backend that is not registered, has no instance or no adapter
// here is skipped. Vulkan instances run with the validation layer
// (startVulkanValidation): fn gets the capture to drain per step, and any
// message left at the end fails the backend's subtest. vvl is nil for the
// other backends.
func forEachHALAdapter(t *testing.T, fn func(t *testing.T, backend backendName, a *hal.ExposedAdapter, vvl *vvlCapture)) {
	t.Helper()
	for _, cfg := range halBackendConfigs {
		t.Run(string(cfg.name), func(t *testing.T) {
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
			var flags gputypes.InstanceFlags
			var vvl *vvlCapture
			if cfg.variant == gputypes.BackendVulkan {
				flags, vvl = startVulkanValidation(t)
			}
			instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Backends: cfg.mask, Flags: flags})
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
					fn(t, cfg.name, a, vvl)
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

// halBinding is one buffer of a halDispatch, in @group(0).
type halBinding struct {
	binding uint32
	// kind is BufferBindingTypeStorage, BufferBindingTypeReadOnlyStorage or
	// BufferBindingTypeUniform.
	kind  gputypes.BufferBindingType
	words []uint32 // initial contents; the buffer is len(words)*4 bytes
}

// usage is the buffer usage the shader needs for the binding.
func (b *halBinding) usage() gputypes.BufferUsage {
	if b.kind == gputypes.BufferBindingTypeUniform {
		return gputypes.BufferUsageUniform
	}
	return gputypes.BufferUsageStorage
}

// halDispatch is one compute dispatch for halRunner.dispatch.
type halDispatch struct {
	label      string
	wgsl       string
	entry      string
	constants  map[string]float64
	workgroups [3]uint32
	bindings   []halBinding
}

// run creates the pipeline (its error is returned), uploads inputs into
// read-write storage bindings 0..n-1, runs one workgroup of entry point main
// and returns the contents of every binding.
func (r *halRunner) run(wgsl string, constants map[string]float64, inputs [][]uint32) ([][]uint32, error) {
	bindings := make([]halBinding, len(inputs))
	for i, in := range inputs {
		bindings[i] = halBinding{binding: uint32(i), kind: gputypes.BufferBindingTypeStorage, words: in}
	}
	return r.dispatch(&halDispatch{
		label: "overrides", wgsl: wgsl, entry: "main", constants: constants,
		workgroups: [3]uint32{1, 1, 1}, bindings: bindings,
	})
}

// dispatch creates the pipeline (its error is returned), uploads every
// binding's words, runs the dispatch and returns the contents of every
// binding after it, in the order of disp.bindings.
func (r *halRunner) dispatch(disp *halDispatch) ([][]uint32, error) {
	d := r.device
	var cleanup []func()
	defer func() {
		_ = d.WaitIdle()
		for i := len(cleanup) - 1; i >= 0; i-- {
			cleanup[i]()
		}
	}()

	shader, err := d.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: disp.label, Source: hal.ShaderSource{WGSL: disp.wgsl}})
	if err != nil {
		return nil, fmt.Errorf("CreateShaderModule: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyShaderModule(shader) })

	entries := make([]gputypes.BindGroupLayoutEntry, len(disp.bindings))
	for i := range disp.bindings {
		b := &disp.bindings[i]
		entries[i] = gputypes.BindGroupLayoutEntry{
			Binding: b.binding, Visibility: gputypes.ShaderStageCompute,
			Buffer: &gputypes.BufferBindingLayout{Type: b.kind},
		}
	}
	bgl, err := d.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{Label: disp.label + "-bgl", Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("CreateBindGroupLayout: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyBindGroupLayout(bgl) })
	pl, err := d.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{Label: disp.label + "-pl", BindGroupLayouts: []hal.BindGroupLayout{bgl}})
	if err != nil {
		return nil, fmt.Errorf("CreatePipelineLayout: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyPipelineLayout(pl) })
	pipeline, err := d.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:  disp.label,
		Layout: pl,
		// Zeroing workgroup memory is WebGPU's default, which the public API
		// passes (descriptor.go). The HAL's zero value turns it off, and a
		// runner that left it false read stale workgroup memory on NVIDIA
		// Vulkan (corpus vars/workgroup-zero-rt, 2026-11-08).
		Compute: hal.ComputeState{Module: shader, EntryPoint: disp.entry, Constants: disp.constants,
			ZeroInitializeWorkgroupMemory: true},
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
	n := len(disp.bindings)
	gpu := make([]hal.Buffer, n)
	upload := make([]hal.Buffer, n)
	readback := make([]hal.Buffer, n)
	bgEntries := make([]gputypes.BindGroupEntry, n)
	for i := range disp.bindings {
		b := &disp.bindings[i]
		size := uint64(len(b.words) * 4)
		if gpu[i], err = newBuffer("binding", size, b.usage()|gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst); err != nil {
			return nil, err
		}
		if upload[i], err = newBuffer("upload", size, gputypes.BufferUsageMapWrite|gputypes.BufferUsageCopySrc); err != nil {
			return nil, err
		}
		if readback[i], err = newBuffer("readback", size, gputypes.BufferUsageMapRead|gputypes.BufferUsageCopyDst); err != nil {
			return nil, err
		}
		data := make([]byte, size)
		for j, w := range b.words {
			binary.LittleEndian.PutUint32(data[j*4:], w)
		}
		if err := r.queue.WriteBuffer(upload[i], 0, data); err != nil {
			return nil, fmt.Errorf("WriteBuffer: %w", err)
		}
		bgEntries[i] = gputypes.BindGroupEntry{
			Binding:  b.binding,
			Resource: gputypes.BufferBinding{Buffer: gpu[i].NativeHandle(), Size: size},
		}
	}
	bg, err := d.CreateBindGroup(&hal.BindGroupDescriptor{Label: disp.label + "-bg", Layout: bgl, Entries: bgEntries})
	if err != nil {
		return nil, fmt.Errorf("CreateBindGroup: %w", err)
	}
	cleanup = append(cleanup, func() { d.DestroyBindGroup(bg) })

	if err := r.encodeAndSubmit(disp, pipeline, bg, gpu, upload, readback); err != nil {
		return nil, err
	}

	out := make([][]uint32, n)
	for i := range disp.bindings {
		size := uint64(len(disp.bindings[i].words) * 4)
		m, err := d.MapBuffer(readback[i], 0, size)
		if err != nil {
			return nil, fmt.Errorf("MapBuffer: %w", err)
		}
		data := unsafe.Slice((*byte)(m.Ptr), size)
		out[i] = make([]uint32, len(disp.bindings[i].words))
		for j := range out[i] {
			out[i][j] = binary.LittleEndian.Uint32(data[j*4:])
		}
		if err := d.UnmapBuffer(readback[i]); err != nil {
			return nil, fmt.Errorf("UnmapBuffer: %w", err)
		}
	}
	return out, nil
}

// encodeAndSubmit copies the uploads into the binding buffers, runs the
// dispatch, copies the binding buffers to the readback buffers and waits for
// the GPU. The HAL needs explicit barriers between the steps.
func (r *halRunner) encodeAndSubmit(disp *halDispatch, pipeline hal.ComputePipeline, bg hal.BindGroup, gpu, upload, readback []hal.Buffer) error {
	enc, err := r.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: disp.label})
	if err != nil {
		return fmt.Errorf("CreateCommandEncoder: %w", err)
	}
	if err := enc.BeginEncoding(disp.label); err != nil {
		return fmt.Errorf("BeginEncoding: %w", err)
	}
	fixed := func(u gputypes.BufferUsage) func(int) gputypes.BufferUsage {
		return func(int) gputypes.BufferUsage { return u }
	}
	shaderUse := func(i int) gputypes.BufferUsage { return disp.bindings[i].usage() }
	barriers := func(from, to func(int) gputypes.BufferUsage) {
		b := make([]hal.BufferBarrier, len(gpu))
		for i := range gpu {
			b[i] = hal.BufferBarrier{Buffer: gpu[i], Usage: hal.BufferUsageTransition{OldUsage: from(i), NewUsage: to(i)}}
		}
		enc.TransitionBuffers(b)
	}
	barriers(fixed(0), fixed(gputypes.BufferUsageCopyDst))
	for i := range gpu {
		enc.CopyBufferToBuffer(upload[i], gpu[i], []hal.BufferCopy{{Size: uint64(len(disp.bindings[i].words) * 4)}})
	}
	barriers(fixed(gputypes.BufferUsageCopyDst), shaderUse)
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{Label: disp.label})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(disp.workgroups[0], disp.workgroups[1], disp.workgroups[2])
	pass.End()
	barriers(shaderUse, fixed(gputypes.BufferUsageCopySrc))
	for i := range gpu {
		enc.CopyBufferToBuffer(gpu[i], readback[i], []hal.BufferCopy{{Size: uint64(len(disp.bindings[i].words) * 4)}})
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
