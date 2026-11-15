# apigolden

Signature-level golden of this module's public API, one file per build context. It exists so that the pre-v1.0
layout change (ADR-070) can prove it does not change the frozen surface: a regeneration that differs from the
baseline is a reviewed delta, not a surprise at v1.0.

## What it records

* **Packages**: the module's importable packages — root, `hal/**` (bindings included), `core/**` and `wgputest`.
  Excluded: `internal/**`, `cmd/**`, `examples/**`, `tmp/**`, `tools/**`.
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
| `windows/amd64` | 20 | 8200 |
| `windows/amd64 -tags rust` | 20 | 8200 |
| `js/wasm` | 7 | 5200 |

`-min-packages` and `-min-identifiers` override the context floor when **both** are passed (a half-set override is
refused). They exist for the v0.35.0 window: after the internal-layout move the public set shrinks to three packages,
and the A5 commit regenerates with the same binary and pins the post-window floors to the exact generated values; the
layoutcheck gate then uses those.

## Baselines

Generated at `main` **4577b0e**.

| File | Context | Packages | Identifiers | Bytes | Lines |
|---|---|---:|---:|---:|---:|
| `testdata/api_windows.txt` | windows/amd64 | 23 | 9,107 | 1,037,893 | 31,666 |
| `testdata/api_rust.txt` | windows/amd64 `-tags rust` | 23 | 9,095 | 1,034,922 | 31,602 |
| `testdata/api_wasm.txt` | js/wasm | 8 | 5,832 | 589,610 | 20,414 |

About 7,300 of the windows identifiers are the generated binding packages `hal/vulkan/vk` (6,270 declaration lines)
and `hal/dx12/d3d12` (1,017). They are public today and become internal in the layout change, so the first delta is
large and expected: after it the golden describes the root package, the thin `hal` and `wgputest`.

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
