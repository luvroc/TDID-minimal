package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sharedtypes "tdid-final/shared/types"
)

type HTTPEnclaveClient struct {
	baseURL    string
	httpClient *http.Client
}

type enclaveEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type enclaveNodeStatusBody struct {
	OK           bool                     `json:"ok"`
	NodeIdentity sharedtypes.NodeIdentity `json:"nodeIdentity"`
	Session      *sharedtypes.CurrentSessionResponse
}

func NewHTTPEnclaveClient(baseURL string, httpClient *http.Client) *HTTPEnclaveClient {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		}
	}
	return &HTTPEnclaveClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *HTTPEnclaveClient) InitNode(ctx context.Context) error {
	_, err := c.postNoData(ctx, "/v1/tdid/node/init", map[string]any{})
	return err
}

func (c *HTTPEnclaveClient) GetNodeIdentity(ctx context.Context) (sharedtypes.NodeIdentity, error) {
	var body enclaveNodeStatusBody
	if err := c.get(ctx, "/v1/tdid/node/status", &body); err != nil {
		return sharedtypes.NodeIdentity{}, err
	}
	return body.NodeIdentity, nil
}

func (c *HTTPEnclaveClient) BindSession(ctx context.Context, req sharedtypes.BindSessionRequest) (sharedtypes.BindSessionResponse, error) {
	var out sharedtypes.BindSessionResponse
	if err := c.postWithData(ctx, "/v1/tdid/session/bind", req, &out); err != nil {
		return sharedtypes.BindSessionResponse{}, err
	}
	return out, nil
}

func (c *HTTPEnclaveClient) CurrentSession(ctx context.Context) (*sharedtypes.CurrentSessionResponse, error) {
	var body enclaveNodeStatusBody
	if err := c.get(ctx, "/v1/tdid/node/status", &body); err != nil {
		return nil, err
	}
	return body.Session, nil
}

func (c *HTTPEnclaveClient) SignLock(ctx context.Context, req sharedtypes.SignLockRequest) (sharedtypes.SignedPayload, error) {
	payload := map[string]any{
		"action":     "lock",
		"chain":      req.Chain,
		"traceId":    req.TraceID,
		"srcChainId": req.SrcChainID,
		"dstChainId": req.DstChainID,
		"asset":      req.Asset,
		"amount":     req.Amount,
		"sender":     req.Sender,
		"recipient":  req.Recipient,
		"keyId":      req.KeyID,
		"nonce":      req.Nonce,
		"expireAt":   req.ExpireAt,
	}
	var out sharedtypes.SignedPayload
	if err := c.postWithData(ctx, "/v1/tdid/session/sign", payload, &out); err != nil {
		return sharedtypes.SignedPayload{}, err
	}
	return out, nil
}

func (c *HTTPEnclaveClient) SignMintOrUnlock(ctx context.Context, req sharedtypes.SignMintOrUnlockRequest) (sharedtypes.SignedPayload, error) {
	payload := map[string]any{
		"action":     "mint",
		"chain":      req.Chain,
		"traceId":    req.TraceID,
		"srcChainId": req.SrcChainID,
		"dstChainId": req.DstChainID,
		"asset":      req.Asset,
		"amount":     req.Amount,
		"sender":     req.Sender,
		"recipient":  req.Recipient,
		"keyId":      req.KeyID,
		"nonce":      req.Nonce,
		"expireAt":   req.ExpireAt,
	}
	var out sharedtypes.SignedPayload
	if err := c.postWithData(ctx, "/v1/tdid/session/sign", payload, &out); err != nil {
		return sharedtypes.SignedPayload{}, err
	}
	return out, nil
}

func (c *HTTPEnclaveClient) SignRefundV2(ctx context.Context, req sharedtypes.SignRefundV2Request) (sharedtypes.SignedPayload, error) {
	payload := map[string]any{
		"action":   "refund",
		"chain":    req.Chain,
		"traceId":  req.TraceID,
		"keyId":    req.KeyID,
		"nonce":    req.Nonce,
		"expireAt": req.ExpireAt,
	}
	var out sharedtypes.SignedPayload
	if err := c.postWithData(ctx, "/v1/tdid/session/sign", payload, &out); err != nil {
		return sharedtypes.SignedPayload{}, err
	}
	return out, nil
}

func (c *HTTPEnclaveClient) VerifyPeerCrossMessage(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error {
	_, err := c.postNoData(ctx, "/v1/tdid/evidence/verify-peer", req)
	return err
}

func (c *HTTPEnclaveClient) VerifyTargetExecutionEvidence(ctx context.Context, req sharedtypes.TargetExecutionEvidenceRequest) error {
	_, err := c.postNoData(ctx, "/v1/tdid/evidence/verify-target", req)
	return err
}

func (c *HTTPEnclaveClient) BuildReceipt(ctx context.Context, req sharedtypes.BuildReceiptRequest) (sharedtypes.BuildReceiptResponse, error) {
	var out sharedtypes.BuildReceiptResponse
	if err := c.postWithData(ctx, "/v1/tdid/receipt/hash", req, &out); err != nil {
		return sharedtypes.BuildReceiptResponse{}, err
	}
	return out, nil
}

func (c *HTTPEnclaveClient) Health(ctx context.Context) error {
	var body map[string]any
	return c.get(ctx, "/health", &body)
}

func (c *HTTPEnclaveClient) postNoData(ctx context.Context, path string, req any) (enclaveEnvelope, error) {
	raw, err := c.post(ctx, path, req)
	if err != nil {
		return enclaveEnvelope{}, err
	}
	var env enclaveEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return enclaveEnvelope{}, fmt.Errorf("decode response envelope failed: %w", err)
	}
	if !env.OK {
		return enclaveEnvelope{}, fmt.Errorf("enclave api error: %s", strings.TrimSpace(env.Error))
	}
	return env, nil
}

func (c *HTTPEnclaveClient) postWithData(ctx context.Context, path string, req any, out any) error {
	env, err := c.postNoData(ctx, path, req)
	if err != nil {
		return err
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("enclave api response data is empty")
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode response data failed: %w", err)
	}
	return nil
}

func (c *HTTPEnclaveClient) post(ctx context.Context, path string, req any) ([]byte, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("http enclave client is not initialized")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *HTTPEnclaveClient) get(ctx context.Context, path string, out any) error {
	if c == nil || c.httpClient == nil {
		return fmt.Errorf("http enclave client is not initialized")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}
	return nil
}
