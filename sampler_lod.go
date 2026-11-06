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

// anisotropyDefault is the WebGPU default of GPUSamplerDescriptor.maxAnisotropy
// (index.bs): anisotropic filtering off.
const anisotropyDefault uint16 = 1

// normalizeAnisotropy maps the zero value of SamplerDescriptor.Anisotropy to
// the WebGPU default 1. The spec has no maxAnisotropy 0 (validation requires
// at least 1), so the Go zero can only mean "not set". Every CreateSampler
// path calls it, so the backends, wgpu-native and the browser get the same
// value; it was already the behavior of each path (core validation treated 0
// as 1, go-webgpu sends 1 to wgpu-native, the browser omits it).
func normalizeAnisotropy(v uint16) uint16 {
	if v == 0 {
		return anisotropyDefault
	}
	return v
}
