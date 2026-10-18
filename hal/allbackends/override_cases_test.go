// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The compute cases for WGSL overrides, shared by the public API test
// (TestOverrideCompute) and the HAL test on every adapter
// (TestOverridesAllAdapters). Every HAL backend must resolve overrides per
// pipeline stage with naga.ProcessOverrides before its shader backend, as
// Rust wgpu-hal does (vulkan/device.rs:804, dx12/device.rs:349,
// gles/device.rs:385, metal/device.rs:174).

// overrideWorkgroupShader writes i+1 at every local invocation index i. The
// workgroup size is given by an override, so one workgroup writes wg*2
// elements: 8 with the default. Each invocation writes its own element, so the
// result does not depend on atomics or scheduling.
//
// BUG-OVERRIDE-WORKGROUP-001: with naga v0.19.0 the size compiled as 1x2x1.
const overrideWorkgroupShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

override wg: u32 = 4u;

@compute @workgroup_size(wg, 2)
fn main(@builtin(local_invocation_index) i: u32) {
    out[i] = i + 1u;
}
`

// overrideArithmeticShader uses overrides addressed by @id and by name, of
// types u32, f32 and bool, in expressions that become constant only after
// the values are substituted.
const overrideArithmeticShader = `
@group(0) @binding(0) var<storage, read_write> out: array<u32, 16>;

@id(7) override scale: u32 = 2u;
override bias: f32 = 0.5;
override flag: bool = false;

@compute @workgroup_size(1)
fn main() {
    out[0] = scale * 3u + 1u;
    out[1] = u32(bias * 4.0);
    out[2] = select(10u, 20u, flag);
    out[3] = scale % 4u;
}
`

// anonymousOverrideShader sizes a workgroup array by `n * 2u`, an expression
// of an override: the WGSL frontend adds an anonymous override (no name, no
// @id) for it. Rust naga 30's process_overrides panics on such an override
// (unreachable!() in pipeline_constants.rs); naga-go evaluates it.
//
// One workgroup of m invocations fills a[0..n*2) with j*10+1, then
// invocation 0 copies a to result. invocations counts the invocations.
const anonymousOverrideShader = `
override n: u32 = 4u;
var<workgroup> a: array<u32, n * 2u>;
override m: u32 = 1u;

@group(0) @binding(0) var<storage, read_write> result: array<u32>;
@group(0) @binding(1) var<storage, read_write> invocations: atomic<u32>;

@compute @workgroup_size(m)
fn main(@builtin(local_invocation_index) i: u32) {
    atomicAdd(&invocations, 1u);
    for (var j = i; j < n * 2u; j += m) {
        a[j] = j * 10u + 1u;
    }
    workgroupBarrier();
    if (i == 0u) {
        for (var j = 0u; j < n * 2u; j++) {
            result[j] = a[j];
        }
    }
}
`

const (
	overrideElems   = 16 // u32 elements of the first binding
	anonSentinel    = 0xDEADBEEF
	overrideBufSize = overrideElems * 4
)

// overrideCase is one compute dispatch of one workgroup. Binding i is a
// storage buffer @group(0) @binding(i) with the initial contents inputs[i].
type overrideCase struct {
	name      string
	wgsl      string
	constants map[string]float64 // pipeline constants
	// literals are the same values as WGSL literals by override name, for
	// the control shader (controlShader).
	literals map[string]string
	inputs   [][]uint32
	check    func(out [][]uint32) string // "" if out is right
}

// overrideCases returns the compute cases.
func overrideCases() []overrideCase {
	zero := func() [][]uint32 { return [][]uint32{make([]uint32, overrideElems)} }
	anonInputs := func() [][]uint32 {
		result := make([]uint32, overrideElems)
		for i := range result {
			result[i] = anonSentinel
		}
		return [][]uint32{result, {0}}
	}
	return []overrideCase{
		{
			name: "workgroup-default", wgsl: overrideWorkgroupShader, inputs: zero(),
			check: wantInvocations(8), // 4x2
		},
		{
			name: "workgroup-constant", wgsl: overrideWorkgroupShader, inputs: zero(),
			constants: map[string]float64{"wg": 3}, literals: map[string]string{"wg": "3u"},
			check: wantInvocations(6), // 3x2
		},
		{
			// scale*3+1, u32(bias*4), select(10, 20, flag), scale%4
			name: "arithmetic-defaults", wgsl: overrideArithmeticShader, inputs: zero(),
			check: wantFirst(7, 2, 10, 2),
		},
		{
			name: "arithmetic-constants", wgsl: overrideArithmeticShader, inputs: zero(),
			constants: map[string]float64{"7": 5, "bias": 2.25, "flag": 1},
			literals:  map[string]string{"scale": "5u", "bias": "2.25", "flag": "true"},
			check:     wantFirst(16, 9, 20, 1),
		},
		{
			name: "anonymous-defaults", wgsl: anonymousOverrideShader, inputs: anonInputs(),
			check: wantAnonymous(8, 1),
		},
		{
			name: "anonymous-constants", wgsl: anonymousOverrideShader, inputs: anonInputs(),
			constants: map[string]float64{"n": 5, "m": 64}, literals: map[string]string{"n": "5u", "m": "64u"},
			check: wantAnonymous(10, 64),
		},
	}
}

var overrideDeclRe = regexp.MustCompile(`(?m)^(?:@id\(\d+\) )?override (\w+): (\w+) = ([^;]+);`)

// controlShader returns c.wgsl with every override declaration turned into a
// const of the case's value. The control runs no override code: when it
// fails the same way as the override shader, the backend fails on this
// shader for another reason than overrides.
func (c *overrideCase) controlShader() string {
	return overrideDeclRe.ReplaceAllStringFunc(c.wgsl, func(decl string) string {
		m := overrideDeclRe.FindStringSubmatch(decl)
		value := m[3]
		if v, ok := c.literals[m[1]]; ok {
			value = v
		}
		return fmt.Sprintf("const %s: %s = %s;", m[1], m[2], value)
	})
}

// checkCase checks out; if it is wrong, it runs the control shader with
// runControl. A control that fails too skips the test (the backend fails on
// this shader without overrides); otherwise the override case fails.
func checkCase(t *testing.T, c *overrideCase, out [][]uint32, runControl func(wgsl string) ([][]uint32, error)) {
	t.Helper()
	t.Logf("readback: %s", formatWords(out))
	msg := c.check(out)
	if msg == "" {
		return
	}
	control, err := runControl(c.controlShader())
	switch {
	case err != nil:
		t.Errorf("%s (control without overrides: %v)", msg, err)
	case c.check(control) != "":
		t.Skipf("%s; the control shader without overrides fails too (%s): not an override failure", msg, formatWords(control))
	default:
		t.Errorf("%s; the control shader without overrides passes (%s)", msg, formatWords(control))
	}
}

// wantInvocations checks that overrideWorkgroupShader ran n invocations.
func wantInvocations(n uint32) func([][]uint32) string {
	want := make([]uint32, overrideElems)
	for i := range n {
		want[i] = i + 1
	}
	return func(out [][]uint32) string {
		if !slices.Equal(out[0], want) {
			return fmt.Sprintf("out = %v, want %v", out[0], want)
		}
		return ""
	}
}

func wantFirst(want ...uint32) func([][]uint32) string {
	return func(out [][]uint32) string {
		if got := out[0][:len(want)]; !slices.Equal(got, want) {
			return fmt.Sprintf("out[:%d] = %v, want %v", len(want), got, want)
		}
		return ""
	}
}

// wantAnonymous checks result[0..filled) = j*10+1, result[filled] still the
// sentinel, and invocations = m.
func wantAnonymous(filled, m uint32) func([][]uint32) string {
	want := make([]uint32, filled+1)
	for j := range filled {
		want[j] = j*10 + 1
	}
	want[filled] = anonSentinel
	return func(out [][]uint32) string {
		if got := out[0][:filled+1]; !slices.Equal(got, want) || out[1][0] != m {
			return fmt.Sprintf("result[0..%d] = %#x, invocations = %d; want %#x, %d", filled+1, got, out[1][0], want, m)
		}
		return ""
	}
}

func formatWords(out [][]uint32) string {
	parts := make([]string, len(out))
	for i, w := range out {
		parts[i] = fmt.Sprintf("binding%d=%#x", i, w)
	}
	return strings.Join(parts, " ")
}
