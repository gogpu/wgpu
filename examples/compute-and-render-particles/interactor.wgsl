// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT
const CellCap: u32 = 4u;
const Repulsion: f32 = 0.001;
const CentralGravity: f32 = 0.00002;
const DistEpsilon: f32 = 5e-5;
const Damping: f32 = 1-1e-4

struct UniformData {
    particleCount: u32,
    gridWidth: u32,
    gridHeight: u32,
};

@group(0) @binding(0) var<uniform> globalData: UniformData;
@group(0) @binding(2) var<storage, read_write> particles0: array<vec2<f32>>;
@group(0) @binding(3) var<storage, read> particles1: array<vec2<f32>>;

fn force(p1: vec2<f32>, p2:vec2<f32>) -> f32 {
    var a = p1 - p2;
    let dist2 = dot(a,a);
    if (dist2 < DistEpsilon || dist2 >= 1.0) { return 0f; }
    let bell = 2.0/(1.0 + dist2)-1.0;
    return bell*Repulsion;
}

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) global_id: vec3<u32>) {
    let thisId = global_id.x;
    let numParticles = globalData.particleCount;
    if (thisId >= numParticles) { return; }
    let p = particles1[thisId];

    // Calculate gravity towards the center of the grid
    let center = vec2<f32>(f32(globalData.gridWidth)/2, f32(globalData.gridHeight)/2);
    let toCenter = center - p;
    let distSq = dot(toCenter, toCenter);
    var f = vec2(0.0, 0.0);

    let mass = 1.0 + f32(thisId & 7u);
    if (distSq > 0.01) {
        f = f + normalize(toCenter) * CentralGravity * mass;
    }

    // Iterate over all particles
    for (var otherId = 0u; otherId < numParticles; otherId++) {
        if (otherId == thisId) { continue; }
        let otherP = particles1[otherId];

        // 3. Calculate the force with the other particles
        let fScalar = force(p, otherP);

        // Accumulate the force
        if (fScalar != 0.0) {
            let otherMass = 1.0 + f32(otherId & 7u);
            let dir = normalize(p - otherP);
            f = f + dir * fScalar * otherMass;
        }
    }
    // 4. Update the particle position
    let v = (p - particles0[thisId]) * Damping;
    particles0[thisId] = p + v + f/mass;
}
