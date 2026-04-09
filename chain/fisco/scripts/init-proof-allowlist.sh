#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/home/ecs-user}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${ROOT_DIR}/fisco/console}"

GATEWAY_ADDR="${1:-}"
ATTESTER_RAW="${2:-temp-attester}"
SIGNER_RAW="${3:-temp-signer}"

usage() {
  cat <<'EOF'
Usage:
  init-proof-allowlist.sh <gateway_addr> [attester_text_or_hex] [signer_text_or_hex]
EOF
}

if [[ -z "${GATEWAY_ADDR}" ]]; then
  usage
  exit 1
fi

run_fisco_console() {
  (
    cd "${FISCO_CONSOLE_DIR}"
    bash console.sh "$@"
  )
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

ATTESTER_B32="$(to_bytes32 "${ATTESTER_RAW}")"
SIGNER_B32="$(to_bytes32 "${SIGNER_RAW}")"

echo "gateway=${GATEWAY_ADDR}"
echo "attester=${ATTESTER_RAW} -> ${ATTESTER_B32}"
echo "signer=${SIGNER_RAW} -> ${SIGNER_B32}"

run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofAttester "${ATTESTER_B32}" true
run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofSigner "${SIGNER_B32}" true
if [[ "${SIGNER_RAW}" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
  run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofSignerAddress "${SIGNER_RAW}" true
fi

echo "proof allowlist initialized"
