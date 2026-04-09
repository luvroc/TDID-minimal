package chain

import (
	"context"
	"encoding/json"
)

type ChainWriteResult struct {
	SessionID  string          `json:"sessionId,omitempty"`
	TransferID string          `json:"transferId,omitempty"`
	TraceID    string          `json:"traceId,omitempty"`
	TxHash     string          `json:"txHash,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type SourceLockRequest struct {
	SessionID   string
	TransferID  string
	TraceID     string
	SrcChainID  string
	DstChainID  string
	Asset       string
	Amount      string
	Sender      string
	Recipient   string
	KeyID       string
	Nonce       uint64
	ExpireAt    int64
	PayloadHash string
	SessSig     string
}

type SourceLockProofRequest struct {
	TraceID  string
	Attester string
	Signer   string
}

type SourceLockProofPayloadRequest struct {
	ProofJSON string
}

type SourceCommitRequest struct {
	TraceID         string
	KeyID           string
	TargetChainTx   string
	TargetReceipt   string
	TargetChainID   string
	TargetChainHash string
}

type TargetExecuteRequest struct {
	SessionID         string
	TransferID        string
	TraceID           string
	SrcChainID        string
	DstChainID        string
	Asset             string
	Amount            string
	Sender            string
	Recipient         string
	KeyID             string
	Nonce             uint64
	ExpireAt          int64
	PayloadHash       string
	SessSig           string
	SourceLockTx      string
	SourceReceipt     string
	SourceSessSig     string
	SourcePayloadHash string
	SourceLockProof   string
}

type AuditReceiptRequest struct {
	TransferID  string
	TraceID     string
	ReceiptHash string
}

type AuditMatchRequest struct {
	TransferID string
	TraceID    string
}

type SourceChainClient interface {
	InvokeSourceLock(ctx context.Context, req SourceLockRequest) (ChainWriteResult, error)
	BuildSourceLockProof(ctx context.Context, req SourceLockProofRequest) (string, error)
	EncodeSourceLockProofPayload(ctx context.Context, req SourceLockProofPayloadRequest) (string, error)
	InvokeSourceCommit(ctx context.Context, req SourceCommitRequest) (ChainWriteResult, error)
}

type TargetChainClient interface {
	InvokeTargetExecute(ctx context.Context, req TargetExecuteRequest) (ChainWriteResult, error)
}

type AuditChainClient interface {
	SubmitSourceReceipt(ctx context.Context, req AuditReceiptRequest) (ChainWriteResult, error)
	SubmitTargetReceipt(ctx context.Context, req AuditReceiptRequest) (ChainWriteResult, error)
	MatchTransfer(ctx context.Context, req AuditMatchRequest) (ChainWriteResult, error)
}

type RegistrationClient interface {
	RegisterNode(ctx context.Context, payload map[string]any) (ChainWriteResult, error)
	RegisterSession(ctx context.Context, payload map[string]any) (ChainWriteResult, error)
}

type LegacyGatewayClient interface {
	Lock(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	MintOrUnlock(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	MintOrUnlockWithProof(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	BuildSourceLockProof(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	EncodeSourceLockProofPayload(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	CommitV2(ctx context.Context, payload map[string]any) (InvokeResponse, error)
}

type LegacyAuditClient interface {
	SubmitReceiptSrc(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	SubmitReceiptDst(ctx context.Context, payload map[string]any) (InvokeResponse, error)
	MatchReceipt(ctx context.Context, payload map[string]any) (InvokeResponse, error)
}

type BridgeSourceChainClient struct {
	gateway LegacyGatewayClient
}

type BridgeTargetChainClient struct {
	gateway LegacyGatewayClient
}

type BridgeAuditChainClient struct {
	audit LegacyAuditClient
}

func NewBridgeSourceChainClient(gateway LegacyGatewayClient) *BridgeSourceChainClient {
	return &BridgeSourceChainClient{gateway: gateway}
}

func NewBridgeTargetChainClient(gateway LegacyGatewayClient) *BridgeTargetChainClient {
	return &BridgeTargetChainClient{gateway: gateway}
}

func NewBridgeAuditChainClient(audit LegacyAuditClient) *BridgeAuditChainClient {
	return &BridgeAuditChainClient{audit: audit}
}

func (c *BridgeSourceChainClient) InvokeSourceLock(ctx context.Context, req SourceLockRequest) (ChainWriteResult, error) {
	resp, err := c.gateway.Lock(ctx, map[string]any{
		"sessionId":   req.SessionID,
		"transferId":  req.TransferID,
		"traceId":     req.TraceID,
		"srcChainId":  req.SrcChainID,
		"dstChainId":  req.DstChainID,
		"asset":       req.Asset,
		"amount":      req.Amount,
		"sender":      req.Sender,
		"recipient":   req.Recipient,
		"keyId":       req.KeyID,
		"nonce":       req.Nonce,
		"expireAt":    req.ExpireAt,
		"payloadHash": req.PayloadHash,
		"sessSig":     req.SessSig,
	})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{SessionID: req.SessionID, TransferID: req.TransferID, TraceID: req.TraceID, TxHash: extractTxHash(resp.Data), Data: resp.Data}, nil
}

func (c *BridgeSourceChainClient) BuildSourceLockProof(ctx context.Context, req SourceLockProofRequest) (string, error) {
	resp, err := c.gateway.BuildSourceLockProof(ctx, map[string]any{
		"traceId":  req.TraceID,
		"attester": req.Attester,
		"signer":   req.Signer,
	})
	if err != nil {
		return "", err
	}
	return extractString(resp.Data), nil
}

func (c *BridgeSourceChainClient) EncodeSourceLockProofPayload(ctx context.Context, req SourceLockProofPayloadRequest) (string, error) {
	resp, err := c.gateway.EncodeSourceLockProofPayload(ctx, map[string]any{
		"proofJSON": req.ProofJSON,
	})
	if err != nil {
		return "", err
	}
	return extractString(resp.Data), nil
}

func (c *BridgeSourceChainClient) InvokeSourceCommit(ctx context.Context, req SourceCommitRequest) (ChainWriteResult, error) {
	resp, err := c.gateway.CommitV2(ctx, map[string]any{
		"traceId":         req.TraceID,
		"keyId":           req.KeyID,
		"targetChainTx":   req.TargetChainTx,
		"targetReceipt":   req.TargetReceipt,
		"targetChainId":   req.TargetChainID,
		"targetChainHash": req.TargetChainHash,
	})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{TraceID: req.TraceID, TxHash: extractTxHash(resp.Data), Data: resp.Data}, nil
}

func (c *BridgeTargetChainClient) InvokeTargetExecute(ctx context.Context, req TargetExecuteRequest) (ChainWriteResult, error) {
	resp, err := c.gateway.MintOrUnlockWithProof(ctx, map[string]any{
		"sessionId":         req.SessionID,
		"transferId":        req.TransferID,
		"traceId":           req.TraceID,
		"srcChainId":        req.SrcChainID,
		"dstChainId":        req.DstChainID,
		"asset":             req.Asset,
		"amount":            req.Amount,
		"sender":            req.Sender,
		"recipient":         req.Recipient,
		"keyId":             req.KeyID,
		"nonce":             req.Nonce,
		"expireAt":          req.ExpireAt,
		"payloadHash":       req.PayloadHash,
		"sessSig":           req.SessSig,
		"sourceLockTx":      req.SourceLockTx,
		"sourceReceipt":     req.SourceReceipt,
		"sourceSessSig":     req.SourceSessSig,
		"sourcePayloadHash": req.SourcePayloadHash,
		"proofPayload":      req.SourceLockProof,
	})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{SessionID: req.SessionID, TransferID: req.TransferID, TraceID: req.TraceID, TxHash: extractTxHash(resp.Data), Data: resp.Data}, nil
}

func (c *BridgeAuditChainClient) SubmitSourceReceipt(ctx context.Context, req AuditReceiptRequest) (ChainWriteResult, error) {
	resp, err := c.audit.SubmitReceiptSrc(ctx, map[string]any{"transferId": req.TransferID, "traceId": req.TraceID, "receiptHash": req.ReceiptHash})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{TransferID: req.TransferID, TraceID: req.TraceID, Data: resp.Data}, nil
}

func (c *BridgeAuditChainClient) SubmitTargetReceipt(ctx context.Context, req AuditReceiptRequest) (ChainWriteResult, error) {
	resp, err := c.audit.SubmitReceiptDst(ctx, map[string]any{"transferId": req.TransferID, "traceId": req.TraceID, "receiptHash": req.ReceiptHash})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{TransferID: req.TransferID, TraceID: req.TraceID, Data: resp.Data}, nil
}

func (c *BridgeAuditChainClient) MatchTransfer(ctx context.Context, req AuditMatchRequest) (ChainWriteResult, error) {
	resp, err := c.audit.MatchReceipt(ctx, map[string]any{"transferId": req.TransferID, "traceId": req.TraceID})
	if err != nil {
		return ChainWriteResult{}, err
	}
	return ChainWriteResult{TransferID: req.TransferID, TraceID: req.TraceID, Data: resp.Data}, nil
}

func extractTxHash(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return ""
	}
	v, ok := body["txHash"].(string)
	if !ok {
		return ""
	}
	return v
}

func extractString(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		return asString
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return ""
	}
	for _, key := range []string{"proofPayload", "proof", "result", "data", "value"} {
		if v, ok := body[key].(string); ok {
			return v
		}
	}
	return ""
}
