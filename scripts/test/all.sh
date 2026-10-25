#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bash "${SCRIPT_DIR}/api.sh"
bash "${SCRIPT_DIR}/web.sh"
bash "${SCRIPT_DIR}/bot.sh"
bash "${SCRIPT_DIR}/gateway-capture.sh"
bash "${SCRIPT_DIR}/gateway-race.sh"
bash "${SCRIPT_DIR}/gateway-contract.sh"
