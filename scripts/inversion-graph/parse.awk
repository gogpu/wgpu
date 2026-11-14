# parse.awk — records = (file, imported package, build context).
# Inputs are passed by path (no absolute paths, no fixed locations):
#   -v tags=tags.tsv -v imports=imports.tsv -v testkind=testkind.tsv
# Output: file<TAB>import<TAB>context<TAB>class
#   class = N (non-test) | INT (internal test package) | EXT (external test package)
# A line is an import only if its content is not a comment; doc examples like
#   //\t"github.com/gogpu/wgpu/core"
# are therefore not records (they were counted by the first draft).

BEGIN {
  MOD = "github.com/gogpu/wgpu"
  while ((getline l < tags) > 0)     { n = index(l, "\t"); if (n) tag[substr(l,1,n-1)] = substr(l,n+1) }
  while ((getline l < testkind) > 0) { n = index(l, "\t"); if (n) kind[substr(l,1,n-1)] = substr(l,n+1) }
  while ((getline l < imports) > 0) {
    n1 = index(l, "\t");            if (!n1) continue
    path = substr(l, 1, n1-1)
    rest = substr(l, n1+1)
    n2 = index(rest, "\t");         if (!n2) continue
    content = substr(rest, n2+1)

    # skip comments and non-import contexts
    t = content; sub(/^[ \t]+/, "", t)
    if (t ~ /^\/\// || t ~ /^\/\*/) continue
    if (!match(content, /"[^"]*"/)) continue
    imp = substr(content, RSTART+1, RLENGTH-2)
    if (index(imp, MOD) != 1) continue
    suffix = substr(imp, length(MOD)+1); sub(/^\//, "", suffix)
    if (suffix == "") suffix = "(root)"    # a plain module import is the root package

    ctx = (path in tag) ? tag[path] : "all"
    cls = (path in kind) ? kind[path] : "N"
    k = path "\t" suffix "\t" ctx "\t" cls
    if (!(k in seen)) { seen[k] = 1; print k }
  }
}
