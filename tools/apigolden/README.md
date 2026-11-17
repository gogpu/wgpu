# apigolden

Signature-level golden of this module's public API, one file per build context. It exists so that the pre-v1.0
layout change (ADR-070) can prove it does not change the frozen surface: a regeneration that differs from the
baseline is a reviewed delta, not a surprise at v1.0.

## What it records

* **Packages**: the module's importable packages. Since the v0.35.0 internal-layout change (ADR-070) that set is
  three packages — the root module, the thin `hal` (declarations only) and `wgputest`. Excluded: `internal/**` (which
  now holds the former `core/` and backend `hal/**` packages), `cmd/**`, `examples/**`, `tmp/**`, `tools/**`.
* **Per exported identifier**, sorted and stable:
  * constants — `const Name Type = exact-value`;
  * variables — `var Name Type`;
  * types — `type Name <underlying>`; for a struct every exported field with its type **in declaration order**,
    embedded fields as the type alone; for an interface every method signature and every embedded type; an alias is
    printed as `type Name = Rhs   # alias`, so `type X = Y` can never be mistaken for a defined type;
  * functions — the full signature;
  * exported methods declared on exported receivers — the full signature.
* Unexported identifiers, unexported fields and methods, doc comments and struct tags are **not** recorded.
* The header's `identifiers` count counts **exported package-scope names only**. Exported methods are printed on
  their receiver's block but not counted (`countIdentifiers` in `main.go`), so a floor derived from this number
  pins the top-level surface, not every method — a declaration added as a method moves the text, not the count.

## Commands

Run from the repository root; `GOWORK=off` is required because the developer workspace above the repository does not
contain the `tools` module. The generator loads the analysed module with `GOWORK=off` as well, so a golden describes
the module and never a local workspace.

```sh
# windows/amd64, default build
GOWORK=off go -C tools run ./apigolden -out apigolden/testdata/api_windows.txt

# windows/amd64 with -tags rust
GOWORK=off go -C tools run ./apigolden -tags rust -out apigolden/testdata/api_rust.txt

# js/wasm
GOWORK=off go -C tools run ./apigolden -goos js -goarch wasm -out apigolden/testdata/api_wasm.txt
```

The module root is found by walking up to the `go.mod` whose module path is exactly `github.com/gogpu/wgpu`, so the
tool also works when invoked from `tools/` or from a copy of the repository.

## Determinism

Two runs on the same tree are byte-identical: the output has no timestamp, no absolute path, no Go version and no map
iteration order (packages sorted by path, identifiers by name, methods by name; struct fields keep declaration order).
Check it with:

```sh
GOWORK=off go -C tools run ./apigolden -out g1.txt && cmp apigolden/testdata/api_windows.txt g1.txt && echo identical
```

## Floors

A partial or empty load must fail loudly instead of writing a short golden that still looks valid. Each supported
context has its own floor, and a context that is not listed is refused:

| Context | Packages | Identifiers |
|---|---:|---:|
| `windows/amd64` | 3 | 258 |
| `windows/amd64 -tags rust` | 3 | 246 |
| `js/wasm` | 3 | 241 |

`-min-packages` and `-min-identifiers` override the context floor when **both** are passed (a half-set override is
refused). They exist for the v0.35.0 window: after the internal-layout move the public set shrank to three packages,
and the A5 commit regenerated with the same binary and pinned these floors to the exact generated values; the
layoutcheck gate then uses them.

## Baselines

The current baseline is the post-A golden committed in `testdata/` on 2026-11-15 (commit `3eb1f41`, the
regeneration after the implementation moved into `internal/`): 3 packages and 258 / 246 / 241 identifiers, the same
numbers the Floors table pins.

| File | Context | Packages | Identifiers |
|---|---|---:|---:|
| `testdata/api_windows.txt` | windows/amd64 | 3 | 258 |
| `testdata/api_rust.txt` | windows/amd64 `-tags rust` | 3 | 246 |
| `testdata/api_wasm.txt` | js/wasm | 3 | 241 |

History: the comparison point for the v0.35.0 window was `a3705c2` (2026-11-15, the A1 re-take after the seam
patch, PR #410; the earlier snapshot at `4577b0e` is superseded) — 23 packages and 9,109 / 9,097 / 5,834
identifiers. Counting the same measure — exported package-scope identifiers per package, so methods and struct
fields are not counted — 6,517 of those 9,109 identifiers sit in the two generated binding packages:
`hal/vulkan/vk` holds 5,672 and `hal/dx12/d3d12` 845 (`git show a3705c2:tools/apigolden/testdata/api_windows.txt`,
one `package` block per package). Both became internal in the layout change, so the first delta was large and
expected.

A hunk walk of `a3705c2` against the post-A golden gives 11 / 8 / 8 hunks with default context and 40 / 32 / 27
with `-U0` (`-U0` splits adjacent blocks that the default context merges):

```sh
git diff a3705c2..3eb1f41 -- tools/apigolden/testdata/api_windows.txt | grep -c '^@@'    # 11 (rust 8, wasm 8)
git diff -U0 a3705c2..3eb1f41 -- tools/apigolden/testdata/api_windows.txt | grep -c '^@@' # 40 (rust 32, wasm 27)
```

## Honest limits

* **Signatures, not runtime identity.** The golden records types and signatures. It cannot prove that the seven
  re-exported sentinels (`ErrDeviceLost`, `ErrOutOfMemory`, `ErrSurfaceLost`, `ErrSurfaceOutdated`, `ErrTimeout`,
  `ErrContextReleased`, `ErrWorkgroupBarrierUnsupported`) keep their `errors.Is` identity — that is a separate test.
* **Struct tags are not recorded.** A tag-only change does not move a baseline.
* **Three contexts only**: windows/amd64, windows/amd64 `-tags rust`, js/wasm. A change confined to darwin/metal,
  linux, android, 386, or to an opt-out tag (`wgpu_nodx12`, `wgpu_nogles`, `wgpu_novulkan`, `wgpu_nometal`,
  `wgpu_moltenvk`, `wgpu_nosoftware`) will not move a baseline.
* **It does not compare.** Diffing a fresh run against the baselines is the review step; the tool writes a file.

## Tests

```sh
GOWORK=off go test ./apigolden -run 'TestFormat|TestSorting|TestUnexported'
```

The formatter tests build packages in memory (`go/types`), so they need no module load and no network.

## Module note

`tools/` is its own module. Adding `golang.org/x/tools v0.52.0` raised its `go` directive to 1.26.0; the main
module's `go.mod` and `go.sum` are untouched.
