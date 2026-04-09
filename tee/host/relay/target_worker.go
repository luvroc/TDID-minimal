package relay

import (
	"context"
	"time"

	"tdid-final/host/chain"
	"tdid-final/shared/errors"
	sharedtypes "tdid-final/shared/types"
)

type TargetWorker struct {
	enclave EnclaveClient
	target  TargetChainClient
	audit   AuditClient
}

func NewTargetWorker(enclave EnclaveClient, target TargetChainClient, audit AuditClient) *TargetWorker {
	return &TargetWorker{enclave: enclave, target: target, audit: audit}
}

func (w *TargetWorker) HandlePeerExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	flowStart := time.Now()
	if w == nil || w.enclave == nil || w.target == nil {
		return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false}, errors.New(errors.CodeInternal, "target worker dependencies are not ready", nil)
	}
	if err := w.enclave.VerifyPeerCrossMessage(ctx, req); err != nil {
		return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false, Reason: "peer message verify failed"}, err
	}
	verifyDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=target_verify_peer duration_ms=%d", req.TraceID, req.TransferID, verifyDone.Sub(flowStart).Milliseconds())
	signed, err := w.enclave.SignMintOrUnlock(ctx, sharedtypes.SignMintOrUnlockRequest{
		Chain:      chainFromID(req.DstChainID),
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
		return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false, Reason: "sign mintOrUnlock failed"}, err
	}
	signDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=target_sign_mint duration_ms=%d", req.TraceID, signed.TransferID, signDone.Sub(verifyDone).Milliseconds())
	mintResp, err := w.target.InvokeTargetExecute(ctx, chain.TargetExecuteRequest{
		SessionID:         signed.SessionID,
		TransferID:        signed.TransferID,
		TraceID:           req.TraceID,
		SrcChainID:        req.SrcChainID,
		DstChainID:        req.DstChainID,
		Asset:             req.Asset,
		Amount:            req.Amount,
		Sender:            req.Sender,
		Recipient:         req.Recipient,
		KeyID:             signed.KeyID,
		Nonce:             signed.Nonce,
		ExpireAt:          signed.ExpireAt,
		PayloadHash:       toHex(signed.PayloadHash),
		SessSig:           toHex(signed.SessSig),
		SourceLockTx:      req.SrcLockTx,
		SourceReceipt:     req.SrcReceipt,
		SourceSessSig:     req.SrcSessSig,
		SourcePayloadHash: req.SrcPayloadHash,
		SourceLockProof:   req.SourceLockProof,
	})
	if err != nil {
		return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false, Reason: "mintOrUnlock failed"}, err
	}
	mintDone := time.Now()
	relayEvalLog("trace=%s transfer_id=%s stage=target_mint_onchain duration_ms=%d", req.TraceID, signed.TransferID, mintDone.Sub(signDone).Milliseconds())
	if req.Timestamp > 0 {
		tmintMinusTlockUS := mintDone.UnixMicro() - req.Timestamp
		relayEvalLog("trace=%s transfer_id=%s stage=expA_tmint_minus_tlock duration_us=%d tlock_us=%d tmint_us=%d", req.TraceID, signed.TransferID, tmintMinusTlockUS, req.Timestamp, mintDone.UnixMicro())
	}

	receipt, receiptErr := w.enclave.BuildReceipt(ctx, sharedtypes.BuildReceiptRequest{
		TransferID:  signed.TransferID,
		TraceID:     req.TraceID,
		TxHash:      mintResp.TxHash,
		ChainID:     req.DstChainID,
		Amount:      req.Amount,
		Recipient:   req.Recipient,
		PayloadHash: toHex(signed.PayloadHash),
		FinalState:  "MINTED",
		SrcChainID:  req.SrcChainID,
		DstChainID:  req.DstChainID,
	})
	if receiptErr != nil {
		return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false, Reason: "build dst receipt failed"}, receiptErr
	}

	if w.audit != nil {
		if _, err := w.audit.SubmitTargetReceipt(ctx, chain.AuditReceiptRequest{TransferID: signed.TransferID, TraceID: req.TraceID, ReceiptHash: receipt.ReceiptHashHex}); err != nil {
			return sharedtypes.CrossChainExecutionResponse{TransferID: req.TransferID, TraceID: req.TraceID, Accepted: false, Reason: "submit dst receipt failed"}, err
		}
		relayEvalLog("trace=%s transfer_id=%s stage=audit_submit_dst duration_ms=%d", req.TraceID, signed.TransferID, time.Since(mintDone).Milliseconds())
	}

	relayEvalLog("trace=%s transfer_id=%s stage=target_flow_done total_ms=%d", req.TraceID, signed.TransferID, time.Since(flowStart).Milliseconds())
	return sharedtypes.CrossChainExecutionResponse{
		TransferID:      signed.TransferID,
		TraceID:         req.TraceID,
		Accepted:        true,
		TargetChainTx:   mintResp.TxHash,
		TargetReceipt:   receipt.ReceiptHashHex,
		TargetChainID:   req.DstChainID,
		TargetChainHash: toHex(signed.PayloadHash),
	}, nil
}
