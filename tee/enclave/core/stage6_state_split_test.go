package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	sharedtypes "tdid-final/shared/types"
	"tdid-final/tee"
)

func TestStage6_InitNodePersistsNodeRoleAndAllowList(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "tee-a-state.sealed")
	sealKey := []byte("stage6-test-seal-key-123")

	svc, err := NewService(Config{
		StatePath:     statePath,
		SealKey:       sealKey,
		NodeID:        "tee-a-node",
		Role:          "source",
		PeerAllowList: []string{"tee-b-node", "tee-b-node"},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	store, err := tee.NewFileSealedStateStore(statePath, sealKey)
	if err != nil {
		t.Fatalf("open store failed: %v", err)
	}
	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state failed: %v", err)
	}
	if state.NodeID != "tee-a-node" {
		t.Fatalf("unexpected node id: %s", state.NodeID)
	}
	if state.Role != "source" {
		t.Fatalf("unexpected role: %s", state.Role)
	}
	if len(state.PeerAllowList) != 1 || state.PeerAllowList[0] != "tee-b-node" {
		t.Fatalf("unexpected peer allow list: %+v", state.PeerAllowList)
	}
	if state.UsedNonceWindow == nil {
		t.Fatalf("usedNonceWindow should be initialized")
	}
}

func TestStage6_SealedStateIsolatedBetweenTeeAAndTeeB(t *testing.T) {
	dir := t.TempDir()
	sealKey := []byte("stage6-test-seal-key-456")
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
	if err := svcA.InitNode(context.Background()); err != nil {
		t.Fatalf("init A failed: %v", err)
	}
	if err := svcB.InitNode(context.Background()); err != nil {
		t.Fatalf("init B failed: %v", err)
	}

	_, err = svcA.BindSession(context.Background(), sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFISCO,
		ChainID:      "fisco",
		ContractAddr: "0x32b93e0117ddc9cd5b6abf166c97b6a78294bc97",
		ExpireAt:     time.Now().Add(10 * time.Minute).UnixMilli(),
		RatchetSeed:  []byte("ratchet-seed-stage6"),
	})
	if err != nil {
		t.Fatalf("bind session on A failed: %v", err)
	}

	sA, err := svcA.CurrentSession(context.Background())
	if err != nil {
		t.Fatalf("current session A failed: %v", err)
	}
	if sA == nil {
		t.Fatalf("expected A has active session")
	}
	sB, err := svcB.CurrentSession(context.Background())
	if err != nil {
		t.Fatalf("current session B failed: %v", err)
	}
	if sB != nil {
		t.Fatalf("expected B session isolated and empty, got %+v", sB)
	}

	storeA, _ := tee.NewFileSealedStateStore(statePathA, sealKey)
	storeB, _ := tee.NewFileSealedStateStore(statePathB, sealKey)
	stateA, err := storeA.Load(context.Background())
	if err != nil {
		t.Fatalf("load state A failed: %v", err)
	}
	stateB, err := storeB.Load(context.Background())
	if err != nil {
		t.Fatalf("load state B failed: %v", err)
	}
	if stateA.Role != "source" || stateB.Role != "target" {
		t.Fatalf("role split mismatch: A=%s B=%s", stateA.Role, stateB.Role)
	}
	if stateA.NodeID == stateB.NodeID {
		t.Fatalf("expected distinct node ids, both got %s", stateA.NodeID)
	}
}
