package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"tdid-final/host/chain"
	"tdid-final/shared/errors"
	sharedtypes "tdid-final/shared/types"
)

type SourceWorker struct {
	enclave EnclaveClient
	source  SourceChainClient
	audit   AuditClient
	peer    PeerSender
}

func NewSourceWorker(enclave EnclaveClient, source SourceChainClient, audit AuditClient, peer PeerSender) *SourceWorker {
	return &SourceWorker{enclave: enclave, source: source, audit: audit, peer: peer}
}

func (w *SourceWorker) HandleSourceEvent(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error {
	flowStart := time.Now()
	if w == nil || w.enclave == nil || w.source == nil || w.peer == nil {
		return errors.New(errors.CodeInternal, "source worker dependencies are not ready", nil)
	}
	if req.TraceID == "" {
		return errors.New(errors.CodeInvalidInput, "traceId is required", nil)
	}

	signed, err := w.enclave.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      chainFromID(req.SrcChainID),
		SessionID:  req.SessionID,
		TransferID: req.TransferID,
		TraceID:    req.TraceID,
		SrcChainID: req.SrcChainID,
		DstChainID: req.DstChainID,
		Asset:      req.Asset,
		Amount:     req.Amount,
		Sender:     req.Sender,
		Recipient:  req.Recipient,
		KeyID:      req.KeyID,
		Nonce:      req.Nonce,
		ExpireAt:   req.ExpireAt,
	})
	if err != nil {
		return err
	}
	signLockDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=source_sign_lock duration_ms=%d", req.TraceID, signed.TransferID, signLockDone.Sub(flowStart).Milliseconds())

	lockResp, err := w.source.InvokeSourceLock(ctx, chain.SourceLockRequest{
		SessionID:   signed.SessionID,
		TransferID:  signed.TransferID,
		TraceID:     req.TraceID,
		SrcChainID:  req.SrcChainID,
		DstChainID:  req.DstChainID,
		Asset:       req.Asset,
		Amount:      req.Amount,
		Sender:      req.Sender,
		Recipient:   req.Recipient,
		KeyID:       signed.KeyID,
		Nonce:       signed.Nonce,
		ExpireAt:    signed.ExpireAt,
		PayloadHash: toHex(signed.PayloadHash),
		SessSig:     toHex(signed.SessSig),
	})
	if err != nil {
		return err
	}
	lockDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=source_lock_onchain duration_ms=%d", req.TraceID, signed.TransferID, lockDone.Sub(signLockDone).Milliseconds())

	forwardReq := req
	forwardReq.SessionID = signed.SessionID
	forwardReq.TransferID = signed.TransferID
	forwardReq.RequestDigest = deriveRequestDigest(signed.TransferID, req.TraceID, toHex(signed.PayloadHash))
	forwardReq.KeyID = signed.KeyID
	forwardReq.Nonce = signed.Nonce + 1
	forwardReq.ExpireAt = signed.ExpireAt
	forwardReq.Timestamp = lockDone.UnixMicro()
	forwardReq.SrcPayloadHash = toHex(signed.PayloadHash)
	forwardReq.SrcSessSig = toHex(signed.SessSig)
	forwardReq.SrcLockTx = lockResp.TxHash
	sourceProof, err := w.resolveSourceLockProof(ctx, req, forwardReq)
	if err != nil {
		return err
	}
	forwardReq.SourceLockProof = sourceProof

	peerResp, err := w.peer.SendExecution(ctx, forwardReq)
	if err != nil {
		return err
	}
	peerDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=source_peer_roundtrip duration_ms=%d", req.TraceID, signed.TransferID, peerDone.Sub(lockDone).Milliseconds())
	if !peerResp.Accepted {
		return errors.New(errors.CodeInternal, "peer rejected execution request", fmt.Errorf("trace=%s transfer=%s reason=%s", req.TraceID, signed.TransferID, peerResp.Reason))
	}
	if err := verifyTargetExecutionEvidence(ctx, w.enclave, req, signed.TransferID, peerResp); err != nil {
		return err
	}
	commitResp, err := w.source.InvokeSourceCommit(ctx, chain.SourceCommitRequest{
		TraceID:         req.TraceID,
		KeyID:           signed.KeyID,
		TargetChainTx:   peerResp.TargetChainTx,
		TargetReceipt:   peerResp.TargetReceipt,
		TargetChainID:   peerResp.TargetChainID,
		TargetChainHash: peerResp.TargetChainHash,
	})
	if err != nil {
		return errors.New(errors.CodeInternal, "source commit failed", err)
	}
	commitDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=source_commit_onchain duration_ms=%d", req.TraceID, signed.TransferID, commitDone.Sub(peerDone).Milliseconds())

	if w.audit != nil {
		receipt, receiptErr := w.enclave.BuildReceipt(ctx, sharedtypes.BuildReceiptRequest{
			TransferID:  forwardReq.TransferID,
			TraceID:     req.TraceID,
			TxHash:      commitResp.TxHash,
			ChainID:     req.SrcChainID,
			Amount:      req.Amount,
			Recipient:   req.Recipient,
			PayloadHash: forwardReq.SrcPayloadHash,
			FinalState:  "COMMITTED",
			SrcChainID:  req.SrcChainID,
			DstChainID:  req.DstChainID,
		})
		if receiptErr != nil {
			return receiptErr
		}
		if _, err := w.audit.SubmitSourceReceipt(ctx, chain.AuditReceiptRequest{TransferID: forwardReq.TransferID, TraceID: req.TraceID, ReceiptHash: receipt.ReceiptHashHex}); err != nil {
			return err
		}
		submitSrcDone := time.Now()
		relayEvalLog("trace=%s transfer_id=%s stage=audit_submit_src duration_ms=%d", req.TraceID, signed.TransferID, submitSrcDone.Sub(commitDone).Milliseconds())
		if _, err := w.audit.MatchTransfer(ctx, chain.AuditMatchRequest{TransferID: forwardReq.TransferID, TraceID: req.TraceID}); err != nil {
			return err
		}
		matchDone := time.Now()
		relayEvalLog("trace=%s transfer_id=%s stage=audit_match duration_ms=%d total_ms=%d", req.TraceID, signed.TransferID, matchDone.Sub(submitSrcDone).Milliseconds(), matchDone.Sub(flowStart).Milliseconds())
	}

	relayEvalLog("trace=%s transfer_id=%s stage=source_flow_done total_ms=%d", req.TraceID, signed.TransferID, time.Since(flowStart).Milliseconds())
	return nil
}

func (w *SourceWorker) resolveSourceLockProof(ctx context.Context, req sharedtypes.CrossChainExecutionRequest, forwardReq sharedtypes.CrossChainExecutionRequest) (string, error) {
	if proof := strings.TrimSpace(req.SourceLockProof); proof != "" {
		return proof, nil
	}

	proofJSON, err := w.source.BuildSourceLockProof(ctx, chain.SourceLockProofRequest{
		TraceID:  forwardReq.TraceID,
		Attester: "relay-auto",
		Signer:   "relay-auto",
	})
	if err != nil {
		return "", errors.New(errors.CodeInternal, "build source lock proof failed", err)
	}
	proofJSON = strings.TrimSpace(proofJSON)
	if proofJSON == "" {
		return "", errors.New(errors.CodeInternal, "source lock proof is empty", nil)
	}

	payload, err := w.source.EncodeSourceLockProofPayload(ctx, chain.SourceLockProofPayloadRequest{ProofJSON: proofJSON})
	if err != nil {
		return "", errors.New(errors.CodeInternal, "encode source lock proof payload failed", err)
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", errors.New(errors.CodeInternal, "source lock proof payload is empty", nil)
	}
	return payload, nil
}

func toHex(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return fmt.Sprintf("0x%x", raw)
}

func chainFromID(chainID string) sharedtypes.ChainKind {
	if strings.Contains(strings.ToLower(chainID), "fisco") || strings.Contains(strings.ToLower(chainID), "group") {
		return sharedtypes.ChainFISCO
	}
	return sharedtypes.ChainFabric
}

func deriveRequestDigest(transferID string, traceID string, payloadHash string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{transferID, traceID, payloadHash}, "|")))
	return "sha256:" + hex.EncodeToString(h[:])
}

func verifyTargetExecutionEvidence(ctx context.Context, enclave EnclaveClient, req sharedtypes.CrossChainExecutionRequest, transferID string, peerResp sharedtypes.CrossChainExecutionResponse) error {
	typed := sharedtypes.TargetExecutionEvidenceRequest{
		TraceID:          req.TraceID,
		TransferID:       transferID,
		DstChainID:       req.DstChainID,
		TargetTraceID:    peerResp.TraceID,
		TargetTransferID: peerResp.TransferID,
		TargetChainTx:    peerResp.TargetChainTx,
		TargetReceipt:    peerResp.TargetReceipt,
		TargetChainID:    peerResp.TargetChainID,
		TargetChainHash:  peerResp.TargetChainHash,
	}
	if err := enclave.VerifyTargetExecutionEvidence(ctx, typed); err != nil {
		return errors.New(errors.CodeInternal, "peer response evidence verify failed", err)
	}
	return nil
}
