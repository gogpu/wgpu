#!/bin/sh
# graph.sh — build the package import graph (A side) and append the planned
# extraction edges (B side), then run the cycle check per build context.
#
#   usage: sh graph.sh [--worktree] <repo-root> <out-dir>
#
# Default reads the committed tree (HEAD); --worktree reads the working tree of
# tracked files. Reads only through git, writes only into <out-dir>. No absolute
# paths, no build, no module command.
#
# Every stage is checked on its own: a failed `git grep`, an empty stage output or
# a mismatch between the A side, the B side and the merged file exits non-zero
# instead of silently producing a graph that looks clean.
set -eu

REV=HEAD
if [ "${1:-}" = "--worktree" ]; then REV=""; shift; fi

ROOT=${1:?usage: graph.sh [--worktree] <repo-root> <out-dir>}
OUT=${2:?usage: graph.sh [--worktree] <repo-root> <out-dir>}
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

# Floor (README §Floor). A scan below any of them is a failure, not a clean graph.
MIN_IMPORT_LINES=800
MIN_RECORDS=600
MIN_PAIRS=100
MIN_EDGES=120
MIN_PACKAGES=45

fail() { echo "graph.sh: $*" >&2; exit 1; }

mkdir -p "$OUT"

# --- stage 0: raw inputs, each grep checked on its own -----------------------
# grep_into <pattern> <pathspec> <basename>
grep_into() {
  if ! git -C "$ROOT" grep -n "$1" $REV -- "$2" > "$3.raw" 2> "$3.err"; then
    fail "stage 0: git grep failed for '$1' in $2 (rev='${REV:-<worktree>}'): $(cat "$3.err")"
  fi
  [ -s "$3.raw" ] || fail "stage 0: git grep matched nothing for '$1' in $2"
}

grep_into 'go:build'              '*.go'       "$OUT/tags"
grep_into 'github.com/gogpu/wgpu' '*.go'       "$OUT/imports"
grep_into '^package'              '*_test.go'  "$OUT/testkind"

sed 's/^HEAD://' "$OUT/tags.raw" |
  awk -F: '{p=$1; rest=$0; for(i=1;i<=2;i++) sub(/^[^:]*:/,"",rest); print p "\t" rest}' |
  awk -F'\t' '!seen[$1]++' | sort > "$OUT/tags.tsv" || fail "stage 0: tag transform failed"
[ -s "$OUT/tags.tsv" ] || fail "stage 0: tags.tsv is empty"

sed 's/^HEAD://' "$OUT/imports.raw" |
  awk -F: '{p=$1; ln=$2; rest=$0; for(i=1;i<=2;i++) sub(/^[^:]*:/,"",rest); print p "\t" ln "\t" rest}' |
  sort > "$OUT/imports.tsv" || fail "stage 0: import transform failed"
[ -s "$OUT/imports.tsv" ] || fail "stage 0: imports.tsv is empty"

sed 's/^HEAD://' "$OUT/testkind.raw" |
  awk -F: '{p=$1; rest=$0; for(i=1;i<=2;i++) sub(/^[^:]*:/,"",rest); n=split(rest,a," ");
            if (a[n] ~ /_test$/) print p "\tEXT"; else print p "\tINT"}' |
  awk -F'\t' '!seen[$1]++' | sort > "$OUT/testkind.tsv" || fail "stage 0: test-kind transform failed"
[ -s "$OUT/testkind.tsv" ] || fail "stage 0: testkind.tsv is empty"

# intermediates are not part of the output set (kept on failure for diagnosis)
rm -f "$OUT/tags.raw" "$OUT/tags.err" "$OUT/imports.raw" "$OUT/imports.err" \
      "$OUT/testkind.raw" "$OUT/testkind.err"

# --- stage 1: records -> edges (A side) --------------------------------------
awk -v tags="$OUT/tags.tsv" -v imports="$OUT/imports.tsv" -v testkind="$OUT/testkind.tsv" \
    -f "$HERE/parse.awk" > "$OUT/records.tsv" || fail "stage 1: parse.awk failed"
[ -s "$OUT/records.tsv" ] || fail "stage 1: records.tsv is empty"

awk -F'\t' -f "$HERE/map.awk" "$OUT/records.tsv" > "$OUT/edges-a.tsv" || fail "stage 1: map.awk failed"
[ -s "$OUT/edges-a.tsv" ] || fail "stage 1: edges-a.tsv is empty"

# --- stage 2: B side (planned extraction edges, hand-verified) ---------------
B_ROWS=$(grep -vc '^#' "$HERE/target-edges.tsv" || true)
[ "${B_ROWS:-0}" -gt 0 ] || fail "stage 2: target-edges.tsv has no data rows"
cat "$OUT/edges-a.tsv" "$HERE/target-edges.tsv" | grep -v '^#' > "$OUT/edges-all.tsv" \
  || fail "stage 2: merge of A and B failed"

A_ROWS=$(wc -l < "$OUT/edges-a.tsv" | tr -d ' ')
ALL_ROWS=$(wc -l < "$OUT/edges-all.tsv" | tr -d ' ')
[ "$ALL_ROWS" -eq "$((A_ROWS + B_ROWS))" ] \
  || fail "stage 2: edge count mismatch (A=$A_ROWS B=$B_ROWS merged=$ALL_ROWS)"

# --- stage 3: cycle check per context ----------------------------------------
awk -f "$HERE/scc.awk" "$OUT/edges-all.tsv" > "$OUT/scc.txt" || fail "stage 3: scc.awk failed"
grep -q '^== total cycles:' "$OUT/scc.txt" || fail "stage 3: scc.txt has no summary line"

awk -F'\t' 'NR==FNR { cut[$1 "\t" $2] = 1; next } !(($1 "\t" $2) in cut)' \
    "$HERE/inverted-edges.tsv" "$OUT/edges-all.tsv" > "$OUT/edges-after.tsv" \
  || fail "stage 3: building edges-after.tsv failed"
[ -s "$OUT/edges-after.tsv" ] || fail "stage 3: edges-after.tsv is empty"

awk -f "$HERE/scc.awk" "$OUT/edges-after.tsv" > "$OUT/scc-after.txt" || fail "stage 3: scc.awk (after) failed"
grep -q '^== total cycles:' "$OUT/scc-after.txt" || fail "stage 3: scc-after.txt has no summary line"

# --- stage 4: external-test edges, pairs, summary ----------------------------
awk -F'\t' '$4 !~ /(^|\+)N(\+|$)/ && $4 !~ /(^|\+)INT(\+|$)/' "$OUT/edges-all.tsv" \
    > "$OUT/edges-ext.tsv" || fail "stage 4: edges-ext.tsv failed"

awk -F'\t' '{print $1 "\t" $2}' "$OUT/edges-all.tsv" | sort -u > "$OUT/pairs.tsv" \
  || fail "stage 4: pairs.tsv failed"
[ -s "$OUT/pairs.tsv" ] || fail "stage 4: pairs.tsv is empty"

IMPORT_LINES=$(wc -l < "$OUT/imports.tsv" | tr -d ' ')
RECORDS=$(wc -l < "$OUT/records.tsv" | tr -d ' ')
EDGES=$ALL_ROWS
PAIRS=$(wc -l < "$OUT/pairs.tsv" | tr -d ' ')
PACKAGES=$(awk -F'\t' '{n[$1]=1; n[$2]=1} END{print length(n)}' "$OUT/edges-all.tsv")
CYCLES=$(grep -c 'CYCLE' "$OUT/scc.txt" || true)
CYCLES_AFTER=$(grep -c 'CYCLE' "$OUT/scc-after.txt" || true)

{
  echo "imports.tsv lines : $IMPORT_LINES   (floor $MIN_IMPORT_LINES)"
  echo "records           : $RECORDS   (floor $MIN_RECORDS)"
  echo "edges (A+B)       : $EDGES   (floor $MIN_EDGES)"
  echo "pairs             : $PAIRS   (floor $MIN_PAIRS)"
  echo "packages          : $PACKAGES   (floor $MIN_PACKAGES)"
  echo "cycles, A+B       : $CYCLES"
  echo "cycles, after Q12 : $CYCLES_AFTER"
} > "$OUT/summary.txt"
cat "$OUT/summary.txt"

FAIL=0
[ "$IMPORT_LINES" -ge "$MIN_IMPORT_LINES" ] || { echo "FLOOR FAILED: import lines" >&2; FAIL=1; }
[ "$RECORDS" -ge "$MIN_RECORDS" ]          || { echo "FLOOR FAILED: records" >&2; FAIL=1; }
[ "$EDGES" -ge "$MIN_EDGES" ]              || { echo "FLOOR FAILED: edges" >&2; FAIL=1; }
[ "$PAIRS" -ge "$MIN_PAIRS" ]              || { echo "FLOOR FAILED: pairs" >&2; FAIL=1; }
[ "$PACKAGES" -ge "$MIN_PACKAGES" ]        || { echo "FLOOR FAILED: packages" >&2; FAIL=1; }
exit "$FAIL"
