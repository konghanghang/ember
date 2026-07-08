#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

WEB_DIR="${REPO_ROOT}/services/web"
RESULT_DIR="${ARTIFACTS_ROOT}/web"
STARTED_AT="$(timestamp_utc)"

reset_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "web" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT
log "running Web test/build"

(
  cd "${WEB_DIR}"

  run_and_capture "${RESULT_DIR}/vitest.log" \
    npm run test -- --reporter=json --outputFile "${RESULT_DIR}/vitest-report.json"

  run_and_capture "${RESULT_DIR}/build.log" npm run build
)

log "Web artifacts written to ${RESULT_DIR}"
