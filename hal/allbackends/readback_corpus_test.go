// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// nagaModule is the module whose testdata/readback holds the corpus.
const nagaModule = "github.com/gogpu/naga"

// TestReadbackCorpus runs naga's GPU readback corpus (naga
// testdata/readback) on every adapter of every HAL backend and compares the
// buffers read back with each case's expected words (naga's README.md is the
// format). Static validators check structure, not values: naga v0.20.3 shipped
// three DXIL loop miscompiles that every validator accepted, and only a
// readback run caught them.
//
// The corpus is the naga version this module resolves: under go.work the local
// naga tree, otherwise the version go.mod pins, read from the module cache
// (testdata/ ships in the module zip). A naga candidate is run through a
// private gate go.work (GOWORK=<file>), never an environment variable.
//
// Subtests are backend/adapter/area/name; every run logs one RESULT line with
// PASS, FAIL, XFAIL(card), XPASS(close card) or SKIP(reason). A known failure
// that passes fails the test, so a fixed bug cannot hide behind a stale entry.
// On Vulkan, a validation message reported during a case fails that case.
func TestReadbackCorpus(t *testing.T) {
	dir, err := nagaReadbackDir()
	if err != nil {
		t.Fatalf("locate naga's readback corpus: %v", err)
	}
	// Every naga version this module can resolve ships the corpus (go.mod
	// pins v0.21.8 or later; it holds cases that no other test here runs), so
	// a missing directory means discovery broke or the corpus moved: a
	// failure, never a skip that would pass for "tested".
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("naga's readback corpus: %v", err)
	}
	cases, err := loadReadbackCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatalf("readback corpus %s has no cases", dir)
	}
	t.Logf("%d cases from %s", len(cases), dir)

	forEachHALAdapter(t, func(t *testing.T, backend backendName, a *hal.ExposedAdapter, vvl *vvlCapture) {
		if a.Capabilities.DownlevelCapabilities.Flags&gputypes.DownlevelFlagsComputeShaders == 0 {
			t.Skipf("%s has no compute shaders (DownlevelFlagsComputeShaders unset)", a.Info.Name)
		}
		// Open with every feature some case requires and the adapter has, so a
		// case skips only when the adapter really lacks its feature.
		var want gputypes.Features
		for i := range cases {
			want |= cases[i].features & a.Features
		}
		open, err := a.Adapter.Open(want, a.Capabilities.Limits)
		if err != nil {
			t.Fatalf("Open %q with features %#x: %v", a.Info.Name, uint64(want), err)
		}
		defer func() {
			_ = open.Device.WaitIdle()
			open.Device.Destroy()
		}()
		for _, m := range vvl.drain() {
			t.Errorf("Vulkan validation message while opening the device: %s", m)
		}
		r := &halRunner{device: open.Device, queue: open.Queue}
		for i := range cases {
			c := &cases[i]
			t.Run(c.name, func(t *testing.T) {
				runCorpusCase(t, r, backend, a, vvl, c)
			})
		}
	})
}

// runCorpusCase runs one case on one device and reports its RESULT line.
func runCorpusCase(t *testing.T, r *halRunner, backend backendName, a *hal.ExposedAdapter, vvl *vvlCapture, c *corpusCase) {
	t.Helper()
	result := func(tag, detail string) {
		t.Logf("RESULT %-40s %-9s %-36s %s %s", c.name, backend, a.Info.Name, tag, detail)
	}
	for _, name := range c.requires {
		if a.Features&corpusFeatures[name] == 0 {
			result("SKIP(adapter lacks "+name+")", "")
			t.Skipf("adapter does not expose %s", name)
		}
	}
	known := c.knownFailure(backend, &a.Info)
	if known != nil && known.Skip {
		result("SKIP("+known.Card+")", known.Reason)
		t.Skipf("known failure %s, not dispatched: %s", known.Card, known.Reason)
	}

	out, err := r.dispatch(c.dispatch())
	detail := ""
	if err != nil {
		detail = err.Error()
	} else {
		detail = c.mismatch(out)
	}
	if msgs := vvl.drain(); len(msgs) > 0 {
		detail = strings.TrimPrefix(detail+"; Vulkan validation: "+strings.Join(msgs, " | "), "; ")
	}
	tag, failed := corpusVerdict(detail == "", known)
	result(tag, detail)
	if failed {
		t.Errorf("%s: %s %s", c.name, tag, detail)
	}
}

// nagaReadbackDir returns <naga module dir>/testdata/readback for the naga
// version this module resolves (go.work or go.mod). When the module is not in
// the cache yet, it is downloaded at the resolved version.
func nagaReadbackDir() (string, error) {
	out, err := goCommand("list", "-m", "-f", "{{.Dir}}\t{{.Version}}", nagaModule)
	if err != nil {
		return "", err
	}
	dir, version, _ := strings.Cut(strings.TrimSpace(out), "\t")
	if dir == "" {
		if version == "" {
			return "", fmt.Errorf("go list -m %s: neither a directory nor a version", nagaModule)
		}
		out, err := goCommand("mod", "download", "-json", nagaModule+"@"+version)
		if err != nil {
			return "", err
		}
		var mod struct{ Dir string }
		if err := json.Unmarshal([]byte(out), &mod); err != nil {
			return "", fmt.Errorf("go mod download -json %s@%s: %w", nagaModule, version, err)
		}
		if mod.Dir == "" {
			return "", fmt.Errorf("go mod download %s@%s: no directory", nagaModule, version)
		}
		dir = mod.Dir
	}
	return filepath.Join(dir, "testdata", "readback"), nil
}

// goCommand runs the go command in the package directory, with the
// environment of the test (GOWORK included), and returns its stdout.
func goCommand(args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
