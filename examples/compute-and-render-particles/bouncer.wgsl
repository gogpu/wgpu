// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT
struct UniformData {
    particleCount: u32,
    gridWidth: u32,
    gridHeight: u32,
};

@group(0) @binding(0) var<uniform> globalData: UniformData;
@group(0) @binding(1) var<storage, read_write> particles0: array<vec2<f32>>;
@group(0) @binding(2) var<storage, read_write> particles1: array<vec2<f32>>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) global_id: vec3<u32>) {
    let particleIdx = global_id.x;
    if (particleIdx==0 || particleIdx >= globalData.particleCount) { return; }

    let gridWidth = f32(globalData.gridWidth);
    let gridHeight = f32(globalData.gridHeight);
    var p1 = particles1[particleIdx];
    var p0 = particles0[particleIdx];

    // Check X bounds on particle1 (assuming p1 holds the state to check)
    // Note: grid width is 4.0f effectively for f32 comparison
    if (p1.x < 0.0) {
        p1.x = -p1.x;
        p0.x = -p0.x;
    } else if (p1.x > gridWidth) {
        p1.x = 2.0 * gridWidth - p1.x;
        p0.x = 2.0 * gridWidth - p0.x;
    }

    // Check Y bounds on particle1
    if (p1.y < 0.0) {
        p1.y = -p1.y;
        p0.y = -p0.y;
    } else if (p1.y > gridHeight) {
        p1.y = 2.0 * gridHeight - p1.y;
        p0.y = 2.0 * gridHeight - p0.y;
    }

    // The planet
    let center = vec2<f32>(gridWidth/2, gridHeight/2);
    let radius = gridHeight/8;
    let radiusSq = radius * radius;

    let diff1 = p1 - center;
    let distSq = dot(diff1, diff1);

    // Check if particle is inside the circular obstacle
    if (distSq < radiusSq && distSq > 0.01) {
        let refP1 = center + normalize(diff1) * radius;
        p1 = refP1 * 2 - p1;

        let diff0 = p0 - center;
        let refP0 = center + normalize(diff0) * radius;
        p0 = refP0 * 2 - p0;
    }

    particles0[particleIdx] = p0;
    particles1[particleIdx] = p1;
}
