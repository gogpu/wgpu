// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// These tests check the corpus reader and the verdict logic of
// TestReadbackCorpus, and that the harness fails a wrong value on a real
// device: a harness that passes everything would make the corpus worthless.

const selftestWGSL = `
@group(0) @binding(0) var<storage, read> src: array<u32>;
@group(0) @binding(1) var<storage, read_write> dst: array<u32>;

@compute @workgroup_size(1)
fn main() {
    dst[0] = src[0] + src[1];
    dst[1] = src[0] * src[1];
}
`

// selftestCase is a valid format 1 case for selftestWGSL: 3+4 and 3*4, and a
// sentinel word the shader must not write.
const selftestCase = `{
  "format": 1,
  "desc": "harness self-test: sum and product, sentinel after them",
  "entry": "main",
  "workgroups": [1, 1, 1],
  "bindings": [
    {"binding": 0, "kind": "storage-read", "words": [3, "0x4"]},
    {"binding": 1, "kind": "storage-rw", "len": 3, "fill": "0xDEADBEEF", "expect": [[7, 12, "fill"]]}
  ]
}`

func TestReadbackCorpusFormatAccepts(t *testing.T) {
	c, err := parseCorpusCase("self/test", []byte(selftestCase), selftestWGSL)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.bindings[0].words; !equalWords(got, []uint32{3, 4}) {
		t.Errorf("binding 0 words = %v, want [3 4]", got)
	}
	if got := c.bindings[1].words; !equalWords(got, []uint32{0xDEADBEEF, 0xDEADBEEF, 0xDEADBEEF}) {
		t.Errorf("binding 1 words = %#x, want three fills", got)
	}
	if got := c.bindings[1].expect[0]; !equalWords(got, []uint32{7, 12, 0xDEADBEEF}) {
		t.Errorf("binding 1 expect = %#x, want [7 12 fill]", got)
	}
	if c.workgroups != [3]uint32{1, 1, 1} || c.entry != "main" {
		t.Errorf("workgroups %v entry %q", c.workgroups, c.entry)
	}
}

func TestReadbackCorpusFormatRejects(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"newer format", `"format": 1`, `"format": 3`, "corpus format 3 > supported 2: update wgpu"},
		{"unknown field", `"entry": "main",`, `"entry": "main", "extra": 1,`, "unknown field"},
		{"missing desc", `"desc": "harness self-test: sum and product, sentinel after them",`, ``, "missing desc"},
		{"two workgroup counts", `[1, 1, 1]`, `[1, 1]`, "workgroups has 2 counts"},
		{"zero workgroups", `[1, 1, 1]`, `[1, 0, 1]`, "workgroups[1] = 0"},
		{"unknown requires", `"entry": "main",`, `"entry": "main", "requires": ["subgroups"],`, `unknown requires "subgroups"`},
		{"words and len", `"words": [3, "0x4"]`, `"words": [3, "0x4"], "len": 2`, "both words and len"},
		{"fill with words", `"words": [3, "0x4"]`, `"words": [3, "0x4"], "fill": 0`, "fill without len"},
		{"negative word", `[3, "0x4"]`, `[-1, "0x4"]`, "not a word"},
		{"fraction word", `[3, "0x4"]`, `[1.5, "0x4"]`, "not a word"},
		{"nine hex digits", `"0x4"`, `"0x100000000"`, "not a word"},
		{"hex without prefix", `"0x4"`, `"4"`, "not a word"},
		{"fill in words", `[3, "0x4"]`, `[3, "fill"]`, "not a word"},
		{"expect on read-only", `"words": [3, "0x4"]`, `"words": [3, "0x4"], "expect": [[3, 4]]`, "expect on a storage-read binding"},
		{"expect length", `[[7, 12, "fill"]]`, `[[7, 12]]`, "expect[0] has 2 words, the buffer 3"},
		{"no expect", `, "expect": [[7, 12, "fill"]]`, ``, "no binding has expect"},
		{"duplicate binding", `"binding": 1,`, `"binding": 0,`, "binding 0 listed twice"},
		{"unknown kind", `"storage-read"`, `"storage"`, `unknown kind "storage"`},
		{"unknown type", `"kind": "storage-read",`, `"kind": "storage-read", "type": "f16",`, `unknown type "f16"`},
		{"trailing data", `]
}`, `]
} {}`, "data after the case object"},
		{"known failure without card", `"bindings"`, `"knownFailures": [{"backend": "gles", "reason": "x"}], "bindings"`, "missing card"},
		{"known failure unknown backend", `"bindings"`, `"knownFailures": [{"backend": "dx12", "card": "C-1", "reason": "x"}], "bindings"`, `unknown backend "dx12"`},
		{"skip in format 1", `"bindings"`, `"knownFailures": [{"backend": "gles", "card": "C-1", "reason": "x", "skip": true}], "bindings"`, "skip in a known failure needs format 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(selftestCase, tc.from) {
				t.Fatalf("test bug: %q is not in the case", tc.from)
			}
			data := strings.Replace(selftestCase, tc.from, tc.to, 1)
			_, err := parseCorpusCase("self/test", []byte(data), selftestWGSL)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestReadbackCorpusLoaderFiles(t *testing.T) {
	write := func(t *testing.T, dir, name, data string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "README.md", "corpus")
		write(t, dir, "self/test.wgsl", selftestWGSL)
		write(t, dir, "self/test.json", selftestCase)
		cases, err := loadReadbackCorpus(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(cases) != 1 || cases[0].name != "self/test" {
			t.Fatalf("cases = %d, first %q", len(cases), cases[0].name)
		}
	})
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"unpaired shader", map[string]string{"self/test.wgsl": selftestWGSL}, "self/test.wgsl has no .json"},
		{"unpaired case", map[string]string{"self/test.json": selftestCase}, "self/test.json has no .wgsl"},
		{"other file", map[string]string{"self/notes.txt": "x"}, "unexpected file self/notes.txt"},
		{"upper case name", map[string]string{"self/Test.wgsl": selftestWGSL, "self/Test.json": selftestCase}, "not <area>/<name>"},
		{"no area", map[string]string{"test.wgsl": selftestWGSL, "test.json": selftestCase}, "not <area>/<name>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tc.files {
				write(t, dir, name, data)
			}
			_, err := loadReadbackCorpus(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestReadbackCorpusVerdict(t *testing.T) {
	known := &corpusKnownFailure{Card: "C-1"}
	for _, tc := range []struct {
		passed  bool
		known   *corpusKnownFailure
		tag     string
		failing bool
	}{
		{true, nil, "PASS", false},
		{false, nil, "FAIL", true},
		{false, known, "XFAIL(C-1)", false},
		{true, known, "XPASS(unexpected pass, close C-1)", true},
	} {
		tag, failed := corpusVerdict(tc.passed, tc.known)
		if tag != tc.tag || failed != tc.failing {
			t.Errorf("corpusVerdict(%v, %v) = %q, %v; want %q, %v", tc.passed, tc.known != nil, tag, failed, tc.tag, tc.failing)
		}
	}
}

func TestReadbackCorpusKnownFailureMatch(t *testing.T) {
	c := corpusCase{knownFailures: []corpusKnownFailure{
		{Backend: backendDX12DXIL, Adapter: "geforce", Card: "NV-1", Reason: "x"},
		{Backend: backendGLES, Driver: "mesa", Card: "MESA-1", Reason: "x"},
		{Backend: backendVulkan, Card: "ALL-1", Reason: "x"},
	}}
	nvidia := gputypes.AdapterInfo{Name: "NVIDIA GeForce GTX 1660 SUPER", Driver: "581.57"}
	intel := gputypes.AdapterInfo{Name: "Intel(R) Iris(R) Xe Graphics", Driver: "32.0.101"}
	llvmpipe := gputypes.AdapterInfo{Name: "llvmpipe", Driver: "4.5", DriverInfo: "Mesa 24.0.5"}
	for _, tc := range []struct {
		backend backendName
		info    gputypes.AdapterInfo
		want    string
	}{
		{backendDX12DXIL, nvidia, "NV-1"},
		{backendDX12DXIL, intel, ""},
		{backendDX12FXC, nvidia, ""},
		{backendGLES, llvmpipe, "MESA-1"},
		{backendGLES, intel, ""},
		{backendVulkan, intel, "ALL-1"},
	} {
		got := ""
		if k := c.knownFailure(tc.backend, &tc.info); k != nil {
			got = k.Card
		}
		if got != tc.want {
			t.Errorf("knownFailure(%s, %s) = %q, want %q", tc.backend, tc.info.Name, got, tc.want)
		}
	}
}

// TestReadbackCorpusCatchesWrongValue runs the self-test case on the software
// backend (present on every platform and in CI) through the same dispatch and
// comparison as TestReadbackCorpus: the right expectation passes, and a
// corrupted one and a missing store both fail.
func TestReadbackCorpusCatchesWrongValue(t *testing.T) {
	backend, ok := hal.GetBackend(gputypes.BackendEmpty)
	if !ok {
		t.Fatal("software backend not registered")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsAll})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("software backend has no adapter")
	}
	a := &adapters[0]
	open, err := a.Adapter.Open(0, a.Capabilities.Limits)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = open.Device.WaitIdle()
		open.Device.Destroy()
	}()
	r := &halRunner{device: open.Device, queue: open.Queue}

	for _, tc := range []struct {
		name, from, to string
		wantFail       bool
	}{
		{"right expectation", "", "", false},
		{"corrupted value", `[[7, 12, "fill"]]`, `[[7, 13, "fill"]]`, true},
		{"store expected past the shader's writes", `[[7, 12, "fill"]]`, `[[7, 12, 0]]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := selftestCase
			if tc.from != "" {
				data = strings.Replace(data, tc.from, tc.to, 1)
			}
			c, err := parseCorpusCase("self/test", []byte(data), selftestWGSL)
			if err != nil {
				t.Fatal(err)
			}
			out, err := r.dispatch(c.dispatch())
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			mismatch := c.mismatch(out)
			if (mismatch != "") != tc.wantFail {
				t.Fatalf("mismatch = %q, want failure %v (read back %v)", mismatch, tc.wantFail, out)
			}
		})
	}
}
