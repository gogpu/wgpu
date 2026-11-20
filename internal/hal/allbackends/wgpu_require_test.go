// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"os"
	"strings"
	"testing"
)

// requireSwitch reports whether the comma-separated WGPU_REQUIRE list names
// name, case-insensitively (the ecosystem's <REPO>_REQUIRE switch, LEAD-013:
// where a dependency is expected, its absence is a failure, never a skip that
// reads as "tested"). The list is shared — layoutcheck and the GLES tests
// consume their own items — so an item this package does not know is ignored,
// not rejected.
func requireSwitch(name string) bool {
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return true
		}
	}
	return false
}

// requiredBy returns the WGPU_REQUIRE item that requires this backend, or ""
// when none does. "vvl" implies Vulkan: the validation layer runs on a Vulkan
// instance, so requiring it requires the backend too.
func (cfg halBackendConfig) requiredBy() string {
	if requireSwitch(cfg.required) {
		return cfg.required
	}
	if cfg.required == "vulkan" && requireSwitch("vvl") {
		return "vvl"
	}
	return ""
}

// requireOrSkip skips a backend this machine or runner does not have, or fails
// when WGPU_REQUIRE names it: a job that promises a backend's coverage must
// not pass on its absence. The failure text follows the ecosystem's existing
// switch (`<reason>, but WGPU_REQUIRE="<list>" requires <name>`).
func requireOrSkip(t *testing.T, cfg halBackendConfig, reason string) {
	t.Helper()
	if name := cfg.requiredBy(); name != "" {
		t.Fatalf("%s, but WGPU_REQUIRE=%q requires %s", reason, os.Getenv("WGPU_REQUIRE"), name)
	}
	t.Skip(reason)
}

// requireLayerActive fails when WGPU_REQUIRE lists vvl and the capture shows
// that the validation layer never loaded on the instance: a run with no
// messages would otherwise pass for a clean run.
func requireLayerActive(t *testing.T, vvl *vvlCapture) {
	t.Helper()
	if vvl.layerActive() || !requireSwitch("vvl") {
		return
	}
	t.Fatalf("the Vulkan validation layer is not active on the instance, but WGPU_REQUIRE=%q requires vvl",
		os.Getenv("WGPU_REQUIRE"))
}

// halBackendConfigFor returns the config halBackendConfigs registers under
// name, so a test can require that exact backend variant.
func halBackendConfigFor(name backendName) halBackendConfig {
	for _, cfg := range halBackendConfigs {
		if cfg.name == name {
			return cfg
		}
	}
	return halBackendConfig{}
}

// TestRequireSwitch pins the switch's parsing: trimmed items, any case, an
// empty list requires nothing, and an unknown item (another check's) is not a
// requirement of this package.
func TestRequireSwitch(t *testing.T) {
	for _, tc := range []struct {
		env  string
		name string
		want bool
	}{
		{"", "vulkan", false},
		{"vulkan", "vulkan", true},
		{"vulkan,vvl", "vvl", true},
		{" Vulkan , VVL ", "vulkan", true},
		{" Vulkan , VVL ", "vvl", true},
		{"layoutcheck", "vulkan", false},
		{"vulkan,vvl", "dx12", false},
		{"software", "vul", false},
	} {
		t.Run(tc.env+"/"+tc.name, func(t *testing.T) {
			t.Setenv("WGPU_REQUIRE", tc.env)
			if got := requireSwitch(tc.name); got != tc.want {
				t.Errorf("requireSwitch(%q) with WGPU_REQUIRE=%q = %v, want %v", tc.name, tc.env, got, tc.want)
			}
		})
	}
}

// TestRequiredBy pins which requirement a backend config answers to: dx12
// covers both shader-compiler variants, and vvl implies vulkan.
func TestRequiredBy(t *testing.T) {
	for _, tc := range []struct {
		env    string
		config backendName
		want   string
	}{
		{"", backendVulkan, ""},
		{"vulkan", backendVulkan, "vulkan"},
		{"vvl", backendVulkan, "vvl"},
		{"dx12", backendDX12FXC, "dx12"},
		{"dx12", backendDX12DXIL, "dx12"},
		{"gles", backendGLES, "gles"},
		{"metal", backendMetal, "metal"},
		{"software", backendSoftware, "software"},
		{"vulkan", backendDX12FXC, ""},
	} {
		t.Run(tc.env+"/"+string(tc.config), func(t *testing.T) {
			t.Setenv("WGPU_REQUIRE", tc.env)
			cfg := halBackendConfigFor(tc.config)
			if cfg.required == "" {
				t.Fatalf("halBackendConfigs has no %s", tc.config)
			}
			if got := cfg.requiredBy(); got != tc.want {
				t.Errorf("requiredBy() of %s with WGPU_REQUIRE=%q = %q, want %q", tc.config, tc.env, got, tc.want)
			}
		})
	}
}
