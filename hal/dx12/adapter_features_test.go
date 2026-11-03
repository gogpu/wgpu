//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestAdapterFeaturesShaderF16 pins ShaderF16 to Rust wgpu-hal's condition
// (dx12/adapter.rs: shader model >= 6.2 and
// D3D12_OPTIONS4.Native16BitShaderOpsSupported), and to the DXIL path:
// FXC compiles for shader model 5.1, which has no 16-bit types.
func TestAdapterFeaturesShaderF16(t *testing.T) {
	tests := []struct {
		name     string
		sm       d3d12.D3D_SHADER_MODEL
		native16 bool
		dxil     string
		want     bool
	}{
		{"DXIL SM 6.7 native 16-bit", d3d12.D3D_SHADER_MODEL_6_7, true, "1", true},
		{"DXIL SM 6.2 native 16-bit", d3d12.D3D_SHADER_MODEL_6_2, true, "1", true},
		{"DXIL SM 6.1", d3d12.D3D_SHADER_MODEL_6_1, true, "1", false},
		{"DXIL SM 6.0", d3d12.D3D_SHADER_MODEL_6_0, true, "1", false},
		{"DXIL without native 16-bit ops", d3d12.D3D_SHADER_MODEL_6_7, false, "1", false},
		{"FXC", d3d12.D3D_SHADER_MODEL_6_7, true, "", false},
		{"FXC explicit", d3d12.D3D_SHADER_MODEL_6_7, true, "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envDX12DXIL, tt.dxil)
			a := &Adapter{capabilities: AdapterCapabilities{
				FeatureLevel:                  d3d12.D3D_FEATURE_LEVEL_12_0,
				ShaderModel:                   tt.sm,
				Native16BitShaderOpsSupported: tt.native16,
			}}
			got := a.Features().Contains(gputypes.FeatureShaderF16)
			if got != tt.want {
				t.Errorf("ShaderF16 advertised = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAdapterFeaturesTimestampQuery pins TimestampQuery to Rust wgpu-hal,
// which offers it on every D3D12 adapter (dx12/adapter.rs): every feature
// level has timestamp query heaps, and both adapter paths open the same
// Device, whose passes write timestamps through EndQuery.
func TestAdapterFeaturesTimestampQuery(t *testing.T) {
	for _, fl := range []d3d12.D3D_FEATURE_LEVEL{d3d12.D3D_FEATURE_LEVEL_11_0, d3d12.D3D_FEATURE_LEVEL_12_0} {
		caps := AdapterCapabilities{FeatureLevel: fl}
		if !(&Adapter{capabilities: caps}).Features().Contains(gputypes.FeatureTimestampQuery) {
			t.Errorf("Adapter at feature level %#x does not advertise TimestampQuery", fl)
		}
		if !(&AdapterLegacy{capabilities: caps}).Features().Contains(gputypes.FeatureTimestampQuery) {
			t.Errorf("AdapterLegacy at feature level %#x does not advertise TimestampQuery", fl)
		}
	}
}

// TestAdapterFeaturesSubgroups pins the subgroup features off on the
// HLSL/FXC path for good: shader model 5.1 has no wave intrinsics, so a
// device that offered them there could not compile the shaders. Rust
// wgpu-hal offers SUBGROUP on SM >= 6.0 with D3D12_OPTIONS1.WaveOps
// (dx12/adapter.rs); for us that can only be the DXIL path, which offers
// none yet either: naga's DXIL output fails the DXIL validator for
// subgroupAny/subgroupAll, an unused subgroup built-in and a fragment
// subgroup_size input (hal/allbackends TestFeatureSubgroup*). When DXIL
// offers SubgroupOperations, its row here becomes that condition; the FXC
// rows stay empty.
func TestAdapterFeaturesSubgroups(t *testing.T) {
	subgroups := gputypes.Features(gputypes.FeatureSubgroupOperations) |
		gputypes.Features(gputypes.FeatureSubgroupBarrier) |
		gputypes.Features(gputypes.FeatureSubgroupVertex)
	// The most capable adapter: every condition Rust checks holds.
	caps := AdapterCapabilities{
		FeatureLevel:                  d3d12.D3D_FEATURE_LEVEL_12_2,
		ShaderModel:                   d3d12.D3D_SHADER_MODEL_6_7,
		Native16BitShaderOpsSupported: true,
	}
	for _, dxil := range []string{"", "0", "1"} {
		t.Setenv(envDX12DXIL, dxil)
		for name, got := range map[string]gputypes.Features{
			"Adapter":       (&Adapter{capabilities: caps}).Features() & subgroups,
			"AdapterLegacy": (&AdapterLegacy{capabilities: caps}).Features() & subgroups,
		} {
			if got != 0 {
				t.Errorf("%s with GOGPU_DX12_DXIL=%q offers subgroup features %v, want none", name, dxil, got)
			}
		}
	}
}

// TestAdapterFeaturesShaderCapabilities pins the shader capability features
// DX12 offers: primitive index and clip distances on the HLSL/FXC path (clip
// distances a deviation from Rust after Dawn), none on the DXIL path until
// naga's DXIL output passes the readback tests (P1-API-008).
func TestAdapterFeaturesShaderCapabilities(t *testing.T) {
	both := gputypes.Features(gputypes.FeaturePrimitiveIndex) | gputypes.Features(gputypes.FeatureClipDistances)
	caps := AdapterCapabilities{FeatureLevel: d3d12.D3D_FEATURE_LEVEL_12_0, ShaderModel: d3d12.D3D_SHADER_MODEL_6_7}
	for _, tt := range []struct {
		dxil string
		want gputypes.Features
	}{{"0", both}, {"", both}, {"1", 0}} {
		t.Setenv(envDX12DXIL, tt.dxil)
		for name, got := range map[string]gputypes.Features{
			"Adapter":       (&Adapter{capabilities: caps}).Features() & both,
			"AdapterLegacy": (&AdapterLegacy{capabilities: caps}).Features() & both,
		} {
			if got != tt.want {
				t.Errorf("%s with GOGPU_DX12_DXIL=%q: shader capability features %v, want %v", name, tt.dxil, got, tt.want)
			}
		}
	}
}
