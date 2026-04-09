package chain

import (
	"context"

	"tdid-final/tee"
)

type FiscoClient struct {
	invoker *HTTPInvokeClient
}

func NewFiscoClient(adapter tee.ChainAdapter, baseURL string) *FiscoClient {
	return &FiscoClient{invoker: NewHTTPInvokeClient(adapter, map[tee.ChainKind]string{tee.ChainFISCO: baseURL}, nil)}
}

func (c *FiscoClient) Invoke(ctx context.Context, method string, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invokeAuto(ctx, tee.ChainFISCO, method, payload)
}

func (c *FiscoClient) Lock(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "lock", payload)
}

func (c *FiscoClient) MintOrUnlock(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "mintOrUnlock", payload)
}

func (c *FiscoClient) MintOrUnlockWithProof(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "mintOrUnlockWithProof", payload)
}

func (c *FiscoClient) BuildSourceLockProof(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "BuildSourceLockProof", payload)
}

func (c *FiscoClient) EncodeSourceLockProofPayload(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "EncodeSourceLockProofPayload", payload)
}

func (c *FiscoClient) CommitV2(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "commitV2", payload)
}

func (c *FiscoClient) RefundV2(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFISCO, targetGateway, "refundV2", payload)
}
