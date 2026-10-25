#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

API_DIR="${REPO_ROOT}/services/api"
RESULT_DIR="${GATEWAY_TEST_RESULTS_ROOT:-${ARTIFACTS_ROOT}}/gateway-contract"
STARTED_AT="$(timestamp_utc)"
PYTHON_BIN="${PYTHON:-python3}"
export PYTHONDONTWRITEBYTECODE=1

ensure_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "gateway-contract" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT
: > "${RESULT_DIR}/go-test.jsonl"
: > "${RESULT_DIR}/go-stderr.log"
: > "${RESULT_DIR}/validator.log"

log "producing Gateway diagnostics with fake dependencies only"
# Keep toolchain/module download notices out of the JSON event stream.
env EMBER_INTEGRATION_DATABASE_URL="" \
  go -C "${API_DIR}" test -json \
  -run '^(TestGatewayDiagnosticContractTimingProducer|TestGatewayDiagnosticContractLogProducer)$' \
  -count=1 -timeout=120s ./internal/services/directplay ./internal/playbackgateway \
  2> "${RESULT_DIR}/go-stderr.log" | tee "${RESULT_DIR}/go-test.jsonl"

log "checking Go diagnostics with the real Python filter and summary"
run_and_capture "${RESULT_DIR}/validator.log" \
  "${PYTHON_BIN}" -B "${SCRIPT_DIR}/gateway_contract.py" "${RESULT_DIR}/go-test.jsonl"

log "Gateway contract artifacts written to ${RESULT_DIR}"
