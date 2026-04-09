package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	sharedtypes "tdid-final/shared/types"
)

func TestStage9_RestartRecoveryAndNonceContinuity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sealKey := []byte("stage9-restart-seal-key-123")

	statePathA := filepath.Join(dir, "tee-a", "state.sealed")
	statePathB := filepath.Join(dir, "tee-b", "state.sealed")

	svcA, err := NewService(Config{StatePath: statePathA, SealKey: sealKey, NodeID: "tee-a", Role: "source", PeerAllowList: []string{"tee-b"}})
	if err != nil {
		t.Fatalf("new service A failed: %v", err)
	}
	svcB, err := NewService(Config{StatePath: statePathB, SealKey: sealKey, NodeID: "tee-b", Role: "target", PeerAllowList: []string{"tee-a"}})
	if err != nil {
		t.Fatalf("new service B failed: %v", err)
	}
	if err := svcA.InitNode(ctx); err != nil {
		t.Fatalf("init A failed: %v", err)
	}
	if err := svcB.InitNode(ctx); err != nil {
		t.Fatalf("init B failed: %v", err)
	}

	expireAt := time.Now().Add(10 * time.Minute).UnixMilli()
	traceA1 := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	traceB1 := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	traceA2 := "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	traceAReplay := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	traceB2 := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	bindA, err := svcA.BindSession(ctx, sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFISCO,
		ChainID:      "fisco",
		ContractAddr: "0x32b93e0117ddc9cd5b6abf166c97b6a78294bc97",
		ExpireAt:     expireAt,
		RatchetSeed:  []byte("stage9-ratchet-a"),
	})
	if err != nil {
		t.Fatalf("bind session A failed: %v", err)
	}
	bindB, err := svcB.BindSession(ctx, sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFabric,
		ChainID:      "mychannel",
		ContractAddr: "gatewaycc",
		ExpireAt:     expireAt,
		RatchetSeed:  []byte("stage9-ratchet-b"),
	})
	if err != nil {
		t.Fatalf("bind session B failed: %v", err)
	}

	if _, err := svcA.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      sharedtypes.ChainFISCO,
		TraceID:    traceA1,
		SrcChainID: "fisco",
		DstChainID: "fabric",
		Asset:      "USDT",
		Amount:     "10",
		Sender:     "alice",
		Recipient:  "bob",
		KeyID:      bindA.KeyID,
		Nonce:      1,
		ExpireAt:   expireAt,
	}); err != nil {
		t.Fatalf("sign lock A nonce=1 failed: %v", err)
	}
	if _, err := svcB.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      sharedtypes.ChainFabric,
		TraceID:    traceB1,
		SrcChainID: "fabric",
		DstChainID: "fisco",
		Asset:      "USDT",
		Amount:     "20",
		Sender:     "carol",
		Recipient:  "dave",
		KeyID:      bindB.KeyID,
		Nonce:      1,
		ExpireAt:   expireAt,
	}); err != nil {
		t.Fatalf("sign lock B nonce=1 failed: %v", err)
	}

	// restart A only
	svcARestart, err := NewService(Config{StatePath: statePathA, SealKey: sealKey, NodeID: "tee-a", Role: "source", PeerAllowList: []string{"tee-b"}})
	if err != nil {
		t.Fatalf("new service A restart failed: %v", err)
	}
	if err := svcARestart.InitNode(ctx); err != nil {
		t.Fatalf("init A restart failed: %v", err)
	}

	if _, err := svcARestart.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      sharedtypes.ChainFISCO,
		TraceID:    traceA2,
		SrcChainID: "fisco",
		DstChainID: "fabric",
		Asset:      "USDT",
		Amount:     "11",
		Sender:     "alice",
		Recipient:  "bob",
		KeyID:      bindA.KeyID,
		Nonce:      2,
		ExpireAt:   expireAt,
	}); err != nil {
		t.Fatalf("sign lock A nonce continuity failed after restart: %v", err)
	}

	if _, err := svcARestart.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      sharedtypes.ChainFISCO,
		TraceID:    traceAReplay,
		SrcChainID: "fisco",
		DstChainID: "fabric",
		Asset:      "USDT",
		Amount:     "11",
		Sender:     "alice",
		Recipient:  "bob",
		KeyID:      bindA.KeyID,
		Nonce:      2,
		ExpireAt:   expireAt,
	}); err == nil {
		t.Fatalf("expected nonce replay to be rejected after restart")
	}

	if _, err := svcB.SignLock(ctx, sharedtypes.SignLockRequest{
		Chain:      sharedtypes.ChainFabric,
		TraceID:    traceB2,
		SrcChainID: "fabric",
		DstChainID: "fisco",
		Asset:      "USDT",
		Amount:     "21",
		Sender:     "carol",
		Recipient:  "dave",
		KeyID:      bindB.KeyID,
		Nonce:      2,
		ExpireAt:   expireAt,
	}); err != nil {
		t.Fatalf("B state should remain isolated and continuous, got: %v", err)
	}
}
