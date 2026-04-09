package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tdid-final/tee"
)

type InvokeRequest struct {
	ChainID string         `json:"chainId"`
	Target  string         `json:"target"`
	Method  string         `json:"method"`
	Payload map[string]any `json:"payload"`
}

type InvokeResponse struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
	Err  string          `json:"err,omitempty"`
}

type targetKind string

const (
	targetGateway targetKind = "gateway"
	targetAudit   targetKind = "audit"
)

type HTTPInvokeClient struct {
	adapter    tee.ChainAdapter
	baseURL    map[tee.ChainKind]string
	httpClient *http.Client
}

func NewHTTPInvokeClient(adapter tee.ChainAdapter, baseURL map[tee.ChainKind]string, httpClient *http.Client) *HTTPInvokeClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &http.Transport{DisableKeepAlives: true},
		}
	}
	return &HTTPInvokeClient{adapter: adapter, baseURL: baseURL, httpClient: httpClient}
}

func (c *HTTPInvokeClient) invoke(ctx context.Context, chain tee.ChainKind, target targetKind, method string, payload map[string]any) (InvokeResponse, error) {
	start := time.Now()
	chainID, err := c.adapter.ChainID(chain)
	if err != nil {
		return InvokeResponse{}, err
	}
	targetAddr, err := c.resolveTarget(chain, method, target)
	if err != nil {
		return InvokeResponse{}, err
	}
	base := strings.TrimRight(c.baseURL[chain], "/")
	if base == "" {
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeChainConfig, "missing base url for chain", fmt.Errorf("chain=%s", chain))
	}
	body := InvokeRequest{ChainID: chainID, Target: targetAddr, Method: method, Payload: payload}
	raw, err := json.Marshal(body)
	if err != nil {
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeInvalidInput, "failed to marshal invoke request", err)
	}
	requestBytes := len(raw)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/invoke", bytes.NewReader(raw))
	if err != nil {
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeInvalidInput, "failed to build http request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		chainEvalLog("chain=%s method=%s target=%s request_bytes=%d duration_ms=%d error=%q", chain, method, target, requestBytes, time.Since(start).Milliseconds(), err.Error())
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeChainConfig, "gateway invoke failed", err)
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		chainEvalLog("chain=%s method=%s target=%s request_bytes=%d http_status=%d duration_ms=%d read_error=%q", chain, method, target, requestBytes, resp.StatusCode, time.Since(start).Milliseconds(), err.Error())
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeChainConfig, "failed to read gateway response", err)
	}
	responseBytes := len(respRaw)
	var out InvokeResponse
	if err := json.Unmarshal(respRaw, &out); err != nil {
		chainEvalLog("chain=%s method=%s target=%s request_bytes=%d response_bytes=%d http_status=%d duration_ms=%d decode_error=%q", chain, method, target, requestBytes, responseBytes, resp.StatusCode, time.Since(start).Milliseconds(), err.Error())
		return InvokeResponse{}, tee.NewTEEError(tee.ErrCodeChainConfig, "invalid gateway response json", err)
	}
	if resp.StatusCode >= 400 || !out.OK {
		chainEvalLog("chain=%s method=%s target=%s request_bytes=%d response_bytes=%d http_status=%d duration_ms=%d ok=%v err=%q", chain, method, target, requestBytes, responseBytes, resp.StatusCode, time.Since(start).Milliseconds(), out.OK, out.Err)
		return out, tee.NewTEEError(tee.ErrCodeChainConfig, "gateway invoke returned error", fmt.Errorf("status=%d err=%s", resp.StatusCode, out.Err))
	}
	chainEvalLog("chain=%s method=%s target=%s request_bytes=%d response_bytes=%d http_status=%d duration_ms=%d ok=%v", chain, method, target, requestBytes, responseBytes, resp.StatusCode, time.Since(start).Milliseconds(), out.OK)
	return out, nil
}

func (c *HTTPInvokeClient) invokeAuto(ctx context.Context, chain tee.ChainKind, method string, payload map[string]any) (InvokeResponse, error) {
	t := targetGateway
	if isAuditMethod(method) {
		t = targetAudit
	}
	return c.invoke(ctx, chain, t, method, payload)
}

func (c *HTTPInvokeClient) resolveTarget(chain tee.ChainKind, method string, target targetKind) (string, error) {
	if target == targetAudit || isAuditMethod(method) {
		return c.adapter.AuditTarget(chain)
	}
	return c.adapter.GatewayTarget(chain)
}

func isAuditMethod(method string) bool {
	methodKey := strings.ToLower(strings.TrimSpace(method))
	switch methodKey {
	case "submitreceiptsrc", "submitreceiptdst", "matchreceipt", "match", "getstatus":
		return true
	default:
		return false
	}
}
