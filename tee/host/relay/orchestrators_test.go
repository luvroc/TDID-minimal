package relay

import (
	"context"
	"os"
	"testing"

	"tdid-final/host/chain"
	sharedtypes "tdid-final/shared/types"
)

type fakeRegistry struct {
	registerNodeCalls    int
	registerSessionCalls int
	lastSessionPayload   map[string]any
}

func (f *fakeRegistry) RegisterNode(ctx context.Context, payload map[string]any) (chain.ChainWriteResult, error) {
	_ = ctx
	f.registerNodeCalls++
	return chain.ChainWriteResult{Data: nil}, nil
}

func (f *fakeRegistry) RegisterSession(ctx context.Context, payload map[string]any) (chain.ChainWriteResult, error) {
	_ = ctx
	f.registerSessionCalls++
	f.lastSessionPayload = payload
	return chain.ChainWriteResult{SessionID: payload["sessionId"].(string), TransferID: payload["transferId"].(string), TraceID: payload["traceId"].(string)}, nil
}

func (f *fakeEnclave) GetNodeIdentity(ctx context.Context) (sharedtypes.NodeIdentity, error) {
	_ = ctx
	return sharedtypes.NodeIdentity{PublicKey: []byte{0x1}, Address: "0xnode"}, nil
}

func (f *fakeEnclave) BindSession(ctx context.Context, req sharedtypes.BindSessionRequest) (sharedtypes.BindSessionResponse, error) {
	_ = ctx
	_ = req
	return sharedtypes.BindSessionResponse{SessionID: "sess-1", KeyID: "0xkey", ChainID: req.ChainID, ExpireAt: req.ExpireAt}, nil
}

func (f *fakeEnclave) CurrentSession(ctx context.Context) (*sharedtypes.CurrentSessionResponse, error) {
	_ = ctx
	return &sharedtypes.CurrentSessionResponse{SessionID: "sess-1", KeyID: "0xkey"}, nil
}

func (f *fakeEnclave) SignRefundV2(ctx context.Context, req sharedtypes.SignRefundV2Request) (sharedtypes.SignedPayload, error) {
	_ = ctx
	_ = req
	return sharedtypes.SignedPayload{SessionID: req.SessionID, TransferID: req.TransferID, KeyID: req.KeyID, Nonce: req.Nonce, ExpireAt: req.ExpireAt}, nil
}

func TestIntentOrchestrator_HandleIntent(t *testing.T) {
	enclave := &fakeEnclave{}
	registry := &fakeRegistry{}
	orchestrator := NewIntentOrchestrator(enclave, registry)

	result, err := orchestrator.HandleIntent(context.Background(), IntentRequest{TraceID: "trace-1", SrcChainID: "mychannel", DstChainID: "group0", ContractAddr: "gatewaycc", Asset: "USDT", Amount: "10", Sender: "alice", Recipient: "bob", ExpireAt: 12345, RatchetSeed: []byte("seed")})
	if err != nil {
		t.Fatalf("intent orchestrator failed: %v", err)
	}
	if result.Binding.SessionID == "" || result.TransferID == "" {
		t.Fatalf("expected session and transfer ids, got %+v", result)
	}
	if registry.registerSessionCalls != 1 {
		t.Fatalf("expected one session registration call, got %d", registry.registerSessionCalls)
	}
}

func TestRecoveryOrchestrator_HandleRecovery(t *testing.T) {
	enclave := &fakeEnclave{}
	orchestrator := NewRecoveryOrchestrator(enclave)

	result, err := orchestrator.HandleRecovery(context.Background(), RecoveryRequest{SessionID: "sess-1", TransferID: "tx-1", TraceID: "trace-1", Chain: sharedtypes.ChainFabric, KeyID: "0xkey", Nonce: 2, ExpireAt: 12345})
	if err != nil {
		t.Fatalf("recovery orchestrator failed: %v", err)
	}
	if result.TransferID != "tx-1" {
		t.Fatalf("expected transfer id to propagate, got %+v", result)
	}
}

func TestIntentOrchestrator_HandleIntent_NoSessionBaseline(t *testing.T) {
	if err := os.Setenv("TDID_T3_NO_SESSION_BASELINE", "1"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	defer os.Unsetenv("TDID_T3_NO_SESSION_BASELINE")

	enclave := &fakeEnclave{}
	registry := &fakeRegistry{}
	orchestrator := NewIntentOrchestrator(enclave, registry)

	result, err := orchestrator.HandleIntent(context.Background(), IntentRequest{
		TraceID: "trace-nosess", SrcChainID: "mychannel", DstChainID: "group0",
		ContractAddr: "gatewaycc", Asset: "USDT", Amount: "10", Sender: "alice", Recipient: "bob", ExpireAt: 12345,
	})
	if err != nil {
		t.Fatalf("intent orchestrator no-session failed: %v", err)
	}
	if registry.registerSessionCalls != 0 {
		t.Fatalf("expected no session registration, got %d", registry.registerSessionCalls)
	}
	if result.Binding.SessionID == "" || result.Binding.KeyID == "" {
		t.Fatalf("expected synthetic binding in no-session mode, got %+v", result.Binding)
	}
}

func TestIntentOrchestrator_HandleIntent_NoIDBindingBaseline(t *testing.T) {
	if err := os.Setenv("TDID_T6_NO_ID_BINDING", "1"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	defer os.Unsetenv("TDID_T6_NO_ID_BINDING")

	registry := &fakeRegistry{}
	enclave := &fakeEnclave{}
	orchestrator := NewIntentOrchestrator(enclave, registry)
	result, err := orchestrator.HandleIntent(context.Background(), IntentRequest{
		TraceID: "trace-nobind-1", SrcChainID: "fabric", DstChainID: "fisco", ContractAddr: "0x1234",
		Asset: "USDT", Amount: "10", Sender: "alice", Recipient: "bob", ExpireAt: 9999,
	})
	if err != nil {
		t.Fatalf("intent orchestrator no-id-binding failed: %v", err)
	}
	if result.Binding.SessionID == "" || result.TransferID == "" {
		t.Fatalf("expected session and transfer ids, got %+v", result)
	}
	if registry.registerSessionCalls != 0 {
		t.Fatalf("expected no session registration in no-id-binding mode, got %d", registry.registerSessionCalls)
	}
}
