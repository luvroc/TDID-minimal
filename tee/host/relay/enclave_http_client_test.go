package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sharedtypes "tdid-final/shared/types"
)

func TestHTTPEnclaveClient_VerifyPeerCrossMessage(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotMethod string
	var gotReq sharedtypes.CrossChainExecutionRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	client := NewHTTPEnclaveClient(srv.URL, srv.Client())
	err := client.VerifyPeerCrossMessage(context.Background(), sharedtypes.CrossChainExecutionRequest{
		TraceID:         "trace-1",
		TransferID:      "tid-1",
		RequestDigest:   "sha256:abc",
		SourceLockProof: "proof",
		SrcPayloadHash:  "0011",
		SrcSessSig:      "sig",
		SessionID:       "sess-1",
		KeyID:           "key-1",
		Asset:           "asset",
		Amount:          "1",
		Sender:          "alice",
		Recipient:       "bob",
		SrcChainID:      "source",
		DstChainID:      "target",
		Nonce:           7,
		Timestamp:       1,
	})
	if err != nil {
		t.Fatalf("VerifyPeerCrossMessage returned error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %s", gotMethod)
	}
	if gotPath != "/v1/tdid/evidence/verify-peer" {
		t.Fatalf("expected verify peer path, got %s", gotPath)
	}
	if gotReq.TraceID != "trace-1" || gotReq.TransferID != "tid-1" {
		t.Fatalf("unexpected request body: %+v", gotReq)
	}
}

func TestHTTPEnclaveClient_VerifyTargetExecutionEvidence_EnclaveReject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "target chain id mismatch",
		})
	}))
	defer srv.Close()

	client := NewHTTPEnclaveClient(srv.URL, srv.Client())
	err := client.VerifyTargetExecutionEvidence(context.Background(), sharedtypes.TargetExecutionEvidenceRequest{
		TraceID:       "trace-1",
		TransferID:    "tid-1",
		DstChainID:    "chain-b",
		TargetChainID: "chain-a",
	})
	if err == nil {
		t.Fatalf("expected error when enclave rejects evidence")
	}
	if !strings.Contains(err.Error(), "target chain id mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
