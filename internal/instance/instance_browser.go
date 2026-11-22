//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// This file is the browser implementation of the instance domain (ADR-070 S12),
// moved from the root package's instance_browser.go. The root keeps the public
// Instance declaration layer and the canvas creation glue, which names the
// public target types and the js.Value canvas entry point.

import (
	"syscall/js"

	"github.com/gogpu/gputypes"

	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/core"
)

// Instance is the browser-build implementation of the GPU instance.
type Instance struct {
	browser  *browser.Instance
	released bool
}

// Create accesses navigator.gpu and wraps the browser instance.
func Create() (*Instance, error) {
	bi, err := browser.NewInstance()
	if err != nil {
		return nil, err
	}
	return &Instance{browser: bi}, nil
}

// GPU returns the navigator.gpu object the canvas creation glue builds a
// surface on, or a zero value for an instance without one.
func (i *Instance) GPU() js.Value {
	if i == nil || i.browser == nil {
		return js.Value{}
	}
	return i.browser.GPU()
}

// IsReleased reports whether the instance has been released. A nil receiver
// answers true.
func (i *Instance) IsReleased() bool {
	return i == nil || i.released
}

// RequestAdapter requests a GPU adapter for the options the declaration layer
// converted (a nil pointer is the public nil, which becomes js.Undefined).
func (i *Instance) RequestAdapter(opts *gputypes.RequestAdapterOptions) (*Adapter, error) {
	if i.released {
		return nil, core.ErrReleased
	}

	// Build JS options object from Go types.
	var jsOpts js.Value
	if opts != nil {
		jsOpts = browser.BuildRequestAdapterOptions(opts.PowerPreference, opts.ForceFallbackAdapter)
	} else {
		jsOpts = js.Undefined()
	}

	ba, err := i.browser.RequestAdapter(jsOpts)
	if err != nil {
		return nil, err
	}

	return wrapBrowserAdapter(ba), nil
}

// wrapBrowserAdapter builds the implementation adapter for a browser adapter.
func wrapBrowserAdapter(ba *browser.Adapter) *Adapter {
	// Extract features and limits from the JS adapter.
	features := browser.ExtractFeatures(ba.Features())
	limits := browser.ExtractLimits(ba.Limits())

	// WebGPU browser API does not expose detailed adapter info.
	// Return minimal info matching Rust wgpu's WebAdapter::get_info().
	info := gputypes.AdapterInfo{
		Name: "WebGPU Adapter",
	}

	return &Adapter{
		browser:  ba,
		info:     info,
		features: features,
		limits:   limits,
	}
}

// EnumerateAdapters returns at most one adapter. navigator.gpu has no
// enumeration API, so a default adapter request is the only way to see one —
// the same shape Rust wgpu's web backend uses (its enumerate_adapters issues
// one default request and returns zero or one adapter). An environment with no
// adapter yields none; a browser or driver refusal is "none" too, because the
// enumeration has no error channel (P1-API-040 R3).
func (i *Instance) EnumerateAdapters() []*Adapter {
	if i == nil || i.released {
		return nil
	}
	ba, err := i.browser.RequestAdapter(js.Undefined())
	if err != nil {
		return nil
	}
	return []*Adapter{wrapBrowserAdapter(ba)}
}

// Release releases the instance. Surfaces must be released explicitly.
func (i *Instance) Release() {
	if i == nil || i.released {
		return
	}
	i.released = true
}

// HAL is the browser-build body of the public Instance.HAL. A nil receiver is
// the declaration layer's zero value, which reports the browser kind: the
// browser has no instance handle and no destroy.
func (i *Instance) HAL() publichal.Handles {
	if i != nil && i.released {
		return publichal.Handles{}
	}
	return publichal.Handles{Kind: publichal.KindBrowser}
}
