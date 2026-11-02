// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// These tests prove the subgroup features by readback on every backend and
// adapter that offers them (P1-CORE-012): a backend offers
// FeatureSubgroupOperations, FeatureSubgroupBarrier or FeatureSubgroupVertex
// only where these tests pass on it.

// subgroupRustShader is Rust wgpu's GPU test of the subgroup operations,
// unchanged: gfx-rs/wgpu tests/tests/wgpu-gpu/subgroup_operations/shader.wgsl
// (test SUBGROUP_OPERATIONS, commit 14328f49918cce19f7cc8bf703435968c8293035,
// MIT OR Apache-2.0). Every invocation of one 128-wide workgroup sets bit n
// of its mask when check n passes; the checks compute their expected values
// from subgroup_size and subgroup_invocation_id.
const subgroupRustShader = `enable subgroups;

@group(0)
@binding(0)
var<storage, read_write> storage_buffer: array<vec2<u32>>;

var<workgroup> workgroup_buffer: u32;

fn add_result_to_mask(mask: ptr<function, vec2<u32>>, index: u32, value: bool) {
   (*mask)[index / 32u] |= u32(value) << (index % 32u);
}

@compute
@workgroup_size(128)
fn main(
    @builtin(global_invocation_id) global_id: vec3<u32>,
    @builtin(num_subgroups) num_subgroups: u32,
    @builtin(subgroup_id) subgroup_id: u32,
    @builtin(subgroup_size) subgroup_size: u32,
    @builtin(subgroup_invocation_id) subgroup_invocation_id: u32,
) {
    var passed = vec2<u32>(0u);
    var expected: u32;

    add_result_to_mask(&passed, 0u, num_subgroups == 128u / subgroup_size);
    add_result_to_mask(&passed, 1u, subgroup_id == global_id.x / subgroup_size);
    add_result_to_mask(&passed, 2u, subgroup_invocation_id == global_id.x % subgroup_size);

    var expected_ballot = vec4<u32>(0u);
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected_ballot[i / 32u] |= ((global_id.x - subgroup_invocation_id + i) & 1u) << (i % 32u);
    }
    add_result_to_mask(&passed, 3u, dot(vec4<u32>(1u), vec4<u32>(subgroupBallot((subgroup_invocation_id & 1u) == 1u) == expected_ballot)) == 4u);

    add_result_to_mask(&passed, 4u, subgroupAll(true));
    add_result_to_mask(&passed, 5u, !subgroupAll(subgroup_invocation_id != 0u));

    add_result_to_mask(&passed, 6u, subgroupAny(subgroup_invocation_id == 0u));
    add_result_to_mask(&passed, 7u, !subgroupAny(false));

    expected = 0u;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected += global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 8u, subgroupAdd(global_id.x + 1u) == expected);

    expected = 1u;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected *= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 9u, subgroupMul(global_id.x + 1u) == expected);

    expected = 0u;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected = max(expected, global_id.x - subgroup_invocation_id + i + 1u);
    }
    add_result_to_mask(&passed, 10u, subgroupMax(global_id.x + 1u) == expected);

    expected = 0xFFFFFFFFu;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected = min(expected, global_id.x - subgroup_invocation_id + i + 1u);
    }
    add_result_to_mask(&passed, 11u, subgroupMin(global_id.x + 1u) == expected);

    expected = 0xFFFFFFFFu;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected &= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 12u, subgroupAnd(global_id.x + 1u) == expected);

    expected = 0u;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected |= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 13u, subgroupOr(global_id.x + 1u) == expected);

    expected = 0u;
    for(var i = 0u; i < subgroup_size; i += 1u) {
        expected ^= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 14u, subgroupXor(global_id.x + 1u) == expected);

    expected = 0u;
    for(var i = 0u; i < subgroup_invocation_id; i += 1u) {
        expected += global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 15u, subgroupExclusiveAdd(global_id.x + 1u) == expected);

    expected = 1u;
    for(var i = 0u; i < subgroup_invocation_id; i += 1u) {
        expected *= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 16u, subgroupExclusiveMul(global_id.x + 1u) == expected);

    expected = 0u;
    for(var i = 0u; i <= subgroup_invocation_id; i += 1u) {
        expected += global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 17u, subgroupInclusiveAdd(global_id.x + 1u) == expected);

    expected = 1u;
    for(var i = 0u; i <= subgroup_invocation_id; i += 1u) {
        expected *= global_id.x - subgroup_invocation_id + i + 1u;
    }
    add_result_to_mask(&passed, 18u, subgroupInclusiveMul(global_id.x + 1u) == expected);

    add_result_to_mask(&passed, 19u, subgroupBroadcastFirst(u32(subgroup_invocation_id != 0u)) == 0u);
    add_result_to_mask(&passed, 20u, subgroupBroadcastFirst(u32(subgroup_invocation_id == 0u)) == 1u);
    add_result_to_mask(&passed, 21u, subgroupBroadcast(subgroup_invocation_id, 1u) == 1u);
    add_result_to_mask(&passed, 22u, subgroupShuffle(subgroup_invocation_id, subgroup_invocation_id) == subgroup_invocation_id);
    add_result_to_mask(&passed, 23u, subgroupShuffle(subgroup_invocation_id, subgroup_size - 1u - subgroup_invocation_id) == subgroup_size - 1u - subgroup_invocation_id);
    add_result_to_mask(&passed, 24u, subgroupShuffleDown(subgroup_invocation_id, 1u) == subgroup_invocation_id + 1u || subgroup_invocation_id == subgroup_size - 1u);
    add_result_to_mask(&passed, 25u, subgroupShuffleUp(subgroup_invocation_id, 1u) == subgroup_invocation_id - 1u || subgroup_invocation_id == 0u);
    add_result_to_mask(&passed, 26u, subgroupShuffleXor(subgroup_invocation_id, subgroup_size - 1u) == (subgroup_invocation_id ^ (subgroup_size - 1u)));

    // Mac/Apple will fail this test.
    var passed_27 = false;
    if subgroup_invocation_id % 2u == 0u {
        passed_27 |= subgroupAdd(1u) == (subgroup_size / 2u);
    } else {
        passed_27 |= subgroupAdd(1u) == (subgroup_size / 2u);
    }
    add_result_to_mask(&passed, 27u, passed_27);

    // Mac/Apple will fail this test.
    var passed_28 = false;
    switch subgroup_invocation_id % 3u {
        case 0u: {
            passed_28 = subgroupBroadcastFirst(subgroup_invocation_id) == 0u;
        }
        case 1u: {
            passed_28 = subgroupBroadcastFirst(subgroup_invocation_id) == 1u;
        }
        case 2u: {
            passed_28 = subgroupBroadcastFirst(subgroup_invocation_id) == 2u;
        }
        default {  }
    }
    add_result_to_mask(&passed, 28u, passed_28);

    // Mac/Apple will sometimes fail this test. MacOS 14.3 passes it, so the bug in the metal compiler seems to be fixed.
    expected = 0u;
    for (var i = subgroup_size; i >= 0u; i -= 1u) {
        expected = subgroupAdd(1u);
        if i == subgroup_invocation_id {
            break;
        }
    }
    add_result_to_mask(&passed, 29u, expected == (subgroup_invocation_id + 1u));

    if global_id.x == 0u {
        workgroup_buffer = subgroup_size;
    }
    workgroupBarrier();
    add_result_to_mask(&passed, 30u, workgroup_buffer == subgroup_size);

    add_result_to_mask(&passed, 31u, quadBroadcast(quadBroadcast(subgroup_invocation_id, 0u) ^ subgroup_invocation_id, 0u) == 0u);
    add_result_to_mask(&passed, 32u, quadBroadcast(quadBroadcast(subgroup_invocation_id, 1u) ^ quadSwapX(subgroup_invocation_id), 0u) == 0u);
    add_result_to_mask(&passed, 33u, quadBroadcast(quadBroadcast(subgroup_invocation_id, 2u) ^ quadSwapY(subgroup_invocation_id), 0u) == 0u);
    add_result_to_mask(&passed, 34u, quadBroadcast(quadBroadcast(subgroup_invocation_id, 3u) ^ quadSwapDiagonal(subgroup_invocation_id), 0u) == 0u);
    add_result_to_mask(&passed, 35u, quadSwapX(quadSwapY(subgroup_invocation_id)) == quadSwapDiagonal(subgroup_invocation_id));

    // Keep this test last, verify we are still convergent after running other tests
    add_result_to_mask(&passed, 36u, subgroupAdd(1u) == subgroup_size);

    // Increment TEST_COUNT in subgroup_operations/mod.rs if adding more tests

    storage_buffer[global_id.x] = passed;
}
`

// subgroupRustThreads and subgroupRustChecks are THREAD_COUNT and
// TEST_COUNT of the Rust test (subgroup_operations/mod.rs).
const (
	subgroupRustThreads = 128
	subgroupRustChecks  = 37
)

// TestFeatureSubgroupOperationsRust runs Rust wgpu's subgroup GPU test and
// expects every check to pass on every invocation, as Rust does on Vulkan
// and DX12.
func TestFeatureSubgroupOperationsRust(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureSubgroupOperations), gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		out, err := runPublicCompute(t, device, subgroupRustShader, nil, [][]uint32{make([]uint32, subgroupRustThreads*2)})
		if err != nil {
			t.Fatal(err)
		}
		const want = uint64(1)<<subgroupRustChecks - 1
		var failures []string
		for thread := 0; thread < subgroupRustThreads; thread++ {
			got := uint64(out[0][2*thread]) | uint64(out[0][2*thread+1])<<32
			if missing := want &^ got; missing != 0 {
				failures = append(failures, fmt.Sprintf("thread %d failed checks %v", thread, maskBits(missing)))
			}
		}
		if len(failures) > 0 {
			t.Fatalf("%d of %d threads failed:\n%s", len(failures), subgroupRustThreads, strings.Join(failures, "\n"))
		}
	})
}

func maskBits(mask uint64) []int {
	var out []int
	for mask != 0 {
		b := bits.TrailingZeros64(mask)
		out = append(out, b)
		mask &^= 1 << b
	}
	return out
}

// subgroupValuesShader writes the raw results of the subgroup operations,
// one record per invocation, for the host to check. The host groups the
// invocations by the subgroup they report (the local index
// subgroupBroadcastFirst returns), so it assumes neither which local
// indices share a subgroup nor that subgroups are full.
const subgroupValuesShader = `enable subgroups;

@group(0) @binding(0) var<storage, read_write> results: array<u32>;

@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) lid: u32,
        @builtin(subgroup_invocation_id) sid: u32,
        @builtin(subgroup_size) size: u32) {
    let base = lid * 12u;
    let ballot = subgroupBallot(lid % 2u == 1u);
    results[base + 0u] = size;
    results[base + 1u] = sid;
    results[base + 2u] = subgroupBroadcastFirst(lid);
    results[base + 3u] = subgroupAdd(lid + 1u);
    results[base + 4u] = ballot.x;
    results[base + 5u] = ballot.y;
    results[base + 6u] = ballot.z;
    results[base + 7u] = ballot.w;
    results[base + 8u] = subgroupShuffle(lid, sid ^ 1u);
    results[base + 9u] = quadBroadcast(lid, 2u);
    results[base + 10u] = select(0u, 1u, subgroupAny(lid == 5u));
    results[base + 11u] = subgroupExclusiveAdd(1u);
}
`

const (
	subgroupValuesInvocations = 64
	subgroupValuesWords       = 12
)

// subgroupRecord is one invocation's record from subgroupValuesShader.
type subgroupRecord struct {
	lid, size, sid, first, add uint32
	ballot                     [4]uint32
	shuffle, quad, anyFive     uint32
	exclusiveAdd               uint32
}

// TestFeatureSubgroupOperationsValues checks the raw values of
// subgroupBroadcastFirst, subgroupAdd, subgroupBallot, subgroupShuffle,
// quadBroadcast, subgroupAny, subgroupExclusiveAdd and the subgroup_size and
// subgroup_invocation_id builtins against what the host computes from the
// subgroups the invocations report.
func TestFeatureSubgroupOperationsValues(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureSubgroupOperations), gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		out, err := runPublicCompute(t, device, subgroupValuesShader, nil, [][]uint32{make([]uint32, subgroupValuesInvocations*subgroupValuesWords)})
		if err != nil {
			t.Fatal(err)
		}
		groups := map[uint32][]subgroupRecord{}
		for lid := uint32(0); lid < subgroupValuesInvocations; lid++ {
			w := out[0][lid*subgroupValuesWords:]
			r := subgroupRecord{
				lid: lid, size: w[0], sid: w[1], first: w[2], add: w[3],
				ballot:  [4]uint32{w[4], w[5], w[6], w[7]},
				shuffle: w[8], quad: w[9], anyFive: w[10], exclusiveAdd: w[11],
			}
			groups[r.first] = append(groups[r.first], r)
		}
		sizes := make([]string, 0, len(groups))
		for _, g := range groups {
			sizes = append(sizes, fmt.Sprintf("%d/%d", len(g), g[0].size))
			checkSubgroup(t, g)
		}
		sort.Strings(sizes)
		t.Logf("%d subgroups (active/size): %v", len(groups), sizes)
	})
}

// checkSubgroup checks the records of one subgroup's invocations.
func checkSubgroup(t *testing.T, g []subgroupRecord) {
	t.Helper()
	size := g[0].size
	if size == 0 || size > 128 || size&(size-1) != 0 {
		t.Fatalf("subgroup_size %d: want a power of two up to 128", size)
	}
	if uint32(len(g)) > size {
		t.Fatalf("%d invocations report the subgroup of local index %d, its size is %d", len(g), g[0].first, size)
	}
	bySID := map[uint32]subgroupRecord{}
	var want subgroupRecord
	lowest := g[0]
	for _, r := range g {
		if r.size != size {
			t.Fatalf("local index %d: subgroup_size %d, another invocation of its subgroup reports %d", r.lid, r.size, size)
		}
		if _, dup := bySID[r.sid]; dup || r.sid >= size {
			t.Fatalf("local index %d: subgroup_invocation_id %d is repeated or not below the size %d", r.lid, r.sid, size)
		}
		bySID[r.sid] = r
		if r.sid < lowest.sid {
			lowest = r
		}
		want.add += r.lid + 1
		if r.lid%2 == 1 {
			want.ballot[r.sid/32] |= 1 << (r.sid % 32)
		}
		if r.lid == 5 {
			want.anyFive = 1
		}
	}
	for _, r := range g {
		if r.first != lowest.lid {
			t.Errorf("local index %d: subgroupBroadcastFirst = %d, want %d (the lowest subgroup_invocation_id)", r.lid, r.first, lowest.lid)
		}
		if r.add != want.add {
			t.Errorf("local index %d: subgroupAdd = %d, want %d", r.lid, r.add, want.add)
		}
		if r.ballot != want.ballot {
			t.Errorf("local index %d: subgroupBallot = %#x, want %#x", r.lid, r.ballot, want.ballot)
		}
		if r.anyFive != want.anyFive {
			t.Errorf("local index %d: subgroupAny(lid == 5) = %d, want %d", r.lid, r.anyFive, want.anyFive)
		}
		var lower uint32
		for sid := range bySID {
			if sid < r.sid {
				lower++
			}
		}
		if r.exclusiveAdd != lower {
			t.Errorf("local index %d: subgroupExclusiveAdd(1) = %d, want %d", r.lid, r.exclusiveAdd, lower)
		}
		if p, ok := bySID[r.sid^1]; ok && r.shuffle != p.lid {
			t.Errorf("local index %d: subgroupShuffle(lid, sid^1) = %d, want %d", r.lid, r.shuffle, p.lid)
		}
		if q, ok := bySID[r.sid&^3|2]; ok && r.quad != q.lid {
			t.Errorf("local index %d: quadBroadcast(lid, 2) = %d, want %d", r.lid, r.quad, q.lid)
		}
	}
}

// subgroupBarrierShader exchanges values between invocation pairs of a
// subgroup through workgroup memory, ordered by subgroupBarrier.
const subgroupBarrierShader = `enable subgroups;

@group(0) @binding(0) var<storage, read_write> results: array<u32>;

var<workgroup> exchange: array<u32, 64>;

@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) lid: u32,
        @builtin(subgroup_invocation_id) sid: u32) {
    let partner = subgroupShuffle(lid, sid ^ 1u);
    exchange[lid] = lid * 3u + 7u;
    subgroupBarrier();
    results[lid * 2u] = partner;
    results[lid * 2u + 1u] = exchange[partner];
}
`

// TestFeatureSubgroupBarrier reads, after subgroupBarrier, the workgroup
// memory word another invocation of the subgroup wrote before it.
//
// It skips everywhere for now: no backend offers FeatureSubgroupBarrier.
// Vulkan holds it back, unlike Rust, because naga compiles subgroupBarrier
// to SPIR-V the validation layer rejects
// (VUID-StandaloneSpirv-MemorySemantics-10870; see subgroupFeatures in
// hal/vulkan). Once naga emits a valid barrier and Vulkan offers the
// feature, this test runs and must pass clean.
func TestFeatureSubgroupBarrier(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureSubgroupOperations) | gputypes.Features(gputypes.FeatureSubgroupBarrier)
	forEachFeatureDevice(t, features, gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		out, err := runPublicCompute(t, device, subgroupBarrierShader, nil, [][]uint32{make([]uint32, 2*subgroupValuesInvocations)})
		if err != nil {
			t.Fatal(err)
		}
		for lid := uint32(0); lid < subgroupValuesInvocations; lid++ {
			partner, value := out[0][2*lid], out[0][2*lid+1]
			if partner >= subgroupValuesInvocations || partner == lid || out[0][2*partner] != lid {
				t.Fatalf("local index %d: shuffle partner %d is not a distinct invocation paired back with it", lid, partner)
			}
			if want := partner*3 + 7; value != want {
				t.Errorf("local index %d: read %d from its partner %d after subgroupBarrier, want %d", lid, value, partner, want)
			}
		}
	})
}

// subgroupFragmentShader runs subgroup and quad operations in the fragment
// stage over a full-screen triangle, so every 2x2 quad is fully covered.
// Red and green are the quad neighbours' x and y, blue the number of active
// invocations (0 when subgroupBallot disagrees with subgroupAdd), alpha the
// subgroup size.
const subgroupFragmentShader = `enable subgroups;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return vec4f(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(position) pos: vec4f, @builtin(subgroup_size) size: u32) -> @location(0) vec4f {
    let x = u32(pos.x);
    let y = u32(pos.y);
    let active = subgroupAdd(1u);
    let b = subgroupBallot(true);
    let ballotActive = countOneBits(b.x) + countOneBits(b.y) + countOneBits(b.z) + countOneBits(b.w);
    let checked = select(0u, active, ballotActive == active);
    return vec4f(f32(quadSwapX(x)), f32(quadSwapY(y)), f32(checked), f32(size)) / 255.0;
}
`

// TestFeatureSubgroupFragment checks quadSwapX and quadSwapY against the
// pixel grid (a quad is an aligned 2x2 block), and subgroupAdd and
// subgroupBallot against each other and the subgroup size, in the fragment
// stage that Rust's SUBGROUP covers.
func TestFeatureSubgroupFragment(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureSubgroupOperations), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, subgroupFragmentShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		})
		for y := 0; y < featureTargetHeight; y++ {
			for x := 0; x < featureTargetWidth; x++ {
				p := px[y*featureTargetWidth+x]
				swapX, swapY, active, size := p&0xff, p>>8&0xff, p>>16&0xff, p>>24
				if swapX != uint32(x^1) || swapY != uint32(y^1) {
					t.Fatalf("pixel (%d,%d): quadSwapX/Y = (%d,%d), want (%d,%d)", x, y, swapX, swapY, x^1, y^1)
				}
				if size < 4 || size&(size-1) != 0 {
					t.Fatalf("pixel (%d,%d): subgroup_size %d, want a power of two of at least 4", x, y, size)
				}
				if active < 4 || active > size || active%4 != 0 {
					t.Fatalf("pixel (%d,%d): %d active invocations (0: ballot and add disagree), want whole quads up to the size %d", x, y, active, size)
				}
			}
		}
	})
}

// subgroupVertexShader runs subgroup operations in the vertex stage and
// passes the result to the pixels: the number of active invocations, or 0
// when subgroupBallot disagrees with subgroupAdd.
const subgroupVertexShader = `enable subgroups;

struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) @interpolate(flat) active: u32,
}

@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    let active = subgroupAdd(1u);
    let b = subgroupBallot(true);
    let ballotActive = countOneBits(b.x) + countOneBits(b.y) + countOneBits(b.z) + countOneBits(b.w);
    return VOut(vec4f(p[i], 0.0, 1.0), select(0u, active, ballotActive == active));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4f {
    return vec4f(f32(in.active) / 255.0, 0.0, 0.0, 1.0);
}
`

// TestFeatureSubgroupVertex runs subgroupAdd and subgroupBallot in the
// vertex stage and checks that they agree on a non-zero count of active
// invocations. FeatureSubgroupVertex alone grants them, as Rust's
// SUBGROUP_VERTEX grants naga's SUBGROUP capability.
func TestFeatureSubgroupVertex(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureSubgroupVertex), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, subgroupVertexShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		})
		active := red(px[0])
		if active == 0 {
			t.Fatalf("vertex stage: subgroupAdd(1) and subgroupBallot(true) disagree or count 0 (pixel %#08x)", px[0])
		}
		for i, p := range px {
			if red(p) != active {
				t.Fatalf("pixel %d = %#08x, want red %d as at pixel 0 (one flat value per triangle)", i, p, active)
			}
		}
		t.Logf("vertex stage: %d active invocations in the provoking vertex's subgroup", active)
	})
}
