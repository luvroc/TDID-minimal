#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="/home/ecs-user"
TDID_DIR="${ROOT_DIR}/TDID"
FISCO_CONSOLE_DIR="${ROOT_DIR}/fisco/console"
PROOF_SIGNER_PRIVKEY="${PROOF_SIGNER_PRIVKEY:-59c6995e998f97a5a0044976f7d2cbb7d0c7f8f6ec6cf4de4df2f8f8b7f6d1f7}"

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <gateway_addr>"
  exit 1
fi

GATEWAY_ADDR="$1"

mk_bytes32() {
  local input="$1"
  python3 - <<'PY' "$input"
from eth_hash.auto import keccak
import sys
print("0x" + keccak(sys.argv[1].encode()).hex())
PY
}

TRACE_ID=$(mk_bytes32 "trace-proof-$(date +%s)")
TRANSFER_ID=$(mk_bytes32 "transfer-proof-${TRACE_ID}")
SESSION_ID=$(mk_bytes32 "session-proof")
SRC_CHAIN_HASH=$(mk_bytes32 "fabric")
LOCK_STATE_HASH=$(mk_bytes32 "LOCKED")
BLOCK_HEIGHT=1
TX_HASH=$(mk_bytes32 "tx-proof")
EVENT_HASH=$(mk_bytes32 "event-proof")
PROOF_TS=$(date +%s%3N)
ATTESTER=$(mk_bytes32 "temp-attester")

SIGNER_ADDR=$(python3 - <<'PY' "$PROOF_SIGNER_PRIVKEY"
from eth_keys import keys
import sys
pk = sys.argv[1].strip().replace("0x","").replace("0X","")
priv = keys.PrivateKey(bytes.fromhex(pk))
print(priv.public_key.to_checksum_address())
PY
)
SIGNER_B32=$(python3 - <<'PY' "$SIGNER_ADDR"
import sys
v=sys.argv[1].strip().lower()
if v.startswith('0x'):
  v=v[2:]
print('0x'+v.rjust(64,'0'))
PY
)

PROOF_DIGEST=$(python3 - <<'PY' "$TRACE_ID" "$TRANSFER_ID" "$SESSION_ID" "$SRC_CHAIN_HASH" "$LOCK_STATE_HASH" "$BLOCK_HEIGHT" "$TX_HASH" "$EVENT_HASH" "$PROOF_TS" "$ATTESTER" "$SIGNER_B32"
from eth_hash.auto import keccak
import sys
vals=[x.strip() for x in sys.argv[1:]]
raw=b''
for idx,v in enumerate(vals):
    if idx in (5,8):
        raw += int(v).to_bytes(32,'big',signed=False)
    else:
        h=v[2:] if v.startswith('0x') else v
        raw += bytes.fromhex(h.rjust(64,'0'))
print('0x'+keccak(raw).hex())
PY
)

PROOF_SIG=$(python3 - <<'PY' "$PROOF_DIGEST" "$PROOF_SIGNER_PRIVKEY"
from eth_keys import keys
import sys
d=sys.argv[1].strip()
if d.startswith('0x'): d=d[2:]
pk=sys.argv[2].strip().replace('0x','').replace('0X','')
priv=keys.PrivateKey(bytes.fromhex(pk))
sig=priv.sign_msg_hash(bytes.fromhex(d))
print('0x'+sig.to_bytes().hex())
PY
)

echo "SourceLockProof fields prepared:"
echo "  traceId=${TRACE_ID}"
echo "  transferId=${TRANSFER_ID}"
echo "  sessionId=${SESSION_ID}"
echo "  srcChainIdHash=${SRC_CHAIN_HASH}"
echo "  lockStateHash=${LOCK_STATE_HASH}"
echo "  blockHeight=${BLOCK_HEIGHT}"
echo "  txHash=${TX_HASH}"
echo "  eventHash=${EVENT_HASH}"
echo "  proofTimestamp=${PROOF_TS}"
echo "  attester=${ATTESTER}"
echo "  signer=${SIGNER_ADDR} (bytes32=${SIGNER_B32})"
echo "  proofDigest=${PROOF_DIGEST}"
echo "  proofSig=${PROOF_SIG}"

echo
echo "Before mintOrUnlockWithProof, initialize allowlist:"
echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofAttester ${ATTESTER} true"
echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofSigner ${SIGNER_B32} true"
echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} setProofSignerAddress ${SIGNER_ADDR} true"

echo
echo "Then call (replace <keyId>/<nonce>/<expireAt>/<sig>):"
JSON_PAYLOAD=$(cat <<JSON
{"traceId":"$TRACE_ID","transferId":"$TRANSFER_ID","sessionId":"$SESSION_ID","srcChainId":"fabric","lockState":"LOCKED","blockHeight":$BLOCK_HEIGHT,"txHash":"$TX_HASH","eventHash":"$EVENT_HASH","proofTimestamp":$PROOF_TS,"attester":"temp-attester","signer":"$SIGNER_ADDR","proofDigest":"$PROOF_DIGEST","proofSig":"$PROOF_SIG"}
JSON
)
ENCODED=$(python3 "${TDID_DIR}/fisco/scripts/encode_source_lock_proof.py" "${JSON_PAYLOAD}")
echo "proofPayload=${ENCODED}"
echo "bash console.sh call FiscoGateway ${GATEWAY_ADDR} mintOrUnlockWithProof ${ENCODED} <keyId> <nonce> <expireAt> <sig>"
