package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tdid-final/enclave/core"
	sharedtypes "tdid-final/shared/types"
	"tdid-final/tee"
)

type app struct {
	svc       core.Service
	remoteCtx *tee.RemoteChainContext
	remoteErr string
}

type sessionSignRequest struct {
	Action     string                `json:"action"`
	Chain      sharedtypes.ChainKind `json:"chain"`
	TraceID    string                `json:"traceId"`
	SrcChainID string                `json:"srcChainId"`
	DstChainID string                `json:"dstChainId"`
	Asset      string                `json:"asset"`
	Amount     string                `json:"amount"`
	Sender     string                `json:"sender"`
	Recipient  string                `json:"recipient"`
	KeyID      string                `json:"keyId"`
	Nonce      uint64                `json:"nonce"`
	ExpireAt   int64                 `json:"expireAt"`
}

type nodeStatusResponse struct {
	OK            bool                                `json:"ok"`
	NodeIdentity  sharedtypes.NodeIdentity            `json:"nodeIdentity"`
	Session       *sharedtypes.CurrentSessionResponse `json:"session,omitempty"`
	RemoteContext tee.RemoteChainContextSummary       `json:"remoteContext"`
}

func main() {
	addr := envOrDefault("TEE_ENCLAVE_ADDR", ":18080")
	statePath := os.Getenv("TEE_ENCLAVE_STATE_PATH")
	role := os.Getenv("TEE_NODE_ROLE")
	nodeID := os.Getenv("TEE_NODE_ID")
	peerAllow := splitCSVEnv("TEE_PEER_ALLOWLIST")
	sealKey := envOrDefault("TEE_ENCLAVE_SEAL_KEY", "change-me-stage2-seal-key")

	svc, err := core.NewService(core.Config{StatePath: statePath, SealKey: []byte(sealKey), NodeID: nodeID, Role: role, PeerAllowList: peerAllow})
	if err != nil {
		log.Fatalf("init enclave service failed: %v", err)
	}
	if err := svc.InitNode(context.Background()); err != nil {
		log.Fatalf("init node identity failed: %v", err)
	}
	node, err := svc.GetNodeIdentity(context.Background())
	if err != nil {
		log.Fatalf("read node identity failed: %v", err)
	}

	remoteCtx, remoteErr := loadRemoteContext(context.Background())
	if remoteErr != nil {
		log.Printf("remote chain context unavailable: %v", remoteErr)
	} else {
		log.Printf("remote chain context loaded from %s", remoteCtx.StateConfigURL)
	}

	application := &app{svc: svc, remoteCtx: remoteCtx}
	if remoteErr != nil {
		application.remoteErr = remoteErr.Error()
	}

	log.Printf("tee enclave service ready on %s, node=%s", addr, node.Address)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", application.handleHealth)
	mux.HandleFunc("/v1/tdid/node/init", application.handleNodeInit)
	mux.HandleFunc("/v1/tdid/node/status", application.handleNodeStatus)
	mux.HandleFunc("/v1/tdid/context/status", application.handleContextStatus)
	mux.HandleFunc("/v1/tdid/session/bind", application.handleSessionBind)
	mux.HandleFunc("/v1/tdid/session/sign", application.handleSessionSign)
	mux.HandleFunc("/v1/tdid/receipt/hash", application.handleReceiptHash)
	mux.HandleFunc("/v1/tdid/evidence/verify-peer", application.handleVerifyPeerEvidence)
	mux.HandleFunc("/v1/tdid/evidence/verify-target", application.handleVerifyTargetEvidence)

	httpServer := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case serveErr := <-errCh:
		log.Fatalf("http server failed: %v", serveErr)
	case sig := <-sigCh:
		log.Printf("received signal: %s", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "tee-enclave-service"})
}

func (a *app) handleNodeInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if err := a.svc.InitNode(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.handleNodeStatus(w, r)
}

func (a *app) handleNodeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	node, err := a.svc.GetNodeIdentity(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	session, err := a.svc.CurrentSession(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, nodeStatusResponse{OK: true, NodeIdentity: node, Session: session, RemoteContext: a.remoteSummary()})
}

func (a *app) handleContextStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	body := map[string]any{"ok": true, "remoteContext": a.remoteSummary()}
	if a.remoteErr != "" {
		body["warning"] = a.remoteErr
	}
	writeJSON(w, http.StatusOK, body)
}

func (a *app) handleSessionBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req sharedtypes.BindSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := a.svc.BindSession(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": resp})
}

func (a *app) handleSessionSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req sessionSignRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "lock":
		resp, err := a.svc.SignLock(r.Context(), sharedtypes.SignLockRequest{
			Chain: req.Chain, TraceID: req.TraceID, SrcChainID: req.SrcChainID, DstChainID: req.DstChainID,
			Asset: req.Asset, Amount: req.Amount, Sender: req.Sender, Recipient: req.Recipient,
			KeyID: req.KeyID, Nonce: req.Nonce, ExpireAt: req.ExpireAt,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": resp})
		return
	case "mint", "mintorunlock", "settle":
		resp, err := a.svc.SignMintOrUnlock(r.Context(), sharedtypes.SignMintOrUnlockRequest{
			Chain: req.Chain, TraceID: req.TraceID, SrcChainID: req.SrcChainID, DstChainID: req.DstChainID,
			Asset: req.Asset, Amount: req.Amount, Sender: req.Sender, Recipient: req.Recipient,
			KeyID: req.KeyID, Nonce: req.Nonce, ExpireAt: req.ExpireAt,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": resp})
		return
	case "refund", "refundv2":
		resp, err := a.svc.SignRefundV2(r.Context(), sharedtypes.SignRefundV2Request{
			Chain: req.Chain, TraceID: req.TraceID, KeyID: req.KeyID, Nonce: req.Nonce, ExpireAt: req.ExpireAt,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": resp})
		return
	default:
		writeError(w, http.StatusBadRequest, errors.New("unsupported action, expected lock/mint/refund"))
		return
	}
}

func (a *app) handleReceiptHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req sharedtypes.BuildReceiptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp, err := a.svc.BuildReceipt(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": resp})
}

func (a *app) handleVerifyPeerEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req sharedtypes.CrossChainExecutionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.svc.VerifyPeerCrossMessage(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleVerifyTargetEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req sharedtypes.TargetExecutionEvidenceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.svc.VerifyTargetExecutionEvidence(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) remoteSummary() tee.RemoteChainContextSummary {
	if a == nil {
		return tee.RemoteChainContextSummary{Loaded: false}
	}
	return a.remoteCtx.Summary()
}

func loadRemoteContext(ctx context.Context) (*tee.RemoteChainContext, error) {
	stateURL := envOrDefault("TEE_CHAIN_STATE_CONFIG_URL", "http://127.0.0.1:18081/v1/state/config/raw")
	artifactURL := envOrDefault("TEE_CHAIN_ARTIFACT_INDEX_URL", "http://127.0.0.1:18081/v1/artifacts/index")
	schemaURL := envOrDefault("TEE_EVENT_SCHEMA_URL", "http://127.0.0.1:18082/v1/events/schema")
	exampleURL := envOrDefault("TEE_EVENT_EXAMPLE_URL", "http://127.0.0.1:18082/v1/events/example")
	return tee.LoadRemoteChainContext(ctx, stateURL, artifactURL, schemaURL, exampleURL)
}

func splitCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func envOrDefault(key string, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return raw
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write json response failed: %v", err)
	}
}
