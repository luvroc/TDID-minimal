#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "${SCRIPT_DIR}/common.sh"

CHAIN_HOST="${CHAIN_HOST:-${1:-chain}}"
TEE_HOST="${TEE_HOST:-${2:-tee}}"
CHAIN_REMOTE_ROOT="${CHAIN_REMOTE_ROOT:-${3:-/opt/tdid}}"
TEE_REMOTE_ROOT="${TEE_REMOTE_ROOT:-${4:-/opt/tdid}}"
RUN_LEGACY_MUTEX_SUITES="${RUN_LEGACY_MUTEX_SUITES:-1}"

CHAIN_SYNC_FILES=(
  "scripts/closeout/run_chain_closeout_suite.sh"
  "scripts/closeout/run_gateway_regression_suite.sh"
  "scripts/probes/probe_fabric_withproof_availability.sh"
  "scripts/probes/probe_fabric_withproof_call_path.sh"
  "scripts/probes/probe_fabric_withproof_dual_gateway_flow.sh"
  "scripts/probes/verify_fisco_mutex_guards.sh"
  "scripts/probes/verify_fabric_mutex_guards.sh"
)

TEE_SYNC_FILES=(
  "scripts/closeout/run_tee_closeout_suite.sh"
)

baseline_require_commands "${SSH_BIN}" "${SCP_BIN}"

baseline_log "mutex matrix: sync chain-side scripts"
baseline_sync_files "${CHAIN_HOST}" "${CHAIN_REMOTE_ROOT}" "${CHAIN_SYNC_FILES[@]}"
baseline_normalize_remote_files "${CHAIN_HOST}" \
  "${CHAIN_REMOTE_ROOT}/run_chain_closeout_suite.sh" \
  "${CHAIN_REMOTE_ROOT}/run_gateway_regression_suite.sh" \
  "${CHAIN_REMOTE_ROOT}/probe_fabric_withproof_availability.sh" \
  "${CHAIN_REMOTE_ROOT}/probe_fabric_withproof_call_path.sh" \
  "${CHAIN_REMOTE_ROOT}/probe_fabric_withproof_dual_gateway_flow.sh" \
  "${CHAIN_REMOTE_ROOT}/verify_fisco_mutex_guards.sh" \
  "${CHAIN_REMOTE_ROOT}/verify_fabric_mutex_guards.sh"

baseline_log "mutex matrix: sync tee-side scripts"
baseline_sync_files "${TEE_HOST}" "${TEE_REMOTE_ROOT}" "${TEE_SYNC_FILES[@]}"
baseline_normalize_remote_files "${TEE_HOST}" \
  "${TEE_REMOTE_ROOT}/run_tee_closeout_suite.sh"

baseline_log "mutex matrix: chain closeout suite"
baseline_log "mutex matrix: note -> proof-service is treated as dev-only and not required by default"
"${SSH_BIN}" "${CHAIN_HOST}" "bash ${CHAIN_REMOTE_ROOT}/run_chain_closeout_suite.sh"

if [[ "${RUN_LEGACY_MUTEX_SUITES}" == "1" ]]; then
  baseline_log "mutex matrix: legacy FISCO mutex negative suite"
  "${SSH_BIN}" "${CHAIN_HOST}" "bash ${CHAIN_REMOTE_ROOT}/verify_fisco_mutex_guards.sh"

  baseline_log "mutex matrix: legacy Fabric mutex negative suite"
  "${SSH_BIN}" "${CHAIN_HOST}" "bash ${CHAIN_REMOTE_ROOT}/verify_fabric_mutex_guards.sh"
fi

baseline_log "mutex matrix: tee closeout suite"
"${SSH_BIN}" "${TEE_HOST}" "bash ${TEE_REMOTE_ROOT}/run_tee_closeout_suite.sh"

baseline_log "mutex matrix closeout: PASS"
