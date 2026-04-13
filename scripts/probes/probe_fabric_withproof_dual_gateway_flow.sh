#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${CHAIN_DOT_HOME}/test-network}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
SRC_CC="${SRC_CC:-gatewaycc}"
DST_CC="${DST_CC:-gatewaycc_target}"

cd "${FABRIC_NET_DIR}"
export PATH="/usr/local/go/bin:${CHAIN_DOT_HOME}/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export OVERRIDE_ORG=""
export VERBOSE="false"
export TEST_NETWORK_HOME="${CHAIN_DOT_HOME}/test-network"
export FABRIC_CFG_PATH="${CHAIN_DOT_HOME}/config"
source scripts/envVar.sh
setGlobals 1

invoke() {
  local cc="$1"
  local ctor="$2"
  peer chaincode invoke \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "$ORDERER_CA" \
    -C "${CHANNEL_NAME}" -n "${cc}" \
    --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
    --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
    --waitForEvent --waitForEventTimeout 30s \
    -c "${ctor}"
}

query() {
  local cc="$1"
  local ctor="$2"
  peer chaincode query -C "${CHANNEL_NAME}" -n "${cc}" -c "${ctor}"
}

echo "[fabric-proof-dual-gateway] ensure target gateway chaincode is deployed: ${DST_CC}"
echo "[fabric-proof-dual-gateway] deployment is expected to be done before this probe"

cd "${FABRIC_NET_DIR}"
RID="${RID:-$(date +%s)}"
TRACE="trace-fabric-proof-dual-${RID}"
TRANSFER="transfer-fabric-proof-dual-${RID}"
SESSION="session-fabric-proof-dual-${RID}"
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
EXP=$(( $(date +%s) + 3600 ))
NONCE=$(( RID % 100000 + 12000 ))

echo "[fabric-proof-dual-gateway] lock on source cc=${SRC_CC}"
LOCK_CTOR=$(printf '{"function":"LockV2","Args":["%s","%s","%s","fabric","USDT","10","alice","bob","%s","%s","%s","%s"]}' \
  "${TRANSFER}" "${SESSION}" "${TRACE}" "${KEY}" "${NONCE}" "${EXP}" "${SIG}")
LOCK_OUT=$(invoke "${SRC_CC}" "${LOCK_CTOR}" 2>&1 || true)
echo "${LOCK_OUT}"
[[ "${LOCK_OUT}" == *"status:200"* ]] || { echo "ASSERT FAIL: source lock failed"; exit 1; }

echo "[fabric-proof-dual-gateway] build proof on source"
PROOF_BUILD_CTOR=$(printf '{"function":"BuildSourceLockProof","Args":["%s","temp-attester","temp-signer"]}' "${TRACE}")
PROOF_JSON=$(query "${SRC_CC}" "${PROOF_BUILD_CTOR}")
PROOF_PAYLOAD=$(printf "%s" "${PROOF_JSON}" | base64 -w0)

echo "[fabric-proof-dual-gateway] mint with proof on target cc=${DST_CC}"
MINT_CTOR=$(printf '{"function":"MintOrUnlockWithProof","Args":["%s","%s","%s","","fabric","USDT","10","alice","bob","%s","%s","%s","%s","0xsrc","src-tx","src-receipt","0xsrcsig","%s"]}' \
  "${TRANSFER}" "${SESSION}" "${TRACE}" "${KEY}" "$((NONCE+1))" "${EXP}" "${SIG}" "${PROOF_PAYLOAD}")
MINT_OUT=$(invoke "${DST_CC}" "${MINT_CTOR}" 2>&1 || true)
echo "${MINT_OUT}"
[[ "${MINT_OUT}" == *"status:200"* ]] || { echo "ASSERT FAIL: target MintOrUnlockWithProof failed"; exit 1; }

echo "[fabric-proof-dual-gateway] query target trace state"
TRACE_Q_CTOR=$(printf '{"function":"GetTrace","Args":["%s"]}' "${TRACE}")
TRACE_OUT=$(query "${DST_CC}" "${TRACE_Q_CTOR}")
echo "${TRACE_OUT}"
[[ "${TRACE_OUT}" == *"COMMITTED"* ]] || { echo "ASSERT FAIL: target trace not COMMITTED"; exit 1; }

echo "FABRIC_WITH_PROOF_DUAL_GATEWAY: PASS"
