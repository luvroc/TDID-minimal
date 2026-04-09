package chain

import (
	"context"

	"tdid-final/tee"
)

type AuditClient struct {
	chain   tee.ChainKind
	invoker *HTTPInvokeClient
}

func NewAuditClient(chain tee.ChainKind, adapter tee.ChainAdapter, baseURL string) *AuditClient {
	return &AuditClient{
		chain:   chain,
		invoker: NewHTTPInvokeClient(adapter, map[tee.ChainKind]string{chain: baseURL}, nil),
	}
}

func (c *AuditClient) Invoke(ctx context.Context, method string, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, c.chain, targetAudit, method, payload)
}

func (c *AuditClient) SubmitReceiptSrc(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, c.chain, targetAudit, "submitReceiptSrc", payload)
}

func (c *AuditClient) SubmitReceiptDst(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, c.chain, targetAudit, "submitReceiptDst", payload)
}

func (c *AuditClient) MatchReceipt(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, c.chain, targetAudit, "matchReceipt", payload)
}
