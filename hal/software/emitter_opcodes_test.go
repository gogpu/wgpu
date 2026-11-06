//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// emitterFamilies holds one WGSL module per builtin family. Each goes through
// the device's own path (naga with spirvOptions, then shader.ParseModule),
// and ParseModule rejects any function-body opcode the interpreter does not
// execute (ADR-078 rule 6). So a family that parses emits only opcodes the
// interpreter has a case for; a naga change that emits a new opcode for one
// of these builtins fails here, naming the opcode.
var emitterFamilies = []struct{ name, wgsl string }{
	{"arithmetic", `
@group(0) @binding(0) var<storage, read_write> b: array<u32>;
@compute @workgroup_size(1) fn main() {
	let u = vec3<u32>(b[0], b[1], b[2]);
	let i = vec3<i32>(u);
	let f = vec3<f32>(u);
	let m = mat3x3<f32>(f, f.yzx, f.zxy);
	let r = (u + u - u * u) / max(u, vec3<u32>(1u)) % (u | vec3<u32>(1u)) ^ (u & u) << (u >> vec3<u32>(1u));
	let s = (i + i - i * i) / i % i >> vec3<u32>(2u);
	let g = (f + f - f * f) / f % f * m * 2.0 + m * f + (m * m * 0.5)[1] - -f;
	let c = select(r, ~u, u < r | u >= r & !(u == r));
	let k = vec3<f32>(s) + bitcast<vec3<f32>>(c) + vec3<f32>(f < g) + g * transpose(m);
	b[3] = c.x + u32(s.y) + u32(k.z) + u32(any(i > s)) + u32(all(f != g)) + u32(-i.x) + bitcast<u32>(f.x);
	b[4] = u32(dot(f, g)) + u32(f32(i.x)) + u32(i32(f.y));
}`},
	{"integer_builtins", `
@group(0) @binding(0) var<storage, read_write> b: array<u32>;
@compute @workgroup_size(1) fn main() {
	let u = vec2<u32>(b[0], b[1]);
	let i = vec2<i32>(u);
	let x = countOneBits(u) + reverseBits(u) + extractBits(u, 1u, 3u) + insertBits(u, u, 2u, 4u)
		+ firstLeadingBit(u) + firstTrailingBit(u) + countLeadingZeros(u) + countTrailingZeros(u)
		+ min(u, u) + max(u, u) + clamp(u, u, u);
	let y = abs(i) + sign(i) + countOneBits(i) + reverseBits(i) + extractBits(i, 1u, 3u)
		+ insertBits(i, i, 2u, 4u) + firstLeadingBit(i) + min(i, i) + max(i, i) + clamp(i, i, i);
	b[2] = x.x + u32(y.y) + dot4U8Packed(b[0], b[1]) + u32(dot4I8Packed(b[0], b[1])) + dot(u, u) + u32(dot(i, i));
}`},
	{"float_builtins", `
@group(0) @binding(0) var<storage, read_write> b: array<f32>;
@compute @workgroup_size(1) fn main() {
	let v = vec3<f32>(b[0], b[1], b[2]);
	let m = mat2x2<f32>(v.xy, v.yz);
	let a = sin(v) + cos(v) + tan(v) + asin(v) + acos(v) + atan(v) + atan2(v, v) + sinh(v) + cosh(v) + tanh(v)
		+ asinh(v) + acosh(v) + atanh(v) + exp(v) + exp2(v) + log(v) + log2(v) + pow(v, v) + sqrt(v)
		+ inverseSqrt(v) + abs(v) + sign(v) + floor(v) + ceil(v) + round(v) + trunc(v) + fract(v)
		+ min(v, v) + max(v, v) + clamp(v, v, v) + saturate(v) + mix(v, v, v) + step(v, v)
		+ smoothstep(v, v, v) + fma(v, v, v) + degrees(v) + radians(v) + normalize(v) + cross(v, v)
		+ reflect(v, v) + refract(v, v, 0.5) + faceForward(v, v, v) + ldexp(v, vec3<i32>(2))
		+ quantizeToF16(v) + modf(v).fract + frexp(v).fract + vec3<f32>(frexp(v).exp);
	b[3] = a.x + length(v) + distance(v, v) + dot(v, v) + determinant(m) + modf(b[0]).whole
		+ f32(isnan(b[0])) + f32(isinf(b[1]));
}`},
	{"packing", `
@group(0) @binding(0) var<storage, read_write> b: array<u32>;
@compute @workgroup_size(1) fn main() {
	let v4 = unpack4x8snorm(b[0]) + unpack4x8unorm(b[1]);
	let v2 = unpack2x16snorm(b[2]) + unpack2x16unorm(b[3]) + unpack2x16float(b[4]);
	b[5] = pack4x8snorm(v4) + pack4x8unorm(v4) + pack2x16snorm(v2) + pack2x16unorm(v2) + pack2x16float(v2)
		+ pack4xI8(vec4<i32>(unpack4xI8(b[6]))) + pack4xU8(unpack4xU8(b[7]))
		+ pack4xI8Clamp(vec4<i32>(1)) + pack4xU8Clamp(vec4<u32>(1));
}`},
	{"derivatives_and_discard", `
@fragment fn main(@location(0) p: vec2<f32>) -> @location(0) vec4<f32> {
	if (p.x < 0.0) { discard; }
	let d = dpdx(p) + dpdy(p) + fwidth(p) + dpdxFine(p) + dpdyFine(p) + fwidthFine(p)
		+ dpdxCoarse(p) + dpdyCoarse(p) + fwidthCoarse(p);
	return vec4<f32>(d, 0.0, 1.0);
}`},
	{"textures", `
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var ta: texture_2d_array<f32>;
@group(0) @binding(2) var tc: texture_cube<f32>;
@group(0) @binding(3) var t3: texture_3d<f32>;
@group(0) @binding(4) var tu: texture_2d<u32>;
@group(0) @binding(5) var td: texture_depth_2d;
@group(0) @binding(6) var tm: texture_multisampled_2d<f32>;
@group(0) @binding(7) var ts: texture_storage_2d<rgba8unorm, write>;
@group(0) @binding(8) var s: sampler;
@group(0) @binding(9) var sc: sampler_comparison;
@fragment fn main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let c = vec2<i32>(uv * 4.0);
	var r = textureLoad(t, c, 0) + textureLoad(ta, c, 1, 0) + textureLoad(t3, vec3<i32>(c, 0), 0)
		+ vec4<f32>(textureLoad(tu, c, 0)) + textureLoad(tm, c, 0) + vec4<f32>(textureLoad(td, c, 0));
	r += textureSample(t, s, uv) + textureSample(t, s, uv, vec2<i32>(1, 0)) + textureSample(ta, s, uv, 1)
		+ textureSample(tc, s, vec3<f32>(uv, 1.0)) + textureSample(t3, s, vec3<f32>(uv, 0.5))
		+ textureSampleBias(t, s, uv, 0.5) + textureSampleLevel(t, s, uv, 1.0)
		+ textureSampleGrad(t, s, uv, vec2<f32>(0.1), vec2<f32>(0.1)) + textureSampleBaseClampToEdge(t, s, uv)
		+ textureGather(1, t, s, uv) + textureGatherCompare(td, sc, uv, 0.5)
		+ vec4<f32>(textureSampleCompare(td, sc, uv, 0.5)) + vec4<f32>(textureSampleCompareLevel(td, sc, uv, 0.5));
	let n = textureDimensions(t) + textureDimensions(t, 1) + textureDimensions(ta).xy + textureDimensions(t3).xy
		+ vec2<u32>(textureNumLevels(t), textureNumLayers(ta)) + vec2<u32>(textureNumSamples(tm));
	textureStore(ts, c, r);
	return r + vec4<f32>(vec2<f32>(n), 0.0, 0.0);
}`},
	{"atomics", `
@group(0) @binding(0) var<storage, read_write> a: array<atomic<u32>, 4>;
@group(0) @binding(1) var<storage, read_write> i: atomic<i32>;
@compute @workgroup_size(1) fn main() {
	let x = atomicAdd(&a[0], 1u) + atomicSub(&a[0], 1u) + atomicMax(&a[1], 2u) + atomicMin(&a[1], 1u)
		+ atomicAnd(&a[2], 3u) + atomicOr(&a[2], 4u) + atomicXor(&a[2], 5u) + atomicExchange(&a[3], 6u)
		+ atomicCompareExchangeWeak(&a[3], 6u, 7u).old_value + atomicLoad(&a[0]);
	let y = atomicMax(&i, -1) + atomicMin(&i, 1) + atomicAdd(&i, 2);
	atomicStore(&a[1], x + u32(y));
}`},
	{"workgroup_and_barriers", `
@group(0) @binding(0) var<storage, read_write> b: array<u32>;
var<workgroup> shared_data: array<u32, 64>;
var<workgroup> flag: u32;
@compute @workgroup_size(64) fn main(@builtin(local_invocation_index) li: u32, @builtin(workgroup_id) wg: vec3<u32>,
	@builtin(global_invocation_id) gid: vec3<u32>, @builtin(num_workgroups) nw: vec3<u32>) {
	shared_data[li] = b[gid.x];
	workgroupBarrier();
	storageBarrier();
	let f = workgroupUniformLoad(&flag);
	b[gid.x] = shared_data[63u - li] + wg.x + nw.x + f + arrayLength(&b);
}`},
	{"loops_and_switch", `
@group(0) @binding(0) var<storage, read_write> b: array<i32>;
// The pointer store is spelled out: naga v0.21.3 rejects the valid compound
// form *p += x ("Operation Add can't work with Pointer").
fn helper(x: i32, p: ptr<function, i32>) -> i32 { *p = *p + x; return x * 2; }
@compute @workgroup_size(1) fn main() {
	var acc = 0;
	for (var k = 0; k < b[0]; k++) {
		if (k == 3) { continue; }
		acc += k;
	}
	var j = 0;
	while (j < b[1]) { j += 1; if (j > 100) { break; } }
	loop {
		acc -= 1;
		continuing { break if acc < 0; }
	}
	switch b[2] {
		case 0, 1: { acc += 10; }
		case 2: { acc += helper(acc, &j); }
		default: { acc = -acc; }
	}
	b[3] = acc + j;
}`},
}

// TestEmitterOpcodesSupported compiles each builtin family through the
// software device and checks that its SPIR-V parses: every function-body
// opcode naga emits for it is one the interpreter executes.
func TestEmitterOpcodesSupported(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	for _, fam := range emitterFamilies {
		t.Run(fam.name, func(t *testing.T) {
			hm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
				Label:  fam.name,
				Source: hal.ShaderSource{WGSL: fam.wgsl},
			})
			if err != nil {
				t.Fatalf("CreateShaderModule: %v", err)
			}
			sm := hm.(*ShaderModule)
			if sm.err != nil {
				t.Fatalf("module error: %v", sm.err)
			}
			if sm.parsed == nil {
				t.Fatal("module has no parsed SPIR-V")
			}
		})
	}
}
