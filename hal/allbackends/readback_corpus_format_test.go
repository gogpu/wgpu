// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gogpu/gputypes"
)

// This file reads naga's GPU readback corpus (naga testdata/readback, format
// in its README.md) for TestReadbackCorpus. naga owns the corpus and checks it
// with TestReadbackCorpusCompile; this reader enforces the same rules, so a
// case that the README does not allow, or a newer format, fails here instead
// of being misread.

// readbackCorpusFormat is the newest corpus format this reader knows.
// Format 2 is format 1 plus `skip` in a known failure.
const readbackCorpusFormat = 2

// corpusFill is the word "fill" stands for in an expectation.
const corpusFill = "fill"

// corpusBackendNames are the backend names a known failure may name; they are
// the names of halBackendConfigs.
var corpusBackendNames = map[string]bool{
	"vulkan": true, "dx12-fxc": true, "dx12-dxil": true, "gles": true, "metal": true, "software": true,
}

// corpusFeatures maps the corpus `requires` names (WebGPU feature names) to
// gputypes features. Format 1 and 2 know only shader-f16.
var corpusFeatures = map[string]gputypes.Features{
	"shader-f16": gputypes.Features(gputypes.FeatureShaderF16),
}

var corpusKinds = map[string]gputypes.BufferBindingType{
	"storage-read": gputypes.BufferBindingTypeReadOnlyStorage,
	"storage-rw":   gputypes.BufferBindingTypeStorage,
	"uniform":      gputypes.BufferBindingTypeUniform,
}

// corpusNamePart is one of <area> or <name>: lowercase words joined by '-'.
var corpusNamePart = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// corpusCase is one parsed and validated case.
type corpusCase struct {
	name          string // <area>/<name>
	wgsl          string
	desc          string
	entry         string
	workgroups    [3]uint32
	constants     map[string]float64
	requires      []string
	features      gputypes.Features // the features of requires
	bindings      []corpusBinding
	knownFailures []corpusKnownFailure
}

// corpusBinding is one buffer of a case.
type corpusBinding struct {
	binding uint32
	kind    gputypes.BufferBindingType
	words   []uint32   // initial contents
	fill    uint32     // the fill value; 0 when words are given
	expect  [][]uint32 // acceptable contents after the dispatch, "fill" resolved; nil = not checked
	typ     string     // u32, i32 or f32: how a failure prints the words
}

// corpusKnownFailure is an expected failure (README "Known failures").
type corpusKnownFailure struct {
	Backend string `json:"backend"`
	Adapter string `json:"adapter"`
	Driver  string `json:"driver"`
	Card    string `json:"card"`
	Reason  string `json:"reason"`
	Skip    bool   `json:"skip"`
}

// The JSON shapes: pointers tell a missing required field from a zero value.
type corpusCaseJSON struct {
	Format        *int                 `json:"format"`
	Desc          *string              `json:"desc"`
	Entry         *string              `json:"entry"`
	Workgroups    []uint32             `json:"workgroups"`
	Constants     map[string]float64   `json:"constants"`
	Requires      []string             `json:"requires"`
	Bindings      []corpusBindingJSON  `json:"bindings"`
	KnownFailures []corpusKnownFailure `json:"knownFailures"`
}

type corpusBindingJSON struct {
	Binding *uint32             `json:"binding"`
	Kind    *string             `json:"kind"`
	Words   []json.RawMessage   `json:"words"`
	Len     *int                `json:"len"`
	Fill    json.RawMessage     `json:"fill"`
	Expect  [][]json.RawMessage `json:"expect"`
	Type    string              `json:"type"`
}

// loadReadbackCorpus reads every case under dir, sorted by name. Any file the
// README does not allow, an unpaired shader or case, or an invalid case is an
// error.
func loadReadbackCorpus(dir string) ([]corpusCase, error) {
	pairs := map[string]int{} // name -> 1 for .wgsl, 2 for .json
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "README.md" {
			return nil
		}
		ext := filepath.Ext(rel)
		name := strings.TrimSuffix(rel, ext)
		var bit int
		switch ext {
		case ".wgsl":
			bit = 1
		case ".json":
			bit = 2
		default:
			return fmt.Errorf("readback corpus: unexpected file %s", rel)
		}
		parts := strings.Split(name, "/")
		if len(parts) != 2 || !corpusNamePart.MatchString(parts[0]) || !corpusNamePart.MatchString(parts[1]) {
			return fmt.Errorf("readback corpus: %s is not <area>/<name> in lowercase words joined by '-'", rel)
		}
		pairs[name] |= bit
		return nil
	})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pairs))
	for name, bits := range pairs {
		switch bits {
		case 1:
			return nil, fmt.Errorf("readback corpus: %s.wgsl has no .json", name)
		case 2:
			return nil, fmt.Errorf("readback corpus: %s.json has no .wgsl", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	cases := make([]corpusCase, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".json"))
		if err != nil {
			return nil, err
		}
		wgsl, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)+".wgsl"))
		if err != nil {
			return nil, err
		}
		c, err := parseCorpusCase(name, data, string(wgsl))
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// parseCorpusCase decodes and validates one case.
func parseCorpusCase(name string, data []byte, wgsl string) (corpusCase, error) {
	fail := func(format string, args ...any) (corpusCase, error) {
		return corpusCase{}, fmt.Errorf("readback corpus %s: %s", name, fmt.Sprintf(format, args...))
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var raw corpusCaseJSON
	if err := dec.Decode(&raw); err != nil {
		return fail("%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fail("data after the case object")
	}

	switch {
	case raw.Format == nil:
		return fail("missing format")
	case *raw.Format > readbackCorpusFormat:
		return fail("corpus format %d > supported %d: update wgpu", *raw.Format, readbackCorpusFormat)
	case *raw.Format < 1:
		return fail("format %d", *raw.Format)
	}
	if raw.Desc == nil || *raw.Desc == "" {
		return fail("missing desc")
	}
	if raw.Entry == nil || *raw.Entry == "" {
		return fail("missing entry")
	}
	if len(raw.Workgroups) != 3 {
		return fail("workgroups has %d counts, want 3", len(raw.Workgroups))
	}
	c := corpusCase{name: name, wgsl: wgsl, desc: *raw.Desc, entry: *raw.Entry, constants: raw.Constants,
		requires: raw.Requires}
	for i, n := range raw.Workgroups {
		if n < 1 {
			return fail("workgroups[%d] = 0", i)
		}
		c.workgroups[i] = n
	}
	for _, r := range raw.Requires {
		f, ok := corpusFeatures[r]
		if !ok {
			return fail("unknown requires %q", r)
		}
		c.features |= f
	}

	if len(raw.Bindings) == 0 {
		return fail("no bindings")
	}
	seen := map[uint32]bool{}
	checked := false
	for i := range raw.Bindings {
		b, err := parseCorpusBinding(&raw.Bindings[i])
		if err != nil {
			return fail("bindings[%d]: %v", i, err)
		}
		if seen[b.binding] {
			return fail("binding %d listed twice", b.binding)
		}
		seen[b.binding] = true
		checked = checked || b.expect != nil
		c.bindings = append(c.bindings, b)
	}
	if !checked {
		return fail("no binding has expect")
	}

	usesSkip := false
	for i, k := range raw.KnownFailures {
		switch {
		case !corpusBackendNames[k.Backend]:
			return fail("knownFailures[%d]: unknown backend %q", i, k.Backend)
		case k.Card == "":
			return fail("knownFailures[%d]: missing card", i)
		case k.Reason == "":
			return fail("knownFailures[%d]: missing reason", i)
		}
		usesSkip = usesSkip || k.Skip
	}
	// A format 1 reader would ignore skip and dispatch a case that hangs the
	// GPU, so skip needs format 2. The README's other direction (format 2
	// only with skip) is an authoring rule for naga's compile test: a format
	// 2 case without skip reads correctly here (naga v0.21.7 has three).
	if usesSkip && *raw.Format < 2 {
		return fail("skip in a known failure needs format 2")
	}
	c.knownFailures = raw.KnownFailures
	return c, nil
}

func parseCorpusBinding(raw *corpusBindingJSON) (corpusBinding, error) {
	var b corpusBinding
	if raw.Binding == nil {
		return b, errors.New("missing binding")
	}
	b.binding = *raw.Binding
	if raw.Kind == nil {
		return b, errors.New("missing kind")
	}
	kind, ok := corpusKinds[*raw.Kind]
	if !ok {
		return b, fmt.Errorf("unknown kind %q", *raw.Kind)
	}
	b.kind = kind
	switch raw.Type {
	case "", "u32", "i32", "f32":
		b.typ = raw.Type
	default:
		return b, fmt.Errorf("unknown type %q", raw.Type)
	}

	switch {
	case raw.Words != nil && raw.Len != nil:
		return b, errors.New("both words and len")
	case raw.Words != nil:
		if raw.Fill != nil {
			return b, errors.New("fill without len")
		}
		if len(raw.Words) == 0 {
			return b, errors.New("empty words")
		}
		for i, w := range raw.Words {
			v, isFill, err := parseCorpusWord(w)
			if err != nil || isFill {
				return b, fmt.Errorf("words[%d]: not a word: %s", i, w)
			}
			b.words = append(b.words, v)
		}
	case raw.Len != nil:
		if *raw.Len < 1 {
			return b, fmt.Errorf("len %d", *raw.Len)
		}
		if raw.Fill != nil {
			v, isFill, err := parseCorpusWord(raw.Fill)
			if err != nil || isFill {
				return b, fmt.Errorf("fill: not a word: %s", raw.Fill)
			}
			b.fill = v
		}
		b.words = make([]uint32, *raw.Len)
		for i := range b.words {
			b.words[i] = b.fill
		}
	default:
		return b, errors.New("neither words nor len")
	}

	if raw.Expect != nil {
		if b.kind != gputypes.BufferBindingTypeStorage {
			return b, fmt.Errorf("expect on a %s binding (storage-rw only)", *raw.Kind)
		}
		if len(raw.Expect) == 0 {
			return b, errors.New("empty expect")
		}
		for i, alt := range raw.Expect {
			if len(alt) != len(b.words) {
				return b, fmt.Errorf("expect[%d] has %d words, the buffer %d", i, len(alt), len(b.words))
			}
			want := make([]uint32, len(alt))
			for j, w := range alt {
				v, isFill, err := parseCorpusWord(w)
				if err != nil {
					return b, fmt.Errorf("expect[%d][%d]: not a word: %s", i, j, w)
				}
				if isFill {
					v = b.fill
				}
				want[j] = v
			}
			b.expect = append(b.expect, want)
		}
	}
	return b, nil
}

// parseCorpusWord reads one word: a JSON integer in [0, 2^32-1], a string
// "0x" with one to eight hex digits, or the string "fill".
func parseCorpusWord(raw json.RawMessage) (v uint32, isFill bool, err error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == corpusFill {
			return 0, true, nil
		}
		hex, ok := strings.CutPrefix(s, "0x")
		if !ok || hex == "" || len(hex) > 8 {
			return 0, false, fmt.Errorf("word %q", s)
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		return uint32(n), false, err
	}
	// ParseUint rejects signs, fractions and exponents, which a JSON number
	// decode into float64 would accept.
	n, err := strconv.ParseUint(string(raw), 10, 32)
	return uint32(n), false, err
}

// dispatch is the case as one halRunner dispatch.
func (c *corpusCase) dispatch() *halDispatch {
	bindings := make([]halBinding, len(c.bindings))
	for i := range c.bindings {
		b := &c.bindings[i]
		bindings[i] = halBinding{binding: b.binding, kind: b.kind, words: b.words}
	}
	return &halDispatch{label: c.name, wgsl: c.wgsl, entry: c.entry, constants: c.constants,
		workgroups: c.workgroups, bindings: bindings}
}

// mismatch compares the contents read back (in the order of c.bindings) with
// the expectations and returns "" when every checked binding equals one of its
// alternatives, else one line per failing binding.
func (c *corpusCase) mismatch(out [][]uint32) string {
	var msgs []string
	for i := range c.bindings {
		b := &c.bindings[i]
		if b.expect == nil {
			continue
		}
		if !b.matches(out[i]) {
			msgs = append(msgs, fmt.Sprintf("binding %d got %s, want %s", b.binding, b.format(out[i]), b.format(b.expect[0])))
		}
	}
	return strings.Join(msgs, "; ")
}

func (b *corpusBinding) matches(got []uint32) bool {
	for _, want := range b.expect {
		if equalWords(got, want) {
			return true
		}
	}
	return false
}

func equalWords(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// format prints words as the binding's type, with the fill value as "fill"
// when the binding has a fill.
func (b *corpusBinding) format(words []uint32) string {
	s := make([]string, len(words))
	for i, v := range words {
		switch {
		case b.fill != 0 && v == b.fill:
			s[i] = corpusFill
		case b.typ == "i32":
			s[i] = strconv.Itoa(int(int32(v)))
		case b.typ == "f32":
			s[i] = strconv.FormatFloat(float64(math.Float32frombits(v)), 'g', -1, 32)
		default:
			s[i] = strconv.FormatUint(uint64(v), 10)
		}
	}
	return "[" + strings.Join(s, " ") + "]"
}

// knownFailure returns the first known failure of c that matches the run, or
// nil. The adapter substring is matched against the adapter name and the
// driver substring against the driver version and description, all
// case-insensitive; an empty substring matches everything.
func (c *corpusCase) knownFailure(backend string, info *gputypes.AdapterInfo) *corpusKnownFailure {
	contains := func(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }
	for i := range c.knownFailures {
		k := &c.knownFailures[i]
		if k.Backend == backend && contains(info.Name, k.Adapter) &&
			(contains(info.Driver, k.Driver) || contains(info.DriverInfo, k.Driver)) {
			return k
		}
	}
	return nil
}

// corpusVerdict classifies a run (README "Known failures"): it returns the
// result tag and whether the run fails the test. A pass that a known failure
// matches fails: a fixed bug must not hide behind a stale entry.
func corpusVerdict(passed bool, known *corpusKnownFailure) (tag string, failed bool) {
	switch {
	case passed && known == nil:
		return "PASS", false
	case !passed && known != nil:
		return "XFAIL(" + known.Card + ")", false
	case passed:
		return "XPASS(unexpected pass, close " + known.Card + ")", true
	default:
		return "FAIL", true
	}
}
