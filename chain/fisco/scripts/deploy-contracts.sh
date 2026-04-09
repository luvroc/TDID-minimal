#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FISCO_DIR="${FISCO_DIR:-${ROOT_DIR}/fisco}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${FISCO_DIR}/console}"
FISCO_NODES_DIR="${FISCO_NODES_DIR:-${FISCO_DIR}/nodes/127.0.0.1}"
OUT_FILE="${OUT_FILE:-${TDID_DIR}/fisco/config/deployed_contracts.generated.json}"
CHAIN_ID="${CHAIN_ID:-fisco}"
PROOF_ATTESTER="${PROOF_ATTESTER:-temp-attester}"
PROOF_SIGNER="${PROOF_SIGNER:-temp-signer}"

log() {
  echo "[$(date +'%F %T')] $*"
}

run_fisco_console() {
  (
    cd "${FISCO_CONSOLE_DIR}"
    bash console.sh "$@"
  )
}

extract_addr() {
  echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1
}

to_bytes32() {
  local value="$1"
  python3 - <<'PY' "$value"
import re
import sys
from eth_hash.auto import keccak

v = str(sys.argv[1]).strip()
if v.startswith(("0x", "0X")) and re.fullmatch(r"[0-9a-fA-F]+", v[2:]):
    h = v[2:].lower()
    if len(h) > 64:
        h = h[-64:]
    print("0x" + h.rjust(64, "0"))
else:
    print("0x" + keccak(v.encode()).hex())
PY
}

copy_contracts() {
  cp "${TDID_DIR}/fisco/contracts/GovernanceRoot.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/GovernanceRoot.sol"
  cp "${TDID_DIR}/fisco/contracts/TDIDRegistry.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/TDIDRegistry.sol"
  cp "${TDID_DIR}/fisco/contracts/SessionKeyRegistry.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/SessionKeyRegistry.sol"
  cp "${TDID_DIR}/fisco/contracts/SigVerifier.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/SigVerifier.sol"
  cp "${TDID_DIR}/fisco/contracts/TargetGateway.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"
}

start_nodes() {
  (
    cd "${FISCO_NODES_DIR}"
    bash start_all.sh || true
  )
}

main() {
  log "Start FISCO nodes"
  start_nodes

  log "Sync solidity contracts"
  copy_contracts

  local current admin_addr
  current="$(run_fisco_console getCurrentAccount)"
  admin_addr="$(echo "${current}" | sed -n 's/.*0x\([0-9a-fA-F]\{40\}\).*/0x\1/p' | tail -n1)"
  [[ -n "${admin_addr}" ]] || { echo "Unable to determine FISCO admin account from console output"; echo "${current}"; exit 1; }

  log "Deploy GovernanceRoot"
  local out_gov gov_addr
  out_gov="$(run_fisco_console deploy GovernanceRoot "${admin_addr}")"
  gov_addr="$(extract_addr "${out_gov}")"
  [[ -n "${gov_addr}" ]] || { echo "Deploy GovernanceRoot failed"; echo "${out_gov}"; exit 1; }

  log "Deploy TDIDRegistry"
  local out_tdid tdid_addr
  out_tdid="$(run_fisco_console deploy TDIDRegistry "${gov_addr}")"
  tdid_addr="$(extract_addr "${out_tdid}")"
  [[ -n "${tdid_addr}" ]] || { echo "Deploy TDIDRegistry failed"; echo "${out_tdid}"; exit 1; }

  log "Deploy SessionKeyRegistry"
  local out_sess sess_addr
  out_sess="$(run_fisco_console deploy SessionKeyRegistry "${tdid_addr}")"
  sess_addr="$(extract_addr "${out_sess}")"
  [[ -n "${sess_addr}" ]] || { echo "Deploy SessionKeyRegistry failed"; echo "${out_sess}"; exit 1; }

  log "Deploy SigVerifier"
  local out_sig sig_addr
  out_sig="$(run_fisco_console deploy SigVerifier "${sess_addr}")"
  sig_addr="$(extract_addr "${out_sig}")"
  [[ -n "${sig_addr}" ]] || { echo "Deploy SigVerifier failed"; echo "${out_sig}"; exit 1; }

  log "Deploy TargetGateway(FiscoGateway)"
  local out_gateway gateway_addr
  out_gateway="$(run_fisco_console deploy FiscoGateway "${sig_addr}" "${CHAIN_ID}")"
  gateway_addr="$(extract_addr "${out_gateway}")"
  [[ -n "${gateway_addr}" ]] || { echo "Deploy FiscoGateway failed"; echo "${out_gateway}"; exit 1; }

  local proof_attester_b32 proof_signer_b32
  proof_attester_b32="$(to_bytes32 "${PROOF_ATTESTER}")"
  proof_signer_b32="$(to_bytes32 "${PROOF_SIGNER}")"
  log "Init proof allowlist: attester=${PROOF_ATTESTER} (${proof_attester_b32}) signer=${PROOF_SIGNER} (${proof_signer_b32})"
  run_fisco_console call FiscoGateway "${gateway_addr}" setProofAttester "${proof_attester_b32}" true >/dev/null
  run_fisco_console call FiscoGateway "${gateway_addr}" setProofSigner "${proof_signer_b32}" true >/dev/null
  if [[ "${PROOF_SIGNER}" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
    run_fisco_console call FiscoGateway "${gateway_addr}" setProofSignerAddress "${PROOF_SIGNER}" true >/dev/null
  fi

  mkdir -p "$(dirname "${OUT_FILE}")"
  cat > "${OUT_FILE}" <<EOF
{
  "network": {
    "chainId": "${CHAIN_ID}",
    "groupId": "group0"
  },
  "contracts": {
    "GovernanceRoot": "${gov_addr}",
    "TDIDRegistry": "${tdid_addr}",
    "SessionKeyRegistry": "${sess_addr}",
    "SigVerifier": "${sig_addr}",
    "TargetGateway": "${gateway_addr}"
  },
  "abi": {
    "GovernanceRoot": "${FISCO_CONSOLE_DIR}/contracts/.compiled/GovernanceRoot.abi",
    "TDIDRegistry": "${FISCO_CONSOLE_DIR}/contracts/.compiled/TDIDRegistry.abi",
    "SessionKeyRegistry": "${FISCO_CONSOLE_DIR}/contracts/.compiled/SessionKeyRegistry.abi",
    "SigVerifier": "${FISCO_CONSOLE_DIR}/contracts/.compiled/SigVerifier.abi",
    "TargetGateway": "${FISCO_CONSOLE_DIR}/contracts/.compiled/FiscoGateway.abi"
  }
}
EOF

  log "Deploy done, output: ${OUT_FILE}"
}

main "$@"
