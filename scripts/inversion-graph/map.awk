# map.awk — map both endpoints of every record to their ADR-070 destination
# package and aggregate to edges: (source package, destination package, context)
# with the set of file classes that produce it.
# Input : records.tsv  file<TAB>import<TAB>context<TAB>class
# Output: src<TAB>dst<TAB>context<TAB>classes       classes over N / INT / EXT
#
# Destination rules (README §2). Keep in one place: a path that is not listed
# here is already at its destination and maps to itself.

function dest(p) {
  if (p == "(root)")        return "wgpu"
  if (p == "core")          return "internal/core"
  if (p == "core/track")    return "internal/core/track"
  if (p == "core/testutil") return "internal/core/testutil"
  if (p == "hal")           return "internal/hal"
  if (p ~ /^hal\//)         return "internal/" p
  if (p == "internal/thread") return "cmd/internal/thread"
  if (p == "internal/eglrefs") return "internal/hal/gles/internal/eglrefs"
  return p
}

BEGIN { FS = "\t" }

{
  f = $1; imp = dest($2); ctx = $3; cls = $4
  if (imp == "(root)") imp = "wgpu"
  src = f
  if (!sub(/\/[^\/]*$/, "", src)) src = "(root)"
  src = dest(src)
  if (src == imp) next
  k = src "\t" imp "\t" ctx
  if (index(classes[k], cls) == 0) classes[k] = classes[k] (classes[k] == "" ? "" : "+") cls
}

END { for (k in classes) print k "\t" classes[k] }
