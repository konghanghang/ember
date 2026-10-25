#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

RESULT_DIR="${GATEWAY_TEST_RESULTS_ROOT:-${ARTIFACTS_ROOT}}/gateway-capture"
STARTED_AT="$(timestamp_utc)"
# Respect the selected CI matrix interpreter; an invalid override must fail.
PYTHON_BIN="${PYTHON:-python3}"

ensure_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "gateway-capture" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT
: > "${RESULT_DIR}/unittest.log"
: > "${RESULT_DIR}/runner-tests.log"

log "running offline Gateway capture tests with ${PYTHON_BIN}"
run_and_capture "${RESULT_DIR}/unittest.log" \
  "${PYTHON_BIN}" -B -m unittest discover -s "${REPO_ROOT}/scripts/gateway-capture/tests" -v
run_and_capture "${RESULT_DIR}/runner-tests.log" \
  "${PYTHON_BIN}" -B -m unittest discover -s "${SCRIPT_DIR}/tests" -v

log "Gateway capture artifacts written to ${RESULT_DIR}"
