#!/usr/bin/env bash
# gate_command.sh — run one gate command and propagate its TRUE exit code.
#
# Minimal test-execution contract (TH-P05-13, Scope B):
#   - The gate exits non-zero whenever the wrapped command fails. Piping a
#     command into `tail`/`head` (`cmd | tail`) is FORBIDDEN in gate paths
#     because the pipeline's exit status becomes tail's, masking failures.
#   - With GATE_LOG_FILE set, output goes to that file and the last 80
#     lines are shown on stderr AFTER status is captured — the safe pattern
#     `command > logfile 2>&1; status=$?; tail -n 80 logfile; exit $status`.
#   - Timeouts/usage errors exit 2; the wrapped command's code is passed
#     through verbatim otherwise.
#
# Usage:
#   scripts/gate_command.sh <command> [args...]
#   GATE_LOG_FILE=/tmp/gate.log scripts/gate_command.sh go test ./...

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: gate_command.sh <command> [args...]" >&2
  exit 2
fi

if [[ -n "${GATE_LOG_FILE:-}" ]]; then
  set +e
  "$@" >"$GATE_LOG_FILE" 2>&1
  status=$?
  set -e
  tail -n 80 "$GATE_LOG_FILE" >&2 || true
  exit "$status"
fi

set +e
"$@"
status=$?
set -e
exit "$status"
