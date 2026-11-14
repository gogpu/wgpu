# scc.awk — Tarjan SCC over the target graph, once per real build context.
# Input : src<TAB>dst<TAB>context<TAB>classes   (classes = N / INT / EXT combinations)
# Output: per context, the edge count and every SCC with size > 1.
#
# A constraint is not a bucket: `!(js && wasm)` admits both the native and the rust
# build, and `!rust` admits native and wasm. Each edge therefore carries a SET of
# contexts and is included in every context it admits. The three contexts are the
# ones the window builds and gates: native, rust (build tag), wasm (js && wasm).
#
# An edge belongs to the graph when at least one non-test (N) or internal-test (INT)
# file creates it. EXT-only edges come from `package foo_test` files, which Go builds
# as separate packages: they can break layering but never form an import cycle.

function ctxmask(ctx,   pr) {
  pr = (ctx ~ /(^|[^!])rust/)                               # a positive `rust` tag, not `!rust`
  # explicit tokens of the hand-written B side and of `all`
  if (ctx == "all")                         return 7          # native+rust+wasm
  if (ctx == "native")                      return 1
  if (ctx == "rust")                        return 2
  if (ctx == "wasm")                        return 4
  if (ctx == "native+rust")                 return 3
  if (ctx ~ /integration/)                  return 1          # integration tests run native only
  # wasm is admitted only when the constraint positively requires js
  if (pr && ctx ~ /js && wasm/)             return 6          # rust || (js && wasm)
  if (ctx ~ /js && wasm/ && ctx !~ /!/ && !pr) return 4       # js && wasm
  if (ctx ~ /!rust/)                        return 1          # rust excluded -> native, never wasm
  if (pr)                                   return 2          # explicit rust tag
  return 3    # GOOS-only or plain-tag constraint: native+rust (a GOOS atom holds under -tags rust too)
}

function strongconnect(v,   w, key, p, n, i, out) {
  idx[v] = counter; low[v] = counter; counter++
  stack[++sp] = v; onstack[v] = 1

  for (key in adj) {
    split(key, p, SUBSEP)
    if (p[1] != v) continue
    w = p[2]
    if (!(w in idx)) { strongconnect(w); if (low[w] < low[v]) low[v] = low[w] }
    else if (onstack[w] && idx[w] < low[v]) low[v] = idx[w]
  }

  if (low[v] == idx[v]) {
    n = 0
    do { w = stack[sp--]; onstack[w] = 0; scc[++n] = w } while (w != v)
    if (n > 1) {
      out = ""
      for (i = 1; i <= n; i++) out = out (i > 1 ? " <-> " : "") scc[i]
      cycles++
      print "  CYCLE (" n " packages): " out
    }
  }
}

BEGIN { FS = "\t" }

{
  if ($1 == "" || $2 == "" || $1 == $2) next
  if ($4 !~ /(^|\+)N(\+|$)/ && $4 !~ /(^|\+)INT(\+|$)/) next   # EXT-only edge
  e[$1 SUBSEP $2] = e[$1 SUBSEP $2] " " ctxmask($3)
}

END {
  n = split("native rust wasm", BK, " ")
  for (bi = 1; bi <= n; bi++) {
    bit = 2 ^ (bi - 1)                                       # 1, 2, 4
    delete adj; delete idx; delete low; delete stack; delete onstack; delete scc
    counter = 0; sp = 0; cycles = 0; edges = 0
    for (k in e) {
      split(k, p, SUBSEP)
      mask = 0
      m = split(e[k], mm, " ")
      for (i = 1; i <= m; i++) mask = or(mask, mm[i])
      if (!and(mask, bit)) continue
      adj[p[1] SUBSEP p[2]] = 1
      edges++
    }
    print "== context " BK[bi] " — " edges " edges =="
    if (edges == 0) { print "  (none)"; continue }
    for (k in adj) { split(k, p, SUBSEP); if (!(p[1] in idx)) strongconnect(p[1]) }
    if (cycles == 0) print "  acyclic (no SCC > 1)"
    total += cycles
  }
  print "== total cycles: " total " =="
}
