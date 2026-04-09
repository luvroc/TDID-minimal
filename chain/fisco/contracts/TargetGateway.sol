// SPDX-License-Identifier: MIT
pragma solidity ^0.8.11;

interface ISigVerifier {
    function verifySessionSig(bytes32 keyId, bytes32 payloadHash, bytes memory sessSig) external view returns (bool);
}

contract FiscoGateway {
    string private constant STATE_LOCKED = "LOCKED";
    string private constant STATE_COMMITTED = "COMMITTED";
    string private constant STATE_REFUNDED = "REFUNDED";

    struct TraceInfo {
        bool exists;
        bytes32 transferId;
        bytes32 sessionId;
        bytes32 traceId;
        string state;
        string srcChainId;
        string dstChainId;
        string asset;
        uint256 amount;
        string sender;
        string recipient;
        bytes32 keyId;
        uint256 nonce;
        uint256 expireAt;
        uint256 updatedAt;
    }

    struct SourceLockProof {
        bytes32 traceId;
        bytes32 transferId;
        bytes32 sessionId;
        bytes32 srcChainIdHash;
        bytes32 lockStateHash;
        uint256 blockHeight;
        bytes32 txHash;
        bytes32 eventHash;
        uint256 proofTimestamp;
        bytes32 attester;
        bytes32 signer;
        bytes32 proofDigest;
        bytes32 proofSigR;
        bytes32 proofSigS;
        uint8 proofSigV;
    }

    struct TxAttestation {
        uint256 issuedAt;
        bytes32 attester;
        bytes32 signer;
        bytes32 quoteHash;
        bytes32 reportDataHash;
        bytes quoteBody;
    }

    bytes32 private constant LOCK_STATE_HASH = keccak256("LOCKED");
    // FISCO in this environment reports block.timestamp in milliseconds.
    // 10 minutes freshness window = 600000 ms.
    uint256 private constant PROOF_MAX_AGE = 600000;
    uint256 private constant ATTESTATION_MIN_QUOTE_BYTES = 1024;

    address public owner;
    string public localChainId;
    bytes32 public localChainIdHash;
    ISigVerifier public sigVerifier;

    mapping(bytes32 => TraceInfo) private traces;
    mapping(bytes32 => bytes32) private traceToTransfer;
    mapping(bytes32 => mapping(uint256 => bool)) private usedNonce;
    mapping(bytes32 => bytes32) private sourceLockProofEventHash;
    mapping(bytes32 => bytes32) private commitTargetBindingHash;
    mapping(bytes32 => bool) private allowedProofAttesters;
    mapping(bytes32 => bool) private allowedProofSigners;
    mapping(address => bool) private allowedProofSignerAddrs;

    event Event_Lock(bytes32 indexed transferId, bytes32 indexed sessionId, bytes32 indexed traceId, bytes32 payloadHash, uint256 timestamp);
    event LockCreated(bytes32 indexed transferId, bytes32 indexed sessionId, bytes32 indexed traceId, bytes32 payloadHash, uint256 timestamp);
    event Event_Settle(bytes32 indexed transferId, bytes32 indexed sessionId, bytes32 indexed traceId, bytes32 payloadHash, uint256 timestamp);
    event SettleCommitted(bytes32 indexed transferId, bytes32 indexed sessionId, bytes32 indexed traceId, bytes32 payloadHash, uint256 timestamp);
    event Event_Refund(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 keyId, bytes32 payloadHash, uint256 timestamp);
    event RefundExecuted(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 keyId, bytes32 payloadHash, uint256 timestamp);
    event Event_Commit(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 keyId, bytes32 payloadHash, uint256 timestamp);
    event CommitExecuted(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 keyId, bytes32 payloadHash, uint256 timestamp);
    event CommitBound(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 bindingHash, bytes32 targetChainTxHash, bytes32 targetReceiptHash, bytes32 targetChainIdHash, bytes32 targetChainHashHash);
    event SourceLockProofAccepted(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 eventHash, uint256 proofTimestamp, bytes32 attester, bytes32 signer);
    event AttestationRelayAccepted(bytes32 indexed transferId, bytes32 indexed traceId, bytes32 quoteHash, bytes32 reportDataHash, uint256 issuedAt);
    event ProofAttesterSet(bytes32 indexed attester, bool allowed);
    event ProofSignerSet(bytes32 indexed signer, bool allowed);
    event ProofSignerAddressSet(address indexed signer, bool allowed);

    constructor(address sigVerifierAddress, string memory chainIdText) {
        require(sigVerifierAddress != address(0), "invalid verifier");
        require(bytes(chainIdText).length > 0, "chain id empty");
        owner = msg.sender;
        sigVerifier = ISigVerifier(sigVerifierAddress);
        localChainId = chainIdText;
        localChainIdHash = keccak256(bytes(chainIdText));
    }

    modifier onlyOwner() {
        require(msg.sender == owner, "only owner");
        _;
    }

    function setProofAttester(bytes32 attester, bool allowed) external onlyOwner {
        require(attester != bytes32(0), "attester empty");
        allowedProofAttesters[attester] = allowed;
        emit ProofAttesterSet(attester, allowed);
    }

    function setProofSigner(bytes32 signer, bool allowed) external onlyOwner {
        require(signer != bytes32(0), "signer empty");
        allowedProofSigners[signer] = allowed;
        emit ProofSignerSet(signer, allowed);
    }

    function setProofSignerAddress(address signer, bool allowed) external onlyOwner {
        require(signer != address(0), "signer address empty");
        allowedProofSignerAddrs[signer] = allowed;
        emit ProofSignerAddressSet(signer, allowed);
    }

    function isProofAttesterAllowed(bytes32 attester) external view returns (bool) {
        return allowedProofAttesters[attester];
    }

    function isProofSignerAllowed(bytes32 signer) external view returns (bool) {
        return allowedProofSigners[signer];
    }

    function isProofSignerAddressAllowed(address signer) external view returns (bool) {
        return allowedProofSignerAddrs[signer];
    }

    function lock(
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) external {
        bytes32 transferId = _buildTransferId(bytes32(0), traceId, keyId, nonce, expireAt);
        _lockCore(transferId, bytes32(0), traceId, keyId, nonce, expireAt, sessSig);
    }

    function lockV2(
        bytes32 transferId,
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) external {
        _lockCore(transferId, sessionId, traceId, keyId, nonce, expireAt, sessSig);
    }

    function _lockCore(
        bytes32 transferId,
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) internal {
        require(transferId != bytes32(0), "transferId empty");
        require(traceId != bytes32(0), "traceId empty");
        require(keyId != bytes32(0), "keyId empty");
        require(expireAt > block.timestamp, "expireAt invalid");
        require(!traces[transferId].exists, "transfer exists");
        require(traceToTransfer[traceId] == bytes32(0), "trace mapped");
        require(!usedNonce[keyId][nonce], "nonce used");

        bytes32 payloadHash = _buildActionDigest("LOCK", transferId, traceId, keyId, nonce, expireAt);
        require(sigVerifier.verifySessionSig(keyId, payloadHash, sessSig), "verifySessionSig failed");

        traces[transferId] = TraceInfo({
            exists: true,
            transferId: transferId,
            sessionId: sessionId,
            traceId: traceId,
            state: STATE_LOCKED,
            srcChainId: localChainId,
            dstChainId: "",
            asset: "",
            amount: 0,
            sender: "",
            recipient: "",
            keyId: keyId,
            nonce: nonce,
            expireAt: expireAt,
            updatedAt: block.timestamp
        });

        traceToTransfer[traceId] = transferId;
        usedNonce[keyId][nonce] = true;

        emit Event_Lock(transferId, sessionId, traceId, payloadHash, block.timestamp);
        emit LockCreated(transferId, sessionId, traceId, payloadHash, block.timestamp);
    }

    function mintOrUnlock(
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) external {
        bytes32 transferId = _buildTransferId(bytes32(0), traceId, keyId, nonce, expireAt);
        _settleCore(transferId, bytes32(0), traceId, keyId, nonce, expireAt, sessSig);
    }

    function mintOrUnlockV2(
        bytes32 transferId,
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) external {
        _settleCore(transferId, sessionId, traceId, keyId, nonce, expireAt, sessSig);
    }

    function mintOrUnlockWithProof(
        bytes memory proofPayload,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) external {
        SourceLockProof memory p = _decodeSourceLockProof(proofPayload);
        _validateSourceLockProof(p);
        _settleCore(p.transferId, p.sessionId, p.traceId, keyId, nonce, expireAt, sessSig);
        sourceLockProofEventHash[p.transferId] = p.eventHash;
        emit SourceLockProofAccepted(p.transferId, p.traceId, p.eventHash, p.proofTimestamp, p.attester, p.signer);
    }

    function mintOrUnlockWithAttestation(
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        uint256 issuedAt,
        bytes32 attester,
        bytes32 signer,
        bytes32 quoteHash,
        bytes32 reportDataHash,
        bytes calldata quoteBody
    ) external {
        bytes32 transferId = _buildTransferId(bytes32(0), traceId, keyId, nonce, expireAt);
        TxAttestation memory a = TxAttestation({
            issuedAt: issuedAt,
            attester: attester,
            signer: signer,
            quoteHash: quoteHash,
            reportDataHash: reportDataHash,
            quoteBody: quoteBody
        });
        _validateTxAttestation(a, traceId, keyId, nonce, expireAt);
        _settleCoreAttested(transferId, bytes32(0), traceId, keyId, nonce, expireAt);
        emit AttestationRelayAccepted(transferId, traceId, a.quoteHash, a.reportDataHash, a.issuedAt);
    }

    function _settleCore(
        bytes32 transferId,
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        bytes memory sessSig
    ) internal {
        require(transferId != bytes32(0), "transferId empty");
        require(traceId != bytes32(0), "traceId empty");
        require(keyId != bytes32(0), "keyId empty");
        require(expireAt > block.timestamp, "expireAt invalid");
        require(!traces[transferId].exists, "transfer exists");
        require(traceToTransfer[traceId] == bytes32(0), "trace mapped");
        require(!usedNonce[keyId][nonce], "nonce used");

        bytes32 payloadHash = _buildActionDigest("SETTLE", transferId, traceId, keyId, nonce, expireAt);
        require(sigVerifier.verifySessionSig(keyId, payloadHash, sessSig), "verifySessionSig failed");

        traces[transferId] = TraceInfo({
            exists: true,
            transferId: transferId,
            sessionId: sessionId,
            traceId: traceId,
            state: STATE_COMMITTED,
            srcChainId: "",
            dstChainId: localChainId,
            asset: "",
            amount: 0,
            sender: "",
            recipient: "",
            keyId: keyId,
            nonce: nonce,
            expireAt: expireAt,
            updatedAt: block.timestamp
        });

        traceToTransfer[traceId] = transferId;
        usedNonce[keyId][nonce] = true;

        emit Event_Settle(transferId, sessionId, traceId, payloadHash, block.timestamp);
        emit SettleCommitted(transferId, sessionId, traceId, payloadHash, block.timestamp);
    }

    function _settleCoreAttested(
        bytes32 transferId,
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt
    ) internal {
        require(transferId != bytes32(0), "transferId empty");
        require(traceId != bytes32(0), "traceId empty");
        require(keyId != bytes32(0), "keyId empty");
        require(expireAt > block.timestamp, "expireAt invalid");
        require(!traces[transferId].exists, "transfer exists");
        require(traceToTransfer[traceId] == bytes32(0), "trace mapped");
        require(!usedNonce[keyId][nonce], "nonce used");

        bytes32 payloadHash = _buildActionDigest("SETTLE_ATTEST", transferId, traceId, keyId, nonce, expireAt);

        traces[transferId] = TraceInfo({
            exists: true,
            transferId: transferId,
            sessionId: sessionId,
            traceId: traceId,
            state: STATE_COMMITTED,
            srcChainId: "",
            dstChainId: localChainId,
            asset: "",
            amount: 0,
            sender: "",
            recipient: "",
            keyId: keyId,
            nonce: nonce,
            expireAt: expireAt,
            updatedAt: block.timestamp
        });

        traceToTransfer[traceId] = transferId;
        usedNonce[keyId][nonce] = true;

        emit Event_Settle(transferId, sessionId, traceId, payloadHash, block.timestamp);
        emit SettleCommitted(transferId, sessionId, traceId, payloadHash, block.timestamp);
    }

    function refund(bytes32 traceId, bytes32 keyId, bytes memory) external {
        _refundV2(traceId, keyId);
    }

    function refundV2(bytes32 traceId, bytes32 keyId) external {
        _refundV2(traceId, keyId);
    }

    function commit(
        bytes32 traceId,
        bytes32 keyId,
        bytes memory
    ) external {
        revert("use commitV2");
    }

    function commitV2(
        bytes32 traceId,
        bytes32 keyId,
        string memory targetChainTx,
        string memory targetReceipt,
        string memory targetChainID,
        string memory targetChainHash
    ) external {
        _commitV2(traceId, keyId, targetChainTx, targetReceipt, targetChainID, targetChainHash);
    }

    function _commitV2(
        bytes32 traceId,
        bytes32 keyId,
        string memory targetChainTx,
        string memory targetReceipt,
        string memory targetChainID,
        string memory targetChainHash
    ) internal {
        bytes32 transferId = traceToTransfer[traceId];
        require(transferId != bytes32(0), "trace not mapped");
        TraceInfo storage tr = traces[transferId];
        require(tr.exists, "trace not found");
        require(keccak256(bytes(tr.state)) == keccak256(bytes(STATE_LOCKED)), "invalid state");
        require(tr.keyId == keyId, "keyId mismatch");
        require(bytes(targetChainTx).length > 0, "targetChainTx empty");
        require(bytes(targetReceipt).length > 0, "targetReceipt empty");
        require(bytes(targetChainID).length > 0, "targetChainID empty");
        require(bytes(targetChainHash).length > 0, "targetChainHash empty");

        tr.state = STATE_COMMITTED;
        tr.updatedAt = block.timestamp;

        bytes32 targetChainTxHash = keccak256(bytes(targetChainTx));
        bytes32 targetReceiptHash = keccak256(bytes(targetReceipt));
        bytes32 targetChainIDHash = keccak256(bytes(targetChainID));
        bytes32 targetChainHashHash = keccak256(bytes(targetChainHash));
        bytes32 bindingHash = keccak256(abi.encodePacked(targetChainTx, targetReceipt, targetChainID, targetChainHash));
        commitTargetBindingHash[transferId] = bindingHash;

        bytes32 payloadHash = keccak256(
            abi.encodePacked(
                "COMMIT",
                transferId,
                traceId,
                tr.keyId,
                tr.nonce,
                tr.expireAt,
                targetChainTxHash,
                targetReceiptHash,
                targetChainIDHash,
                targetChainHashHash
            )
        );
        emit Event_Commit(transferId, traceId, keyId, payloadHash, block.timestamp);
        emit CommitExecuted(transferId, traceId, keyId, payloadHash, block.timestamp);
        emit CommitBound(transferId, traceId, bindingHash, targetChainTxHash, targetReceiptHash, targetChainIDHash, targetChainHashHash);
    }

    function _refundV2(bytes32 traceId, bytes32 keyId) internal {
        bytes32 transferId = traceToTransfer[traceId];
        require(transferId != bytes32(0), "trace not mapped");
        TraceInfo storage tr = traces[transferId];
        require(tr.exists, "trace not found");
        require(keccak256(bytes(tr.state)) == keccak256(bytes(STATE_LOCKED)), "invalid state");
        require(tr.keyId == keyId, "keyId mismatch");
        require(block.timestamp > tr.expireAt, "not expired");

        tr.state = STATE_REFUNDED;
        tr.updatedAt = block.timestamp;

        bytes32 payloadHash = _buildActionDigest("REFUND", transferId, traceId, tr.keyId, tr.nonce, tr.expireAt);
        emit Event_Refund(transferId, traceId, keyId, payloadHash, block.timestamp);
        emit RefundExecuted(transferId, traceId, keyId, payloadHash, block.timestamp);
    }

    function getTrace(bytes32 traceId) external view returns (
        bool exists,
        string memory state,
        string memory srcChainId,
        string memory dstChainId,
        string memory asset,
        uint256 amount,
        string memory sender,
        string memory recipient,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt,
        uint256 updatedAt
    ) {
        bytes32 transferId = traceToTransfer[traceId];
        TraceInfo storage tr = traces[transferId];
        return (
            tr.exists,
            tr.state,
            tr.srcChainId,
            tr.dstChainId,
            tr.asset,
            tr.amount,
            tr.sender,
            tr.recipient,
            tr.keyId,
            tr.nonce,
            tr.expireAt,
            tr.updatedAt
        );
    }

    function isNonceUsed(bytes32 keyId, uint256 nonce) external view returns (bool) {
        return usedNonce[keyId][nonce];
    }

    function getTransferIdByTrace(bytes32 traceId) external view returns (bytes32) {
        return traceToTransfer[traceId];
    }

    function getSourceLockProofEventHash(bytes32 transferId) external view returns (bytes32) {
        return sourceLockProofEventHash[transferId];
    }

    function getCommitTargetBindingHash(bytes32 transferId) external view returns (bytes32) {
        return commitTargetBindingHash[transferId];
    }

    function _buildTransferId(
        bytes32 sessionId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt
    ) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(sessionId, traceId, keyId, nonce, expireAt));
    }

    function _buildActionDigest(
        string memory action,
        bytes32 transferId,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt
    ) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(action, transferId, traceId, keyId, nonce, expireAt));
    }

    function _decodeSourceLockProof(bytes memory payload) internal pure returns (SourceLockProof memory) {
        return abi.decode(payload, (SourceLockProof));
    }

    function _validateSourceLockProof(SourceLockProof memory p) internal view {
        require(p.traceId != bytes32(0), "proof traceId empty");
        require(p.transferId != bytes32(0), "proof transferId empty");
        require(p.sessionId != bytes32(0), "proof sessionId empty");
        require(p.srcChainIdHash != bytes32(0), "proof srcChain empty");
        require(p.srcChainIdHash != localChainIdHash, "proof srcChain invalid");
        require(p.lockStateHash == LOCK_STATE_HASH, "proof lockState invalid");
        require(p.blockHeight > 0, "proof blockHeight invalid");
        require(p.proofTimestamp > 0, "proof timestamp invalid");
        require(p.proofTimestamp <= block.timestamp, "proof timestamp future");
        require(block.timestamp - p.proofTimestamp <= PROOF_MAX_AGE, "proof timestamp expired");
        require(p.txHash != bytes32(0), "proof txHash empty");
        require(p.eventHash != bytes32(0), "proof eventHash empty");
        require(p.attester != bytes32(0), "proof attester empty");
        require(p.signer != bytes32(0), "proof signer empty");
        require(p.proofDigest != bytes32(0), "proof digest empty");
        require(p.proofDigest == _buildProofDigest(p), "proof digest mismatch");
        require(p.proofSigR != bytes32(0), "proof sigR empty");
        require(p.proofSigS != bytes32(0), "proof sigS empty");
        uint8 normalizedV = p.proofSigV;
        if (normalizedV < 27) {
            normalizedV += 27;
        }
        require(normalizedV == 27 || normalizedV == 28, "proof sigV invalid");
        address recoveredSigner = ecrecover(p.proofDigest, normalizedV, p.proofSigR, p.proofSigS);
        require(recoveredSigner != address(0), "proof signature invalid");
        bytes32 recoveredSignerB32 = bytes32(uint256(uint160(recoveredSigner)));
        require(p.signer == recoveredSignerB32, "proof signer mismatch");
        require(allowedProofAttesters[p.attester], "proof attester not allowed");
        require(allowedProofSigners[p.signer] || allowedProofSignerAddrs[recoveredSigner], "proof signer not allowed");
        require(sourceLockProofEventHash[p.transferId] == bytes32(0), "proof already consumed");
    }

    function _buildProofDigest(SourceLockProof memory p) internal pure returns (bytes32) {
        return keccak256(
            abi.encodePacked(
                p.traceId,
                p.transferId,
                p.sessionId,
                p.srcChainIdHash,
                p.lockStateHash,
                p.blockHeight,
                p.txHash,
                p.eventHash,
                p.proofTimestamp,
                p.attester,
                p.signer
            )
        );
    }

    function _validateTxAttestation(
        TxAttestation memory a,
        bytes32 traceId,
        bytes32 keyId,
        uint256 nonce,
        uint256 expireAt
    ) internal view {
        require(a.issuedAt > 0, "attestation issuedAt empty");
        uint256 nowMillis = block.timestamp;
        if (nowMillis < 100000000000) {
            nowMillis = nowMillis * 1000;
        }
        uint256 issuedMillis = a.issuedAt;
        if (issuedMillis < 100000000000) {
            issuedMillis = issuedMillis * 1000;
        }
        require(issuedMillis <= nowMillis, "attestation issuedAt future");
        require(nowMillis - issuedMillis <= PROOF_MAX_AGE, "attestation expired");
        require(a.attester != bytes32(0), "attestation attester empty");
        require(a.signer != bytes32(0), "attestation signer empty");
        require(a.quoteHash != bytes32(0), "attestation quoteHash empty");
        require(a.reportDataHash != bytes32(0), "attestation reportDataHash empty");
        require(a.quoteBody.length >= ATTESTATION_MIN_QUOTE_BYTES, "attestation quote too short");
        require(allowedProofAttesters[a.attester], "attestation attester not allowed");
        require(allowedProofSigners[a.signer], "attestation signer not allowed");

        bytes32 expectedReportDataHash = keccak256(
            abi.encodePacked(traceId, keyId, nonce, expireAt, localChainIdHash)
        );
        require(a.reportDataHash == expectedReportDataHash, "attestation reportDataHash mismatch");
        require(a.quoteHash == keccak256(a.quoteBody), "attestation quoteHash mismatch");
        require(_scanQuoteBodyDigest(a.quoteBody) != bytes32(0), "attestation quote parse failed");
    }

    function _scanQuoteBodyDigest(bytes memory quoteBody) internal pure returns (bytes32 digest) {
        bytes32 rolling = bytes32(0);
        uint256 len = quoteBody.length;
        for (uint256 round = 0; round < 3; round++) {
            for (uint256 i = 0; i < len; i += 32) {
                bytes32 chunk;
                assembly {
                    chunk := mload(add(add(quoteBody, 32), i))
                }
                rolling = keccak256(abi.encodePacked(rolling, chunk, round));
            }
        }
        return rolling;
    }
}
