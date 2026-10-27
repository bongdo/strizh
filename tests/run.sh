#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
strizh=$root/bin/strizh
if [ ! -x "$strizh" ]; then
  echo "tests/run.sh: build bin/strizh first" >&2
  exit 1
fi
fail=0
cleanup() {
  local d="$1"
  rm -rf "$d/.strizh" "$d/bin" 2>/dev/null || true
  find "$d" -name 'zz_strizh.go' -o -name 'zz_strizh_test.go' 2>/dev/null | while read -r f; do rm -f "$f"; done
  rm -f "$d/go.mod" "$d/go.sum" 2>/dev/null || true
}
for dir in "$root"/tests/cases/*/; do
  name=$(basename "$dir")
  out=$(mktemp)
  err=$(mktemp)
  if [ -f "$dir/test" ]; then
    if ! (cd "$dir" && timeout 180 "$strizh" test >"$out" 2>"$err"); then
      echo "FAIL $name (test)" >&2
      cat "$err" >&2
      fail=1
      rm -f "$out" "$err"
      cleanup "$dir"
      continue
    fi
    if [ -f "$dir/expected-test.txt" ] && ! diff -u "$dir/expected-test.txt" <(grep -E '^(ok|FAIL|---)' "$out" || true); then
      fail=1
    fi
    rm -f "$out" "$err"
    cleanup "$dir"
    continue
  fi
  if ! (cd "$dir" && timeout 120 "$strizh" run >"$out" 2>"$err"); then
    echo "FAIL $name (run)" >&2
    cat "$err" >&2
    fail=1
    rm -f "$out" "$err"
    cleanup "$dir"
    continue
  fi
  if [ -f "$dir/expected.txt" ]; then
    if ! diff -u "$dir/expected.txt" "$out"; then
      echo "FAIL $name (output)" >&2
      fail=1
    fi
  fi
  rm -f "$out" "$err"
  cleanup "$dir"
done
if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "tests: ok"
