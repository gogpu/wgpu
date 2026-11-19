// Command apigolden emits a signature-level golden of this module's public API
// for one build context.
//
//	usage: go -C tools run ./apigolden -out apigolden/testdata/api_windows.txt
//	       go -C tools run ./apigolden -tags rust -out apigolden/testdata/api_rust.txt
//	       go -C tools run ./apigolden -goos js -goarch wasm -out apigolden/testdata/api_wasm.txt
//
// The module root is found by walking up from the working directory to the
// go.mod whose module path is exactly github.com/gogpu/wgpu. Loading runs with
// GOWORK=off so the golden describes the module, not a developer workspace.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const modulePath = "github.com/gogpu/wgpu"

// Floors per build context. A context that is not listed is refused rather than
// given a weak default: a new context must be declared deliberately, with the
// numbers that make a partial load fail.
type floor struct {
	packages    int
	identifiers int
}

// contextFloors are the post-window (v0.35.0, ADR-070) values: the public set
// is root + thin hal + wgputest, so a load that loses a package or shrinks the
// identifier count fails instead of writing a short golden.
var contextFloors = map[string]floor{
	"windows/amd64/":     {3, 258},
	"windows/amd64/rust": {3, 246},
	"js/wasm/":           {3, 241},
}

func floorFor(c config) (floor, error) {
	// An explicit override lets the A5 commit pin the post-window floors with
	// the same binary that produced the golden; both values are required so a
	// half-set override cannot weaken the check by accident.
	if c.minPackages > 0 || c.minIdentifiers > 0 {
		if c.minPackages <= 0 || c.minIdentifiers <= 0 {
			return floor{}, fmt.Errorf("both -min-packages and -min-identifiers are required to override the floors")
		}
		return floor{c.minPackages, c.minIdentifiers}, nil
	}
	key := c.goos + "/" + c.goarch + "/" + strings.Join(c.tags, ",")
	f, ok := contextFloors[key]
	if !ok {
		return floor{}, fmt.Errorf("no floor defined for context %q; add one in contextFloors or pass -min-packages/-min-identifiers", c.describe())
	}
	return f, nil
}

type config struct {
	root   string
	out    string
	goos   string
	goarch string
	tags   []string

	// Explicit floor overrides. A5 sets them from the post-window generation
	// (the public set shrinks to three packages) with the same binary.
	minPackages    int
	minIdentifiers int
}

func main() {
	var c config
	var tags string
	flag.StringVar(&c.root, "root", "", "module root (default: nearest parent go.mod for github.com/gogpu/wgpu)")
	flag.StringVar(&c.out, "out", "", "output file (default: stdout)")
	flag.StringVar(&c.goos, "goos", "windows", "GOOS of the context")
	flag.StringVar(&c.goarch, "goarch", "amd64", "GOARCH of the context")
	flag.StringVar(&tags, "tags", "", "comma-separated build tags")
	flag.IntVar(&c.minPackages, "min-packages", 0, "override the context's minimum public-package count (A5 post-window floors; both min flags required)")
	flag.IntVar(&c.minIdentifiers, "min-identifiers", 0, "override the context's minimum exported-identifier count")
	flag.Parse()

	if tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				c.tags = append(c.tags, t)
			}
		}
	}
	sort.Strings(c.tags)

	root := c.root
	if root == "" {
		var err error
		root, err = findModuleRoot()
		if err != nil {
			fatal("%v", err)
		}
	}

	pkgs, err := load(root, c)
	if err != nil {
		fatal("%v", err)
	}

	text, stats, err := render(c, pkgs)
	if err != nil {
		fatal("%v", err)
	}

	if c.out == "" {
		fmt.Print(text)
		return
	}
	if err := os.WriteFile(c.out, []byte(text), 0o644); err != nil {
		fatal("write %s: %v", c.out, err)
	}
	fmt.Fprintf(os.Stderr, "%s: packages=%d identifiers=%d\n", c.out, stats.packages, stats.identifiers)
}

type stats struct {
	packages    int
	identifiers int
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "apigolden: "+format+"\n", args...)
	os.Exit(1)
}

// findModuleRoot walks up from the working directory to the go.mod whose module
// path is exactly the one this tool describes. It deliberately skips the tools
// module (github.com/gogpu/wgpu/tools) that sits below it.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && modulePathOf(string(data)) == modulePath {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod with module %s above the working directory", modulePath)
		}
		dir = parent
	}
}

func modulePathOf(gomod string) string {
	for _, line := range strings.Split(gomod, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// listedPackage is the subset of `go list -json` this tool reads.
type listedPackage struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
	Export     string
	Error      *struct{ Err string }
}

// load type-checks the module's public packages for one build context.
//
// The loader deliberately uses only the standard library: `go list -export`
// produces export data with the running toolchain, and go/importer decodes it
// with that same toolchain, so the tool cannot drift out of step with the Go
// version that invokes it (golang.org/x/tools bundles its own export-data
// reader, which lags a new Go release and fails on a cold build cache with
// "export data version N is greater than maximum supported version N-1"; the
// x/tools release that fixes it forces the tools module past the module's own
// Go minimum). The package under analysis is parsed from source and
// type-checked here, so parameter names survive into the golden; dependencies
// come from export data.
func load(root string, c config) ([]*types.Package, error) {
	list, err := goList(root, c)
	if err != nil {
		return nil, err
	}

	exports := make(map[string]string, len(list))
	for _, p := range list {
		if p.Export != "" {
			exports[p.ImportPath] = p.Export
		}
	}

	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %q", path)
		}
		return os.Open(file)
	})

	var keep []*types.Package
	for _, p := range list {
		if !include(p.ImportPath) {
			continue
		}
		if len(p.GoFiles) == 0 && len(p.CgoFiles) == 0 {
			// The package has no files for this context (for example the wasm
			// build of a native-only package). It is absent from this
			// context's API, not broken.
			if p.Error != nil && strings.Contains(p.Error.Err, "build constraints exclude all Go files") {
				continue
			}
			return nil, fmt.Errorf("package %s has no files for %s", p.ImportPath, c.describe())
		}
		if p.Error != nil {
			return nil, fmt.Errorf("package %s did not list: %s", p.ImportPath, p.Error.Err)
		}

		files := make([]*ast.File, 0, len(p.GoFiles))
		for _, name := range p.GoFiles {
			file, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", filepath.Join(p.Dir, name), err)
			}
			files = append(files, file)
		}

		conf := types.Config{
			Importer:         imp,
			Sizes:            types.SizesFor("gc", c.goarch),
			IgnoreFuncBodies: true,
		}
		pkg, err := conf.Check(p.ImportPath, fset, files, nil)
		if err != nil {
			return nil, fmt.Errorf("type-check %s: %w", p.ImportPath, err)
		}
		keep = append(keep, pkg)
	}
	sort.Slice(keep, func(i, j int) bool { return keep[i].Path() < keep[j].Path() })
	return keep, nil
}

// goList runs `go list -e -json -deps -export ./...` in the module root with
// the context forced, and decodes the stream of package objects.
func goList(root string, c config) ([]listedPackage, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "-deps", "-export", "./...")
	cmd.Dir = root
	cmd.Env = contextEnv(c)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var list []listedPackage
	for {
		var p listedPackage
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		list = append(list, p)
	}
	return list, nil
}

// contextEnv is os.Environ with the context forced and the workspace disabled.
// Duplicate keys are dropped so the child cannot see two GOOS values.
func contextEnv(c config) []string {
	drop := map[string]bool{
		"GOOS": true, "GOARCH": true, "GOFLAGS": true, "GOWORK": true, "CGO_ENABLED": true,
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if drop[k] {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"GOOS="+c.goos,
		"GOARCH="+c.goarch,
		"CGO_ENABLED=0",
		"GOWORK=off",
		"GOFLAGS=-mod=readonly"+c.tagFlag(),
	)
	return env
}

func (c config) tagFlag() string {
	if len(c.tags) == 0 {
		return ""
	}
	return " -tags=" + strings.Join(c.tags, ",")
}

func (c config) describe() string {
	tags := "none"
	if len(c.tags) > 0 {
		tags = strings.Join(c.tags, ",")
	}
	return fmt.Sprintf("goos=%s goarch=%s tags=%s", c.goos, c.goarch, tags)
}

// include keeps the module's importable packages: root, hal/**, core/** and
// wgputest. Internal, test-only support packages, cmd, examples, tmp and tools
// are excluded.
func include(path string) bool {
	if path == modulePath {
		return true
	}
	rest, ok := strings.CutPrefix(path, modulePath+"/")
	if !ok {
		return false
	}
	if strings.Contains(rest, "/internal/") || strings.HasPrefix(rest, "internal/") {
		return false
	}
	switch {
	case rest == "hal", strings.HasPrefix(rest, "hal/"),
		rest == "core", strings.HasPrefix(rest, "core/"),
		rest == "wgputest":
		return true
	}
	return false
}

// render assembles the golden text: a deterministic header, then one block per
// package in path order.
func render(c config, pkgs []*types.Package) (string, stats, error) {
	var st stats
	blocks := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		block := FormatPackage(p)
		st.identifiers += countIdentifiers(p)
		blocks = append(blocks, block)
	}
	st.packages = len(pkgs)

	fl, err := floorFor(c)
	if err != nil {
		return "", st, err
	}
	if st.packages < fl.packages {
		return "", st, fmt.Errorf("floor: only %d public packages (min %d) for %s", st.packages, fl.packages, c.describe())
	}
	if st.identifiers < fl.identifiers {
		return "", st, fmt.Errorf("floor: only %d exported identifiers (min %d) for %s", st.identifiers, fl.identifiers, c.describe())
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# apigolden — signature-level public API of %s\n", modulePath)
	fmt.Fprintf(&b, "# context: %s\n", c.describe())
	fmt.Fprintf(&b, "# toolchain: %s\n", runtime.Version())
	fmt.Fprintf(&b, "# packages: %d  identifiers: %d\n", st.packages, st.identifiers)
	b.WriteString("#\n# Generated by tools/apigolden. Do not edit; regenerate and review the delta.\n")
	for _, block := range blocks {
		b.WriteString("\n")
		b.WriteString(block)
	}
	return b.String(), st, nil
}

func countIdentifiers(p *types.Package) int {
	n := 0
	for _, name := range p.Scope().Names() {
		if obj := p.Scope().Lookup(name); obj != nil && obj.Exported() {
			n++
		}
	}
	return n
}
