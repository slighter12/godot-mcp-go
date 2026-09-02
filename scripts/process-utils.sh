#!/usr/bin/env sh

# terminate_process stops one child process without allowing cleanup to block
# indefinitely. Callers remain responsible for cleaning up external resources
# created by that process, such as Docker containers.
terminate_process() {
  process_id="${1:-}"
  if [ -z "$process_id" ]; then
    return 0
  fi
  if ! kill -0 "$process_id" >/dev/null 2>&1; then
    wait "$process_id" >/dev/null 2>&1 || true
    return 0
  fi
  kill "$process_id" >/dev/null 2>&1 || true
  attempts=0
  while kill -0 "$process_id" >/dev/null 2>&1 && [ "$attempts" -lt 20 ]; do
    sleep 0.1
    attempts=$((attempts + 1))
  done
  if kill -0 "$process_id" >/dev/null 2>&1; then
    kill -KILL "$process_id" >/dev/null 2>&1 || true
  fi
  wait "$process_id" >/dev/null 2>&1 || true
}

# run_with_deadline runs a command and returns 124 when it exceeds the supplied
# number of seconds. The active PID is exposed for signal-trap cleanup.
run_with_deadline() {
  deadline_seconds="$1"
  shift
  deadline_marker="$(mktemp /tmp/godot-mcp-go-deadline.XXXXXX)"
  rm -f "$deadline_marker"

  "$@" &
  RUN_WITH_DEADLINE_PID=$!
  export RUN_WITH_DEADLINE_PID
  (
    deadline_sleep_pid=""
    trap 'terminate_process "$deadline_sleep_pid"; exit 0' INT TERM
    sleep "$deadline_seconds" &
    deadline_sleep_pid=$!
    wait "$deadline_sleep_pid" || exit 0
    if kill -0 "$RUN_WITH_DEADLINE_PID" >/dev/null 2>&1; then
      : >"$deadline_marker"
      terminate_process "$RUN_WITH_DEADLINE_PID"
    fi
  ) >/dev/null 2>&1 &
  deadline_watchdog_pid=$!

  deadline_status=0
  wait "$RUN_WITH_DEADLINE_PID" || deadline_status=$?
  kill "$deadline_watchdog_pid" >/dev/null 2>&1 || true
  wait "$deadline_watchdog_pid" >/dev/null 2>&1 || true
  RUN_WITH_DEADLINE_PID=""
  export RUN_WITH_DEADLINE_PID

  if [ -f "$deadline_marker" ]; then
    deadline_status=124
  fi
  rm -f "$deadline_marker"
  return "$deadline_status"
}
