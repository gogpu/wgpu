// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT
struct UniformData {
    particleCount: u32,
    gridWidth: u32,
    gridHeight: u32,
};

@group(0) @binding(0) var<uniform> globalData: UniformData;

struct Out {
    @builtin(position) pos: vec4<f32>,
    @location(0) col: vec3<f32>,
}

@vertex
fn vs_main(
    @builtin(vertex_index) vid: u32, 
    @builtin(instance_index) particle_id: u32,
    @location(0) center: vec2<f32>
) -> Out {
    var o: Out;

    let ndc_x = (center.x / f32(globalData.gridWidth)) * 2.0 - 1.0;
    let ndc_y = (center.y / f32(globalData.gridHeight)) * 2.0 - 1.0;
    let mass = 1+(particle_id & 7u);
    let sz = pow(f32(mass), 0.3333) * 1.5;
    let x = f32(vid & 1u) * 2.0 - 1.0;
    let y = f32((vid >> 1u) & 1u) * 2.0 - 1.0;
    let sizex = 0.0009*sz;
    let sizey = 0.0016*sz;
    o.pos = vec4<f32>(ndc_x + x * sizex, ndc_y + y * sizey, 0.0, 1.0);
    switch mass {
        case 1: { o.col = vec3<f32>(0.8, 0.9, 1.0); }
        case 2: { o.col = vec3<f32>(0.6, 0.8, 1.0); }
        case 3: { o.col = vec3<f32>(0.4, 0.9, 1.0); }
        case 4: { o.col = vec3<f32>(0.0, 1.0, 1.0); }
        case 5: { o.col = vec3<f32>(0.2, 1.0, 0.2); }
        case 6: { o.col = vec3<f32>(0.5, 1.0, 0.0); }
        case 7: { o.col = vec3<f32>(1.0, 0.8, 0.0); }
        default:{ o.col = vec3<f32>(1.0, 0.0, 0.0); }
    }
    return o;
}

@fragment
fn fs_main(@location(0) col: vec3<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(col, 1.0);
}
