#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib.sh
source "${SCRIPT_DIR}/lib.sh"

API_DIR="${REPO_ROOT}/services/api"
RESULT_DIR="${GATEWAY_TEST_RESULTS_ROOT:-${ARTIFACTS_ROOT}}/gateway-race"
STARTED_AT="$(timestamp_utc)"

ensure_dir "${RESULT_DIR}"
trap 'status=$?; write_meta "${RESULT_DIR}/meta.json" "gateway-race" "${STARTED_AT}" "${status}"; exit "${status}"' EXIT

log "running Gateway race checks with fake dependencies only"
# Exclude database tests even when the caller has a dedicated integration DSN.
run_and_capture "${RESULT_DIR}/go-race.log" \
  env EMBER_INTEGRATION_DATABASE_URL="" \
  go -C "${API_DIR}" test -race -skip 'Integration|PostgreSQL' -count=1 -timeout=180s \
  ./internal/playbackgateway \
  ./internal/services/directplay \
  ./internal/services/p115account \
  ./internal/services/p115quota \
  ./internal/integrations/p115

log "Gateway race artifacts written to ${RESULT_DIR}"
