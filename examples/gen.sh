#!/usr/bin/env bash
# Rebuilds each example's .diff from its before (a/) and after (b/) trees.
# Then run `go test ./... -update` to refresh the expected output.
#
# An example directory may hold:
#   a/, b/    the files before and after the change
#   from      the name of another example whose a/ and b/ to reuse
#   context   lines of context to diff with (default: the whole file)
set -euo pipefail
cd "$(dirname "$0")"

for dir in testdata/*/; do
  name=$(basename "$dir")
  src="testdata/$name"
  [[ -f "$src/from" ]] && src="testdata/$(cat "$src/from")"
  ctx=$(cat "testdata/$name/context" 2>/dev/null || echo 1000000)
  set +e
  (cd "$src" && git -c core.quotepath=off diff --no-index --no-prefix --no-color --no-ext-diff -U"$ctx" a b) > "$name.diff"
  rc=$?
  set -e
  # git diff exits 1 when the trees differ, which is the point.
  # An empty diff means git could not read a tree (it reports that as exit 1
  # too), most often an a/ or b/ that is empty and so was never committed:
  # give it a .gitkeep.
  if [[ $rc -gt 1 || ! -s "$name.diff" ]]; then
    echo "gen.sh: no diff for $name; check that $src/a and $src/b both exist" >&2
    exit 1
  fi
done
