package relay

import (
	"context"
	"errors"
	"testing"

	"tdid-final/host/chain"
	sharedtypes "tdid-final/shared/types"
)

type fakeEnclave struct {
	signed        sharedtypes.SignedPayload
	mintSigned    sharedtypes.SignedPayload
	receipt       sharedtypes.BuildReceiptResponse
	verifyErr     error
	evidenceErr   error
	evidenceCalls int
	signLockCalls int
	signMintCalls int
}

func (f *fakeEnclave) SignLock(ctx context.Context, req sharedtypes.SignLockRequest) (sharedtypes.SignedPayload, error) {
	_ = ctx
	_ = req
	f.signLockCalls++
	return f.signed, nil
}

func (f *fakeEnclave) SignMintOrUnlock(ctx context.Context, req sharedtypes.SignMintOrUnlockRequest) (sharedtypes.SignedPayload, error) {
	_ = ctx
	_ = req
	f.signMintCalls++
	return f.mintSigned, nil
}

func (f *fakeEnclave) VerifyPeerCrossMessage(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error {
	_ = ctx
	_ = req
	return f.verifyErr
}

func (f *fakeEnclave) BuildReceipt(ctx context.Context, req sharedtypes.BuildReceiptRequest) (sharedtypes.BuildReceiptResponse, error) {
	_ = ctx
	_ = req
	return f.receipt, nil
}

func (f *fakeEnclave) VerifyTargetExecutionEvidence(ctx context.Context, req sharedtypes.TargetExecutionEvidenceRequest) error {
	_ = ctx
	f.evidenceCalls++
	if f.evidenceErr != nil {
		return f.evidenceErr
	}
	if req.TargetTraceID != "" && req.TargetTraceID != req.TraceID {
		return errors.New("peer response trace mismatch")
	}
	if req.TargetTransferID != "" && req.TargetTransferID != req.TransferID {
		return errors.New("peer response transfer mismatch")
	}
	if req.TargetChainTx == "" || req.TargetReceipt == "" {
		return errors.New("peer response missing target commit evidence")
	}
	if req.TargetChainID == "" || req.TargetChainID != req.DstChainID {
		return errors.New("peer response target chain id mismatch")
	}
	if req.TargetChainHash == "" {
		return errors.New("peer response missing target chain hash")
	}
	return nil
}

type fakeSourceChain struct {
	lockCalls        int
	lockResp         chain.ChainWriteResult
	buildProofCalls  int
	encodeProofCalls int
	commitCalls      int
	lastCommitReq    chain.SourceCommitRequest
	commitResp       chain.ChainWriteResult
	proofJSON        string
	proofPayload     string
}

func (f *fakeSourceChain) InvokeSourceLock(ctx context.Context, req chain.SourceLockRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	_ = req
	f.lockCalls++
	return f.lockResp, nil
}

func (f *fakeSourceChain) BuildSourceLockProof(ctx context.Context, req chain.SourceLockProofRequest) (string, error) {
	_ = ctx
	_ = req
	f.buildProofCalls++
	if f.proofJSON != "" {
		return f.proofJSON, nil
	}
	return `{"traceId":"0xtrace","lockState":"LOCKED"}`, nil
}

func (f *fakeSourceChain) EncodeSourceLockProofPayload(ctx context.Context, req chain.SourceLockProofPayloadRequest) (string, error) {
	_ = ctx
	_ = req
	f.encodeProofCalls++
	if f.proofPayload != "" {
		return f.proofPayload, nil
	}
	return "proof-b64-auto", nil
}

func (f *fakeSourceChain) InvokeSourceCommit(ctx context.Context, req chain.SourceCommitRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	f.commitCalls++
	f.lastCommitReq = req
	if f.commitResp.TraceID == "" {
		return chain.ChainWriteResult{TraceID: req.TraceID, TxHash: "0xcommit"}, nil
	}
	return f.commitResp, nil
}

type fakeTargetChain struct {
	mintCalls int
	mintResp  chain.ChainWriteResult
}

func (f *fakeTargetChain) InvokeTargetExecute(ctx context.Context, req chain.TargetExecuteRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	_ = req
	f.mintCalls++
	return f.mintResp, nil
}

type fakeAudit struct {
	subSrc int
	subDst int
	match  int
}

func (f *fakeAudit) SubmitSourceReceipt(ctx context.Context, req chain.AuditReceiptRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	_ = req
	f.subSrc++
	return chain.ChainWriteResult{}, nil
}

func (f *fakeAudit) SubmitTargetReceipt(ctx context.Context, req chain.AuditReceiptRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	_ = req
	f.subDst++
	return chain.ChainWriteResult{}, nil
}

func (f *fakeAudit) MatchTransfer(ctx context.Context, req chain.AuditMatchRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	_ = req
	f.match++
	return chain.ChainWriteResult{}, nil
}

type fakePeer struct {
	calls int
	last  sharedtypes.CrossChainExecutionRequest
	resp  *sharedtypes.CrossChainExecutionResponse
}

func (f *fakePeer) SendExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	_ = ctx
	f.calls++
	f.last = req
	if f.resp != nil {
		return *f.resp, nil
	}
	return sharedtypes.CrossChainExecutionResponse{
		TransferID:      req.TransferID,
		TraceID:         req.TraceID,
		Accepted:        true,
		TargetChainTx:   "0xdst-target",
		TargetReceipt:   "0xreceipt-target",
		TargetChainID:   req.DstChainID,
		TargetChainHash: "0xtargethash",
	}, nil
}

func TestSourceWorker_HandleSourceEvent(t *testing.T) {
	enclave := &fakeEnclave{
		signed:  sharedtypes.SignedPayload{SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 7, ExpireAt: 12345, PayloadHash: []byte{0xaa}, SessSig: []byte{0xbb}},
		receipt: sharedtypes.BuildReceiptResponse{TransferID: "tx-1", TraceID: "0xtrace", ReceiptHashHex: "0xreceiptsrc"},
	}
	source := &fakeSourceChain{
		lockResp:   chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xsrc"},
		commitResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xcommit"},
	}
	audit := &fakeAudit{}
	peer := &fakePeer{}

	w := NewSourceWorker(enclave, source, audit, peer)
	err := w.HandleSourceEvent(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:    "0xtrace",
		Asset:      "asset",
		Amount:     "10",
		Sender:     "alice",
		Recipient:  "bob",
		SrcChainID: "fisco",
		DstChainID: "fabric",
		KeyID:      "0xkey",
		Nonce:      7,
		ExpireAt:   12345,
	})
	if err != nil {
		t.Fatalf("source worker failed: %v", err)
	}
	if enclave.signLockCalls != 1 || source.lockCalls != 1 || peer.calls != 1 {
		t.Fatalf("unexpected call counts: sign=%d lock=%d peer=%d", enclave.signLockCalls, source.lockCalls, peer.calls)
	}
	if peer.last.SrcLockTx != "0xsrc" {
		t.Fatalf("expected src lock tx forwarded, got %s", peer.last.SrcLockTx)
	}
	if peer.last.SessionID != "sess-1" || peer.last.TransferID != "tx-1" {
		t.Fatalf("expected derived ids forwarded, got session=%s transfer=%s", peer.last.SessionID, peer.last.TransferID)
	}
	if peer.last.SourceLockProof != "proof-b64-auto" {
		t.Fatalf("expected auto-generated proof payload forwarded, got %s", peer.last.SourceLockProof)
	}
	expectedDigest := deriveRequestDigest("tx-1", "0xtrace", "0xaa")
	if peer.last.RequestDigest != expectedDigest {
		t.Fatalf("expected requestDigest=%s, got %s", expectedDigest, peer.last.RequestDigest)
	}
	if source.buildProofCalls != 1 || source.encodeProofCalls != 1 {
		t.Fatalf("expected source proof build/encode once, got build=%d encode=%d", source.buildProofCalls, source.encodeProofCalls)
	}
	if source.commitCalls != 1 {
		t.Fatalf("expected source commit once, got %d", source.commitCalls)
	}
	if enclave.evidenceCalls != 1 {
		t.Fatalf("expected enclave evidence verify once, got %d", enclave.evidenceCalls)
	}
	if source.lastCommitReq.TargetChainTx != "0xdst-target" || source.lastCommitReq.TargetReceipt != "0xreceipt-target" {
		t.Fatalf("expected commit evidence from peer response, got tx=%s receipt=%s", source.lastCommitReq.TargetChainTx, source.lastCommitReq.TargetReceipt)
	}
	if source.lastCommitReq.TargetChainID != "fabric" || source.lastCommitReq.TargetChainHash != "0xtargethash" {
		t.Fatalf("expected commit chain binding from peer response, got chainID=%s chainHash=%s", source.lastCommitReq.TargetChainID, source.lastCommitReq.TargetChainHash)
	}
	if audit.subSrc != 1 || audit.match != 1 {
		t.Fatalf("expected src receipt submit and match, got subSrc=%d match=%d", audit.subSrc, audit.match)
	}
}

func TestSourceWorker_StopsWhenEnclaveEvidenceVerifyFails(t *testing.T) {
	enclave := &fakeEnclave{
		signed:      sharedtypes.SignedPayload{SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 7, ExpireAt: 12345, PayloadHash: []byte{0xaa}, SessSig: []byte{0xbb}},
		evidenceErr: errors.New("bad target evidence"),
	}
	source := &fakeSourceChain{
		lockResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xsrc"},
	}
	peer := &fakePeer{}
	w := NewSourceWorker(enclave, source, nil, peer)
	err := w.HandleSourceEvent(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:    "0xtrace",
		Asset:      "asset",
		Amount:     "10",
		Sender:     "alice",
		Recipient:  "bob",
		SrcChainID: "fisco",
		DstChainID: "fabric",
		KeyID:      "0xkey",
		Nonce:      7,
		ExpireAt:   12345,
	})
	if err == nil {
		t.Fatalf("expected evidence verify error")
	}
	if enclave.evidenceCalls != 1 {
		t.Fatalf("expected enclave evidence verify once, got %d", enclave.evidenceCalls)
	}
	if source.commitCalls != 0 {
		t.Fatalf("expected commit not invoked when evidence verify fails")
	}
}

func TestSourceWorker_RejectsPeerEvidenceWhenTargetChainBindingMissing(t *testing.T) {
	enclave := &fakeEnclave{
		signed: sharedtypes.SignedPayload{
			SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 7, ExpireAt: 12345, PayloadHash: []byte{0xaa}, SessSig: []byte{0xbb},
		},
	}
	source := &fakeSourceChain{
		lockResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xsrc"},
	}
	peer := &fakePeer{
		resp: &sharedtypes.CrossChainExecutionResponse{
			TransferID:    "tx-1",
			TraceID:       "0xtrace",
			Accepted:      true,
			TargetChainTx: "0xdst-target",
			TargetReceipt: "0xreceipt-target",
			TargetChainID: "",
		},
	}
	w := NewSourceWorker(enclave, source, nil, peer)
	err := w.HandleSourceEvent(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:    "0xtrace",
		Asset:      "asset",
		Amount:     "10",
		Sender:     "alice",
		Recipient:  "bob",
		SrcChainID: "fisco",
		DstChainID: "fabric",
		KeyID:      "0xkey",
		Nonce:      7,
		ExpireAt:   12345,
	})
	if err == nil {
		t.Fatalf("expected target chain binding validation error")
	}
	if source.commitCalls != 0 {
		t.Fatalf("expected commit not invoked on invalid peer evidence")
	}
}

func TestSourceWorker_RejectsPeerEvidenceWhenTraceMismatch(t *testing.T) {
	enclave := &fakeEnclave{
		signed: sharedtypes.SignedPayload{
			SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 7, ExpireAt: 12345, PayloadHash: []byte{0xaa}, SessSig: []byte{0xbb},
		},
	}
	source := &fakeSourceChain{
		lockResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xsrc"},
	}
	peer := &fakePeer{
		resp: &sharedtypes.CrossChainExecutionResponse{
			TransferID:      "tx-1",
			TraceID:         "0xtrace-wrong",
			Accepted:        true,
			TargetChainTx:   "0xdst-target",
			TargetReceipt:   "0xreceipt-target",
			TargetChainID:   "fabric",
			TargetChainHash: "0xtargethash",
		},
	}
	w := NewSourceWorker(enclave, source, nil, peer)
	err := w.HandleSourceEvent(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:    "0xtrace",
		Asset:      "asset",
		Amount:     "10",
		Sender:     "alice",
		Recipient:  "bob",
		SrcChainID: "fisco",
		DstChainID: "fabric",
		KeyID:      "0xkey",
		Nonce:      7,
		ExpireAt:   12345,
	})
	if err == nil {
		t.Fatalf("expected trace mismatch validation error")
	}
	if source.commitCalls != 0 {
		t.Fatalf("expected commit not invoked on mismatched trace")
	}
}

func TestTargetWorker_HandlePeerExecution(t *testing.T) {
	enclave := &fakeEnclave{
		mintSigned: sharedtypes.SignedPayload{SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 8, ExpireAt: 12345, PayloadHash: []byte{0xcc}, SessSig: []byte{0xdd}},
		receipt:    sharedtypes.BuildReceiptResponse{TransferID: "tx-1", TraceID: "0xtrace", ReceiptHashHex: "0xreceiptdst"},
	}
	target := &fakeTargetChain{mintResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xdst"}}
	audit := &fakeAudit{}

	w := NewTargetWorker(enclave, target, audit)
	resp, err := w.HandlePeerExecution(context.Background(), sharedtypes.CrossChainExecutionRequest{
		SessionID:       "sess-1",
		TransferID:      "tx-1",
		TraceID:         "0xtrace",
		Asset:           "asset",
		Amount:          "10",
		Sender:          "alice",
		Recipient:       "bob",
		SrcChainID:      "fisco",
		DstChainID:      "fabric",
		SrcLockTx:       "0xsrc",
		SrcReceipt:      "0xreceiptsrc",
		SrcPayloadHash:  "0x11",
		SrcSessSig:      "0x22",
		SourceLockProof: "proof-b64",
		KeyID:           "0xkey",
		Nonce:           8,
		ExpireAt:        12345,
	})
	if err != nil {
		t.Fatalf("target worker failed: %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("expected accepted response")
	}
	if enclave.signMintCalls != 1 || target.mintCalls != 1 || audit.subDst != 1 {
		t.Fatalf("unexpected call counts: signMint=%d mint=%d subDst=%d", enclave.signMintCalls, target.mintCalls, audit.subDst)
	}
	if resp.TargetChainTx != "0xdst" || resp.TargetReceipt != "0xreceiptdst" {
		t.Fatalf("expected target evidence in response, got tx=%s receipt=%s", resp.TargetChainTx, resp.TargetReceipt)
	}
	if resp.TargetChainID != "fabric" || resp.TargetChainHash == "" {
		t.Fatalf("expected target chain binding in response, got chainID=%s chainHash=%s", resp.TargetChainID, resp.TargetChainHash)
	}
}

func TestTargetWorker_VerifyFailureStopsMint(t *testing.T) {
	enclave := &fakeEnclave{
		verifyErr: errors.New("verify failed"),
	}
	target := &fakeTargetChain{}
	w := NewTargetWorker(enclave, target, nil)
	_, err := w.HandlePeerExecution(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID: "0xtrace",
	})
	if err == nil {
		t.Fatalf("expected verify error")
	}
	if enclave.signMintCalls != 0 {
		t.Fatalf("expected sign mint not called when verify fails")
	}
	if target.mintCalls != 0 {
		t.Fatalf("expected target execute not called when verify fails")
	}
}
