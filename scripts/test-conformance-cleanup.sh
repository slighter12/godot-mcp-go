#!/usr/bin/env sh
set -eu

test_dir="$(mktemp -d /tmp/godot-mcp-conformance-cleanup.XXXXXX)"
port="${CONFORMANCE_CLEANUP_TEST_PORT:-39190}"

cleanup() {
  rm -rf "$test_dir"
}
trap cleanup EXIT INT TERM

printf '%s\n' '#!/usr/bin/env sh' \
  'output_dir=""' \
  'while [ "$#" -gt 0 ]; do' \
  '  if [ "$1" = "--output-dir" ]; then shift; output_dir="$1"; fi' \
  '  shift' \
  'done' \
  'if [ -n "$output_dir" ]; then mkdir -p "$output_dir"; printf failed >"$output_dir/fake-report"; fi' \
  'if [ "${FAKE_BUNX_FAIL:-0}" = "1" ]; then exit 7; fi' \
  'exit 0' >"$test_dir/bunx"
chmod +x "$test_dir/bunx"

run_output="$(PATH="$test_dir:$PATH" CONFORMANCE_PORT="$port" \
  ./scripts/test-conformance-2026-07-28.sh)"

output_dir="$(printf '%s\n' "$run_output" | sed -n 's/^Conformance output: //p')"
if [ -z "$output_dir" ] || [ -d "$output_dir" ]; then
  echo "default conformance output directory was not cleaned: ${output_dir:-missing}"
  exit 1
fi

explicit_output="$test_dir/retained-output"
explicit_port=$((port + 1))
PATH="$test_dir:$PATH" CONFORMANCE_PORT="$explicit_port" CONFORMANCE_OUTPUT_DIR="$explicit_output" \
  ./scripts/test-conformance-2026-07-28.sh >/dev/null
if [ ! -d "$explicit_output" ]; then
  echo "explicit conformance output directory was not retained"
  exit 1
fi

failure_port=$((port + 2))
set +e
failure_output="$(PATH="$test_dir:$PATH" FAKE_BUNX_FAIL=1 CONFORMANCE_PORT="$failure_port" \
  ./scripts/test-conformance-2026-07-28.sh 2>&1)"
failure_status=$?
set -e
if [ "$failure_status" -ne 7 ]; then
  echo "expected conformance runner failure status 7, got $failure_status"
  exit 1
fi
retained_dir="$(printf '%s\n' "$failure_output" | sed -n 's/^Conformance failure output retained: //p')"
if [ -z "$retained_dir" ] || [ ! -f "$retained_dir/fake-report" ]; then
  echo "failed conformance output was not retained: ${retained_dir:-missing}"
  exit 1
fi
rm -rf "$retained_dir"

if curl -fsS "http://127.0.0.1:${port}/" >/dev/null 2>&1; then
  echo "conformance fixture still owns port ${port} after runner exit"
  exit 1
fi
if curl -fsS "http://127.0.0.1:${explicit_port}/" >/dev/null 2>&1; then
  echo "conformance fixture still owns port ${explicit_port} after runner exit"
  exit 1
fi
if curl -fsS "http://127.0.0.1:${failure_port}/" >/dev/null 2>&1; then
  echo "conformance fixture still owns port ${failure_port} after failed runner"
  exit 1
fi

echo "conformance fixture cleanup passed"
