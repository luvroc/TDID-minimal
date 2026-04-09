#!/usr/bin/env python3
import json
import re
import sys
from eth_hash.auto import keccak


def extract_json_blob(text: str) -> str:
    start = text.find("{")
    end = text.rfind("}")
    if start != -1 and end != -1 and end > start:
        return text[start : end + 1]
    return text


def parse_obj(raw: str):
    blob = extract_json_blob(raw.strip())
    obj = None
    try:
        obj = json.loads(blob)
    except Exception:
        pass
    if isinstance(obj, str):
        obj = json.loads(obj)
    if obj is None:
        obj = json.loads(json.loads(blob))
    return obj


def as_bytes32(v):
    if v is None:
        return "00" * 32
    s = str(v).strip()
    if s == "":
        return "00" * 32
    if s.startswith("0x") or s.startswith("0X"):
        h = s[2:].lower()
        if re.fullmatch(r"[0-9a-f]+", h):
            if len(h) > 64:
                h = h[-64:]
            return h.rjust(64, "0")
    return keccak(s.encode()).hex()


def as_u256(v):
    n = int(v)
    if n < 0:
        n = 0
    return f"{n:064x}"


def parse_sig_parts(sig_value):
    if sig_value is None:
        return ("00" * 32, "00" * 32, 0)
    s = str(sig_value).strip()
    if s == "":
        return ("00" * 32, "00" * 32, 0)
    if s.startswith(("0x", "0X")):
        s = s[2:]
    s = s.lower()
    if not re.fullmatch(r"[0-9a-f]+", s):
        return ("00" * 32, "00" * 32, 0)
    if len(s) != 130:
        return ("00" * 32, "00" * 32, 0)
    r = s[:64]
    sig_s = s[64:128]
    v = int(s[128:130], 16)
    if v in (27, 28):
        v -= 27
    if v not in (0, 1):
        return ("00" * 32, "00" * 32, 0)
    return (r, sig_s, v)


def compute_proof_digest_hex(
    trace_id,
    transfer_id,
    session_id,
    src_chain_hash,
    lock_state_hash,
    block_height,
    tx_hash,
    event_hash,
    proof_ts,
    attester,
    signer,
):
    raw = bytes.fromhex(
        trace_id
        + transfer_id
        + session_id
        + src_chain_hash
        + lock_state_hash
        + block_height
        + tx_hash
        + event_hash
        + proof_ts
        + attester
        + signer
    )
    return keccak(raw).hex()


def main():
    if len(sys.argv) < 2:
        raise SystemExit("usage: encode_source_lock_proof.py '<proof_json_or_output>'")

    raw = sys.argv[1]
    obj = parse_obj(raw)

    trace_id = as_bytes32(obj.get("traceId"))
    transfer_id = as_bytes32(obj.get("transferId"))
    session_id = as_bytes32(obj.get("sessionId"))
    src_chain_hash = as_bytes32(obj.get("srcChainId"))
    lock_state_hash = as_bytes32(obj.get("lockState"))
    block_height = as_u256(obj.get("blockHeight", 0))
    tx_hash = as_bytes32(obj.get("txHash"))
    event_hash = as_bytes32(obj.get("eventHash"))
    proof_ts = as_u256(obj.get("proofTimestamp", 0))
    attester = as_bytes32(obj.get("attester"))
    signer = as_bytes32(obj.get("signer"))
    proof_digest = as_bytes32(obj.get("proofDigest"))
    if proof_digest == ("00" * 32):
        proof_digest = compute_proof_digest_hex(
            trace_id,
            transfer_id,
            session_id,
            src_chain_hash,
            lock_state_hash,
            block_height,
            tx_hash,
            event_hash,
            proof_ts,
            attester,
            signer,
        )
    sig_r, sig_s, sig_v = parse_sig_parts(obj.get("proofSig"))
    sig_v_u256 = as_u256(sig_v)

    payload = (
        trace_id
        + transfer_id
        + session_id
        + src_chain_hash
        + lock_state_hash
        + block_height
        + tx_hash
        + event_hash
        + proof_ts
        + attester
        + signer
        + proof_digest
        + sig_r
        + sig_s
        + sig_v_u256
    )
    print("0x" + payload)


if __name__ == "__main__":
    main()
