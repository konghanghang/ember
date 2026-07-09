#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

BOT_DIR="${REPO_ROOT}/services/bot"
RESULT_DIR="${ARTIFACTS_ROOT}/bot"
STARTED_AT="$(timestamp_utc)"
PYTHON_BIN="$(resolve_bot_python)"
RUNNER="pytest"
WITH_COVERAGE="false"

reset_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "bot" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT

if [[ -z "${PYTHON_BIN}" ]]; then
  log "missing Bot virtualenv: services/bot/.venv"
  log "create it with: cd services/bot && python3.11 -m venv .venv"
  log "then install deps with: cd services/bot && .venv/bin/python -m pip install -r requirements-dev.txt"
  cat >"${RESULT_DIR}/preflight.log" <<EOF
missing Bot virtualenv: services/bot/.venv
create it with: cd services/bot && python3.11 -m venv .venv
then install deps with: cd services/bot && .venv/bin/python -m pip install -r requirements-dev.txt
EOF
  exit 1
fi

log "running Bot py_compile/pytest with ${PYTHON_BIN}"

(
  cd "${BOT_DIR}"

  if ! "${PYTHON_BIN}" -c "import httpx" >/dev/null 2>&1; then
    log "missing bot runtime dependency: httpx"
    log "install it with: cd services/bot && .venv/bin/python -m pip install -r requirements-dev.txt"
    cat >"${RESULT_DIR}/preflight.log" <<EOF
missing bot runtime dependency: httpx
install it with: cd services/bot && .venv/bin/python -m pip install -r requirements-dev.txt
EOF
    exit 1
  fi

  run_and_capture "${RESULT_DIR}/py-compile.log" "${PYTHON_BIN}" -m py_compile main.py

  if "${PYTHON_BIN}" -c "import coverage" >/dev/null 2>&1; then
    WITH_COVERAGE="true"
  fi

  if "${PYTHON_BIN}" -c "import pytest" >/dev/null 2>&1; then
    if [[ "${WITH_COVERAGE}" == "true" ]]; then
      run_and_capture "${RESULT_DIR}/pytest.log" \
        "${PYTHON_BIN}" -m coverage run --source=app -m pytest tests --junitxml="${RESULT_DIR}/junit.xml"
    else
      run_and_capture "${RESULT_DIR}/pytest.log" \
        "${PYTHON_BIN}" -m pytest tests --junitxml="${RESULT_DIR}/junit.xml"
    fi
  else
    RUNNER="unittest"
    log "pytest is unavailable, falling back to unittest discover -v"
    if [[ "${WITH_COVERAGE}" == "true" ]]; then
      run_and_capture "${RESULT_DIR}/unittest.log" \
        "${PYTHON_BIN}" -m coverage run --source=app -m unittest discover -s tests -v
    else
      run_and_capture "${RESULT_DIR}/unittest.log" \
        "${PYTHON_BIN}" -m unittest discover -s tests -v
    fi
  fi

  if [[ "${WITH_COVERAGE}" == "true" ]]; then
    run_and_capture "${RESULT_DIR}/coverage.log" "${PYTHON_BIN}" -m coverage report
    run_and_capture "${RESULT_DIR}/coverage-json.log" "${PYTHON_BIN}" -m coverage json -o "${RESULT_DIR}/coverage.json"
    run_and_capture "${RESULT_DIR}/coverage-xml.log" "${PYTHON_BIN}" -m coverage xml -o "${RESULT_DIR}/coverage.xml"
    "${PYTHON_BIN}" -c 'import json, sys; data = json.load(open(sys.argv[1], "r", encoding="utf-8")); print(f"{data['\''totals'\'']['\''percent_covered'\'']:.2f}")' \
      "${RESULT_DIR}/coverage.json" > "${RESULT_DIR}/coverage-summary.txt"
  else
    log "coverage is unavailable, skipping Bot coverage summary"
    printf 'N/A\n' > "${RESULT_DIR}/coverage-summary.txt"
  fi
)

cat >"${RESULT_DIR}/runner.txt" <<EOF
${RUNNER}
EOF
log "Bot artifacts written to ${RESULT_DIR}"
