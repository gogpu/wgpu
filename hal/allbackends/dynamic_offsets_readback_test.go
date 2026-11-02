// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"context"
	"encoding/binary"
	"log/slog"
	"math"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
)

// Dynamic offsets (P1-DX12-059, #343): a bind group layout entry with
// HasDynamicOffset reads its buffer at the binding offset plus the offset
// given to SetBindGroup, in binding order. Each test binds the same bind group
// twice with different offsets into the same buffers and checks what each
// draw or dispatch read. A backend that drops the offsets reads slot 0 twice.
// DX12 used to: it bound every buffer through the descriptor table and
// ignored the offsets.

// The buffers hold one 256-byte slot per value set; dynamic offsets must be
// multiples of the 256-byte minimum alignments.
const dynamicSlot = 256

// Binding sizes: the storage bindings are 4 words, so arrayLength() is 4
// whatever the dynamic offset (it counts the binding, not the buffer).
const (
	dynamicUniformSize = 32
	dynamicStorageSize = 16
)

// dynamicStorageWords are the words of the read-only storage buffer, one set
// per slot: two values and a 1 the vertex shader scales its quad by.
var dynamicStorageWords = [3][3]uint32{{5, 6, 1}, {7, 8, 1}, {9, 10, 1}}

// dynamicUniformTags are the tags of the uniform buffer, one per slot.
var dynamicUniformTags = [2]uint32{11, 22}

const dynamicComputeShader = `
struct U { tag: vec4<u32>, pad: vec4<u32> }
@group(0) @binding(0) var<uniform> u: U;
@group(0) @binding(1) var<storage, read> s: array<u32>;
@group(0) @binding(2) var<storage, read_write> out: array<u32>;

@compute @workgroup_size(1)
fn main() {
    out[0] = u.tag.x;
    out[1] = s[0];
    out[2] = s[1];
    out[3] = arrayLength(&s) * 100u + arrayLength(&out);
}
`

// dynamicRenderShader draws a full-height quad over [u.rect.x, u.rect.y] in
// NDC x, scaled in y by s[2], and colors it with the uniform tag, two storage
// words and arrayLength(&s). The vertex stage reads both buffers too, so a
// wrong offset there leaves the quad degenerate or misplaced.
const dynamicRenderShader = `
struct U { rect: vec4<f32>, tag: vec4<u32> }
@group(0) @binding(0) var<uniform> u: U;
@group(0) @binding(1) var<storage, read> s: array<u32>;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    let c = quad[v % 6u];
    let x = select(u.rect.x, u.rect.y, c.x > 0.0);
    return VOut(vec4<f32>(x, c.y * f32(s[2]), 0.0, 1.0));
}

@fragment
fn fs() -> @location(0) vec4<f32> {
    return vec4<f32>(f32(u.tag.x), f32(s[0]), f32(s[1]), f32(arrayLength(&s))) / 255.0;
}
`

// dynamicOffsetsCase is one variant of the dynamic offsets tests.
type dynamicOffsetsCase struct {
	name string
	// reverse lists the layout entries from the highest binding down: the
	// offsets follow the binding numbers, not the entry order.
	reverse bool
	// bindFirst sets the bind group before the pipeline, which WebGPU
	// allows: the backend must bind it to the pipeline's layout later.
	bindFirst bool
}

var dynamicOffsetsCases = []dynamicOffsetsCase{
	{name: "sorted"},
	{name: "unsorted", reverse: true},
	{name: "bind-group-first", bindFirst: true},
}

// dynamicOffsetsDefects are the backends that fail these tests for defects
// outside the dynamic offsets path of the DX12 backend, by test or by
// test/case; each is skipped with its defect until its owner fixes it.
var dynamicOffsetsDefects = map[string]map[string]string{
	"compute": {
		"software": "software backend: a uniform vec4<u32> member reads back as the bits of the f32 (11 -> 0x41300000), a gap of the interpreter's typed value model",
	},
}

func skipDynamicOffsetsDefect(t *testing.T, test string, c dynamicOffsetsCase, b overrideBackend) {
	t.Helper()
	for _, key := range []string{test, test + "/" + c.name} {
		if defect, ok := dynamicOffsetsDefects[key][b.name]; ok {
			t.Skipf("known defect, not the DX12 dynamic offsets path: %s", defect)
		}
	}
}

// TestDynamicOffsetsCompute dispatches twice with the same bind group:
// first with offsets (uniform slot 1, storage slot 2, output slot 1), then
// with (uniform slot 0, storage slot 1, output slot 0).
func TestDynamicOffsetsCompute(t *testing.T) {
	for _, c := range dynamicOffsetsCases {
		t.Run(c.name, func(t *testing.T) {
			forEachDynamicOffsetDevice(t, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
				skipDynamicOffsetsDefect(t, "compute", c, b)
				checkDynamicOffsetsCompute(t, device, c)
			})
		})
	}
}

// TestDynamicOffsetsRender draws twice with the same bind group: first with
// offsets (uniform slot 1, storage slot 2), drawing the right half, then with
// (uniform slot 0, storage slot 1), drawing the left half. Both buffers are
// visible to the vertex and fragment stages.
func TestDynamicOffsetsRender(t *testing.T) {
	for _, c := range dynamicOffsetsCases {
		t.Run(c.name, func(t *testing.T) {
			required := gputypes.DownlevelFlagsVertexStorage | gputypes.DownlevelFlagsFragmentStorage
			forEachDynamicOffsetDevice(t, required, func(t *testing.T, b overrideBackend, device *wgpu.Device) {
				skipDynamicOffsetsDefect(t, "render", c, b)
				checkDynamicOffsetsRender(t, device, c)
			})
		})
	}
}

func checkDynamicOffsetsCompute(t *testing.T, device *wgpu.Device, c dynamicOffsetsCase) {
	t.Helper()
	stages := wgpu.ShaderStageCompute
	bgl := newDynamicLayout(t, device, stages, c.reverse, gputypes.BufferBindingTypeStorage)
	pipeline := newDynamicComputePipeline(t, device, bgl)

	uniform := newDynamicUniformBuffer(t, device, func(slot int) []uint32 {
		return []uint32{dynamicUniformTags[slot], 0, 0, 0}
	})
	storage := newDynamicStorageBuffer(t, device)
	const outSize = 2 * dynamicSlot
	out := newPublicBuffer(t, device, outSize, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(out, 0, make([]byte, outSize)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	bg := newDynamicBindGroup(t, device, bgl, uniform, storage, out)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	if c.bindFirst {
		pass.SetBindGroup(0, bg, []uint32{dynamicSlot, 2 * dynamicSlot, dynamicSlot})
		pass.SetPipeline(pipeline)
	} else {
		pass.SetPipeline(pipeline)
		pass.SetBindGroup(0, bg, []uint32{dynamicSlot, 2 * dynamicSlot, dynamicSlot})
	}
	pass.Dispatch(1, 1, 1)
	pass.SetBindGroup(0, bg, []uint32{0, dynamicSlot, 0})
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	staging := newPublicBuffer(t, device, outSize, wgpu.BufferUsageCopyDst|wgpu.BufferUsageMapRead)
	encoder.CopyBufferToBuffer(out, 0, staging, 0, outSize)
	cmd, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got := readPublicBuffer(t, staging, outSize/4)

	const lengths = 4*100 + 4
	slotWords := dynamicSlot / 4
	checkWords(t, "dispatch 2 (offsets 0, 256, 0)", got[:4], []uint32{11, 7, 8, lengths})
	checkWords(t, "dispatch 1 (offsets 256, 512, 256)", got[slotWords:slotWords+4], []uint32{22, 9, 10, lengths})
}

func checkDynamicOffsetsRender(t *testing.T, device *wgpu.Device, c dynamicOffsetsCase) {
	t.Helper()
	stages := wgpu.ShaderStageVertex | wgpu.ShaderStageFragment
	bgl := newDynamicLayout(t, device, stages, c.reverse, 0)
	pipeline := newDynamicRenderPipeline(t, device, bgl)

	// Slot 0 covers the left half of the target, slot 1 the right half.
	rects := [2][2]float32{{-1, 0}, {0, 1}}
	uniform := newDynamicUniformBuffer(t, device, func(slot int) []uint32 {
		r := rects[slot]
		return []uint32{math.Float32bits(r[0]), math.Float32bits(r[1]), 0, 0, dynamicUniformTags[slot], 0, 0, 0}
	})
	storage := newDynamicStorageBuffer(t, device)
	bg := newDynamicBindGroup(t, device, bgl, uniform, storage, nil)

	px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
		if c.bindFirst {
			pass.SetBindGroup(0, bg, []uint32{dynamicSlot, 2 * dynamicSlot})
			pass.SetPipeline(pipeline)
		} else {
			pass.SetPipeline(pipeline)
			pass.SetBindGroup(0, bg, []uint32{dynamicSlot, 2 * dynamicSlot})
		}
		pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		pass.SetBindGroup(0, bg, []uint32{0, dynamicSlot})
		pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	})

	row := featureTargetHeight / 2 * featureTargetWidth
	checkPixel(t, "left half (draw 2: offsets 0, 256)", px[row+8], [4]uint32{11, 7, 8, 4})
	checkPixel(t, "right half (draw 1: offsets 256, 512)", px[row+featureTargetWidth-8], [4]uint32{22, 9, 10, 4})
}

func checkWords(t *testing.T, what string, got, want []uint32) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: got %v, want %v (tag, s[0], s[1], arrayLength(s)*100+arrayLength(out))", what, got, want)
			return
		}
	}
}

func checkPixel(t *testing.T, what string, px uint32, want [4]uint32) {
	t.Helper()
	got := [4]uint32{px & 0xff, px >> 8 & 0xff, px >> 16 & 0xff, px >> 24}
	if got != want {
		t.Errorf("%s: got %v, want %v (tag, s[0], s[1], arrayLength(s))", what, got, want)
	}
}

// newDynamicLayout creates a layout with a dynamic uniform buffer at binding
// 0, a dynamic read-only storage buffer at binding 1 and, when outType is
// not 0, a dynamic buffer of that type at binding 2. reverse lists the
// entries from the highest binding down.
func newDynamicLayout(t *testing.T, device *wgpu.Device, stages gputypes.ShaderStages, reverse bool, outType gputypes.BufferBindingType) *wgpu.BindGroupLayout {
	t.Helper()
	entry := func(binding uint32, ty gputypes.BufferBindingType, size uint64) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{
			Binding:    binding,
			Visibility: stages,
			Buffer:     &gputypes.BufferBindingLayout{Type: ty, HasDynamicOffset: true, MinBindingSize: size},
		}
	}
	entries := []gputypes.BindGroupLayoutEntry{
		entry(0, gputypes.BufferBindingTypeUniform, dynamicUniformSize),
		entry(1, gputypes.BufferBindingTypeReadOnlyStorage, dynamicStorageSize),
	}
	if outType != 0 {
		entries = append(entries, entry(2, outType, dynamicStorageSize))
	}
	if reverse {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "dynamic-offsets", Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	return bgl
}

func newDynamicPipelineLayout(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout) *wgpu.PipelineLayout {
	t.Helper()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "dynamic-offsets",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	return layout
}

func newDynamicShader(t *testing.T, device *wgpu.Device, wgsl string) *wgpu.ShaderModule {
	t.Helper()
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "dynamic-offsets", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	return shader
}

func newDynamicComputePipeline(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout) *wgpu.ComputePipeline {
	t.Helper()
	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "dynamic-offsets",
		Layout:     newDynamicPipelineLayout(t, device, bgl),
		Module:     newDynamicShader(t, device, dynamicComputeShader),
		EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

func newDynamicRenderPipeline(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout) *wgpu.RenderPipeline {
	t.Helper()
	shader := newDynamicShader(t, device, dynamicRenderShader)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "dynamic-offsets",
		Layout: newDynamicPipelineLayout(t, device, bgl),
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll},
			},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)
	return pipeline
}

// newDynamicUniformBuffer creates a uniform buffer of two slots, slot i
// holding the words words(i).
func newDynamicUniformBuffer(t *testing.T, device *wgpu.Device, words func(slot int) []uint32) *wgpu.Buffer {
	t.Helper()
	data := make([]byte, len(dynamicUniformTags)*dynamicSlot)
	for slot := range dynamicUniformTags {
		putWords(data[slot*dynamicSlot:], words(slot))
	}
	buf := newPublicBuffer(t, device, uint64(len(data)), wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buf
}

// newDynamicStorageBuffer creates a read-only storage buffer of three slots
// holding dynamicStorageWords. The word after each set is a poison value that
// a binding read past its 4 words would pick up.
func newDynamicStorageBuffer(t *testing.T, device *wgpu.Device) *wgpu.Buffer {
	t.Helper()
	data := make([]byte, len(dynamicStorageWords)*dynamicSlot)
	for slot, words := range dynamicStorageWords {
		putWords(data[slot*dynamicSlot:], append(words[:], 0, 0xdead))
	}
	buf := newPublicBuffer(t, device, uint64(len(data)), wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buf
}

func putWords(dst []byte, words []uint32) {
	for i, w := range words {
		binary.LittleEndian.PutUint32(dst[i*4:], w)
	}
}

// newDynamicBindGroup binds every buffer at offset 0; out is optional. The
// entries are listed from the highest binding down, so a backend that takes
// the dynamic offsets in entry order instead of binding order gets them
// wrong.
func newDynamicBindGroup(t *testing.T, device *wgpu.Device, bgl *wgpu.BindGroupLayout, uniform, storage, out *wgpu.Buffer) *wgpu.BindGroup {
	t.Helper()
	var entries []wgpu.BindGroupEntry
	if out != nil {
		entries = append(entries, wgpu.BindGroupEntry{Binding: 2, Buffer: out, Size: dynamicStorageSize})
	}
	entries = append(entries,
		wgpu.BindGroupEntry{Binding: 1, Buffer: storage, Size: dynamicStorageSize},
		wgpu.BindGroupEntry{Binding: 0, Buffer: uniform, Size: dynamicUniformSize},
	)
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: "dynamic-offsets", Layout: bgl, Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)
	return bg
}

// renderDynamicTarget runs draw in a pass over a featureTargetWidth x
// featureTargetHeight RGBA8 target cleared to white and returns its pixels,
// row-major.
func renderDynamicTarget(t *testing.T, device *wgpu.Device, draw func(pass *wgpu.RenderPassEncoder)) []uint32 {
	t.Helper()
	extent := wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1}
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "dynamic-offsets",
		Size:          extent,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 1, B: 1, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	draw(pass)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	const words = featureTargetWidth * featureTargetHeight
	staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: featureTargetWidth * 4, RowsPerImage: featureTargetHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         extent,
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return readPublicBuffer(t, staging, words)
}

// forEachDynamicOffsetDevice runs fn with a device of every backend on each
// adapter the two power preferences select, as forEachBackendAdapter does.
// Vulkan runs under the validation layer, DX12 under the debug layer (and
// its DXIL path under the DXIL validator): any message from either fails
// the test.
func forEachDynamicOffsetDevice(t *testing.T, required gputypes.DownlevelFlags, fn func(t *testing.T, b overrideBackend, device *wgpu.Device)) {
	t.Helper()
	for _, b := range overrideBackends {
		t.Run(b.name, func(t *testing.T) {
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
				t.Setenv("GOGPU_DX12_DXIL_VALIDATE", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			seen := map[string]bool{}
			for _, p := range featurePowerPreferences {
				t.Run(p.name, func(t *testing.T) {
					device, adapter := openDebugBackendDevice(t, b, required, p.pref)
					if seen[adapter] {
						t.Skipf("%s already checked", adapter)
					}
					seen[adapter] = true
					fn(t, b, device)
				})
			}
		})
	}
}

// openDebugBackendDevice is openBackendDevice with the DX12 debug layer on.
func openDebugBackendDevice(t *testing.T, b overrideBackend, required gputypes.DownlevelFlags, pref gputypes.PowerPreference) (*wgpu.Device, string) {
	t.Helper()
	var flags gputypes.InstanceFlags
	switch {
	case b.software:
	case b.backend == gputypes.BackendVulkan:
		flags = enableVulkanValidation(t)
	case b.backend == gputypes.BackendDX12:
		flags = enableDX12DebugLayer(t)
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: b.backends, Flags: flags})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: pref, ForceFallbackAdapter: b.software})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	info := adapter.Info()
	isCPU := info.DeviceType == gputypes.DeviceTypeCPU
	if b.software != isCPU || (!b.software && info.Backend != b.backend) {
		t.Skipf("no %s adapter (got %s, backend %v)", b.name, info.Name, info.Backend)
	}
	if missing := required &^ adapter.DownlevelCapabilities().Flags; missing != 0 {
		t.Skipf("%s lacks required downlevel flags %#x", info.Name, uint32(missing))
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	t.Logf("adapter: %s (%v)", info.Name, info.Backend)
	return device, info.Name
}

// dx12DebugCapture is a slog.Handler that keeps the D3D12 debug layer
// messages of severity WARNING and above (hal/dx12 logs each InfoQueue
// message as "dx12: debug message" with "severity" and "id" attributes).
// Like Rust wgpu-hal (auxil/dxgi/exception.rs), it drops the warnings 820 and
// 821, CLEARRENDERTARGETVIEW/CLEARDEPTHSTENCILVIEW_MISMATCHINGCLEARVALUE: a
// clear with a value other than the resource's optimized clear value, which
// WebGPU textures do not have.
type dx12DebugCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (c *dx12DebugCapture) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (c *dx12DebugCapture) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler takes the Record by value
	if r.Message != "dx12: debug message" {
		return nil
	}
	var severity, id, text string
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "severity":
			severity = a.Value.String()
		case "id":
			id = a.Value.String()
		case "msg":
			text = a.Value.String()
		}
		return true
	})
	if severity == "WARNING" && (id == "820" || id == "821") {
		return nil
	}
	switch severity {
	case "CORRUPTION", "ERROR", "WARNING":
		c.mu.Lock()
		c.msgs = append(c.msgs, severity+": "+text)
		c.mu.Unlock()
	}
	return nil
}

func (c *dx12DebugCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *dx12DebugCapture) WithGroup(string) slog.Handler      { return c }

// enableDX12DebugLayer returns the instance flags that turn on the D3D12
// debug layer and fails the test for every warning or error it reports
// until the test's cleanups have run; the device drains its InfoQueue when
// it is released. Without the Graphics Tools feature the HAL runs without
// the layer and nothing is checked.
func enableDX12DebugLayer(t *testing.T) gputypes.InstanceFlags {
	t.Helper()
	capture := &dx12DebugCapture{}
	prev := hal.Logger()
	hal.SetLogger(slog.New(capture))
	t.Cleanup(func() {
		hal.SetLogger(prev)
		capture.mu.Lock()
		defer capture.mu.Unlock()
		for _, m := range capture.msgs {
			t.Errorf("D3D12 debug layer: %s", m)
		}
	})
	return gputypes.InstanceFlagsDebug
}
