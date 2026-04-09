package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tdid-final/tee"
)

func TestInvokeAutoRoutesGatewayAndAuditByMethod(t *testing.T) {
	cfg := tee.DefaultConfig()
	cfg.Chains[tee.ChainFISCO] = tee.ChainConfig{
		ChainID:         "fisco",
		Gateway:         "0xgateway",
		Audit:           "0xaudit",
		SessionRegistry: "0xsession",
		ExpireAtUnit:    tee.ExpireAtMillisecond,
	}
	adapter, err := tee.NewConfigChainAdapter(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	var got []InvokeRequest
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InvokeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(InvokeResponse{OK: false, Err: err.Error()})
			return
		}
		got = append(got, req)
		_ = json.NewEncoder(w).Encode(InvokeResponse{OK: true, Data: json.RawMessage(`{"ok":true}`)})
	}))
	defer svr.Close()

	invoker := NewHTTPInvokeClient(adapter, map[tee.ChainKind]string{tee.ChainFISCO: svr.URL}, nil)
	if _, err := invoker.invokeAuto(context.Background(), tee.ChainFISCO, "lock", map[string]any{"traceId": "0x1"}); err != nil {
		t.Fatalf("lock invoke failed: %v", err)
	}
	if _, err := invoker.invokeAuto(context.Background(), tee.ChainFISCO, "SubmitReceiptSrc", map[string]any{"traceId": "0x2"}); err != nil {
		t.Fatalf("submit receipt invoke failed: %v", err)
	}
	if _, err := invoker.invokeAuto(context.Background(), tee.ChainFISCO, "getStatus", map[string]any{"traceId": "0x3"}); err != nil {
		t.Fatalf("get status invoke failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(got))
	}
	if got[0].Target != "0xgateway" {
		t.Fatalf("lock should target gateway, got %s", got[0].Target)
	}
	if got[1].Target != "0xaudit" {
		t.Fatalf("SubmitReceiptSrc should target audit, got %s", got[1].Target)
	}
	if got[2].Target != "0xaudit" {
		t.Fatalf("getStatus should target audit, got %s", got[2].Target)
	}
}

func TestFiscoClientAndAuditClientInvoke(t *testing.T) {
	cfg := tee.DefaultConfig()
	cfg.Chains[tee.ChainFISCO] = tee.ChainConfig{
		ChainID:         "fisco",
		Gateway:         "0xgateway",
		Audit:           "0xaudit",
		SessionRegistry: "0xsession",
		ExpireAtUnit:    tee.ExpireAtMillisecond,
	}
	adapter, err := tee.NewConfigChainAdapter(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	var got []InvokeRequest
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req InvokeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(InvokeResponse{OK: false, Err: err.Error()})
			return
		}
		got = append(got, req)
		_ = json.NewEncoder(w).Encode(InvokeResponse{OK: true, Data: json.RawMessage(`{"txHash":"0xabc"}`)})
	}))
	defer svr.Close()

	fiscoClient := NewFiscoClient(adapter, svr.URL)
	auditClient := NewAuditClient(tee.ChainFISCO, adapter, svr.URL)

	if _, err := fiscoClient.Lock(context.Background(), map[string]any{"traceId": "0x1"}); err != nil {
		t.Fatalf("fisco lock failed: %v", err)
	}
	if _, err := fiscoClient.RefundV2(context.Background(), map[string]any{"traceId": "0x1", "keyId": "0x2"}); err != nil {
		t.Fatalf("fisco refundV2 failed: %v", err)
	}
	if _, err := auditClient.SubmitReceiptSrc(context.Background(), map[string]any{"traceId": "0x1"}); err != nil {
		t.Fatalf("audit submit src failed: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(got))
	}
	if got[0].Method != "lock" || got[0].Target != "0xgateway" {
		t.Fatalf("unexpected lock routing: %+v", got[0])
	}
	if got[1].Method != "refundV2" || got[1].Target != "0xgateway" {
		t.Fatalf("unexpected refundV2 routing: %+v", got[1])
	}
	if got[2].Method != "submitReceiptSrc" || got[2].Target != "0xaudit" {
		t.Fatalf("unexpected submit routing: %+v", got[2])
	}
}

func TestStage9_RefundV2MinimalPayloadFallback(t *testing.T) {
	cfg := tee.DefaultConfig()
	cfg.Chains[tee.ChainFISCO] = tee.ChainConfig{
		ChainID:         "fisco",
		Gateway:         "0xgateway",
		Audit:           "0xaudit",
		SessionRegistry: "0xsession",
		ExpireAtUnit:    tee.ExpireAtMillisecond,
	}
	adapter, err := tee.NewConfigChainAdapter(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	var got InvokeRequest
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(InvokeResponse{OK: false, Err: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(InvokeResponse{OK: true, Data: json.RawMessage(`{"ok":true}`)})
	}))
	defer svr.Close()

	fiscoClient := NewFiscoClient(adapter, svr.URL)
	payload := map[string]any{
		"traceId": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"keyId":   "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := fiscoClient.RefundV2(context.Background(), payload); err != nil {
		t.Fatalf("refundV2 minimal payload failed: %v", err)
	}

	if got.Method != "refundV2" || got.Target != "0xgateway" {
		t.Fatalf("unexpected refundV2 invocation: %+v", got)
	}
	if len(got.Payload) != 2 {
		t.Fatalf("refundV2 should keep minimal payload, got: %+v", got.Payload)
	}
	if _, ok := got.Payload["sessSig"]; ok {
		t.Fatalf("refundV2 minimal fallback should not require sessSig")
	}
}
