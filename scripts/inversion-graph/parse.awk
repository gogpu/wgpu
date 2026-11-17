# parse.awk — records = (file, imported package, build context).
# Inputs are passed by path (no absolute paths, no fixed locations):
#   -v tags=tags.tsv -v imports=imports.tsv -v testkind=testkind.tsv
# Output: file<TAB>import<TAB>context<TAB>class
#   class = N (non-test) | INT (internal test package) | EXT (external test package)
#
# A line is an import only if it is an import statement: a single-line `import [alias] "path"`, or an
# entry inside an import block (an optional alias and one quoted path on its own indented line). A line
# that merely quotes the module path is not an import — package documentation examples
# (`//\t"github.com/gogpu/wgpu/core"`) and a const such as tools/apigolden/main.go's
# `modulePath = "github.com/gogpu/wgpu"` were both counted by earlier drafts.

function is_import(content,   t) {
  t = content
  sub(/[ \t]*\/\/.*$/, "", t)              # a trailing line comment is not part of the statement
  sub(/^[ \t]+/, "", t)
  sub(/[ \t]+$/, "", t)
  # a single-line import, with an optional alias (`_`, `.` or an identifier)
  if (t ~ /^import[ \t]+([A-Za-z_.][A-Za-z0-9_.]*[ \t]+)?"[^"]*"$/) return 1
  # an entry inside an import block: indented, optional alias, one quoted path, no trailing comma
  if (content ~ /^[ \t]+/ && t ~ /^([A-Za-z_.][A-Za-z0-9_.]*[ \t]+)?"[^"]*"$/) return 1
  return 0
}

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

    # an import statement, not a comment and not any other line that quotes the module path
    if (!is_import(content)) continue
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
