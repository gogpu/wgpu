package types

import "testing"

func TestBackendString(t *testing.T) {
	tests := []struct {
		backend Backend
		want    string
	}{
		{BackendEmpty, "Empty"},
		{BackendVulkan, "Vulkan"},
		{BackendMetal, "Metal"},
		{BackendDX12, "DX12"},
		{BackendGL, "GL"},
		{BackendBrowserWebGPU, "BrowserWebGPU"},
		{Backend(99), "Backend(99)"},
	}

	for _, tt := range tests {
		if got := tt.backend.String(); got != tt.want {
			t.Errorf("Backend(%d).String() = %q, want %q", tt.backend, got, tt.want)
		}
	}
}

func TestBackendsContains(t *testing.T) {
	tests := []struct {
		backends Backends
		backend  Backend
		want     bool
	}{
		{BackendsVulkan, BackendVulkan, true},
		{BackendsVulkan, BackendMetal, false},
		{BackendsPrimary, BackendVulkan, true},
		{BackendsPrimary, BackendMetal, true},
		{BackendsPrimary, BackendGL, false},
		{BackendsAll, BackendGL, true},
		{BackendsAll, BackendEmpty, false},
	}

	for _, tt := range tests {
		if got := tt.backends.Contains(tt.backend); got != tt.want {
			t.Errorf("Backends(%d).Contains(%d) = %v, want %v", tt.backends, tt.backend, got, tt.want)
		}
	}
}

func TestDeviceTypeString(t *testing.T) {
	tests := []struct {
		dt   DeviceType
		want string
	}{
		{DeviceTypeOther, "Other"},
		{DeviceTypeIntegratedGPU, "IntegratedGpu"},
		{DeviceTypeDiscreteGPU, "DiscreteGpu"},
		{DeviceTypeVirtualGPU, "VirtualGpu"},
		{DeviceTypeCPU, "Cpu"},
		{DeviceType(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.dt.String(); got != tt.want {
			t.Errorf("DeviceType(%d).String() = %q, want %q", tt.dt, got, tt.want)
		}
	}
}

func TestFeaturesContains(t *testing.T) {
	f := Features(FeatureDepthClipControl | FeatureTimestampQuery)

	if !f.Contains(FeatureDepthClipControl) {
		t.Error("Features should contain FeatureDepthClipControl")
	}
	if !f.Contains(FeatureTimestampQuery) {
		t.Error("Features should contain FeatureTimestampQuery")
	}
	if f.Contains(FeatureShaderF16) {
		t.Error("Features should not contain FeatureShaderF16")
	}
}

func TestFeaturesInsertRemove(t *testing.T) {
	var f Features

	f.Insert(FeatureShaderF16)
	if !f.Contains(FeatureShaderF16) {
		t.Error("Insert should add feature")
	}

	f.Remove(FeatureShaderF16)
	if f.Contains(FeatureShaderF16) {
		t.Error("Remove should remove feature")
	}
}

func TestFeaturesUnionIntersect(t *testing.T) {
	f1 := Features(FeatureDepthClipControl | FeatureTimestampQuery)
	f2 := Features(FeatureTimestampQuery | FeatureShaderF16)

	union := f1.Union(f2)
	if !union.Contains(FeatureDepthClipControl) || !union.Contains(FeatureTimestampQuery) || !union.Contains(FeatureShaderF16) {
		t.Error("Union should contain all features")
	}

	intersect := f1.Intersect(f2)
	if !intersect.Contains(FeatureTimestampQuery) {
		t.Error("Intersect should contain common feature")
	}
	if intersect.Contains(FeatureDepthClipControl) || intersect.Contains(FeatureShaderF16) {
		t.Error("Intersect should not contain unique features")
	}
}

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MaxTextureDimension2D != 8192 {
		t.Errorf("MaxTextureDimension2D = %d, want 8192", limits.MaxTextureDimension2D)
	}
	if limits.MaxBindGroups != 4 {
		t.Errorf("MaxBindGroups = %d, want 4", limits.MaxBindGroups)
	}
	if limits.MaxComputeWorkgroupSizeX != 256 {
		t.Errorf("MaxComputeWorkgroupSizeX = %d, want 256", limits.MaxComputeWorkgroupSizeX)
	}
}

func TestDownlevelLimits(t *testing.T) {
	limits := DownlevelLimits()

	if limits.MaxTextureDimension2D != 2048 {
		t.Errorf("MaxTextureDimension2D = %d, want 2048", limits.MaxTextureDimension2D)
	}
}

func TestDefaultSamplerDescriptor(t *testing.T) {
	desc := DefaultSamplerDescriptor()

	if desc.AddressModeU != AddressModeClampToEdge {
		t.Errorf("AddressModeU = %d, want ClampToEdge", desc.AddressModeU)
	}
	if desc.MagFilter != FilterModeNearest {
		t.Errorf("MagFilter = %d, want Nearest", desc.MagFilter)
	}
	if desc.MaxAnisotropy != 1 {
		t.Errorf("MaxAnisotropy = %d, want 1", desc.MaxAnisotropy)
	}
}

func TestDefaultMultisampleState(t *testing.T) {
	state := DefaultMultisampleState()

	if state.Count != 1 {
		t.Errorf("Count = %d, want 1", state.Count)
	}
	if state.Mask != 0xFFFFFFFF {
		t.Errorf("Mask = %x, want 0xFFFFFFFF", state.Mask)
	}
}

func TestVertexFormatSize(t *testing.T) {
	tests := []struct {
		format VertexFormat
		want   uint64
	}{
		{VertexFormatUint8x2, 2},
		{VertexFormatFloat32, 4},
		{VertexFormatFloat32x2, 8},
		{VertexFormatFloat32x3, 12},
		{VertexFormatFloat32x4, 16},
	}

	for _, tt := range tests {
		if got := tt.format.Size(); got != tt.want {
			t.Errorf("VertexFormat(%d).Size() = %d, want %d", tt.format, got, tt.want)
		}
	}
}

func TestColorConstants(t *testing.T) {
	if ColorBlack.R != 0 || ColorBlack.G != 0 || ColorBlack.B != 0 || ColorBlack.A != 1 {
		t.Error("ColorBlack should be (0, 0, 0, 1)")
	}
	if ColorWhite.R != 1 || ColorWhite.G != 1 || ColorWhite.B != 1 || ColorWhite.A != 1 {
		t.Error("ColorWhite should be (1, 1, 1, 1)")
	}
}

func TestDefaultInstanceDescriptor(t *testing.T) {
	desc := DefaultInstanceDescriptor()

	if desc.Backends != BackendsPrimary {
		t.Errorf("Backends = %d, want BackendsPrimary", desc.Backends)
	}
	if desc.Dx12ShaderCompiler != Dx12ShaderCompilerDxc {
		t.Errorf("Dx12ShaderCompiler = %d, want Dxc", desc.Dx12ShaderCompiler)
	}
}

func TestDefaultDeviceDescriptor(t *testing.T) {
	desc := DefaultDeviceDescriptor()

	if len(desc.RequiredFeatures) != 0 {
		t.Error("RequiredFeatures should be empty by default")
	}
	if desc.MemoryHints != MemoryHintsPerformance {
		t.Errorf("MemoryHints = %d, want Performance", desc.MemoryHints)
	}
}
