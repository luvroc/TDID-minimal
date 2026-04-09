#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/home/ecs-user}"
TDID_DIR="${ROOT_DIR:-${ROOT_DIR}/TDID}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${ROOT_DIR}/chain-DOT/test-network}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${ROOT_DIR}/fisco/console}"

CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
FABRIC_GATEWAY_CC="${FABRIC_GATEWAY_CC:-gatewaycc}"

TRACE_ID=""
GATEWAY_ADDR=""
KEY_ID=""
NONCE=""
EXPIRE_AT=""
SESS_SIG=""
ATTESTER="temp-attester"
SIGNER="temp-signer"
DRY_RUN="false"
SKIP_ALLOWLIST="false"

usage() {
  cat <<'EOF'
Usage:
  bridge-proof-to-fisco.sh \
    --trace-id <fabric_trace_id> \
    --gateway-addr <fisco_gateway_addr> \
    --key-id <bytes32_hex> \
    --nonce <uint> \
    --expire-at <unix_ts> \
    --sig <hex_sig> \
    [--attester <text_or_hex>] \
    [--signer <text_or_hex>] \
    [--skip-allowlist] \
    [--dry-run]

What it does:
1. Invoke Fabric gateway BuildSourceLockProof(traceId, attester, signer)
2. Query GetTrace + GetSourceLockProof from Fabric
3. Encode SourceLockProof into ABI bytes payload (temporary static tuple encoding)
4. Optionally initialize proof allowlist on FISCO TargetGateway
5. Call FISCO TargetGateway.mintOrUnlockWithProof(payload, keyId, nonce, expireAt, sig)
EOF
}

log() {
  echo "[$(date +'%F %T')] $*"
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
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

while [[ $# -gt 0 ]]; do
  case "$1" in
    --trace-id) TRACE_ID="$2"; shift 2 ;;
    --gateway-addr) GATEWAY_ADDR="$2"; shift 2 ;;
    --key-id) KEY_ID="$2"; shift 2 ;;
    --nonce) NONCE="$2"; shift 2 ;;
    --expire-at) EXPIRE_AT="$2"; shift 2 ;;
    --sig) SESS_SIG="$2"; shift 2 ;;
    --attester) ATTESTER="$2"; shift 2 ;;
    --signer) SIGNER="$2"; shift 2 ;;
    --skip-allowlist) SKIP_ALLOWLIST="true"; shift ;;
    --dry-run) DRY_RUN="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown arg: $1" ;;
  esac
done

[[ -n "${TRACE_ID}" ]] || fail "--trace-id is required"
[[ -n "${GATEWAY_ADDR}" ]] || fail "--gateway-addr is required"
[[ -n "${KEY_ID}" ]] || fail "--key-id is required"
[[ -n "${NONCE}" ]] || fail "--nonce is required"
[[ -n "${EXPIRE_AT}" ]] || fail "--expire-at is required"
[[ -n "${SESS_SIG}" ]] || fail "--sig is required"

if [[ "${EXPIRE_AT}" =~ ^[0-9]+$ ]] && (( EXPIRE_AT < 1000000000000 )); then
  EXPIRE_AT=$((EXPIRE_AT * 1000))
  log "normalized --expire-at to milliseconds: ${EXPIRE_AT}"
fi

fabric_invoke() {
  local cc_name="$1"
  local ctor_json="$2"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${FABRIC_NET_DIR}/../config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode invoke \
      -o localhost:7050 \
      --ordererTLSHostnameOverride orderer.example.com \
      --tls --cafile "$ORDERER_CA" \
      -C "${CHANNEL_NAME}" -n "${cc_name}" \
      --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
      --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
      --waitForEvent --waitForEventTimeout 30s \
      -c "${ctor_json}"
  )
}

fabric_query() {
  local cc_name="$1"
  local ctor_json="$2"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${FABRIC_NET_DIR}/../config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode query -C "${CHANNEL_NAME}" -n "${cc_name}" -c "${ctor_json}"
  )
}

run_fisco_console() {
  (
    cd "${FISCO_CONSOLE_DIR}"
    bash console.sh "$@"
  )
}

log "[1/5] Query trace from Fabric: ${TRACE_ID}"
TRACE_RAW=$(fabric_query "${FABRIC_GATEWAY_CC}" "{\"function\":\"GetTrace\",\"Args\":[\"${TRACE_ID}\"]}" 2>&1)

TRACE_TRANSFER_ID=$(python3 - <<'PY' "${TRACE_RAW}"
import json, sys
raw = sys.argv[1].strip()

def extract_json_blob(text: str) -> str:
  start = text.find("{")
  end = text.rfind("}")
  if start != -1 and end != -1 and end > start:
    return text[start:end+1]
  return text

blob = extract_json_blob(raw)
try:
  obj = json.loads(blob)
except Exception:
  obj = json.loads(json.loads(blob))
print(obj.get("transferId", ""))
PY
)

[[ -n "${TRACE_TRANSFER_ID}" ]] || fail "failed to parse transferId from GetTrace output: ${TRACE_RAW}"
log "transferId=${TRACE_TRANSFER_ID}"

log "[2/5] Build source lock proof on Fabric"
BUILD_OUT=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"BuildSourceLockProof\",\"Args\":[\"${TRACE_ID}\",\"${ATTESTER}\",\"${SIGNER}\"]}" 2>&1 || true)
if [[ "${BUILD_OUT}" != *"status:200"* ]]; then
  fail "BuildSourceLockProof failed: ${BUILD_OUT}"
fi

PROOF_RAW=$(fabric_query "${FABRIC_GATEWAY_CC}" "{\"function\":\"GetSourceLockProof\",\"Args\":[\"${TRACE_TRANSFER_ID}\"]}" 2>&1)

log "[3/5] Encode proof payload"
PAYLOAD_HEX=$(python3 "${TDID_DIR}/fisco/scripts/encode_source_lock_proof.py" "${PROOF_RAW}")
ATTESTER_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("attester",""))
PY
)
SIGNER_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("signer",""))
PY
)
PROOF_SCHEMA_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("proofSchemaVersion",""))
PY
)
PROOF_LEVEL_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("proofSemanticLevel",""))
PY
)
PROOF_SOURCE_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("proofSourceMode",""))
PY
)
PROOF_BLOCK_REF_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("proofBlockHeightRef",""))
PY
)
PROOF_EVENT_REF_FROM_PROOF=$(python3 - <<'PY' "${PROOF_RAW}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find("{"); end=raw.rfind("}")
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
  obj=json.loads(obj)
print(obj.get("proofEventHashRef",""))
PY
)
ATTESTER_EFFECTIVE="${ATTESTER_FROM_PROOF:-${ATTESTER}}"
SIGNER_EFFECTIVE="${SIGNER_FROM_PROOF:-${SIGNER}}"
ATTESTER_B32="$(to_bytes32 "${ATTESTER_EFFECTIVE}")"
SIGNER_B32="$(to_bytes32 "${SIGNER_EFFECTIVE}")"

log "proofPayload bytes length=$(( (${#PAYLOAD_HEX} - 2) / 2 ))"
log "attester(bytes32)=${ATTESTER_B32} (raw=${ATTESTER_EFFECTIVE})"
log "signer(bytes32)=${SIGNER_B32} (raw=${SIGNER_EFFECTIVE})"
log "proofMeta schema=${PROOF_SCHEMA_FROM_PROOF:-n/a} level=${PROOF_LEVEL_FROM_PROOF:-n/a} source=${PROOF_SOURCE_FROM_PROOF:-n/a}"
log "proofMeta blockRef=${PROOF_BLOCK_REF_FROM_PROOF:-n/a} eventRef=${PROOF_EVENT_REF_FROM_PROOF:-n/a}"

if [[ "${SKIP_ALLOWLIST}" != "true" ]]; then
  log "[4/5] Ensure proof allowlist on FISCO gateway"
  run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofAttester "${ATTESTER_B32}" true >/dev/null
  run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofSigner "${SIGNER_B32}" true >/dev/null
  if [[ "${SIGNER_EFFECTIVE}" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
    run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" setProofSignerAddress "${SIGNER_EFFECTIVE}" true >/dev/null
  fi
fi

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "proofPayload=${PAYLOAD_HEX}"
  echo "next call:"
  if [[ "${SKIP_ALLOWLIST}" != "true" ]]; then
    echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofAttester ${ATTESTER_B32} true"
    echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofSigner ${SIGNER_B32} true"
    if [[ "${SIGNER_EFFECTIVE}" =~ ^0x[0-9a-fA-F]{40}$ ]]; then
      echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofSignerAddress ${SIGNER_EFFECTIVE} true"
    fi
  fi
  echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} mintOrUnlockWithProof ${PAYLOAD_HEX} ${KEY_ID} ${NONCE} ${EXPIRE_AT} ${SESS_SIG}"
  exit 0
fi

log "[5/5] Call FISCO mintOrUnlockWithProof"
OUT=$(run_fisco_console call FiscoGateway "${GATEWAY_ADDR}" mintOrUnlockWithProof "${PAYLOAD_HEX}" "${KEY_ID}" "${NONCE}" "${EXPIRE_AT}" "${SESS_SIG}")
echo "${OUT}"
if [[ "${OUT}" != *"transaction status: 0"* ]]; then
  fail "mintOrUnlockWithProof failed"
fi

log "bridge proof call succeeded"
