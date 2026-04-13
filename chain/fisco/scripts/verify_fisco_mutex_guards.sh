#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${ROOT_DIR}/fisco/console}"

cd "${FISCO_CONSOLE_DIR}"

run() { bash console.sh "$@"; }
addr() { echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1; }

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

payload_hex() {
  local trace="$1" transfer="$2" session="$3" pts="$4" att="$5" sgn="$6"
  local js
  js=$(cat <<JSON
{"traceId":"$trace","transferId":"$transfer","sessionId":"$session","srcChainId":"fabric","lockState":"LOCKED","blockHeight":1,"txHash":"$(mk_b32 tx-$trace)","eventHash":"$(mk_b32 ev-$trace)","proofTimestamp":$pts,"attester":"$att","signer":"$sgn"}
JSON
)
  python3 "${TDID_DIR}/fisco/scripts/encode_source_lock_proof.py" "$js"
}

must_contain() {
  local out="$1" expected="$2" hint="$3"
  [[ "$out" == *"$expected"* ]] || { echo "ASSERT FAIL: $hint"; echo "$out"; exit 1; }
}
must_not_contain() {
  local out="$1" bad="$2" hint="$3"
  [[ "$out" != *"$bad"* ]] || { echo "ASSERT FAIL: $hint"; echo "$out"; exit 1; }
}

cp "${TDID_DIR}/fisco/contracts/TargetGateway.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"
mock_out=$(run deploy MockSigVerifier); mock=$(addr "$mock_out")
gw_out=$(run deploy FiscoGateway "$mock" fisco); gw=$(addr "$gw_out")
echo "gateway=$gw"

key="0x1111111111111111111111111111111111111111111111111111111111111111"
sig="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
attester_txt="temp-attester"
signer_txt="temp-signer"
attester_b32=$(mk_b32 "$attester_txt")
signer_b32=$(mk_b32 "$signer_txt")
run call FiscoGateway "$gw" setProofAttester "$attester_b32" true >/dev/null
run call FiscoGateway "$gw" setProofSigner "$signer_b32" true >/dev/null

rid=$(date +%s)
now_ms=$(date +%s%3N)

echo "==== mint_then_refund_should_fail ===="
trace1=$(mk_b32 "fisco-mutex-trace-1-$rid")
transfer1=$(mk_b32 "fisco-mutex-transfer-1-$rid")
session1=$(mk_b32 "fisco-mutex-session-1-$rid")
payload1=$(payload_hex "$trace1" "$transfer1" "$session1" "$now_ms" "$attester_txt" "$signer_txt")
mint1=$(run call FiscoGateway "$gw" mintOrUnlockWithProof "$payload1" "$key" 7001 $((now_ms+600000)) "$sig" 2>&1 || true)
echo "$mint1"
must_contain "$mint1" "transaction status: 0" "mint should succeed"
refund_after_mint=$(run call FiscoGateway "$gw" refundV2 "$trace1" "$key" 2>&1 || true)
echo "$refund_after_mint"
must_not_contain "$refund_after_mint" "transaction status: 0" "refund must fail after mint"

echo "==== refund_then_mint_should_fail ===="
trace2=$(mk_b32 "fisco-mutex-trace-2-$rid")
transfer2=$(mk_b32 "fisco-mutex-transfer-2-$rid")
session2=$(mk_b32 "fisco-mutex-session-2-$rid")
now2_ms=$(date +%s%3N)
lock2=$(run call FiscoGateway "$gw" lockV2 "$transfer2" "$session2" "$trace2" "$key" 7002 $((now2_ms+10000)) "$sig" 2>&1 || true)
echo "$lock2"
must_contain "$lock2" "transaction status: 0" "lock should succeed"
sleep 12
refund2=$(run call FiscoGateway "$gw" refundV2 "$trace2" "$key" 2>&1 || true)
echo "$refund2"
must_contain "$refund2" "transaction status: 0" "refund should succeed after expiry"
payload2=$(payload_hex "$trace2" "$transfer2" "$session2" "$now_ms" "$attester_txt" "$signer_txt")
mint_after_refund=$(run call FiscoGateway "$gw" mintOrUnlockWithProof "$payload2" "$key" 7003 $((now_ms+600000)) "$sig" 2>&1 || true)
echo "$mint_after_refund"
must_not_contain "$mint_after_refund" "transaction status: 0" "mint must fail after refund"

echo "==== stale_locked_proof_after_refund_should_fail ===="
stale_pts=$((now_ms-720000))
payload3=$(payload_hex "$trace2" "$transfer2" "$session2" "$stale_pts" "$attester_txt" "$signer_txt")
stale_after_refund=$(run call FiscoGateway "$gw" mintOrUnlockWithProof "$payload3" "$key" 7004 $((now_ms+600000)) "$sig" 2>&1 || true)
echo "$stale_after_refund"
must_not_contain "$stale_after_refund" "transaction status: 0" "stale proof must fail"

echo "FISCO_MUTEX_GUARDS: PASS"
