package core

import (
	"context"

	sharedtypes "tdid-final/shared/types"
)

type Service interface {
	InitNode(ctx context.Context) error
	GetNodeIdentity(ctx context.Context) (sharedtypes.NodeIdentity, error)
	BindSession(ctx context.Context, req sharedtypes.BindSessionRequest) (sharedtypes.BindSessionResponse, error)
	CurrentSession(ctx context.Context) (*sharedtypes.CurrentSessionResponse, error)
	SignLock(ctx context.Context, req sharedtypes.SignLockRequest) (sharedtypes.SignedPayload, error)
	SignMintOrUnlock(ctx context.Context, req sharedtypes.SignMintOrUnlockRequest) (sharedtypes.SignedPayload, error)
	SignRefundV2(ctx context.Context, req sharedtypes.SignRefundV2Request) (sharedtypes.SignedPayload, error)
	BuildReceipt(ctx context.Context, req sharedtypes.BuildReceiptRequest) (sharedtypes.BuildReceiptResponse, error)
	VerifyPeerCrossMessage(ctx context.Context, req sharedtypes.CrossChainExecutionRequest) error
	VerifyTargetExecutionEvidence(ctx context.Context, req sharedtypes.TargetExecutionEvidenceRequest) error
	Health(ctx context.Context) error
}
