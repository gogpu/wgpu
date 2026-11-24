// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogpu/naga/backend"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/naga/wgsl"
)

// rtWGSL is the ray query shader the naga tests trace: the same rays, buffer
// layout and result struct as the hand-written MSL kernel in raytracing_hw_test.go.
// Arrays have fixed sizes so naga writes no buffer-sizes argument.
const rtWGSL = `enable wgpu_ray_query;

struct Hit { t: f32, prim: u32, inst: u32, hit: u32 }

@group(0) @binding(0) var<storage, read> rays: array<f32, 3072>;
@group(0) @binding(1) var<storage, read_write> hits: array<Hit, 512>;
@group(0) @binding(2) var acc: acceleration_structure;

@compute @workgroup_size(32)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let i = gid.x;
    let origin = vec3<f32>(rays[6u*i+0u], rays[6u*i+1u], rays[6u*i+2u]);
    let dir = vec3<f32>(rays[6u*i+3u], rays[6u*i+4u], rays[6u*i+5u]);
    var rq: ray_query;
    rayQueryInitialize(&rq, acc, RayDesc(0u, 0xFFu, 0.0, 1.0e30, origin, dir));
    rayQueryProceed(&rq);
    let x = rayQueryGetCommittedIntersection(&rq);
    var h: Hit;
    h.hit = 0u;
    h.t = 0.0;
    h.prim = 0u;
    h.inst = 0u;
    if x.kind == RAY_QUERY_INTERSECTION_TRIANGLE {
        h.hit = 1u;
        h.t = x.t;
        h.prim = x.primitive_index;
        h.inst = x.instance_index;
    }
    hits[i] = h;
}
`

// rtNagaMSL writes MSL for rtWGSL with the options the Metal HAL gives naga.
func rtNagaMSL(t *testing.T) (string, msl.TranslationInfo) {
	t.Helper()
	m, _, err := wgsl.Parse(rtWGSL, wgsl.ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := valid.Validate(m, valid.Options{Capabilities: valid.CapAll}); err != nil {
		t.Fatalf("validate: %v", err)
	}
	opts := msl.DefaultOptions()
	opts.LangVersion = systemShaderCompileOptions().mslVersion
	src, info, err := msl.Compile(m, opts)
	if err != nil {
		t.Fatalf("msl.Compile at %v: %v", opts.LangVersion, err)
	}
	if p := os.Getenv("RT_DUMP_MSL"); p != "" {
		_ = os.WriteFile(p, []byte(src), 0o600)
	}
	return src, info
}

func TestNagaRayQueryMSLCompilesOnMetal(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	raw := CreateSystemDefaultDevice()
	if raw == 0 {
		skipWithoutMetal(t, "no Metal device available")
	}
	defer Release(raw)
	d := &Device{raw: raw, adapter: &Adapter{raw: raw, compileOptions: systemShaderCompileOptions()}}
	src, _ := rtNagaMSL(t)
	lib, nsErr := d.newLibraryWithSource(src)
	if lib == 0 {
		t.Fatalf("Metal rejected naga's MSL: %s\n%s", formatNSError(nsErr), src)
	}
	Release(lib)
}

// TestNagaRayExamplesCompileOnMetal compiles every ray query example shader
// in the directory named by RT_RAY_EXAMPLES (gfx-rs/wgpu's examples/features/src,
// which naga vendors under testdata/upstream) to MSL and gives the MSL to
// Metal. It skips when the variable is unset.
func TestNagaRayExamplesCompileOnMetal(t *testing.T) {
	root := os.Getenv("RT_RAY_EXAMPLES")
	if root == "" {
		t.Skip("RT_RAY_EXAMPLES not set")
	}
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	raw := CreateSystemDefaultDevice()
	if raw == 0 {
		skipWithoutMetal(t, "no Metal device available")
	}
	defer Release(raw)
	d := &Device{raw: raw, adapter: &Adapter{raw: raw, compileOptions: systemShaderCompileOptions()}}
	dirs, err := filepath.Glob(filepath.Join(root, "ray_*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no ray_* examples in %s: %v", root, err)
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			wgslSrc, err := os.ReadFile(filepath.Join(dir, "shader.wgsl"))
			if err != nil {
				t.Fatal(err)
			}
			m, _, err := wgsl.Parse(string(wgslSrc), wgsl.ParseOptions{})
			if err != nil {
				t.Skipf("naga does not parse it: %v", err)
			}
			if _, err := valid.Validate(m, valid.Options{Capabilities: valid.CapAll}); err != nil {
				t.Fatalf("validate: %v", err)
			}
			opts := msl.DefaultOptions()
			opts.LangVersion = systemShaderCompileOptions().mslVersion
			for _, ep := range m.EntryPoints {
				opts.EntryPoint = &backend.EntryPointSelector{Stage: ep.Stage, Name: ep.Name}
				src, _, err := msl.Compile(m, opts)
				if err != nil {
					t.Errorf("%s: msl.Compile: %v", ep.Name, err)
					continue
				}
				lib, nsErr := d.newLibraryWithSource(src)
				if lib == 0 {
					t.Errorf("%s: Metal rejected the MSL: %s", ep.Name, formatNSError(nsErr))
					continue
				}
				Release(lib)
			}
		})
	}
}
