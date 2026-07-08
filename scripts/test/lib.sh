#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ARTIFACTS_ROOT="${REPO_ROOT}/artifacts/test-results"

log() {
  printf '[test] %s\n' "$*"
}

ensure_dir() {
  mkdir -p "$1"
}

reset_dir() {
  rm -rf "$1"
  mkdir -p "$1"
}

timestamp_utc() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

write_meta() {
  local target="$1"
  local suite="$2"
  local started_at="$3"
  local exit_code="${4:-0}"
  local status="passed"
  if [[ "${exit_code}" != "0" ]]; then
    status="failed"
  fi
  cat >"${target}" <<EOF
{
  "suite": "${suite}",
  "startedAt": "${started_at}",
  "finishedAt": "$(timestamp_utc)",
  "exitCode": ${exit_code},
  "status": "${status}"
}
EOF
}

run_and_capture() {
  local log_file="$1"
  shift

  ensure_dir "$(dirname "${log_file}")"
  (
    set -o pipefail
    "$@" 2>&1 | tee "${log_file}"
  )
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

resolve_python() {
  if [[ -n "${PYTHON:-}" ]] && command_exists "${PYTHON}"; then
    printf '%s\n' "${PYTHON}"
    return
  fi
  if command_exists python3.11; then
    printf '%s\n' "python3.11"
    return
  fi
  if command_exists python3; then
    printf '%s\n' "python3"
    return
  fi
  if command_exists python; then
    printf '%s\n' "python"
    return
  fi
  printf 'python3\n'
}

resolve_bot_python() {
  local bot_python="${REPO_ROOT}/services/bot/.venv/bin/python"
  if [[ -x "${bot_python}" ]]; then
    printf '%s\n' "${bot_python}"
    return
  fi
  printf '%s\n' ""
}
