#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FISCO_CONSOLE_DIR="${FISCO_CONSOLE_DIR:-${ROOT_DIR}/fisco/console}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${CHAIN_DOT_HOME}/test-network}"

run_console() { (cd "${FISCO_CONSOLE_DIR}" && bash console.sh "$@"); }
extract_addr(){ echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1; }

cp "${TDID_DIR}/fisco/contracts/TargetGateway.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"
out_m=$(run_console deploy MockSigVerifier); mock=$(extract_addr "$out_m")
out_g=$(run_console deploy FiscoGateway "$mock" fisco); gw=$(extract_addr "$out_g")
echo "[P1-2] gateway=$gw"

cd "${FABRIC_NET_DIR}"
export PATH="${ROOT_DIR}/chain-DOT/bin:/usr/local/go/bin:${PATH}"
export OVERRIDE_ORG=""
export VERBOSE="false"
export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
export FABRIC_CFG_PATH="${ROOT_DIR}/chain-DOT/config"
source scripts/envVar.sh
setGlobals 1

RID=$(date +%s%N)
TRACE="trace-proofsig-${RID}"
TRANSFER="transfer-proofsig-${RID}"
SESSION="session-proofsig-${RID}"
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
NONCE=$(( (RID % 1000000000) + 70000 ))
EXP=$(( $(date +%s) + 3600 ))

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C mychannel -n gatewaycc \
  --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
  --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
  --waitForEvent --waitForEventTimeout 30s \
  -c "{\"function\":\"LockV2\",\"Args\":[\"$TRANSFER\",\"$SESSION\",\"$TRACE\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$NONCE\",\"$EXP\",\"$SIG\"]}" >/tmp/proofsig_lock.out

echo "[P1-2] lock trace=${TRACE}"
bash "${TDID_DIR}/fisco/scripts/bridge-proof-to-fisco.sh" \
  --trace-id "$TRACE" \
  --gateway-addr "$gw" \
  --key-id "$KEY" \
  --nonce "$((NONCE + 1))" \
  --expire-at "$(( $(date +%s) + 3600 ))" \
  --sig "$SIG" \
  --attester "temp-attester" \
  --signer "temp-signer" >/tmp/proofsig_positive.out 2>&1
cat /tmp/proofsig_positive.out
grep -q "transaction status: 0" /tmp/proofsig_positive.out

PROOF_RAW=$(peer chaincode query -C mychannel -n gatewaycc -c "{\"function\":\"GetSourceLockProof\",\"Args\":[\"$TRANSFER\"]}")
python3 - <<'PY' "$PROOF_RAW"
import json,sys
raw=sys.argv[1].strip()
obj=json.loads(raw)
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
    raise SystemExit(f"[P1-2] proof meta mismatch: {k} expected={v} got={got}")
print("[P1-2] proof meta check: PASS")
PY
TAMPERED=$(python3 - <<'PY' "$PROOF_RAW"
import json,sys
obj=json.loads(sys.argv[1])
sig=obj.get("proofSig","")
if sig.startswith("0x") and len(sig) >= 4:
  sig=sig[:-1]+("0" if sig[-1]!="0" else "1")
obj["proofSig"]=sig
print(json.dumps(obj,separators=(",",":")))
PY
)
PAYLOAD=$(python3 "${TDID_DIR}/fisco/scripts/encode_source_lock_proof.py" "$TAMPERED")
NEG_OUT=$(run_console call FiscoGateway "$gw" mintOrUnlockWithProof "$PAYLOAD" "$KEY" "$((NONCE + 2))" "$(( ($(date +%s) + 3600) * 1000 ))" "$SIG" || true)
echo "$NEG_OUT"
if [[ "$NEG_OUT" == *"transaction status: 0"* ]]; then
  echo "ASSERT FAIL: tampered signature should fail"
  exit 1
fi
if [[ "$NEG_OUT" != *"proof signer mismatch"* && "$NEG_OUT" != *"proof signature invalid"* ]]; then
  echo "ASSERT FAIL: expected proof signature failure, got:"
  echo "$NEG_OUT"
  exit 1
fi

echo "[P1-2] verify-proof-signature: PASS"
