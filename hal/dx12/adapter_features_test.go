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
