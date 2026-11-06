package wgpu

// lodMaxClampDefault is the WebGPU default of GPUSamplerDescriptor.lodMaxClamp.
// It exceeds the mip level count of any texture a backend can create, so it
// clamps nothing.
const lodMaxClampDefault float32 = 32

// normalizeLodMaxClamp maps the zero value of SamplerDescriptor.LodMaxClamp to
// the WebGPU default, so a zero-value descriptor samples the whole mip chain.
//
// Every CreateSampler path (native, -tags rust, js/wasm) calls it once before
// validation and before the value reaches a backend. The HAL, wgpu-native and
// the browser take the value literally, as the spec and Rust wgpu do, so the
// rule has to live here: none of them treats 0 as "no clamp" on its own.
func normalizeLodMaxClamp(v float32) float32 {
	if v == 0 {
		return lodMaxClampDefault
	}
	return v
}
