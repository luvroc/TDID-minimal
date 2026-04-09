#!/usr/bin/env bash
set -euo pipefail
cd ~/fisco/console

run_console(){ bash console.sh "$@"; }
extract_addr(){ echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1; }

out_mock=$(run_console deploy MockSigVerifier)
mock_addr=$(extract_addr "$out_mock")
out_gw=$(run_console deploy FiscoGateway "$mock_addr" fisco)
gw_addr=$(extract_addr "$out_gw")

echo "[A2] mock=$mock_addr gateway=$gw_addr"

mk_b32(){ python3 - <<'PY' "$1"
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

build_payload(){
  local trace="$1" transfer="$2" session="$3" attester="$4" signer="$5" pts="$6"
  local js
  js=$(cat <<JSON
{"traceId":"$trace","transferId":"$transfer","sessionId":"$session","srcChainId":"fabric","lockState":"LOCKED","blockHeight":1,"txHash":"$(mk_b32 tx-$trace)","eventHash":"$(mk_b32 ev-$trace)","proofTimestamp":$pts,"attester":"$attester","signer":"$signer"}
JSON
)
  python3 ~/TDID/fisco/scripts/encode_source_lock_proof.py "$js"
}

now=$(date +%s)
exp=$((now+600000))
sig=0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222
key=0x1111111111111111111111111111111111111111111111111111111111111111

trace1=$(mk_b32 trace-a2-neg)
transfer1=$(mk_b32 transfer-a2-neg)
session1=$(mk_b32 session-a2-neg)
payload1=$(build_payload "$trace1" "$transfer1" "$session1" bad-attester bad-signer "$now")
out1=$(run_console call FiscoGateway "$gw_addr" mintOrUnlockWithProof "$payload1" "$key" 1001 "$exp" "$sig" || true)
echo "[A2-NEG-ALLOWLIST] $out1"

att=$(mk_b32 temp-attester)
sgn=$(mk_b32 temp-signer)
run_console call FiscoGateway "$gw_addr" setProofAttester "$att" true >/dev/null
run_console call FiscoGateway "$gw_addr" setProofSigner "$sgn" true >/dev/null
trace2=$(mk_b32 trace-a2-pos)
transfer2=$(mk_b32 transfer-a2-pos)
session2=$(mk_b32 session-a2-pos)
payload2=$(build_payload "$trace2" "$transfer2" "$session2" temp-attester temp-signer "$now")
out2=$(run_console call FiscoGateway "$gw_addr" mintOrUnlockWithProof "$payload2" "$key" 1002 "$exp" "$sig" || true)
echo "[A2-POS] $out2"

oldts=$((now-7200))
trace3=$(mk_b32 trace-a2-expired)
transfer3=$(mk_b32 transfer-a2-expired)
session3=$(mk_b32 session-a2-expired)
payload3=$(build_payload "$trace3" "$transfer3" "$session3" temp-attester temp-signer "$oldts")
out3=$(run_console call FiscoGateway "$gw_addr" mintOrUnlockWithProof "$payload3" "$key" 1003 "$exp" "$sig" || true)
echo "[A2-NEG-EXPIRED] $out3"
