package chain

import (
	"context"

	"tdid-final/tee"
)

type FabricClient struct {
	invoker *HTTPInvokeClient
}

func NewFabricClient(adapter tee.ChainAdapter, baseURL string) *FabricClient {
	return &FabricClient{invoker: NewHTTPInvokeClient(adapter, map[tee.ChainKind]string{tee.ChainFabric: baseURL}, nil)}
}

func (c *FabricClient) Invoke(ctx context.Context, method string, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invokeAuto(ctx, tee.ChainFabric, method, payload)
}

func (c *FabricClient) Lock(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "lock", payload)
}

func (c *FabricClient) MintOrUnlock(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "mintOrUnlock", payload)
}

func (c *FabricClient) MintOrUnlockWithProof(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "mintOrUnlockWithProof", payload)
}

func (c *FabricClient) BuildSourceLockProof(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "BuildSourceLockProof", payload)
}

func (c *FabricClient) EncodeSourceLockProofPayload(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "EncodeSourceLockProofPayload", payload)
}

func (c *FabricClient) CommitV2(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "commitV2", payload)
}

func (c *FabricClient) RefundV2(ctx context.Context, payload map[string]any) (InvokeResponse, error) {
	return c.invoker.invoke(ctx, tee.ChainFabric, targetGateway, "refundV2", payload)
}
