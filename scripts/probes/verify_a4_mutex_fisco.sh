#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${ROOT_DIR}/fisco/console}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${CHAIN_DOT_HOME}/test-network}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
FABRIC_GATEWAY_CC="${FABRIC_GATEWAY_CC:-gatewaycc}"

run_console() { (cd "${FISCO_CONSOLE_DIR}" && bash console.sh "$@"); }
extract_addr(){ echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1; }
mk_b32() {
  python3 - <<'PY' "$1"
from eth_hash.auto import keccak
import re,sys
v=sys.argv[1].strip()
if v.startswith(("0x","0X")) and re.fullmatch(r"[0-9a-fA-F]+",v[2:]):
  h=v[2:].lower()
  print("0x"+h[-64:].rjust(64,"0"))
else:
  print("0x"+keccak(v.encode()).hex())
PY
}

must_contain() {
  local output="$1" expected="$2" hint="$3"
  [[ "${output}" == *"${expected}"* ]] || {
    echo "ASSERT FAIL: ${hint}"
    echo "Expected: ${expected}"
    echo "Actual:"
    echo "${output}"
    exit 1
  }
}

must_not_contain() {
  local output="$1" bad="$2" hint="$3"
  [[ "${output}" != *"${bad}"* ]] || {
    echo "ASSERT FAIL: ${hint}"
    echo "Unexpected: ${bad}"
    echo "Actual:"
    echo "${output}"
    exit 1
  }
}

fabric_invoke() {
  local ctor_json="$1"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:/usr/local/go/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${ROOT_DIR}/chain-DOT/config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode invoke \
      -o localhost:7050 \
      --ordererTLSHostnameOverride orderer.example.com \
      --tls --cafile "$ORDERER_CA" \
      -C "${CHANNEL_NAME}" -n "${FABRIC_GATEWAY_CC}" \
      --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
      --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
      --waitForEvent --waitForEventTimeout 30s \
      -c "${ctor_json}"
  )
}

fabric_query() {
  local ctor_json="$1"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:/usr/local/go/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${ROOT_DIR}/chain-DOT/config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode query -C "${CHANNEL_NAME}" -n "${FABRIC_GATEWAY_CC}" -c "${ctor_json}"
  )
}

assert_proof_meta() {
  local transfer_id="$1"
  local proof_raw
  proof_raw=$(fabric_query "{\"function\":\"GetSourceLockProof\",\"Args\":[\"${transfer_id}\"]}" 2>&1)
  python3 - <<'PY' "${proof_raw}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find('{'); end=raw.rfind('}')
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
    obj=json.loads(obj)
required={
  "proofSchemaVersion":"fabric-source-lock-proof/v1",
  "proofSemanticLevel":"attested_lock_snapshot",
  "proofSourceMode":"fabric_chaincode_context",
  "proofBlockHeightRef":"tx_timestamp_millis_placeholder",
  "proofEventHashRef":"deterministic_lock_payload_hash",
}
for k,v in required.items():
    got=obj.get(k,"")
    if got!=v:
        raise SystemExit(f"proof meta mismatch: {k} expected={v} got={got}")
print("proof meta check: PASS")
PY
}

run_bridge() {
  local trace_id="$1"
  local nonce="$2"
  local expire_s="$3"
  bash "${TDID_DIR}/fisco/scripts/bridge-proof-to-fisco.sh" \
    --trace-id "${trace_id}" \
    --gateway-addr "${GW}" \
    --key-id "${KEY}" \
    --nonce "${nonce}" \
    --expire-at "${expire_s}" \
    --sig "${SIG}" \
    --attester "temp-attester" \
    --signer "temp-signer"
}

lock_on_fabric() {
  local transfer="$1" session="$2" trace="$3" nonce="$4" exp_s="$5"
  fabric_invoke "{\"function\":\"LockV2\",\"Args\":[\"${transfer}\",\"${session}\",\"${trace}\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"${KEY}\",\"${nonce}\",\"${exp_s}\",\"${SIG}\"]}" >/tmp/a4_fisco_fabric_lock.out
}

# Deploy FISCO gateway with current TargetGateway.
cp "${TDID_DIR}/fisco/contracts/TargetGateway.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"
out_m=$(run_console deploy MockSigVerifier)
mock=$(extract_addr "${out_m}")
out_g=$(run_console deploy FiscoGateway "${mock}" fisco)
GW=$(extract_addr "${out_g}")
echo "gateway=${GW}"

RID=$(date +%s%N)
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"

# 1) mint_then_refund_should_fail
trace1="trace-a4-fisco-1-${RID}"
transfer1="transfer-a4-fisco-1-${RID}"
session1="session-a4-fisco-1-${RID}"
trace1_b32=$(mk_b32 "${trace1}")
transfer1_b32=$(mk_b32 "${transfer1}")
session1_b32=$(mk_b32 "${session1}")
nonce_fabric_1=$((RID % 900000000 + 6101))
exp_fabric_1=$(( $(date +%s) + 3600 ))

lock_on_fabric "${transfer1}" "${session1}" "${trace1}" "${nonce_fabric_1}" "${exp_fabric_1}"

echo "==== mint_then_refund_should_fail ===="
mint1=$(run_bridge "${trace1}" 7001 $(( $(date +%s) + 3600 )) 2>&1 || true)
echo "${mint1}"
must_contain "${mint1}" "transaction status: 0" "mint with proof should succeed"
assert_proof_meta "${transfer1}"

refund_after_mint=$(run_console call FiscoGateway "${GW}" refundV2 "${trace1_b32}" "${KEY}" 2>&1 || true)
echo "${refund_after_mint}"
must_not_contain "${refund_after_mint}" "transaction status: 0" "refund must fail after mint"

# 2) refund_then_mint_should_fail
trace2="trace-a4-fisco-2-${RID}"
transfer2="transfer-a4-fisco-2-${RID}"
session2="session-a4-fisco-2-${RID}"
trace2_b32=$(mk_b32 "${trace2}")
transfer2_b32=$(mk_b32 "${transfer2}")
session2_b32=$(mk_b32 "${session2}")
nonce_fabric_2=$((RID % 900000000 + 6102))
exp_fabric_2=$(( $(date +%s) + 3600 ))
lock_on_fabric "${transfer2}" "${session2}" "${trace2}" "${nonce_fabric_2}" "${exp_fabric_2}"

echo "==== refund_then_mint_should_fail ===="
now_ms=$(date +%s%3N)
lock2=$(run_console call FiscoGateway "${GW}" lockV2 "${transfer2_b32}" "${session2_b32}" "${trace2_b32}" "${KEY}" 7102 $((now_ms + 15000)) "${SIG}" 2>&1 || true)
echo "${lock2}"
must_contain "${lock2}" "transaction status: 0" "fisco lock should succeed"
sleep 16
refund2=$(run_console call FiscoGateway "${GW}" refundV2 "${trace2_b32}" "${KEY}" 2>&1 || true)
echo "${refund2}"
must_contain "${refund2}" "transaction status: 0" "refund should succeed after expiry"

mint_after_refund=$(run_bridge "${trace2}" 7002 $(( $(date +%s) + 3600 )) 2>&1 || true)
echo "${mint_after_refund}"
must_not_contain "${mint_after_refund}" "transaction status: 0" "mint must fail after refund"
assert_proof_meta "${transfer2}"

# 3) stale/tampered proof should fail (signature mismatch path)
echo "==== stale_locked_proof_after_refund_should_fail ===="
build3=$(fabric_invoke "{\"function\":\"BuildSourceLockProof\",\"Args\":[\"${trace2}\",\"temp-attester\",\"temp-signer\"]}" 2>&1 || true)
must_contain "${build3}" "status:200" "BuildSourceLockProof should succeed"
proof_raw=$(fabric_query "{\"function\":\"GetSourceLockProof\",\"Args\":[\"${transfer2}\"]}" 2>&1)
tampered=$(python3 - <<'PY' "${proof_raw}"
import json,sys
raw=sys.argv[1].strip()
start=raw.find('{'); end=raw.rfind('}')
blob=raw[start:end+1] if start!=-1 and end!=-1 and end>start else raw
obj=json.loads(blob)
if isinstance(obj,str):
    obj=json.loads(obj)
obj['proofSig']=(obj.get('proofSig') or '0x')[:-1] + ('0' if (obj.get('proofSig') or '0x')[-1:]!='0' else '1')
print(json.dumps(obj,separators=(',',':')))
PY
)
payload=$(python3 "${TDID_DIR}/fisco/scripts/encode_source_lock_proof.py" "${tampered}")
neg3=$(run_console call FiscoGateway "${GW}" mintOrUnlockWithProof "${payload}" "${KEY}" 7003 $(( $(date +%s%3N) + 600000 )) "${SIG}" 2>&1 || true)
echo "${neg3}"
must_not_contain "${neg3}" "transaction status: 0" "tampered/stale proof must fail"

echo "A4_FISCO_MUTEX: PASS"
