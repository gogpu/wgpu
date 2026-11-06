// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package stencilblit

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/spirv"
	"github.com/gogpu/wgpu/internal/shaderir"
)

var sourceDims = []gputypes.TextureViewDimension{
	gputypes.TextureViewDimension2D,
	gputypes.TextureViewDimension2DArray,
}

type blitEntry struct {
	stage ir.ShaderStage
	name  string
}

func blitEntries() []blitEntry {
	entries := make([]blitEntry, 0, 2+StencilBits)
	entries = append(entries,
		blitEntry{ir.StageVertex, VertexEntryPoint},
		blitEntry{ir.StageFragment, ClearEntryPoint},
	)
	for bit := range StencilBits {
		entries = append(entries, blitEntry{ir.StageFragment, BitEntryPoint(bit)})
	}
	return entries
}

// glslTargets are the GLSL targets of the GLES backend with the image-load
// bounds-check policy it picks for them (hal/gles glslBoundsCheckPolicies,
// after Rust wgpu-hal gles/device.rs): checked only on desktop GLSL 4.30+.
var glslTargets = []struct {
	version   glsl.Version
	imageLoad backend.BoundsCheckPolicy
}{
	{glsl.VersionES300, backend.BoundsCheckUnchecked},
	{glsl.Version330, backend.BoundsCheckUnchecked},
	{glsl.Version430, backend.BoundsCheckReadZeroSkipWrite},
}

// TestShaderCompiles lowers the shader of each source view dimension and
// writes every entry point as SPIR-V and as GLSL for each GLES target
// (ES 3.00 is the oldest), the backend that needs the blit.
func TestShaderCompiles(t *testing.T) {
	for _, dim := range sourceDims {
		t.Run(dim.String(), func(t *testing.T) {
			src, err := ShaderSource(dim)
			if err != nil {
				t.Fatal(err)
			}
			module, err := shaderir.Lower(src)
			if err != nil {
				t.Fatalf("lower: %v\n%s", err, src)
			}
			if _, err := shaderir.SPIRV(module.IR, spirv.Options{Version: spirv.Version1_0}); err != nil {
				t.Errorf("SPIR-V: %v", err)
			}
			for _, entry := range blitEntries() {
				for _, target := range glslTargets {
					_, _, err := glsl.Compile(module.IR, glsl.Options{
						LangVersion:         target.version,
						EntryPoint:          &backend.EntryPointSelector{Stage: entry.stage, Name: entry.name},
						BoundsCheckPolicies: backend.BoundsCheckPolicies{ImageLoad: target.imageLoad},
					})
					version := target.version
					if err != nil {
						t.Errorf("GLSL %v %s: %v", version, entry.name, err)
					}
				}
			}
		})
	}
}

// TestShaderBakesEachBit checks that the entry point of each bit tests that
// bit and no other.
func TestShaderBakesEachBit(t *testing.T) {
	src, err := ShaderSource(gputypes.TextureViewDimension2D)
	if err != nil {
		t.Fatal(err)
	}
	for bit := range StencilBits {
		decl := fmt.Sprintf("const STENCIL_BIT_%d: u32 = %du;", bit, 1<<bit)
		if !strings.Contains(src, decl) {
			t.Errorf("bit %d: no %q", bit, decl)
		}
		body := fmt.Sprintf("fn %s(@builtin(position) position: vec4f) {\n    if ((source_stencil(position) & STENCIL_BIT_%d) == 0u) {", BitEntryPoint(bit), bit)
		if !strings.Contains(src, body) {
			t.Errorf("bit %d: entry point does not test STENCIL_BIT_%d", bit, bit)
		}
	}
}

func TestShaderSourceRejectsOtherDimensions(t *testing.T) {
	for _, dim := range []gputypes.TextureViewDimension{
		gputypes.TextureViewDimensionCube,
		gputypes.TextureViewDimension3D,
		gputypes.TextureViewDimensionUndefined,
	} {
		if _, err := ShaderSource(dim); err == nil {
			t.Errorf("ShaderSource(%v): no error", dim)
		}
	}
}

func TestKeyFor(t *testing.T) {
	cases := []struct {
		format gputypes.TextureFormat
		layers uint32
		want   gputypes.TextureViewDimension
	}{
		{gputypes.TextureFormatDepth24PlusStencil8, 1, gputypes.TextureViewDimension2D},
		{gputypes.TextureFormatDepth24PlusStencil8, 2, gputypes.TextureViewDimension2DArray},
		{gputypes.TextureFormatDepth32FloatStencil8, 1, gputypes.TextureViewDimension2D},
		{gputypes.TextureFormatDepth32FloatStencil8, 6, gputypes.TextureViewDimension2DArray},
	}
	for _, c := range cases {
		got := KeyFor(c.format, c.layers)
		if got != (Key{Format: c.format, ViewDimension: c.want}) {
			t.Errorf("KeyFor(%v, %d) = %+v, want view dimension %v", c.format, c.layers, got, c.want)
		}
	}
}

func TestParamsBytes(t *testing.T) {
	got := Params{OriginX: 0x01020304, OriginY: 5, Layer: 0xA0B0C0D0}.Bytes()
	want := [ParamsSize]byte{4, 3, 2, 1, 5, 0, 0, 0, 0xD0, 0xC0, 0xB0, 0xA0, 0, 0, 0, 0}
	if got != want {
		t.Errorf("Bytes() = %v, want %v", got, want)
	}
}

func TestStencilStates(t *testing.T) {
	cs := ClearState()
	if cs.Compare != gputypes.CompareFunctionAlways || cs.PassOp != gputypes.StencilOperationReplace ||
		cs.FailOp != gputypes.StencilOperationKeep || cs.DepthFail != gputypes.StencilOperationKeep ||
		cs.WriteMask != 0xFF || cs.ReadMask != 0xFF {
		t.Errorf("ClearState() = %+v", cs)
	}
	for bit := range StencilBits {
		s := BitState(bit)
		want := cs
		want.WriteMask = 1 << uint(bit)
		if s != want {
			t.Errorf("BitState(%d) = %+v, want %+v", bit, s, want)
		}
	}
}

type recordedCall struct {
	op   string
	args [4]uint32
}

type fakePass struct{ calls []recordedCall }

func (p *fakePass) SetPipeline(id int) {
	p.calls = append(p.calls, recordedCall{"pipeline", [4]uint32{uint32(id)}})
}

func (p *fakePass) SetStencilReference(ref uint32) {
	p.calls = append(p.calls, recordedCall{"ref", [4]uint32{ref}})
}

func (p *fakePass) SetScissorRect(x, y, w, h uint32) {
	p.calls = append(p.calls, recordedCall{"scissor", [4]uint32{x, y, w, h}})
}

func (p *fakePass) Draw(vc, ic, fv, fi uint32) {
	p.calls = append(p.calls, recordedCall{"draw", [4]uint32{vc, ic, fv, fi}})
}

// TestRecordLayer checks Dawn's order (BlitR8ToStencil): the scissor, the
// clear draw with reference 0, then one draw per bit with reference 255.
func TestRecordLayer(t *testing.T) {
	pipelines := &Pipelines[int]{Clear: 100}
	for bit := range StencilBits {
		pipelines.Bits[bit] = bit
	}
	var pass fakePass
	RecordLayer[int](&pass, pipelines, 0, 0, 7, 5)

	draw := recordedCall{"draw", [4]uint32{3, 1, 0, 0}}
	want := make([]recordedCall, 0, 5+2*StencilBits)
	want = append(want,
		recordedCall{"scissor", [4]uint32{0, 0, 7, 5}},
		recordedCall{"ref", [4]uint32{0}},
		recordedCall{"pipeline", [4]uint32{100}},
		draw,
		recordedCall{"ref", [4]uint32{255}},
	)
	for bit := range StencilBits {
		want = append(want, recordedCall{"pipeline", [4]uint32{uint32(bit)}}, draw)
	}
	if len(pass.calls) != len(want) {
		t.Fatalf("%d calls, want %d: %v", len(pass.calls), len(want), pass.calls)
	}
	for i := range want {
		if pass.calls[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, pass.calls[i], want[i])
		}
	}
}

type countingFactory struct {
	mu       sync.Mutex
	builds   map[Key]int
	released []Key
	fail     map[Key]bool
}

func (f *countingFactory) build(key Key) (Key, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail[key] {
		delete(f.fail, key)
		return Key{}, errors.New("build failed")
	}
	f.builds[key]++
	return key, nil
}

func (f *countingFactory) release(e Key) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, e)
}

// TestCacheBuildsEachKeyOnce gets the same keys from many goroutines and
// checks that each key is built once and that its entry is returned.
func TestCacheBuildsEachKeyOnce(t *testing.T) {
	f := &countingFactory{builds: map[Key]int{}}
	c := NewCache(f.build, f.release)
	keys := []Key{
		KeyFor(gputypes.TextureFormatDepth24PlusStencil8, 1),
		KeyFor(gputypes.TextureFormatDepth24PlusStencil8, 4),
		KeyFor(gputypes.TextureFormatDepth32FloatStencil8, 1),
		KeyFor(gputypes.TextureFormatDepth32FloatStencil8, 4),
	}
	var wg sync.WaitGroup
	for range 8 {
		for _, key := range keys {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := c.Get(key)
				if err != nil || got != key {
					t.Errorf("Get(%+v) = %+v, %v", key, got, err)
				}
			}()
		}
	}
	wg.Wait()
	for _, key := range keys {
		if f.builds[key] != 1 {
			t.Errorf("key %+v built %d times, want 1", key, f.builds[key])
		}
	}
	if c.Len() != len(keys) {
		t.Errorf("Len() = %d, want %d", c.Len(), len(keys))
	}
}

func TestCacheRetriesFailedBuild(t *testing.T) {
	key := KeyFor(gputypes.TextureFormatDepth24PlusStencil8, 1)
	f := &countingFactory{builds: map[Key]int{}, fail: map[Key]bool{key: true}}
	c := NewCache(f.build, f.release)
	if _, err := c.Get(key); err == nil {
		t.Fatal("first Get: no error")
	}
	if c.Len() != 0 {
		t.Fatalf("a failed build was cached")
	}
	if _, err := c.Get(key); err != nil {
		t.Fatalf("second Get: %v", err)
	}
}

func TestCacheDispose(t *testing.T) {
	f := &countingFactory{builds: map[Key]int{}}
	c := NewCache(f.build, f.release)
	a := KeyFor(gputypes.TextureFormatDepth24PlusStencil8, 1)
	b := KeyFor(gputypes.TextureFormatDepth32FloatStencil8, 3)
	for _, key := range []Key{a, b} {
		if _, err := c.Get(key); err != nil {
			t.Fatal(err)
		}
	}
	c.Dispose()
	c.Dispose()
	if len(f.released) != 2 {
		t.Errorf("released %v, want each entry once", f.released)
	}
	if c.Len() != 0 {
		t.Errorf("Len() after Dispose = %d", c.Len())
	}
	if _, err := c.Get(a); !errors.Is(err, ErrDisposed) {
		t.Errorf("Get after Dispose: %v, want ErrDisposed", err)
	}
	if f.builds[a] != 1 {
		t.Errorf("Get after Dispose built again")
	}
}
