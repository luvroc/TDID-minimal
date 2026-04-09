package relay

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"tdid-final/host/chain"
	sharedtypes "tdid-final/shared/types"
)

type localPeerBridge struct {
	target *TargetWorker
}

func (b *localPeerBridge) SendExecution(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) (sharedtypes.CrossChainExecutionResponse, error) {
	return b.target.HandlePeerExecution(ctx, req)
}

func TestStage9_HostAtoBIntegrationFlow(t *testing.T) {
	enclaveA := &fakeEnclave{
		signed:  sharedtypes.SignedPayload{SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 7, ExpireAt: 12345, PayloadHash: []byte{0xaa}, SessSig: []byte{0xbb}},
		receipt: sharedtypes.BuildReceiptResponse{TransferID: "tx-1", TraceID: "0xtrace", ReceiptHashHex: "0xreceiptsrc"},
	}
	enclaveB := &fakeEnclave{
		mintSigned: sharedtypes.SignedPayload{SessionID: "sess-1", TransferID: "tx-1", KeyID: "0xkey", Nonce: 8, ExpireAt: 12345, PayloadHash: []byte{0xcc}, SessSig: []byte{0xdd}},
		receipt:    sharedtypes.BuildReceiptResponse{TransferID: "tx-1", TraceID: "0xtrace", ReceiptHashHex: "0xreceiptdst"},
	}
	sourceChain := &fakeSourceChain{lockResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xsrc"}}
	targetChain := &fakeTargetChain{mintResp: chain.ChainWriteResult{TransferID: "tx-1", TraceID: "0xtrace", TxHash: "0xdst"}}
	audit := &fakeAudit{}

	target := NewTargetWorker(enclaveB, targetChain, audit)
	source := NewSourceWorker(enclaveA, sourceChain, audit, &localPeerBridge{target: target})

	err := source.HandleSourceEvent(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:         "0xtrace",
		Asset:           "asset",
		Amount:          "10",
		Sender:          "alice",
		Recipient:       "bob",
		SrcChainID:      "fisco",
		DstChainID:      "fabric",
		KeyID:           "0xkey",
		Nonce:           7,
		ExpireAt:        12345,
		SourceLockProof: "proof-b64",
	})
	if err != nil {
		t.Fatalf("integration flow failed: %v", err)
	}

	if enclaveA.signLockCalls != 1 {
		t.Fatalf("expected source enclave SignLock once, got %d", enclaveA.signLockCalls)
	}
	if enclaveB.signMintCalls != 1 {
		t.Fatalf("expected target enclave SignMintOrUnlock once, got %d", enclaveB.signMintCalls)
	}
	if sourceChain.lockCalls != 1 || targetChain.mintCalls != 1 {
		t.Fatalf("expected lock/mint once, got lock=%d mint=%d", sourceChain.lockCalls, targetChain.mintCalls)
	}
	if audit.subSrc != 1 || audit.subDst != 1 || audit.match != 1 {
		t.Fatalf("expected receipt src/dst/match all once, got src=%d dst=%d match=%d", audit.subSrc, audit.subDst, audit.match)
	}
}

type a4MutexLedger struct {
	state map[string]string
}

func newA4MutexLedger() *a4MutexLedger {
	return &a4MutexLedger{state: map[string]string{}}
}

func (l *a4MutexLedger) lock(traceID string) error {
	if traceID == "" {
		return fmt.Errorf("traceID empty")
	}
	if _, exists := l.state[traceID]; exists {
		return fmt.Errorf("trace already exists")
	}
	l.state[traceID] = "LOCKED"
	return nil
}

func (l *a4MutexLedger) mint(traceID string) error {
	switch l.state[traceID] {
	case "LOCKED":
		l.state[traceID] = "COMMITTED"
		return nil
	case "REFUNDED":
		return fmt.Errorf("invalid state: REFUNDED")
	default:
		return fmt.Errorf("invalid state: %s", l.state[traceID])
	}
}

func (l *a4MutexLedger) refund(traceID string) error {
	switch l.state[traceID] {
	case "LOCKED":
		l.state[traceID] = "REFUNDED"
		return nil
	case "COMMITTED":
		return fmt.Errorf("invalid state: COMMITTED")
	default:
		return fmt.Errorf("invalid state: %s", l.state[traceID])
	}
}

type a4TargetChain struct {
	ledger *a4MutexLedger
}

func (f *a4TargetChain) InvokeTargetExecute(ctx context.Context, req chain.TargetExecuteRequest) (chain.ChainWriteResult, error) {
	_ = ctx
	if strings.Contains(strings.ToLower(req.SourceLockProof), "stale") {
		return chain.ChainWriteResult{}, fmt.Errorf("stale source lock proof")
	}
	if err := f.ledger.mint(req.TraceID); err != nil {
		return chain.ChainWriteResult{}, err
	}
	return chain.ChainWriteResult{TransferID: req.TransferID, TraceID: req.TraceID, TxHash: "0xa4dst"}, nil
}

func newA4TargetWorker(ledger *a4MutexLedger) *TargetWorker {
	enclave := &fakeEnclave{
		mintSigned: sharedtypes.SignedPayload{
			SessionID:  "sess-a4",
			TransferID: "tx-a4",
			KeyID:      "0xkey",
			Nonce:      9,
			ExpireAt:   12345,
			PayloadHash: []byte{
				0x44,
			},
			SessSig: []byte{0x55},
		},
		receipt: sharedtypes.BuildReceiptResponse{TransferID: "tx-a4", TraceID: "0xtrace-a4", ReceiptHashHex: "0xa4receipt"},
	}
	return NewTargetWorker(enclave, &a4TargetChain{ledger: ledger}, nil)
}

func buildA4PeerReq(traceID, proof string) sharedtypes.CrossChainExecutionRequest {
	return sharedtypes.CrossChainExecutionRequest{
		SessionID:       "sess-a4",
		TransferID:      "tx-a4",
		TraceID:         traceID,
		Asset:           "asset",
		Amount:          "10",
		Sender:          "alice",
		Recipient:       "bob",
		SrcChainID:      "fisco",
		DstChainID:      "fabric",
		SrcLockTx:       "0xsrc",
		SrcReceipt:      "0xsrc-receipt",
		SrcPayloadHash:  "0x11",
		SrcSessSig:      "0x22",
		SourceLockProof: proof,
		KeyID:           "0xkey",
		Nonce:           9,
		ExpireAt:        12345,
	}
}

func TestA4_MintThenRefundShouldFail(t *testing.T) {
	traceID := "0xtrace-a4-1"
	ledger := newA4MutexLedger()
	if err := ledger.lock(traceID); err != nil {
		t.Fatalf("pre-lock failed: %v", err)
	}

	target := newA4TargetWorker(ledger)
	resp, err := target.HandlePeerExecution(context.Background(), buildA4PeerReq(traceID, "proof-fresh"))
	if err != nil || !resp.Accepted {
		t.Fatalf("mint should succeed, err=%v accepted=%v", err, resp.Accepted)
	}

	if err := ledger.refund(traceID); err == nil || !strings.Contains(err.Error(), "COMMITTED") {
		t.Fatalf("refund after mint should fail with COMMITTED state, got err=%v", err)
	}
}

func TestA4_RefundThenMintShouldFail(t *testing.T) {
	traceID := "0xtrace-a4-2"
	ledger := newA4MutexLedger()
	if err := ledger.lock(traceID); err != nil {
		t.Fatalf("pre-lock failed: %v", err)
	}
	if err := ledger.refund(traceID); err != nil {
		t.Fatalf("pre-refund failed: %v", err)
	}

	target := newA4TargetWorker(ledger)
	resp, err := target.HandlePeerExecution(context.Background(), buildA4PeerReq(traceID, "proof-fresh"))
	if err == nil {
		t.Fatalf("mint after refund should fail")
	}
	if resp.Accepted {
		t.Fatalf("expected not accepted response")
	}
}

func TestA4_StaleLockedProofAfterRefundShouldFail(t *testing.T) {
	traceID := "0xtrace-a4-3"
	ledger := newA4MutexLedger()
	if err := ledger.lock(traceID); err != nil {
		t.Fatalf("pre-lock failed: %v", err)
	}
	if err := ledger.refund(traceID); err != nil {
		t.Fatalf("pre-refund failed: %v", err)
	}

	target := newA4TargetWorker(ledger)
	resp, err := target.HandlePeerExecution(context.Background(), buildA4PeerReq(traceID, "proof-stale"))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "stale") {
		t.Fatalf("stale locked proof after refund should fail, got err=%v", err)
	}
	if resp.Accepted {
		t.Fatalf("expected not accepted response")
	}
}
