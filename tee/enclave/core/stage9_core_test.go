package core

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	sharedtypes "tdid-final/shared/types"
)

func TestStage9_VerifyPeerCrossMessageRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-verify-seal-key-123"),
		NodeID:    "tee-a",
		Role:      "source",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	err = svc.VerifyPeerCrossMessage(context.Background(), sharedtypes.CrossChainExecutionRequest{})
	if err == nil {
		t.Fatalf("expected invalid input error")
	}
}

func TestStage9_VerifyPeerCrossMessageAcceptsValidSignedRequest(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath:     filepath.Join(dir, "state.sealed"),
		SealKey:       []byte("stage9-core-peerok-seal-key-123"),
		NodeID:        "tee-b",
		Role:          "target",
		PeerAllowList: []string{"addr:" + peerAddr},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("expected valid peer request, got %v", err)
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-peerdigest-seal-key"),
		NodeID:    "tee-b",
		Role:      "target",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.RequestDigest = "sha256:deadbeef"
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected digest mismatch error")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsAllowlistMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath:     filepath.Join(dir, "state.sealed"),
		SealKey:       []byte("stage9-core-peerallow-seal-key-1"),
		NodeID:        "tee-b",
		Role:          "target",
		PeerAllowList: []string{"addr:0x0000000000000000000000000000000000000001"},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected allowlist reject")
	}
}

func TestStage9_VerifyPeerCrossMessageAcceptsAllowlistMismatchWhenNoIDBindingEnabled(t *testing.T) {
	if err := os.Setenv("TDID_T6_NO_ID_BINDING", "1"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	defer os.Unsetenv("TDID_T6_NO_ID_BINDING")

	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath:     filepath.Join(dir, "state.sealed"),
		SealKey:       []byte("stage9-core-peerallow-seal-key-2"),
		NodeID:        "tee-b",
		Role:          "target",
		PeerAllowList: []string{"addr:0x0000000000000000000000000000000000000001"},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("expected allowlist mismatch ignored in no-id-binding mode, got %v", err)
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsSessionMismatchWithBoundSession(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-peersession-seal"),
		NodeID:    "tee-b",
		Role:      "target",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	bind, err := svc.BindSession(context.Background(), sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFISCO,
		ChainID:      "fisco",
		ContractAddr: "0x32b93e0117ddc9cd5b6abf166c97b6a78294bc97",
		ExpireAt:     time.Now().Add(2 * time.Minute).UnixMilli(),
		RatchetSeed:  []byte("stage9-peer-session-seed"),
	})
	if err != nil {
		t.Fatalf("bind session failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.KeyID = bind.KeyID
	req.SessionID = "session-not-current"
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected session mismatch error")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsReplayNonce(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-peerreplay-seal"),
		NodeID:    "tee-b",
		Role:      "target",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	bind, err := svc.BindSession(context.Background(), sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFISCO,
		ChainID:      "fisco",
		ContractAddr: "0x32b93e0117ddc9cd5b6abf166c97b6a78294bc97",
		ExpireAt:     time.Now().Add(2 * time.Minute).UnixMilli(),
		RatchetSeed:  []byte("stage9-peer-replay-seed"),
	})
	if err != nil {
		t.Fatalf("bind session failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.KeyID = bind.KeyID
	req.SessionID = bind.SessionID
	req.Nonce = 9
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("first verify failed: %v", err)
	}
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected replay nonce error")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsMissingCapabilityToken(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-capability-seal-1"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"cap:mint_or_unlock",
			"vc:tdid_bridge_v1",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = "proof-b64-without-vc"
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected missing capability token error")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsMissingCapabilityTokenEvenWhenNoIDBindingEnabled(t *testing.T) {
	if err := os.Setenv("TDID_T6_NO_ID_BINDING", "1"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	defer os.Unsetenv("TDID_T6_NO_ID_BINDING")

	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-capability-seal-3"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:0x0000000000000000000000000000000000000001",
			"cap:mint_or_unlock",
			"vc:tdid_bridge_v1",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = "proof-b64-without-vc"
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected missing capability token error in no-id-binding mode")
	}
}

func TestStage9_VerifyPeerCrossMessageAcceptsCapabilityTokensWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-capability-seal-2"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"cap:mint_or_unlock",
			"vc:tdid_bridge_v1",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = "proof-b64-with-cap=mint_or_unlock;vc=tdid_bridge_v1"
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("expected capability tokens accepted, got %v", err)
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsVCAttesterMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-attester-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:attester:expected-attester",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = makeProofPayloadHex("actual-attester", "proof-signer")
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected vc attester mismatch")
	}
}

func TestStage9_VerifyPeerCrossMessageAcceptsStructuredVCRules(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-structured-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:attester:expected-attester",
			"vc:signer:proof-signer",
			"cap:proofsig",
			"cap:proofdigest",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = makeProofPayloadHex("expected-attester", "proof-signer")
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("expected structured vc/cap checks pass, got %v", err)
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsVCSubjectMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-subject-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:subject:carol",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected vc subject mismatch")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsVCActionMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-action-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:action:refund_v2",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected vc action mismatch")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsVCResourceMismatch(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-resource-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:resource:eth@fisco",
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected vc resource mismatch")
	}
}

func TestStage9_VerifyPeerCrossMessageRejectsVCIssuerAndTimeWindowViolation(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	nowMillis := time.Now().UnixMilli()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-issuer-time-seal"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:issuer:issuer-a",
			"vc:notbefore:" + strconv.FormatInt(nowMillis+60000, 10),
			"vc:expireat:" + strconv.FormatInt(nowMillis+120000, 10),
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = makeProofPayloadHex("issuer-b", "proof-signer")
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err == nil {
		t.Fatalf("expected vc issuer/time violation")
	}
}

func TestStage9_VerifyPeerCrossMessageAcceptsExtendedVCRules(t *testing.T) {
	dir := t.TempDir()
	peerKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	peerAddr := crypto.PubkeyToAddress(peerKey.PublicKey).Hex()
	nowMillis := time.Now().UnixMilli()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-vc-extended-pass"),
		NodeID:    "tee-b",
		Role:      "target",
		PeerAllowList: []string{
			"addr:" + peerAddr,
			"vc:subject:alice",
			"vc:action:mint_or_unlock",
			"vc:resource:usdt@fisco",
			"vc:issuer:issuer-a",
			"vc:notbefore:" + strconv.FormatInt(nowMillis-60000, 10),
			"vc:expireat:" + strconv.FormatInt(nowMillis+120000, 10),
		},
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}
	req := makeSignedPeerReq(t, peerKey)
	req.SourceLockProof = makeProofPayloadHex("issuer-a", "proof-signer")
	if err := svc.VerifyPeerCrossMessage(context.Background(), req); err != nil {
		t.Fatalf("expected extended vc rules pass, got %v", err)
	}
}

func TestStage9_BuildReceiptWorks(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-receipt-seal-key-123"),
		NodeID:    "tee-a",
		Role:      "source",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	resp, err := svc.BuildReceipt(context.Background(), sharedtypes.BuildReceiptRequest{
		TraceID:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TxHash:      "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ChainID:     "fisco",
		Amount:      "100",
		Recipient:   "alice",
		PayloadHash: "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		FinalState:  "LOCKED",
		SrcChainID:  "fisco",
		DstChainID:  "fabric",
	})
	if err != nil {
		t.Fatalf("build receipt failed: %v", err)
	}
	if resp.TraceID == "" || len(resp.ReceiptHash) == 0 || resp.ReceiptHashHex == "" {
		t.Fatalf("invalid receipt response: %+v", resp)
	}
}

func TestStage9_SignRefundV2FailsWhenSessionExpired(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Config{
		StatePath: filepath.Join(dir, "state.sealed"),
		SealKey:   []byte("stage9-core-expire-seal-key-123"),
		NodeID:    "tee-a",
		Role:      "source",
	})
	if err != nil {
		t.Fatalf("new service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		t.Fatalf("init node failed: %v", err)
	}

	bind, err := svc.BindSession(context.Background(), sharedtypes.BindSessionRequest{
		Chain:        sharedtypes.ChainFISCO,
		ChainID:      "fisco",
		ContractAddr: "0x32b93e0117ddc9cd5b6abf166c97b6a78294bc97",
		ExpireAt:     time.Now().Add(500 * time.Millisecond).UnixMilli(),
		RatchetSeed:  []byte("stage9-expire-seed"),
	})
	if err != nil {
		t.Fatalf("bind session failed: %v", err)
	}
	time.Sleep(700 * time.Millisecond)

	_, err = svc.SignRefundV2(context.Background(), sharedtypes.SignRefundV2Request{
		Chain:    sharedtypes.ChainFISCO,
		TraceID:  "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		KeyID:    bind.KeyID,
		Nonce:    1,
		ExpireAt: time.Now().Add(1 * time.Minute).UnixMilli(),
	})
	if err == nil {
		t.Fatalf("expected session expired error for SignRefundV2")
	}
}

func makeSignedPeerReq(t *testing.T, key *ecdsa.PrivateKey) sharedtypes.CrossChainExecutionRequest {
	t.Helper()
	payloadHash := crypto.Keccak256([]byte("stage9-peer-message"))
	sig, err := crypto.Sign(payloadHash, key)
	if err != nil {
		t.Fatalf("sign payload failed: %v", err)
	}
	srcPayloadHex := "0x" + hex.EncodeToString(payloadHash)
	srcSigHex := "0x" + hex.EncodeToString(sig)
	transferID := "tx-stage9-1"
	traceID := "0xtrace-stage9-1"
	return sharedtypes.CrossChainExecutionRequest{
		SessionID:       "session-stage9-1",
		TransferID:      transferID,
		RequestDigest:   derivePeerRequestDigest(transferID, traceID, srcPayloadHex),
		TraceID:         traceID,
		Asset:           "USDT",
		Amount:          "10",
		Sender:          "alice",
		Recipient:       "bob",
		SrcChainID:      "fabric",
		DstChainID:      "fisco",
		KeyID:           "0xkey-stage9-1",
		Nonce:           1,
		ExpireAt:        time.Now().Add(1 * time.Minute).UnixMilli(),
		SrcLockTx:       "0xsrc",
		SrcReceipt:      "0xreceipt",
		SrcPayloadHash:  srcPayloadHex,
		SrcSessSig:      srcSigHex,
		SourceLockProof: "proof-b64",
		Timestamp:       time.Now().UnixMicro(),
	}
}

func makeProofPayloadHex(attester string, signer string) string {
	toWord := func(v string) string {
		if v == "" {
			return strings.Repeat("0", 64)
		}
		sum := crypto.Keccak256([]byte(strings.ToLower(strings.TrimSpace(v))))
		return hex.EncodeToString(sum)
	}
	words := []string{
		strings.Repeat("1", 64), // trace
		strings.Repeat("2", 64), // transfer
		strings.Repeat("3", 64), // session
		strings.Repeat("4", 64), // srcChain
		strings.Repeat("5", 64), // lockState
		strings.Repeat("0", 63) + "1",
		strings.Repeat("6", 64), // tx
		strings.Repeat("7", 64), // event
		strings.Repeat("0", 63) + "2",
		toWord(attester),
		toWord(signer),
		strings.Repeat("8", 64), // proofDigest
		strings.Repeat("9", 64), // sigR
		strings.Repeat("a", 64), // sigS
		strings.Repeat("0", 63) + "1",
	}
	return "0x" + strings.Join(words, "")
}
