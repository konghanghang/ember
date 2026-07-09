#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

WEB_DIR="${REPO_ROOT}/services/web"
RESULT_DIR="${ARTIFACTS_ROOT}/web"
STARTED_AT="$(timestamp_utc)"
WEB_INTEGRATION_SPEC="src/integration/media-library-policy.flow.spec.ts"

reset_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "web" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT
log "running Web test/build"

(
  cd "${WEB_DIR}"

  run_and_capture "${RESULT_DIR}/vitest.log" \
    npm run test -- --reporter=json --outputFile "${RESULT_DIR}/vitest-report.json"

  run_and_capture "${RESULT_DIR}/build.log" npm run build

  if [[ -n "${EMBER_INTEGRATION_DATABASE_URL:-}" ]]; then
    log "running Web integration flow against real API + database"
    run_and_capture "${RESULT_DIR}/integration-vitest.log" \
      env EMBER_WEB_RUN_INTEGRATION=1 \
      npm run test -- "${WEB_INTEGRATION_SPEC}" --reporter=json --outputFile "${RESULT_DIR}/integration-vitest-report.json"
  else
    log "EMBER_INTEGRATION_DATABASE_URL is empty, skipping Web integration flow"
  fi
)

log "Web artifacts written to ${RESULT_DIR}"
