// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"fmt"
	"testing"
)

// samplerBinds formats the BindSamplerCommands among cmds as "unit:sampler".
func samplerBinds(cmds []Command) string {
	var out []string
	for _, c := range cmds {
		if b, ok := c.(*BindSamplerCommand); ok {
			out = append(out, fmt.Sprintf("%d:%d", b.unit, b.sampler))
		}
	}
	return fmt.Sprint(out)
}

// TestSamplerStatePipelineSwitch walks the encoder's sampler state through a
// bind group kept across pipelines that pair texture unit 0 with different
// sampler slots (Rust wgpu-hal gles set_pipeline_inner and
// rebind_sampler_states).
func TestSamplerStatePipelineSwitch(t *testing.T) {
	var s samplerState
	steps := []struct {
		name string
		run  func() []Command
		want string
	}{
		{"samplers before any pipeline", func() []Command {
			return s.rebind(0, s.setSampler(0, 100)|s.setSampler(1, 200))
		}, "[]"},
		{"pipeline pairs unit 0 with slot 0", func() []Command {
			return s.rebind(s.setPipelineMap(&samplerBindMap{0: 1}), 0)
		}, "[0:100]"},
		{"pipeline pairs unit 0 with slot 1", func() []Command {
			return s.rebind(s.setPipelineMap(&samplerBindMap{0: 2}), 0)
		}, "[0:200]"},
		{"same pairing again", func() []Command {
			return s.rebind(s.setPipelineMap(&samplerBindMap{0: 2}), 0)
		}, "[]"},
		{"new sampler at slot 1", func() []Command {
			return s.rebind(0, s.setSampler(1, 300))
		}, "[0:300]"},
		{"new sampler at an unpaired slot", func() []Command {
			return s.rebind(0, s.setSampler(5, 400))
		}, "[]"},
		{"pipeline without samplers unbinds unit 0", func() []Command {
			return s.rebind(s.setPipelineMap(&samplerBindMap{}), 0)
		}, "[0:0]"},
	}
	for _, step := range steps {
		if got := samplerBinds(step.run()); got != step.want {
			t.Errorf("%s: bound %s, want %s", step.name, got, step.want)
		}
	}
	if s.setSampler(maxTextureSlots, 1) != 0 {
		t.Error("setSampler accepted a slot out of range")
	}
}
