# inversion-graph

Reproducible package import graph for the pre-v1.0 layout change (ADR-070), used by the layout check and by
reviewers of the refactor. It rebuilds the graph from git alone — no build, no module command, no network — maps
every package to its destination package, appends the edges the root extraction will create, and runs a
cycle check (Tarjan SCC) once per build context.

## Usage

```sh
sh graph.sh <repo-root> <out-dir>              # reads the committed tree (HEAD)
sh graph.sh --worktree <repo-root> <out-dir>   # reads the working tree of tracked files
```

Default reads `HEAD`: that is what CI checks, and it makes a local run mean "HEAD is clean". If you have uncommitted
changes and want them included, use `--worktree`. Both modes write only into `<out-dir>` and exit non-zero when the
scan falls below the floor (see below), so an empty or partial scan cannot report "acyclic".

## Pipeline

`graph.sh` → `parse.awk` → `map.awk` → `target-edges.tsv` (+ `inverted-edges.tsv`) → `scc.awk`

1. `graph.sh` emits `tags.tsv`, `imports.tsv` and `testkind.tsv` from `git grep`, then runs the stages.
2. `parse.awk` turns import lines into **records**; `map.awk` maps both endpoints and aggregates to **edges**.
3. `target-edges.tsv` adds the edges the extraction creates (they cannot appear as imports today, because the root
   package is one package). Each row cites the `file:line` that forces it.
4. `inverted-edges.tsv` lists the edges the inversion plan removes; `scc-after.txt` is the graph without them.
5. `scc.awk` runs Tarjan over the build graph, once per context.

## Units

| Unit | Definition |
|---|---|
| **import line** | one source line that imports a package of this module (comment lines are excluded) |
| **record** | `(file, imported package, build context)`; class is an attribute: `N` non-test, `INT` internal test (`package foo`), `EXT` external test (`package foo_test`) |
| **edge** | `(source package, destination package, build context)` with both endpoints mapped to their destination |
| **pair** | `(source package, destination package)` — an edge with the context dropped |

An edge is in the build graph when at least one `N` or `INT` file produces it. `EXT`-only edges come from
`package foo_test` files, which Go compiles as separate packages: they can break layering rules but never form an
import cycle, so they are excluded from the SCC and listed separately.

## Build contexts

A constraint is not a bucket: a file tagged `!(js && wasm)` compiles in the native *and* the rust build, and
`!rust` admits native and wasm. Each edge therefore carries a set over the three contexts that are built and gated:

| Constraint | Contexts |
|---|---|
| no tag | native + rust + wasm |
| `!(js && wasm)` | native + rust |
| `!rust && !(js && wasm)`, `integration` | native |
| a GOOS token with `!rust` and no `js` | native |
| a GOOS token alone (`windows`, `linux`, `android && arm64`) | native + rust — a GOOS atom holds under `-tags rust` too, and it can never be wasm |
| any other plain tag (`swinstrcount`, …) | native + rust |
| `rust`, `rust && windows` | rust |
| `js && wasm` | wasm |
| `rust \|\| (js && wasm)` | rust + wasm |

The hand-written B side uses the explicit tokens `all`, `native`, `rust`, `wasm` and `native+rust`; they are matched
before any expression rule, so `native` never leaks into rust. wasm is admitted only by a constraint that positively
requires `js`.

### The constraint grammar (the gate's contract)

`scc.awk` decides a context with a string heuristic (`ctxmask`), which is enough to run Tarjan over the tree it
reads, but it is not a contract. The layout check (`internal/layoutcheck`) evaluates the build constraint of every
generated B-side edge exactly, as a boolean expression. This is the grammar it accepts:

```
expr   := or
or     := and ( '||' and )*
and    := unary ( '&&' unary )*
unary  := '!' unary | '(' expr ')' | atom
atom   := one identifier
```

Space and tab separate tokens; an optional `//go:build ` prefix is accepted. An empty constraint, or the token
`all`, admits every context. An atom is one of:

- `js`, `wasm` — the wasm context only;
- `rust` — the rust context only;
- `integration` — the native context only;
- any other identifier — a GOOS token (`windows`, `linux`, `darwin`, `android`, …), an architecture token (`386`,
  `amd64`, `arm64`, …) or any other plain tag (`wgpu_nosoftware`, `wgpu_novulkan`, `swinstrcount`, …): native +
  rust, never wasm.

`||` is set union and `&&` is set intersection over the three contexts. Negation is where the abstraction shows:

| Expression | Contexts |
|---|---|
| `!js`, `!wasm` | native + rust — the complement of the wasm context |
| `!rust` | native + wasm — the complement of the rust context |
| `!integration` | rust + wasm — the complement of the native context |
| `!<GOOS>`, `!<arch>`, `!<plain tag>` | native + rust — **not** the complement: the abstraction cannot decide a GOOS, because the native and rust builds target an unspecified platform, so the negation holds there as well |
| `!(group)` built only from `js`, `wasm`, `rust`, `integration` | the complement of the group's set |
| `!(group)` that names a GOOS, an architecture or any other plain tag | native + rust |
| `!!x` | `x` — a boolean identity |

**The hard rule.** An expression outside this grammar — a missing parenthesis, a trailing token, a dangling `&&` or
`||` — or one that evaluates to the empty context set (`wasm && rust`, `rust && !rust`) is a **hard failure that
quotes the expression**. It is never a guess and never a silent pass: an empty set must not be able to satisfy "this
edge is covered by the golden" vacuously, and a constraint no one can read must not be allowed to pick a bucket.
`ctxmask`'s heuristic is not a fallback: where the tool and the evaluator could disagree, the gate fails instead.

**The proof.** These are the tests, not a claim in prose:

- `TestConstraintContexts` pins the grammar and the closure table above, error cases included;
  `TestConstraintEmptySetIsAnError` pins the empty-set rule.
- `TestConstraintTreeExpressions` pins all 42 real build constraints of the tool's own `tags.tsv` against
  `scc.awk`'s `ctxmask`: the evaluator and the tool agree on every one. (`tags.tsv` holds 43 distinct values; the
  43rd is the source line in `cmd/vk-gen/main.go` that writes a `//go:build` string for generated code, not a
  constraint of this graph.)
- `TestConstraintTreeZeroFixes` pins the 14 tree constraints that the earlier atom-complement model collapsed to
  an empty set. The two `wgputest/internal/backend` cases show why the closure rules matter:

| Constraint | Earlier model | This evaluator | `ctxmask` |
|---|---|---|---|
| `opensoftware.go`: `((windows && (386 \|\| amd64 \|\| arm64)) \|\| …) && !rust && !(js && wasm) && !wgpu_nosoftware` | empty, rejected | native | native |
| `opensoftware_off.go`: `!rust && !(js && wasm) && !(((windows && …) \|\| …) && !wgpu_nosoftware)` | native — but only by complementing an empty negated group | native | native |

The off expression is the one that shows the shape of the defect rather than a wrong number: its negated group names
a plain tag, so under the atom-complement model the inner `… && !wgpu_nosoftware` came out empty and `!(empty)` came
out as everything. The final value matched by accident. An empty set is now an error, so the same expression can
only pass through the rule that a negation naming a plain atom is native + rust.

## Outputs

| File | Content |
|---|---|
| `tags.tsv`, `imports.tsv`, `testkind.tsv` | raw inputs, reproducible from git |
| `records.tsv` | every record |
| `edges-a.tsv` | edges from today's imports, mapped to destinations |
| `edges-all.tsv` | A + B — the graph the change plans to build |
| `edges-after.tsv` | A + B minus the inverted edges |
| `edges-ext.tsv` | external-test-only edges (layering findings, never cycles) |
| `pairs.tsv` | distinct (source, destination) pairs over A+B |
| `scc.txt` | Tarjan per context over A+B |
| `scc-after.txt` | Tarjan per context over A+B minus the inversion |
| `summary.txt` | counts, floors, cycle counts |

The pinned output in `reference/` holds every file a run produces, raw inputs included, so the derivation is
checkable end to end against the commit that pinned it. Refreshing the pin is the command in "Regenerating the
reference" pointed at `scripts/inversion-graph/reference`, and the change goes in the same commit as what caused it.

**Note (2026-11-15):** ADR-070 §15.2 supersedes "the same commit": `reference/` is the dated HEAD `84e547f`
snapshot, and a re-run at a later head is expected to differ. Refresh the pin only as a deliberate re-baseline,
never as a gate.

## Reference numbers

Measured at the commit this directory was added; the pinned output in `reference/` is authoritative.

| Measure | Value |
|---|---|
| lines mentioning the module path | 886, of which 11 are comments → 875 import statements |
| records | 875 (one per import statement) |
| edges, A only | 172 |
| pairs, A only | 123 (all classes) / 115 (build graph only) |
| packages, A only | 54 |
| edges, A+B | 217 |
| pairs, A+B | 168 |
| packages, A+B | 63 |
| edges per context, A+B | native 156, rust 125, wasm 52 |
| planned B-side edges | 54 rows in `target-edges.tsv` (the reviewed intent; the generated totals are reported by `graph.sh` at run time) |
| inverted edges | 7 rows in `inverted-edges.tsv` |
| cycles, A+B | native: 1 SCC of 8 packages; rust: 1 SCC of 6; wasm: none |
| cycles, A+B minus the inversion | 0 in every context |

Reconciliation with earlier figures quoted in review: 879 = 875 + 4 indented comment lines that a column-0 comment
filter keeps (they mention an issue URL, not an import); 176 = 172 + the 4 edges those lines would create; 127 = 123 +
the same 4 pairs. A per-context figure of 26 for rust comes from assigning each constraint to one bucket instead of a
set — a `!(js && wasm)` file does compile under `-tags rust`.

## Floor

An empty or partial scan must fail loudly rather than report a clean graph. `graph.sh` exits non-zero below:

| Input | Floor |
|---|---|
| import lines | 800 |
| records | 600 |
| edges (A+B) | 120 |
| pairs | 100 |
| packages | 45 |

The floors sit well below the reference run and far above a broken scan, which collapses to tens of lines. The layout
check must treat a non-zero exit as a failed check, never as "acyclic".

## Cycle results

`scc.txt` is the graph as planned, before the inversion: native has one SCC of 8 packages
(`device, queue, pendingwrites, encoder, resource, pipeline, instance, surface`), rust one SCC of 6
(`device, encoder, resource, pipeline, instance, surface`); wasm is acyclic. `scc-after.txt` is the same graph minus
`inverted-edges.tsv` and is acyclic in every context, which is the check that the inversion closes the cycle rather
than moving it. A `scc-after.txt` that reports a cycle means the inversion list is incomplete.

## Regenerating the reference

```sh
sh scripts/inversion-graph/graph.sh . <out-dir>
diff <out-dir>/summary.txt scripts/inversion-graph/reference/summary.txt
```

Any difference outside `summary.txt` is a change in the graph, and the pinned output in `reference/` must be updated
in the same commit as the change that caused it.

**Note (2026-11-15):** the ADR-070 §15.2 rule wins over "in the same commit as the change": the pinned output is the
dated HEAD `84e547f` snapshot, a re-run at a later head is expected to differ, and the pin is refreshed only as a
deliberate re-baseline, never as a gate.

**Note (2026-11-15, S0.7d):** `map.awk` now maps the public thin `hal` to `hal`; before the fix it mapped it to
`internal/hal`, merging the declarations-only leaf with the backend tree. `internal/hal/**` maps to itself, and the
legacy pre-A `hal/**` paths still map under `internal/hal/**`, so older rows resolve. The table above and the pinned
`reference/` tree were produced before the fix and stay as the dated snapshot (ADR-070 §14.1, §15.2); a run at the
current head reports the corrected numbers.

## claims.tsv

`claims.tsv` is the machine-readable claim of every golden row by the B step that creates it (ADR-070 §15.2). It sits
next to the goldens and is part of the pinned tool set: a change to it is a claim change, reviewed like the golden.

- **Header:** one `phase: open|final` line. `open` during the window — unclaimed rows are expected. The S13 sweep sets
  `final`; from then every golden row must be claimed.
- **Data rows:** five tab-separated columns — `golden` (`target|inverted`), `src`, `dst`, `context`
  (`all|native|native+rust|rust|wasm`), `step` (`S1`…`S13`).
- **Who adds rows:** the step's own commit, exactly the rows that step creates; the commit message carries the same
  `Migrates: <src>→<dst> (<context>)` trailer as the human record. Never add another step's rows.
- **The gate** (`internal/layoutcheck`) fails on a duplicated row, a row that names no golden row, a malformed line,
  and — once `phase: final` — an unclaimed golden row.
- **2026-11-15 — inverted rows are claims-only; the twin rule.** The seven child→device pairs no longer exist as
  `target` rows: a target row may never legitimize an edge an `inverted` row forbids, because `covers()` matches
  generated edges against `target` rows only — a target twin would let the inversion's edge pass unnoticed. The six
  inverted contexts are narrowed to the field sites' own build tags (queue native; encoder/resource/pipeline
  native+rust; surface→device all; surface→instance native; device→instance native+rust), and S1's claim follows.
  Enforcement: the subset predicate fails on any generated edge not covered by a `target` row (the SCC runs on the
  post-inversion graph, so it cannot prove absence); `TestNoTargetInvertedTwins` in `internal/layoutcheck` fails on
  any remaining pair. Proof both ways, 2026-11-15: a planted native `internal/resource → internal/device` import and
  a planted native `internal/resource → internal/browser` import each red the gate in a scratch copy of the tree,
  while the same copy clean runs green. Row history: 45 (draft) → 61 (the class enumeration) → 54 (this correction).

## Notes

* `surface_rust.go:34` is the line that declares the `device` field of the rust `Surface`; a review note citing `:36`
  is a misread.
* `target-edges.tsv` and `inverted-edges.tsv` are hand-maintained data: every row is a decision, and every row must
  cite the line that forces it. Editing them is an architecture change, not a tooling change.
* Context rule, current revision: a plain GOOS token or any other plain tag is **native + rust, never wasm** (a GOOS
  atom holds under `-tags rust` too). Before this fix those files counted native-only, so the rust context read 104
  edges; it reads 124 now. SCC membership did not change (same six packages) and no summary number moved.
