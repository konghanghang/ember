#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

API_DIR="${REPO_ROOT}/services/api"
RESULT_DIR="${ARTIFACTS_ROOT}/api"
STARTED_AT="$(timestamp_utc)"

reset_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "api" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT
log "running API vet/test/build"

(
  cd "${API_DIR}"

  run_and_capture "${RESULT_DIR}/go-vet.log" go vet ./...

  if command_exists gotestsum; then
    run_and_capture "${RESULT_DIR}/go-test.log" \
      env EMBER_INTEGRATION_DATABASE_URL="" \
      gotestsum \
      --format standard-verbose \
      --junitfile "${RESULT_DIR}/junit.xml" \
      --jsonfile "${RESULT_DIR}/go-test.json" \
      ./...
  else
    run_and_capture "${RESULT_DIR}/go-test.log" \
      env EMBER_INTEGRATION_DATABASE_URL="" \
      bash -o pipefail -lc 'go test -json ./... | tee "'"${RESULT_DIR}"'/go-test.json" >/dev/null'
  fi

  run_and_capture "${RESULT_DIR}/go-build.log" go build ./...

  if [[ -n "${EMBER_INTEGRATION_DATABASE_URL:-}" ]]; then
    run_and_capture "${RESULT_DIR}/go-integration.log" \
      bash -o pipefail -lc 'go test ./internal/app -run Integration -count=1 -json | tee "'"${RESULT_DIR}"'/go-integration.json" >/dev/null'
  else
    log "EMBER_INTEGRATION_DATABASE_URL is empty, skipping API integration suite"
  fi
)

log "API artifacts written to ${RESULT_DIR}"
